// Command retention-dry-run inventories a conservative task retention set.
// It is read-only by construction and never deletes data.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/storage/postgres"
)

type report struct {
	Mode              string                        `json:"mode"`
	GeneratedAt       time.Time                     `json:"generated_at_utc"`
	FinishedBefore    time.Time                     `json:"finished_before_utc"`
	Limit             int                           `json:"limit"`
	CandidateCount    int                           `json:"candidate_count"`
	Candidates        []postgres.RetentionCandidate `json:"candidates"`
	DeletionPerformed bool                          `json:"deletion_performed"`
	OperatorWarnings  []string                      `json:"operator_warnings"`
}

func run() error {
	if os.Getenv("RETENTION_DRY_RUN") != "true" {
		return errors.New("RETENTION_DRY_RUN=true is required; this command never deletes")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL is required")
	}
	cutoffArg := flag.String("finished-before", "", "UTC RFC3339 cutoff for terminal task finish time (required)")
	limit := flag.Int("limit", 100, "maximum candidates to list (1..1000)")
	flag.Parse()
	if *cutoffArg == "" {
		return errors.New("-finished-before is required")
	}
	cutoff, err := time.Parse(time.RFC3339, *cutoffArg)
	if err != nil {
		return fmt.Errorf("parse -finished-before: %w", err)
	}
	db, err := postgres.NewDB(dsn)
	if err != nil {
		return fmt.Errorf("open PostgreSQL: %w", err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
	defer cancel()
	candidates, err := postgres.ListRetentionCandidates(ctx, db, cutoff, *limit)
	if err != nil {
		return err
	}
	result := report{Mode: "read-only-dry-run", GeneratedAt: time.Now().UTC(), FinishedBefore: cutoff.UTC(), Limit: *limit,
		CandidateCount: len(candidates), Candidates: candidates, DeletionPerformed: false,
		OperatorWarnings: []string{
			"No records were deleted or updated.",
			"task_events are ON DELETE CASCADE; task history is not immutable archival storage.",
			"Artifact references are not normalized; manually verify payloads, results, and object-store objects before any separately approved purge.",
			"Effects and security-audit references are excluded from this conservative candidate query.",
		}}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "retention-dry-run:", err)
		os.Exit(1)
	}
}
