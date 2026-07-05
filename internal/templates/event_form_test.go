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