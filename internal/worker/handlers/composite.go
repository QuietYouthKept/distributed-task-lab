package handlers

import (
	"context"
	"os"
	"path/filepath"

	"github.com/fantasticyi/distributed-task-lab/internal/worker"
)

// Composite dispatches rag_ingestion tasks that carry a source_path to the
// real RAG ingestion pipeline, and everything else to the mock handler.
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
func NewComposite(docDir, artifactDir string) *Composite {
	return &Composite{
		mock:    NewMockHandler(),
		rag:     NewRealRAGIngestion(docDir, artifactDir),
		replay:  NewStrategyReplay(artifactDir),
		eval:    newEvalBackend(artifactDir),
		effects: NewSideEffectHandler(filepath.Join(artifactDir, "receipts")),
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
	if taskType == "rag_ingestion" && getString(payload, "source_path", "") != "" {
		return c.rag.Ingest(ctx, payload)
	}
	if taskType == "strategy_replay" {
		return c.replay.Run(ctx, payload)
	}
	if taskType == "eval" && getString(payload, "dataset_version", "") != "" {
		return c.eval.Run(ctx, payload)
	}
	if taskType == "notification" || taskType == "payment" {
		return c.effects.Run(ctx, payload)
	}
	return c.mock.Handle(ctx, taskType, payload, attempt)
}
