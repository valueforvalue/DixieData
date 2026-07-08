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
	"os"
	"strings"
	"testing"

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
	// Set up a fresh scratch archive.
	dir := t.TempDir()
	t.Setenv("DIXIEDATA_DATA_DIR", dir)
	_ = appdata.DefaultDir // pin import

	// Seed via the seed package.
	if _, err := seed.Generate(seed.Options{
		DataDir: dir,
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
	defer a.Shutdown(ctx)

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