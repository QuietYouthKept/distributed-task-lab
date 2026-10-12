package artifacts

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	aliyunoss "github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss"
)

type fakeOSS struct {
	putRequest       *aliyunoss.PutObjectRequest
	headResult       *aliyunoss.HeadObjectResult
	headErr          error
	putErr           error
	getResult        *aliyunoss.GetObjectResult
	getErr           error
	deleteErr        error
	getBucketErr     error
	versioningStatus *string
	versioningErr    error
}

func (f *fakeOSS) GetBucketInfo(context.Context, *aliyunoss.GetBucketInfoRequest, ...func(*aliyunoss.Options)) (*aliyunoss.GetBucketInfoResult, error) {
	return &aliyunoss.GetBucketInfoResult{}, f.getBucketErr
}
func (f *fakeOSS) GetBucketVersioning(context.Context, *aliyunoss.GetBucketVersioningRequest, ...func(*aliyunoss.Options)) (*aliyunoss.GetBucketVersioningResult, error) {
	return &aliyunoss.GetBucketVersioningResult{VersionStatus: f.versioningStatus}, f.versioningErr
}
func (f *fakeOSS) PutObject(_ context.Context, request *aliyunoss.PutObjectRequest, _ ...func(*aliyunoss.Options)) (*aliyunoss.PutObjectResult, error) {
	f.putRequest = request
	return &aliyunoss.PutObjectResult{}, f.putErr
}
func (f *fakeOSS) GetObject(context.Context, *aliyunoss.GetObjectRequest, ...func(*aliyunoss.Options)) (*aliyunoss.GetObjectResult, error) {
	return f.getResult, f.getErr
}
func (f *fakeOSS) HeadObject(context.Context, *aliyunoss.HeadObjectRequest, ...func(*aliyunoss.Options)) (*aliyunoss.HeadObjectResult, error) {
	return f.headResult, f.headErr
}
func (f *fakeOSS) DeleteObject(context.Context, *aliyunoss.DeleteObjectRequest, ...func(*aliyunoss.Options)) (*aliyunoss.DeleteObjectResult, error) {
	return &aliyunoss.DeleteObjectResult{}, f.deleteErr
}

// atomicOSS models the OSS conditional-write contract for local concurrency
// tests. It is not evidence of the real service's cross-host consistency.
type atomicOSS struct {
	mu             sync.Mutex
	objects        map[string][]byte
	metadata       map[string]map[string]string
	putResponseErr error
	headErr        error
}

func (f *atomicOSS) GetBucketInfo(context.Context, *aliyunoss.GetBucketInfoRequest, ...func(*aliyunoss.Options)) (*aliyunoss.GetBucketInfoResult, error) {
	return &aliyunoss.GetBucketInfoResult{}, nil
}
func (f *atomicOSS) GetBucketVersioning(context.Context, *aliyunoss.GetBucketVersioningRequest, ...func(*aliyunoss.Options)) (*aliyunoss.GetBucketVersioningResult, error) {
	return &aliyunoss.GetBucketVersioningResult{VersionStatus: aliyunoss.Ptr(string(aliyunoss.VersionSuspended))}, nil
}
func (f *atomicOSS) PutObject(_ context.Context, request *aliyunoss.PutObjectRequest, _ ...func(*aliyunoss.Options)) (*aliyunoss.PutObjectResult, error) {
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if *request.ForbidOverwrite != "true" {
		return nil, errors.New("test requires forbid-overwrite")
	}
	if _, exists := f.objects[*request.Key]; exists {
		return nil, &aliyunoss.ServiceError{Code: "FileAlreadyExists", StatusCode: 409}
	}
	f.objects[*request.Key] = body
	f.metadata[*request.Key] = map[string]string{"sha256": request.Metadata["sha256"]}
	return &aliyunoss.PutObjectResult{}, f.putResponseErr
}
func (f *atomicOSS) GetObject(_ context.Context, request *aliyunoss.GetObjectRequest, _ ...func(*aliyunoss.Options)) (*aliyunoss.GetObjectResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body, exists := f.objects[*request.Key]
	if !exists {
		return nil, &aliyunoss.ServiceError{Code: "NoSuchKey", StatusCode: 404}
	}
	return &aliyunoss.GetObjectResult{Body: io.NopCloser(bytes.NewReader(append([]byte(nil), body...)))}, nil
}
func (f *atomicOSS) HeadObject(_ context.Context, request *aliyunoss.HeadObjectRequest, _ ...func(*aliyunoss.Options)) (*aliyunoss.HeadObjectResult, error) {
	if f.headErr != nil {
		return nil, f.headErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	body, exists := f.objects[*request.Key]
	if !exists {
		return nil, &aliyunoss.ServiceError{Code: "NoSuchKey", StatusCode: 404}
	}
	return &aliyunoss.HeadObjectResult{ContentLength: int64(len(body)), Metadata: f.metadata[*request.Key]}, nil
}
func (f *atomicOSS) DeleteObject(_ context.Context, request *aliyunoss.DeleteObjectRequest, _ ...func(*aliyunoss.Options)) (*aliyunoss.DeleteObjectResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.objects, *request.Key)
	delete(f.metadata, *request.Key)
	return &aliyunoss.DeleteObjectResult{}, nil
}

func newAtomicOSS() *atomicOSS {
	return &atomicOSS{objects: make(map[string][]byte), metadata: make(map[string]map[string]string)}
}

func TestNewOSSStoreRequiresSecureEndpointAndCredentialSource(t *testing.T) {
	base := OSSConfig{Endpoint: "https://oss-cn-hangzhou.aliyuncs.com", Region: "cn-hangzhou", Bucket: "tasklab-artifacts"}
	if _, err := NewOSSStore(base); err == nil || !strings.Contains(err.Error(), "credential source") {
		t.Fatalf("missing credentials should fail closed, got %v", err)
	}
	base.Endpoint = "http://oss-cn-hangzhou.aliyuncs.com"
	base.ECSRAMRoleName = "tasklab-artifact-worker"
	if _, err := NewOSSStore(base); err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("HTTP OSS endpoint should fail closed, got %v", err)
	}
	base.AllowHTTP = true
	if _, err := NewOSSStore(base); err != nil {
		t.Fatalf("explicit local HTTP override should permit constructor-only test config: %v", err)
	}
}

func TestOSSStorePutUsesOSSForbidOverwriteAndVerifiesSHA256(t *testing.T) {
	body := []byte(`{"immutable":true}`)
	sum := sha256.Sum256(body)
	sha := hex.EncodeToString(sum[:])
	fake := &fakeOSS{headResult: &aliyunoss.HeadObjectResult{
		ContentLength: int64(len(body)), ContentType: aliyunoss.Ptr("application/json"), Metadata: map[string]string{"sha256": sha},
	}}
	store := &OSSStore{client: fake, bucket: "tasklab-artifacts", prefix: "runs", maxBytes: 1024}
	info, err := store.PutIfAbsent(context.Background(), "output.json", body, "application/json")
	if err != nil {
		t.Fatal(err)
	}
	if fake.putRequest == nil || fake.putRequest.ForbidOverwrite == nil || *fake.putRequest.ForbidOverwrite != "true" {
		t.Fatalf("OSS PutObject must use the native forbid-overwrite header: %+v", fake.putRequest)
	}
	if got := fake.putRequest.Metadata["sha256"]; got != sha {
		t.Fatalf("metadata SHA256=%q, want %q", got, sha)
	}
	md5sum := md5.Sum(body)
	if fake.putRequest.ContentMD5 == nil || *fake.putRequest.ContentMD5 != base64.StdEncoding.EncodeToString(md5sum[:]) {
		t.Fatal("OSS PutObject must include the content MD5 integrity header")
	}
	if info.Key != "output.json" || info.SHA256 != sha || info.Size != int64(len(body)) {
		t.Fatalf("unexpected object info: %+v", info)
	}
}

func TestOSSStoreMapsMissingObjectsAndDuplicateKeys(t *testing.T) {
	missing := &aliyunoss.ServiceError{Code: "NoSuchKey", StatusCode: 404}
	fake := &fakeOSS{headErr: missing}
	store := &OSSStore{client: fake, bucket: "bucket", maxBytes: 1024}
	if _, err := store.Head(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing object error=%v, want ErrNotFound", err)
	}

	body := []byte("same bytes")
	sum := sha256.Sum256(body)
	sha := hex.EncodeToString(sum[:])
	fake.putErr = &aliyunoss.ServiceError{Code: "FileAlreadyExists", StatusCode: 409}
	fake.headErr = nil
	fake.headResult = &aliyunoss.HeadObjectResult{ContentLength: int64(len(body)), Metadata: map[string]string{"sha256": sha}}
	if _, err := store.PutIfAbsent(context.Background(), "same", body, "text/plain"); err != nil {
		t.Fatalf("same-content retry should be idempotent: %v", err)
	}
	fake.headResult = &aliyunoss.HeadObjectResult{ContentLength: int64(len(body)), Metadata: map[string]string{"sha256": strings.Repeat("0", 64)}}
	if _, err := store.PutIfAbsent(context.Background(), "same", body, "text/plain"); !errors.Is(err, ErrConflict) {
		t.Fatalf("different-content duplicate should conflict, got %v", err)
	}
}

func TestOSSStoreGetRecomputesSHA256AndDelete(t *testing.T) {
	body := []byte("downloaded from OSS")
	sum := sha256.Sum256(body)
	sha := hex.EncodeToString(sum[:])
	fake := &fakeOSS{
		headResult: &aliyunoss.HeadObjectResult{ContentLength: int64(len(body)), ContentType: aliyunoss.Ptr("text/plain"), Metadata: map[string]string{"SHA256": sha}},
		getResult:  &aliyunoss.GetObjectResult{Body: io.NopCloser(strings.NewReader(string(body)))},
	}
	store := &OSSStore{client: fake, bucket: "bucket", maxBytes: 1024}
	data, info, err := store.Get(context.Background(), "key")
	if err != nil || string(data) != string(body) || info.SHA256 != sha {
		t.Fatalf("Get data=%q info=%+v err=%v", data, info, err)
	}
	fake.headResult = &aliyunoss.HeadObjectResult{ContentLength: int64(len(body)), Metadata: map[string]string{"sha256": strings.Repeat("0", 64)}}
	fake.getResult = &aliyunoss.GetObjectResult{Body: io.NopCloser(strings.NewReader(string(body)))}
	if _, _, err := store.Get(context.Background(), "key"); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("tampered metadata should be rejected, got %v", err)
	}
	if err := store.Delete(context.Background(), "key"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestOSSStoreEnsureBucketRejectsVersioning(t *testing.T) {
	fake := &fakeOSS{versioningStatus: aliyunoss.Ptr(string(aliyunoss.VersionEnabled))}
	store := &OSSStore{client: fake, bucket: "bucket", maxBytes: 1024}
	if err := store.EnsureBucket(context.Background()); err == nil || !strings.Contains(err.Error(), "unsupported versioning status \"Enabled\"") {
		t.Fatalf("enabled OSS versioning should fail closed for immutable writes, got %v", err)
	}
	fake.versioningStatus = aliyunoss.Ptr(string(aliyunoss.VersionSuspended))
	if err := store.EnsureBucket(context.Background()); err != nil {
		t.Fatalf("suspended versioning should allow the explicit immutable-write contract: %v", err)
	}
	fake.versioningStatus = aliyunoss.Ptr("Unexpected")
	if err := store.EnsureBucket(context.Background()); err == nil || !strings.Contains(err.Error(), "unsupported versioning status") {
		t.Fatalf("unknown OSS versioning status should fail closed, got %v", err)
	}
}

func TestOSSStoreConcurrentPutIfAbsentNeverOverwrites(t *testing.T) {
	for _, test := range []struct {
		name       string
		first      []byte
		second     []byte
		wantErrors int
	}{
		{name: "same content is idempotent", first: []byte("same"), second: []byte("same")},
		{name: "different content conflicts", first: []byte("first"), second: []byte("second"), wantErrors: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := newAtomicOSS()
			store := &OSSStore{client: client, bucket: "bucket", prefix: "runs/run-1", maxBytes: 1024}
			start := make(chan struct{})
			errs := make(chan error, 2)
			for _, body := range [][]byte{test.first, test.second} {
				body := append([]byte(nil), body...)
				go func() {
					<-start
					_, err := store.PutIfAbsent(context.Background(), "artifact", body, "application/octet-stream")
					errs <- err
				}()
			}
			close(start)
			errorCount := 0
			for range 2 {
				if err := <-errs; err != nil {
					if !errors.Is(err, ErrConflict) {
						t.Fatalf("PutIfAbsent error=%v, want nil or ErrConflict", err)
					}
					errorCount++
				}
			}
			if errorCount != test.wantErrors {
				t.Fatalf("conflict count=%d, want %d", errorCount, test.wantErrors)
			}
			client.mu.Lock()
			stored := append([]byte(nil), client.objects["runs/run-1/artifact"]...)
			client.mu.Unlock()
			if !bytes.Equal(stored, test.first) && !bytes.Equal(stored, test.second) {
				t.Fatalf("stored object %q is neither complete writer payload", stored)
			}
			if test.wantErrors == 1 && bytes.Equal(test.first, test.second) {
				t.Fatal("test setup error: conflict case must use distinct payloads")
			}
		})
	}
}

func TestOSSStoreClassifiesLostPutResponseAndUnverifiableOutcome(t *testing.T) {
	body := []byte("write committed before response was lost")
	client := newAtomicOSS()
	client.putResponseErr = errors.New("connection reset after request body was sent")
	store := &OSSStore{client: client, bucket: "bucket", maxBytes: 1024}
	if _, err := store.PutIfAbsent(context.Background(), "committed", body, "text/plain"); err != nil {
		t.Fatalf("matching HEAD should reconcile a lost response as success: %v", err)
	}

	client.headErr = &aliyunoss.ServiceError{Code: "AccessDenied", StatusCode: 403}
	if _, err := store.PutIfAbsent(context.Background(), "unknown", []byte("uncertain"), "text/plain"); !errors.Is(err, ErrUnknownOutcome) {
		t.Fatalf("unavailable verification after a potentially committed write should be ErrUnknownOutcome, got %v", err)
	}
}
