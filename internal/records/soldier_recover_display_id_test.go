package records

import (
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
)

// === Issue #416 RED tests ===
//
// RecoverDisplayID mints a fresh DXDID for a soldier whose
// display_id is empty (corrupted by the pre-#376 Update path).
// Once a row has a non-empty id, the method refuses — the
// affordance is for recovery, not for re-generating ids on
// healthy rows.

func blankDisplayIDSoldier(t *testing.T, d *db.DB, id int64, firstName, lastName string) {
	t.Helper()
	// Create normally (auto-generates display_id) then blank it
	// out via raw UPDATE so we simulate the pre-#376 corruption.
	svc := NewSoldierService(d)
	s, err := svc.Create(models.Soldier{
		FirstName: firstName,
		LastName:  lastName,
	})
	if err != nil {
		t.Fatalf("seed Create: %v", err)
	}
	if _, err := d.Conn().Exec(`UPDATE soldiers SET display_id = '' WHERE id = ?`, s.ID); err != nil {
		t.Fatalf("blank display_id: %v", err)
	}
}

func TestSoldierService_RecoverDisplayID_Success(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)
	blankDisplayIDSoldier(t, d, 1, "James", "Gillespie")

	got, err := svc.RecoverDisplayID(1)
	if err != nil {
		t.Fatalf("RecoverDisplayID: %v", err)
	}
	if got == "" {
		t.Fatalf("RecoverDisplayID returned empty string")
	}
	if !strings.HasPrefix(got, "DXD-") {
		t.Errorf("RecoverDisplayID returned %q; expected DXD-XXXXX prefix", got)
	}

	// Verify the row was actually updated in the DB.
	row, err := svc.GetByID(1)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if row.DisplayID != got {
		t.Errorf("DB display_id = %q; want %q", row.DisplayID, got)
	}
}

func TestSoldierService_RecoverDisplayID_NoopIfAlreadyHasID(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)
	// Create a normal soldier (gets an auto-generated display_id).
	s, err := svc.Create(models.Soldier{FirstName: "Robert", LastName: "Lee"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	originalID := s.DisplayID

	_, err = svc.RecoverDisplayID(s.ID)
	if err == nil {
		t.Fatalf("RecoverDisplayID accepted a row that already has a display_id; want typed error")
	}
	if !errors.Is(err, ErrDisplayIDNotEmpty) {
		t.Fatalf("err = %v; want ErrDisplayIDNotEmpty", err)
	}
	// And the DB was not touched.
	row, err := svc.GetByID(s.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if row.DisplayID != originalID {
		t.Errorf("DB display_id changed from %q to %q despite rejection", originalID, row.DisplayID)
	}
}

func TestSoldierService_RecoverDisplayID_RejectsMissingRow(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)
	_, err := svc.RecoverDisplayID(99999)
	if err == nil {
		t.Fatalf("RecoverDisplayID accepted a missing row; want error")
	}
	if !errors.Is(err, sql.ErrNoRows) && !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v; want ErrSoldierNotFound or 'not found' substring", err)
	}
}

func TestSoldierService_RecoverDisplayID_ConcurrentRaceReturnsSameID(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)
	blankDisplayIDSoldier(t, d, 1, "James", "Gillespie")

	// Two goroutines hit RecoverDisplayID simultaneously. The
	// empty-guard in the UPDATE WHERE means at most one UPDATE
	// wins (rows_affected=1). The losing goroutine falls through
	// to the rows_affected=0 branch and re-reads the now-existing
	// id. Both goroutines return the SAME id; the DB has exactly
	// one id (not two different ones).
	const goroutines = 8
	results := make(chan string, goroutines)
	errs := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			id, err := svc.RecoverDisplayID(1)
			results <- id
			errs <- err
		}()
	}
	firstID, firstErr := <-results, <-errs
	if firstErr != nil {
		t.Fatalf("first goroutine err: %v", firstErr)
	}
	if firstID == "" {
		t.Fatalf("first goroutine returned empty id")
	}
	for i := 1; i < goroutines; i++ {
		id, err := <-results, <-errs
		if err != nil {
		// A goroutine that arrived AFTER the winning UPDATE
		// will see the now-non-empty display_id and return
		// ErrDisplayIDNotEmpty. That's also acceptable: the
		// handler maps it to 409 so the UI can re-render the
		// scan results. What we DON'T want is a different
		// minted id from any goroutine.
			if !errors.Is(err, ErrDisplayIDNotEmpty) {
				t.Errorf("goroutine %d: err = %v; want nil or ErrDisplayIDNotEmpty", i, err)
			}
		} else if id != firstID {
			t.Errorf("goroutine %d returned id %q; want %q (idempotent)", i, id, firstID)
		}
	}
	// DB has exactly one id (not two different ones).
	row, _ := svc.GetByID(1)
	if row.DisplayID != firstID {
		t.Errorf("DB display_id = %q; want %q", row.DisplayID, firstID)
	}
}
