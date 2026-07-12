package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestLayoutRendersShareReviewMegaMenu pins issue #380 slice 3:
// the 2 foldouts (Research & Review + Share) + the standalone
// Insights pill collapse into one Share & Review mega-menu
// (NN/g 2D panel pattern). Two groups: Review & Research
// (Review Queue with badge + Timeline + Research Log +
// Collections + Insights) + Share (Landing + Export + Import
// + Share Queue + Sync). The Review Queue menuitem keeps the
// data-research-review-has-count flag echo logic from the
// pre-#380 foldout (issue #460).
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
		`href="/research"`,
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
		if !strings.Contains(content, "border-review-red/60") {
			t.Errorf("has-count menuitem should carry border-review-red/60 class for the red border treatment")
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