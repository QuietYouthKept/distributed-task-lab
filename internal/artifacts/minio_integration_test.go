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
	cfg := S3Config{Endpoint: endpoint, Bucket: envOr("MINIO_BUCKET", "tasklab-test"), Prefix: "contract-test", AccessKey: envOr("MINIO_ACCESS_KEY", "minioadmin"), SecretKey: envOr("MINIO_SECRET_KEY", "minioadmin"), AllowHTTP: true, AllowBucketCreate: true, MaxBytes: 1 << 20}
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
	data, got, err := store.Get(ctx, key)
	if err != nil || string(data) != string(body) || got.SHA256 != first.SHA256 {
		t.Fatalf("Get body=%s info=%+v err=%v", data, got, err)
	}

	concurrentKey := key + "-race"
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

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
