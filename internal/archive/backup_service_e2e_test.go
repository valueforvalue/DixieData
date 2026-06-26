package archive

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
)

// TestBackupService_ImportSeededArchiveRoundTrip is the feedback loop
// for the user-reported "can no longer load ddbak archives" bug.
//
// Steps:
//  1. Locate a real .ddbak file in the repo (schema_version may be
//     much older than the current build — exercises the full
//     migration chain on import).
//  2. Open a temp DB so we can construct a BackupService (the
//     import handler closes + reopens the DB around the import).
//  3. Import the .ddbak into a fresh dataDir using the same code
//     path as handleImportBackup (ImportWithLocalIdentity).
//  4. Assert the import succeeds, the manifest matches what the
//     archive declares, and the restored DB is queryable.
//
// Skipped under -short so CI stays green.
func TestBackupService_ImportSeededArchiveRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("requires a real .ddbak archive")
	}
	backupPath, err := lookupBackupArchive(t)
	if err != nil {
		t.Skipf("%v", err)
	}

	// Need an open DB to construct a BackupService. Use a tempdir +
	// fresh DB; we'll close it before invoking the import (which
	// itself opens the staged DB).
	sourceDir := t.TempDir()
	sourceDB, err := db.Open(sourceDir)
	if err != nil {
		t.Fatalf("db.Open fresh: %v", err)
	}
	if _, err := sourceDB.ConfigureUserIdentity("Test", "M", "User", 1990); err != nil {
		t.Fatalf("ConfigureUserIdentity: %v", err)
	}
	sourceSoldierSvc := NewSoldierService(sourceDB)
	sourceBackupSvc := NewBackupService(sourceDB, sourceSoldierSvc)

	// Capture the archive's declared counts before importing.
	zr, err := openZipForManifest(backupPath)
	if err != nil {
		t.Fatalf("openZipForManifest: %v", err)
	}
	srcManifest, err := readManifestFromZip(zr)
	zr.Close()
	if err != nil {
		t.Fatalf("readManifestFromZip: %v", err)
	}
	t.Logf("source archive: %d soldiers, %d records, %d images (schema_v=%d, app=%s)",
		srcManifest.Soldiers, srcManifest.Records, srcManifest.Images,
		srcManifest.SchemaVersion, srcManifest.AppVersion)

	// Now do the actual import. BackupService.ImportWithLocalIdentity
	// takes a dataDir + identity + preserve flag.
	restoreDir := t.TempDir()
	identity := models.UserIdentity{FirstName: "Test", LastName: "User"}
	sourceDB.Close()

	manifest, err := sourceBackupSvc.ImportWithLocalIdentity(backupPath, restoreDir, identity, false)
	if err != nil {
		t.Fatalf("ImportWithLocalIdentity: %v", err)
	}
	t.Logf("imported manifest: %+v", manifest)

	// Reopen the restored DB to verify it's queryable.
	restoreDB, err := db.Open(restoreDir)
	if err != nil {
		t.Fatalf("db.Open restored: %v", err)
	}
	defer restoreDB.Close()
	restoreSoldierSvc := NewSoldierService(restoreDB)
	dstCounts, err := restoreSoldierSvc.ArchiveCounts()
	if err != nil {
		t.Fatalf("restored ArchiveCounts: %v", err)
	}
	t.Logf("restored archive counts: %+v", dstCounts)

	// Compare against the TOTAL entries in the restored soldiers
	// table (the manifest's Soldiers field counts every soldier-
	// like row regardless of entry_type, while ArchiveCounts splits
	// by entry_type).
	row := restoreDB.Conn().QueryRow(`SELECT COUNT(*) FROM soldiers`)
	var restoredSoldierCount int
	if err := row.Scan(&restoredSoldierCount); err != nil {
		t.Fatalf("count soldiers: %v", err)
	}
	if restoredSoldierCount != srcManifest.Soldiers {
		t.Errorf("soldier row count mismatch: archive=%d restored=%d", srcManifest.Soldiers, restoredSoldierCount)
	}

	// Compare source records table directly.
	row = restoreDB.Conn().QueryRow(`SELECT COUNT(*) FROM records`)
	var restoredRecordCount int
	if err := row.Scan(&restoredRecordCount); err != nil {
		t.Fatalf("count records: %v", err)
	}
	t.Logf("restored soldiers=%d, records=%d (archive declared soldiers=%d, records=%d)",
		restoredSoldierCount, restoredRecordCount, srcManifest.Soldiers, srcManifest.Records)
	if restoredRecordCount != srcManifest.Records {
		t.Errorf("records table row count mismatch: archive=%d restored=%d", srcManifest.Records, restoredRecordCount)
	}
}

// lookupSeededDir returns the absolute path to a seeded .dixiedata
// archive the test harness can round-trip through. Walks a few
// candidate locations so the test is flexible about where the
// user keeps their seeded data.
func lookupSeededDir(t *testing.T) (string, error) {
	t.Helper()
	candidates := []string{
		filepath.Join(os.TempDir(), "dixie-source"),
		filepath.Join(os.TempDir(), "dixie-audit"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(filepath.Join(c, "dixiedata.db")); err == nil {
			return c, nil
		}
	}
	return "", &os.PathError{Op: "lookup", Path: "seeded", Err: os.ErrNotExist}
}

// lookupBackupArchive returns the absolute path to a .ddbak file the
// test can use as the SOURCE for an import round-trip. The feedback
// loop for the user-reported 'can no longer load ddbak archives'
// bug: this archive is schema_version 20 (current head is v55), so
// importing it runs 35 schema migrations. If any migration fails or
// corrupts data, the import fails.
func lookupBackupArchive(t *testing.T) (string, error) {
	t.Helper()
	// Walk up from cwd looking for the repo root (signalled by wails.json
	// OR go.mod). The repo root contains the .ddbak we want to test.
	cwd, _ := os.Getwd()
	for i := 0; i < 6; i++ {
		candidate := filepath.Join(cwd, "dixiedata-backup-2026-05-30.ddbak")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		cwd = filepath.Dir(cwd)
	}
	return "", &os.PathError{Op: "lookup", Path: "*.ddbak", Err: os.ErrNotExist}
}

// openZipForManifest opens the .ddbak as a zip.Reader.
func openZipForManifest(path string) (*zip.ReadCloser, error) {
	return zip.OpenReader(path)
}

// readManifestFromZip extracts the archive manifest from the zip.
func readManifestFromZip(zr *zip.ReadCloser) (BackupManifest, error) {
	for _, f := range zr.File {
		if filepath.Base(f.Name) != "manifest.json" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return BackupManifest{}, err
		}
		defer rc.Close()
		var manifest BackupManifest
		if err := json.NewDecoder(rc).Decode(&manifest); err != nil {
			return BackupManifest{}, err
		}
		return manifest, nil
	}
	return BackupManifest{}, fmt.Errorf("manifest.json not found in archive")
}