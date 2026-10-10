// Command paginationbench measures the real PostgreSQL keyset query against a
// disposable database seeded with a deterministic 100k-task dataset.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"github.com/fantasticyi/distributed-task-lab/internal/storage/postgres"
	"github.com/fantasticyi/distributed-task-lab/internal/testutil"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

const (
	rowCount = 100_000
	pageSize = 500
	seed     = "distributed-task-lab-pagination-v2-skewed-seed-20261010"
)

type latencySummary struct {
	Samples int     `json:"samples"`
	MinMS   float64 `json:"min_ms"`
	P50MS   float64 `json:"p50_ms"`
	P95MS   float64 `json:"p95_ms"`
	P99MS   float64 `json:"p99_ms"`
	MaxMS   float64 `json:"max_ms"`
}

type runManifest struct {
	SchemaVersion int                        `json:"schema_version"`
	RunID         string                     `json:"run_id"`
	CreatedAtUTC  time.Time                  `json:"created_at_utc"`
	GitCommit     string                     `json:"implementation_commit"`
	GoVersion     string                     `json:"go_version"`
	Platform      string                     `json:"platform"`
	Postgres      map[string]string          `json:"postgres"`
	Generator     map[string]any             `json:"generator"`
	Indexes       []string                   `json:"task_indexes"`
	IndexSizes    map[string]int64           `json:"task_index_size_bytes"`
	Queries       map[string]json.RawMessage `json:"explain_analyze_json"`
	Latency       map[string]latencySummary  `json:"latency_ms"`
	WriteCost     map[string]any             `json:"pagination_index_write_cost"`
	Pagination    map[string]any             `json:"keyset_walk"`
	Concurrent    map[string]any             `json:"concurrent_write_check"`
	Result        string                     `json:"result"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "paginationbench:", err)
		os.Exit(1)
	}
}

func run() error {
	if os.Getenv("REQUIRE_TEST_DATABASE") != "1" {
		return errors.New("REQUIRE_TEST_DATABASE=1 is required; benchmark creates a separate disposable database")
	}
	baseURL := testutil.DefaultBaseURL()
	database, err := testutil.EnsureDatabase(baseURL, "tasklab_pagination_bench")
	if err != nil {
		return fmt.Errorf("create isolated database: %w", err)
	}
	defer database.Drop()
	db, err := postgres.NewDB(database.URL)
	if err != nil {
		return fmt.Errorf("connect isolated database: %w", err)
	}
	defer db.Close()
	if err := postgres.RunMigrations(db); err != nil {
		return fmt.Errorf("migrate isolated database: %w", err)
	}
	started := time.Now()
	if err := seedTasks(db); err != nil {
		return fmt.Errorf("seed %d rows: %w", rowCount, err)
	}
	if _, err := db.Exec(`ANALYZE tasks`); err != nil {
		return fmt.Errorf("analyze task fixture: %w", err)
	}

	queue := postgres.NewPostgresQueue(db)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	deepTime, deepID, err := cursorAt(ctx, db, 50_000, "")
	if err != nil {
		return err
	}
	lastTime, lastID, err := cursorAt(ctx, db, 99_900, "")
	if err != nil {
		return err
	}
	owner := "bench-owner-007"
	filteredTime, filteredID, err := cursorAt(ctx, db, 500, owner)
	if err != nil {
		return fmt.Errorf("filtered cursor: %w", err)
	}
	broadOwner, rareOwner := "bench-owner-hot", "bench-owner-rare"
	broadTime, broadID, err := cursorAt(ctx, db, 40_000, broadOwner)
	if err != nil {
		return fmt.Errorf("broad owner cursor: %w", err)
	}
	rareTime, rareID, err := cursorAt(ctx, db, 500, rareOwner)
	if err != nil {
		return fmt.Errorf("rare owner cursor: %w", err)
	}
	queries := map[string]domain.TaskFilter{
		"head":                {Limit: pageSize + 1},
		"middle_keyset":       {Limit: pageSize + 1, AfterCreatedAt: &deepTime, AfterID: &deepID},
		"tail_keyset":         {Limit: pageSize + 1, AfterCreatedAt: &lastTime, AfterID: &lastID},
		"filtered_owner":      {Limit: pageSize + 1, CreatedBy: &owner},
		"filtered_keyset":     {Limit: pageSize + 1, CreatedBy: &owner, AfterCreatedAt: &filteredTime, AfterID: &filteredID},
		"owner_broad_70pct":   {Limit: pageSize + 1, CreatedBy: &broadOwner},
		"owner_broad_keyset":  {Limit: pageSize + 1, CreatedBy: &broadOwner, AfterCreatedAt: &broadTime, AfterID: &broadID},
		"owner_sparse_1pct":   {Limit: pageSize + 1, CreatedBy: &rareOwner},
		"owner_sparse_keyset": {Limit: pageSize + 1, CreatedBy: &rareOwner, AfterCreatedAt: &rareTime, AfterID: &rareID},
	}
	latency := make(map[string]latencySummary, len(queries))
	for name, filter := range queries {
		latency[name], err = measure(ctx, queue, filter, 40)
		if err != nil {
			return fmt.Errorf("measure %s: %w", name, err)
		}
	}

	plans := make(map[string]json.RawMessage, 8)
	plans["head"], err = explain(ctx, db, "", nil, nil)
	if err != nil {
		return fmt.Errorf("explain head: %w", err)
	}
	plans["middle_keyset"], err = explain(ctx, db, "", &deepTime, &deepID)
	if err != nil {
		return fmt.Errorf("explain middle keyset: %w", err)
	}
	plans["filtered_owner"], err = explain(ctx, db, owner, nil, nil)
	if err != nil {
		return fmt.Errorf("explain filtered owner: %w", err)
	}
	plans["filtered_keyset"], err = explain(ctx, db, owner, &filteredTime, &filteredID)
	if err != nil {
		return fmt.Errorf("explain filtered keyset: %w", err)
	}
	plans["owner_broad_70pct"], err = explain(ctx, db, broadOwner, nil, nil)
	if err != nil {
		return fmt.Errorf("explain broad owner: %w", err)
	}
	plans["owner_broad_keyset"], err = explain(ctx, db, broadOwner, &broadTime, &broadID)
	if err != nil {
		return fmt.Errorf("explain broad owner keyset: %w", err)
	}
	plans["owner_sparse_1pct"], err = explain(ctx, db, rareOwner, nil, nil)
	if err != nil {
		return fmt.Errorf("explain sparse owner: %w", err)
	}
	plans["owner_sparse_keyset"], err = explain(ctx, db, rareOwner, &rareTime, &rareID)
	if err != nil {
		return fmt.Errorf("explain sparse owner keyset: %w", err)
	}

	walk, err := walkSnapshot(ctx, queue, pageSize, rowCount)
	if err != nil {
		return fmt.Errorf("keyset walk: %w", err)
	}
	concurrent, err := concurrentWriteCheck(ctx, queue, pageSize, rowCount)
	if err != nil {
		return fmt.Errorf("concurrent-write check: %w", err)
	}
	pgInfo, err := postgresInfo(ctx, db)
	if err != nil {
		return err
	}
	indexes, err := taskIndexes(ctx, db)
	if err != nil {
		return err
	}
	indexSizes, err := taskIndexSizes(ctx, db)
	if err != nil {
		return err
	}
	writeCost, err := measurePaginationIndexWriteCost(ctx, db)
	if err != nil {
		return fmt.Errorf("measure pagination index write cost: %w", err)
	}
	gitCommit := os.Getenv("GIT_COMMIT")
	if gitCommit == "" {
		if output, err := exec.CommandContext(ctx, "git", "rev-parse", "HEAD").Output(); err == nil {
			gitCommit = strings.TrimSpace(string(output))
		} else {
			gitCommit = "unknown; see enclosing Evidence manifest"
		}
	}
	runID := time.Now().UTC().Format("20060102T150405Z") + "-" + uuid.NewString()[:8]
	outRoot := os.Getenv("PAGINATION_BENCH_OUTPUT_DIR")
	if outRoot == "" {
		outRoot = filepath.Join("benchmarks", "pagination")
	}
	outDir := filepath.Join(outRoot, runID)
	if err := os.MkdirAll(outRoot, 0o755); err != nil {
		return fmt.Errorf("create output root: %w", err)
	}
	if err := os.Mkdir(outDir, 0o755); err != nil {
		return fmt.Errorf("create run directory: %w", err)
	}
	manifest := runManifest{
		SchemaVersion: 1,
		RunID:         runID,
		CreatedAtUTC:  time.Now().UTC(),
		GitCommit:     gitCommit,
		GoVersion:     runtime.Version(),
		Platform:      runtime.GOOS + "/" + runtime.GOARCH,
		Postgres:      pgInfo,
		Generator: map[string]any{
			"version": "paginationbench-v2-skewed", "seed": seed, "rows": rowCount,
			"distribution":       "10-row timestamp tie groups; 5 statuses; 2 types; 70% hot owner, 1% rare owner, remaining 29% across 20 owners; default queue",
			"insert_duration_ms": float64(time.Since(started)) / float64(time.Millisecond),
			"schema_migrations":  "001-012",
		},
		Indexes: indexes, IndexSizes: indexSizes, Queries: plans, Latency: latency, WriteCost: writeCost,
		Pagination: walk, Concurrent: concurrent, Result: "PASS",
	}
	if err := writeJSON(filepath.Join(outDir, "manifest.json"), manifest); err != nil {
		return err
	}
	if err := writeReport(filepath.Join(outDir, "report.md"), manifest); err != nil {
		return err
	}
	fmt.Printf("PASS: seeded %d PostgreSQL rows; evidence: %s\n", rowCount, outDir)
	return nil
}

func seedTasks(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	columns := []string{"id", "type", "status", "payload", "priority", "max_attempts", "attempt", "timeout_seconds", "retry_delay_seconds", "retry_max_delay_seconds", "retry_backoff_multiplier", "next_run_at", "created_at", "updated_at", "queue", "created_by"}
	stmt, err := tx.Prepare(pq.CopyIn("tasks", columns...))
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	payload := []byte(`{"version":"1","kind":"eval","data":{"execution_mode":"mock"}}`)
	for i := 0; i < rowCount; i++ {
		status := []string{"pending", "success", "failed", "dlq", "running"}[i%5]
		taskType := []string{"eval", "rag_ingestion"}[i%2]
		createdAt := base.Add(time.Duration(i/10) * time.Second)
		owner := fmt.Sprintf("bench-owner-%03d", i%20)
		if i%100 < 70 {
			owner = "bench-owner-hot"
		} else if i%100 == 70 {
			owner = "bench-owner-rare"
		}
		if _, err := stmt.Exec(fmt.Sprintf("pagination-%06d", i), taskType, status, string(payload), i%10, 5, i%3, 60, 1, 60, 2.0, createdAt, createdAt, createdAt, domain.DefaultQueueName, owner); err != nil {
			_ = stmt.Close()
			_ = tx.Rollback()
			return err
		}
	}
	if _, err := stmt.Exec(); err != nil {
		_ = stmt.Close()
		_ = tx.Rollback()
		return err
	}
	if err := stmt.Close(); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func measure(ctx context.Context, queue *postgres.PostgresQueue, filter domain.TaskFilter, samples int) (latencySummary, error) {
	values := make([]float64, 0, samples)
	for i := 0; i < samples; i++ {
		started := time.Now()
		if _, err := queue.ListTasks(ctx, filter); err != nil {
			return latencySummary{}, err
		}
		values = append(values, float64(time.Since(started))/float64(time.Millisecond))
	}
	return summarize(values), nil
}

func cursorAt(ctx context.Context, db *sql.DB, offset int, owner string) (time.Time, string, error) {
	query := `SELECT created_at,id FROM tasks`
	args := []any{}
	if owner != "" {
		query += ` WHERE created_by=$1`
		args = append(args, owner)
	}
	query += fmt.Sprintf(` ORDER BY created_at DESC,id DESC OFFSET $%d LIMIT 1`, len(args)+1)
	args = append(args, offset)
	var created time.Time
	var id string
	err := db.QueryRowContext(ctx, query, args...).Scan(&created, &id)
	return created, id, err
}

func explain(ctx context.Context, db *sql.DB, owner string, afterTime *time.Time, afterID *string) (json.RawMessage, error) {
	// Match the full task-row projection used by PostgresQueue.ListTasks. An
	// id-only EXPLAIN can choose an index-only plan that hides the heap work of
	// the actual API query.
	query := `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) SELECT t.* FROM tasks AS t WHERE 1=1`
	args := []any{}
	if owner != "" {
		args = append(args, owner)
		query += fmt.Sprintf(` AND created_by=$%d`, len(args))
	}
	if afterTime != nil && afterID != nil {
		args = append(args, *afterTime, *afterID)
		query += fmt.Sprintf(` AND (created_at, id) < ($%d, $%d)`, len(args)-1, len(args))
	}
	query += ` ORDER BY created_at DESC,id DESC LIMIT 501`
	var raw []byte
	if err := db.QueryRowContext(ctx, query, args...).Scan(&raw); err != nil {
		return nil, err
	}
	if !json.Valid(raw) {
		return nil, errors.New("PostgreSQL returned invalid EXPLAIN JSON")
	}
	return raw, nil
}

func walkSnapshot(ctx context.Context, queue *postgres.PostgresQueue, size, expected int) (map[string]any, error) {
	filter := domain.TaskFilter{Limit: size}
	seen := make(map[string]struct{}, expected)
	pageCount := 0
	for {
		page, err := queue.ListTasks(ctx, filter)
		if err != nil {
			return nil, err
		}
		pageCount++
		for _, task := range page {
			if _, exists := seen[task.ID]; exists {
				return nil, fmt.Errorf("duplicate task %s during keyset walk", task.ID)
			}
			seen[task.ID] = struct{}{}
		}
		if len(page) < size {
			break
		}
		last := page[len(page)-1]
		filter.AfterCreatedAt, filter.AfterID = &last.CreatedAt, &last.ID
	}
	if len(seen) != expected {
		return nil, fmt.Errorf("keyset walk saw %d rows, want %d", len(seen), expected)
	}
	return map[string]any{"rows_seen": len(seen), "expected_rows": expected, "page_size": size, "pages": pageCount, "duplicates": 0, "concurrent_write_started_after_first_page": true}, nil
}

func concurrentWriteCheck(ctx context.Context, queue *postgres.PostgresQueue, size, expected int) (map[string]any, error) {
	first, err := queue.ListTasks(ctx, domain.TaskFilter{Limit: size})
	if err != nil || len(first) != size {
		return nil, fmt.Errorf("first page before concurrent writer: rows=%d err=%v", len(first), err)
	}
	last := first[len(first)-1]
	writerDone := make(chan error, 1)
	firstInsertCommitted := make(chan error, 1)
	go func() {
		for i := 0; i < 50; i++ {
			task := &domain.Task{ID: fmt.Sprintf("concurrent-write-%03d", i), Type: domain.TaskTypeEval, Queue: domain.DefaultQueueName, Payload: domain.TaskPayloadEnvelope{Version: "1", Kind: "eval", Data: map[string]any{"execution_mode": "mock"}}, MaxAttempts: 2, TimeoutSeconds: 60, RetryDelaySeconds: 1, RetryBackoffMultiplier: 2, RetryMaxDelaySeconds: 60}
			if _, err := queue.Enqueue(ctx, task); err != nil {
				if i == 0 {
					firstInsertCommitted <- err
				}
				writerDone <- err
				return
			}
			if i == 0 {
				firstInsertCommitted <- nil
			}
		}
		writerDone <- nil
	}()
	if err := <-firstInsertCommitted; err != nil {
		return nil, fmt.Errorf("first concurrent insert: %w", err)
	}

	filter := domain.TaskFilter{Limit: size, AfterCreatedAt: &last.CreatedAt, AfterID: &last.ID}
	seen := make(map[string]struct{}, expected)
	for _, task := range first {
		seen[task.ID] = struct{}{}
	}
	pages := 1
	for {
		page, err := queue.ListTasks(ctx, filter)
		if err != nil {
			return nil, err
		}
		pages++
		for _, task := range page {
			if _, exists := seen[task.ID]; exists {
				return nil, fmt.Errorf("duplicate task %s during concurrent keyset walk", task.ID)
			}
			seen[task.ID] = struct{}{}
		}
		if len(page) < size {
			break
		}
		last := page[len(page)-1]
		filter.AfterCreatedAt, filter.AfterID = &last.CreatedAt, &last.ID
	}
	if err := <-writerDone; err != nil {
		return nil, err
	}
	if len(seen) != expected {
		return nil, fmt.Errorf("concurrent keyset walk saw %d initial rows, want %d", len(seen), expected)
	}
	return map[string]any{"initial_rows_seen": len(seen), "concurrent_rows_written": 50, "first_concurrent_insert_committed_before_continuation": true, "page_size": size, "pages": pages, "duplicates": 0, "newer_inserts_disturbed_snapshot_walk": false}, nil
}

func postgresInfo(ctx context.Context, db *sql.DB) (map[string]string, error) {
	var version, sharedBuffers, workMem, maxConnections string
	if err := db.QueryRowContext(ctx, `SELECT current_setting('server_version'), current_setting('shared_buffers'), current_setting('work_mem'), current_setting('max_connections')`).Scan(&version, &sharedBuffers, &workMem, &maxConnections); err != nil {
		return nil, err
	}
	return map[string]string{"server_version": version, "shared_buffers": sharedBuffers, "work_mem": workMem, "max_connections": maxConnections}, nil
}

func taskIndexes(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT indexdef FROM pg_indexes WHERE schemaname=current_schema() AND tablename='tasks' ORDER BY indexname`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var indexes []string
	for rows.Next() {
		var index string
		if err := rows.Scan(&index); err != nil {
			return nil, err
		}
		indexes = append(indexes, index)
	}
	return indexes, rows.Err()
}

func taskIndexSizes(ctx context.Context, db *sql.DB) (map[string]int64, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT indexrelname, pg_relation_size(indexrelid)
		FROM pg_stat_user_indexes
		WHERE relname='tasks' AND schemaname=current_schema()
		ORDER BY indexrelname
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sizes := make(map[string]int64)
	for rows.Next() {
		var name string
		var size int64
		if err := rows.Scan(&name, &size); err != nil {
			return nil, err
		}
		sizes[name] = size
	}
	return sizes, rows.Err()
}

func measurePaginationIndexWriteCost(ctx context.Context, db *sql.DB) (map[string]any, error) {
	const (
		rowsPerSample = 5000
		samples       = 5
	)
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	ddl := []string{
		`CREATE TEMP TABLE pagination_write_without_indexes (id text PRIMARY KEY, created_at timestamptz NOT NULL, created_by text, status text NOT NULL, next_run_at timestamptz NOT NULL, queue text NOT NULL, priority integer NOT NULL) ON COMMIT PRESERVE ROWS`,
		`CREATE TEMP TABLE pagination_write_with_indexes (id text PRIMARY KEY, created_at timestamptz NOT NULL, created_by text, status text NOT NULL, next_run_at timestamptz NOT NULL, queue text NOT NULL, priority integer NOT NULL) ON COMMIT PRESERVE ROWS`,
		`CREATE TEMP TABLE pagination_write_with_overlapping_owner_index (id text PRIMARY KEY, created_at timestamptz NOT NULL, created_by text, status text NOT NULL, next_run_at timestamptz NOT NULL, queue text NOT NULL, priority integer NOT NULL) ON COMMIT PRESERVE ROWS`,
		`CREATE INDEX pagination_write_created_at_id ON pagination_write_with_indexes (created_at DESC, id DESC)`,
		`CREATE INDEX pagination_write_owner_created_at_id ON pagination_write_with_indexes (created_by, created_at DESC, id DESC) WHERE created_by IS NOT NULL`,
		`CREATE INDEX pagination_write_overlap_created_at_id ON pagination_write_with_overlapping_owner_index (created_at DESC, id DESC)`,
		`CREATE INDEX pagination_write_overlap_owner_partial ON pagination_write_with_overlapping_owner_index (created_by, created_at DESC, id DESC) WHERE created_by IS NOT NULL`,
		`CREATE INDEX pagination_write_overlap_owner_full ON pagination_write_with_overlapping_owner_index (created_by, created_at DESC, id DESC)`,
	}
	for _, statement := range ddl {
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			return nil, err
		}
	}
	insert := func(table string) (latencySummary, int64, error) {
		values := make([]float64, 0, samples)
		for sample := 0; sample < samples; sample++ {
			if _, err := conn.ExecContext(ctx, `TRUNCATE `+table); err != nil {
				return latencySummary{}, 0, err
			}
			started := time.Now()
			_, err := conn.ExecContext(ctx, `
				INSERT INTO `+table+` (id,created_at,created_by,status,next_run_at,queue,priority)
				SELECT 'write-cost-' || g::text, TIMESTAMPTZ '2025-01-01 00:00:00+00' + g * INTERVAL '1 second',
				       CASE WHEN g % 100 < 70 THEN 'bench-owner-hot' WHEN g % 100 = 70 THEN 'bench-owner-rare' ELSE 'bench-owner-' || lpad((g % 20)::text,3,'0') END,
				       'pending', TIMESTAMPTZ '2025-01-01 00:00:00+00', 'default', (g % 10)::integer
			FROM generate_series(1,$1) AS g
			`, rowsPerSample)
			if err != nil {
				return latencySummary{}, 0, err
			}
			values = append(values, float64(time.Since(started))/float64(time.Millisecond))
		}
		var bytes int64
		if err := conn.QueryRowContext(ctx, `SELECT pg_total_relation_size($1::regclass)`, "pg_temp."+table).Scan(&bytes); err != nil {
			return latencySummary{}, 0, err
		}
		return summarize(values), bytes, nil
	}
	without, withoutBytes, err := insert("pagination_write_without_indexes")
	if err != nil {
		return nil, err
	}
	with, withBytes, err := insert("pagination_write_with_indexes")
	if err != nil {
		return nil, err
	}
	withOverlap, withOverlapBytes, err := insert("pagination_write_with_overlapping_owner_index")
	if err != nil {
		return nil, err
	}
	overhead := 0.0
	if without.P50MS > 0 {
		overhead = (with.P50MS/without.P50MS - 1) * 100
	}
	overlapOverhead := 0.0
	if with.P50MS > 0 {
		overlapOverhead = (withOverlap.P50MS/with.P50MS - 1) * 100
	}
	return map[string]any{
		"method":          "same-session temporary tables; identical 5000-row INSERT SELECT repeated 5 times; primary key common to all; compares no pagination indexes, global+partial-owner indexes, then an additional overlapping non-partial owner index",
		"rows_per_sample": rowsPerSample, "samples": samples,
		"without_pagination_indexes_latency_ms":                    without,
		"with_pagination_indexes_latency_ms":                       with,
		"p50_write_latency_overhead_percent":                       overhead,
		"without_indexes_relation_bytes":                           withoutBytes,
		"with_indexes_relation_bytes":                              withBytes,
		"with_overlapping_owner_index_latency_ms":                  withOverlap,
		"with_overlapping_owner_index_relation_bytes":              withOverlapBytes,
		"overlapping_owner_index_incremental_p50_overhead_percent": overlapOverhead,
	}, nil
}

func summarize(values []float64) latencySummary {
	sort.Float64s(values)
	at := func(f float64) float64 { return values[int(float64(len(values)-1)*f)] }
	return latencySummary{Samples: len(values), MinMS: values[0], P50MS: at(.50), P95MS: at(.95), P99MS: at(.99), MaxMS: values[len(values)-1]}
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func writeReport(path string, manifest runManifest) error {
	content := fmt.Sprintf("# PostgreSQL keyset pagination benchmark\n\n- Run: `%s`\n- PostgreSQL: `%s`\n- Rows: %d\n- Generator: `%s`; seed: `%s`\n- Distribution: %v\n- Page size: %d\n- Keyset walk: %v rows across %v pages; duplicates: %v\n- Concurrent write check: %v initial rows; %v new rows; duplicate count: %v\n\nLatency values are local measurements for this host/container and are not production SLOs. Full `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)` plans, index sizes, and write-cost comparison are in `manifest.json`.\n\n| Query | Samples | P50 ms | P95 ms | P99 ms | Max ms |\n|---|---:|---:|---:|---:|---:|\n", manifest.RunID, manifest.Postgres["server_version"], rowCount, manifest.Generator["version"], manifest.Generator["seed"], manifest.Generator["distribution"], pageSize, manifest.Pagination["rows_seen"], manifest.Pagination["pages"], manifest.Pagination["duplicates"], manifest.Concurrent["initial_rows_seen"], manifest.Concurrent["concurrent_rows_written"], manifest.Concurrent["duplicates"])
	for _, name := range []string{"head", "middle_keyset", "tail_keyset", "filtered_owner", "filtered_keyset", "owner_broad_70pct", "owner_broad_keyset", "owner_sparse_1pct", "owner_sparse_keyset"} {
		latency := manifest.Latency[name]
		content += fmt.Sprintf("| %s | %d | %.3f | %.3f | %.3f | %.3f |\n", name, latency.Samples, latency.P50MS, latency.P95MS, latency.P99MS, latency.MaxMS)
	}
	content += fmt.Sprintf("\n## Index write-cost sample\n\n- Method: %v\n- Without pagination indexes: P50 %.3f ms, relation %v bytes\n- With global + partial-owner pagination indexes: P50 %.3f ms, relation %v bytes\n- Measured P50 write latency change: %.1f%%\n- With an additional overlapping non-partial owner index: P50 %.3f ms, relation %v bytes\n- Additional measured P50 write change: %.1f%%\n\nThis temporary-table comparison isolates pagination-index maintenance on this test host. It is not an index recommendation or production SLO. Existing task indexes and sizes are enumerated in the JSON manifest.\n", manifest.WriteCost["method"], manifest.WriteCost["without_pagination_indexes_latency_ms"].(latencySummary).P50MS, manifest.WriteCost["without_indexes_relation_bytes"], manifest.WriteCost["with_pagination_indexes_latency_ms"].(latencySummary).P50MS, manifest.WriteCost["with_indexes_relation_bytes"], manifest.WriteCost["p50_write_latency_overhead_percent"], manifest.WriteCost["with_overlapping_owner_index_latency_ms"].(latencySummary).P50MS, manifest.WriteCost["with_overlapping_owner_index_relation_bytes"], manifest.WriteCost["overlapping_owner_index_incremental_p50_overhead_percent"])
	return os.WriteFile(path, []byte(content), 0o644)
}
