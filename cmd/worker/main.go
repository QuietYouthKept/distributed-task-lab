package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/fantasticyi/distributed-task-lab/internal/config"
	"github.com/fantasticyi/distributed-task-lab/internal/observability"
	"github.com/fantasticyi/distributed-task-lab/internal/storage/postgres"
	"github.com/fantasticyi/distributed-task-lab/internal/worker"
	"github.com/fantasticyi/distributed-task-lab/internal/worker/handlers"
)

func main() {
	cfg := config.Load()
	logger := observability.Logger()

	// Use configured worker ID.
	workerID := cfg.WorkerID
	logger = logger.With("worker_id", workerID)

	logger.Info("starting worker",
		"concurrency", cfg.Concurrency,
		"poll_interval", cfg.PollInterval,
		"lease_timeout", cfg.LeaseTimeout,
	)

	// Initialize database.
	db, err := postgres.NewDB(cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Run migrations (idempotent).
	if err := postgres.RunMigrations(db); err != nil {
		logger.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}
	logger.Info("database migrations applied")

	// Initialize queue.
	pq := postgres.NewPostgresQueue(db)

	// Composite handler: real RAG ingestion for source_path tasks, mock otherwise.
	handler := handlers.NewComposite(cfg.DocDir, cfg.ArtifactDir)

	// Create and start the worker pool.
	pool := worker.NewPool(
		pq,
		workerID,
		cfg.Concurrency,
		cfg.PollInterval,
		cfg.LeaseTimeout,
		cfg.ClaimBatchSize,
		handler,
		logger,
	)

	pool.Start()

	// Wait for shutdown signal.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logger.Info("shutting down worker")
	pool.Stop()
	logger.Info("worker stopped")
}
