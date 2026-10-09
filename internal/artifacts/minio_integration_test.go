package artifacts

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"
)

func TestMinIOStoreContractAndConcurrentConditionalWrite(t *testing.T) {
	endpoint := os.Getenv("MINIO_ENDPOINT")
	if endpoint == "" {
		if os.Getenv("REQUIRE_MINIO") == "1" {
			t.Fatal("REQUIRE_MINIO=1 but MINIO_ENDPOINT is not configured")
		}
		t.Skip("set MINIO_ENDPOINT and REQUIRE_MINIO=1 to run against a real MinIO service")
	}
	for _, name := range []string{"MINIO_BUCKET", "MINIO_ACCESS_KEY", "MINIO_SECRET_KEY"} {
		if os.Getenv(name) == "" {
			if os.Getenv("REQUIRE_MINIO") == "1" {
				t.Fatalf("REQUIRE_MINIO=1 but %s is not configured", name)
			}
			t.Skipf("%s must be configured to run against a real MinIO service", name)
		}
	}
	cfg := S3Config{Endpoint: endpoint, Bucket: os.Getenv("MINIO_BUCKET"), Prefix: "contract-test", AccessKey: os.Getenv("MINIO_ACCESS_KEY"), SecretKey: os.Getenv("MINIO_SECRET_KEY"), AllowHTTP: true, AllowBucketCreate: true, MaxBytes: 1 << 20}
	store, err := NewS3Store(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := store.EnsureBucket(ctx); err != nil {
		t.Fatal(err)
	}
	key := "runs/" + time.Now().UTC().Format("20060102T150405.000000000") + "/result.json"
	body := []byte(`{"artifact":"shared","value":7}`)
	first, err := store.PutIfAbsent(ctx, key, body, "application/json")
	if err != nil {
		t.Fatal(err)
	}
	if first.Key != key {
		t.Fatalf("returned key=%q, want logical key %q", first.Key, key)
	}
	second, err := store.PutIfAbsent(ctx, key, body, "application/json")
	if err != nil || first.SHA256 != second.SHA256 {
		t.Fatalf("same content retry first=%+v second=%+v err=%v", first, second, err)
	}
	if second.ContentType != "application/json" {
		t.Fatalf("content type=%q", second.ContentType)
	}
	if _, err := store.PutIfAbsent(ctx, key, []byte(`{"artifact":"different"}`), "application/json"); !errors.Is(err, ErrConflict) {
		t.Fatalf("different-content write=%v, want conflict", err)
	}
	{
		conflictKey := key + "-conflicting-writers"
		var wg sync.WaitGroup
		errCh := make(chan error, 2)
		for _, body := range [][]byte{[]byte(`{"writer":1}`), []byte(`{"writer":2}`)} {
			body := body
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := store.PutIfAbsent(ctx, conflictKey, body, "application/json")
				errCh <- err
			}()
		}
		wg.Wait()
		close(errCh)
		succeeded, conflicted := 0, 0
		for err := range errCh {
			switch {
			case err == nil:
				succeeded++
			case errors.Is(err, ErrConflict):
				conflicted++
			default:
				t.Errorf("concurrent different-content MinIO put: %v", err)
			}
		}
		if succeeded != 1 || conflicted != 1 {
			t.Fatalf("concurrent different-content writes: succeeded=%d conflicts=%d, want 1/1", succeeded, conflicted)
		}
	}
	data, got, err := store.Get(ctx, key)
	if err != nil || string(data) != string(body) || got.SHA256 != first.SHA256 {
		t.Fatalf("Get body=%s info=%+v err=%v", data, got, err)
	}
	if _, err := store.Head(ctx, key+"-missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing object Head=%v, want ErrNotFound", err)
	}
	canceledCtx, cancelRequest := context.WithCancel(ctx)
	cancelRequest()
	if _, _, err := store.Get(canceledCtx, key); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Get=%v, want context.Canceled", err)
	}
	wrongCredentials := cfg
	wrongCredentials.AccessKey += "-invalid"
	wrongCredentials.SecretKey += "-invalid"
	deniedStore, err := NewS3Store(wrongCredentials)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := deniedStore.Head(ctx, key); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("Head with invalid credentials=%v, want ErrAccessDenied", err)
	}

	concurrentKey := key + "-identical-writers"
	const writers = 8
	var wg sync.WaitGroup
	errCh := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.PutIfAbsent(ctx, concurrentKey, body, "application/json")
			errCh <- err
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Errorf("concurrent identical MinIO put: %v", err)
		}
	}
	if err := store.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
}
