package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestLayoutRendersRecordsMegaMenu pins issue #380 slice 2:
// the 5 flat top-nav pills (Search/Browse/Events/Articles/Tags)
// collapse into one Records mega-menu (NN/g 2D panel pattern).
// Each assertion catches a single anti-pattern:
//
//   - mega-menu trigger must render with the right aria
//     contract (mirrors Foldout's contract — issue #264)
//   - all 5 destinations must appear as menuitems inside
//     the panel (any dropped item is a regression)
//   - the 2 group headings (People + More records) must
//     render in the panel — one dropped group is a
//     regression
//   - the 5 old flat pills must be GONE from the top nav —
//     if they survive inside the nav as duplicates, the
//     mega-menu is decorative, not functional
func TestLayoutRendersRecordsMegaMenu(t *testing.T) {
	var buf bytes.Buffer
	if err := Layout("Test").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()

	// (1) Mega-menu trigger renders.
	const trigger = `data-mega-menu-trigger="layout.records.menu"`
	if !strings.Contains(content, trigger) {
		t.Fatalf("layout should render Records mega-menu trigger with %q", trigger)
	}
	if !strings.Contains(content, `aria-controls="layout.records.menu"`) {
		t.Fatalf("Records mega-menu trigger must declare aria-controls pointing at the panel id")
	}
	if !strings.Contains(content, `aria-haspopup="menu"`) {
		t.Fatalf("Records mega-menu trigger must declare aria-haspopup")
	}

	// (2) Panel renders.
	if !strings.Contains(content, `data-mega-menu-panel="layout.records.menu"`) {
		t.Fatalf("layout should render Records mega-menu panel")
	}
	if !strings.Contains(content, `role="menu"`) {
		t.Fatalf("Records mega-menu panel must carry role=\"menu\" for screen-reader nav")
	}

	// (3) All 5 destinations inside the panel. Each item carries
	// the data-marker + role="menuitem" + the original data-* hook
	// (when one existed pre-mega-menu) so downstream tests stay
	// stable.
	mustContainItems := []string{
		// People group.
		`href="/soldiers"`,
		`href="/events"`,
		`href="/tags"`,
		// More records group.
		`href="/browse"`,
		`href="/articles"`,
	}
	for _, needle := range mustContainItems {
		if !strings.Contains(content, needle) {
			t.Fatalf("Records mega-menu should contain item with %q", needle)
		}
	}
	// Each item should be marked role="menuitem" (5x — one per item).
	if got := strings.Count(content, `role="menuitem"`); got < 5 {
		t.Errorf("Records mega-menu should have at least 5 role=\"menuitem\" entries; got %d", got)
	}

	// (4) Both group headings render. One dropped group is a
	// dispatch regression — same anti-pattern as the
	// FoldoutWithBadge delegation regression (#456).
	if !strings.Contains(content, "People") {
		t.Fatalf("Records mega-menu should render the People group heading")
	}
	if !strings.Contains(content, "More records") {
		t.Fatalf("Records mega-menu should render the More records group heading")
	}

	// (5) The 5 old flat pills must be GONE from the top nav.
	// Pre-#380 the top-nav used pill-link top-nav-link for
	// each destination. After slice 2, only Calendar + Insights
	// (slice 2 keeps Insights as a flat pill — it gets absorbed
	// into the Share & Review mega-menu in slice 3) + Settings
	// remain as pill-link top-nav-link anchors. The mega-menu
	// trigger button also carries the pill-link top-nav-link
	// class so the visual shape survives — but it uses
	// <button>, not <a href>. So counting the number of <a
	// href="/soldiers"> links in the page should drop from 2
	// (top-nav + dock) to 1 (dock — slice 4 will drop that too).
	oldPills := []string{
		`<a href="/soldiers" class="pill-link top-nav-link"`,
		`<a href="/browse" class="pill-link top-nav-link"`,
		`<a href="/events" class="pill-link top-nav-link"`,
		`<a href="/articles" class="pill-link top-nav-link"`,
		`<a href="/tags" class="pill-link top-nav-link"`,
	}
	for _, pill := range oldPills {
		if strings.Contains(content, pill) {
			t.Errorf("top-nav should no longer render flat pill %q — moved into Records mega-menu", pill)
		}
	}
}

// TestLayoutRecordsMegaMenuItemDataHooks pins that the
// pre-#380 data-* hooks on each top-nav destination survive
// the move into the Records mega-menu. audit/smoke.mjs + any
// future analytics selectors depend on these hooks; losing
// them silently would break every probe without a compile
// error.
func TestLayoutRecordsMegaMenuItemDataHooks(t *testing.T) {
	var buf bytes.Buffer
	if err := Layout("Test").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()

	wantHooks := map[string]string{
		"records-search":     `href="/soldiers"`,
		"records-events":     `href="/events"`,
		"records-tags":       `href="/tags"`,
		"records-browse":     `href="/browse"`,
		"records-articles":   `href="/articles"`,
	}
	for marker, href := range wantHooks {
		want := `data-marker="` + marker + `"`
		if !strings.Contains(content, want) {
			t.Errorf("Records mega-menu should carry marker %q (item pointing at %s)", marker, href)
		}
	}
}