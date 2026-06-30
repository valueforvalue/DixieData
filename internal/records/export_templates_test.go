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