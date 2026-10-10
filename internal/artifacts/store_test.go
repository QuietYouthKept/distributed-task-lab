package artifacts

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

func TestValidateKeyRejectsTraversalAndURLForms(t *testing.T) {
	for _, key := range []string{"", "../secret", "a/../../secret", "/absolute", `a\b`, "a//b", "https://host/object", "a%2f..%2fsecret", "a/./b"} {
		if err := ValidateKey(key); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("ValidateKey(%q)=%v, want ErrInvalidKey", key, err)
		}
	}
	if err := ValidateKey("documents/folder/doc.txt"); err != nil {
		t.Fatalf("valid key rejected: %v", err)
	}
}

func TestLocalStorePutGetHeadDeleteAndConflict(t *testing.T) {
	store, err := NewLocalStore(t.TempDir(), "tenant-a", 32)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	first, err := store.PutIfAbsent(ctx, "runs/result.json", []byte(`{"value":1}`), "application/json")
	if err != nil {
		t.Fatal(err)
	}
	if first.Key != "runs/result.json" {
		t.Fatalf("PutIfAbsent returned non-logical key %q", first.Key)
	}
	second, err := store.PutIfAbsent(ctx, "runs/result.json", []byte(`{"value":1}`), "application/json")
	if err != nil || second.SHA256 != first.SHA256 {
		t.Fatalf("same-content retry info=%+v err=%v", second, err)
	}
	if _, err := store.PutIfAbsent(ctx, "runs/result.json", []byte(`{"value":2}`), "application/json"); !errors.Is(err, ErrConflict) {
		t.Fatalf("different-content retry error=%v, want conflict", err)
	}
	data, info, err := store.Get(ctx, "runs/result.json")
	if err != nil || string(data) != `{"value":1}` || info.SHA256 != first.SHA256 || info.Size != int64(len(data)) || info.Key != "runs/result.json" {
		t.Fatalf("Get data=%q info=%+v err=%v", data, info, err)
	}
	head, err := store.Head(ctx, "runs/result.json")
	if err != nil || head.SHA256 != first.SHA256 || head.Key != "runs/result.json" {
		t.Fatalf("Head=%+v err=%v", head, err)
	}
	if err := store.Delete(ctx, "runs/result.json"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Get(ctx, "runs/result.json"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after delete=%v, want not found", err)
	}
}

func TestLocalStoreConcurrentPutIfAbsentNeverPublishesPartialOrOverwrites(t *testing.T) {
	store, err := NewLocalStore(t.TempDir(), "", 1024)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	const writers = 12
	var wg sync.WaitGroup
	errCh := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.PutIfAbsent(ctx, "same/key", []byte(strings.Repeat("a", 256)), "text/plain")
			errCh <- err
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Errorf("identical concurrent PutIfAbsent: %v", err)
		}
	}
	if _, err := store.PutIfAbsent(ctx, "same/key", []byte(strings.Repeat("b", 256)), "text/plain"); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflicting concurrent key=%v", err)
	}
	data, _, err := store.Get(ctx, "same/key")
	if err != nil || string(data) != strings.Repeat("a", 256) {
		t.Fatalf("stored value changed or partial: size=%d err=%v", len(data), err)
	}
}

func TestLocalStoreRespectsCanceledContextAndSizeLimit(t *testing.T) {
	store, err := NewLocalStore(t.TempDir(), "", 3)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.PutIfAbsent(ctx, "cancelled", []byte("x"), "text/plain"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled put=%v", err)
	}
	if _, err := store.PutIfAbsent(context.Background(), "too-large", []byte("four"), "text/plain"); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversized put=%v", err)
	}
}
