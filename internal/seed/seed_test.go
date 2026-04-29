package seed

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
)

func TestGenerateCreatesDatabaseRecordsAndImages(t *testing.T) {
	dataDir := t.TempDir()

	summary, err := Generate(Options{
		DataDir:  dataDir,
		Soldiers: 12,
		Seed:     42,
		Reset:    true,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if summary.Soldiers != 12 {
		t.Fatalf("soldiers=%d want 12", summary.Soldiers)
	}
	if summary.Records < 12 {
		t.Fatalf("records=%d want at least 12", summary.Records)
	}
	if summary.Images < 12 {
		t.Fatalf("images=%d want at least 12", summary.Images)
	}

	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer database.Close()

	assertCount(t, database, "soldiers", 12)
	assertCount(t, database, "records", summary.Records)
	assertCount(t, database, "images", summary.Images)

	imageFiles := 0
	if err := filepath.WalkDir(filepath.Join(dataDir, "images"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			imageFiles++
		}
		return nil
	}); err != nil {
		t.Fatalf("WalkDir: %v", err)
	}
	if imageFiles != summary.Images {
		t.Fatalf("image files=%d want %d", imageFiles, summary.Images)
	}

	var storedPath string
	if err := database.Conn().QueryRow("SELECT file_path FROM images LIMIT 1").Scan(&storedPath); err != nil {
		t.Fatalf("select image path: %v", err)
	}
	if filepath.IsAbs(storedPath) {
		t.Fatalf("image path should be stored relative, got %q", storedPath)
	}
}

func assertCount(t *testing.T, database *db.DB, table string, want int) {
	t.Helper()

	var got int
	if err := database.Conn().QueryRow("SELECT COUNT(*) FROM " + table).Scan(&got); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if got != want {
		t.Fatalf("%s count=%d want %d", table, got, want)
	}
}
