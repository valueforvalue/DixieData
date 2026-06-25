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

func TestOpenJobsRegistryRehydratesFromExistingLog(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, jobsLogFilename)
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
	dir := t.TempDir()
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
	dir := t.TempDir()
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

	logPath := filepath.Join(dir, jobsLogFilename)
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