// person_record_repo_parity_test.go — issue #613 slice 1
// regression net.
//
// Pins the contract that the new repository-backed
// SoldierService.GetByID + List paths return identical results
// to the legacy inline-SQL paths on the same fixture.
//
// Strategy:
//
//   1. Seed an in-memory *db.DB (full schema via db.Open +
//      t.TempDir) with N soldiers using NewSoldierService.Create
//      (the same create path every consumer uses).
//
//   2. Call GetByID + List through the NEW (slice 1)
//      SoldierService — which internally delegates to
//      PersonRecordRepo.
//
//   3. Snapshot the returned models.Soldier slice.
//
// The legacy inline-SQL path is gone in slice 1 (the service
// delegates entirely to the repo). This parity test therefore
// asserts the new repo-backed path produces the same shape the
// previous soldier_service_test.go suite expected: same fields,
// same ordering, same total count. The existing test suite
// (TestSoldierService_GetByID*, TestSoldierService_List*) is
// the regression net for the legacy behavior; if the slice-1
// delegation breaks anything, those tests fail.
//
// This file adds 2 NEW tests that pin slice-1-specific
// invariants:
//
//   - The repo-backed path returns the same paginated count
//     the legacy code returned (10/page by default).
//
//   - GetByID via the repo returns the same soldier the legacy
//     Create path stored (round-trip identity).
package records

import (
	"context"
	"testing"

	sqliterepo "github.com/valueforvalue/DixieData/internal/db/repo/sqlite"
	"github.com/valueforvalue/DixieData/internal/models"
)

// TestPersonRecordRepo_Parity_GetByID_RoundTrip seeds a soldier
// via the full Create path (which uses every SoldierService
// field including audit timestamps + Display ID generation),
// then reads it back via the slice-1 repo-backed GetByID. The
// returned models.Soldier must equal the original on every
// field the slice-1 column list covers.
func TestPersonRecordRepo_Parity_GetByID_RoundTrip(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	created, err := svc.Create(models.Soldier{
		FirstName: "Robert",
		LastName:  "Lee",
		Rank:      "General",
		Unit:      "Army of Northern Virginia",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := svc.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID = %d, want %d", got.ID, created.ID)
	}
	if got.DisplayID != created.DisplayID {
		t.Errorf("DisplayID = %q, want %q", got.DisplayID, created.DisplayID)
	}
	if got.FirstName != created.FirstName {
		t.Errorf("FirstName = %q, want %q", got.FirstName, created.FirstName)
	}
	if got.LastName != created.LastName {
		t.Errorf("LastName = %q, want %q", got.LastName, created.LastName)
	}
	if got.Rank != created.Rank {
		t.Errorf("Rank = %q, want %q", got.Rank, created.Rank)
	}
	if got.Unit != created.Unit {
		t.Errorf("Unit = %q, want %q", got.Unit, created.Unit)
	}
}

// TestPersonRecordRepo_Parity_List_Pagination seeds 25 soldiers,
// then asserts the repo-backed List returns the expected total
// + the expected page-1 size (10 rows).
func TestPersonRecordRepo_Parity_List_Pagination(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	for i := 0; i < 25; i++ {
		_, err := svc.Create(models.Soldier{
			FirstName: "First" + itoaSlice1(i),
			LastName:  "Last" + itoaSlice1(i),
		})
		if err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	soldiers, total, err := svc.List(1, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 25 {
		t.Errorf("total = %d, want 25", total)
	}
	if len(soldiers) != 10 {
		t.Errorf("page 1 size = %d, want 10", len(soldiers))
	}
}

// TestPersonRecordRepo_Direct_List ensures the repo interface
// can be used directly (without going through SoldierService).
// Pins that the seam is usable from any caller — future slices
// can plug other consumers (analytics, exports) into the same
// interface without going through the domain service layer.
func TestPersonRecordRepo_Direct_List(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	for i := 0; i < 5; i++ {
		if _, err := svc.Create(models.Soldier{
			FirstName: "First" + itoaSlice1(i),
			LastName:  "Last" + itoaSlice1(i),
		}); err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	r := sqliterepo.NewPersonRecordRepo(d)
	rows, total, err := r.List(context.Background(), 1, 100)
	if err != nil {
		t.Fatalf("repo.List: %v", err)
	}
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if rows == nil {
		t.Fatalf("rows = nil")
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		count++
	}
	if count != 5 {
		t.Errorf("rows iterated = %d, want 5", count)
	}
	if err := rows.Err(); err != nil {
		t.Errorf("rows.Err: %v", err)
	}
}

// itoaSlice1 is a tiny int-to-string helper for test fixture
// building. Inlined here rather than importing strconv so the
// test reads at one glance.
func itoaSlice1(n int) string {
	if n == 0 {
		return "0"
	}
	digits := make([]byte, 0, 4)
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}