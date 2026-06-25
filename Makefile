.PHONY: up down build api worker migrate submit-rag-task submit-multi list-tasks watch-tasks demo demo-claim demo-retry demo-timeout demo-dlq test fmt logs logs-worker clean dev

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

submit-multi: ## Submit 5 tasks to show multi-worker claim
	@for i in 1 2 3 4 5; do \
		echo "=== Task $$i ==="; \
		curl -s -X POST http://localhost:8080/tasks \
			-H "Content-Type: application/json" \
			-d "{\"type\":\"rag_ingestion\",\"payload\":{\"version\":\"1\",\"kind\":\"rag_ingestion\",\"data\":{\"document_id\":\"doc-multi-$$i\",\"dataset_id\":\"ds-demo\",\"index_name\":\"demo-index\"}},\"max_attempts\":1,\"timeout_seconds\":30,\"retry_delay_seconds\":3}" | jq '{id, status}'; \
	done

list-tasks: ## List all tasks
	@curl -s http://localhost:8080/tasks | jq '.[] | {id, status, type, leased_by, attempt, last_error}'

watch-tasks: ## Watch tasks in real-time (requires 'watch' command)
	@watch -n 1 'curl -s http://localhost:8080/tasks | jq ".[] | {id, status, type, leased_by, attempt}"'

demo: up ## FULL DEMO: success → retry → DLQ — the complete story
	@echo ""
	@echo "=============================================================="
	@echo "  distributed-task-lab — Complete MVP Demo"
	@echo "  Showcases: claim, lease, retry, timeout, DLQ"
	@echo "=============================================================="
	@echo ""
	@sleep 5
	@echo "=== [1/7] Submitting a normal RAG ingestion task (will succeed) ==="
	@curl -s -X POST http://localhost:8080/tasks -H "Content-Type: application/json" \
		-d @examples/payloads/rag_ingestion.json | jq '{id, status}'
	@sleep 3
	@echo "=== [2/7] Task after worker claimed it ==="
	@curl -s http://localhost:8080/tasks | jq '.[] | {id, status, type, leased_by}'
	@sleep 2
	@echo "=== [3/7] Task completed (pending → running → success) ==="
	@curl -s http://localhost:8080/tasks | jq '.[] | {id, status, type, result}'
	@echo ""
	@echo "=== [4/7] Submitting a flaky task (fails once, then succeeds — retry demo) ==="
	@curl -s -X POST http://localhost:8080/tasks -H "Content-Type: application/json" \
		-d @examples/payloads/retry.json | jq '{id, status}'
	@echo "  Waiting for retry cycle..."
	@sleep 6
	@echo "=== [5/7] Task after retry (attempt increased, then succeeded) ==="
	@curl -s http://localhost:8080/tasks | jq '.[] | {id, status, attempt, last_error}'
	@echo ""
	@echo "=== [6/7] Submitting a doomed task (always fails → DLQ) ==="
	@curl -s -X POST http://localhost:8080/tasks -H "Content-Type: application/json" \
		-d @examples/payloads/dlq.json | jq '{id, status}'
	@echo "  Waiting for retry → DLQ..."
	@sleep 12
	@echo "=== [7/7] DLQ tasks ==="
	@curl -s "http://localhost:8080/tasks?status=dlq" | jq '.[] | {id, status, dlq_reason}'
	@echo ""
	@echo "=== Metrics ==="
	@curl -s http://localhost:8080/stats | jq .
	@echo ""
	@echo "=============================================================="
	@echo "  Demo complete!"
	@echo "  make logs     — view all service logs"
	@echo "  make down     — stop everything"
	@echo "=============================================================="

demo-claim: up ## Phase 2: submit 5 tasks + watch multi-worker claim
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

demo-retry: up ## Phase 3: task fails once then succeeds
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

demo-timeout: up ## Phase 3: task times out
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

demo-dlq: up ## Phase 4: task goes through retry → DLQ
	@echo "=== Waiting for services to be ready... ==="
	@sleep 5
	@echo "=== Submitting task destined for DLQ (mock_always_fail, max_attempts=2) ==="
	@curl -s -X POST http://localhost:8080/tasks \
		-H "Content-Type: application/json" \
		-d @examples/payloads/dlq.json | jq .
	@echo "=== Watching retry → DLQ flow (checking every 4s for 28s)... ==="
	@for i in 1 2 3 4 5 6 7; do \
		sleep 4; \
		echo "--- Check $$i ---"; \
		curl -s http://localhost:8080/tasks | jq '.[] | {id, status, attempt, last_error, dlq_reason}'; \
	done
	@echo ""
	@echo "=== DLQ tasks via status filter ==="
	@curl -s "http://localhost:8080/tasks?status=dlq" | jq '.[] | {id, status, dlq_reason}'

# ── Utility targets ─────────────────────────────────────────

logs: ## View all Docker Compose logs
	docker compose -f deployments/docker-compose.yml logs -f

logs-worker: ## View worker logs only
	docker compose -f deployments/docker-compose.yml logs -f worker

test: ## Run all tests
	go test ./...

fmt: ## Format Go source code
	go fmt ./...

clean: ## Remove build artifacts
	rm -rf bin/

# ── Quick start ─────────────────────────────────────────────

dev: up ## Start everything (alias for 'up')
