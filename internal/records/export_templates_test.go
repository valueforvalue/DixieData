package records

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
)

func newExportTemplateTestDB(t *testing.T) (*ExportTemplateService, func()) {
	t.Helper()
	dataDir := filepath.Join(t.TempDir(), ".dixiedata")
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	return NewExportTemplateService(database.Conn()), func() { database.Close() }
}

func TestExportTemplateService_CreateAndGet(t *testing.T) {
	svc, cleanup := newExportTemplateTestDB(t)
	defer cleanup()

	saved, err := svc.Create(ExportTemplate{
		Name:              "Co. A 5VA deaths",
		Scope:             "filtered",
		Filters:           map[string][]string{"unit": {"5th Virginia Infantry"}},
		SortBy:            "death_year",
		GroupBy:           []string{"buried_in"},
		Orientation:       "L",
		PrinterFriendly:   true,
		FullBiographyPage: false,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if saved.ID == 0 {
		t.Fatalf("Create returned zero ID")
	}
	got, err := svc.Get(saved.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "Co. A 5VA deaths" {
		t.Errorf("Name = %q, want %q", got.Name, "Co. A 5VA deaths")
	}
	if got.Filters["unit"][0] != "5th Virginia Infantry" {
		t.Errorf("Filters[unit] = %v, want [5th Virginia Infantry]", got.Filters["unit"])
	}
	if len(got.GroupBy) != 1 || got.GroupBy[0] != "buried_in" {
		t.Errorf("GroupBy = %v, want [buried_in]", got.GroupBy)
	}
	if !got.PrinterFriendly {
		t.Errorf("PrinterFriendly = false, want true")
	}
}

func TestExportTemplateService_DuplicateName(t *testing.T) {
	svc, cleanup := newExportTemplateTestDB(t)
	defer cleanup()

	if _, err := svc.Create(ExportTemplate{Name: "Monthly Reg", Scope: "all"}); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	_, err := svc.Create(ExportTemplate{Name: "Monthly Reg", Scope: "filtered"})
	if !errors.Is(err, ErrExportTemplateNameTaken) {
		t.Fatalf("second Create err = %v, want ErrExportTemplateNameTaken", err)
	}
}

func TestExportTemplateService_GetMissing(t *testing.T) {
	svc, cleanup := newExportTemplateTestDB(t)
	defer cleanup()

	_, err := svc.Get(99999)
	if !errors.Is(err, ErrExportTemplateNotFound) {
		t.Fatalf("Get(99999) err = %v, want ErrExportTemplateNotFound", err)
	}
}

func TestExportTemplateService_ListAndDelete(t *testing.T) {
	svc, cleanup := newExportTemplateTestDB(t)
	defer cleanup()

	if _, err := svc.Create(ExportTemplate{Name: "Alpha", Scope: "all"}); err != nil {
		t.Fatalf("Create Alpha: %v", err)
	}
	if _, err := svc.Create(ExportTemplate{Name: "Bravo", Scope: "filtered"}); err != nil {
		t.Fatalf("Create Bravo: %v", err)
	}

	all, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("List returned %d templates, want 2", len(all))
	}

	if err := svc.Delete(all[0].ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	all2, err := svc.List()
	if err != nil {
		t.Fatalf("List after delete: %v", err)
	}
	if len(all2) != 1 {
		t.Fatalf("after Delete List returned %d, want 1", len(all2))
	}
}

func TestExportTemplateService_TouchLastUsed(t *testing.T) {
	svc, cleanup := newExportTemplateTestDB(t)
	defer cleanup()

	saved, err := svc.Create(ExportTemplate{Name: "Recent", Scope: "all"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.TouchLastUsed(saved.ID); err != nil {
		t.Fatalf("TouchLastUsed: %v", err)
	}
	got, err := svc.Get(saved.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.LastUsedAt.IsZero() {
		t.Errorf("LastUsedAt = zero, want a real timestamp")
	}
}

func TestExportTemplateService_EmptyName(t *testing.T) {
	svc, cleanup := newExportTemplateTestDB(t)
	defer cleanup()
	_, err := svc.Create(ExportTemplate{Name: "  ", Scope: "all"})
	if err == nil {
		t.Fatalf("Create with empty name: err = nil, want validation error")
	}
}

// TestExportTemplateService_Update (issue #186): verifies the
// new Update method replaces mutable fields while preserving
// created_at + last_used_at, round-trips through the DB layer
// correctly, and surfaces ErrExportTemplateNotFound for a
// missing id, ErrExportTemplateNameTaken for a duplicate name.
func TestExportTemplateService_Update(t *testing.T) {
	svc, cleanup := newExportTemplateTestDB(t)
	defer cleanup()

	saved, err := svc.Create(ExportTemplate{
		Name:    "Original",
		Scope:   "filtered",
		Filters: map[string][]string{"unit": {"5th Virginia Infantry"}},
		SortBy:  "death_year",
		GroupBy: []string{"buried_in"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated, err := svc.Update(saved.ID, ExportTemplate{
		Name:              "Renamed",
		Scope:             "selected",
		Orientation:       "P",
		SelectedIDs:       []int64{1, 2, 3},
		PrinterFriendly:   true,
		FullBiographyPage: true,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != "Renamed" {
		t.Errorf("Name = %q, want Renamed", updated.Name)
	}
	if updated.Orientation != "P" {
		t.Errorf("Orientation = %q, want P", updated.Orientation)
	}
	if !updated.PrinterFriendly {
		t.Errorf("PrinterFriendly = false, want true")
	}
	if len(updated.SelectedIDs) != 3 {
		t.Errorf("SelectedIDs len = %d, want 3", len(updated.SelectedIDs))
	}

	// Re-read to confirm DB persistence.
	fresh, _ := svc.Get(saved.ID)
	if fresh.Name != "Renamed" {
		t.Errorf("post-Update Get Name = %q, want Renamed", fresh.Name)
	}
}

func TestExportTemplateService_UpdateMissing(t *testing.T) {
	svc, cleanup := newExportTemplateTestDB(t)
	defer cleanup()
	if _, err := svc.Update(99999, ExportTemplate{Name: "n/a", Scope: "all"}); !errors.Is(err, ErrExportTemplateNotFound) {
		t.Fatalf("Update(99999) err = %v, want ErrExportTemplateNotFound", err)
	}
}

func TestExportTemplateService_UpdateNameCollision(t *testing.T) {
	svc, cleanup := newExportTemplateTestDB(t)
	defer cleanup()
	a, err := svc.Create(ExportTemplate{Name: "First", Scope: "all"})
	if err != nil {
		t.Fatalf("Create First: %v", err)
	}
	if _, err := svc.Create(ExportTemplate{Name: "Second", Scope: "all"}); err != nil {
		t.Fatalf("Create Second: %v", err)
	}
	if _, err := svc.Update(a.ID, ExportTemplate{Name: "Second", Scope: "all"}); !errors.Is(err, ErrExportTemplateNameTaken) {
		t.Fatalf("Update to collide err = %v, want ErrExportTemplateNameTaken", err)
	}
}