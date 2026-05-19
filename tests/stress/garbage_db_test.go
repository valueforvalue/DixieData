package stress

import (
	"testing"

	"github.com/valueforvalue/DixieData/internal/records"
)

func TestGenerateGarbageDatabase(t *testing.T) {
	dataDir := t.TempDir()
	summary, err := GenerateGarbageDatabase(dataDir, 24)
	if err != nil {
		t.Fatalf("GenerateGarbageDatabase: %v", err)
	}
	if summary.SoldiersInserted != 24 || summary.RecordsInserted != 24 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	if summary.MaxPayloadBytes < 10000 {
		t.Fatalf("max payload too small: %#v", summary)
	}

	database, err := openExistingStressDB(dataDir)
	if err != nil {
		t.Fatalf("openExistingStressDB: %v", err)
	}
	defer database.Close()
	soldierSvc := records.NewSoldierService(database)

	results, total, err := soldierSvc.SearchPage("DROP TABLE soldiers", 1, 10)
	if err != nil {
		t.Fatalf("SearchPage: %v", err)
	}
	if total == 0 || len(results) == 0 {
		t.Fatal("expected injection-like payload to remain searchable")
	}

	first, err := soldierSvc.GetByID(1)
	if err != nil {
		t.Fatalf("GetByID first: %v", err)
	}
	second, err := soldierSvc.GetByID(2)
	if err != nil {
		t.Fatalf("GetByID second: %v", err)
	}
	if first.SpouseSoldierID != second.ID || second.SpouseSoldierID != first.ID {
		t.Fatalf("expected spouse loop, got first=%d second=%d", first.SpouseSoldierID, second.SpouseSoldierID)
	}
}
