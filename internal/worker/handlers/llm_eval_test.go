package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/fantasticyi/distributed-task-lab/internal/artifacts"
)

func evalPayload() map[string]any {
	return map[string]any{"dataset_version": "golden-2026-10", "model_profile": "mock-gpt", "prompt_version": "v3", "run_id": "release-gate", "case_start": 0, "case_end": 100}
}

func TestLLMEvalWritesAndReusesShard(t *testing.T) {
	h := NewLLMEval(t.TempDir())
	first, err := h.Run(context.Background(), evalPayload())
	if err != nil {
		t.Fatal(err)
	}
	if first.Artifacts["idempotent_reused"] != "false" {
		t.Fatal("first run should create the shard")
	}
	key := strings.TrimPrefix(first.Artifacts["case_results_uri"], "artifact://")
	if _, info, err := h.Store.Get(context.Background(), key); err != nil || info.SHA256 == "" {
		t.Fatalf("read eval artifact: info=%+v err=%v", info, err)
	}
	second, err := h.Run(context.Background(), evalPayload())
	if err != nil {
		t.Fatal(err)
	}
	if second.Artifacts["idempotent_reused"] != "true" || second.Artifacts["result_fingerprint"] != first.Artifacts["result_fingerprint"] {
		t.Fatal("retry must reuse the same shard")
	}
}

func TestLLMEvalCanUseSharedArtifactStoreAcrossHandlerInstances(t *testing.T) {
	store, err := artifacts.NewLocalStore(t.TempDir(), "shared", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	first, err := NewLLMEvalWithStore(store).Run(context.Background(), evalPayload())
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewLLMEvalWithStore(store).Run(context.Background(), evalPayload())
	if err != nil {
		t.Fatal(err)
	}
	if second.Artifacts["idempotent_reused"] != "true" || second.Artifacts["case_results_uri"] != first.Artifacts["case_results_uri"] {
		t.Fatalf("second handler did not reuse shared artifact: first=%v second=%v", first.Artifacts, second.Artifacts)
	}
}

func TestLLMEvalRejectsArtifactWithMismatchedExecutionIdentity(t *testing.T) {
	store, err := artifacts.NewLocalStore(t.TempDir(), "", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	payload := evalPayload()
	identity := fmt.Sprintf("%s|%s|%s|%s|%d|%d", "golden-2026-10", "mock-gpt", "v3", "release-gate", 0, 100)
	sum := sha256.Sum256([]byte(identity))
	fingerprint := hex.EncodeToString(sum[:])
	wrong := []byte(`{"DatasetVersion":"wrong","ModelProfile":"mock-gpt","PromptVersion":"v3","RunID":"release-gate","Fingerprint":"` + fingerprint + `","CaseStart":0,"CaseEnd":100}`)
	if _, err := store.PutIfAbsent(context.Background(), "eval/release-gate/"+fingerprint+".json", wrong, "application/json"); err != nil {
		t.Fatal(err)
	}
	_, err = NewLLMEvalWithStore(store).Run(context.Background(), payload)
	if !errors.Is(err, artifacts.ErrCorrupt) {
		t.Fatalf("mismatched stored eval shard error=%v, want ErrCorrupt", err)
	}
}

func TestLLMEvalRejectsIncompletePayload(t *testing.T) {
	if _, err := NewLLMEval(t.TempDir()).Run(context.Background(), map[string]any{"run_id": "x"}); err == nil {
		t.Fatal("expected validation error")
	}
}
