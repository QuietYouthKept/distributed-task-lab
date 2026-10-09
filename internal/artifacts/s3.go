package artifacts

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type S3Config struct {
	Endpoint          string
	Bucket            string
	Prefix            string
	AccessKey         string
	SecretKey         string
	Region            string
	MaxBytes          int64
	AllowHTTP         bool
	AllowBucketCreate bool
}

type S3Store struct {
	client            *minio.Client
	bucket            string
	prefix            string
	maxBytes          int64
	allowBucketCreate bool
}

func NewS3Store(cfg S3Config) (*S3Store, error) {
	endpoint, err := url.Parse(cfg.Endpoint)
	if err != nil || endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || endpoint.Path != "" {
		return nil, errors.New("S3 endpoint must be an absolute URL without path, userinfo, query, or fragment")
	}
	if endpoint.Scheme != "https" && !(cfg.AllowHTTP && endpoint.Scheme == "http") {
		return nil, errors.New("S3 endpoint must use HTTPS outside explicit local development")
	}
	if cfg.Bucket == "" || strings.TrimSpace(cfg.Bucket) != cfg.Bucket || strings.ContainsAny(cfg.Bucket, "/\\:%") {
		return nil, errors.New("S3 bucket is invalid")
	}
	if cfg.Prefix != "" {
		if err := ValidateKey(cfg.Prefix); err != nil {
			return nil, err
		}
	}
	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = 64 << 20
	}
	if cfg.MaxBytes > 5<<30 {
		return nil, errors.New("S3 max object size must not exceed 5 GiB for conditional single-request writes")
	}
	client, err := minio.New(endpoint.Host, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: endpoint.Scheme == "https", Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("create S3 client: %w", err)
	}
	return &S3Store{client: client, bucket: cfg.Bucket, prefix: cfg.Prefix, maxBytes: cfg.MaxBytes, allowBucketCreate: cfg.AllowBucketCreate}, nil
}

func (s *S3Store) scopedKey(key string) (string, error) {
	if err := ValidateKey(key); err != nil {
		return "", err
	}
	if s.prefix != "" {
		return s.prefix + "/" + key, nil
	}
	return key, nil
}

// EnsureBucket verifies the configured bucket exists. Creation is permitted
// only when explicitly enabled for local development/test environments.
func (s *S3Store) EnsureBucket(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return classifyS3Error("bucket check", s.bucket, err)
	}
	if exists {
		return nil
	}
	if !s.allowBucketCreate {
		return fmt.Errorf("configured artifact bucket is missing")
	}
	return s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{})
}

func (s *S3Store) PutIfAbsent(ctx context.Context, key string, data []byte, contentType string) (ObjectInfo, error) {
	if err := ctx.Err(); err != nil {
		return ObjectInfo{}, err
	}
	if int64(len(data)) > s.maxBytes {
		return ObjectInfo{}, wrapStoreError("put", key, ErrTooLarge, nil)
	}
	scoped, err := s.scopedKey(key)
	if err != nil {
		return ObjectInfo{}, wrapStoreError("put", key, ErrInvalidKey, err)
	}
	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])
	opts := minio.PutObjectOptions{
		ContentType:      contentType,
		UserMetadata:     map[string]string{"sha256": sha},
		DisableMultipart: true,
		SendContentMd5:   true,
	}
	// MinIO's S3-compatible conditional PUT provides atomic create-if-absent.
	// It is deliberately a single PUT (bounded below 5 GiB), not multipart.
	opts.SetMatchETagExcept("*")
	_, putErr := s.client.PutObject(ctx, s.bucket, scoped, bytes.NewReader(data), int64(len(data)), opts)
	if putErr != nil {
		response := minio.ToErrorResponse(putErr)
		if response.StatusCode == 403 || response.Code == "AccessDenied" {
			return ObjectInfo{}, wrapStoreError("put", key, ErrAccessDenied, putErr)
		}
		// A failed response can mean the server committed the object before the
		// connection broke, or another writer won the conditional create. Verify
		// by trusted SHA256 metadata and size; never use ETag as a content hash.
		if ctx.Err() != nil {
			return ObjectInfo{}, wrapStoreError("put", key, ErrUnknownOutcome, putErr)
		}
		verifyCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		existing, headErr := s.headScoped(verifyCtx, scoped)
		if headErr == nil {
			if existing.SHA256 == sha && existing.Size == int64(len(data)) {
				existing.Key = key
				return existing, nil
			}
			return ObjectInfo{}, wrapStoreError("put", key, ErrConflict, putErr)
		}
		if errors.Is(headErr, ErrAccessDenied) {
			return ObjectInfo{}, headErr
		}
		return ObjectInfo{}, wrapStoreError("put", key, ErrUnknownOutcome, errors.Join(putErr, headErr))
	}
	info, err := s.headScoped(ctx, scoped)
	if err != nil {
		return ObjectInfo{}, wrapStoreError("put verify", key, ErrUnknownOutcome, err)
	}
	if info.SHA256 != sha || info.Size != int64(len(data)) {
		return ObjectInfo{}, wrapStoreError("put verify", key, ErrCorrupt, nil)
	}
	info.Key = key
	return info, nil
}

func (s *S3Store) Get(ctx context.Context, key string) ([]byte, ObjectInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, ObjectInfo{}, err
	}
	scoped, err := s.scopedKey(key)
	if err != nil {
		return nil, ObjectInfo{}, wrapStoreError("get", key, ErrInvalidKey, err)
	}
	expected, err := s.headScoped(ctx, scoped)
	if err != nil {
		return nil, ObjectInfo{}, err
	}
	object, err := s.client.GetObject(ctx, s.bucket, scoped, minio.GetObjectOptions{})
	if err != nil {
		return nil, ObjectInfo{}, classifyS3Error("get", key, err)
	}
	defer object.Close()
	data, err := io.ReadAll(io.LimitReader(object, s.maxBytes+1))
	if err != nil {
		return nil, ObjectInfo{}, classifyS3Error("get", key, err)
	}
	if int64(len(data)) > s.maxBytes {
		return nil, ObjectInfo{}, wrapStoreError("get", key, ErrTooLarge, nil)
	}
	sum := sha256.Sum256(data)
	actual := hex.EncodeToString(sum[:])
	if actual != expected.SHA256 || int64(len(data)) != expected.Size {
		return nil, ObjectInfo{}, wrapStoreError("get", key, ErrCorrupt, nil)
	}
	expected.Key = key
	return data, expected, nil
}

func (s *S3Store) Head(ctx context.Context, key string) (ObjectInfo, error) {
	if err := ctx.Err(); err != nil {
		return ObjectInfo{}, err
	}
	scoped, err := s.scopedKey(key)
	if err != nil {
		return ObjectInfo{}, wrapStoreError("head", key, ErrInvalidKey, err)
	}
	info, err := s.headScoped(ctx, scoped)
	info.Key = key
	return info, err
}

func (s *S3Store) headScoped(ctx context.Context, scoped string) (ObjectInfo, error) {
	remote, err := s.client.StatObject(ctx, s.bucket, scoped, minio.StatObjectOptions{})
	if err != nil {
		return ObjectInfo{}, classifyS3Error("head", scoped, err)
	}
	if remote.Size < 0 || remote.Size > s.maxBytes {
		return ObjectInfo{}, wrapStoreError("head", scoped, ErrTooLarge, nil)
	}
	sha := ""
	for name, value := range remote.UserMetadata {
		if strings.EqualFold(name, "sha256") {
			sha = value
			break
		}
	}
	if sha == "" {
		sha = remote.Metadata.Get("X-Amz-Meta-Sha256")
	}
	if len(sha) != 64 {
		return ObjectInfo{}, wrapStoreError("head", scoped, ErrCorrupt, errors.New("missing SHA256 object metadata"))
	}
	return ObjectInfo{Key: scoped, SHA256: strings.ToLower(sha), Size: remote.Size, ContentType: remote.ContentType, VersionID: remote.VersionID}, nil
}

func (s *S3Store) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	scoped, err := s.scopedKey(key)
	if err != nil {
		return wrapStoreError("delete", key, ErrInvalidKey, err)
	}
	if err := s.client.RemoveObject(ctx, s.bucket, scoped, minio.RemoveObjectOptions{}); err != nil {
		return classifyS3Error("delete", key, err)
	}
	return nil
}

func classifyS3Error(operation, key string, err error) error {
	response := minio.ToErrorResponse(err)
	switch {
	case response.StatusCode == 403 || response.Code == "AccessDenied":
		return wrapStoreError(operation, key, ErrAccessDenied, err)
	case response.Code == "NoSuchKey" || response.Code == "NoSuchObject" || response.StatusCode == 404:
		return wrapStoreError(operation, key, ErrNotFound, err)
	default:
		return fmt.Errorf("artifact %s %q: %w", operation, key, err)
	}
}
