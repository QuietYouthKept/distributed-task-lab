package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/api"
	"github.com/fantasticyi/distributed-task-lab/internal/api/handlers"
	"github.com/fantasticyi/distributed-task-lab/internal/application"
	"github.com/fantasticyi/distributed-task-lab/internal/config"
	"github.com/fantasticyi/distributed-task-lab/internal/observability"
	"github.com/fantasticyi/distributed-task-lab/internal/storage/postgres"
)

func main() {
	cfg := config.Load()
	logger := observability.Logger()
	shutdownTracing, err := observability.SetupTracing(context.Background(), "distributed-task-lab-api")
	if err != nil {
		logger.Error("failed to initialize tracing", "error", err)
		os.Exit(1)
	}
	defer shutdownTracing(context.Background())

	logger.Info("starting API server", "port", cfg.APIPort)
	if cfg.APIToken == "" || cfg.AdminToken == "" {
		logger.Warn("HTTP authentication is incomplete; local-development mode only",
			"api_token_configured", cfg.APIToken != "", "admin_token_configured", cfg.AdminToken != "")
	}

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

	// Run migrations.
	if err := postgres.RunMigrations(db); err != nil {
		logger.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}
	logger.Info("database migrations applied")

	// Initialize queue and service layer.
	pq := postgres.NewPostgresQueueWithTimeout(db, cfg.DBOperationTimeout)
	svc := application.NewTaskService(pq)

	// Initialize HTTP handlers and router.
	taskHandler := handlers.NewTaskHandler(svc, logger)

	// Metrics handler reads aggregated counts, per-queue depth, and oldest-pending age.
	metricsHandler := handlers.NewMetricsHandler(svc)

	router := api.NewRouterWithConfig(taskHandler, metricsHandler, svc, logger, api.RouterConfig{
		APIToken: cfg.APIToken, AdminToken: cfg.AdminToken,
		MaxRequestBodyBytes: cfg.MaxRequestBodyBytes, RequestTimeout: cfg.DBOperationTimeout,
	})

	// Start HTTP server.
	addr := fmt.Sprintf(":%d", cfg.APIPort)
	srv := &http.Server{
		Addr: addr, Handler: router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Graceful shutdown.
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		logger.Info("shutting down API server")
		ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			logger.Warn("API shutdown forced", "error", err)
		}
	}()

	logger.Info("API server listening", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("API server failed", "error", err)
		os.Exit(1)
	}
	logger.Info("API server stopped")
}
