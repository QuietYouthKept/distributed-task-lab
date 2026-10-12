package artifacts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestOSSCloudContract is intentionally opt-in and must run on a Worker ECS
// with a narrowly scoped RAM role. A skip is not cloud acceptance evidence.
func TestOSSCloudContract(t *testing.T) {
	if os.Getenv("TASKLAB_OSS_CONTRACT") != "true" {
		t.Skip("set TASKLAB_OSS_CONTRACT=true on an authorized staging ECS to run live OSS contract checks")
	}
	runID := os.Getenv("TASKLAB_RUN_ID")
	if len(runID) < 8 || len(runID) > 40 || strings.Trim(runID, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-") != "" {
		t.Fatal("TASKLAB_RUN_ID must be an 8-40 character run-scoped token")
	}
	store, err := NewOSSStore(OSSConfig{
		Endpoint: os.Getenv("ARTIFACT_OSS_ENDPOINT"), Region: os.Getenv("ARTIFACT_OSS_REGION"),
		Bucket: os.Getenv("ARTIFACT_OSS_BUCKET"), Prefix: "tasklab/" + runID,
		ECSRAMRoleName: os.Getenv("ARTIFACT_OSS_RAM_ROLE_NAME"), MaxBytes: 1 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := store.EnsureBucket(ctx); err != nil {
		t.Fatalf("preflight bucket/versioning check: %v", err)
	}

	keys := []string{"contract/put-get-head-" + uuid.NewString(), "contract/conflict-" + uuid.NewString(), "contract/concurrent-" + uuid.NewString()}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		for _, key := range keys {
			if err := store.Delete(cleanupCtx, key); err != nil && !errors.Is(err, ErrNotFound) {
				t.Errorf("delete run-owned contract object %q: %v", key, err)
			}
		}
	})

	content := []byte(`{"cloud_contract":"oss","run":"` + runID + `"}`)
	putInfo, err := store.PutIfAbsent(ctx, keys[0], content, "application/json")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, getInfo, err := store.Get(ctx, keys[0])
	if err != nil || string(got) != string(content) {
		t.Fatalf("Get content=%q err=%v", got, err)
	}
	wantSHA := sha256.Sum256(content)
	wantHex := hex.EncodeToString(wantSHA[:])
	if putInfo.SHA256 != wantHex || getInfo.SHA256 != wantHex || getInfo.Size != int64(len(content)) {
		t.Fatalf("SHA/size mismatch put=%+v get=%+v want_sha=%s", putInfo, getInfo, wantHex)
	}
	if head, err := store.Head(ctx, keys[0]); err != nil || head.SHA256 != wantHex {
		t.Fatalf("Head info=%+v err=%v", head, err)
	}
	if _, err := store.PutIfAbsent(ctx, keys[0], content, "application/json"); err != nil {
		t.Fatalf("same-content retry must be idempotent: %v", err)
	}
	if _, err := store.PutIfAbsent(ctx, keys[1], []byte("original"), "text/plain"); err != nil {
		t.Fatalf("seed conflict key: %v", err)
	}
	if _, err := store.PutIfAbsent(ctx, keys[1], []byte("replacement"), "text/plain"); !errors.Is(err, ErrConflict) {
		t.Fatalf("different-content duplicate must conflict, got %v", err)
	}
	if _, err := store.Head(ctx, "contract/missing-"+uuid.NewString()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing object must map to ErrNotFound, got %v", err)
	}

	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, body := range [][]byte{[]byte("writer-one"), []byte("writer-two")} {
		body := body
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.PutIfAbsent(ctx, keys[2], body, "text/plain")
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrConflict):
			conflicts++
		default:
			t.Errorf("concurrent write returned unexpected result: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent immutable writes: successes=%d conflicts=%d, want 1/1", successes, conflicts)
	}
	final, info, err := store.Get(ctx, keys[2])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(final), "writer-") || info.SHA256 != hex.EncodeToString(sha256Sum(final)) {
		t.Fatalf("concurrent final object failed SHA verification: info=%+v body=%q", info, final)
	}
	fmt.Printf("OSS contract PASS run_id=%s object_count=3 sha256_verified=true\n", runID)
}

func sha256Sum(data []byte) []byte {
	sum := sha256.Sum256(data)
	return sum[:]
}
