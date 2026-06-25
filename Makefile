.PHONY: up down build api worker migrate submit-rag-task submit-multi list-tasks watch-tasks demo-claim demo-retry demo-timeout test logs logs-worker clean dev

# Default target
.DEFAULT_GOAL := help

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

# ── Docker targets ──────────────────────────────────────────

up: ## Start PostgreSQL + API + Workers (scale with: docker compose up --scale worker=3)
	docker compose -f deployments/docker-compose.yml up -d --build

down: ## Stop all services
	docker compose -f deployments/docker-compose.yml down

build: ## Build Go binaries locally
	go build -o bin/api ./cmd/api
	go build -o bin/worker ./cmd/worker

# ── Development targets ─────────────────────────────────────

api: ## Run API server locally (requires PostgreSQL running)
	go run ./cmd/api

worker: ## Run worker locally (requires PostgreSQL running)
	go run ./cmd/worker

migrate: ## Run database migrations (handled automatically on startup)
	@echo "Migrations run automatically when API or Worker starts."

# ── Demo targets ────────────────────────────────────────────

submit-rag-task: ## Submit a single RAG ingestion task
	@curl -s -X POST http://localhost:8080/tasks \
		-H "Content-Type: application/json" \
		-d @examples/payloads/rag_ingestion.json | jq .

submit-multi: ## Submit 5 RAG ingestion tasks to show multi-worker claim
	@for i in 1 2 3 4 5; do \
		echo "=== Task $$i ==="; \
		curl -s -X POST http://localhost:8080/tasks \
			-H "Content-Type: application/json" \
			-d "{\"type\":\"rag_ingestion\",\"payload\":{\"version\":\"1\",\"kind\":\"rag_ingestion\",\"data\":{\"document_id\":\"doc-multi-$$i\",\"dataset_id\":\"ds-demo\",\"index_name\":\"demo-index\"}},\"max_attempts\":1,\"timeout_seconds\":30,\"retry_delay_seconds\":3}" | jq '{id, status}'; \
	done

list-tasks: ## List all tasks via curl
	@curl -s http://localhost:8080/tasks | jq '.[] | {id, status, type, leased_by, attempt, last_error}'

watch-tasks: ## Watch tasks in real-time (requires 'watch' command)
	@watch -n 1 'curl -s http://localhost:8080/tasks | jq ".[] | {id, status, type, leased_by, attempt}"'

demo-claim: up ## Full demo: submit 5 tasks + watch multi-worker claim (Phase 2)
	@echo "=== Waiting for services to be ready... ==="
	@sleep 5
	@echo "=== Submitting 5 tasks ==="
	@for i in 1 2 3 4 5; do \
		curl -s -X POST http://localhost:8080/tasks \
			-H "Content-Type: application/json" \
			-d "{\"type\":\"rag_ingestion\",\"payload\":{\"version\":\"1\",\"kind\":\"rag_ingestion\",\"data\":{\"document_id\":\"doc-$$i\",\"dataset_id\":\"ds-demo\",\"index_name\":\"demo-index\"}},\"max_attempts\":1,\"timeout_seconds\":30,\"retry_delay_seconds\":3}" | jq '{id, status}'; \
	done
	@sleep 3
	@echo "=== Current tasks ==="
	@curl -s http://localhost:8080/tasks | jq '.[] | {id, status, type, leased_by}'

demo-retry: up ## Demo: submit a task that fails once then succeeds (Phase 3 retry)
	@echo "=== Waiting for services to be ready... ==="
	@sleep 5
	@echo "=== Submitting task with mock_failures_before_success=1 ==="
	@curl -s -X POST http://localhost:8080/tasks \
		-H "Content-Type: application/json" \
		-d @examples/payloads/retry.json | jq .
	@echo "=== Watching retry (checking every 4s for 20s)... ==="
	@for i in 1 2 3 4 5; do \
		sleep 4; \
		echo "--- Check $$i ---"; \
		curl -s http://localhost:8080/tasks | jq '.[] | {id, status, attempt, last_error}'; \
	done

demo-timeout: up ## Demo: submit a task that times out (Phase 3 timeout)
	@echo "=== Waiting for services to be ready... ==="
	@sleep 5
	@echo "=== Submitting task with short timeout ==="
	@curl -s -X POST http://localhost:8080/tasks \
		-H "Content-Type: application/json" \
		-d @examples/payloads/timeout.json | jq .
	@echo "=== Watching timeout + retry (checking every 3s for 15s)... ==="
	@for i in 1 2 3 4 5; do \
		sleep 3; \
		echo "--- Check $$i ---"; \
		curl -s http://localhost:8080/tasks | jq '.[] | {id, status, attempt, last_error}'; \
	done

# ── Utility targets ─────────────────────────────────────────

logs: ## View all Docker Compose logs
	docker compose -f deployments/docker-compose.yml logs -f

logs-worker: ## View worker logs only
	docker compose -f deployments/docker-compose.yml logs -f worker

test: ## Run tests
	go test ./...

clean: ## Remove build artifacts
	rm -rf bin/

# ── Quick start ─────────────────────────────────────────────

dev: up ## Start everything (alias for 'up')
