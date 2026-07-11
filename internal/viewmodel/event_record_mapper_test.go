package viewmodel

// Tests for the EventRecord mapper introduced by #343 #1.
// The mapper is the canonical way to build an EventRecord from
// a models.Soldier row + linked persons + event sources. Each
// test pins a contract that the consumer sites (event_*.
// templ, presentation/views.go) rely on.

import (
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

// TestEventRecordFromModelCopiesBaseIdentity asserts that the
// base identity fields promote from the embedded PersonRecord.
// Without this, templates reading event.DisplayID or event.Tags
// would render empty even when the source row has values.
func TestEventRecordFromModelCopiesBaseIdentity(t *testing.T) {
	in := models.Soldier{
		ID:        42,
		DisplayID: "EVT-00001",
		SyncID:    "evt-sync-001",
		EntryType: models.EntryTypeEvent,
	}
	got := EventRecordFromModel(in, nil, nil)
	if got.ID != 42 {
		t.Errorf("ID = %d, want 42", got.ID)
	}
	if got.DisplayID != "EVT-00001" {
		t.Errorf("DisplayID = %q, want EVT-00001", got.DisplayID)
	}
	if got.SyncID != "evt-sync-001" {
		t.Errorf("SyncID = %q, want evt-sync-001", got.SyncID)
	}
	if got.EntryType != models.EntryTypeEvent {
		t.Errorf("EntryType = %q, want %q", got.EntryType, models.EntryTypeEvent)
	}
}

// TestEventRecordFromModelCopiesEventFields asserts the six
// Event-only fields populate from the source row.
func TestEventRecordFromModelCopiesEventFields(t *testing.T) {
	in := models.Soldier{
		Kind:        "Battle",
		BeginDate:   "07/01/1862",
		EndDate:     "07/03/1862",
		Description: "Seven Days Battles",
	}
	got := EventRecordFromModel(in, nil, nil)
	if got.Kind != "Battle" {
		t.Errorf("Kind = %q, want Battle", got.Kind)
	}
	if got.BeginDate != "07/01/1862" {
		t.Errorf("BeginDate = %q, want 07/01/1862", got.BeginDate)
	}
	if got.EndDate != "07/03/1862" {
		t.Errorf("EndDate = %q, want 07/03/1862", got.EndDate)
	}
	if got.Description != "Seven Days Battles" {
		t.Errorf("Description = %q, want Seven Days Battles", got.Description)
	}
}

// TestEventRecordFromModelAttachesEventSources asserts that the
// mapper projects the per-Event sources (the event_sources
// table content, passed in as sources) into EventRecord.
// EventSources, NOT into the embedded PersonRecord.SourceRecords.
// The Person Record's source projection stays separate per #343
// #1 deletion-test signal: "remove EventSources from
// PersonRecord — does the v61 sources-wiped-on-Update bug come
// back? No. The table + service carry the load; the viewmodel
// field is a mirror."
func TestEventRecordFromModelAttachesEventSources(t *testing.T) {
	in := models.Soldier{}
	sources := []models.Record{
		{ID: 7, AppID: "APP-1880-7701", RecordType: "Pension"},
		{ID: 8, AppID: "APP-1880-7702", RecordType: "Application"},
	}
	got := EventRecordFromModel(in, nil, sources)
	if len(got.EventSources) != 2 {
		t.Fatalf("EventSources len = %d, want 2", len(got.EventSources))
	}
	if got.EventSources[0].AppID != "APP-1880-7701" {
		t.Errorf("EventSources[0].AppID = %q, want APP-1880-7701", got.EventSources[0].AppID)
	}
	// PersonRecord.SourceRecords must stay empty when sources go
	// through the EventSources path. (The mapper does NOT split
	// sources between the two fields; callers route Event sources
	// here and Person sources to PersonRecordFromModel.)
	if len(got.SourceRecords) != 0 {
		t.Errorf("SourceRecords len = %d, want 0 (PersonRecord sources stay separate from Event sources)", len(got.SourceRecords))
	}
}

// TestEventRecordFromModelAttachesLinkedPersons asserts the
// mapper projects the per-Event linked persons into EventRecord
// .LinkedPersons (slice 2 of #361). The mapper accepts the
// linked rows as a separate slice so callers can pass a
// zero-length slice for the new-event form path where no links
// are possible yet.
func TestEventRecordFromModelAttachesLinkedPersons(t *testing.T) {
	in := models.Soldier{}
	linked := []models.Soldier{
		{ID: 100, DisplayID: "SOL-00100"},
		{ID: 101, DisplayID: "SOL-00101"},
	}
	got := EventRecordFromModel(in, linked, nil)
	if len(got.LinkedPersons) != 2 {
		t.Fatalf("LinkedPersons len = %d, want 2", len(got.LinkedPersons))
	}
	if got.LinkedPersons[0].DisplayID != "SOL-00100" {
		t.Errorf("LinkedPersons[0].DisplayID = %q, want SOL-00100", got.LinkedPersons[0].DisplayID)
	}
	if got.LinkedPersons[1].DisplayID != "SOL-00101" {
		t.Errorf("LinkedPersons[1].DisplayID = %q, want SOL-00101", got.LinkedPersons[1].DisplayID)
	}
}

// TestEventRecordFromModelIgnoresInputEventSourcesField asserts
// that the mapper does NOT read sources from the input model's
// EventSources field. The mapper signature takes sources as a
// dedicated parameter; the input.EventSources field is ignored.
// This keeps the mapper's contract predictable: callers control
// the source flow rather than relying on what the input row
// happens to have populated.
func TestEventRecordFromModelIgnoresInputEventSourcesField(t *testing.T) {
	in := models.Soldier{
		EventSources: []models.Record{
			{ID: 99, AppID: "SHOULD-NOT-APPEAR"},
		},
	}
	got := EventRecordFromModel(in, nil, nil)
	if len(got.EventSources) != 0 {
		t.Errorf("EventSources len = %d, want 0 (mapper must ignore input.EventSources; use the dedicated parameter)", len(got.EventSources))
	}
}

// TestEventRecordsFromModelsProjectsList asserts the slice
// mapper applies EventRecordFromModel element-wise and preserves
// ordering.
func TestEventRecordsFromModelsProjectsList(t *testing.T) {
	in := []models.Soldier{
		{ID: 1, DisplayID: "EVT-00001", Kind: "Battle"},
		{ID: 2, DisplayID: "EVT-00002", Kind: "Earthquake"},
		{ID: 3, DisplayID: "EVT-00003", Kind: "Hospital Stay"},
	}
	got := EventRecordsFromModels(in, nil, nil)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0].Kind != "Battle" || got[2].Kind != "Hospital Stay" {
		t.Errorf("ordering or projection wrong: %+v", []string{got[0].Kind, got[2].Kind})
	}
}

// TestEventRecordCreatedAtRoundTrip exercises the embedded
// PersonRecord.CreatedAt field via the mapper. The architecture
// review (#343) lists CreatedAt as a base-identity field; the
// split must preserve it on the embedded struct. models.Soldier
// .CreatedAt is a string (RFC3339 from the DB layer) so the
// mapper is a straight copy.
func TestEventRecordCreatedAtRoundTrip(t *testing.T) {
	in := models.Soldier{CreatedAt: "2026-07-10T12:00:00Z"}
	got := EventRecordFromModel(in, nil, nil)
	if got.CreatedAt != "2026-07-10T12:00:00Z" {
		t.Errorf("CreatedAt = %q, want 2026-07-10T12:00:00Z", got.CreatedAt)
	}
}