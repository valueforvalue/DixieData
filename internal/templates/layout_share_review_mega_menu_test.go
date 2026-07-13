package templates

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestLayoutRendersShareReviewMegaMenu pins issue #380 slice 3:
// the 2 foldouts (Research & Review + Share) + the standalone
// Insights pill collapse into one Share & Review mega-menu
// (NN/g 2D panel pattern). Two groups: Review & Research
// (Review Queue with badge + Archive Inventory + Timeline +
// Research Log + Collections + Insights) + Share (Landing +
// Export + Import + Share Queue + Sync). The Review Queue
// menuitem keeps the data-research-review-has-count flag
// echo logic from the pre-#380 foldout (issue #460).
//
// Issue #549: the "Change Person…" menuitem (which linked
// to /research, the picker landing itself) is REMOVED. Every
// soldier-scoped R&R sub-page already picks the person from
// the URL or its own browse/recents affordance; /research IS
// the picker landing, so the menuitem added no affordance
// the sub-pages did not already have. The RED assertion
// below pins that data-research-menu-change-person is gone.
//
// Slice 3 also retires the 2 old foldout UIIDs (LayoutShareMenu
// + LayoutResearchMenu) and the 2 foldout UIID constant
// references inside layout_test.go. The live-count badge wire
// (data-layout-research-review-count + hx-get + hx-trigger
// + hx-swap + hx-target) moves from the old foldout trigger
// onto the new mega-menu trigger button — same span, same
// wire shape, same 30s poll.
func TestLayoutRendersShareReviewMegaMenu(t *testing.T) {
	var buf bytes.Buffer
	if err := Layout("Test").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()

	// (1) Mega-menu trigger renders.
	if !strings.Contains(content, `data-mega-menu-trigger="layout.share-review.menu"`) {
		t.Fatalf("layout should render Share & Review mega-menu trigger")
	}
	if !strings.Contains(content, `aria-controls="layout.share-review.menu"`) {
		t.Fatalf("Share & Review mega-menu trigger must declare aria-controls")
	}
	if !strings.Contains(content, `aria-haspopup="menu"`) {
		t.Fatalf("Share & Review mega-menu trigger must declare aria-haspopup")
	}

	// (2) Panel renders.
	if !strings.Contains(content, `data-mega-menu-panel="layout.share-review.menu"`) {
		t.Fatalf("layout should render Share & Review mega-menu panel")
	}

	// (3) Both group headings render. templ escapes & as &amp;
	// in the rendered output, so check for either form.
	if !strings.Contains(content, "Review &amp; Research") && !strings.Contains(content, "Review & Research") {
		t.Fatalf("Share & Review mega-menu should render the 'Review & Research' group heading")
	}
	if !strings.Contains(content, ">Share<") {
		t.Fatalf("Share & Review mega-menu should render the 'Share' group heading as an <h3>")
	}

	// (4) All destinations inside the panel. Each item carries
	// the same data-* hook (data-research-menu-* / data-share-*)
	// the old foldouts had so existing audit probes + analytics
	// selectors survive the move.
	mustContainItems := []string{
		// Review & Research group.
		`href="/review-queue"`,
		`href="/research-collections"`,
		`href="/insights"`,
		// Share group — /share is the NEW first item (was a
		// footgun before #380 — no nav path from /jobs/{id}
		// back to the landing without going through the
		// floating dock).
		`href="/share"`,
		`href="/share/exports"`,
		`href="/share/imports"`,
		`href="/share/queue"`,
		`href="/share/sync"`,
	}
	for _, needle := range mustContainItems {
		if !strings.Contains(content, needle) {
			t.Errorf("Share & Review mega-menu should contain item with %q", needle)
		}
	}

	// Issue #549: "Change Person…" menuitem is REMOVED. Every
	// soldier-scoped R&R sub-page already picks the person from
	// the URL or its own browse/recents affordance, and /research
	// itself IS the picker landing, so the mega-menu item added
	// no affordance the sub-pages did not already have. The
	// redundant <li> with data-research-menu-change-person
	// must be gone from the rendered HTML so the audit probe's
	// expectedLabels regression net (smoke_mega_menu_nav.mjs)
	// does not see the label.
	if strings.Contains(content, `data-research-menu-change-person`) {
		t.Errorf("Share & Review mega-menu must not render the 'Change Person…' item (issue #549); data-research-menu-change-person marker should be gone")
	}

	// (5) The OLD foldout panels must be GONE — both
	// layout.share.menu + layout.research.menu UIIDs are
	// retired. If either survives, the slice is incomplete.
	if strings.Contains(content, `data-foldout-trigger="layout.share.menu"`) {
		t.Errorf("layout should no longer render the Share foldout trigger; relocated to Share & Review mega-menu")
	}
	if strings.Contains(content, `data-foldout-trigger="layout.research.menu"`) {
		t.Errorf("layout should no longer render the Research & Review foldout trigger; relocated to Share & Review mega-menu")
	}
	if strings.Contains(content, `data-foldout-panel="layout.share.menu"`) {
		t.Errorf("layout should no longer render the Share foldout panel")
	}
	if strings.Contains(content, `data-foldout-panel="layout.research.menu"`) {
		t.Errorf("layout should no longer render the Research & Review foldout panel")
	}

	// (6) The standalone Insights top-nav pill is GONE —
	// Insights moved into the Share & Review mega-menu.
	if strings.Contains(content, `<a href="/insights" class="pill-link top-nav-link"`) {
		t.Errorf("Insights top-nav pill should be gone — relocated to Share & Review mega-menu")
	}
}

// TestLayoutShareReviewMegaMenuBadgeWireRelocated pins that
// the live-count badge wire (issue #455 slice 1.5) follows
// the trigger when it moves from the old foldout to the new
// mega-menu. The wire shape is byte-identical: span class +
// hx-get + hx-trigger + hx-swap + hx-target="this".
func TestLayoutShareReviewMegaMenuBadgeWireRelocated(t *testing.T) {
	var buf bytes.Buffer
	if err := Layout("Test").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()

	// (a) Badge wrapper must exist.
	const openTag = `<span data-layout-research-review-count hx-get="`
	idx := strings.Index(content, openTag)
	if idx < 0 {
		t.Fatalf("layout should render the share-review-mega-menu review-count badge wrapper %q", openTag)
	}
	closeIdx := strings.Index(content[idx:], `></span>`)
	if closeIdx < 0 {
		t.Fatalf("wrapper close tag not found")
	}
	wrapperTag := content[idx : idx+closeIdx+len(`></span>`)]
	if !strings.Contains(wrapperTag, `hx-target="this"`) {
		t.Errorf("review-count badge wrapper must declare hx-target=\"this\"; got:\n%s", wrapperTag)
	}
	if !strings.Contains(wrapperTag, `hx-swap="innerHTML"`) {
		t.Errorf("review-count badge wrapper must declare hx-swap=\"innerHTML\"; got:\n%s", wrapperTag)
	}

	// (b) The badge span must live INSIDE the new mega-menu
	// trigger button — between the "Share & Review" label
	// and the chevron. If it's anywhere else (e.g. leftover
	// at the old foldout location), the trigger won't
	// re-render the badge when the count updates.
	triggerIdx := strings.Index(content, `data-mega-menu-trigger="layout.share-review.menu"`)
	if triggerIdx < 0 {
		t.Fatalf("mega-menu trigger not found")
	}
	// Find the trigger button's <button ...> ... </button> span.
	// Templ emits <button ...>...</button>; we need the badge
	// to appear after the trigger's <button open tag and
	// before the matching </button>.
	buttonOpenIdx := strings.LastIndex(content[:triggerIdx], `<button`)
	if buttonOpenIdx < 0 {
		t.Fatalf("could not find <button> open tag for mega-menu trigger")
	}
	buttonCloseIdx := strings.Index(content[triggerIdx:], `</button>`)
	if buttonCloseIdx < 0 {
		t.Fatalf("could not find </button> close tag for mega-menu trigger")
	}
	buttonCloseIdx += triggerIdx
	triggerSpan := content[buttonOpenIdx : buttonCloseIdx+len(`</button>`)]
	if !strings.Contains(triggerSpan, `data-layout-research-review-count`) {
		t.Errorf("review-count badge must live INSIDE the mega-menu trigger button so the count badge updates with the trigger label; got button:\n%s", triggerSpan)
	}
}

// TestLayoutShareReviewMegaMenuResearchMenuItemHasFlag pins
// that the data-research-review-has-count flag echo logic
// (issue #460) survives the move from the old foldout into
// the new mega-menu. Default state (no open reviews): no
// flag. Open state: flag + red border.
func TestLayoutShareReviewMegaMenuResearchMenuItemHasFlag(t *testing.T) {
	t.Run("default state carries no flag", func(t *testing.T) {
		if LayoutHasOpenReviewFromContext(context.Background()) {
			t.Fatalf("LayoutHasOpenReviewFromContext should default to false")
		}
		var buf bytes.Buffer
		if err := Layout("Test").Render(context.Background(), &buf); err != nil {
			t.Fatalf("Render: %v", err)
		}
		content := buf.String()
		if !strings.Contains(content, `data-research-menu-review-queue`) {
			t.Fatalf("layout should render the Open Review Queue menuitem (data-research-menu-review-queue hook survives slice 3)")
		}
		if strings.Contains(content, "data-research-review-has-count") {
			t.Fatalf("default state should NOT carry data-research-review-has-count:\n%s", content)
		}
	})

	t.Run("open count sets the flag on the menuitem", func(t *testing.T) {
		ctx := WithLayoutHasOpenReview(context.Background(), true)
		var buf bytes.Buffer
		if err := Layout("Test").Render(ctx, &buf); err != nil {
			t.Fatalf("Render: %v", err)
		}
		content := buf.String()
		if !strings.Contains(content, "data-research-review-has-count") {
			t.Fatalf("expected data-research-review-has-count on the menuitem:\n%s", content)
		}
		// Issue #472 follow-up: red border is now border-2 (thicker)
		// and the bg is 0.32 alpha so the cue reads as "this needs
		// attention" at a glance instead of pink-on-pink.
		if !strings.Contains(content, "border-2 border-review-red") {
			t.Errorf("has-count menuitem should carry border-2 border-review-red for the prominent red border treatment")
		}
		if !strings.Contains(content, "bg-review-red/[0.32]") {
			t.Errorf("has-count menuitem should carry bg-review-red/[0.32] for the bumped background alpha")
		}
		// Issue #472: the has-count menuitem must NOT set a
		// static text-[#fbe1de] color (it blended into the
		// review-red pill background). The .mega-menu-item
		// rule in frontend/tailwind.css owns the text color
		// and themes it per data-theme.
		if strings.Contains(content, "text-[#fbe1de]") {
			t.Errorf("has-count menuitem should not set text-[#fbe1de] (issue #472 pink-on-pink); .mega-menu-item rule in tailwind.css owns the color")
			t.Fatalf("expected red border classes on the menuitem:\n%s", content)
		}
	})
}

// TestReviewQueueMenuItemPerThemeRedUrgency pins the per-theme
// red urgency treatment for the "Open Review Queue" mega-menu
// menuitem. The Default theme gets the red cue from Tailwind
// utility classes on the markup (border-2 border-review-red +
// bg-review-red/[0.32]); High Contrast and Soft render the
// menuitem inside a light panel (HC = white, Soft = parchment)
// so the utility classes' red-on-dark doesn't show through —
// the cue needs an explicit override per theme.
//
// Bug history: when issue #380 slice 3 relocated the menuitem
// from the retired R&R foldout to the Share & Review mega-menu,
// the pre-existing per-theme CSS overrides at
// `.foldout-menuitem[data-research-review-has-count]` were left
// behind (the menuitem no longer carries the .foldout-menuitem
// class) and no replacement selectors were added. In HC + Soft
// the menuitem rendered with the generic panel bg (white in HC,
// parchment in Soft) and no red urgency cue at all — the same
// defect class as the original issue #472 pink-on-pink report,
// but for the missing-themes case instead of the low-alpha case.
//
// The fix: add `.mega-menu-item[data-research-review-has-count]`
// base + hover selectors under [data-theme="high-contrast"] and
// [data-theme="soft"] in frontend/tailwind.css. Each carries a
// distinct light-red bg + dark red border + deep red text tuned
// to its panel surface (HC = #fde0dc / #6f2c26 / #6f2c26;
// Soft = #f4d7d2 / #6f2c26 / #4a1d18).
//
// This test is a source-scan: it walks frontend/tailwind.css
// and asserts each required selector is present. A regression
// that drops one of the four selectors (or relocates the
// menuitem back to .foldout-menuitem) fails the test with the
// exact missing selector in the message.
func TestReviewQueueMenuItemPerThemeRedUrgency(t *testing.T) {
	css := readTailwindCSSForTemplates(t)
	required := []struct {
		selector string
		why      string
	}{
		{
			selector: `html[data-theme="high-contrast"] .mega-menu-item[data-research-review-has-count]`,
			why:      "HC base: deep red on light-red bg, on the white panel",
		},
		{
			selector: `html[data-theme="high-contrast"] .mega-menu-item[data-research-review-has-count]:hover`,
			why:      "HC hover: bumped bg + darker red text",
		},
		{
			selector: `html[data-theme="soft"] .mega-menu-item[data-research-review-has-count]`,
			why:      "Soft base: deep red on warm-light-red bg, on the parchment panel",
		},
		{
			selector: `html[data-theme="soft"] .mega-menu-item[data-research-review-has-count]:hover`,
			why:      "Soft hover: bumped bg + darker red text",
		},
	}
	for _, r := range required {
		if !strings.Contains(css, r.selector) {
			t.Errorf("tailwind.css is missing required per-theme review-queue urgency selector %q (%s); the Open Review Queue menuitem will render with no red urgency cue in this theme", r.selector, r.why)
		}
	}

	// Defense in depth: pin the menuitem's selector targets the
	// mega-menu class, not the orphaned foldout class. If a future
	// refactor renames `.mega-menu-item` back to `.foldout-menuitem`,
	// these per-theme overrides must follow.
	if strings.Contains(css, `html[data-theme="high-contrast"] .foldout-menuitem[data-research-review-has-count]`) {
		t.Errorf("tailwind.css still carries the orphaned .foldout-menuitem HC override (the menuitem moved to .mega-menu-item in #380 slice 3); the override is dead code")
	}
	if strings.Contains(css, `html[data-theme="soft"] .foldout-menuitem[data-research-review-has-count]`) {
		t.Errorf("tailwind.css still carries the orphaned .foldout-menuitem Soft override (the menuitem moved to .mega-menu-item in #380 slice 3); the override is dead code")
	}
}

// readTailwindCSSForTemplates locates frontend/tailwind.css
// relative to this test file. Mirrors readTailwindCSS in
// internal/theme/theme_css_test.go so each test package can
// find the file from runtime.Caller without a shared helper.
func readTailwindCSSForTemplates(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	// thisFile = .../internal/templates/layout_share_review_mega_menu_test.go
	pkgDir := filepath.Dir(thisFile)
	repoRoot := filepath.Dir(filepath.Dir(pkgDir))
	cssPath := filepath.Join(repoRoot, "frontend", "tailwind.css")
	data, err := os.ReadFile(cssPath)
	if err != nil {
		t.Fatalf("read %s: %v", cssPath, err)
	}
	return string(data)
}