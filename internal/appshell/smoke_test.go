package appshell

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRunSmokeAllPass boots a real App against a fresh temp dir
// and asserts every smoke check passes. This is the integration
// coverage for the smoke command itself.
func TestRunSmokeAllPass(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), ".dixiedata")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("mkdir dataDir: %v", err)
	}

	var jsonBuf strings.Builder
	res, code := RunSmoke(context.Background(), SmokeOptions{
		JSON:    true,
		DataDir: dataDir,
		Writer:  &jsonBuf,
		Now:     func() time.Time { return time.Date(2026, 6, 28, 0, 0, 0, 0, time.UTC) },
	})

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
		t.Logf("output:\n%s", jsonBuf.String())
	}
	if res.Exit != 0 {
		t.Errorf("result.Exit = %d, want 0", res.Exit)
	}
	if res.Command != "smoke" {
		t.Errorf("result.Command = %q, want %q", res.Command, "smoke")
	}
	if res.StartedAt != "2026-06-28T00:00:00Z" {
		t.Errorf("result.StartedAt = %q, want %q", res.StartedAt, "2026-06-28T00:00:00Z")
	}
	if got := len(res.Checks); got != 8 {
		t.Errorf("len(Checks) = %d, want 8", got)
	}
	for _, c := range res.Checks {
		if !c.Passed && !c.Optional {
			t.Errorf("hard check %q failed: %s", c.Name, c.Error)
		}
	}

	// JSON envelope must be parseable.
	lines := strings.Split(strings.TrimSpace(jsonBuf.String()), "\n")
	if len(lines) < 9 {
		// 8 per-check lines + 1 envelope
		t.Fatalf("expected >=9 JSON lines, got %d", len(lines))
	}
	var envelope SmokeResult
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &envelope); err != nil {
		t.Fatalf("parse envelope: %v", err)
	}
	if envelope.Exit != 0 {
		t.Errorf("envelope.Exit = %d, want 0", envelope.Exit)
	}
}

// TestHasSmokeFlag exercises the CLI flag detector. Kept here
// so a future refactor of main.go's dispatch logic has a clear
// place to update the contract.
func TestHasSmokeFlag(t *testing.T) {
	cases := []struct {
		args []string
		want bool
	}{
		{[]string{}, false},
		{[]string{"--smoke"}, true},
		{[]string{"--smoke-json"}, true},
		{[]string{"--help"}, false},
		{[]string{"some", "random", "args"}, false},
		{nil, false},
	}
	for _, tc := range cases {
		if got := HasSmokeFlag(tc.args); got != tc.want {
			t.Errorf("HasSmokeFlag(%v) = %v, want %v", tc.args, got, tc.want)
		}
	}
}

func TestWantsSmokeJSON(t *testing.T) {
	if !WantsSmokeJSON([]string{"--smoke-json"}) {
		t.Error("--smoke-json must request JSON")
	}
	if WantsSmokeJSON([]string{"--smoke"}) {
		t.Error("--smoke alone must not request JSON")
	}
	if WantsSmokeJSON(nil) {
		t.Error("no args must not request JSON")
	}
}

func TestEnvRequestsSmoke(t *testing.T) {
	t.Setenv("DIXIEDATA_SMOKE", "1")
	if !EnvRequestsSmoke() {
		t.Error("DIXIEDATA_SMOKE=1 should request smoke")
	}
	t.Setenv("DIXIEDATA_SMOKE", "")
	if EnvRequestsSmoke() {
		t.Error("empty DIXIEDATA_SMOKE should not request smoke")
	}
	t.Setenv("DIXIEDATA_SMOKE", "true")
	if !EnvRequestsSmoke() {
		t.Error("DIXIEDATA_SMOKE=true (case-insensitive) should request smoke")
	}
}

// TestSamePath sanity-checks the helper used by logs_dir_separate.
func TestSamePath(t *testing.T) {
	if !samePath(`C:\foo\bar`, `C:\foo\bar`) {
		t.Error("identical paths should match")
	}
	if samePath(`C:\foo\bar`, `C:\foo\baz`) {
		t.Error("different paths should not match")
	}
}