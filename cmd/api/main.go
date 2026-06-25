package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/yourusername/distributed-task-lab/internal/api"
	"github.com/yourusername/distributed-task-lab/internal/api/handlers"
	"github.com/yourusername/distributed-task-lab/internal/application"
	"github.com/yourusername/distributed-task-lab/internal/config"
	"github.com/yourusername/distributed-task-lab/internal/domain"
	"github.com/yourusername/distributed-task-lab/internal/observability"
	"github.com/yourusername/distributed-task-lab/internal/storage/postgres"
)

func main() {
	cfg := config.Load()
	logger := observability.Logger()

	logger.Info("starting API server", "port", cfg.APIPort)

	// Initialize database.
	db, err := postgres.NewDB(cfg.DatabaseURL)
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
	pq := postgres.NewPostgresQueue(db)
	svc := application.NewTaskService(pq)

	// Initialize HTTP handlers and router.
	taskHandler := handlers.NewTaskHandler(svc, logger)

	// Metrics handler uses the service for task counting.
	metricsLister := func(status *domain.TaskStatus) ([]domain.Task, error) {
		return svc.ListTasks(context.Background(), domain.TaskFilter{Status: status, Limit: 10000})
	}
	metricsHandler := handlers.NewMetricsHandler(metricsLister)

	router := api.NewRouter(taskHandler, metricsHandler, logger)

	// Start HTTP server.
	addr := fmt.Sprintf(":%d", cfg.APIPort)
	srv := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	// Graceful shutdown.
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		logger.Info("shutting down API server")
		srv.Close()
	}()

	logger.Info("API server listening", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("API server failed", "error", err)
		os.Exit(1)
	}
	logger.Info("API server stopped")
}
