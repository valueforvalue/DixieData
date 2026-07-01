package archive

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/records"
)

// newSubsetTestEnv spins up a temp DB + SoldierService +
// BackupService the subset tests can drive directly without
// booting the full appshell harness. Mirrors the constructor
// pair used in backup_service_e2e_test.go so the layering is
// consistent.
func newSubsetTestEnv(t *testing.T) (*db.DB, *records.SoldierService, string) {
	t.Helper()
	dataDir := filepath.Join(t.TempDir(), ".dixiedata")
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	soldier := records.NewSoldierService(database)
	return database, soldier, dataDir
}

func TestExportSharedSubset_EmptyIDsRejected(t *testing.T) {
	database, soldier, dataDir := newSubsetTestEnv(t)
	b := NewBackupService(database, soldier)
	out := filepath.Join(t.TempDir(), "subset-empty.ddshare")
	if _, err := b.ExportSharedSubset(out, dataDir, nil); err == nil {
		t.Errorf("ExportSharedSubset(nil) should error")
	}
}

// TestExportSharedSubset_Roundtrip (issue #182) seeds 500
// soldiers, exports 5 of them via ExportSharedSubset, opens the
// resulting zip, asserts the data/soldiers.json payload contains
// exactly the requested rows in caller order, and confirms the
// zip embeds manifest.json + the soldiers.json.
func TestExportSharedSubset_Roundtrip(t *testing.T) {
	database, soldier, dataDir := newSubsetTestEnv(t)
	_ = database
	b := NewBackupService(database, soldier)

	// Seed more than the subset size so the test confirms the
	// service restricted the payload to the supplied ids, not
	// the entire archive.
	const seedCount = 500
	ids := make([]int64, 0, 5)
	for i := 0; i < seedCount; i++ {
		s, err := soldier.Create(models.Soldier{
			DisplayID: fmt.Sprintf("SQ-%04d", i),
			FirstName: fmt.Sprintf("Subset-%d", i),
			LastName:  "Archive",
			Unit:      "1st Virginia Infantry",
		})
		if err != nil {
			t.Fatalf("seed Create %d: %v", i, err)
		}
		if i < 5 {
			ids = append(ids, s.ID)
		}
	}

	out := filepath.Join(t.TempDir(), "subset.ddshare")
	manifest, err := b.ExportSharedSubset(out, dataDir, ids)
	if err != nil {
		t.Fatalf("ExportSharedSubset: %v", err)
	}
	if manifest.Soldiers != 5 {
		t.Errorf("manifest.Soldiers = %d, want 5", manifest.Soldiers)
	}

	// Open the zip and validate the payload.
	r, err := zip.OpenReader(out)
	if err != nil {
		t.Fatalf("zip.OpenReader: %v", err)
	}
	defer r.Close()

	var manifestFile, soldiersFile *zip.File
	for _, f := range r.File {
		switch f.Name {
		case "manifest.json":
			manifestFile = f
		case "data/soldiers.json":
			soldiersFile = f
		}
	}
	if manifestFile == nil {
		t.Fatalf("subset zip missing manifest.json")
	}
	if soldiersFile == nil {
		t.Fatalf("subset zip missing data/soldiers.json")
	}

	var payload []models.Soldier
	rc, err := soldiersFile.Open()
	if err != nil {
		t.Fatalf("open soldiers.json: %v", err)
	}
	body, _ := io.ReadAll(rc)
	rc.Close()
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal soldiers.json: %v", err)
	}
	if len(payload) != 5 {
		t.Errorf("payload len = %d, want 5 (subset over a 500-row archive)", len(payload))
	}
	for i := range payload {
		if payload[i].ID != ids[i] {
			t.Errorf("payload[%d].ID = %d, want %d (caller order)", i, payload[i].ID, ids[i])
		}
	}

	// Cleanup — remove the temp archive file (t.TempDir() handles
	// the dir).
	_ = os.Remove(out)
}
