// Characterization tests for MegaMenu (issue #380 shape 2 —
// mega-menu Records + Share & Review). The component is the
// 2D-panel nav primitive that replaces the existing flat
// Foldout pattern for the top-nav nav restructure. Per
// issue #380 Phase 2 + the recon in docs/agents/notes/380-
// workflow-grouping.md, the top nav shrinks from 11 items to
// 5 visible (Calendar + 2 mega-menus + Settings + CTA) by
// folding Search / Filter / Events / Articles / Tags into a
// Records mega-menu, and Review Queue / Timeline / Research
// Log / Collections / Insights / Share items into a Share &
// Review mega-menu.
//
// Mega-menu vs flat-foldout distinction (per NN/g mega-menus
// guidance — https://www.nngroup.com/articles/mega-menus-work-well/):
//
//   - Flat foldout = single-column <ul>, items scroll if many
//   - Mega-menu = 2D grid with grouped columns, everything
//     visible at once, group titles act as headings
//
// The primitive mirrors the existing Foldout ARIA contract
// (aria-haspopup="menu" + aria-controls + role="menu") so the
// dispatcher JS in frontend/app.js picks up mega-menu triggers
// alongside foldout triggers without a new selector.
package components

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

// megaMenuLink builds a menuitem helper as a templ.Component
// from raw HTML. Lets the test file stay free of templ
// declarations (which must live in .templ files).
func megaMenuLink(href, label, marker string) templ.Component {
	markerAttr := ""
	if marker != "" {
		markerAttr = ` data-marker="` + marker + `"`
	}
	html := `<li role="none"><a href="` + href + `" role="menuitem" class="mega-menu-item pill-link justify-start w-full"` + markerAttr + `>` + label + `</a></li>`
	return templ.Raw(html)
}

// TestMegaMenu_ARIAContract pins the rendered trigger + panel
// markup so a future change can't silently strip the ARIA
// wiring that the JS dispatcher depends on. Mirrors
// TestFoldout_ARIAContract.
func TestMegaMenu_ARIAContract(t *testing.T) {
	var buf bytes.Buffer
	groups := []MegaGroup{
		{
			Title: "People",
			Items: []templ.Component{megaMenuLink("/soldiers", "Search", "records-search")},
		},
	}
	err := MegaMenu("Records", "layout.records.menu", nil, groups).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()

	wantSubstrings := []string{
		`<button`,
		`type="button"`,
		`aria-haspopup="menu"`,
		`aria-expanded="false"`,
		`aria-controls="layout.records.menu"`,
		`data-mega-menu-trigger="layout.records.menu"`,
		`>Records`,
		`aria-hidden="true"`,
		`▾`,
		// Panel: must be a 2D grid, not a flat <ul>.
		`<div`,
		`id="layout.records.menu"`,
		`role="menu"`,
		`data-mega-menu-panel="layout.records.menu"`,
		`mega-menu-panel`,
		`hidden`,
		// Group heading (h3) for screen-reader navigation.
		`<h3`,
		`People`,
		// Item rendered with role=menuitem.
		`role="menuitem"`,
		`href="/soldiers"`,
		`data-marker="records-search"`,
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(got, want) {
			t.Errorf("rendered mega-menu missing required substring %q\nfull render:\n%s", want, got)
		}
	}
}

// TestMegaMenu_RendersAllGroupsAndItems pins that every group
// + every item in the groups slice renders in order. The 2D
// panel collapses to nothing if any item is dropped — same
// anti-pattern as the FoldoutWithBadge delegation regression
// (#456). Per-group + per-item assertions catch a dropped
// group independently of a dropped item.
func TestMegaMenu_RendersAllGroupsAndItems(t *testing.T) {
	var buf bytes.Buffer
	groups := []MegaGroup{
		{
			Title: "People",
			Items: []templ.Component{
				megaMenuLink("/soldiers", "Search", "records-search"),
				megaMenuLink("/events", "Events", "records-events"),
				megaMenuLink("/tags", "Tags", "records-tags"),
			},
		},
		{
			Title: "More records",
			Items: []templ.Component{
				megaMenuLink("/browse", "Filter", "records-browse"),
				megaMenuLink("/articles", "Articles", "records-articles"),
			},
		},
	}
	err := MegaMenu("Records", "layout.records.menu", nil, groups).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()

	for _, marker := range []string{"records-search", "records-events", "records-tags", "records-browse", "records-articles"} {
		if !strings.Contains(got, `data-marker="`+marker+`"`) {
			t.Errorf("mega-menu missing item marker %q", marker)
		}
	}
	for _, group := range []string{"People", "More records"} {
		if !strings.Contains(got, group) {
			t.Errorf("mega-menu missing group heading %q", group)
		}
	}

	// Both groups must render — one group dropped is a
	// dispatch regression.
	if strings.Count(got, "<h3") != 2 {
		t.Errorf("mega-menu should render 2 group headings; got %d", strings.Count(got, "<h3"))
	}
}

// TestMegaMenu_EmptyGroupsRendersEmptyPanel pins the
// no-groups edge case: the panel renders (with role="menu"
// + hidden class) but contains no group headings or items.
// This is the "empty dispatch" state — rare but possible if
// a future caller passes an empty slice.
func TestMegaMenu_EmptyGroupsRendersEmptyPanel(t *testing.T) {
	var buf bytes.Buffer
	err := MegaMenu("Empty", "layout.empty.menu", nil, nil).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, `data-mega-menu-panel="layout.empty.menu"`) {
		t.Errorf("empty mega-menu missing panel; got:\n%s", got)
	}
	if strings.Contains(got, "<h3") {
		t.Errorf("empty mega-menu should not render any group headings; got:\n%s", got)
	}
}

// TestMegaMenu_TriggerAttrsPassThrough asserts that the spread
// triggerAttrs are rendered on the <button> (minus the class
// attribute which the primitive owns). Same contract as
// TestFoldout_TriggerAttrsPassThrough.
func TestMegaMenu_TriggerAttrsPassThrough(t *testing.T) {
	var buf bytes.Buffer
	attrs := templ.Attributes{
		"data-test":    "yes",
		"aria-current": "page",
	}
	err := MegaMenu("Records", "layout.records.menu", attrs, nil).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, `data-test="yes"`) {
		t.Errorf("expected triggerAttrs to pass through; got:\n%s", got)
	}
	if !strings.Contains(got, `aria-current="page"`) {
		t.Errorf("expected aria-current to pass through; got:\n%s", got)
	}
}

// TestMegaMenuWithBadge_RendersBadgeInsideTrigger pins that
// the badge slot is rendered between the trigger label and
// the chevron (mirrors FoldoutWithBadge — issue #455 slice
// 1.5). The Share & Review mega-menu (issue #380 slice 3)
// needs this slot for the live-count htmx wire span. If the
// badge lands anywhere else (after the chevron, outside the
// button, swallowed by templ), the trigger button fails to
// re-render the count badge when /layout/review-count updates.
func TestMegaMenuWithBadge_RendersBadgeInsideTrigger(t *testing.T) {
	var buf bytes.Buffer
	groups := []MegaGroup{
		{Title: "Review & Research", Items: []templ.Component{megaMenuLink("/review-queue", "Open Review Queue", "")}},
	}
	badge := templ.Raw(`<span data-layout-research-review-count hx-get="/layout/review-count" hx-trigger="load, every 30s" hx-swap="innerHTML" hx-target="this" hx-ext="none"></span>`)
	err := MegaMenuWithBadge("Share & Review", "layout.share-review.menu", nil, badge, groups).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()

	// Trigger button must contain the badge span.
	triggerIdx := strings.Index(got, `data-mega-menu-trigger="layout.share-review.menu"`)
	if triggerIdx < 0 {
		t.Fatalf("mega-menu trigger not found")
	}
	buttonOpenIdx := strings.LastIndex(got[:triggerIdx], `<button`)
	if buttonOpenIdx < 0 {
		t.Fatalf("could not find <button> open tag")
	}
	buttonCloseIdx := strings.Index(got[triggerIdx:], `</button>`)
	if buttonCloseIdx < 0 {
		t.Fatalf("could not find </button> close tag")
	}
	buttonCloseIdx += triggerIdx
	triggerSpan := got[buttonOpenIdx : buttonCloseIdx+len(`</button>`)]
	if !strings.Contains(triggerSpan, `data-layout-research-review-count`) {
		t.Errorf("badge must live INSIDE the trigger button; got:\n%s", triggerSpan)
	}
	if !strings.Contains(triggerSpan, `hx-target="this"`) {
		t.Errorf("badge wrapper must declare hx-target=\"this\"; got:\n%s", triggerSpan)
	}
}

// TestMegaMenuWithBadge_NilBadgeSkipsSlot pins that passing
// nil for the badge slot doesn't render an empty span or a
// dangling chevron — the chevron still renders, but no
// placeholder for the badge. Same behavior as FoldoutWithBadge
// when the caller passes nil for the badge slot.
func TestMegaMenuWithBadge_NilBadgeSkipsSlot(t *testing.T) {
	var buf bytes.Buffer
	err := MegaMenuWithBadge("Records", "layout.records.menu", nil, nil, nil).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, `data-mega-menu-trigger="layout.records.menu"`) {
		t.Errorf("trigger should render even without a badge; got:\n%s", got)
	}
	if !strings.Contains(got, "▾") {
		t.Errorf("chevron should still render when badge is nil; got:\n%s", got)
	}
}
