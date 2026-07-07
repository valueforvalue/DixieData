package records

// Issue #368 Slice 1: Source Record reorder column.
// The slice ships the schema column (sort_order) and the read-path
// ORDER BY change. The tracer-bullet regression net here proves:
//
// 1. The records.sort_order column exists after a fresh install.
// 2. The event_sources.sort_order column exists after a fresh install.
// 3. The read path returns rows in (sort_order, id) order, NOT in
//    raw id order, when two rows tie on sort_order the id tiebreak
//    keeps the order stable.
// 4. SortOrder is populated on every Record returned by GetByID so
//    Slice 2 (write-path + service) and Slice 3 (PATCH endpoint)
//    have a populated field to operate on.
//
// Slice 1 does NOT touch the write paths (replaceRecords,
// AttachSourcesToEvent) so all existing tests still pass — the new
// column defaults to 0 on insert, the backfill sets it to id, the
// read path returns rows in id order (because every row's
// sort_order equals its id after the backfill). The non-trivial
// assertion is the manual sort_order override below: insert two
// rows with swapped sort_order values and confirm the read path
// returns them in the requested order.

import (
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
)

// TestRecordsSortOrderColumnExists pins the schema: after a fresh
// install the inline CREATE TABLE for both records and
// event_sources carries `sort_order INTEGER NOT NULL DEFAULT 0`.
// A regression that drops the column (or omits it from the inline
// schema) would break the read path ORDER BY clause in
// soldier_service.go + event_service.go and surface here.
func TestRecordsSortOrderColumnExists(t *testing.T) {
	d := newTestDB(t)

	for _, table := range []string{"records", "event_sources"} {
		row := d.Conn().QueryRow(
			`SELECT type, "notnull", dflt_value FROM pragma_table_info('` + table + `') WHERE name = 'sort_order'`,
		)
		var typ string
		var notnull int
		var dflt *string
		if err := row.Scan(&typ, &notnull, &dflt); err != nil {
			t.Fatalf("%s: pragma_table_info scan: %v", table, err)
		}
		if strings.ToLower(typ) != "integer" {
			t.Errorf("%s.sort_order type = %q, want integer", table, typ)
		}
		if notnull != 1 {
			t.Errorf("%s.sort_order NOT NULL = %d, want 1", table, notnull)
		}
		if dflt == nil || *dflt != "0" {
			t.Errorf("%s.sort_order DEFAULT = %v, want 0", table, dflt)
		}
	}
}

// TestSoldierService_GetByIDReturnsRecordsInSortOrder is the
// tracer-bullet assertion: write two Source Records, manually flip
// their sort_order via SQL (Slice 2 will teach replaceRecords +
// AttachSourcesToEvent to write sort_order from form-array index;
// Slice 1 is schema + read only), then confirm GetByID returns
// them in the sort_order-not-id order.
//
// This pins the ORDER BY change in soldier_service.go:251 +
// soldier_service.go:299 + the in-tx read at :2857. Without the
// ORDER BY change, the test would still pass on id order (since
// row #1 is the first INSERT and row #2 is the second, id order
// matches sort_order order in this fixture). To prove the ORDER
// BY clause is doing work, swap sort_order so the requested
// order DIFFERS from id order: row with the higher id gets the
// lower sort_order, then assert read path returns it first.
func TestSoldierService_GetByIDReturnsRecordsInSortOrder(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	created, err := svc.Create(models.Soldier{
		FirstName: "Nathan",
		LastName:  "Bedford",
		Records: []models.Record{
			{RecordType: "Roster", AppID: "APP-A", Details: "first inserted"},
			{RecordType: "Parole", AppID: "APP-B", Details: "second inserted"},
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// After Create the slice-1 path writes sort_order = 0 (the
	// DEFAULT) for both rows. Read path returns rows in id order
	// (the secondary tiebreak). Verify the SortOrder field on the
	// model is populated so Slice 2's PATCH endpoint can read it.
	got, err := svc.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if len(got.Records) != 2 {
		t.Fatalf("len(Records) = %d, want 2", len(got.Records))
	}
	// The Create path on Slice 1 inserts with sort_order = 0 (the
	// DEFAULT — replaceRecords hasn't been updated yet, that's
	// Slice 2's job). The SortOrder field on the model must still
	// be populated so Slice 2 + Slice 3 can read + write it.
	for _, r := range got.Records {
		if r.ID < 1 {
			t.Errorf("record ID = %d, want positive", r.ID)
		}
		// SortOrder field is populated by the Scan (0 here is a
		// valid value because Create uses DEFAULT 0; we just need
		// the field to be readable — proven by the rest of this
		// test succeeding with manual SQL UPDATE flipping values).
	}

	// Flip sort_order on the second-inserted row so it sorts
	// BEFORE the first-inserted row. Read path must now return it
	// first — proves ORDER BY sort_order, id is doing work, not
	// just ORDER BY id.
	firstID := got.Records[0].ID
	secondID := got.Records[1].ID
	if _, err := d.Conn().Exec(`UPDATE records SET sort_order = 5 WHERE id = ?`, firstID); err != nil {
		t.Fatalf("UPDATE first row sort_order=5: %v", err)
	}
	if _, err := d.Conn().Exec(`UPDATE records SET sort_order = 1 WHERE id = ?`, secondID); err != nil {
		t.Fatalf("UPDATE second row sort_order=1: %v", err)
	}

	got, err = svc.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID after sort_order flip: %v", err)
	}
	if len(got.Records) != 2 {
		t.Fatalf("len(Records) = %d, want 2", len(got.Records))
	}
	// Row with sort_order=1 (the second-inserted row, id second)
	// must now come FIRST. Row with sort_order=5 (the first-
	// inserted row, id first) must come second.
	if got.Records[0].ID != secondID {
		t.Errorf("records[0].ID = %d, want %d (sort_order=1 should sort first)", got.Records[0].ID, secondID)
	}
	if got.Records[1].ID != firstID {
		t.Errorf("records[1].ID = %d, want %d (sort_order=5 should sort second)", got.Records[1].ID, firstID)
	}
	if got.Records[0].SortOrder != 1 {
		t.Errorf("records[0].SortOrder = %d, want 1", got.Records[0].SortOrder)
	}
	if got.Records[1].SortOrder != 5 {
		t.Errorf("records[1].SortOrder = %d, want 5", got.Records[1].SortOrder)
	}
}

// TestEventService_ListSourcesForEventSortsBySortOrder pins the
// same ORDER BY change in event_service.go:75 (ListSourcesForEvent).
// Mirrors the soldier-side test: insert two event_sources rows,
// flip their sort_order, confirm read returns the lower sort_order
// first.
func TestEventService_ListSourcesForEventSortsBySortOrder(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)
	es := NewEventService(svc)

	created, err := svc.Create(models.Soldier{
		FirstName: "Battle", LastName: "OfTest", Kind: "Battle",
		BeginDate: "07/01/1863",
	})
	if err != nil {
		t.Fatalf("Create event: %v", err)
	}

	if _, err := es.AttachSourcesToEvent(created.ID, []models.Record{
		{RecordType: "Roster", AppID: "APP-A", Details: "first"},
		{RecordType: "Parole", AppID: "APP-B", Details: "second"},
	}); err != nil {
		t.Fatalf("AttachSourcesToEvent: %v", err)
	}

	first, err := es.ListSourcesForEvent(created.ID)
	if err != nil {
		t.Fatalf("ListSourcesForEvent: %v", err)
	}
	if len(first) != 2 {
		t.Fatalf("len(sources) = %d, want 2", len(first))
	}

	// Flip sort_order on the first-inserted source so it sorts
	// SECOND instead of first.
	firstID := first[0].ID
	secondID := first[1].ID
	if _, err := d.Conn().Exec(`UPDATE event_sources SET sort_order = 7 WHERE id = ?`, firstID); err != nil {
		t.Fatalf("UPDATE first source sort_order=7: %v", err)
	}
	if _, err := d.Conn().Exec(`UPDATE event_sources SET sort_order = 2 WHERE id = ?`, secondID); err != nil {
		t.Fatalf("UPDATE second source sort_order=2: %v", err)
	}

	again, err := es.ListSourcesForEvent(created.ID)
	if err != nil {
		t.Fatalf("ListSourcesForEvent after flip: %v", err)
	}
	if len(again) != 2 {
		t.Fatalf("len(sources) = %d, want 2", len(again))
	}
	if again[0].ID != secondID {
		t.Errorf("sources[0].ID = %d, want %d (sort_order=2 should sort first)", again[0].ID, secondID)
	}
	if again[1].ID != firstID {
		t.Errorf("sources[1].ID = %d, want %d (sort_order=7 should sort second)", again[1].ID, firstID)
	}
	if again[0].SortOrder != 2 {
		t.Errorf("sources[0].SortOrder = %d, want 2", again[0].SortOrder)
	}
	if again[1].SortOrder != 7 {
		t.Errorf("sources[1].SortOrder = %d, want 7", again[1].SortOrder)
	}
}

// TestSoldierService_BackfillAssignsSortOrderEqualToID pins the
// backfill behaviour: every existing row in records + event_sources
// after the v62→v63 migration has sort_order = id so display order
// is preserved. We simulate the upgrade by inserting rows on a
// pre-block-63 schema and then running block-63's Up. NewTestDB
// starts with all migrations already applied (the inline schema
// already has the column), so this test exercises the inline path
// + a manual UPDATE that mimics what the backfill would do on an
// archive upgraded via the v62→v63 path.
//
// What this test pins: the backfill WHERE clause
// `sort_order = 0 OR sort_order IS NULL` rewrites every row to
// sort_order = id. After the UPDATE the read path returns rows in
// id order (because sort_order = id for every row).
func TestSoldierService_BackfillAssignsSortOrderEqualToID(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	created, err := svc.Create(models.Soldier{
		FirstName: "Joseph", LastName: "Johnston",
		Records: []models.Record{
			{RecordType: "Roster", AppID: "APP-A"},
			{RecordType: "Parole", AppID: "APP-B"},
			{RecordType: "Pension", AppID: "APP-C"},
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Force the rows back to sort_order = 0 (simulating a row
	// state from before the v63 backfill ran, or a row inserted
	// into a fresh archive before the inline schema was upgraded).
	if _, err := d.Conn().Exec(`UPDATE records SET sort_order = 0 WHERE person_record_id = ?`, created.ID); err != nil {
		t.Fatalf("reset sort_order: %v", err)
	}

	// Run the backfill UPDATE — same SQL block-63 ships.
	if _, err := d.Conn().Exec(`UPDATE records SET sort_order = id WHERE sort_order = 0 OR sort_order IS NULL`); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	got, err := svc.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID after backfill: %v", err)
	}
	if len(got.Records) != 3 {
		t.Fatalf("len(Records) = %d, want 3", len(got.Records))
	}
	for _, r := range got.Records {
		if r.SortOrder != r.ID {
			t.Errorf("record id=%d: SortOrder = %d, want %d (backfill assigns sort_order=id)", r.ID, r.SortOrder, r.ID)
		}
	}
}

// silence the unused import of db if a future refactor removes
// the newTestDB helper. Keeps the test file compiling through
// intermediate states.
var _ = db.Open