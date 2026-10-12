package artifacts

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	aliyunoss "github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss"
	osscredentials "github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss/credentials"
)

// OSSConfig configures the Alibaba Cloud OSS-native artifact adapter. Cloud
// deployments should use ECS RAM-role credentials and HTTPS; static keys are
// retained only for isolated contract tests and local development.
type OSSConfig struct {
	Endpoint            string
	Region              string
	Bucket              string
	Prefix              string
	MaxBytes            int64
	AllowHTTP           bool
	AccessKey           string
	SecretKey           string
	SecurityToken       string
	ECSRAMRoleName      string
	CredentialsProvider osscredentials.CredentialsProvider
}

type OSSStore struct {
	client   ossAPI
	bucket   string
	prefix   string
	maxBytes int64
}

type ossAPI interface {
	GetBucketInfo(context.Context, *aliyunoss.GetBucketInfoRequest, ...func(*aliyunoss.Options)) (*aliyunoss.GetBucketInfoResult, error)
	GetBucketVersioning(context.Context, *aliyunoss.GetBucketVersioningRequest, ...func(*aliyunoss.Options)) (*aliyunoss.GetBucketVersioningResult, error)
	PutObject(context.Context, *aliyunoss.PutObjectRequest, ...func(*aliyunoss.Options)) (*aliyunoss.PutObjectResult, error)
	GetObject(context.Context, *aliyunoss.GetObjectRequest, ...func(*aliyunoss.Options)) (*aliyunoss.GetObjectResult, error)
	HeadObject(context.Context, *aliyunoss.HeadObjectRequest, ...func(*aliyunoss.Options)) (*aliyunoss.HeadObjectResult, error)
	DeleteObject(context.Context, *aliyunoss.DeleteObjectRequest, ...func(*aliyunoss.Options)) (*aliyunoss.DeleteObjectResult, error)
}

func NewOSSStore(cfg OSSConfig) (*OSSStore, error) {
	endpoint, err := url.Parse(cfg.Endpoint)
	if err != nil || endpoint.Host == "" || endpoint.User != nil || endpoint.Path != "" || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return nil, errors.New("OSS endpoint must be an absolute URL without path, userinfo, query, or fragment")
	}
	if endpoint.Scheme != "https" && !(cfg.AllowHTTP && endpoint.Scheme == "http") {
		return nil, errors.New("OSS endpoint must use HTTPS outside explicit local development")
	}
	if cfg.Region == "" || strings.TrimSpace(cfg.Region) != cfg.Region {
		return nil, errors.New("OSS region is required")
	}
	if cfg.Bucket == "" || strings.TrimSpace(cfg.Bucket) != cfg.Bucket || strings.ContainsAny(cfg.Bucket, "/\\:%") {
		return nil, errors.New("OSS bucket is invalid")
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
		return nil, errors.New("OSS max object size must not exceed 5 GiB for single-request immutable writes")
	}

	provider := cfg.CredentialsProvider
	if provider == nil {
		switch {
		case cfg.ECSRAMRoleName != "" && cfg.AccessKey == "" && cfg.SecretKey == "" && cfg.SecurityToken == "":
			provider = osscredentials.NewEcsRoleCredentialsProvider(osscredentials.EcsRamRole(cfg.ECSRAMRoleName))
		case cfg.ECSRAMRoleName == "" && cfg.AccessKey != "" && cfg.SecretKey != "":
			provider = osscredentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, cfg.SecurityToken)
		default:
			return nil, errors.New("configure exactly one OSS credential source: ECS RAM role or static contract-test credentials")
		}
	}
	clientConfig := aliyunoss.LoadDefaultConfig().
		WithRegion(cfg.Region).
		WithEndpoint(endpoint.Host).
		WithCredentialsProvider(provider).
		WithRetryMaxAttempts(3).
		WithConnectTimeout(3 * time.Second).
		WithReadWriteTimeout(15 * time.Second)
	client := aliyunoss.NewClient(clientConfig)
	return &OSSStore{client: client, bucket: cfg.Bucket, prefix: cfg.Prefix, maxBytes: cfg.MaxBytes}, nil
}

func (s *OSSStore) scopedKey(key string) (string, error) {
	if err := ValidateKey(key); err != nil {
		return "", err
	}
	if s.prefix != "" {
		return s.prefix + "/" + key, nil
	}
	return key, nil
}

// EnsureBucket checks the pre-provisioned bucket and never creates it.
func (s *OSSStore) EnsureBucket(ctx context.Context) error {
	_, err := s.client.GetBucketInfo(ctx, &aliyunoss.GetBucketInfoRequest{Bucket: aliyunoss.Ptr(s.bucket)})
	if err != nil {
		return classifyOSSError("bucket check", s.bucket, err)
	}
	versioning, err := s.client.GetBucketVersioning(ctx, &aliyunoss.GetBucketVersioningRequest{Bucket: aliyunoss.Ptr(s.bucket)})
	if err != nil {
		return classifyOSSError("bucket versioning check", s.bucket, err)
	}
	if versioning.VersionStatus != nil && *versioning.VersionStatus != string(aliyunoss.VersionSuspended) {
		return fmt.Errorf("artifact OSS bucket %q has unsupported versioning status %q; immutable PutIfAbsent requires versioning disabled/suspended because OSS may ignore forbid-overwrite", s.bucket, *versioning.VersionStatus)
	}
	return nil
}

func (s *OSSStore) PutIfAbsent(ctx context.Context, key string, data []byte, contentType string) (ObjectInfo, error) {
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
	md5sum := md5.Sum(data)
	md5Base64 := base64.StdEncoding.EncodeToString(md5sum[:])
	size := int64(len(data))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	_, putErr := s.client.PutObject(ctx, &aliyunoss.PutObjectRequest{
		Bucket: aliyunoss.Ptr(s.bucket), Key: aliyunoss.Ptr(scoped),
		Body: bytes.NewReader(data), ContentLength: &size, ContentType: &contentType,
		ContentMD5: &md5Base64, ForbidOverwrite: aliyunoss.Ptr("true"),
		Metadata: map[string]string{"sha256": sha},
	})
	if putErr != nil {
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return ObjectInfo{}, wrapStoreError("put", key, ErrUnknownOutcome, putErr)
		}
		var serviceErr *aliyunoss.ServiceError
		if errors.As(putErr, &serviceErr) && (serviceErr.Code == "FileAlreadyExists" || serviceErr.Code == "PreconditionFailed") {
			return s.verifyExisting(ctx, key, scoped, sha, size, ErrConflict, putErr)
		}
		return s.verifyExisting(ctx, key, scoped, sha, size, ErrUnknownOutcome, putErr)
	}
	return s.verifyExisting(ctx, key, scoped, sha, size, ErrUnknownOutcome, nil)
}

func (s *OSSStore) verifyExisting(ctx context.Context, key, scoped, sha string, size int64, fallback error, cause error) (ObjectInfo, error) {
	verifyCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	info, err := s.headScoped(verifyCtx, scoped)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ObjectInfo{}, wrapStoreError("put", key, fallback, errors.Join(cause, err))
		}
		return ObjectInfo{}, wrapStoreError("put", key, ErrUnknownOutcome, errors.Join(cause, err))
	}
	if info.SHA256 != sha || info.Size != size {
		return ObjectInfo{}, wrapStoreError("put", key, ErrConflict, cause)
	}
	info.Key = key
	return info, nil
}

func (s *OSSStore) Get(ctx context.Context, key string) ([]byte, ObjectInfo, error) {
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
	result, err := s.client.GetObject(ctx, &aliyunoss.GetObjectRequest{Bucket: aliyunoss.Ptr(s.bucket), Key: aliyunoss.Ptr(scoped)})
	if err != nil {
		return nil, ObjectInfo{}, classifyOSSError("get", key, err)
	}
	if result.Body == nil {
		return nil, ObjectInfo{}, wrapStoreError("get", key, ErrCorrupt, errors.New("OSS returned an empty response body"))
	}
	defer result.Body.Close()
	data, err := io.ReadAll(io.LimitReader(result.Body, s.maxBytes+1))
	if err != nil {
		return nil, ObjectInfo{}, classifyOSSError("get", key, err)
	}
	if int64(len(data)) > s.maxBytes {
		return nil, ObjectInfo{}, wrapStoreError("get", key, ErrTooLarge, nil)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != expected.SHA256 || int64(len(data)) != expected.Size {
		return nil, ObjectInfo{}, wrapStoreError("get", key, ErrCorrupt, nil)
	}
	expected.Key = key
	return data, expected, nil
}

func (s *OSSStore) Head(ctx context.Context, key string) (ObjectInfo, error) {
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

func (s *OSSStore) headScoped(ctx context.Context, scoped string) (ObjectInfo, error) {
	result, err := s.client.HeadObject(ctx, &aliyunoss.HeadObjectRequest{Bucket: aliyunoss.Ptr(s.bucket), Key: aliyunoss.Ptr(scoped)})
	if err != nil {
		return ObjectInfo{}, classifyOSSError("head", scoped, err)
	}
	sha := ""
	for name, value := range result.Metadata {
		if strings.EqualFold(name, "sha256") {
			sha = value
			break
		}
	}
	if len(sha) != 64 || result.ContentLength < 0 || result.ContentLength > s.maxBytes {
		return ObjectInfo{}, wrapStoreError("head", scoped, ErrCorrupt, errors.New("invalid OSS content length or missing SHA256 metadata"))
	}
	contentType := ""
	if result.ContentType != nil {
		contentType = *result.ContentType
	}
	versionID := ""
	if result.VersionId != nil {
		versionID = *result.VersionId
	}
	return ObjectInfo{Key: scoped, SHA256: strings.ToLower(sha), Size: result.ContentLength, ContentType: contentType, VersionID: versionID}, nil
}

func (s *OSSStore) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	scoped, err := s.scopedKey(key)
	if err != nil {
		return wrapStoreError("delete", key, ErrInvalidKey, err)
	}
	if _, err := s.client.DeleteObject(ctx, &aliyunoss.DeleteObjectRequest{Bucket: aliyunoss.Ptr(s.bucket), Key: aliyunoss.Ptr(scoped)}); err != nil {
		return classifyOSSError("delete", key, err)
	}
	return nil
}

func classifyOSSError(operation, key string, err error) error {
	var serviceErr *aliyunoss.ServiceError
	if errors.As(err, &serviceErr) {
		switch {
		case serviceErr.StatusCode == 403 || serviceErr.Code == "AccessDenied":
			return wrapStoreError(operation, key, ErrAccessDenied, err)
		case serviceErr.StatusCode == 404 || serviceErr.Code == "NoSuchKey" || serviceErr.Code == "NoSuchObject" || serviceErr.Code == "NoSuchBucket":
			return wrapStoreError(operation, key, ErrNotFound, err)
		}
	}
	return fmt.Errorf("artifact OSS %s %q: %w", operation, key, err)
}
