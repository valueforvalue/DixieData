// Issue #357: Source Records attach UI is missing from the
// Event Record form (new + edit). The detail page exposes a
// record_type / app_id / details form posting to
// /events/{id}/sources/attach, but the create + edit forms do
// not. This test pins the acceptance criterion: the form must
// expose inline Source Record rows so a user can attach sources
// on submit (mirrors the soldier entry form pattern via the
// shared RecordInputRow component).
//
// RED today (issue #357): the form renders only kind / dates /
// description / pdf_excerpt / notes — no Source Records section.
// The test fails on the absence of "Source Records" header +
// record_type / record_app_id / record_details inputs (the
// field-name triple RecordInputRow emits; parseRecordInputs in
// app.go flattens them into []models.Record).
package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

func TestEventFormFragmentRendersSourceRecordsSection(t *testing.T) {
	cases := []struct {
		name    string
		isEdit  bool
		eventID int64
	}{
		{"new", false, 0},
		{"edit", true, 519},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := EventFormFragment(viewmodel.PersonRecord{
				ID:        tc.eventID,
				DisplayID: "EVT-00519",
				Kind:      "Battle",
			}, tc.isEdit, "").Render(context.Background(), &buf)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}

			content := buf.String()
			// Source Records header — mirrors entry_form.templ:286
			if !strings.Contains(content, "Source Records") {
				t.Errorf("event form missing Source Records section header")
			}
			// Inline row inputs — the shared RecordInputRow emits
			// record_type / record_app_id / record_details (the
			// triple parseRecordInputs in app.go flattens into
			// []models.Record). The detail page uses different
			// names (app_id / details); the inline pattern reuses
			// the soldier entry form helper for consistency.
			for _, needle := range []string{
				`name="record_type"`,
				`name="record_app_id"`,
				`name="record_details"`,
			} {
				if !strings.Contains(content, needle) {
					t.Errorf("event form missing %s input", needle)
				}
			}
			// Edit path: existing EventSources round-trip back into
			// the form so the user sees what's attached before
			// adding more. The empty row still renders after them
			// (matching entry_form.templ:301-306).
			if tc.isEdit {
				if !strings.Contains(content, "data-record-template") {
					t.Errorf("edit form missing data-record-template (JS dispatcher hook)")
				}
			}
		})
	}
}
// TestEventFormFragmentRendersLinkedPersonsSection pins slice
// 2 of #361: the Event editor form (rendered for both new +
// edit modes) must expose a Linked Person Records section
// inline so the user can attach/detach without bouncing
// through the Person Record's Events tab.
//
// RED today: the form has no Linked Persons section at all.
// The test fails on the absence of the "Linked Person Records"
// header + the "display_id" input + the inline Unlink button
// shape. After slice 2 lands: the header renders, the input
// renders with name="display_id", the Add form posts to the
// new /events/{id}/links route, and per-row Unlink buttons
// post to /events/{id}/links/{personId}/detach.
//
// Mirrors the TestEventFormFragmentRendersSourceRecordsSection
// (issue #357) shape — same fixture (event with id=519), same
// assertions structure.
func TestEventFormFragmentRendersLinkedPersonsSection(t *testing.T) {
	// Linked Persons section only renders for edit mode (you
	// can't link a Person to an Event that doesn't exist yet
	// — the link route requires a persisted event id). New-
	// mode form asserts the section is ABSENT.
	cases := []struct {
		name           string
		isEdit         bool
		eventID        int64
		wantSection    bool
	}{
		{"new-skips-section", false, 0, false},
		{"edit-shows-section", true, 519, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := EventFormFragment(viewmodel.PersonRecord{
				ID:        tc.eventID,
				DisplayID: "EVT-00519",
				Kind:      "Battle",
			}, tc.isEdit, "").Render(context.Background(), &buf)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}

			content := buf.String()
			hasSection := strings.Contains(content, "Linked Person Records") &&
				strings.Contains(content, `name="display_id"`)
			if hasSection != tc.wantSection {
				t.Errorf("Linked Persons section presence: got %v, want %v (isEdit=%v)",
					hasSection, tc.wantSection, tc.isEdit)
			}

			if !tc.wantSection {
				return
			}
			// Edit-only assertions: the Add form posts to the
			// new /events/{id}/links route; the handler resolves
			// the Display ID via LookupPersonIDByDisplayID and
			// delegates to existing AttachEventToPerson.
			if !strings.Contains(content, `action="/events/519/links"`) {
				t.Errorf("event form Linked Persons section missing Add form action posting to /events/{id}/links")
			}
			// data-dixie-submit on the Add form (full-page nav
			// to /events/{id}/edit on success).
			if !strings.Contains(content, `data-dixie-submit="true"`) {
				t.Errorf("event form Linked Persons Add form missing data-dixie-submit=\"true\"")
			}
			// Edit path: existing LinkedPersons render via
			// the per-row Unlink button shape. The fixture has
			// an empty LinkedPersons slice, so we only assert
			// the empty-state copy is present, not Unlink
			// buttons (those would require seeding a Person
			// Record into the viewmodel).
			if !strings.Contains(content, "No linked Person Records") {
				t.Errorf("event form Linked Persons empty-state copy missing")
			}
		})
	}
}

func TestEventFormFragmentRendersTagsSection(t *testing.T) {
	// Issue #361 slice 3: the Event editor exposes an inline
	// Tags section (below Linked Persons) so the user can
	// attach/detach tags without bouncing through a separate
	// page. Section renders only for edit mode (same shape as
	// the Linked Persons section from slice 2).
	cases := []struct {
		name        string
		isEdit      bool
		eventID     int64
		wantSection bool
	}{
		{"new-skips-section", false, 0, false},
		{"edit-shows-section", true, 519, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := EventFormFragment(viewmodel.PersonRecord{
				ID:        tc.eventID,
				DisplayID: "EVT-00519",
				Kind:      "Battle",
			}, tc.isEdit, "").Render(context.Background(), &buf)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}

			content := buf.String()
			// Section must wrap a div with the in-place swap
			// target id (matches the detail-page fragment's
			// data-results-target="#data-event-tags-list"). If
			// the section renders, the swap target div must
			// also render so JS swap has somewhere to land.
			hasSection := strings.Contains(content, "Tags") &&
				strings.Contains(content, `action="/events/519/tags"`)
			if hasSection != tc.wantSection {
				t.Errorf("Tags section presence: got %v, want %v (isEdit=%v)",
					hasSection, tc.wantSection, tc.isEdit)
			}

			if !tc.wantSection {
				return
			}
			// Edit-only assertions: the Add form posts to the
			// existing /events/{id}/tags route. The handler
			// now accepts tag_name (slice 3) and returns a
			// fragment (no X-DixieData-Redirect) so the JS
			// dispatcher can swap in place.
			if !strings.Contains(content, `data-dixie-submit="true"`) {
				t.Errorf("event form Tags section Add form missing data-dixie-submit=\"true\"")
			}
			// The free-text input carries name="tag_name"
			// (matches the soldier-side /soldiers/{id}/tags
			// picker UX).
			if !strings.Contains(content, `name="tag_name"`) {
				t.Errorf("event form Tags section Add form missing name=\"tag_name\" input")
			}
			// In-place swap target div must render on the
			// edit page so the post-add fragment has a place
			// to land.
			if !strings.Contains(content, `id="data-event-tags-list"`) {
				t.Errorf("event form Tags section missing in-place swap target div id=\"data-event-tags-list\"")
			}
			// Empty state: the form renders with no tags in
			// the fixture (PersonRecord.Tags is nil). The
			// empty-state copy must match what the existing
			// fragment uses.
			if !strings.Contains(content, "No tags attached") {
				t.Errorf("event form Tags section missing empty-state copy")
			}
		})
	}
}
