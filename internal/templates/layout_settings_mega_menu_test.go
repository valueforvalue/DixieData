// layout_settings_mega_menu_test.go -- issue #584 slice 5
// + #599 slice 1 (drop /settings/build duplication; /about
// is canonical for build info per user direction).
//
// Pins the top-nav Settings mega-menu wiring. The trigger
// sits between Share & Review and About; the panel exposes
// 5 destinations (the 5 /settings/<section> sub-pages; the
// build sub-page is gone — build info now lives on /about
// per issue #585).

package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestLayoutSettingsMegaMenuTriggerPresent pins the 5-item
// mega-menu shape. The build destination is intentionally
// absent: per issue #599, the build info now lives on /about
// (the canonical surface) and /settings/build is removed from
// routes, the menu, and the templ layer.
func TestLayoutSettingsMegaMenuTriggerPresent(t *testing.T) {
	var buf bytes.Buffer
	if err := Layout("Test").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Layout.Render: %v", err)
	}
	content := buf.String()
	for _, want := range []string{
		`data-mega-menu-trigger="layout.settings.menu"`,
		`aria-controls="layout.settings.menu"`,
		`data-mega-menu-panel="layout.settings.menu"`,
		`href="/settings/appearance"`,
		`href="/settings/updates"`,
		`href="/settings/maintenance"`,
		`href="/settings/data"`,
		`href="/settings/diagnostics"`,
		`data-marker="settings-appearance"`,
		`data-marker="settings-updates"`,
		`data-marker="settings-maintenance"`,
		`data-marker="settings-data"`,
		`data-marker="settings-diagnostics"`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("Layout missing %q in settings mega-menu wiring", want)
		}
	}
	// Issue #599 RED: the build destination must be absent
	// from the menu + the templ layer. Defensive: also assert
	// the link href is gone (a future refactor might keep the
	// data-marker but drop the href, or vice versa).
	for _, want := range []string{
		`href="/settings/build"`,
		`data-marker="settings-build"`,
	} {
		if strings.Contains(content, want) {
			t.Errorf("Layout still has build menu item %q — issue #599 (build info now lives on /about)", want)
		}
	}
}

func TestLayoutSettingsMegaMenuPosition(t *testing.T) {
	var buf bytes.Buffer
	if err := Layout("Test").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Layout.Render: %v", err)
	}
	content := buf.String()
	// Locked nav order: Records | Share & Review | Settings | About.
	shareReview := strings.Index(content, `data-mega-menu-trigger="layout.share-review.menu"`)
	settings := strings.Index(content, `data-mega-menu-trigger="layout.settings.menu"`)
	about := strings.Index(content, `data-mega-menu-trigger="layout.about.menu"`)
	if shareReview < 0 || settings < 0 || about < 0 {
		t.Fatalf("missing one of the mega-menu triggers (share-review=%d, settings=%d, about=%d)", shareReview, settings, about)
	}
	if !(shareReview < settings && settings < about) {
		t.Errorf("nav order off: share-review=%d, settings=%d, about=%d (expected share-review < settings < about)", shareReview, settings, about)
	}
}

// TestLayoutNoLegacySettingsPill pins the cleanup from #584.
// The legacy flat pill <a href="/settings" class="pill-link
// top-nav-link">Settings</a> was replaced by the Settings
// mega-menu trigger (issue #592). The floating-dock Quick Nav
// panel keeps a flat "Settings" entry by design (#380 OQ4 lock
// slice 4), so we assert against the specific top-nav marker
// class rather than all "Settings" occurrences.
func TestLayoutNoLegacySettingsPill(t *testing.T) {
	var buf bytes.Buffer
	if err := Layout("Test").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Layout.Render: %v", err)
	}
	content := buf.String()
	if strings.Contains(content, `class="pill-link top-nav-link">Settings</a>`) {
		t.Errorf("legacy flat <a class=\"pill-link top-nav-link\">Settings</a> is still in the top nav (issue #592)")
	}
}