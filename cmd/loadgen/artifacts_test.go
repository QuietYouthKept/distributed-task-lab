package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
)

func TestGitEvidenceDistinguishesCleanDirtyAndUnknown(t *testing.T) {
	repo := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}

	runGit("init", "--quiet")
	runGit("config", "user.name", "Loadgen Evidence Test")
	runGit("config", "user.email", "loadgen-evidence@example.invalid")
	tracked := filepath.Join(repo, "source.txt")
	if err := os.WriteFile(tracked, []byte("baseline\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit("add", "source.txt")
	runGit("commit", "--quiet", "-m", "baseline")

	clean := gitEvidence(repo)
	if clean.Commit == "unknown" || clean.Tree == "unknown" || clean.Dirty == nil || *clean.Dirty {
		t.Fatalf("clean repository metadata = %+v", clean)
	}
	if err := os.WriteFile(tracked, []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dirty := gitEvidence(repo)
	if dirty.Commit != clean.Commit || dirty.Tree != clean.Tree || dirty.Dirty == nil || !*dirty.Dirty {
		t.Fatalf("dirty repository metadata = %+v, clean=%+v", dirty, clean)
	}
	unknown := gitEvidence(t.TempDir())
	if unknown.Commit != "unknown" || unknown.Tree != "unknown" || unknown.Dirty != nil {
		t.Fatalf("non-repository metadata = %+v", unknown)
	}
}

func TestLoadgenCountsOnlyTerminalTasksAndUsesWholeRunDuration(t *testing.T) {
	stats := map[domain.TaskStatus]int64{
		domain.TaskStatusPending: 3,
		domain.TaskStatusRunning: 2,
		domain.TaskStatusSuccess: 7,
		domain.TaskStatusFailed:  1,
		domain.TaskStatusDLQ:     2,
	}
	if got := terminalTaskCount(stats); got != 10 {
		t.Fatalf("terminalTaskCount() = %d, want 10", got)
	}
	if got := throughput(10, 5*time.Second); got != 2 {
		t.Fatalf("throughput() = %v, want 2 tasks/s", got)
	}
	if got := throughput(10, 0); got != 0 {
		t.Fatalf("throughput with non-positive elapsed time = %v, want 0", got)
	}
	if got := throughput(0, time.Second); got != 0 {
		t.Fatalf("throughput with no terminal tasks = %v, want 0", got)
	}
}
