package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestLayoutDockMirrorsParityRule pins issue #380 OQ4 lock
// (slice 4): the floating-dock Quick Nav panel mirrors the
// top-nav's flat pills + the first item of each mega-menu
// only. Foldouts/mega-menus stay top-nav exclusive; the dock
// is a flat list.
//
// Pre-#380 the dock had: Calendar, Search/Quick View, Browse,
// Review Queue, Insights, Share, Tags, Settings, Add Person
// Record (9 items). The 'Search/Quick View' label was renamed
// to 'Search' in slice 5 (issue #380 OQ7 lock); URL stays
// /soldiers.
//
// Post-#380 (slice 4) the dock has: Calendar, Search,
// Share landing, Settings, Add Person Record (5 items).
//
// Each anti-pattern is asserted individually so the test
// fails clearly when a slice author either (a) leaves a
// non-parity item in the dock or (b) drops a parity item.
func TestLayoutDockMirrorsParityRule(t *testing.T) {
	var buf bytes.Buffer
	if err := Layout("Test").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()

	// (1) The floating-dock panel exists.
	if !strings.Contains(content, `data-floating-nav-panel`) {
		t.Fatalf("layout should render the floating-dock nav panel")
	}

	// (2) Extract the dock-nav list (the <nav> inside the
	// panel). We use a substring scan + the panel-anchored
	// <nav class="mt-3 flex flex-col gap-2"> anchor.
	panelIdx := strings.Index(content, `data-floating-nav-panel`)
	if panelIdx < 0 {
		t.Fatalf("panel not found")
	}
	// Find the <nav ...> opener after the panel open.
	navOpen := strings.Index(content[panelIdx:], `<nav class="mt-3 flex flex-col gap-2"`)
	if navOpen < 0 {
		t.Fatalf("dock <nav> not found inside panel")
	}
	navOpen += panelIdx
	navClose := strings.Index(content[navOpen:], `</nav>`)
	if navClose < 0 {
		t.Fatalf("dock </nav> close not found")
	}
	navClose += navOpen
	dockNav := content[navOpen : navClose+len(`</nav>`)]

	// (3) Items the dock MUST contain (flat pills + first
	// mega-menu item each).
	mustContain := []string{
		`<a href="/calendar"`,
		`<a href="/soldiers"`,   // first item of Records mega-menu (Search)
		`<a href="/share"`,      // first item of Share & Review mega-menu (Landing)
		`<a href="/settings"`,
		`<a href="/soldiers/new"`,
	}
	for _, want := range mustContain {
		if !strings.Contains(dockNav, want) {
			t.Errorf("dock must contain %q per OQ4 parity lock; dock nav:\n%s", want, dockNav)
		}
	}

	// (4) Items the dock MUST NOT contain (buried under the
	// mega-menus; reachable via the top-nav trigger).
	mustNotContain := []string{
		`<a href="/browse"`,
		`<a href="/events"`,
		`<a href="/tags"`,
		`<a href="/articles"`,
		`<a href="/review-queue"`,
		`<a href="/insights"`,
		`<a href="/share/exports"`,
		`<a href="/share/imports"`,
		`<a href="/share/queue"`,
		`<a href="/share/sync"`,
		`<a href="/research-collections"`,
		`>Search/Quick View<`, // pre-#380 label must be gone (slice 5 renamed it to 'Search')
	}
	for _, bad := range mustNotContain {
		if strings.Contains(dockNav, bad) {
			t.Errorf("dock must NOT contain %q per OQ4 parity lock (reachable via top-nav mega-menu); dock nav:\n%s", bad, dockNav)
		}
	}
}