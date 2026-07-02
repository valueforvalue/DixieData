package appshell

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/valueforvalue/DixieData/internal/jobs"
)

// testDataDir returns a t.TempDir() with the sibling .dixiedata-logs/
// directory pre-created, matching the convention openJobsRegistry
// expects. dataDir passed to openJobsRegistry resolves its log file
// at <parent>/.dixiedata-logs/jobs.jsonl, so the parent must exist
// and be writable.
func testDataDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(filepath.Dir(dir), ".dixiedata-logs"), 0o755); err != nil {
		t.Fatalf("setup logs dir: %v", err)
	}
	return dir
}

func TestOpenJobsRegistryRehydratesFromExistingLog(t *testing.T) {
	dir := testDataDir(t)
	logPath := jobsLogPath(dir)
	// Seed a JSONL log with one done job and one running job.
	now := time.Now().UTC()
	seed := `{"id":"done1","kind":"static_archive","status":"done","progress":100,"started_at":"` + now.Format(time.RFC3339Nano) + `","finished_at":"` + now.Format(time.RFC3339Nano) + `","result_path":"/tmp/a.zip"}
{"id":"run1","kind":"database_pdf","status":"running","progress":40,"started_at":"` + now.Format(time.RFC3339Nano) + `"}
`
	if err := os.WriteFile(logPath, []byte(seed), 0o644); err != nil {
		t.Fatalf("seed log: %v", err)
	}

	reg := openJobsRegistry(dir)
	t.Cleanup(func() { reg.SetLogWriter(nil, nil) })

	doneSnap, ok := reg.Get("done1")
	if !ok {
		t.Fatalf("rehydrate lost done job")
	}
	if doneSnap.Status != jobs.StatusDone || doneSnap.ResultPath != "/tmp/a.zip" {
		t.Fatalf("done snap = %+v", doneSnap)
	}

	runSnap, ok := reg.Get("run1")
	if !ok {
		t.Fatalf("rehydrate lost running job")
	}
	if runSnap.Status != jobs.StatusInterrupted {
		t.Fatalf("running snap status = %s, want interrupted", runSnap.Status)
	}
}

func TestOpenJobsRegistryStartsEmptyWhenLogMissing(t *testing.T) {
	dir := testDataDir(t)
	reg := openJobsRegistry(dir)
	t.Cleanup(func() { reg.SetLogWriter(nil, nil) })
	if reg.Concurrency() < 1 {
		t.Fatalf("registry concurrency = %d, want >= 1", reg.Concurrency())
	}
	// Confirm no jobs from a previous run are loaded.
	if _, ok := reg.Get("does-not-exist"); ok {
		t.Fatalf("registry unexpectedly knows about a job")
	}
}

func TestOpenJobsRegistryWritesStateChangesBackToLog(t *testing.T) {
	dir := testDataDir(t)
	reg := openJobsRegistry(dir)
	t.Cleanup(func() { reg.SetLogWriter(nil, nil) })

	var id string
	id = reg.Start("unit", func(ctx context.Context, p *jobs.Progress) error {
		p.Set(100, "Done")
		reg.SetResultPath(id, "/tmp/out.zip")
		return nil
	})

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snap, _ := reg.Get(id)
		if snap.Status == jobs.StatusDone {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	logPath := jobsLogPath(dir)
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(raw), `"id":"`+id+`"`) {
		t.Fatalf("log should contain the job id; got:\n%s", raw)
	}
	// Each line is a valid JSON object.
	for _, line := range bytes.Split(bytes.TrimSpace(raw), []byte{'\n'}) {
		var payload map[string]interface{}
		if err := json.Unmarshal(line, &payload); err != nil {
			t.Fatalf("malformed log line %q: %v", line, err)
		}
	}
}

// TestMigrateLegacyJobsLogRenamesOldFile covers the upgrade path: an
// existing install has <dataDir>/jobs.jsonl (legacy path). On the
// first openJobsRegistry after the move-to-LogsDir change, the legacy
// file must be relocated to <parent>/.dixiedata-logs/jobs.jsonl so
// the open file handle no longer lives inside the data dir. This is
// the fix for the "rename .dixiedata failed after 5 attempts" bug
// where DixieData.exe itself held the handle on jobs.jsonl and
// blocked the atomic rename performed by replaceDataDir.
func TestMigrateLegacyJobsLogRenamesOldFile(t *testing.T) {
	dir := testDataDir(t)
	legacyPath := filepath.Join(dir, jobsLogFilename)
	seed := `{"id":"legacy1","kind":"unit","status":"done","progress":100,"started_at":"2026-01-01T00:00:00Z"}` + "\n"
	if err := os.WriteFile(legacyPath, []byte(seed), 0o644); err != nil {
		t.Fatalf("seed legacy: %v", err)
	}

	reg := openJobsRegistry(dir)
	t.Cleanup(func() { reg.SetLogWriter(nil, nil) })

	// Legacy file must be gone.
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy file should be moved, stat err = %v", err)
	}
	// New path must hold the same content.
	newPath := jobsLogPath(dir)
	raw, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("read new path: %v", err)
	}
	if !strings.Contains(string(raw), `"id":"legacy1"`) {
		t.Fatalf("migrated log lost seed content; got:\n%s", raw)
	}
	// Registry rehydrated from the migrated file.
	if _, ok := reg.Get("legacy1"); !ok {
		t.Fatalf("registry should rehydrate from migrated log")
	}
}

// TestMigrateLegacyJobsLogIsIdempotent ensures repeated calls
// (re-opens, restarts) are no-ops once the migration has run.
func TestMigrateLegacyJobsLogIsIdempotent(t *testing.T) {
	dir := testDataDir(t)
	legacyPath := filepath.Join(dir, jobsLogFilename)
	if err := os.WriteFile(legacyPath, []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("seed legacy: %v", err)
	}
	if err := migrateLegacyJobsLog(dir); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy file should be gone after first migrate")
	}
	// Second call: legacy is gone, new path exists. Must be a no-op
	// (no error, no destructive changes).
	if err := migrateLegacyJobsLog(dir); err != nil {
		t.Fatalf("second migrate should be no-op, got: %v", err)
	}
	if _, err := os.Stat(jobsLogPath(dir)); err != nil {
		t.Fatalf("new path should still exist: %v", err)
	}
}

// TestOpenJobsRegistryLogPathOutsideDataDir is the regression net
// for the rename-during-import bug. The jobs log must live outside
// the data directory so replaceDataDir's atomic rename
// (.dixiedata → .dixiedata-previous-*) doesn't fail with "Access is
// denied" when DixieData.exe holds the log file open.
func TestOpenJobsRegistryLogPathOutsideDataDir(t *testing.T) {
	dir := testDataDir(t)
	logPath := jobsLogPath(dir)
	logDir := filepath.Dir(logPath)
	if filepath.Dir(logDir) == dir {
		t.Fatalf("jobs log must NOT live inside data dir; logDir=%s, dataDir=%s", logDir, dir)
	}
	if !strings.Contains(logDir, ".dixiedata-logs") {
		t.Fatalf("jobs log must live under .dixiedata-logs/, got %s", logDir)
	}
	// Run openJobsRegistry and confirm it doesn't put any file
	// inside the data dir beyond the dir itself.
	reg := openJobsRegistry(dir)
	t.Cleanup(func() { reg.SetLogWriter(nil, nil) })
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read data dir: %v", err)
	}
	for _, e := range entries {
		if e.Name() == jobsLogFilename {
			t.Fatalf("jobs.jsonl leaked back into data dir: %s", filepath.Join(dir, e.Name()))
		}
	}
}