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