package services

import (
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
)

func newTestDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestSoldierService_CreateWithCSAID(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	s, err := svc.Create(models.Soldier{
		FirstName: "Robert",
		LastName:  "Lee",
		Rank:      "General",
		Unit:      "Army of Northern Virginia",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if s.DisplayID != "CSA-000001" {
		t.Errorf("expected CSA-000001, got %s", s.DisplayID)
	}
	if !s.IsGenerated {
		t.Error("expected IsGenerated=true")
	}
}

func TestSoldierService_CreateWithPensionID(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	s, err := svc.Create(models.Soldier{
		DisplayID: "PENSION-9999",
		FirstName: "Stonewall",
		LastName:  "Jackson",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if s.DisplayID != "PENSION-9999" {
		t.Errorf("expected PENSION-9999, got %s", s.DisplayID)
	}
	if s.IsGenerated {
		t.Error("expected IsGenerated=false for explicit pension ID")
	}
}

func TestSoldierService_GetByID(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	created, err := svc.Create(models.Soldier{
		FirstName: "James",
		LastName:  "Longstreet",
		Rank:      "Lieutenant General",
		Unit:      "First Corps",
		DeathYear: 1904, DeathMonth: 1, DeathDay: 2,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := svc.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.FirstName != "James" || got.LastName != "Longstreet" {
		t.Errorf("got %s %s, want James Longstreet", got.FirstName, got.LastName)
	}
	if got.DeathMonth != 1 || got.DeathDay != 2 {
		t.Errorf("got death %d/%d, want 1/2", got.DeathMonth, got.DeathDay)
	}
}

func TestSoldierService_Update(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	created, err := svc.Create(models.Soldier{FirstName: "Jubal", LastName: "Early"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	created.Notes = "Updated note"
	if err := svc.Update(*created); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := svc.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID after update: %v", err)
	}
	if got.Notes != "Updated note" {
		t.Errorf("got notes %q, want 'Updated note'", got.Notes)
	}
}

func TestSoldierService_Delete(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	created, err := svc.Create(models.Soldier{FirstName: "P.G.T.", LastName: "Beauregard"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := svc.Delete(created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = svc.GetByID(created.ID)
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestSoldierService_List(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	for i := 0; i < 5; i++ {
		_, err := svc.Create(models.Soldier{
			FirstName: "Soldier",
			LastName:  "Test",
		})
		if err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	soldiers, total, err := svc.List(1, 3)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 5 {
		t.Errorf("total=%d, want 5", total)
	}
	if len(soldiers) != 3 {
		t.Errorf("page size=%d, want 3", len(soldiers))
	}
}

func TestSoldierService_Search(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	_, err := svc.Create(models.Soldier{
		FirstName: "Nathan",
		LastName:  "Forrest",
		Unit:      "Forrest's Cavalry",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, err = svc.Create(models.Soldier{
		FirstName: "Joseph",
		LastName:  "Johnston",
		Unit:      "Army of Tennessee",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	results, err := svc.Search("Forrest")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Search returned %d results, want 1", len(results))
	}
	if results[0].LastName != "Forrest" {
		t.Errorf("got %s, want Forrest", results[0].LastName)
	}
}
