package appshell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPruneFeedbackLogKeepsRecentEntries(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "logs", "feedback-log.jsonl")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatalf("mkdir logs: %v", err)
	}

	now := time.Now().UTC()
	lines := []string{
		entryLine(now.Add(-10*24*time.Hour), "recent"),
		entryLine(now.Add(-100*24*time.Hour), "old-but-kept-with-365-day-window"),
		entryLine(now.Add(-400*24*time.Hour), "should-be-pruned"),
	}
	if err := os.WriteFile(logPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	pruneFeedbackLogAtPath(logPath, 365)

	got, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read after prune: %v", err)
	}
	body := string(got)
	if !strings.Contains(body, "recent") {
		t.Fatalf("recent entry should be kept; got:\n%s", body)
	}
	if !strings.Contains(body, "old-but-kept") {
		t.Fatalf("100-day-old entry should be kept under 365-day window; got:\n%s", body)
	}
	if strings.Contains(body, "should-be-pruned") {
		t.Fatalf("400-day-old entry should be pruned; got:\n%s", body)
	}
}

func TestPruneFeedbackLogZeroDaysIsNoop(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "logs", "feedback-log.jsonl")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatalf("mkdir logs: %v", err)
	}
	if err := os.WriteFile(logPath, []byte(entryLine(time.Now().UTC(), "keep")+"\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	pruneFeedbackLogAtPath(logPath, 0)
	got, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(got), "keep") {
		t.Fatalf("retention=0 should keep every entry; got:\n%s", got)
	}
}

func TestPruneFeedbackLogMissingFileIsNoop(t *testing.T) {
	pruneFeedbackLogAtPath(filepath.Join(t.TempDir(), "missing.jsonl"), 365)
}

func entryLine(t time.Time, marker string) string {
	return `{"submitted_at":"` + t.Format(time.RFC3339) + `","message":"` + marker + `","app_version":"v1.2.54","build_identity":"dev","schema_version":54}`
}