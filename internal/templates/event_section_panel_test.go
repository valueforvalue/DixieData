// Characterization tests for EventSectionPanel (issue #343
// finding #2). The component extracts the near-identical
// Sources + Tags panel scaffolding from event_detail.templ —
// a <section> wrapper, a header row (title + count + optional
// Edit Event CTA), and a body wrapper div carrying the literal
// id that the JS dispatcher swaps into.
//
// The component MUST produce byte-identical HTML for both
// panels so the existing appshell/events_handlers_test.go pin
// (id="data-event-sources-list" + id="data-event-tags-list" +
// data-action="/events/{id}/edit") stays green. RED-then-GREEN:
// write the assertions first against the new component
// signature (which does not exist yet), confirm RED, implement
// the component, confirm GREEN.
//
// The extracted shape covers the Sources + Tags pair (the
// issue's literal scope). Linked Persons + Images stay inline
// for now — they each have asymmetric structure (Linked Persons
// has no body wrapper; Images has no Edit CTA + a tail import
// form) and would need either a wider param set or a follow-up
// slice. Documented in the issue's "apply opportunistically"
// recommendation.
package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

// TestEventSectionPanelSourcesShape pins the rendered HTML for
// the Sources panel: section wrapper, header title + count,
// Edit Event CTA pointing at /events/{id}/edit, body wrapper
// carrying the literal id the JS dispatcher swaps into.
//
// Uses a non-empty viewmodel.SourceRecord slice so the list
// branch (not empty-state) is exercised. The empty-state path
// is covered by the inline-shape appshell tests.
func TestEventSectionPanelSourcesShape(t *testing.T) {
	const (
		eventID  = "519"
		panelID  = "data-event-sources-list"
		title    = "Source Records"
		countFmt = "attached"
	)

	sources := []viewmodel.SourceRecord{
		{ID: 100, SourceRecordType: "Roster", AppID: "APP-X", Details: "first"},
	}

	var buf bytes.Buffer
	err := EventSectionPanel(
		title,
		len(sources),
		countFmt,
		eventID,
		panelID,
		EventSourcesListFragment(519, sources),
	).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()

	// Header title — must be inside an uppercase-tracking <p>.
	wantTitle := ">" + title + "</p>"
	if !strings.Contains(got, wantTitle) {
		t.Errorf("EventSectionPanel missing title %q; got %q", wantTitle, got)
	}

	// Count line — "{N} {label}" lowercase, with the docs'
	// standard slate-500 span.
	wantCount := ">1 attached</span>"
	if !strings.Contains(got, wantCount) {
		t.Errorf("EventSectionPanel missing count %q; got %q", wantCount, got)
	}

	// Edit Event CTA — must point at /events/{id}/edit so the
	// existing appshell/events_handlers_test.go editHref pin
	// keeps passing on the full detail page.
	wantEdit := `data-action="/events/` + eventID + `/edit"`
	if !strings.Contains(got, wantEdit) {
		t.Errorf("EventSectionPanel missing Edit Event CTA %q; got %q", wantEdit, got)
	}
	if !strings.Contains(got, "ghost-link") {
		t.Errorf("EventSectionPanel Edit Event CTA must use ghost-link class; got %q", got)
	}
	if !strings.Contains(got, "Edit Event") {
		t.Errorf("EventSectionPanel Edit Event CTA label missing; got %q", got)
	}

	// Body wrapper — must carry the literal id (NOT a uiid
	// dot-separated constant). The JS dispatcher swaps into
	// this exact id per events_handlers.go.
	if !strings.Contains(got, `id="`+panelID+`"`) {
		t.Errorf("EventSectionPanel missing body wrapper id %q; got %q", panelID, got)
	}

	// Body content — the source fragment row rendered inside
	// the wrapper (the reorder form action carries the id).
	if !strings.Contains(got, `action="/events/519/sources/100/position"`) {
		t.Errorf("EventSectionPanel missing source row 100 reorder form; got %q", got)
	}
}

// TestEventSectionPanelTagsShape pins the same shape for Tags
// (the other half of the issue's scope). Same wrapping, same
// CTA, different id + title + body.
func TestEventSectionPanelTagsShape(t *testing.T) {
	const (
		eventID  = "519"
		panelID  = "data-event-tags-list"
		title    = "Tags"
		countFmt = "attached"
	)

	tags := []viewmodel.TagOption{
		{ID: 7, Name: "veteran"},
		{ID: 9, Name: "paroled"},
	}

	var buf bytes.Buffer
	err := EventSectionPanel(
		title,
		len(tags),
		countFmt,
		eventID,
		panelID,
		EventTagsListFragment(519, tags),
	).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()

	if !strings.Contains(got, ">"+title+"</p>") {
		t.Errorf("EventSectionPanel missing Tags title; got %q", got)
	}
	if !strings.Contains(got, ">2 attached</span>") {
		t.Errorf("EventSectionPanel missing Tags count; got %q", got)
	}
	if !strings.Contains(got, `data-action="/events/`+eventID+`/edit"`) {
		t.Errorf("EventSectionPanel missing Tags Edit Event CTA; got %q", got)
	}
	if !strings.Contains(got, `id="`+panelID+`"`) {
		t.Errorf("EventSectionPanel missing Tags body wrapper id; got %q", got)
	}

	// Body content — Tags fragment row rendered inside the
	// wrapper. EventTagsListFragment renders an action for
	// detach; assert one of them survives.
	if !strings.Contains(got, `action="/events/519/tags/7/detach"`) {
		t.Errorf("EventSectionPanel missing tag 7 detach form; got %q", got)
	}
}

// TestEventSectionPanelMultiRowSources pins that the Sources
// fragment renders all rows when wrapped in the new panel
// component. Pins the per-row reorder form actions so a
// future refactor can't silently drop a row.
func TestEventSectionPanelMultiRowSources(t *testing.T) {
	const (
		eventID = "519"
		panelID = "data-event-sources-list"
	)

	sources := []viewmodel.SourceRecord{
		{ID: 1, SourceRecordType: "Roster", AppID: "APP-A", Details: "first"},
		{ID: 2, SourceRecordType: "Parole", AppID: "APP-B", Details: "second"},
	}

	var buf bytes.Buffer
	err := EventSectionPanel(
		"Source Records",
		len(sources),
		"attached",
		eventID,
		panelID,
		EventSourcesListFragment(519, sources),
	).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()

	if !strings.Contains(got, ">Source Records</p>") {
		t.Errorf("composed panel missing Sources title; got %q", got)
	}
	if !strings.Contains(got, `action="/events/519/sources/1/position"`) {
		t.Errorf("composed Sources panel missing row 1 reorder form; got %q", got)
	}
	if !strings.Contains(got, `action="/events/519/sources/2/position"`) {
		t.Errorf("composed Sources panel missing row 2 reorder form; got %q", got)
	}
	if !strings.Contains(got, `id="`+panelID+`"`) {
		t.Errorf("composed panel missing body wrapper id; got %q", got)
	}
}
