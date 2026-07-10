package components

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

// TestFoldout_ARIAContract asserts that the rendered trigger
// button + panel carry the ARIA contract documented in
// foldout.templ. This locks:
//   - trigger is a <button> with aria-haspopup="menu",
//     aria-expanded="false" on initial render, and
//     aria-controls pointing at the panel's id
//   - panel is a <ul> with role="menu" and the items inside
//     are <a> with role="menuitem" (not <div>s with click
//     handlers — screen readers + middle-click + cmd-click
//     must work)
//   - the panel starts hidden (.foldout-panel .hidden)
//   - the chevron is aria-hidden (decorative)
func TestFoldout_ARIAContract(t *testing.T) {
	var buf bytes.Buffer
	if err := Foldout("Share", "layout.share.menu", nil).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()

	wantSubstrings := []string{
		`<button`,
		`type="button"`,
		`aria-haspopup="menu"`,
		`aria-expanded="false"`,
		`aria-controls="layout.share.menu"`,
		`data-foldout-trigger="layout.share.menu"`,
		`>Share`,
		`aria-hidden="true"`,
		`▾`,
		`<ul`,
		`id="layout.share.menu"`,
		`role="menu"`,
		`data-foldout-panel="layout.share.menu"`,
		`foldout-panel`,
		`hidden`,
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(got, want) {
			t.Errorf("rendered foldout missing required substring %q\nfull render:\n%s", want, got)
		}
	}
}

// TestFoldout_TriggerAttrsPassThrough asserts that the spread
// triggerAttrs are rendered on the <button> (minus the class
// attribute which the primitive owns). This is what the
// aria-current=page contract depends on — the layout passes
// nil today, but a future caller that wants to mark the
// trigger active on a non-/-prefixed path can pass the
// aria-current via triggerAttrs.
func TestFoldout_TriggerAttrsPassThrough(t *testing.T) {
	var buf bytes.Buffer
	attrs := templ.Attributes{
		"data-test":    "yes",
		"aria-current": "page",
	}
	if err := Foldout("Share", "layout.share.menu", attrs).Render(context.Background(), &buf); err != nil {
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

// TestFoldout_PanelResponsiveSizing locks in the responsive
// sizing contract documented in issue #288. The panel class
// must carry BOTH a floor (min-w-[14rem]) so the menu items
// stay readable on wide screens AND a cap (max-w-[calc(100vw-2rem)])
// so the panel can never overflow the viewport on narrow
// screens. This is the same three-cap pattern as
// .floating-nav-panel (internal/templates/layout.templ:129) —
// preferred width, min floor, max cap.
//
// If a future change strips the max-w token, this test fails.
// The CSS-layer enforcement (html[data-layout-mode=split-screen]
// .foldout-panel rule) lives in frontend/tailwind.css and is
// covered by a separate assertion in internal/templates/layout_test.go.
func TestFoldout_PanelResponsiveSizing(t *testing.T) {
	var buf bytes.Buffer
	if err := Foldout("Share", "layout.share.menu", nil).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()

	wantSubstrings := []string{
		`foldout-panel`,
		`min-w-[14rem]`,
		`max-w-[calc(100vw-2rem)]`,
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(got, want) {
			t.Errorf("rendered foldout panel missing required responsive-sizing class %q (issue #288)\nfull render:\n%s", want, got)
		}
	}

	// Regression net: confirm the min-w floor was not removed
	// by the responsive cap. The cap is additive, not a
	// replacement — losing the floor lets the panel collapse
	// below the 14rem threshold on wide screens and break
	// the menu-item readability contract.
	if !strings.Contains(got, `min-w-[14rem]`) {
		t.Errorf("foldout panel must keep its min-w-[14rem] floor on wide screens; the max-w cap is additive (issue #288)")
	}
}

// TestFoldout_WithoutBadge_DoesNotEmitBadgeMarker pins the
// issue #455 slice 1.5 invariant: when a Foldout caller does
// NOT pass a badge component (i.e. the old Share-style API),
// the rendered trigger must NOT carry the data-foldout-has-badge
// marker and must NOT emit any badge wrapper. Share foldout
// (and any future non-badged consumer) keeps the exact wire
// shape today — no htmx span, no extra div, just the label +
// chevron. If a future refactor accidentally promotes the
// trigger to always render a badge wrapper, this test fails.
func TestFoldout_WithoutBadge_DoesNotEmitBadgeMarker(t *testing.T) {
	var buf bytes.Buffer
	if err := Foldout("Share", "layout.share.menu", nil).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	if strings.Contains(got, `data-foldout-has-badge`) {
		t.Errorf("non-badged Foldout must NOT emit data-foldout-has-badge; got:\n%s", got)
	}
	if strings.Contains(got, `data-layout-research-review-count`) {
		t.Errorf("non-badged Foldout must NOT emit the R&R review-count span; got:\n%s", got)
	}
}

// TestFoldoutWithBadge_RendersBadgeInsideTrigger verifies the
// issue #455 slice 1.5 variant emits the caller's badge
// content INSIDE the trigger button (so it's visible when the
// panel is collapsed) BETWEEN the label and the chevron (so
// the chevron keeps its trailing position). The data-foldout-has-badge
// marker on the trigger lets the layout-level aria-label sync
// in frontend/app.js identify trigger buttons that need their
// aria-label updated when the count changes.
func TestFoldoutWithBadge_RendersBadgeInsideTrigger(t *testing.T) {
	var buf bytes.Buffer
	const badgeHTML = `<span data-layout-research-review-count class="foldout-badge" hx-get="/layout/review-count" hx-trigger="load, every 30s" hx-swap="innerHTML" hx-target="this">0</span>`
	badge := templ.Raw(badgeHTML)
	if err := FoldoutWithBadge("Research & Review", "layout.research.menu", nil, badge).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()

	// 1) Trigger must carry the marker.
	if !strings.Contains(got, `data-foldout-has-badge="1"`) {
		t.Errorf("badged foldout trigger must carry data-foldout-has-badge marker; got:\n%s", got)
	}
	// 2) The badge span must live inside the <button>...</button>
	// block (between the label "Research & Review" and the chevron).
	btnOpenIdx := strings.Index(got, `<button`)
	btnCloseIdx := strings.Index(got, `</button>`)
	if btnOpenIdx < 0 || btnCloseIdx < 0 {
		t.Fatalf("badged foldout missing <button> tag; got:\n%s", got)
	}
	btn := got[btnOpenIdx : btnCloseIdx+len(`</button>`)]
	if !strings.Contains(btn, `Research &amp; Review`) {
		t.Errorf("trigger button missing label; got:\n%s", btn)
	}
	if !strings.Contains(btn, `data-layout-research-review-count`) {
		t.Errorf("trigger button missing badge htmx span; got:\n%s", btn)
	}
	if !strings.Contains(btn, `hx-target="this"`) {
		t.Errorf("badge span must declare hx-target=\"this\" so its innerHTML swap does not inherit hx-target from any future shell change; got:\n%s", btn)
	}
	if !strings.Contains(btn, `hx-swap="innerHTML"`) {
		t.Errorf("badge span must declare hx-swap=\"innerHTML\"; got:\n%s", btn)
	}
	// 3) Chevron must still be inside the trigger, AFTER the badge.
	chevronIdx := strings.Index(btn, `▾`)
	badgeIdx := strings.Index(btn, `data-layout-research-review-count`)
	if chevronIdx < 0 || badgeIdx < 0 {
		t.Fatalf("trigger missing chevron or badge; got:\n%s", btn)
	}
	if badgeIdx > chevronIdx {
		t.Errorf("badge must render BEFORE the chevron inside the trigger button; got:\n%s", btn)
	}
	// 4) The badge content must NOT appear outside the <button> (i.e.
	// inside the <ul> menu would be wrong).
	panelOpenIdx := strings.Index(got, `<ul`)
	if panelOpenIdx < 0 {
		t.Fatalf("badged foldout missing <ul> panel; got:\n%s", got)
	}
	panel := got[panelOpenIdx:]
	if strings.Contains(panel, `data-layout-research-review-count`) {
		t.Errorf("badge must NOT leak into the <ul> panel; got:\n%s", panel)
	}
}

// TestFoldoutWithBadge_NilBadgeMatchesFoldout pins the
// nil-badge branch of FoldoutWithBadge renders the same wire
// shape as Foldout (minus the new marker, since we only set
// the marker on non-nil badge). The Share foldout keeps its
// existing byte-stable output.
func TestFoldoutWithBadge_NilBadgeMatchesFoldout(t *testing.T) {
	var a, b bytes.Buffer
	if err := Foldout("Share", "layout.share.menu", nil).Render(context.Background(), &a); err != nil {
		t.Fatalf("Foldout Render: %v", err)
	}
	if err := FoldoutWithBadge("Share", "layout.share.menu", nil, nil).Render(context.Background(), &b); err != nil {
		t.Fatalf("FoldoutWithBadge(nil) Render: %v", err)
	}
	// Both renders must contain the same wire-shape markers
	// (no data-foldout-has-badge, no badge span).
	for _, want := range []string{
		`data-foldout-trigger="layout.share.menu"`,
		`>Share`,
		`▾`,
		`role="menu"`,
		`foldout-panel`,
	} {
		if !strings.Contains(a.String(), want) || !strings.Contains(b.String(), want) {
			t.Errorf("nil-badge FoldoutWithBadge missing %q\n  Foldout: %s\n  FoldoutWithBadge(nil): %s", want, a.String(), b.String())
		}
	}
}

// TestFoldout_RendersChildren is the regression net for the
// Share top-nav foldout rendering a blank panel (issue #456).
//
// In commit 8438e1f (issue #455 slice 1.5), Foldout was
// rewritten as a one-line delegation to FoldoutWithBadge:
//   templ Foldout(...) { @FoldoutWithBadge(..., nil) }
// templ generate compiles that wrapper by capturing the
// caller-supplied children into Var1, calling
// templ.ClearChildren(ctx), then rendering FoldoutWithBadge.
// The captured Var1 is never re-injected, so the inner
// templ.GetChildren(ctx) returns nil and every <li>
// menuitem the caller passes in the children block is
// silently dropped. Result: the Share top-nav render becomes
// `<ul id="layout.share.menu" ...></ul>` — open the panel and
// it is empty.
//
// The pre-existing test surface (TestFoldout_ARIAContract,
// TestFoldout_WithoutBadge_DoesNotEmitBadgeMarker,
// TestFoldoutWithBadge_NilBadgeMatchesFoldout) calls
// Foldout("Share", "layout.share.menu", nil) WITHOUT a
// children block, so the bug is invisible to the unit
// suite today. This test fails on the broken codegen and
// passes once Foldout's body renders children inline (no
// delegating wrapper).
//
// Locked invariants:
//   - the first <li role="none"> inside the panel contains
//     the first menuitem's <a role="menuitem"> with the
//     expected href + label verbatim
//   - the second <li> follows the first in order
//   - both menuitems live INSIDE the <ul role="menu">
//     panel, not anywhere else in the rendered HTML
//   - the trigger button text is unaffected by the children
//     block (no leak from the panel into the button)
func TestFoldout_RendersChildren(t *testing.T) {
	var buf bytes.Buffer
	children := templ.Raw(
		`<li role="none"><a href="/share/exports" role="menuitem" class="foldout-menuitem pill-link justify-start w-full" data-share-menu-export>Export</a></li>` +
			`<li role="none"><a href="/share/imports" role="menuitem" class="foldout-menuitem pill-link justify-start w-full" data-share-menu-import>Import</a></li>`,
	)
	err := Foldout("Share", "layout.share.menu", nil).Render(templ.WithChildren(context.Background(), children), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()

	panelOpenIdx := strings.Index(got, "<ul")
	if panelOpenIdx < 0 {
		t.Fatalf("rendered foldout missing <ul> panel; got:\n%s", got)
	}
	panelOpenEnd := strings.Index(got[panelOpenIdx:], ">")
	if panelOpenEnd < 0 {
		t.Fatalf("rendered <ul> is malformed; got:\n%s", got)
	}
	panelCloseIdx := strings.LastIndex(got, "</ul>")
	if panelCloseIdx < 0 || panelCloseIdx <= panelOpenIdx+panelOpenEnd {
		t.Fatalf("rendered foldout missing </ul> close tag; got:\n%s", got)
	}
	panel := got[panelOpenIdx+panelOpenEnd+1 : panelCloseIdx]

	// 1) Both menuitems land INSIDE the panel.
	if !strings.Contains(panel, `data-share-menu-export`) {
		t.Errorf("first menuitem (Export) missing inside <ul role=\"menu\"> panel; got panel:\n%s\nfull render:\n%s", panel, got)
	}
	if !strings.Contains(panel, `data-share-menu-import`) {
		t.Errorf("second menuitem (Import) missing inside <ul role=\"menu\"> panel; got panel:\n%s\nfull render:\n%s", panel, got)
	}
	// 2) Hrefs are rendered verbatim.
	if !strings.Contains(panel, `href="/share/exports"`) {
		t.Errorf("first menuitem href not rendered; got panel:\n%s", panel)
	}
	if !strings.Contains(panel, `href="/share/imports"`) {
		t.Errorf("second menuitem href not rendered; got panel:\n%s", panel)
	}
	// 3) Order is preserved — Export appears before Import.
	exportIdx := strings.Index(panel, `data-share-menu-export`)
	importIdx := strings.Index(panel, `data-share-menu-import`)
	if exportIdx < 0 || importIdx < 0 || exportIdx > importIdx {
		t.Errorf("menuitems out of order (Export must precede Import); export idx=%d import idx=%d panel:\n%s", exportIdx, importIdx, panel)
	}
	// 4) Items are wrapped in <li role="none">.
	if !strings.Contains(panel, `<li role="none">`) {
		t.Errorf("menuitems must be wrapped in <li role=\"none\"> per the WAI-ARIA menu pattern; got panel:\n%s", panel)
	}
	// 5) Menuitems must NOT leak into the trigger button.
	btnOpenIdx := strings.Index(got, `<button`)
	btnCloseIdx := strings.Index(got, `</button>`)
	if btnOpenIdx < 0 || btnCloseIdx < 0 || btnCloseIdx <= btnOpenIdx {
		t.Fatalf("rendered foldout missing <button> block; got:\n%s", got)
	}
	btn := got[btnOpenIdx : btnCloseIdx+len(`</button>`)]
	if strings.Contains(btn, `data-share-menu-export`) || strings.Contains(btn, `data-share-menu-import`) {
		t.Errorf("menuitems leaked into the trigger button; got:\n%s", btn)
	}
}