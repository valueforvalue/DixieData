package appshell

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestHasAdminSubcommand(t *testing.T) {
	cases := []struct {
		args []string
		want bool
	}{
		{nil, false},
		{[]string{}, false},
		{[]string{"migrate"}, true},
		{[]string{"backup"}, true},
		{[]string{"restore"}, true},
		{[]string{"logs"}, true},
		{[]string{"config"}, true},
		{[]string{"export"}, false},
		{[]string{"doctor"}, false},
		{[]string{"list"}, false},
		{[]string{"frobnicate"}, false},
	}
	for _, tc := range cases {
		if got := HasAdminSubcommand(tc.args); got != tc.want {
			t.Errorf("HasAdminSubcommand(%v) = %v, want %v", tc.args, got, tc.want)
		}
	}
}

func TestParseAdminArgs_MigrateStatus(t *testing.T) {
	opts, err := ParseAdminArgs([]string{"migrate", "status"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.Kind != AdminMigrate || opts.Action != AdminMigrateStatus {
		t.Errorf("got Kind=%v Action=%v, want AdminMigrate/AdminMigrateStatus", opts.Kind, opts.Action)
	}
}

func TestParseAdminArgs_MigrateUp(t *testing.T) {
	opts, err := ParseAdminArgs([]string{"migrate", "up"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.Action != AdminMigrateUp {
		t.Errorf("got Action=%v, want AdminMigrateUp", opts.Action)
	}
}

func TestParseAdminArgs_MigrateMissingVerb(t *testing.T) {
	if _, err := ParseAdminArgs([]string{"migrate"}); err == nil {
		t.Fatalf("expected error for migrate without subcommand")
	}
}

func TestParseAdminArgs_MigrateDown_AcceptsVersionAndFlags(t *testing.T) {
	// Per issue #273 PR 2: migrate down is now shipped. Verify
	// the parser accepts the version + --yes + --force-irreversible.
	opts, err := ParseAdminArgs([]string{"migrate", "down", "58", "--yes", "--force-irreversible"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.Action != AdminMigrateDown {
		t.Errorf("got Action=%v, want AdminMigrateDown", opts.Action)
	}
	if opts.TargetVersion != 58 {
		t.Errorf("got TargetVersion=%d, want 58", opts.TargetVersion)
	}
	if !opts.Yes {
		t.Error("got Yes=false, want true (--yes flag set)")
	}
	if !opts.ForceIrreversible {
		t.Error("got ForceIrreversible=false, want true (--force-irreversible flag set)")
	}
}

func TestParseAdminArgs_MigrateDown_RejectsMissingVersion(t *testing.T) {
	if _, err := ParseAdminArgs([]string{"migrate", "down"}); err == nil {
		t.Fatal("expected error for migrate down without target version")
	}
}

func TestParseAdminArgs_MigrateDown_RejectsNonIntegerVersion(t *testing.T) {
	if _, err := ParseAdminArgs([]string{"migrate", "down", "fifty"}); err == nil {
		t.Fatal("expected error for migrate down with non-integer target version")
	}
}

func TestParseAdminArgs_MigrateDown_RejectsUnknownFlag(t *testing.T) {
	if _, err := ParseAdminArgs([]string{"migrate", "down", "58", "--bogus"}); err == nil {
		t.Fatal("expected error for migrate down with unknown flag")
	}
}

func TestParseAdminArgs_MigrateUnknown(t *testing.T) {
	if _, err := ParseAdminArgs([]string{"migrate", "frobnicate"}); err == nil {
		t.Fatalf("expected error for unknown migrate subcommand")
	}
}

func TestParseAdminArgs_BackupList(t *testing.T) {
	opts, err := ParseAdminArgs([]string{"backup", "list"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.Action != AdminBackupList {
		t.Errorf("got Action=%v, want AdminBackupList", opts.Action)
	}
}

func TestParseAdminArgs_BackupPruneWithKeepLast(t *testing.T) {
	opts, err := ParseAdminArgs([]string{"backup", "prune", "--keep-last", "3"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.Action != AdminBackupPrune {
		t.Errorf("got Action=%v, want AdminBackupPrune", opts.Action)
	}
	if opts.KeepLast != 3 {
		t.Errorf("KeepLast = %d, want 3", opts.KeepLast)
	}
}

func TestParseAdminArgs_BackupPruneDefault(t *testing.T) {
	opts, err := ParseAdminArgs([]string{"backup", "prune"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.KeepLast != 5 {
		t.Errorf("KeepLast default = %d, want 5", opts.KeepLast)
	}
}

func TestParseAdminArgs_BackupKeepLastEqualsForm(t *testing.T) {
	opts, err := ParseAdminArgs([]string{"backup", "prune", "--keep-last=2"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.KeepLast != 2 {
		t.Errorf("KeepLast = %d, want 2", opts.KeepLast)
	}
}

func TestParseAdminArgs_RestorePointList(t *testing.T) {
	opts, err := ParseAdminArgs([]string{"restore", "point", "list"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.Action != AdminRestorePointList {
		t.Errorf("got Action=%v, want AdminRestorePointList", opts.Action)
	}
}

func TestParseAdminArgs_RestorePointCreate(t *testing.T) {
	opts, err := ParseAdminArgs([]string{
		"restore", "point", "create", "--note", "pre-import", "--root", "/tmp/rp",
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.Action != AdminRestorePointCreate {
		t.Errorf("got Action=%v, want AdminRestorePointCreate", opts.Action)
	}
	if opts.Note != "pre-import" {
		t.Errorf("Note = %q, want pre-import", opts.Note)
	}
	if opts.RestorePointRoot != "/tmp/rp" {
		t.Errorf("RestorePointRoot = %q, want /tmp/rp", opts.RestorePointRoot)
	}
}

func TestParseAdminArgs_RestorePointApply(t *testing.T) {
	opts, err := ParseAdminArgs([]string{
		"restore", "point", "apply", "restore-point-20260628-070504",
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.Action != AdminRestorePointApply {
		t.Errorf("got Action=%v, want AdminRestorePointApply", opts.Action)
	}
	if opts.RestorePointID != "restore-point-20260628-070504" {
		t.Errorf("RestorePointID = %q", opts.RestorePointID)
	}
}

func TestParseAdminArgs_RestorePointApplyMissingID(t *testing.T) {
	if _, err := ParseAdminArgs([]string{"restore", "point", "apply"}); err == nil {
		t.Fatalf("expected error for apply without <id>")
	}
}

func TestParseAdminArgs_RestorePointUnknownVerb(t *testing.T) {
	if _, err := ParseAdminArgs([]string{"restore", "point", "delete", "x"}); err == nil {
		t.Fatalf("expected error for delete (not shipped)")
	}
}

func TestParseAdminArgs_RestorePointApplyDryRun(t *testing.T) {
	opts, err := ParseAdminArgs([]string{
		"restore", "point", "apply", "--dry-run", "restore-point-20260628-070504",
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !opts.DryRun {
		t.Error("expected DryRun=true")
	}
	if opts.RestorePointID != "restore-point-20260628-070504" {
		t.Errorf("RestorePointID = %q", opts.RestorePointID)
	}
}

// TestParseAdminArgs_DataDirPathWithSpaces verifies that
// --data-dir accepts a path with embedded spaces. Issue #276.
// We don't boot the app here — that requires a temp archive
// and the path is what's under test, not the boot. We do
// assert that the parser stores the path verbatim (no
// shell-style splitting, no trim).
func TestParseAdminArgs_DataDirPathWithSpaces(t *testing.T) {
	cases := []string{
		"/Users/Jane Doe/AppData/Local/DixieData",
		`C:\Users\Jane Doe\AppData\Local\DixieData`,
		"/path/with space",
		"  /leading/space",
		"/trailing/space  ",
		`/path/with"quote`,
	}
	for _, p := range cases {
		t.Run(p, func(t *testing.T) {
			opts, err := ParseAdminArgs([]string{
				"config", "show", "--data-dir=" + p,
			})
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if opts.DataDir != p {
				t.Errorf("DataDir = %q, want %q", opts.DataDir, p)
			}
		})
	}
}

func TestParseAdminArgs_LogsPath(t *testing.T) {
	opts, err := ParseAdminArgs([]string{"logs", "path"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.Action != AdminLogsPath {
		t.Errorf("got Action=%v, want AdminLogsPath", opts.Action)
	}
}

func TestParseAdminArgs_LogsTailWithLines(t *testing.T) {
	opts, err := ParseAdminArgs([]string{"logs", "tail", "--lines", "50", "--follow"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.Action != AdminLogsTail {
		t.Errorf("got Action=%v, want AdminLogsTail", opts.Action)
	}
	if opts.TailLines != 50 {
		t.Errorf("TailLines = %d, want 50", opts.TailLines)
	}
	if !opts.Follow {
		t.Errorf("Follow = false, want true")
	}
}

func TestParseAdminArgs_LogsTailDefault(t *testing.T) {
	opts, err := ParseAdminArgs([]string{"logs", "tail"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.TailLines != 100 {
		t.Errorf("TailLines default = %d, want 100", opts.TailLines)
	}
}

func TestParseAdminArgs_ConfigShow(t *testing.T) {
	opts, err := ParseAdminArgs([]string{"config", "show"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.Action != AdminConfigShow {
		t.Errorf("got Action=%v, want AdminConfigShow", opts.Action)
	}
}

func TestParseAdminArgs_ConfigSetDebugMode(t *testing.T) {
	opts, err := ParseAdminArgs([]string{"config", "set", "debug_mode", "true"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.Action != AdminConfigSet {
		t.Errorf("got Action=%v, want AdminConfigSet", opts.Action)
	}
	if opts.ConfigKey != "debug_mode" {
		t.Errorf("ConfigKey = %q", opts.ConfigKey)
	}
	if opts.ConfigValue != "true" {
		t.Errorf("ConfigValue = %q", opts.ConfigValue)
	}
}

func TestParseAdminArgs_ConfigSetUnknownKey(t *testing.T) {
	if _, err := ParseAdminArgs([]string{"config", "set", "frobnicate", "1"}); err == nil {
		t.Fatalf("expected error for unknown config key")
	}
}

func TestParseAdminArgs_ConfigSetMissingValue(t *testing.T) {
	if _, err := ParseAdminArgs([]string{"config", "set", "debug_mode"}); err == nil {
		t.Fatalf("expected error for config set without value")
	}
}

func TestParseAdminArgs_DataDirFlag(t *testing.T) {
	opts, err := ParseAdminArgs([]string{"config", "show", "--data-dir", "/tmp/custom"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.DataDir != "/tmp/custom" {
		t.Errorf("DataDir = %q, want /tmp/custom", opts.DataDir)
	}
}

func TestParseAdminArgs_DataDirEqualsForm(t *testing.T) {
	opts, err := ParseAdminArgs([]string{"config", "show", "--data-dir=/tmp/eq"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.DataDir != "/tmp/eq" {
		t.Errorf("DataDir = %q, want /tmp/eq", opts.DataDir)
	}
}

func TestParseAdminArgs_JSONFlag(t *testing.T) {
	opts, err := ParseAdminArgs([]string{"backup", "list", "--json"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !opts.JSON {
		t.Errorf("JSON = false, want true")
	}
}

func TestParseAdminArgs_NotAdmin(t *testing.T) {
	opts, err := ParseAdminArgs([]string{"export", "pdf"})
	if err != nil {
		t.Fatalf("not-admin args should not error, got %v", err)
	}
	if opts.Kind != AdminUnknown {
		t.Errorf("Kind = %v, want AdminUnknown", opts.Kind)
	}
}

func TestIsKnownConfigKey(t *testing.T) {
	if !isKnownConfigKey("debug_mode") {
		t.Errorf("debug_mode should be known")
	}
	if isKnownConfigKey("frobnicate") {
		t.Errorf("frobnicate should not be known")
	}
}

// --- handler tests (read-only, no App) ---

func TestRunAdminLogsPath_NoApp(t *testing.T) {
	// LogsPath needs App.dataDir; without an App it returns 2.
	opts := AdminOptions{Kind: AdminLogs, Action: AdminLogsPath}
	code, err := RunAdmin(t.Context(), opts)
	if err == nil || code != 2 {
		t.Errorf("got code=%d err=%v, want code=2 err!=nil", code, err)
	}
}

func TestRunAdminConfigShow_NoApp(t *testing.T) {
	opts := AdminOptions{Kind: AdminConfig, Action: AdminConfigShow}
	code, err := RunAdmin(t.Context(), opts)
	if err == nil || code != 2 {
		t.Errorf("got code=%d err=%v, want code=2 err!=nil", code, err)
	}
}

func TestRunAdmin_UnknownAction(t *testing.T) {
	opts := AdminOptions{}
	code, err := RunAdmin(t.Context(), opts)
	if err == nil || code != 3 {
		t.Errorf("got code=%d err=%v, want code=3 err!=nil", code, err)
	}
}

// TestRunAdminMigrateDown_RefusesWithoutYes covers the CLI gate
// that requires --yes before runAdminMigrateDown will even open
// the database. The runner refuses with code 1 + a user-facing
// message before any DB work happens.
func TestRunAdminMigrateDown_RefusesWithoutYes(t *testing.T) {
	app := newHeadlessAppForTest(t)
	var buf bytes.Buffer
	opts := AdminOptions{
		Kind:          AdminMigrate,
		Action:        AdminMigrateDown,
		TargetVersion: 58,
		Yes:           false, // explicit; gate must trip
		Writer:        &buf,
		App:           app,
	}
	code, err := RunAdmin(t.Context(), opts)
	if err == nil {
		t.Fatal("expected error from RunAdmin (refusal without --yes)")
	}
	if code != 1 {
		t.Errorf("got code=%d, want 1 (refusal exit code)", code)
	}
	if !strings.Contains(buf.String(), "--yes is required") {
		t.Errorf("user-facing message missing --yes hint; got: %q", buf.String())
	}
}

// TestRunAdminMigrateDown_ManifestPrinted covers the manifest
// path: the runner prints a per-block table for the path BEFORE
// applying the inverse, regardless of whether the path is fully
// Reversible or crosses an Irreversible boundary. The manifest
// must list every block in the window with its reversibility
// marker (R/P/I) and one-line Reason. The runner also takes a
// pre-downgrade snapshot (issue #273 PR 3) whose ID is reported
// so the operator can roll back via restore point apply.
func TestRunAdminMigrateDown_ManifestPrinted(t *testing.T) {
	app := newHeadlessAppForTest(t)
	var buf bytes.Buffer
	opts := AdminOptions{
		Kind:               AdminMigrate,
		Action:             AdminMigrateDown,
		TargetVersion:      0, // crosses every block
		Yes:                true,
		ForceIrreversible:  true, // forces the manifest branch even though the path refuses
		Writer:             &buf,
		App:                app,
	}
	_, err := RunAdmin(t.Context(), opts)
	// The path crosses block-60-v54-to-v60-jump (Irreversible) so
	// the runner refuses. We don't care about the error here; we
	// only care that the manifest + the pre-downgrade snapshot
	// appeared in the output.
	out := buf.String()
	if !strings.Contains(out, "schema down manifest") {
		t.Errorf("manifest header missing; got: %q", out)
	}
	if !strings.Contains(out, "[I] block-") {
		t.Errorf("manifest should mark at least one Irreversible block; got: %q", out)
	}
	if !strings.Contains(out, "block-60-v54-to-v60-jump") {
		t.Errorf("manifest should enumerate block-60-v54-to-v60-jump; got: %q", out)
	}
	// Pre-downgrade snapshot is taken BEFORE the runner fires
	// (per issue #273 PR 3); the runner refuses on the jump but
	// the snapshot is still on disk. The output must report the
	// snapshot ID and direct the operator to use it for rollback.
	if !strings.Contains(out, "pre-downgrade snapshot:") {
		t.Errorf("pre-downgrade snapshot line missing; got: %q", out)
	}
	if !strings.Contains(out, "available for rollback") {
		t.Errorf("rollback hint missing; got: %q", out)
	}
	if err == nil {
		t.Errorf("expected refusal error (path crosses block-60-v54-to-v60-jump Irreversible)")
	}
}

// --- tailFile test (pure I/O) ---

func TestTailFile_ReturnsLastNLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	lines := []string{}
	for i := 0; i < 10; i++ {
		lines = append(lines, "line-"+strconv.Itoa(i))
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := tailFile(path, 3)
	if err != nil {
		t.Fatalf("tailFile: %v", err)
	}
	if len(got) != 3 || got[0] != "line-7" || got[2] != "line-9" {
		t.Errorf("tailFile(3) = %v, want [line-7 line-8 line-9]", got)
	}
}

func TestTailFile_FewerThanN(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	if err := os.WriteFile(path, []byte("only\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := tailFile(path, 50)
	if err != nil {
		t.Fatalf("tailFile: %v", err)
	}
	if len(got) != 1 || got[0] != "only" {
		t.Errorf("tailFile(50) = %v, want [only]", got)
	}
}

func TestTailFile_Missing(t *testing.T) {
	if _, err := tailFile(filepath.Join(t.TempDir(), "nope"), 10); err == nil {
		t.Errorf("expected error for missing file")
	}
}
