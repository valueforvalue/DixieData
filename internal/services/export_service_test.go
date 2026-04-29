package services

import (
	"encoding/csv"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

func TestExportService_ExportJSON(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	exportSvc := NewExportService(d, soldierSvc)

	_, _ = soldierSvc.Create(models.Soldier{FirstName: "Robert", LastName: "Lee", Rank: "General"})
	_, _ = soldierSvc.Create(models.Soldier{FirstName: "Stonewall", LastName: "Jackson", DisplayID: "PENSION-001"})

	outPath := filepath.Join(t.TempDir(), "export.json")
	if err := exportSvc.ExportJSON(outPath); err != nil {
		t.Fatalf("ExportJSON: %v", err)
	}

	f, err := os.Open(outPath)
	if err != nil {
		t.Fatalf("open exported file: %v", err)
	}
	defer f.Close()

	data, _ := io.ReadAll(f)
	var soldiers []models.Soldier
	if err := json.Unmarshal(data, &soldiers); err != nil {
		t.Fatalf("unmarshal JSON: %v", err)
	}
	if len(soldiers) != 2 {
		t.Errorf("exported %d soldiers, want 2", len(soldiers))
	}
}

func TestExportService_ExportCSV(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	exportSvc := NewExportService(d, soldierSvc)

	_, _ = soldierSvc.Create(models.Soldier{FirstName: "P.G.T.", LastName: "Beauregard", Unit: "Army of the Potomac (CSA)"})
	_, _ = soldierSvc.Create(models.Soldier{FirstName: "Braxton", LastName: "Bragg", Unit: "Army of Tennessee"})

	outPath := filepath.Join(t.TempDir(), "export.csv")
	if err := exportSvc.ExportCSV(outPath); err != nil {
		t.Fatalf("ExportCSV: %v", err)
	}

	f, err := os.Open(outPath)
	if err != nil {
		t.Fatalf("open exported file: %v", err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("read CSV: %v", err)
	}

	// Header + 2 data rows
	if len(records) != 3 {
		t.Errorf("CSV has %d rows (incl header), want 3", len(records))
	}

	header := records[0]
	if len(header) < 5 {
		t.Errorf("CSV header too short: %v", header)
	}
	// Verify header contains expected columns
	expected := map[string]bool{
		"id": true, "display_id": true, "first_name": true,
		"last_name": true, "death_year": true,
	}
	for _, col := range header {
		delete(expected, col)
	}
	if len(expected) > 0 {
		t.Errorf("CSV missing columns: %v", expected)
	}
}
