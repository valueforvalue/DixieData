// RED-first regression net for issue #371 Slice 1: the
// `dixiedata soldier create --from <json> | --from-stdin`
// subcommand. Per the body (Phase 8 of cli-plan.md), the CLI
// dispatches to existing *App.soldiers.Create; we don't
// duplicate handler logic.
//
// This slice is intentionally narrow: just `soldier create`.
// `update`, `delete`, `tags list`, and event-side verbs land
// in follow-up slices per the tracer-bullet rule.
package appshell

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"unsafe"

	"github.com/valueforvalue/DixieData/internal/appdata"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/seed"
)

func TestParseMutateCommand_SoldierCreate(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		wantOK    bool
		wantCmd   MutateCommand
		wantStdin bool
	}{
		{
			name:    "soldier create --from <path>",
			args:    []string{"soldier", "create", "--from", "/tmp/soldier.json"},
			wantOK:  true,
			wantCmd: MutateSoldierCreate,
		},
		{
			name:      "soldier create --from-stdin",
			args:      []string{"soldier", "create", "--from-stdin"},
			wantOK:    true,
			wantCmd:   MutateSoldierCreate,
			wantStdin: true,
		},
		{
			name:    "unknown verb is rejected",
			args:    []string{"soldier", "frobnicate"},
			wantOK:  false,
		},
		{
			name:    "missing --from / --from-stdin",
			args:    []string{"soldier", "create"},
			wantOK:  false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts, ok := ParseMutateCommand(tc.args)
			if ok != tc.wantOK {
				t.Fatalf("ParseMutateCommand ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if opts.Command != tc.wantCmd {
				t.Errorf("Command = %v, want %v", opts.Command, tc.wantCmd)
			}
			if opts.FromStdin != tc.wantStdin {
				t.Errorf("FromStdin = %v, want %v", opts.FromStdin, tc.wantStdin)
			}
		})
	}
}

// TestRunMutateSoldierCreate is the integration test. Seeds a
// fresh archive, runs `soldier create` against it via RunMutate,
// and asserts the new row is in the DB + the JSON echo includes
// the new id + display_id.
func TestRunMutateSoldierCreate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode (seeds archive + starts App)")
	}
	// Set up a fresh scratch archive. Use os.MkdirTemp +
	// explicit RemoveAll in t.Cleanup because t.TempDir cleanup
	// races with the jobs log file handle on Windows (the
	// JSONL file is held open by the jobs.Registry's
	// SetLogWriter; App.Shutdown does not close it). See
	// cli_debug_test.go::newDebugTestApp for the same pattern.
	dir, err := os.MkdirTemp("", "dixiedata-mutate-create-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Setenv("DIXIEDATA_DATA_DIR", dir)
	_ = appdata.DefaultDir // pin import

	// Seed via the seed package.
	if _, err := seed.Generate(seed.Options{
		DataDir:  dir,
		Soldiers: 0,
		Seed:     1865,
		Reset:    true,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Build the input JSON for a new soldier.
	input := models.Soldier{
		FirstName: "TestFirst",
		LastName:  "TestLast",
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}

	// Write the input to a temp file (so we exercise the
	// --from path, not the --from-stdin path).
	inputPath := dir + "/input.json"
	if err := writeTestFile(inputPath, inputJSON); err != nil {
		t.Fatalf("write input: %v", err)
	}

	a := NewApp()
	ctx := context.Background()
	a.Startup(ctx)
	t.Cleanup(func() {
		closeJobsLogWriterTest(a)
		a.Shutdown(ctx)
		_ = os.RemoveAll(dir)
	})

	var buf bytes.Buffer
	opts := MutateOptions{
		Command: MutateSoldierCreate,
		From:    inputPath,
		JSON:    true,
		Writer:  &buf,
		App:     a,
	}
	code, err := RunMutate(ctx, opts)
	if err != nil {
		t.Fatalf("RunMutate: %v (code=%d)", err, code)
	}
	if code != 0 {
		t.Fatalf("RunMutate exit code = %d, want 0; output:\n%s", code, buf.String())
	}

	// JSON envelope echo per the locked decision (4): the
	// response carries the new row's id + display_id so a
	// script can chain `... | jq -r .id | xargs ... show`.
	var resp struct {
		ID        int64  `json:"id"`
		DisplayID string `json:"display_id"`
	}
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v\noutput: %s", err, buf.String())
	}
	if resp.ID == 0 {
		t.Errorf("response missing id:\n%s", buf.String())
	}
	if !strings.HasPrefix(resp.DisplayID, "DXD-") {
		t.Errorf("response display_id = %q; expected DXD-* prefix", resp.DisplayID)
	}

	// Confirm the row is in the DB.
	got, err := a.soldiers.GetByID(resp.ID)
	if err != nil {
		t.Fatalf("GetByID(%d): %v", resp.ID, err)
	}
	if got.FirstName != "TestFirst" || got.LastName != "TestLast" {
		t.Errorf("created row mismatch: first=%q last=%q", got.FirstName, got.LastName)
	}
}

func writeTestFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}

// RED tests for issue #371 Slice 2: `soldier update` + `soldier
// delete`. Per the body (Phase 8 of cli-plan.md), the CLI
// dispatches to existing *App.soldiers.{Update,Delete}; no new
// business logic. `--from <path> | --from-stdin` mirrors the
// create path; `delete <id>` takes the id (or display_id) as a
// positional arg.

func TestParseMutateCommand_SoldierUpdate(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantOK  bool
		wantCmd MutateCommand
	}{
		{
			name:    "soldier update <id> --from <path>",
			args:    []string{"soldier", "update", "42", "--from", "/tmp/soldier.json"},
			wantOK:  true,
			wantCmd: MutateSoldierUpdate,
		},
		{
			name:    "soldier update <id> --from-stdin",
			args:    []string{"soldier", "update", "DXD-00001", "--from-stdin"},
			wantOK:  true,
			wantCmd: MutateSoldierUpdate,
		},
		{
			name:    "missing id",
			args:    []string{"soldier", "update", "--from", "/tmp/x.json"},
			wantOK:  false,
		},
		{
			name:    "missing --from / --from-stdin",
			args:    []string{"soldier", "update", "42"},
			wantOK:  false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts, ok := ParseMutateCommand(tc.args)
			if ok != tc.wantOK {
				t.Fatalf("ParseMutateCommand ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if opts.Command != tc.wantCmd {
				t.Errorf("Command = %v, want %v", opts.Command, tc.wantCmd)
			}
		})
	}
}

func TestParseMutateCommand_SoldierDelete(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantOK  bool
		wantCmd MutateCommand
		wantID  string
	}{
		{
			name:    "soldier delete <numeric id>",
			args:    []string{"soldier", "delete", "42"},
			wantOK:  true,
			wantCmd: MutateSoldierDelete,
			wantID:  "42",
		},
		{
			name:    "soldier delete <display_id>",
			args:    []string{"soldier", "delete", "DXD-00001"},
			wantOK:  true,
			wantCmd: MutateSoldierDelete,
			wantID:  "DXD-00001",
		},
		{
			name:   "missing id",
			args:   []string{"soldier", "delete"},
			wantOK: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts, ok := ParseMutateCommand(tc.args)
			if ok != tc.wantOK {
				t.Fatalf("ParseMutateCommand ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if opts.Command != tc.wantCmd {
				t.Errorf("Command = %v, want %v", opts.Command, tc.wantCmd)
			}
			if opts.TargetID != tc.wantID {
				t.Errorf("TargetID = %q, want %q", opts.TargetID, tc.wantID)
			}
		})
	}
}

// TestRunMutateSoldierUpdate seeds a fresh archive, creates a
// soldier, then updates it via RunMutate and asserts the row in
// the DB reflects the new field values.
func TestRunMutateSoldierUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode (seeds archive + starts App)")
	}
	dir, err := os.MkdirTemp("", "dixiedata-mutate-update-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Setenv("DIXIEDATA_DATA_DIR", dir)

	if _, err := seed.Generate(seed.Options{
		DataDir:  dir,
		Soldiers: 0,
		Seed:     1865,
		Reset:    true,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	a := NewApp()
	ctx := context.Background()
	a.Startup(ctx)
	t.Cleanup(func() {
		closeJobsLogWriterTest(a)
		a.Shutdown(ctx)
		_ = os.RemoveAll(dir)
	})

	// Create the row we'll update.
	created, err := a.soldiers.Create(models.Soldier{
		FirstName: "OldFirst",
		LastName:  "OldLast",
	})
	if err != nil {
		t.Fatalf("seed create: %v", err)
	}

	// Build the input JSON for the update.
	updated := models.Soldier{
		ID:        created.ID,
		FirstName: "NewFirst",
		LastName:  "NewLast",
	}
	inputJSON, err := json.Marshal(updated)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	inputPath := dir + "/update.json"
	if err := writeTestFile(inputPath, inputJSON); err != nil {
		t.Fatalf("write input: %v", err)
	}

	var buf bytes.Buffer
	opts := MutateOptions{
		Command:  MutateSoldierUpdate,
		From:     inputPath,
		TargetID: strconv.FormatInt(created.ID, 10),
		JSON:     true,
		Writer:   &buf,
		App:      a,
	}
	code, err := RunMutate(ctx, opts)
	if err != nil {
		t.Fatalf("RunMutate: %v (code=%d)", err, code)
	}
	if code != 0 {
		t.Fatalf("RunMutate exit code = %d, want 0; output:\n%s", code, buf.String())
	}

	var resp struct {
		ID        int64  `json:"id"`
		DisplayID string `json:"display_id"`
	}
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v\noutput: %s", err, buf.String())
	}
	if resp.ID != created.ID {
		t.Errorf("response id = %d, want %d", resp.ID, created.ID)
	}

	got, err := a.soldiers.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID(%d): %v", created.ID, err)
	}
	if got.FirstName != "NewFirst" || got.LastName != "NewLast" {
		t.Errorf("update did not apply: first=%q last=%q", got.FirstName, got.LastName)
	}
}

// TestRunMutateSoldierDelete seeds a fresh archive, creates a
// soldier, then deletes it via RunMutate and asserts the row is
// gone.
func TestRunMutateSoldierDelete(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode (seeds archive + starts App)")
	}
	dir, err := os.MkdirTemp("", "dixiedata-mutate-delete-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Setenv("DIXIEDATA_DATA_DIR", dir)

	if _, err := seed.Generate(seed.Options{
		DataDir:  dir,
		Soldiers: 0,
		Seed:     1865,
		Reset:    true,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	a := NewApp()
	ctx := context.Background()
	a.Startup(ctx)
	t.Cleanup(func() {
		closeJobsLogWriterTest(a)
		a.Shutdown(ctx)
		_ = os.RemoveAll(dir)
	})

	created, err := a.soldiers.Create(models.Soldier{
		FirstName: "Doomed",
		LastName:  "Soldier",
	})
	if err != nil {
		t.Fatalf("seed create: %v", err)
	}

	var buf bytes.Buffer
	opts := MutateOptions{
		Command:  MutateSoldierDelete,
		TargetID: strconv.FormatInt(created.ID, 10),
		JSON:     true,
		Writer:   &buf,
		App:      a,
	}
	code, err := RunMutate(ctx, opts)
	if err != nil {
		t.Fatalf("RunMutate: %v (code=%d)", err, code)
	}
	if code != 0 {
		t.Fatalf("RunMutate exit code = %d, want 0; output:\n%s", code, buf.String())
	}

	if strings.TrimSpace(buf.String()) == "" {
		t.Errorf("expected JSON output, got empty")
	}

	// Confirm the row is gone.
	if _, err := a.soldiers.GetByID(created.ID); err == nil {
		t.Errorf("row %d still present after delete", created.ID)
	}
}

// closeJobsLogWriterTest reaches into the jobs.Registry to
// close the JSONL log file handle before t.Cleanup unlinks
// the data dir. Mirrors cli_debug_test.go::closeJobsLogWriter
// — the registry keeps the log writer open for the life of
// the process and App.Shutdown does not reach it. Test-only;
// production code calls os.Exit after Shutdown so the
// dangling handle is never observed outside tests.
func closeJobsLogWriterTest(a *App) {
	if a == nil || a.jobs == nil {
		return
	}
	v := reflect.ValueOf(a.jobs).Elem()
	closer := v.FieldByName("logCloser")
	if !closer.IsValid() || closer.IsZero() {
		return
	}
	ptr := unsafe.Pointer(closer.UnsafeAddr())
	c := *(*io.Closer)(ptr)
	if c != nil {
		_ = c.Close()
	}
}