// Regression tests for EventSectionPanel. Panel headers show counts
// and body content; page-level Edit Event link is the only edit entry point.
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

	if strings.Contains(got, `data-action="/events/`+eventID+`/edit"`) || strings.Contains(got, "Edit Event") {
		t.Errorf("EventSectionPanel renders duplicate Edit Event action; got %q", got)
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
	if strings.Contains(got, `data-action="/events/`+eventID+`/edit"`) || strings.Contains(got, "Edit Event") {
		t.Errorf("Tags panel must not render duplicate Edit Event action; got %q", got)
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
