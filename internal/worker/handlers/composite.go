package handlers

import (
	"context"
	"os"
	"path/filepath"

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
	effects *SideEffectHandler
}

// NewComposite creates a composite handler.
func NewComposite(docDir, artifactDir string, ledgers ...effects.Ledger) *Composite {
	var ledger effects.Ledger
	if len(ledgers) > 0 {
		ledger = ledgers[0]
	}
	return &Composite{
		mock:    NewMockHandler(),
		rag:     NewRealRAGIngestion(docDir, artifactDir),
		replay:  NewStrategyReplay(artifactDir),
		eval:    newEvalBackend(artifactDir),
		effects: NewSideEffectHandler(filepath.Join(artifactDir, "receipts"), ledger),
	}
}

func newEvalBackend(artifactDir string) interface {
	Run(context.Context, map[string]any) (*worker.TaskExecResult, error)
} {
	if os.Getenv("EVAL_BACKEND") == "gateway" {
		return NewGatewayLLMEvalFromEnv(artifactDir)
	}
	return NewLLMEval(artifactDir)
}

// Handle routes a task to the real RAG pipeline, the strategy replay worker, or
// the mock handler.
func (c *Composite) Handle(ctx context.Context, taskType string, payload map[string]any, attempt int) (*worker.TaskExecResult, error) {
	canonicalPayload, err := taskpayload.ValidateData(domain.TaskType(taskType), payload)
	if err != nil {
		return nil, worker.NewNonRetryableError(err, domain.ErrorTypeBadPayload)
	}
	payload = canonicalPayload
	if taskType == "rag_ingestion" && getString(payload, "execution_mode", "") == "real" {
		return c.rag.Ingest(ctx, payload)
	}
	if taskType == "strategy_replay" && getString(payload, "execution_mode", "") == "replay" {
		return c.replay.Run(ctx, payload)
	}
	if taskType == "eval" && getString(payload, "execution_mode", "") == "deterministic" {
		return c.eval.Run(ctx, payload)
	}
	if (taskType == "notification" || taskType == "payment") && getString(payload, "execution_mode", "") == "effect" {
		return c.effects.Run(ctx, payload)
	}
	return c.mock.Handle(ctx, taskType, payload, attempt)
}
