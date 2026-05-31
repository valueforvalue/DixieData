package db

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/valueforvalue/DixieData/internal/appdata"
)

func TestSaveScratchpadStoresCanonicalContent(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	if _, err := d.Conn().Exec(`INSERT INTO soldiers (display_id, sync_id, first_name, last_name) VALUES (?, ?, ?, ?)`,
		"DXD-00001", "sync-1", "Thomas", "Carter"); err != nil {
		t.Fatalf("insert soldier: %v", err)
	}

	if err := d.SaveScratchpad("DXD-00001", "SQLite-first scratch pad text"); err != nil {
		t.Fatalf("SaveScratchpad: %v", err)
	}

	content, updatedAt, err := d.Scratchpad("DXD-00001")
	if err != nil {
		t.Fatalf("Scratchpad: %v", err)
	}
	if content != "SQLite-first scratch pad text" {
		t.Fatalf("content=%q", content)
	}
	if updatedAt.IsZero() {
		t.Fatalf("expected scratch pad updated_at to be populated")
	}
}

func TestImportLegacyScratchpadFilesMigratesTextIntoSQLite(t *testing.T) {
	dataDir := t.TempDir()
	d, err := Open(dataDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	if _, err := d.Conn().Exec(`INSERT INTO soldiers (display_id, sync_id, first_name, last_name) VALUES (?, ?, ?, ?)`,
		"DXD-00077", "sync-77", "Legacy", "Scratchpad"); err != nil {
		t.Fatalf("insert soldier: %v", err)
	}

	textPath, _ := appdata.ScratchpadPaths(dataDir, "DXD-00077")
	if err := os.MkdirAll(filepath.Dir(textPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(textPath, []byte("Imported from legacy bridge file."), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := d.SetSystemConfig(scratchpadLegacyImportConfigKey, ""); err != nil {
		t.Fatalf("SetSystemConfig: %v", err)
	}

	if err := d.ImportLegacyScratchpadFiles(); err != nil {
		t.Fatalf("ImportLegacyScratchpadFiles: %v", err)
	}

	content, _, err := d.Scratchpad("DXD-00077")
	if err != nil {
		t.Fatalf("Scratchpad: %v", err)
	}
	if content != "Imported from legacy bridge file." {
		t.Fatalf("content=%q", content)
	}
}
