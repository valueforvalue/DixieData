package viewmodel

// Tests for #343 finding #1: split viewmodel.EventRecord out of
// viewmodel.PersonRecord. Event-only fields (Kind / BeginDate /
// EndDate / Description / EventSources / LinkedPersons) belong on
// the EventRecord projection, not on the base PersonRecord.
//
// RED-first regression net (per docs/agents/tdd.md):
//
//   - TestPersonRecordOmitsEventFields — pins the deletion of the
//     six Event-only fields from PersonRecord. Tests reflect on
//     the struct so a future re-add trips the assertion.
//   - TestEventRecordOwnsEventFields — pins EventRecord as the
//     home of those six fields.
//   - TestEventRecordPromotesBaseIdentity — pins the embedded
//     PersonRecord so DisplayID / Tags / etc. resolve via the
//     embedded struct.
//
// The deletion-test signal from #343 body: "delete EventSources
// — does the v61 sources-wiped-on-Update bug come back? No. The
// table + service carry the load; the viewmodel field is a
// mirror." The mapper in mappers.go preserves this: PersonRecord
// becomes the pure base identity (no Event fields), and
// EventRecordFromModel is the new mapper for Event Rows.

import (
	"reflect"
	"testing"
)

// forbiddenEventFields is the set of field names PersonRecord
// MUST NOT carry once the split lands. Each entry cites the
// architecture-review finding that motivated the move.
var forbiddenEventFields = map[string]string{
	"Kind":          "#343 #1: Event-only; empty for Person Records.",
	"BeginDate":     "#343 #1: Event-only date-range field.",
	"EndDate":       "#343 #1: Event-only date-range field.",
	"Description":   "#343 #1: Event-only long-form write-up.",
	"EventSources":  "#343 #1: per-Event sources (event_sources table).",
	"LinkedPersons": "#361 slice 2: per-Event person links.",
}

// TestPersonRecordOmitsEventFields pins the deletion of Event-only
// fields from PersonRecord. Uses reflection so a future re-add
// (e.g. someone copy-pasting the old struct back) trips the
// assertion with a clear message pointing at #343.
func TestPersonRecordOmitsEventFields(t *testing.T) {
	rt := reflect.TypeOf(PersonRecord{})
	for name, reason := range forbiddenEventFields {
		if _, found := rt.FieldByName(name); found {
			t.Errorf("PersonRecord.%s still present; %s", name, reason)
		}
	}
}

// TestEventRecordOwnsEventFields pins EventRecord as the home of
// the six Event-only fields. The mapper
// EventRecordFromModel is the canonical way to build one (see
// mappers_test.go once it lands).
func TestEventRecordOwnsEventFields(t *testing.T) {
	rt := reflect.TypeOf(EventRecord{})
	for _, name := range []string{"Kind", "BeginDate", "EndDate", "Description", "EventSources", "LinkedPersons"} {
		if _, found := rt.FieldByName(name); !found {
			t.Errorf("EventRecord.%s missing; #343 #1 splits this field out of PersonRecord", name)
		}
	}
}

// TestEventRecordPromotesBaseIdentity pins the embedded
// PersonRecord so DisplayID / Tags / etc. resolve via the
// embedded struct (templates read event.DisplayID, not
// event.PersonRecord.DisplayID).
func TestEventRecordPromotesBaseIdentity(t *testing.T) {
	rt := reflect.TypeOf(EventRecord{})
	prField, found := rt.FieldByName("PersonRecord")
	if !found {
		t.Fatal("EventRecord must embed PersonRecord for base-identity fields (DisplayID, Tags, ...)")
	}
	if prField.Type != reflect.TypeOf(PersonRecord{}) {
		t.Errorf("EventRecord.PersonRecord type = %v, want %v", prField.Type, reflect.TypeOf(PersonRecord{}))
	}
	// Sanity: DisplayID must resolve through the embedded field.
	displayIDField, found := rt.FieldByName("DisplayID")
	if !found {
		t.Errorf("EventRecord.DisplayID must be promoted from embedded PersonRecord (anonymous field); not found")
	}
	if found && displayIDField.Type.Kind() != reflect.String {
		t.Errorf("EventRecord.DisplayID kind = %v, want string", displayIDField.Type.Kind())
	}
}