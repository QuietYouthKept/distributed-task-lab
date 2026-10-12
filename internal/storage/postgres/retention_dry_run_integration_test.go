package postgres

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
)

func TestRetentionDryRunListsOnlyConservativeTerminalCandidates(t *testing.T) {
	q := setupTestDB(t)
	cutoff := time.Now().UTC().Add(-24 * time.Hour)
	old := cutoff.Add(-time.Hour)
	base := newTask("retention-candidate", 1)
	base.Status, base.FinishedAt, base.Result = domain.TaskStatusSuccess, &old, nil
	if _, err := q.Enqueue(context.Background(), base); err != nil {
		t.Fatal(err)
	}
	if _, err := q.db.Exec(`UPDATE tasks SET status='success', finished_at=$2, updated_at=$2 WHERE id=$1`, base.ID, old); err != nil {
		t.Fatal(err)
	}
	if _, err := q.db.Exec(`INSERT INTO task_events (task_id, event_type, message) VALUES ($1, 'success', 'retention event')`, base.ID); err != nil {
		t.Fatal(err)
	}

	withResult := newTask("retention-has-result", 1)
	withResult.Status, withResult.FinishedAt = domain.TaskStatusSuccess, &old
	result, _ := json.Marshal(map[string]string{"artifact_uri": "s3://private/bucket/object"})
	if _, err := q.db.Exec(`INSERT INTO tasks (id, type, status, payload, result, finished_at) VALUES ($1,'eval','success','{}',$2,$3)`, withResult.ID, result, old); err != nil {
		t.Fatal(err)
	}
	withEffect := newTask("retention-has-effect", 1)
	withEffect.Status, withEffect.FinishedAt = domain.TaskStatusFailed, &old
	if _, err := q.Enqueue(context.Background(), withEffect); err != nil {
		t.Fatal(err)
	}
	if _, err := q.db.Exec(`UPDATE tasks SET status='failed', finished_at=$2 WHERE id=$1`, withEffect.ID, old); err != nil {
		t.Fatal(err)
	}
	if _, err := q.db.Exec(`INSERT INTO effects (effect_id,task_id,operation_key,kind,policy,status,request_hash,generation,attempt) VALUES ('00000000-0000-4000-8000-000000000001',$1,'key','notify','idempotent','unknown','hash',1,1)`, withEffect.ID); err != nil {
		t.Fatal(err)
	}
	withAudit := newTask("retention-has-audit", 1)
	withAudit.Status, withAudit.FinishedAt = domain.TaskStatusDLQ, &old
	if _, err := q.Enqueue(context.Background(), withAudit); err != nil {
		t.Fatal(err)
	}
	if _, err := q.db.Exec(`UPDATE tasks SET status='dlq', finished_at=$2 WHERE id=$1`, withAudit.ID, old); err != nil {
		t.Fatal(err)
	}
	if _, err := q.db.Exec(`INSERT INTO security_audit_events (actor_id,actor_kind,request_id,action,resource_type,resource_id,outcome,reason) VALUES ('test','service','retention-test','read','task',$1,'success','')`, withAudit.ID); err != nil {
		t.Fatal(err)
	}
	newer := newTask("retention-too-new", 1)
	newer.Status, newer.FinishedAt = domain.TaskStatusSuccess, &cutoff
	if _, err := q.Enqueue(context.Background(), newer); err != nil {
		t.Fatal(err)
	}
	if _, err := q.db.Exec(`UPDATE tasks SET status='success', finished_at=$2 WHERE id=$1`, newer.ID, cutoff); err != nil {
		t.Fatal(err)
	}

	var eventCountBefore int
	if err := q.db.QueryRow(`SELECT COUNT(*) FROM task_events WHERE task_id=$1`, base.ID).Scan(&eventCountBefore); err != nil {
		t.Fatal(err)
	}
	candidates, err := ListRetentionCandidates(context.Background(), q.db, cutoff, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].TaskID != base.ID || candidates[0].Status != "success" {
		t.Fatalf("conservative candidate set = %+v, want only %s", candidates, base.ID)
	}
	var eventCount int
	if err := q.db.QueryRow(`SELECT COUNT(*) FROM task_events WHERE task_id=$1`, base.ID).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if eventCount != eventCountBefore {
		t.Fatalf("dry run mutated task events: before=%d after=%d", eventCountBefore, eventCount)
	}
}

func TestRetentionDryRunRejectsUnsafeBounds(t *testing.T) {
	q := setupTestDB(t)
	for _, tc := range []struct {
		cutoff time.Time
		limit  int
	}{{time.Time{}, 1}, {time.Now().Add(time.Minute), 1}, {time.Now(), 0}, {time.Now(), retentionDryRunBatchMax + 1}} {
		if _, err := ListRetentionCandidates(context.Background(), q.db, tc.cutoff, tc.limit); err == nil {
			t.Errorf("unsafe retention parameters accepted: cutoff=%v limit=%d", tc.cutoff, tc.limit)
		}
	}
}
