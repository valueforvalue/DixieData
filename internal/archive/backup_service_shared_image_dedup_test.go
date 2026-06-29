package archive

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
)

// TestBackupService_ImportSharedBackupImageDedup covers the regression
// net for issue #136: a full-duplicate shared import must NOT inflate
// ImagesUpdated (every image row was already there with identical
// mutable columns) and must NOT touch the on-disk image file
// (copySharedImageFile short-circuits on matching size).
//
// Pre-fix behaviour: every image re-copied to disk + ImagesUpdated
// incremented for every row, so a 1140-image round-trip reported
// "Imported 1140 images" with no net change. Post-fix: zero counters
// + unchanged mtime for a full-duplicate second import.
func TestBackupService_ImportSharedBackupImageDedup(t *testing.T) {
	targetDir := t.TempDir()
	targetDB, err := db.Open(targetDir)
	if err != nil {
		t.Fatalf("db.Open target: %v", err)
	}
	defer targetDB.Close()
	targetSvc := NewSoldierService(targetDB)
	backupSvc := NewBackupService(targetDB, targetSvc)

	sourceDir := t.TempDir()
	if err := targetDB.SnapshotTo(db.Path(sourceDir)); err != nil {
		t.Fatalf("SnapshotTo source: %v", err)
	}
	sourceDB, err := db.Open(sourceDir)
	if err != nil {
		t.Fatalf("db.Open source: %v", err)
	}
	defer sourceDB.Close()
	sourceSvc := NewSoldierService(sourceDB)
	sourceBackupSvc := NewBackupService(sourceDB, sourceSvc)

	created, err := sourceSvc.Create(models.Soldier{
		FirstName: "Imaged",
		LastName:  "Veteran",
	})
	if err != nil {
		t.Fatalf("Create source soldier: %v", err)
	}
	imageRel := `images\dedup\portrait.png`
	imageAbs := filepath.Join(sourceDir, "images", "dedup", "portrait.png")
	if err := os.MkdirAll(filepath.Dir(imageAbs), 0o755); err != nil {
		t.Fatalf("MkdirAll source image: %v", err)
	}
	if err := os.WriteFile(imageAbs, pngFixture(), 0o644); err != nil {
		t.Fatalf("WriteFile source image: %v", err)
	}
	if err := sourceSvc.AddImage(created.ID, "portrait.png", imageRel, "Portrait"); err != nil {
		t.Fatalf("AddImage source: %v", err)
	}

	backupPath := filepath.Join(t.TempDir(), "shared-images.ddshare")
	if _, err := sourceBackupSvc.ExportShared(backupPath, sourceDir); err != nil {
		t.Fatalf("Export shared backup: %v", err)
	}

	// First import establishes the baseline: 1 insert, 0 updates.
	first, err := backupSvc.ImportSharedBackup(backupPath, targetDir)
	if err != nil {
		t.Fatalf("ImportSharedBackup first: %v", err)
	}
	if first.ImagesInserted != 1 || first.ImagesUpdated != 0 {
		t.Fatalf("first import: expected 1 inserted, 0 updated, got %+v", first)
	}
	targetImagePath := filepath.Join(targetDir, "images", "dedup", "portrait.png")
	firstInfo, err := os.Stat(targetImagePath)
	if err != nil {
		t.Fatalf("Stat first copy: %v", err)
	}

	// Force a stable mtime snapshot before the second import so we can
	// assert the dedup short-circuit left the file byte-identical.
	stableMtime := firstInfo.ModTime()
	if err := os.Chtimes(targetImagePath, stableMtime, stableMtime); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	// Second import is the full-duplicate case. Every image already
	// exists in the target DB with identical mutable columns, and the
	// on-disk file has the same byte count. Pre-fix this reported
	// ImagesUpdated=1 and re-wrote the file (changing mtime).
	second, err := backupSvc.ImportSharedBackup(backupPath, targetDir)
	if err != nil {
		t.Fatalf("ImportSharedBackup second: %v", err)
	}
	if second.ImagesInserted != 0 || second.ImagesUpdated != 0 {
		t.Fatalf("second import: expected 0 inserted, 0 updated (full duplicate); got %+v", second)
	}

	secondInfo, err := os.Stat(targetImagePath)
	if err != nil {
		t.Fatalf("Stat second copy: %v", err)
	}
	if secondInfo.Size() != firstInfo.Size() {
		t.Fatalf("dedup short-circuit failed: size changed %d \u2192 %d", firstInfo.Size(), secondInfo.Size())
	}
	// Mtime is a strong signal that copySharedImageFile actually
	// skipped the file write. Use !Equal on the time.Time so
	// monotonic-clock noise doesn't matter \u2014 wall equality is
	// what Chtimes set above.
	if !secondInfo.ModTime().Equal(stableMtime) {
		t.Fatalf("dedup short-circuit failed: mtime changed %v \u2192 %v", stableMtime, secondInfo.ModTime())
	}
}