package appshell

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHasImportSubcommand(t *testing.T) {
	cases := []struct {
		args []string
		want bool
	}{
		{nil, false},
		{[]string{}, false},
		{[]string{"import"}, false}, // missing kind
		{[]string{"import", "backup"}, true},
		{[]string{"import", "shared-archive"}, true},
		{[]string{"import", "images"}, true},
		{[]string{"import", "memorial-json"}, true},
		{[]string{"import", "frobnicate"}, false},
		{[]string{"export", "pdf"}, false},
		{[]string{"list"}, false},
	}
	for _, tc := range cases {
		if got := HasImportSubcommand(tc.args); got != tc.want {
			t.Errorf("HasImportSubcommand(%v) = %v, want %v", tc.args, got, tc.want)
		}
	}
}

func TestImportKindString(t *testing.T) {
	cases := []struct {
		k    ImportKind
		want string
	}{
		{ImportBackup, "backup"},
		{ImportSharedArchive, "shared-archive"},
		{ImportImages, "images"},
		{ImportMemorialJSON, "memorial-json"},
		{ImportUnknown, "unknown"},
	}
	for _, tc := range cases {
		if got := tc.k.String(); got != tc.want {
			t.Errorf("ImportKind(%d).String() = %q, want %q", tc.k, got, tc.want)
		}
	}
}

func TestImportKindIsDestructive(t *testing.T) {
	if !ImportBackup.IsDestructive() {
		t.Error("ImportBackup should be destructive")
	}
	if !ImportSharedArchive.IsDestructive() {
		t.Error("ImportSharedArchive should be destructive")
	}
	if ImportImages.IsDestructive() {
		t.Error("ImportImages should NOT be destructive")
	}
	if ImportMemorialJSON.IsDestructive() {
		t.Error("ImportMemorialJSON should NOT be destructive")
	}
}

func TestParseImportArgs_BackupDryRun(t *testing.T) {
	opts, err := ParseImportArgs([]string{"import", "backup", "--from", "/tmp/x.ddbak", "--dry-run"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.Kind != ImportBackup {
		t.Errorf("Kind = %v, want ImportBackup", opts.Kind)
	}
	if !opts.DryRun {
		t.Error("DryRun = false, want true")
	}
	if len(opts.FromPaths) != 1 || opts.FromPaths[0] != "/tmp/x.ddbak" {
		t.Errorf("FromPaths = %v, want [/tmp/x.ddbak]", opts.FromPaths)
	}
}

func TestParseImportArgs_BackupRequiresYes(t *testing.T) {
	// Without --dry-run or --yes, the validator should refuse
	// because backup overwrites data.
	_, err := ParseImportArgs([]string{"import", "backup", "--from", "/tmp/x.ddbak"})
	if err == nil {
		t.Fatal("expected error for non-dry-run backup without --yes")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("error %q should mention --yes", err.Error())
	}
}

func TestParseImportArgs_BackupWithYes(t *testing.T) {
	opts, err := ParseImportArgs([]string{"import", "backup", "--from", "/tmp/x.ddbak", "--yes"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !opts.Yes {
		t.Error("Yes = false, want true")
	}
}

func TestParseImportArgs_SharedArchiveFromEqForm(t *testing.T) {
	opts, err := ParseImportArgs([]string{"import", "shared-archive", "--from=/tmp/x.ddshare", "--yes"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.Kind != ImportSharedArchive {
		t.Errorf("Kind = %v, want ImportSharedArchive", opts.Kind)
	}
	if opts.FromPaths[0] != "/tmp/x.ddshare" {
		t.Errorf("FromPaths[0] = %q", opts.FromPaths[0])
	}
}

func TestParseImportArgs_ImagesSoldierNumeric(t *testing.T) {
	opts, err := ParseImportArgs([]string{
		"import", "images", "--soldier", "54", "--from", "a.jpg", "--from", "b.jpg",
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.Kind != ImportImages {
		t.Errorf("Kind = %v, want ImportImages", opts.Kind)
	}
	if opts.SoldierID != 54 {
		t.Errorf("SoldierID = %d, want 54", opts.SoldierID)
	}
	if opts.DisplayID != "" {
		t.Errorf("DisplayID = %q, want empty", opts.DisplayID)
	}
	if len(opts.FromPaths) != 2 {
		t.Errorf("FromPaths = %v, want 2 entries", opts.FromPaths)
	}
}

func TestParseImportArgs_ImagesSoldierDisplayID(t *testing.T) {
	opts, err := ParseImportArgs([]string{
		"import", "images", "--soldier", "DXD-00052", "--from", "a.jpg",
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.SoldierID != 0 {
		t.Errorf("SoldierID = %d, want 0", opts.SoldierID)
	}
	if opts.DisplayID != "DXD-00052" {
		t.Errorf("DisplayID = %q, want DXD-00052", opts.DisplayID)
	}
}

func TestParseImportArgs_ImagesMissingSoldier(t *testing.T) {
	_, err := ParseImportArgs([]string{"import", "images", "--from", "a.jpg"})
	if err == nil {
		t.Fatal("expected error for missing --soldier")
	}
}

func TestParseImportArgs_ImagesMissingFrom(t *testing.T) {
	_, err := ParseImportArgs([]string{"import", "images", "--soldier", "54"})
	if err == nil {
		t.Fatal("expected error for missing --from")
	}
}

func TestParseImportArgs_MemorialJSON(t *testing.T) {
	opts, err := ParseImportArgs([]string{"import", "memorial-json", "--from", "/tmp/m.json"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.Kind != ImportMemorialJSON {
		t.Errorf("Kind = %v, want ImportMemorialJSON", opts.Kind)
	}
	// Memorial-json is additive, no --yes required.
	if opts.Yes {
		t.Error("Yes should be false for memorial-json")
	}
}

func TestParseImportArgs_UnknownFlag(t *testing.T) {
	_, err := ParseImportArgs([]string{"import", "backup", "--frobnicate", "--from", "/tmp/x.ddbak", "--yes"})
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
}

func TestParseImportArgs_UnknownKind(t *testing.T) {
	_, err := ParseImportArgs([]string{"import", "frobnicate", "--from", "/tmp/x"})
	if err == nil {
		t.Fatal("expected error for unknown kind")
	}
}

func TestParseImportArgs_NotImport(t *testing.T) {
	// Non-import args should error cleanly so the caller can fall
	// through to the next dispatch (HasImportSubcommand is the
	// gatekeeper; this test locks the error message).
	_, err := ParseImportArgs([]string{"list", "soldiers"})
	if err == nil {
		t.Fatal("expected error for non-import args")
	}
	if !strings.Contains(err.Error(), "not an import subcommand") {
		t.Errorf("error %q should mention 'not an import subcommand'", err.Error())
	}
}

func TestParseImportArgs_JSON(t *testing.T) {
	opts, err := ParseImportArgs([]string{"import", "backup", "--from", "/tmp/x.ddbak", "--dry-run", "--json"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !opts.JSON {
		t.Error("JSON = false, want true")
	}
}

func TestParseImportArgs_TooManyFromBackup(t *testing.T) {
	_, err := ParseImportArgs([]string{"import", "backup", "--from", "a", "--from", "b", "--yes"})
	if err == nil {
		t.Fatal("expected error for multiple --from with backup")
	}
}

// TestReadBackupManifestFromZip creates a real .ddbak on disk
// with a manifest.json entry and verifies the CLI dry-run
// preview helper reads it correctly.
func TestReadBackupManifestFromZip(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "test.ddbak")

	manifest := map[string]any{
		"format":         "ddbak",
		"version":        1,
		"archive_kind":   "backup",
		"app_version":    "1.2.54",
		"schema_version": 17,
		"soldiers":       501,
		"records":        1683,
		"images":         1139,
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	zw := zip.NewWriter(f)
	mw, err := zw.Create("manifest.json")
	if err != nil {
		t.Fatalf("create manifest entry: %v", err)
	}
	if _, err := mw.Write(manifestBytes); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}

	got, err := readBackupManifestFromZip(zipPath)
	if err != nil {
		t.Fatalf("readBackupManifestFromZip: %v", err)
	}
	if got.Soldiers != 501 {
		t.Errorf("Soldiers = %d, want 501", got.Soldiers)
	}
	if got.Records != 1683 {
		t.Errorf("Records = %d, want 1683", got.Records)
	}
	if got.Images != 1139 {
		t.Errorf("Images = %d, want 1139", got.Images)
	}
}

func TestReadBackupManifestFromZip_NotAZip(t *testing.T) {
	tmpDir := t.TempDir()
	notZip := filepath.Join(tmpDir, "not-a-zip.txt")
	if err := os.WriteFile(notZip, []byte("plain text"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := readBackupManifestFromZip(notZip)
	if err == nil {
		t.Fatal("expected error reading plain text as zip")
	}
}

func TestReadBackupManifestFromZip_NoManifest(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "no-manifest.zip")

	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("readme.txt")
	if err != nil {
		t.Fatalf("create entry: %v", err)
	}
	if _, err := w.Write([]byte("no manifest here")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	_, err = readBackupManifestFromZip(zipPath)
	if err == nil {
		t.Fatal("expected error for zip without manifest.json")
	}
}

func TestReadSharedArchiveManifest_RejectsWrongKind(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "test.ddshare")

	manifest := map[string]any{
		"format":       "ddbak",
		"version":      1,
		"archive_kind": "backup", // wrong kind
		"soldiers":     100,
		"records":      200,
		"images":       50,
	}
	manifestBytes, _ := json.Marshal(manifest)

	f, _ := os.Create(zipPath)
	zw := zip.NewWriter(f)
	mw, _ := zw.Create("manifest.json")
	mw.Write(manifestBytes)
	zw.Close()
	f.Close()

	_, err := readSharedArchiveManifest(zipPath)
	if err == nil {
		t.Fatal("expected error for wrong archive_kind")
	}
}

func TestReadSharedArchiveManifest_AcceptsSharedKind(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "test.ddshare")

	manifest := map[string]any{
		"format":       "ddbak",
		"version":      1,
		"archive_kind": "shared",
		"owner_name":   "Jeremy Morris",
		"soldiers":     100,
		"records":      200,
		"images":       50,
	}
	manifestBytes, _ := json.Marshal(manifest)

	f, _ := os.Create(zipPath)
	zw := zip.NewWriter(f)
	mw, _ := zw.Create("manifest.json")
	mw.Write(manifestBytes)
	zw.Close()
	f.Close()

	got, err := readSharedArchiveManifest(zipPath)
	if err != nil {
		t.Fatalf("readSharedArchiveManifest: %v", err)
	}
	if got.OwnerName != "Jeremy Morris" {
		t.Errorf("OwnerName = %q, want Jeremy Morris", got.OwnerName)
	}
}

// loadLocalImportIdentity is a thin wrapper over SystemConfig +
// UserIdentity. Without a database open it returns an error.
// Lock the error so we don't accidentally swallow real errors.
func TestLoadLocalImportIdentity_NilDatabase(t *testing.T) {
	a := &App{}
	_, _, err := loadLocalImportIdentity(a)
	if err == nil {
		t.Fatal("expected error for nil database")
	}
}

// TestImportRestorePointSiblingConvention locks the path
// convention that Phase 5 + Phase 6 both rely on. The
// sibling root is <parent>/<dataDirBase>-restore-points/ so
// it sorts alphabetically next to the data dir. If this
// changes, every existing on-disk sibling restore point
// will orphan and the next import's rollback will silently
// fail to find them.
//
// Uses filepath.Join directly in the test so the assertion
// is platform-aware (Windows would otherwise produce
// backslashes via filepath.Join inside the helper).
func TestImportRestorePointSiblingConvention(t *testing.T) {
	cases := []struct {
		dataDir string
		want    string
	}{
		{`C:\proj\.dixiedata`, filepath.Join(`C:\proj`, `.dixiedata-restore-points`)},
		{`/home/u/proj/.dixiedata`, filepath.Join(`/home/u/proj`, `.dixiedata-restore-points`)},
		{`/tmp/.dixiedata`, filepath.Join(`/tmp`, `.dixiedata-restore-points`)},
	}
	for _, tc := range cases {
		if got := importRestorePointSibling(tc.dataDir); got != tc.want {
			t.Errorf("importRestorePointSibling(%q) = %q, want %q", tc.dataDir, got, tc.want)
		}
	}
}