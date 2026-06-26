package appshell

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/valueforvalue/DixieData/internal/appdata"
)

// TestMigrateLogsToSiblingDir is the regression test for the
// "every .ddbak restore fails on Windows" bug. Before the
// appdata layout split, app logs lived at <dataDir>/logs/, which
// forced the .ddbak restore code path (replaceDataDir → os.Rename)
// to release the open log file handle on Windows. Every restore
// attempt failed with "Access is denied" while the log file was
// still open. The split moves logs to <parent>/.dixiedata-logs/ so
// the rename never touches them.
//
// This test verifies the one-time migration that runs at startup
// when a user upgrades from the old layout to the new one:
//   - if <dataDir>/logs/ exists, its contents are moved to
//     <parent>/.dixiedata-logs/
//   - if the old dir is empty after the move, it is removed
//   - if the new location already has files, only the missing
//     stragglers are moved (no overwrite)
//   - if neither old nor new exists, the helper returns 0 moved
func TestMigrateLogsToSiblingDir(t *testing.T) {
	dir := t.TempDir()
	// The migration helper treats dataDir as the inner data folder
	// (the one being renamed on restore). The parent is where
	// .dixiedata-logs/ lives.
	dataDir := filepath.Join(dir, ".dixiedata")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("mkdir dataDir: %v", err)
	}
	oldLogs := filepath.Join(dataDir, "logs")
	if err := os.MkdirAll(oldLogs, 0o755); err != nil {
		t.Fatalf("mkdir old logs: %v", err)
	}

	// Seed the old layout with two files that match the production
	// layout: app.log.jsonl + feedback-log.jsonl.
	appLogPath := filepath.Join(oldLogs, "app.log.jsonl")
	if err := os.WriteFile(appLogPath, []byte(`{"level":"INFO","msg":"seed"}`), 0o644); err != nil {
		t.Fatalf("seed app log: %v", err)
	}
	feedbackPath := filepath.Join(oldLogs, "feedback-log.jsonl")
	if err := os.WriteFile(feedbackPath, []byte(`{"message":"seed"}`), 0o644); err != nil {
		t.Fatalf("seed feedback log: %v", err)
	}

	moved, err := migrateLogsToSiblingDir(dataDir)
	if err != nil {
		t.Fatalf("migrateLogsToSiblingDir: %v", err)
	}
	if moved != 2 {
		t.Fatalf("expected 2 files moved, got %d", moved)
	}

	// New location: <parent>/.dixiedata-logs/
	newLogs := appdata.LogsRoot(dataDir)
	for _, name := range []string{"app.log.jsonl", "feedback-log.jsonl"} {
		if _, err := os.Stat(filepath.Join(newLogs, name)); err != nil {
			t.Errorf("expected %q in new location: %v", name, err)
		}
	}

	// Old location should be cleaned up if empty.
	if _, err := os.Stat(oldLogs); err == nil {
		t.Errorf("expected old logs dir to be removed when empty")
	} else if !os.IsNotExist(err) {
		t.Errorf("unexpected stat error on old logs dir: %v", err)
	}

	// Idempotent: re-running on the migrated state is a no-op.
	moved2, err := migrateLogsToSiblingDir(dataDir)
	if err != nil {
		t.Fatalf("re-run migrate: %v", err)
	}
	if moved2 != 0 {
		t.Errorf("expected 0 moves on second run, got %d", moved2)
	}
}

func TestMigrateLogsToSiblingDir_FreshInstall(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, ".dixiedata")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("mkdir dataDir: %v", err)
	}
	// No old logs dir exists. The helper should return 0 without
	// touching anything.
	moved, err := migrateLogsToSiblingDir(dataDir)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if moved != 0 {
		t.Errorf("expected 0 moves on fresh install, got %d", moved)
	}
}

func TestMigrateLogsToSiblingDir_PartialMigration(t *testing.T) {
	// Simulate a half-migrated state: old dir has one file, new
	// dir already has a different one. The helper should move only
	// the missing file from old to new without overwriting.
	dir := t.TempDir()
	dataDir := filepath.Join(dir, ".dixiedata")
	oldLogs := filepath.Join(dataDir, "logs")
	newLogs := appdata.LogsRoot(dataDir)
	if err := os.MkdirAll(oldLogs, 0o755); err != nil {
		t.Fatalf("mkdir old logs: %v", err)
	}
	if err := os.MkdirAll(newLogs, 0o755); err != nil {
		t.Fatalf("mkdir new logs: %v", err)
	}

	if err := os.WriteFile(filepath.Join(newLogs, "app.log.jsonl"), []byte("new"), 0o644); err != nil {
		t.Fatalf("seed new app log: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldLogs, "feedback-log.jsonl"), []byte("old"), 0o644); err != nil {
		t.Fatalf("seed old feedback log: %v", err)
	}

	moved, err := migrateLogsToSiblingDir(dataDir)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if moved != 1 {
		t.Errorf("expected 1 move (only the missing file), got %d", moved)
	}

	// Both files should now be in the new location, with the new
	// app.log.jsonl untouched.
	newAppLog, err := os.ReadFile(filepath.Join(newLogs, "app.log.jsonl"))
	if err != nil {
		t.Fatalf("read new app log: %v", err)
	}
	if string(newAppLog) != "new" {
		t.Errorf("new app log was overwritten: got %q", string(newAppLog))
	}
	newFeedbackLog, err := os.ReadFile(filepath.Join(newLogs, "feedback-log.jsonl"))
	if err != nil {
		t.Fatalf("read new feedback log: %v", err)
	}
	if string(newFeedbackLog) != "old" {
		t.Errorf("feedback log content not preserved: got %q", string(newFeedbackLog))
	}
}