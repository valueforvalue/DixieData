package appshell

import (
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

func TestParseAdminArgs_MigrateUnknown(t *testing.T) {
	if _, err := ParseAdminArgs([]string{"migrate", "down", "1"}); err == nil {
		t.Fatalf("expected error for migrate down (not shipped)")
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
