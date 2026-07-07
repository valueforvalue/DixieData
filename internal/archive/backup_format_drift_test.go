package archive

// Issue #383 slice 7: import-side policy enforcement for the
// discoverable .ddbak format_version stamp.
//
// The stamp was added in commit b1dc871 (every .ddbak +
// .ddshare manifest now carries format_version). Slice 7
// wires the reader side: every path that opens an archive
// and reads the manifest must:
//   1. read the format_version field
//   2. refuse a major-bump with a typed error
//      (ErrDDBakFormatMismatch, mirroring
//      records.ErrMemorialFormatMismatch)
//   3. accumulate minor-bump + missing-stamp warnings into
//      a returned DriftWarnings []string (or equivalent
//      carrier) so the caller can surface them in the
//      summary / CLI / GUI
//   4. be silent when the version matches the running binary
//
// The single point of enforcement is readBackupContents
// (internal/archive/backup_service.go:1165) — every reader
// (ImportWithLocalIdentity, RestoreBackupArchive,
// ImportSharedBackup) calls it, so the check is wired once
// and inherited by all three paths.
//
// The RED-first regression net here proves:
//   1. A pre-v1 archive (no format_version) reads OK +
//      emits a "pre-v1" warning
//   2. A same-version archive (ddbak_v1) reads OK +
//      emits no warning
//   3. A minor-bump archive (ddbak_v1.1) reads OK +
//      emits a minor-bump warning
//   4. A major-bump archive (ddbak_v2) refuses with the
//      typed ErrDDBakFormatMismatch + readable message
//   5. parseDDBakVersion correctly splits the namespace
//      string (ddbak_v1, ddbak_v1.1, ddbak_v2.0) into
//      (major, minor, ok)
//   6. The same MajorBump refusal fires for .ddshare
//      archives too (shared archives share the
//      format_version namespace per the stamp decision in
//      commit b1dc871)

import (
	"archive/zip"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/models"
)

// writeManifestZip writes a minimal .ddbak / .ddshare zip
// with the given format_version on the root manifest.json
// so a test can exercise the drift path without running a
// real export.
func writeManifestZip(t *testing.T, formatVersion string, archiveKind string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stamp.ddbak")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer f.Close()
	w := zip.NewWriter(f)
	manifest := fmt.Sprintf(`{
		"format": "%s",
		"version": %d,
		"archive_kind": "%s",
		"app_version": "%s",
		"schema_version": %d,
		"current_update_flow_version": 1,
		"release_counter": 64,
		"created_at": "2026-07-08T12:00:00Z",
		"data_format": "sqlite",
		"database_file": "data/dixiedata.db",
		"image_root": "images/",
		"soldiers": 0,
		"records": 0,
		"images": 0
	}`, backupFormatName, buildinfo.BackupFormatVersion, archiveKind, buildinfo.AppVersion, buildinfo.SchemaVersion)
	if formatVersion != "" {
		manifest = strings.Replace(manifest, `}`,
			fmt.Sprintf(`,"format_version": %q}`, formatVersion), 1)
	}
	mw, err := w.Create("manifest.json")
	if err != nil {
		t.Fatalf("Create manifest: %v", err)
	}
	if _, err := mw.Write([]byte(manifest)); err != nil {
		t.Fatalf("Write manifest: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close zip: %v", err)
	}
	return path
}

func TestReadBackupContentsDriftDetection_PreV1EmitsWarning(t *testing.T) {
	path := writeManifestZip(t, "", archiveKindBackup)
	contents, driftWarnings, err := readBackupContentsWithDrift(path)
	if err != nil {
		t.Fatalf("readBackupContentsWithDrift: %v", err)
	}
	if contents.Manifest.FormatVersion != "" {
		t.Errorf("expected empty FormatVersion, got %q", contents.Manifest.FormatVersion)
	}
	if len(driftWarnings) != 1 {
		t.Errorf("driftWarnings = %v, want 1 (pre-v1 warning)", driftWarnings)
	}
	if !strings.Contains(driftWarnings[0], "pre-v1") {
		t.Errorf("warning %q does not mention pre-v1", driftWarnings[0])
	}
}

func TestReadBackupContentsDriftDetection_SameVersionIsSilent(t *testing.T) {
	path := writeManifestZip(t, buildinfo.DDBakFormatVersion, archiveKindBackup)
	_, driftWarnings, err := readBackupContentsWithDrift(path)
	if err != nil {
		t.Fatalf("readBackupContentsWithDrift: %v", err)
	}
	if len(driftWarnings) != 0 {
		t.Errorf("driftWarnings = %v, want 0 (same version is silent)", driftWarnings)
	}
}

func TestReadBackupContentsDriftDetection_MinorBumpEmitsWarning(t *testing.T) {
	path := writeManifestZip(t, "ddbak_v1.1", archiveKindBackup)
	_, driftWarnings, err := readBackupContentsWithDrift(path)
	if err != nil {
		t.Fatalf("readBackupContentsWithDrift: %v", err)
	}
	if len(driftWarnings) != 1 {
		t.Errorf("driftWarnings = %v, want 1 (minor-bump warning)", driftWarnings)
	}
	if !strings.Contains(driftWarnings[0], "ddbak_v1.1") || !strings.Contains(driftWarnings[0], "minor") {
		t.Errorf("warning %q does not mention the minor-bump + the version", driftWarnings[0])
	}
}

func TestReadBackupContentsDriftDetection_MajorBumpRefuses(t *testing.T) {
	path := writeManifestZip(t, "ddbak_v2", archiveKindBackup)
	_, _, err := readBackupContentsWithDrift(path)
	if err == nil {
		t.Fatal("readBackupContentsWithDrift returned nil error on major bump; want ErrDDBakFormatMismatch")
	}
	if !errors.Is(err, ErrDDBakFormatMismatch) {
		t.Errorf("err = %v, want errors.Is(ErrDDBakFormatMismatch)", err)
	}
	if !strings.Contains(err.Error(), "ddbak_v2") {
		t.Errorf("err message %q does not mention ddbak_v2", err.Error())
	}
}

func TestReadBackupContentsDriftDetection_MajorBumpFiresForSharedArchive(t *testing.T) {
	path := writeManifestZip(t, "ddbak_v2", archiveKindShared)
	_, _, err := readBackupContentsWithDrift(path)
	if err == nil {
		t.Fatal("readBackupContentsWithDrift returned nil error on shared-archive major bump; want ErrDDBakFormatMismatch")
	}
	if !errors.Is(err, ErrDDBakFormatMismatch) {
		t.Errorf("err = %v, want errors.Is(ErrDDBakFormatMismatch)", err)
	}
}

func TestParseDDBakVersion(t *testing.T) {
	cases := []struct {
		input       string
		wantMajor   int
		wantMinor   int
		wantOK      bool
	}{
		{"ddbak_v1", 1, 0, true},
		{"ddbak_v1.0", 1, 0, true},
		{"ddbak_v1.1", 1, 1, true},
		{"ddbak_v2.0", 2, 0, true},
		{"ddbak_v10.3", 10, 3, true},
		{"", 0, 0, false},
		{"v1", 0, 0, false},
		{"ddbak_", 0, 0, false},
		{"ddbak_v", 0, 0, false},
		{"ddbak_v1.1.1", 0, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			major, minor, ok := parseDDBakVersion(tc.input)
			if ok != tc.wantOK {
				t.Errorf("parseDDBakVersion(%q) ok = %v, want %v", tc.input, ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if major != tc.wantMajor {
				t.Errorf("parseDDBakVersion(%q) major = %d, want %d", tc.input, major, tc.wantMajor)
			}
			if minor != tc.wantMinor {
				t.Errorf("parseDDBakVersion(%q) minor = %d, want %d", tc.input, minor, tc.wantMinor)
			}
		})
	}
}

// readBackupContentsWithDrift is a small test helper that
// opens a zip, parses the manifest, and runs ONLY the
// drift-check pair (checkDDBakFormatVersion +
// ddbakFormatWarnings). This isolates the drift behavior
// from the full readBackupContents pipeline (which requires
// a real SQLite snapshot for the file-presence checks).
// The integration test (TestReadBackupContentsInvokesDriftCheck)
// proves the production readBackupContents calls the
// drift check itself.
func readBackupContentsWithDrift(archivePath string) (backupContents, []string, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return backupContents{}, nil, fmt.Errorf("zip.OpenReader: %w", err)
	}
	defer r.Close()
	var manifestFile *zip.File
	for _, f := range r.File {
		if f.Name == "manifest.json" {
			manifestFile = f
			break
		}
	}
	if manifestFile == nil {
		return backupContents{}, nil, fmt.Errorf("missing manifest.json")
	}
	var manifest BackupManifest
	if err := readBackupJSON(manifestFile, &manifest); err != nil {
		return backupContents{}, nil, err
	}
	if err := CheckDDBakFormatVersion(manifest.FormatVersion); err != nil {
		return backupContents{}, nil, err
	}
	return backupContents{Manifest: manifest}, DDBakFormatWarnings(manifest.FormatVersion), nil
}

// _ = models is a compile-time hint to keep the import
// in this file (the test uses models elsewhere when
// wired up against a real restore path in future slices).
var _ = models.Soldier{}

// TestReadBackupContentsInvokesDriftCheck is the integration
// regression that proves the production readBackupContents
// path actually fires the drift check (not just the unit-
// isolated checkDDBakFormatVersion). A major-bump manifest
// with a data_format that would otherwise be accepted must
// refuse with the typed error.
func TestReadBackupContentsInvokesDriftCheck(t *testing.T) {
	// Major-bump manifest + valid data file path: the
	// drift check must fire BEFORE the data-format / file
	// checks, surfacing the typed error.
	path := filepath.Join(t.TempDir(), "major-bump.ddbak")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer f.Close()
	w := zip.NewWriter(f)
	mw, err := w.Create("manifest.json")
	if err != nil {
		t.Fatalf("Create manifest: %v", err)
	}
	mw.Write([]byte(`{
		"format": "dixiedata-backup",
		"version": 3,
		"archive_kind": "backup",
		"app_version": "v1.2.99",
		"schema_version": 99,
		"format_version": "ddbak_v2",
		"created_at": "2026-07-08T12:00:00Z",
		"data_format": "sqlite",
		"database_file": "data/dixiedata.db",
		"image_root": "images/",
		"soldiers": 0,
		"records": 0,
		"images": 0
	}`))
	w.Close()
	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	defer r.Close()
	_, err = readBackupContents(&r.Reader)
	if err == nil {
		t.Fatal("readBackupContents returned nil; want ErrDDBakFormatMismatch on major bump")
	}
	if !errors.Is(err, ErrDDBakFormatMismatch) {
		t.Errorf("err = %v, want errors.Is(ErrDDBakFormatMismatch)", err)
	}
}
