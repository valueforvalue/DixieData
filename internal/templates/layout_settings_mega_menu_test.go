// layout_settings_mega_menu_test.go -- issue #584 slice 5.
//
// Pins the top-nav Settings mega-menu wiring. The trigger
// sits between Share & Review and About; the panel exposes
// 6 destinations (the 6 /settings/<section> sub-pages).

package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

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
		`href="/settings/build"`,
		`href="/settings/diagnostics"`,
		`data-marker="settings-appearance"`,
		`data-marker="settings-updates"`,
		`data-marker="settings-maintenance"`,
		`data-marker="settings-data"`,
		`data-marker="settings-build"`,
		`data-marker="settings-diagnostics"`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("Layout missing %q in settings mega-menu wiring", want)
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