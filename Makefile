.PHONY: up down build api worker migrate submit-rag-task submit-multi list-tasks watch-tasks demo demo-claim demo-retry demo-timeout demo-dlq demo-failure-lab demo-rate-limit demo-observe loadgen benchmark-matrix chaos-matrix assetlab bench test integration race vet verify fmt logs logs-worker clean dev

GO_PACKAGES := ./cmd/... ./internal/... ./pkg/...
CURL_API = curl -s -H "Authorization: Bearer $(API_TOKEN)"
CURL_ADMIN = curl -s -H "Authorization: Bearer $(ADMIN_TOKEN)"

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
	@$(CURL_API) -X POST http://localhost:8080/tasks \
		-H "Content-Type: application/json" \
		-d @examples/payloads/rag_ingestion.json | jq .

submit-multi: ## Submit 5 tasks to show multi-worker claim
	@for i in 1 2 3 4 5; do \
		echo "=== Task $$i ==="; \
		$(CURL_API) -X POST http://localhost:8080/tasks \
			-H "Content-Type: application/json" \
			-d "{\"type\":\"rag_ingestion\",\"payload\":{\"version\":\"1\",\"kind\":\"rag_ingestion\",\"data\":{\"document_id\":\"doc-multi-$$i\",\"dataset_id\":\"ds-demo\",\"index_name\":\"demo-index\"}},\"max_attempts\":1,\"timeout_seconds\":30,\"retry_delay_seconds\":3}" | jq '{id, status}'; \
	done

list-tasks: ## List all tasks
	@$(CURL_API) http://localhost:8080/tasks | jq '.[] | {id, status, type, leased_by, attempt, last_error}'

watch-tasks: ## Watch tasks in real-time (requires 'watch' command)
	@watch -n 1 '$(CURL_API) http://localhost:8080/tasks | jq ".[] | {id, status, type, leased_by, attempt}"'

demo: up ## FULL DEMO: success → retry → DLQ — the complete story
	@echo ""
	@echo "=============================================================="
	@echo "  distributed-task-lab — Complete MVP Demo"
	@echo "  Showcases: claim, lease, retry, timeout, DLQ"
	@echo "=============================================================="
	@echo ""
	@sleep 5
	@echo "=== [1/7] Submitting a normal RAG ingestion task (will succeed) ==="
	@$(CURL_API) -X POST http://localhost:8080/tasks -H "Content-Type: application/json" \
		-d @examples/payloads/rag_ingestion.json | jq '{id, status}'
	@sleep 3
	@echo "=== [2/7] Task after worker claimed it ==="
	@$(CURL_API) http://localhost:8080/tasks | jq '.[] | {id, status, type, leased_by}'
	@sleep 2
	@echo "=== [3/7] Task completed (pending → running → success) ==="
	@$(CURL_API) http://localhost:8080/tasks | jq '.[] | {id, status, type, result}'
	@echo ""
	@echo "=== [4/7] Submitting a flaky task (fails once, then succeeds — retry demo) ==="
	@$(CURL_API) -X POST http://localhost:8080/tasks -H "Content-Type: application/json" \
		-d @examples/payloads/retry.json | jq '{id, status}'
	@echo "  Waiting for retry cycle..."
	@sleep 6
	@echo "=== [5/7] Task after retry (attempt increased, then succeeded) ==="
	@$(CURL_API) http://localhost:8080/tasks | jq '.[] | {id, status, attempt, last_error}'
	@echo ""
	@echo "=== [6/7] Submitting a doomed task (always fails → DLQ) ==="
	@$(CURL_API) -X POST http://localhost:8080/tasks -H "Content-Type: application/json" \
		-d @examples/payloads/dlq.json | jq '{id, status}'
	@echo "  Waiting for retry → DLQ..."
	@sleep 12
	@echo "=== [7/7] DLQ tasks ==="
	@$(CURL_API) "http://localhost:8080/tasks?status=dlq" | jq '.[] | {id, status, dlq_reason}'
	@echo ""
	@echo "=== Metrics ==="
	@$(CURL_API) http://localhost:8080/stats | jq .
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
		$(CURL_API) -X POST http://localhost:8080/tasks \
			-H "Content-Type: application/json" \
			-d "{\"type\":\"rag_ingestion\",\"payload\":{\"version\":\"1\",\"kind\":\"rag_ingestion\",\"data\":{\"document_id\":\"doc-$$i\",\"dataset_id\":\"ds-demo\",\"index_name\":\"demo-index\"}},\"max_attempts\":1,\"timeout_seconds\":30,\"retry_delay_seconds\":3}" | jq '{id, status}'; \
	done
	@sleep 3
	@echo "=== Current tasks ==="
	@$(CURL_API) http://localhost:8080/tasks | jq '.[] | {id, status, type, leased_by}'

demo-retry: up ## Phase 3: task fails once then succeeds
	@echo "=== Waiting for services to be ready... ==="
	@sleep 5
	@echo "=== Submitting task with mock_failures_before_success=1 ==="
	@$(CURL_API) -X POST http://localhost:8080/tasks \
		-H "Content-Type: application/json" \
		-d @examples/payloads/retry.json | jq .
	@echo "=== Watching retry (checking every 4s for 20s)... ==="
	@for i in 1 2 3 4 5; do \
		sleep 4; \
		echo "--- Check $$i ---"; \
		$(CURL_API) http://localhost:8080/tasks | jq '.[] | {id, status, attempt, last_error}'; \
	done

demo-timeout: up ## Phase 3: task times out
	@echo "=== Waiting for services to be ready... ==="
	@sleep 5
	@echo "=== Submitting task with short timeout ==="
	@$(CURL_API) -X POST http://localhost:8080/tasks \
		-H "Content-Type: application/json" \
		-d @examples/payloads/timeout.json | jq .
	@echo "=== Watching timeout + retry (checking every 3s for 15s)... ==="
	@for i in 1 2 3 4 5; do \
		sleep 3; \
		echo "--- Check $$i ---"; \
		$(CURL_API) http://localhost:8080/tasks | jq '.[] | {id, status, attempt, last_error}'; \
	done

demo-dlq: up ## Phase 4: task goes through retry → DLQ
	@echo "=== Waiting for services to be ready... ==="
	@sleep 5
	@echo "=== Submitting task destined for DLQ (mock_always_fail, max_attempts=2) ==="
	@$(CURL_API) -X POST http://localhost:8080/tasks \
		-H "Content-Type: application/json" \
		-d @examples/payloads/dlq.json | jq .
	@echo "=== Watching retry → DLQ flow (checking every 4s for 28s)... ==="
	@for i in 1 2 3 4 5 6 7; do \
		sleep 4; \
		echo "--- Check $$i ---"; \
		$(CURL_API) http://localhost:8080/tasks | jq '.[] | {id, status, attempt, last_error, dlq_reason}'; \
	done
	@echo ""
	@echo "=== DLQ tasks via status filter ==="
	@$(CURL_API) "http://localhost:8080/tasks?status=dlq" | jq '.[] | {id, status, dlq_reason}'

demo-failure-lab: ## Run 12 database-oracled crash, lease, fencing, rate, and shutdown scenarios
	@docker rm -f tasklab-failurelab 2>/dev/null || true
	@docker run -d --name tasklab-failurelab -p 5434:5432 \
		-e POSTGRES_USER=postgres -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=tasklab \
		postgres:16-alpine >/dev/null
	@echo "Waiting for Postgres..."
	@for i in 1 2 3 4 5 6 7 8 9 10; do \
		docker exec tasklab-failurelab pg_isready -U postgres >/dev/null 2>&1 && break; \
		sleep 1; \
	done
	@DATABASE_URL=postgres://postgres:postgres@localhost:5434/tasklab?sslmode=disable go run ./cmd/failurelab
	@docker rm -f tasklab-failurelab >/dev/null

demo-rate-limit: up ## Queue rate limit: 6 tasks dispatched at 2/s
	@echo "Waiting for services..."
	@sleep 5
	@echo "=== Configuring rag_ingestion queue: 2 tasks/sec, burst 1 ==="
	@$(CURL_ADMIN) -X PUT http://localhost:8080/queues/rag_ingestion -H "Content-Type: application/json" \
		-d '{"max_concurrency":10,"rate_limit_per_second":2,"burst":1}' | jq .
	@echo "=== Submitting 6 tasks to the rag_ingestion queue ==="
	@for i in 1 2 3 4 5 6; do \
		$(CURL_API) -X POST http://localhost:8080/tasks -H "Content-Type: application/json" \
			-d "{\"type\":\"rag_ingestion\",\"queue\":\"rag_ingestion\",\"payload\":{\"version\":\"1\",\"kind\":\"rag_ingestion\",\"data\":{\"document_id\":\"doc-rl-$$i\",\"mock_duration_ms\":200}},\"max_attempts\":1,\"timeout_seconds\":30}" | jq '{id, status}'; \
	done
	@echo "=== Watching dispatch (success count should grow ~2/s) ==="
	@for i in 1 2 3 4 5; do \
		sleep 1; \
		echo "--- t+$$i s: $$($(CURL_API) 'http://localhost:8080/tasks?status=success' | jq 'length') succeeded ---"; \
	done

demo-observe: up ## Observability: Prometheus (:9090) + Grafana (:3000) with tasklab dashboard
	@docker compose -f deployments/observability/docker-compose.observability.yml up -d
	@echo ""
	@echo "Prometheus : http://localhost:9090"
	@echo "Grafana    : http://localhost:3000  (dashboard: distributed-task-lab Overview)"
	@echo "Submit some tasks first (make submit-multi) to see metrics."

loadgen: ## Load test: throughput + claim latency + duplicate detection (dedicated Postgres)
	@docker rm -f tasklab-loadgen 2>/dev/null || true
	@docker run -d --name tasklab-loadgen -p 5435:5432 \
		-e POSTGRES_USER=postgres -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=tasklab \
		postgres:16-alpine >/dev/null
	@for i in 1 2 3 4 5 6 7 8 9 10; do \
		docker exec tasklab-loadgen pg_isready -U postgres >/dev/null 2>&1 && break; \
		sleep 1; \
	done
	@DATABASE_URL=postgres://postgres:postgres@localhost:5435/tasklab?sslmode=disable go run ./cmd/loadgen
	@docker rm -f tasklab-loadgen >/dev/null

benchmark-matrix: ## Run the fixed 1/4/8/16-worker benchmark matrix and keep evidence bundles
	@for workers in 1 4 8 16; do \
		LOADGEN_WORKERS=$$workers LOADGEN_CONCURRENCY=1 LOADGEN_TASKS=1000 LOADGEN_QUEUES=4 LOADGEN_PAYLOAD_SIZE=1024 LOADGEN_PROFILE=backlog-drain LOADGEN_RUN_ID=workers-$$workers-queues-4-payload-1k $(MAKE) loadgen; \
	done

chaos-matrix: ## Repeat crash/lease/fencing/effect recovery oracles and write distributions
	@docker rm -f tasklab-chaosmatrix 2>/dev/null || true
	@docker run -d --name tasklab-chaosmatrix -p 5436:5432 \
		-e POSTGRES_USER=postgres -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=tasklab \
		postgres:16-alpine >/dev/null
	@for i in 1 2 3 4 5 6 7 8 9 10; do docker exec tasklab-chaosmatrix pg_isready -U postgres >/dev/null 2>&1 && break; sleep 1; done
	@TEST_DATABASE_URL=postgres://postgres:postgres@localhost:5436/postgres?sslmode=disable go run ./cmd/chaosmatrix
	@docker rm -f tasklab-chaosmatrix >/dev/null

assetlab: ## Run asset-derived histories/oracles (requires ASSETLAB_RESULT_DIR and DATABASE_URL)
	@test -n "$(ASSETLAB_RESULT_DIR)" || (echo "ASSETLAB_RESULT_DIR is required and must be new"; exit 2)
	@go run ./cmd/assetlab

bench: ## Run Go benchmarks (requires a test Postgres on :5433)
	@TEST_DATABASE_URL=postgres://postgres:postgres@localhost:5433/postgres?sslmode=disable \
		go test ./internal/storage/postgres/ -run '^$$' -bench . -benchtime=5s

# ── Utility targets ─────────────────────────────────────────

logs: ## View all Docker Compose logs
	docker compose -f deployments/docker-compose.yml logs -f

logs-worker: ## View worker logs only
	docker compose -f deployments/docker-compose.yml logs -f worker

test: ## Run all tests
	go test $(GO_PACKAGES)

integration: ## Run required PostgreSQL integration tests (never silently skips)
	REQUIRE_TEST_DATABASE=1 go test ./internal/storage/postgres -count=1 -v

race: ## Run race detector across production packages
	go test -race $(GO_PACKAGES) -count=1

vet: ## Run go vet across production packages
	go vet $(GO_PACKAGES)

verify: fmt vet test race ## Run local production-package verification

fmt: ## Format Go source code
	go fmt $(GO_PACKAGES)

clean: ## Remove build artifacts
	rm -rf bin/

# ── Quick start ─────────────────────────────────────────────

dev: up ## Start everything (alias for 'up')
