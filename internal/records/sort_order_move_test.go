package records

import (
	"strconv"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

// === Issue #368 slice 2 RED tests ===
//
// SoldierService.MoveRecordWithinPerson + EventService.MoveEventSource
// rewrite the sort_order of a Source Record / Event Source so the
// user can reorder via the PATCH endpoint. The method:
//   - accepts position in 1..N (clamped to that range)
//   - writes a single transaction that shifts other rows' sort_order
//     so the requested row lands at `position`
//   - returns 404-equivalent for missing rows / rows that don't
//     belong to the named owner

func TestSoldierService_MoveRecordWithinPerson_ReordersMigratedNonDenseRows(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)
	created, err := svc.Create(models.Soldier{
		FirstName: "Thomas",
		LastName:  "Thrasher",
		Records: []models.Record{
			{RecordType: "Find a Grave"},
			{RecordType: "Census"},
			{RecordType: "Pension"},
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, _ := svc.GetByID(created.ID)
	firstID, secondID := got.Records[0].ID, got.Records[1].ID
	if _, err := d.Conn().Exec(`UPDATE records SET sort_order = id + 3000 WHERE person_record_id = ?`, created.ID); err != nil {
		t.Fatalf("seed migrated sort order: %v", err)
	}

	if err := svc.MoveRecordWithinPerson(created.ID, firstID, 2); err != nil {
		t.Fatalf("MoveRecordWithinPerson: %v", err)
	}
	got, _ = svc.GetByID(created.ID)
	if got.Records[0].ID != secondID || got.Records[1].ID != firstID {
		t.Fatalf("migrated move-down order = [%d %d ...], want [%d %d ...]", got.Records[0].ID, got.Records[1].ID, secondID, firstID)
	}
	for i, record := range got.Records {
		if record.SortOrder != int64(i+1) {
			t.Errorf("records[%d].SortOrder = %d, want dense ordinal %d", i, record.SortOrder, i+1)
		}
	}
}

func TestSoldierService_MoveRecordWithinPerson_ReordersToPosition(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	created, err := svc.Create(models.Soldier{
		FirstName: "Jeb",
		LastName:  "Stuart",
		Records: []models.Record{
			{RecordType: "Roster", AppID: "APP-A", Details: "first"},
			{RecordType: "Parole", AppID: "APP-B", Details: "second"},
			{RecordType: "Letter", AppID: "APP-C", Details: "third"},
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, _ := svc.GetByID(created.ID)
	firstID := got.Records[0].ID

	// Move the first record to position 3 (the end).
	if err := svc.MoveRecordWithinPerson(created.ID, firstID, 3); err != nil {
		t.Fatalf("MoveRecordWithinPerson: %v", err)
	}

	got, _ = svc.GetByID(created.ID)
	if len(got.Records) != 3 {
		t.Fatalf("len = %d, want 3", len(got.Records))
	}
	if got.Records[2].ID != firstID {
		t.Errorf("moved record landed at position 3 (index 2); got id=%d want id=%d", got.Records[2].ID, firstID)
	}
}

func TestSoldierService_MoveRecordWithinPerson_ClampsOutOfRangePosition(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	created, err := svc.Create(models.Soldier{
		FirstName: "Jeb",
		LastName:  "Stuart",
		Records: []models.Record{
			{RecordType: "Roster", AppID: "APP-A"},
			{RecordType: "Parole", AppID: "APP-B"},
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, _ := svc.GetByID(created.ID)
	firstID := got.Records[0].ID

	// Position 99 is out of range; the method clamps to the max
	// (which is 2 for 2 rows). The row lands at position 2.
	if err := svc.MoveRecordWithinPerson(created.ID, firstID, 99); err != nil {
		t.Fatalf("MoveRecordWithinPerson: %v", err)
	}
	got, _ = svc.GetByID(created.ID)
	if got.Records[1].ID != firstID {
		t.Errorf("clamp failed; got id=%d at position 2, want id=%d", got.Records[1].ID, firstID)
	}
}

func TestSoldierService_MoveRecordWithinPerson_RejectsMissingRow(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	created, _ := svc.Create(models.Soldier{FirstName: "Jeb", LastName: "Stuart"})

	if err := svc.MoveRecordWithinPerson(created.ID, 99999, 1); err == nil {
		t.Fatalf("MoveRecordWithinPerson accepted a non-existent record id; want error")
	}
}

func TestSoldierService_MoveRecordWithinPerson_DoesNotLeakBetweenSoldiers(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	a, _ := svc.Create(models.Soldier{
		FirstName: "A",
		Records:   []models.Record{{RecordType: "A1", AppID: "A1"}},
	})
	b, _ := svc.Create(models.Soldier{
		FirstName: "B",
		Records:   []models.Record{{RecordType: "B1", AppID: "B1"}},
	})

	// Get A's record id and try to move it under B.
	aRec, _ := svc.GetByID(a.ID)
	bRec, _ := svc.GetByID(b.ID)
	if err := svc.MoveRecordWithinPerson(b.ID, aRec.Records[0].ID, 1); err == nil {
		t.Fatalf("MoveRecordWithinPerson accepted a record from a different soldier; want error")
	}

	// B's records must be unchanged.
	bRecAfter, _ := svc.GetByID(b.ID)
	if bRecAfter.Records[0].ID != bRec.Records[0].ID {
		t.Errorf("B's records were mutated by an attempt to move A's record into B")
	}
}

func TestEventService_MoveEventSource_ReordersMigratedNonDenseRows(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)
	esvc := NewEventService(svc)
	created, err := svc.Create(models.Soldier{FirstName: "Battle", EntryType: "event", Kind: "battle", BeginDate: "1862-09-17"})
	if err != nil {
		t.Fatalf("Create event: %v", err)
	}
	firstID, _ := esvc.AttachSourceToEvent(created.ID, models.Record{RecordType: "Roster"}, 0)
	secondID, _ := esvc.AttachSourceToEvent(created.ID, models.Record{RecordType: "Report"}, 1)
	_, _ = esvc.AttachSourceToEvent(created.ID, models.Record{RecordType: "Letter"}, 2)
	if _, err := d.Conn().Exec(`UPDATE event_sources SET sort_order = id + 4000 WHERE event_id = ?`, created.ID); err != nil {
		t.Fatalf("seed migrated sort order: %v", err)
	}

	if err := esvc.MoveEventSource(created.ID, firstID, 2); err != nil {
		t.Fatalf("MoveEventSource: %v", err)
	}
	rows, err := d.Conn().Query(`SELECT id, sort_order FROM event_sources WHERE event_id = ? ORDER BY sort_order, id`, created.ID)
	if err != nil {
		t.Fatalf("read event sources: %v", err)
	}
	defer rows.Close()
	var ids, orders []int64
	for rows.Next() {
		var id, order int64
		if err := rows.Scan(&id, &order); err != nil {
			t.Fatalf("Scan: %v", err)
		}
		ids, orders = append(ids, id), append(orders, order)
	}
	if ids[0] != secondID || ids[1] != firstID {
		t.Fatalf("migrated event move-down order = %v, want second=%d first=%d", ids, secondID, firstID)
	}
	for i, order := range orders {
		if order != int64(i+1) {
			t.Errorf("event order[%d] = %d, want %d", i, order, i+1)
		}
	}
}

func TestEventService_MoveEventSource_ReordersToPosition(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)
	esvc := NewEventService(svc)

	// Create the Event row first (so we have a valid event_id),
	// then attach sources via the dedicated event_sources path
	// (Create's Records[] goes into the generic `records` table,
	// not `event_sources`).
	created, err := svc.Create(models.Soldier{
		FirstName: "Pvt",
		LastName:  "Doe",
		EntryType: "event",
		Kind:      "battle",
		BeginDate: "1862-09-17",
		EndDate:   "1862-09-17",
	})
	if err != nil {
		t.Fatalf("Create event: %v", err)
	}

	id1, err := esvc.AttachSourceToEvent(created.ID, models.Record{RecordType: "Roster", AppID: "ER-A"}, 0)
	if err != nil {
		t.Fatalf("AttachSourceToEvent 1: %v", err)
	}
	id2, err := esvc.AttachSourceToEvent(created.ID, models.Record{RecordType: "Parole", AppID: "ER-B"}, 1)
	if err != nil {
		t.Fatalf("AttachSourceToEvent 2: %v", err)
	}
	id3, err := esvc.AttachSourceToEvent(created.ID, models.Record{RecordType: "Letter", AppID: "ER-C"}, 2)
	if err != nil {
		t.Fatalf("AttachSourceToEvent 3: %v", err)
	}

	// Move the first source to position 3 (the end).
	if err := esvc.MoveEventSource(created.ID, id1, 3); err != nil {
		t.Fatalf("MoveEventSource: %v", err)
	}

	// Read back in sort_order order.
	rows, err := d.Conn().Query(
		`SELECT id, sort_order FROM event_sources WHERE event_id = ? ORDER BY sort_order, id`,
		created.ID,
	)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	defer rows.Close()
	var orderedIDs []int64
	for rows.Next() {
		var id, ord int64
		if err := rows.Scan(&id, &ord); err != nil {
			t.Fatalf("scan: %v", err)
		}
		orderedIDs = append(orderedIDs, id)
	}
	if len(orderedIDs) != 3 {
		t.Fatalf("len = %d, want 3", len(orderedIDs))
	}
	// First source should now be at the end (position 3).
	if orderedIDs[2] != id1 {
		t.Errorf("moved event-source id1=%d should be at position 3; got ids=%v", id1, orderedIDs)
	}
	// id2 (originally at position 2) shifts up to position 1;
	// id3 stays at position 2; id1 lands at position 3.
	if orderedIDs[0] != id2 {
		t.Errorf("id2=%d should be at position 1 after the shift-up; got ids=%v", id2, orderedIDs)
	}
	if orderedIDs[1] != id3 {
		t.Errorf("id3=%d should be at position 2 after the shift-up; got ids=%v", id3, orderedIDs)
	}
	_ = id2
	_ = id3
}

func TestEventService_MoveEventSource_RejectsRecordOwnedByOtherEvent(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)
	esvc := NewEventService(svc)

	eventA, _ := svc.Create(models.Soldier{
		FirstName: "EventA",
		EntryType: "event",
		Kind:      "battle",
		BeginDate: "1862-09-17",
		EndDate:   "1862-09-17",
		Records:   []models.Record{{RecordType: "Roster", AppID: "ERA-1"}},
	})
	eventB, _ := svc.Create(models.Soldier{
		FirstName: "EventB",
		EntryType: "event",
		Kind:      "battle",
		BeginDate: "1862-09-18",
		EndDate:   "1862-09-18",
		Records:   []models.Record{{RecordType: "Parole", AppID: "ERB-1"}},
	})

	aRec, _ := svc.GetByID(eventA.ID)
	bRec, _ := svc.GetByID(eventB.ID)

	// Try to move A's record under B.
	if err := esvc.MoveEventSource(eventB.ID, aRec.Records[0].ID, 1); err == nil {
		t.Fatalf("MoveEventSource accepted a record from a different event; want error")
	}

	// B's records must be unchanged.
	bAfter, _ := svc.GetByID(eventB.ID)
	if bAfter.Records[0].ID != bRec.Records[0].ID {
		t.Errorf("B's event sources were mutated by an attempt to move A's record into B")
	}
}

func TestReplaceRecordsWritesSortOrderFromArrayIndex(t *testing.T) {
	// The write path that ships in slice 2 must set sort_order
	// from the form-array index on insert, so re-saving the
	// records preserves the user's current display order (and
	// the secondary `id` tiebreak no longer hides reorders).
	d := newTestDB(t)
	svc := NewSoldierService(d)

	created, err := svc.Create(models.Soldier{
		FirstName: "Custis",
		LastName:  "Lee",
		Records: []models.Record{
			{RecordType: "Roster", AppID: "A", Details: "index 0"},
			{RecordType: "Parole", AppID: "B", Details: "index 1"},
			{RecordType: "Letter", AppID: "C", Details: "index 2"},
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, _ := svc.GetByID(created.ID)
	if len(got.Records) != 3 {
		t.Fatalf("len = %d, want 3", len(got.Records))
	}
	// sort_order must be 0, 1, 2 — set from the form-array index.
	wantOrder := []int64{0, 1, 2}
	for i, r := range got.Records {
		if r.SortOrder != wantOrder[i] {
			t.Errorf("records[%d].SortOrder = %d, want %d", i, r.SortOrder, wantOrder[i])
		}
	}
	_ = strconv.Itoa // keep import used; helper reserved for future raw-SQL probes
}
