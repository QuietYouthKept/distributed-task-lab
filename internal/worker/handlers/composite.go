package handlers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fantasticyi/distributed-task-lab/internal/artifacts"
	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"github.com/fantasticyi/distributed-task-lab/internal/domain/taskpayload"
	"github.com/fantasticyi/distributed-task-lab/internal/effects"
	"github.com/fantasticyi/distributed-task-lab/internal/worker"
)

// Composite dispatches explicitly-modeled task payloads to their handler.
type Composite struct {
	mock   *MockHandler
	rag    *RealRAGIngestion
	replay *StrategyReplay
	eval   interface {
		Run(context.Context, map[string]any) (*worker.TaskExecResult, error)
	}
	evalMode string
	effects  *SideEffectHandler
	admit    taskpayload.ExecutionPolicy
}

// NewComposite creates a composite handler.
func NewComposite(docDir, artifactDir string, ledgers ...effects.Ledger) *Composite {
	return NewCompositeWithAdmission(docDir, artifactDir, nil, ledgers...)
}

func NewCompositeWithAdmission(docDir, artifactDir string, admission taskpayload.ExecutionPolicy, ledgers ...effects.Ledger) *Composite {
	store, err := artifacts.NewLocalStore(artifactDir, "", 64<<20)
	return newCompositeWithStore(docDir, artifactDir, admission, nil, store, err, ledgers...)
}

// NewCompositeWithArtifactStore wires handlers that produce immutable outputs
// to the same configured Store. It does not enable production execution; the
// deployment admission policy remains the caller's responsibility.
func NewCompositeWithArtifactStore(docDir, artifactDir string, admission taskpayload.ExecutionPolicy, store artifacts.Store, ledgers ...effects.Ledger) *Composite {
	return newCompositeWithStore(docDir, artifactDir, admission, store, store, nil, ledgers...)
}

func newCompositeWithStore(docDir, artifactDir string, admission taskpayload.ExecutionPolicy, sourceStore, store artifacts.Store, storeErr error, ledgers ...effects.Ledger) *Composite {
	var ledger effects.Ledger
	if len(ledgers) > 0 {
		ledger = ledgers[0]
	}
	eval, evalMode := newEvalBackend(artifactDir, store)
	if deterministic, ok := eval.(*LLMEval); ok {
		deterministic.storeErr = storeErr
	}
	replay := NewStrategyReplayWithStore(store)
	replay.storeErr = storeErr
	rag := NewRealRAGIngestionWithStores(docDir, sourceStore, store)
	rag.storeErr = storeErr
	if storeErr != nil {
		rag.Store = nil
	}
	return &Composite{
		mock:     NewMockHandler(),
		rag:      rag,
		replay:   replay,
		eval:     eval,
		evalMode: evalMode,
		effects:  NewSideEffectHandler(filepath.Join(artifactDir, "receipts"), ledger),
		admit:    admission,
	}
}

func newEvalBackend(artifactDir string, store artifacts.Store) (interface {
	Run(context.Context, map[string]any) (*worker.TaskExecResult, error)
}, string) {
	if os.Getenv("EVAL_BACKEND") == "gateway" {
		return NewGatewayLLMEvalFromEnvWithStore(store), "gateway"
	}
	return NewLLMEvalWithStore(store), "deterministic"
}

// Handle routes a task to the real RAG pipeline, the strategy replay worker, or
// the mock handler.
func (c *Composite) Handle(ctx context.Context, taskType string, payload map[string]any, attempt int) (*worker.TaskExecResult, error) {
	canonicalPayload, err := taskpayload.ValidateData(domain.TaskType(taskType), payload)
	if err != nil {
		return nil, worker.NewNonRetryableError(err, domain.ErrorTypeBadPayload)
	}
	payload = canonicalPayload
	mode := getString(payload, "execution_mode", "")
	if !c.admit.Allows(domain.TaskType(taskType), mode) {
		return nil, worker.NewNonRetryableError(fmt.Errorf("task type %q execution mode %q is not enabled in this worker deployment", taskType, mode), domain.ErrorTypeBadPayload)
	}
	if taskType == "rag_ingestion" && getString(payload, "execution_mode", "") == "real" {
		return c.rag.Ingest(ctx, payload)
	}
	if taskType == "strategy_replay" && getString(payload, "execution_mode", "") == "replay" {
		return c.replay.Run(ctx, payload)
	}
	if taskType == "eval" {
		mode := getString(payload, "execution_mode", "")
		if mode == "deterministic" || mode == "gateway" {
			if mode != c.evalMode {
				return nil, worker.NewNonRetryableError(fmt.Errorf("eval task mode %q does not match worker backend %q", mode, c.evalMode), domain.ErrorTypeBadPayload)
			}
			return c.eval.Run(ctx, payload)
		}
	}
	if (taskType == "notification" || taskType == "payment") && getString(payload, "execution_mode", "") == "effect" {
		return c.effects.Run(ctx, payload)
	}
	return c.mock.Handle(ctx, taskType, payload, attempt)
}
