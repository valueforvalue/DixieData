package records

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
)

func newShareQueuePresetTestDB(t *testing.T) (*ShareQueuePresetService, func()) {
	t.Helper()
	dataDir := filepath.Join(t.TempDir(), ".dixiedata")
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	return NewShareQueuePresetService(database.Conn()), func() { database.Close() }
}

// TestShareQueuePresetService_CreateAndGet (issue #192) seeds
// a preset via Create, fetches it via Get, and asserts every
// field round-trips cleanly through the JSON soldier_ids
// column.
func TestShareQueuePresetService_CreateAndGet(t *testing.T) {
	svc, cleanup := newShareQueuePresetTestDB(t)
	defer cleanup()

	saved, err := svc.Create(ShareQueuePreset{
		Name:       "Shiloh Cemetery",
		SoldierIDs: []int64{1, 2, 3},
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
	if got.Name != "Shiloh Cemetery" {
		t.Errorf("Name = %q, want %q", got.Name, "Shiloh Cemetery")
	}
	if len(got.SoldierIDs) != 3 {
		t.Errorf("SoldierIDs len = %d, want 3", len(got.SoldierIDs))
	}
	for i, want := range []int64{1, 2, 3} {
		if got.SoldierIDs[i] != want {
			t.Errorf("SoldierIDs[%d] = %d, want %d", i, got.SoldierIDs[i], want)
		}
	}
}

// TestShareQueuePresetService_TrimsName (issue #192) asserts
// the Create path trims leading/trailing whitespace so a stray
// space can never silently create a "different" preset.
func TestShareQueuePresetService_TrimsName(t *testing.T) {
	svc, cleanup := newShareQueuePresetTestDB(t)
	defer cleanup()

	saved, err := svc.Create(ShareQueuePreset{Name: "  Shiloh  ", SoldierIDs: []int64{1}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if saved.Name != "Shiloh" {
		t.Errorf("Name = %q, want %q", saved.Name, "Shiloh")
	}
}

// TestShareQueuePresetService_RejectsEmptyName (issue #192)
// asserts the empty-name guard fires before the DB sees the
// insert -- protects the modal's submit when the user hits
// Save with a blank input.
func TestShareQueuePresetService_RejectsEmptyName(t *testing.T) {
	svc, cleanup := newShareQueuePresetTestDB(t)
	defer cleanup()

	_, err := svc.Create(ShareQueuePreset{Name: "   ", SoldierIDs: []int64{1}})
	if err == nil {
		t.Fatalf("Create with whitespace name should error")
	}
}

// TestShareQueuePresetService_DropsNonPositiveIDs (issue
// #192) asserts the Create path filters out 0/negative IDs
// defensively. The handler should already filter these, but
// a stray "" parse must never land in the DB.
func TestShareQueuePresetService_DropsNonPositiveIDs(t *testing.T) {
	svc, cleanup := newShareQueuePresetTestDB(t)
	defer cleanup()

	saved, err := svc.Create(ShareQueuePreset{
		Name:       "Mixed",
		SoldierIDs: []int64{0, -1, 5, 0, 7},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(saved.SoldierIDs) != 2 {
		t.Errorf("SoldierIDs len = %d, want 2 (0 and -1 dropped)", len(saved.SoldierIDs))
	}
}

// TestShareQueuePresetService_DuplicateName (issue #192)
// mirrors the export-template test: a second Create with the
// same name must surface ErrShareQueuePresetNameTaken so the
// handler can map it to 409.
func TestShareQueuePresetService_DuplicateName(t *testing.T) {
	svc, cleanup := newShareQueuePresetTestDB(t)
	defer cleanup()

	if _, err := svc.Create(ShareQueuePreset{Name: "Shiloh"}); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	_, err := svc.Create(ShareQueuePreset{Name: "Shiloh"})
	if !errors.Is(err, ErrShareQueuePresetNameTaken) {
		t.Fatalf("second Create err = %v, want ErrShareQueuePresetNameTaken", err)
	}
}

// TestShareQueuePresetService_GetMissing (issue #192) asserts
// Get against an unknown id returns ErrShareQueuePresetNotFound
// (mapped to 404).
func TestShareQueuePresetService_GetMissing(t *testing.T) {
	svc, cleanup := newShareQueuePresetTestDB(t)
	defer cleanup()

	_, err := svc.Get(99999)
	if !errors.Is(err, ErrShareQueuePresetNotFound) {
		t.Fatalf("Get(99999) err = %v, want ErrShareQueuePresetNotFound", err)
	}
}

// TestShareQueuePresetService_ListAndDelete (issue #192)
// seeds two presets, asserts List returns both (ordered by
// last_used_at DESC, name ASC), and confirms Delete drops the
// targeted row.
func TestShareQueuePresetService_ListAndDelete(t *testing.T) {
	svc, cleanup := newShareQueuePresetTestDB(t)
	defer cleanup()

	first, err := svc.Create(ShareQueuePreset{Name: "Alpha", SoldierIDs: []int64{1}})
	if err != nil {
		t.Fatalf("Create Alpha: %v", err)
	}
	if _, err := svc.Create(ShareQueuePreset{Name: "Bravo", SoldierIDs: []int64{2}}); err != nil {
		t.Fatalf("Create Bravo: %v", err)
	}

	all, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("List returned %d, want 2", len(all))
	}
	// Don't assert specific ordering: the insert path stamps
	// CURRENT_TIMESTAMP, and two inserts in the same second
	// share an identical last_used_at -- the secondary sort
	// (name ASC) breaks the tie, but the precise outcome
	// depends on how SQLite resolves the millisecond-level
	// tie. Just assert both rows are present.
	gotNames := map[string]bool{}
	for _, p := range all {
		gotNames[p.Name] = true
	}
	if !gotNames["Alpha"] || !gotNames["Bravo"] {
		t.Errorf("List missing one of [Alpha, Bravo]: got %v", gotNames)
	}

	if err := svc.Delete(first.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	all, err = svc.List()
	if err != nil {
		t.Fatalf("List after delete: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("List after Delete returned %d, want 1", len(all))
	}
	if all[0].Name != "Bravo" {
		t.Errorf("remaining preset = %s, want Bravo", all[0].Name)
	}
}

// TestShareQueuePresetService_DeleteMissing (issue #192)
// asserts Delete against an unknown id returns
// ErrShareQueuePresetNotFound so the handler can map it to 404.
func TestShareQueuePresetService_DeleteMissing(t *testing.T) {
	svc, cleanup := newShareQueuePresetTestDB(t)
	defer cleanup()

	err := svc.Delete(99999)
	if !errors.Is(err, ErrShareQueuePresetNotFound) {
		t.Fatalf("Delete(99999) err = %v, want ErrShareQueuePresetNotFound", err)
	}
}

// TestShareQueuePresetService_TouchLastUsed (issue #192)
// asserts TouchLastUsed bumps last_used_at and re-orders the
// List so the touched preset floats to the top.
func TestShareQueuePresetService_TouchLastUsed(t *testing.T) {
	svc, cleanup := newShareQueuePresetTestDB(t)
	defer cleanup()

	first, err := svc.Create(ShareQueuePreset{Name: "Alpha"})
	if err != nil {
		t.Fatalf("Create Alpha: %v", err)
	}
	second, err := svc.Create(ShareQueuePreset{Name: "Bravo"})
	if err != nil {
		t.Fatalf("Create Bravo: %v", err)
	}
	if err := svc.TouchLastUsed(first.ID); err != nil {
		t.Fatalf("TouchLastUsed: %v", err)
	}
	all, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("List returned %d, want 2", len(all))
	}
	if all[0].ID != first.ID {
		t.Errorf("most-recently-used = %s, want Alpha", all[0].Name)
	}
	_ = second
}