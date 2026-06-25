.PHONY: up down build api worker migrate test demo demo-success demo-retry demo-dlq demo-timeout logs clean

# Default target
.DEFAULT_GOAL := help

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

# ── Docker targets ──────────────────────────────────────────

up: ## Start all services (PostgreSQL, API, Workers)
	docker compose -f deployments/docker-compose.yml up -d --build

down: ## Stop all services
	docker compose -f deployments/docker-compose.yml down

build: ## Build Go binaries locally
	go build -o bin/api ./cmd/api
	go build -o bin/worker ./cmd/worker

# ── Development targets ─────────────────────────────────────

api: ## Run API server locally (requires PostgreSQL)
	go run ./cmd/api

worker: ## Run worker locally (requires PostgreSQL)
	go run ./cmd/worker

migrate: ## Run database migrations only
	@echo "Migrations run automatically on startup."

# ── Demo targets ────────────────────────────────────────────

demo: demo-success ## Run full demo (success + retry + dlq)

demo-success: up ## Submit a RAG ingestion task and watch it succeed
	@echo "=== Waiting for services to be ready... ==="
	@sleep 3
	@echo "=== Submitting RAG ingestion task ==="
	@curl -s -X POST http://localhost:8080/tasks \
		-H "Content-Type: application/json" \
		-d @examples/payloads/rag_ingestion.json | jq .
	@sleep 2
	@echo "=== Checking task status ==="
	@curl -s http://localhost:8080/tasks | jq '.[0]'

demo-retry: up ## Submit a failing task and watch it retry
	@echo "=== Submitting task that fails then succeeds ==="
	@curl -s -X POST http://localhost:8080/tasks \
		-H "Content-Type: application/json" \
		-d @examples/payloads/retry.json | jq .
	@sleep 12
	@echo "=== Task after retry ==="
	@curl -s http://localhost:8080/tasks | jq '.[0]'

demo-dlq: up ## Submit a task that goes to DLQ
	@echo "=== Submitting task destined for DLQ ==="
	@curl -s -X POST http://localhost:8080/tasks \
		-H "Content-Type: application/json" \
		-d @examples/payloads/dlq.json | jq .
	@sleep 20
	@echo "=== DLQ tasks ==="
	@curl -s "http://localhost:8080/tasks?status=dlq" | jq .

demo-timeout: up ## Submit a task that times out
	@echo "=== Submitting task with short timeout ==="
	@curl -s -X POST http://localhost:8080/tasks \
		-H "Content-Type: application/json" \
		-d @examples/payloads/timeout.json | jq .
	@sleep 10
	@echo "=== Checking task (should be retrying) ==="
	@curl -s http://localhost:8080/tasks | jq '.[0]'

submit-rag-task: ## Submit a simple RAG ingestion task
	@curl -s -X POST http://localhost:8080/tasks \
		-H "Content-Type: application/json" \
		-d @examples/payloads/rag_ingestion.json | jq .

watch-tasks: ## Watch task list (run in another terminal)
	@watch -n 1 'curl -s http://localhost:8080/tasks | jq .'

# ── Utility targets ─────────────────────────────────────────

logs: ## View Docker Compose logs
	docker compose -f deployments/docker-compose.yml logs -f

test: ## Run tests
	go test ./...

clean: ## Remove build artifacts
	rm -rf bin/

# ── Quick start ─────────────────────────────────────────────

dev: up ## Alias for 'up' — start everything
