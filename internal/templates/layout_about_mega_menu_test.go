// layout_about_mega_menu_test.go -- issue #585 slice 5.
//
// Pins the top-nav About mega-menu wiring. The trigger sits
// between Settings and Add Person Record; the panel exposes a
// single destination (About DixieData -> /about) per the
// locked decision (mega-menu trigger with single-item panel,
// in-page section anchors live on the page itself).

package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestLayoutAboutMegaMenuTriggerPresent pins the trigger
// button + ARIA contract. The trigger is identified by
// data-mega-menu-trigger + aria-controls pointing at the panel
// id (the same install-once marker the other top-nav mega-
// menus use).
func TestLayoutAboutMegaMenuTriggerPresent(t *testing.T) {
	var buf bytes.Buffer
	if err := Layout("Test").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Layout.Render: %v", err)
	}
	content := buf.String()
	for _, want := range []string{
		`data-mega-menu-trigger="layout.about.menu"`,
		`aria-controls="layout.about.menu"`,
		`data-mega-menu-panel="layout.about.menu"`,
		`<a role="menuitem" href="/about"`,
		`data-marker="about-overview"`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("Layout missing %q in about mega-menu wiring", want)
		}
	}
}

// TestLayoutAboutMegaMenuPosition pins the nav order: Records
// / Share & Review / Settings / About. About is the rightmost
// mega-menu (per the locked decision: far right) and lands
// AFTER the Settings mega-menu trigger.
//
// We anchor on the Settings mega-menu trigger marker
// (`data-mega-menu-trigger="layout.settings.menu"`) rather
// than the loose `href="/settings"` substring because the
// floating-dock Quick Nav panel also contains an `<a
// href="/settings">` entry by design (#380 OQ4 lock slice 4)
// that appears AFTER the top-nav About mega-menu in the
// rendered HTML. The trigger marker scopes the assertion to
// the top nav only. (Before the issue #592 cleanup, the
// stale top-nav pill link happened to satisfy the loose
// substring match; once the pill is removed, the assertion
// must use the trigger marker to stay top-nav-scoped.)
func TestLayoutAboutMegaMenuPosition(t *testing.T) {
	var buf bytes.Buffer
	if err := Layout("Test").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Layout.Render: %v", err)
	}
	content := buf.String()
	settingsIdx := strings.Index(content, `data-mega-menu-trigger="layout.settings.menu"`)
	aboutIdx := strings.Index(content, `data-mega-menu-trigger="layout.about.menu"`)
	if settingsIdx < 0 {
		t.Fatalf("settings mega-menu trigger not found in nav")
	}
	if aboutIdx < 0 {
		t.Fatalf("about mega-menu trigger not found in nav")
	}
	if settingsIdx > aboutIdx {
		t.Errorf("settings (idx %d) appears AFTER about (idx %d); expected nav order Records / Share & Review / Settings / About",
			settingsIdx, aboutIdx)
	}
}