package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/artifacts"
	"github.com/fantasticyi/distributed-task-lab/internal/config"
	"github.com/fantasticyi/distributed-task-lab/internal/domain/taskpayload"
	"github.com/fantasticyi/distributed-task-lab/internal/observability"
	"github.com/fantasticyi/distributed-task-lab/internal/storage/postgres"
	"github.com/fantasticyi/distributed-task-lab/internal/worker"
	"github.com/fantasticyi/distributed-task-lab/internal/worker/handlers"
)

func main() {
	cfg := config.Load()
	logger := observability.Logger()
	if err := cfg.ValidateWorker(); err != nil {
		logger.Error("invalid startup configuration", "error", err)
		os.Exit(1)
	}
	logger.Info("validated effective configuration", "profile", cfg.Profile,
		"database", cfg.DatabaseURLWithoutCredentials(), "config_checksum", cfg.EffectiveChecksum())
	shutdownTracing, err := observability.SetupTracing(context.Background(), "distributed-task-lab-worker")
	if err != nil {
		logger.Error("failed to initialize tracing", "error", err)
		os.Exit(1)
	}
	defer shutdownTracing(context.Background())

	// Use configured worker ID.
	workerID := cfg.WorkerID
	logger = logger.With("worker_id", workerID)

	logger.Info("starting worker",
		"concurrency", cfg.Concurrency,
		"poll_interval", cfg.PollInterval,
		"lease_timeout", cfg.LeaseTimeout,
	)

	// Initialize database.
	db, err := postgres.NewDBWithConfig(cfg.DatabaseURL, postgres.DBConfig{
		MaxOpenConns: cfg.DBMaxOpenConns, MaxIdleConns: cfg.DBMaxIdleConns,
		ConnMaxLifetime: cfg.DBConnMaxLifetime, ConnMaxIdleTime: cfg.DBConnMaxIdleTime,
	})
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
	pq := postgres.NewPostgresQueueWithTimeout(db, cfg.DBOperationTimeout)

	// Composite handler: explicit real-RAG object-key tasks or explicit mock workloads.
	var handler worker.Handler
	if cfg.ArtifactBackend == "s3" {
		artifactStore, err := newArtifactStore(cfg)
		if err != nil {
			logger.Error("failed to initialize artifact store", "error", err)
			os.Exit(1)
		}
		if cfg.Profile == "production" {
			handler = handlers.NewCompositeWithArtifactStore(cfg.DocDir, cfg.ArtifactDir, taskpayload.ProductionExecutionPolicy(), artifactStore, postgres.NewEffectLedger(db))
		} else {
			handler = handlers.NewCompositeWithArtifactStore(cfg.DocDir, cfg.ArtifactDir, nil, artifactStore, postgres.NewEffectLedger(db))
		}
	} else if cfg.Profile == "production" {
		handler = handlers.NewCompositeWithAdmission(cfg.DocDir, cfg.ArtifactDir, taskpayload.ProductionExecutionPolicy(), postgres.NewEffectLedger(db))
	} else {
		handler = handlers.NewComposite(cfg.DocDir, cfg.ArtifactDir, postgres.NewEffectLedger(db))
	}

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

	healthMux := http.NewServeMux()
	healthMux.HandleFunc("GET /live", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("live\n"))
	})
	healthMux.HandleFunc("GET /ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), cfg.DBOperationTimeout)
		defer cancel()
		if err := pq.HealthCheck(ctx); err != nil {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready\n"))
	})
	healthMux.HandleFunc("GET /metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		observability.Runtime().WritePrometheus(w)
	})
	healthServer := &http.Server{
		Addr: fmt.Sprintf(":%d", cfg.WorkerHealthPort), Handler: healthMux,
		ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second,
		WriteTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second,
	}
	go func() {
		if err := healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("worker health server failed", "error", err)
		}
	}()

	// Wait for shutdown signal.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logger.Info("shutting down worker")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := pool.StopContext(shutdownCtx); err != nil {
		logger.Warn("worker drain deadline reached; remaining handlers cancelled", "error", err)
	}
	_ = healthServer.Shutdown(shutdownCtx)
	logger.Info("worker stopped")
}

func newArtifactStore(cfg *config.Config) (artifacts.Store, error) {
	if cfg.ArtifactBackend == "s3" {
		store, err := artifacts.NewS3Store(artifacts.S3Config{
			Endpoint: cfg.ArtifactS3Endpoint, Bucket: cfg.ArtifactS3Bucket, Prefix: cfg.ArtifactS3Prefix,
			AccessKey: cfg.ArtifactS3AccessKey, SecretKey: cfg.ArtifactS3SecretKey, Region: cfg.ArtifactS3Region,
			MaxBytes: cfg.ArtifactMaxObjectBytes, AllowHTTP: cfg.ArtifactAllowHTTP,
			AllowBucketCreate: cfg.ArtifactAllowBucketCreate,
		})
		if err != nil {
			return nil, err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := store.EnsureBucket(ctx); err != nil {
			return nil, fmt.Errorf("verify artifact bucket: %w", err)
		}
		return store, nil
	}
	return artifacts.NewLocalStore(cfg.ArtifactDir, "", cfg.ArtifactMaxObjectBytes)
}
