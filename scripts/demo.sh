#!/usr/bin/env bash
# demo.sh — Full workflow demonstration for distributed-task-lab
# Requires: curl, jq, docker compose
set -euo pipefail

API="http://localhost:8080"
BLUE='\033[0;34m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

header() {
    echo ""
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
}

step() {
    echo ""
    echo -e "${YELLOW}▶ $1${NC}"
}

check_deps() {
    for cmd in curl jq docker; do
        if ! command -v "$cmd" &>/dev/null; then
            echo -e "${RED}Error: $cmd is required but not installed.${NC}"
            exit 1
        fi
    done
}

# ── Main Demo ───────────────────────────────────────────────

header "distributed-task-lab — Full Workflow Demo"

check_deps

# 1. Start the system
step "Starting services (PostgreSQL + API + 2 Workers)..."
docker compose -f deployments/docker-compose.yml up -d --build
echo "Waiting for services to be ready..."
sleep 5
echo -e "${GREEN}✓ Services running${NC}"

# 2. Health check
step "Checking API health..."
curl -s "$API/health" | jq .
echo -e "${GREEN}✓ API is healthy${NC}"

# 3. Submit a RAG ingestion task (success)
header "SCENARIO 1: Successful RAG Ingestion Task"

step "Submitting RAG ingestion task..."
TASK1=$(curl -s -X POST "$API/tasks" \
    -H "Content-Type: application/json" \
    -d @examples/payloads/rag_ingestion.json)
echo "$TASK1" | jq .
TASK1_ID=$(echo "$TASK1" | jq -r '.id')

step "Querying task (should be pending → Worker claims → success)..."
sleep 3
curl -s "$API/tasks/$TASK1_ID" | jq '{id, status, type, attempt, result}'

# 4. Submit a failing task → retry
header "SCENARIO 2: Retry After Failure"

step "Submitting task that fails (will retry)..."
TASK2=$(curl -s -X POST "$API/tasks" \
    -H "Content-Type: application/json" \
    -d @examples/payloads/retry.json)
echo "$TASK2" | jq .
TASK2_ID=$(echo "$TASK2" | jq -r '.id')

step "Watching retry behavior (checking every 5s)..."
for i in 1 2 3 4; do
    sleep 5
    echo "--- Check $i ---"
    curl -s "$API/tasks/$TASK2_ID" | jq '{id, status, attempt, last_error}'
done

# 5. Submit a task → DLQ
header "SCENARIO 3: Dead Letter Queue (DLQ)"

step "Submitting task with max_attempts=1 (fails → DLQ)..."
TASK3=$(curl -s -X POST "$API/tasks" \
    -H "Content-Type: application/json" \
    -d @examples/payloads/dlq.json)
echo "$TASK3" | jq .
TASK3_ID=$(echo "$TASK3" | jq -r '.id')

sleep 8
step "Checking task (should be in DLQ)..."
curl -s "$API/tasks/$TASK3_ID" | jq '{id, status, dlq_reason, last_error}'

step "Querying all DLQ tasks..."
curl -s "$API/tasks?status=dlq" | jq '.[] | {id, type, dlq_reason}'

# 6. Summary
header "DEMO SUMMARY"

step "All tasks in the system:"
curl -s "$API/tasks" | jq '.[] | {id, status, type, attempt}'

echo ""
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}  Demo complete!${NC}"
echo ""
echo "  Try these commands:"
echo "    make logs              — View all service logs"
echo "    make submit-rag-task   — Submit another task"
echo "    make watch-tasks       — Watch tasks in real-time"
echo "    make down              — Stop all services"
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
