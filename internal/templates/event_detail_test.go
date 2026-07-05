// Issue #361 Slice 1: Event detail page Linked Persons + Tags
// panel headers must surface an "Edit Event" CTA that links to
// /events/{id}/edit, mirroring the post-#360 Sources panel
// pattern. Empty-state copy on the Linked Persons panel must
// reference the event editor (no longer tell the user to bounce
// through the Person Record's Events tab).
//
// RED today: Linked Persons panel header has no Edit Event CTA;
// Tags panel header has no Edit Event CTA AND no count span.
// After Slice 1 lands: both panel headers render an Edit Event
// button that POSTs to /events/{id}/edit, and the Tags panel
// gets a count span next to its title.
//
// Render test pins the markup contract. The companion smoke
// probe step-04c + step-04d in audit/smoke_events.mjs pins the
// end-to-end invoker wiring (response shape + post-nav URL +
// DOM state). The Go test alone would miss the #1 tdd.md failure
// mode (modal/CTA invoker silently swallowed); the smoke alone
// would miss the markup drift. Both are needed.
package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

// sliceBody extracts the substring between a panel's opening
// tag and the next sibling section. We use it to assert that
// specific markup is INSIDE a given panel rather than anywhere
// on the page (the page also has a top-level Edit Event <a>
// link in the header action cluster — a substring search would
// falsely pass for any panel by matching the top-level link).
//
// Returns "" if the panel can't be located.
func sliceBody(body, panelTitle string) string {
	// Find the <p ...>PanelTitle</p> marker. The class string
	// differs per panel (some have `mb-3`, some don't), so we
	// search for the title text inside a <p> element with
	// closing </p> immediately after. This is robust to the
	// class-string drift.
	needle := ">" + panelTitle + "</p>"
	titleIdx := strings.Index(body, needle)
	if titleIdx < 0 {
		return ""
	}
	// Walk backward to the start of the enclosing <p>.
	pStart := strings.LastIndex(body[:titleIdx], "<p ")
	if pStart < 0 {
		return ""
	}
	// Walk further back to the enclosing <section>.
	sectionStart := strings.LastIndex(body[:pStart], "<section")
	if sectionStart < 0 {
		return ""
	}
	// Walk forward to the next <section after this one — that
	// terminates our panel.
	rest := body[sectionStart+len("<section"):]
	sectionEndRel := strings.Index(rest, "<section")
	if sectionEndRel < 0 {
		// Last section on the page — take to end of body.
		return body[sectionStart:]
	}
	return body[sectionStart : sectionStart+len("<section")+sectionEndRel]
}

// TestEventDetailLinkedPersonsPanelEditCTAPins pins the Edit
// Event CTA on the Linked Persons panel header for an empty
// event. The CTA must render even when there are no linked
// Person Records (the user needs the affordance to add the
// first one). Also pins the empty-state copy referencing the
// event editor.
func TestEventDetailLinkedPersonsPanelEditCTAPins(t *testing.T) {
	var buf bytes.Buffer
	err := EventDetail(
		viewmodel.PersonRecord{ID: 519, DisplayID: "EVT-00519", Kind: "Battle"},
		nil, // no linked Persons
	).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	body := buf.String()

	panel := sliceBody(body, "Linked Person Records")
	if panel == "" {
		t.Fatalf("Linked Persons panel not found in rendered body — slice cannot be GREEN")
	}

	// Edit Event CTA must be present IN THIS PANEL (substring
	// search on whole body would falsely pass via the top-level
	// header Edit Event <a>).
	if !strings.Contains(panel, `data-action="/events/519/edit"`) {
		t.Errorf("Linked Persons panel missing Edit Event CTA (data-action=\"/events/519/edit\")")
	}
	if !strings.Contains(panel, `data-dixie-submit="true"`) {
		t.Errorf("Linked Persons Edit Event CTA missing data-dixie-submit=\"true\"")
	}
	// button must be type="button" per §1.1 bug pattern (NOT
	// type="submit" — would cause accidental form submission).
	if !strings.Contains(panel, `<button type="button"`) {
		t.Errorf("Linked Persons Edit Event CTA must use literal <button type=\"button\" ...> (sidesteps open #365 components.Button bug + matches §1.1 pattern)")
	}

	// Empty-state copy must reference the event editor.
	// We match the substring "event editor" (case-insensitive
	// via LoweredContains) because the exact phrasing is the
	// implementer's call.
	if !strings.Contains(strings.ToLower(panel), "event editor") {
		t.Errorf("Linked Persons empty-state copy still tells the user to attach from the Person Record's Events tab")
	}
	// Negative: the bounce copy (in any of its rendered forms —
	// literal apostrophe OR HTML-entity-escaped apostrophe
	// `&#39;`) must be gone.
	bounceCopy := "from the Person Record detail page"
	if strings.Contains(panel, bounceCopy) {
		t.Errorf("Linked Persons empty-state copy still references the old bounce-through-Person path (substring %q present)", bounceCopy)
	}
}

// TestEventDetailTagsPanelEditCTAPins pins the Edit Event CTA
// + count span on the Tags panel header. Pre-slice, the Tags
// header has neither the CTA nor a count span (Linked Persons
// and Sources panels had both). Slice 1 closes the
// inconsistency.
func TestEventDetailTagsPanelEditCTAPins(t *testing.T) {
	var buf bytes.Buffer
	err := EventDetail(
		viewmodel.PersonRecord{ID: 519, DisplayID: "EVT-00519", Kind: "Battle"},
		nil,
	).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	body := buf.String()

	panel := sliceBody(body, "Tags")
	if panel == "" {
		t.Fatalf("Tags panel not found in rendered body — slice cannot be GREEN")
	}

	// Edit Event CTA must be present IN THIS PANEL.
	if !strings.Contains(panel, `data-action="/events/519/edit"`) {
		t.Errorf("Tags panel missing Edit Event CTA (data-action=\"/events/519/edit\")")
	}
	if !strings.Contains(panel, `<button type="button"`) {
		t.Errorf("Tags Edit Event CTA must use literal <button type=\"button\" ...>")
	}

	// Count span — slice 1 adds one to mirror the Linked
	// Persons `' N linked '` and Sources `' N attached '`
	// shapes. We pin the presence of a count-shaped pattern
	// (digit + attached/linked/tag) inside the header's flex
	// row, not in the fragment body below it. We exclude the
	// fragment div by truncating at the first <div id=
	// data-event-tags-list> marker.
	headerEnd := strings.Index(panel, `<div id="data-event-tags-list"`)
	if headerEnd < 0 {
		headerEnd = len(panel)
	}
	headerOnly := panel[:headerEnd]
	hasCount := strings.Contains(headerOnly, "0 attached") ||
		strings.Contains(headerOnly, "0 tag") ||
		strings.Contains(headerOnly, `>0<`) // bare "0" inside a span
	if !hasCount {
		t.Errorf("Tags panel header missing count-shaped element (slice 1 adds one to mirror Linked Persons + Sources)")
	}
}

// TestEventDetailEditEventCTACountPinsAcrossPopulatedAndEmpty
// guards against a partial slice: the Edit Event CTA in the
// new panels (Linked Persons + Tags) must render regardless of
// whether the panel body is empty or populated. Mirrors the
// post-#360 Sources panel behavior.
func TestEventDetailEditEventCTACountPinsAcrossPopulatedAndEmpty(t *testing.T) {
	cases := []struct {
		name   string
		linked []viewmodel.PersonRecord
	}{
		{"empty", nil},
		{"populated", []viewmodel.PersonRecord{
			{ID: 1, DisplayID: "SOL-00001"},
			{ID: 2, DisplayID: "SOL-00002"},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := EventDetail(
				viewmodel.PersonRecord{ID: 519, DisplayID: "EVT-00519"},
				tc.linked,
			).Render(context.Background(), &buf)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}

			// Each of Linked Persons + Tags panels must carry
			// its own Edit Event CTA, regardless of populated
			// state.
			for _, panelTitle := range []string{"Linked Person Records", "Tags"} {
				panel := sliceBody(buf.String(), panelTitle)
				if panel == "" {
					t.Errorf("[%s] panel %q not found", tc.name, panelTitle)
					continue
				}
				if !strings.Contains(panel, `data-action="/events/519/edit"`) {
					t.Errorf("[%s] %s panel missing Edit Event CTA (populated=%v)",
						tc.name, panelTitle, len(tc.linked) > 0)
				}
			}
		})
	}
}