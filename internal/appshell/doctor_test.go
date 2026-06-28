package appshell

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCheckFeedbackLogOpen_AllValid seeds a JSONL with only
// valid lines and asserts the check returns nil. Mirrors the
// real export path's success state.
func TestCheckFeedbackLogOpen_AllValid(t *testing.T) {
	dataDir := t.TempDir()
	logsDir := filepath.Join(filepath.Dir(dataDir), ".dixiedata-logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatalf("mkdir logs dir: %v", err)
	}
	logPath := filepath.Join(logsDir, "feedback-log.jsonl")
	content := "{\"submitted_at\":\"2026-06-27T10:00:00Z\",\"category\":\"bug\",\"message\":\"x\"}\n" +
		"{\"submitted_at\":\"2026-06-27T10:01:00Z\",\"category\":\"feature\",\"message\":\"y\"}\n"
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	a := &App{dataDir: dataDir}
	if err := checkFeedbackLogOpen(context.Background(), a); err != nil {
		t.Errorf("all-valid log should pass, got: %v", err)
	}
}

// TestCheckFeedbackLogOpen_CorruptLines seeds a JSONL with one
// corrupt line and asserts the check returns an error that
// identifies the corrupt line.
func TestCheckFeedbackLogOpen_CorruptLines(t *testing.T) {
	dataDir := t.TempDir()
	logsDir := filepath.Join(filepath.Dir(dataDir), ".dixiedata-logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatalf("mkdir logs dir: %v", err)
	}
	logPath := filepath.Join(logsDir, "feedback-log.jsonl")
	content := "{\"valid\":\"line\"}\nthis is not json\n{\"another\":\"valid\"}\n"
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	a := &App{dataDir: dataDir}
	err := checkFeedbackLogOpen(context.Background(), a)
	if err == nil {
		t.Fatal("corrupt log should fail check")
	}
	if !strings.Contains(err.Error(), "1 corrupt") {
		t.Errorf("error should report corrupt count, got: %v", err)
	}
	if !strings.Contains(err.Error(), "line 2") {
		t.Errorf("error should identify line 2, got: %v", err)
	}
}

// TestCheckFeedbackLogOpen_MissingLog asserts the check passes
// when there's no log file yet. (No log = nothing to corrupt.)
func TestCheckFeedbackLogOpen_MissingLog(t *testing.T) {
	dataDir := t.TempDir()
	a := &App{dataDir: dataDir}
	if err := checkFeedbackLogOpen(context.Background(), a); err != nil {
		t.Errorf("missing log should pass, got: %v", err)
	}
}

// TestFilterChecks covers the --check= flag matcher semantics.
func TestFilterChecks(t *testing.T) {
	all := []smokeCheckDef{
		{name: "data_dir_resolves"},
		{name: "logs_dir_separate"},
		{name: "sqlite_open"},
		{name: "migrations_applied"},
		{name: "templates_dir"},
		{name: "templates_parseable"},
		{name: "archive_writable"},
	}

	cases := []struct {
		name  string
		wanted []string
		want  []string
	}{
		{"empty wanted = all", nil, []string{
			"data_dir_resolves", "logs_dir_separate", "sqlite_open",
			"migrations_applied", "templates_dir", "templates_parseable",
			"archive_writable",
		}},
		{"exact match", []string{"sqlite_open"}, []string{"sqlite_open"}},
		{"stem prefix matches child", []string{"data_dir"}, []string{"data_dir_resolves"}},
		{"stem prefix matches multiple", []string{"templates"}, []string{
			"templates_dir", "templates_parseable",
		}},
		{"no match returns empty", []string{"doesnotexist"}, []string{}},
		{"multiple wanted", []string{"data_dir", "archive"}, []string{
			"data_dir_resolves", "archive_writable",
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := filterChecks(all, tc.wanted)
			gotNames := make([]string, len(got))
			for i, c := range got {
				gotNames[i] = c.name
			}
			if len(gotNames) != len(tc.want) {
				t.Fatalf("got %d checks (%v), want %d (%v)", len(gotNames), gotNames, len(tc.want), tc.want)
			}
			for i, want := range tc.want {
				if gotNames[i] != want {
					t.Errorf("[%d] got %q, want %q", i, gotNames[i], want)
				}
			}
		})
	}
}

// TestIsParseError verifies the typst parse-error classifier.
func TestIsParseError(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   bool
	}{
		{"empty output", "", false},
		{"clean compile", "compiled successfully", false},
		{"parse error expected", "error: expected expression\n  at line 1", true},
		{"parse error unexpected", "error: unexpected token", true},
		{"runtime error: type none", "error: type none has no method `at`", false},
		{"runtime error: cannot calculate", "error: cannot calculate sum of empty array", false},
		{"mismatched bracket", "error: mismatched closing brace", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isParseError(tc.output); got != tc.want {
				t.Errorf("isParseError(%q) = %v, want %v", truncate(tc.output, 40), got, tc.want)
			}
		})
	}
}

// TestFixTruncateFeedbackLog proves the --fix mode actually
// rewrites the log and preserves the good lines.
func TestFixTruncateFeedbackLog(t *testing.T) {
	dataDir := t.TempDir()
	logsDir := filepath.Join(filepath.Dir(dataDir), ".dixiedata-logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatalf("mkdir logs dir: %v", err)
	}
	logPath := filepath.Join(logsDir, "feedback-log.jsonl")
	original := "{\"good\":1}\nbad line 1\n{\"good\":2}\nbad line 2\n{\"good\":3}\n"
	if err := os.WriteFile(logPath, []byte(original), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	a := &App{dataDir: dataDir}
	detail, err := fixTruncateFeedbackLog(context.Background(), a)
	if err != nil {
		t.Fatalf("fix: %v", err)
	}
	if !strings.Contains(detail, "truncated 2") {
		t.Errorf("expected 'truncated 2' in detail, got %q", detail)
	}

	// Read back: only the 3 good lines should remain.
	got, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(got)), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines after fix, got %d:\n%s", len(lines), string(got))
	}
	for _, line := range lines {
		if !json.Valid([]byte(line)) {
			t.Errorf("post-fix line should be valid JSON: %q", line)
		}
	}

	// Backup should exist.
	matches, err := filepath.Glob(logPath + ".bak-*")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) != 1 {
		t.Errorf("expected 1 backup file, got %d", len(matches))
	}
}

// TestHasDoctorFlag / TestWantsDoctorFix / TestParseDoctorChecks /
// TestWantsDoctorJSON exercise the CLI flag detectors.
func TestHasDoctorFlag(t *testing.T) {
	if !HasDoctorFlag([]string{"doctor"}) {
		t.Error("'doctor' must be detected")
	}
	if HasDoctorFlag([]string{"--doctor"}) {
		t.Error("'--doctor' must NOT match (subcommand, not flag)")
	}
	if HasDoctorFlag(nil) {
		t.Error("nil args must not match")
	}
}

func TestWantsDoctorFix(t *testing.T) {
	if !WantsDoctorFix([]string{"doctor", "--fix"}) {
		t.Error("--fix must be detected")
	}
	if WantsDoctorFix([]string{"doctor"}) {
		t.Error("plain doctor must not request fix")
	}
}

func TestParseDoctorChecks(t *testing.T) {
	got := ParseDoctorChecks([]string{"doctor", "--check=data_dir", "--check=sqlite", "--fix"})
	want := []string{"data_dir", "sqlite"}
	if len(got) != len(want) {
		t.Fatalf("got %d checks, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestWantsDoctorJSON(t *testing.T) {
	if !WantsDoctorJSON([]string{"doctor", "--json"}) {
		t.Error("--json must be detected")
	}
	if WantsDoctorJSON([]string{"doctor", "--smoke-json"}) {
		t.Error("--smoke-json must not match doctor --json")
	}
	if WantsDoctorJSON(nil) {
		t.Error("nil args must not match")
	}
}