package templates

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/buildinfo"
)

func TestLayoutUsesLocalBootstrapScript(t *testing.T) {
	var buf bytes.Buffer
	if err := Layout("Test").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	if !strings.Contains(content, `src="/app.js"`) {
		t.Fatalf("layout should load local app.js")
	}
	if !strings.Contains(content, `href="/app.css"`) {
		t.Fatalf("layout should load local app.css")
	}
	if !strings.Contains(content, `href="/calendar"`) {
		t.Fatalf("layout should link calendar navigation to /calendar")
	}
	if strings.Contains(content, "unpkg.com/htmx.org") {
		t.Fatalf("layout should not depend on remote htmx")
	}
	if strings.Contains(content, "cdn.tailwindcss.com") {
		t.Fatalf("layout should not depend on remote tailwind")
	}
	if !strings.Contains(content, buildinfo.AppLabel()) || !strings.Contains(content, fmt.Sprintf("Schema v%d", buildinfo.SchemaVersion)) {
		t.Fatalf("layout should include app and schema versions")
	}
	if !strings.Contains(content, `data-build-identity="`) || !strings.Contains(content, buildinfo.BuildIdentity()) {
		t.Fatalf("layout should surface build identity")
	}
	if !strings.Contains(content, `data-floating-nav-toggle`) || !strings.Contains(content, "Quick Navigation") {
		t.Fatalf("layout should include floating navigation controls")
	}
	for _, needle := range []string{
		`dixiedata.layout.mode`,
		`data-layout-mode-option="auto"`,
		`data-layout-mode-option="split-screen"`,
		`data-layout-mode-status`,
		`id="feedback-modal"`,
		`data-feedback-modal`,
		`overflow-y-auto`,
		`max-h-[calc(100vh-2rem)]`,
		`sm:max-h-[calc(100vh-4rem)]`,
		`class="pill-link top-nav-link"`,
		`class="primary-button top-nav-primary"`,
		`class="secondary-button floating-dock-button"`,
		`class="primary-button floating-dock-button"`,
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("layout should include responsive foundation control %s", needle)
		}
	}
	if !strings.Contains(content, `data-scratchpad-open`) || !strings.Contains(content, "Scratch Pad") {
		t.Fatalf("layout should include floating scratch pad controls")
	}
	if !strings.Contains(content, `data-feedback-open`) || !strings.Contains(content, `data-feedback-modal`) {
		t.Fatalf("layout should include global feedback controls")
	}
	if !strings.Contains(content, `aria-required="true"`) {
		t.Fatalf("feedback textarea should carry aria-required alongside required for assistive tech")
	}
	if !strings.Contains(content, `floating-nav-panel`) || !strings.Contains(content, `max-w-[calc(100vw-2rem)]`) {
		t.Fatalf("layout should keep floating menu viewport-bounded")
	}
	if !strings.Contains(content, `class="top-shell flex`) || !strings.Contains(content, `class="app-shell"`) {
		t.Fatalf("layout should render the header as a floating top bar")
	}

	// Mobile hamburger drawer is dead UI on a Wails desktop app; it
	// must not be rendered. The inline top-nav (md+) and the floating
	// dock quick-nav panel stay.
	if strings.Contains(content, "data-top-nav-toggle") {
		t.Fatalf("layout should not render the mobile hamburger toggle")
	}
	if strings.Contains(content, "data-top-nav-drawer") {
		t.Fatalf("layout should not render the mobile hamburger drawer")
	}

	// CSS rules that used to live inline in the layout (pre-PR1) were
	// extracted to frontend/tailwind.css and compiled into app.css.
	// The Tailwind minifier drops quotes from attribute selectors, so
	// we accept either form when scanning the built CSS.
	appCSS := readCompiledAppCSS(t)
	type cssCheck struct {
		needle string
		// alternates is OR'd; matches any one pass.
		alternates []string
	}
	checks := []cssCheck{
		{needle: "layout-mode floating-dock-button selector",
			alternates: []string{
				`html[data-layout-mode="split-screen"] .floating-dock-button`,
				`html[data-layout-mode=split-screen] .floating-dock-button`,
			}},
		{needle: ".top-shell rule",
			alternates: []string{
				`.top-shell{`,
				`.top-shell `,
			}},
		{needle: "position: sticky",
			alternates: []string{
				`position:sticky`,
				`position: sticky`,
			}},
	}
	for _, c := range checks {
		ok := false
		for _, alt := range c.alternates {
			if strings.Contains(appCSS, alt) {
				ok = true
				break
			}
		}
		if !ok {
			t.Errorf("compiled app.css should include %s; tried: %v", c.needle, c.alternates)
		}
	}
}

func TestLayoutDialogsAreLabelledByTheirHeading(t *testing.T) {
	var buf bytes.Buffer
	if err := Layout("Test").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	for _, pair := range []struct {
		dialog, heading, aria string
	}{
		{`id="feedback-modal"`, `id="feedback-modal-heading"`, `aria-labelledby="feedback-modal-heading"`},
	} {
		if !strings.Contains(content, pair.dialog) {
			t.Fatalf("layout missing dialog root %s", pair.dialog)
		}
		if !strings.Contains(content, pair.heading) {
			t.Fatalf("layout missing heading id %s", pair.heading)
		}
		if !strings.Contains(content, pair.aria) {
			t.Fatalf("dialog %s should be aria-labelledby %s", pair.dialog, pair.aria)
		}
	}
}

// TestLayoutFeedbackModalIsOverlayDiv asserts the feedback modal
// renders as a <div role="dialog" aria-modal="true"> overlay.
//
// The native <dialog> element was tried in issue #117 but caused
// WebView2 focus-event reentry that crashed any native Save/Open
// dialog opened from inside the modal (and from any sibling export
// button). Reverting to the div overlay keeps the focus trap +
// ESC close working — both implemented manually in app.js —
// without leaking Chromium.Focus calls into the native dialog.
func TestLayoutFeedbackModalIsOverlayDiv(t *testing.T) {
	var buf bytes.Buffer
	if err := Layout("Test").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	needle := `<div id="feedback-modal" role="dialog" aria-modal="true"`
	if !strings.Contains(content, needle) {
		t.Fatalf("feedback modal should render as a <div role=\"dialog\" aria-modal=\"true\"> overlay; got:\n%s", content)
	}
	if strings.Contains(content, `<dialog id="feedback-modal"`) {
		t.Fatalf("feedback modal must not be a native <dialog>; it regresses to WebView2 focus-event crash")
	}
}

// readCompiledAppCSS loads frontend/app.css (gitignored build output).
// If the file does not exist (e.g. fresh clone without a CSS rebuild)
// the test fails with a clear message rather than panic-reading nil.
func readCompiledAppCSS(t *testing.T) string {
	t.Helper()
	candidates := []string{
		// Run from the repo root (e.g. via `go test ./...`).
		"frontend/app.css",
		// Run from this package directory (e.g. via `go test ./internal/templates/...`).
		"../../frontend/app.css",
	}
	for _, rel := range candidates {
		abs, err := filepath.Abs(rel)
		if err != nil {
			continue
		}
		data, err := os.ReadFile(abs)
		if err == nil {
			return string(data)
		}
	}
	t.Fatalf("frontend/app.css not found; run `make css` (or `npm run build:css`) before running this test")
	return ""
}

// TestLayoutReviewCountBadgeTargetsItself pins the htmx-target
// discipline on the review-count badge wrapper.
//
// Symptom when regressed (commit 89372a2 introduced the badge
// wrapper without considering htmx target resolution): the badge's
// hx-trigger="load" fires when the wrapper enters the DOM during
// the initial /calendar swap. htmx walks up the DOM to resolve
// hx-target via getClosestAttributeValue. If a shell <body>
// carries hx-target="body" (from frontend/index.html's load
// trigger that survived the swap because innerHTML replacement
// preserves body attrs), the badge inherits it. The badge's
// innerHTML swap then targets <body>, which wipes the entire
// layout chrome and leaves only the badge fragment as a direct
// child of <body>. The user sees a blank page with just a small
// "2" pill in the top-left.
//
// Two-pronged fix: (a) the shell <body> trigger must NOT carry
// hx-target="body" or hx-swap="outerHTML" (default innerHTML on
// the trigger element achieves the same visual result, since
// htmx 1.x/2.x note that outerHTML on body upgrades to
// innerHTML anyway); (b) the badge wrapper must declare an
// explicit hx-target so it never inherits from a future shell
// change. This test pins both invariants on the rendered output.
//
// See docs/COMMON_BUGS.md §1.12 for the full pattern.
func TestLayoutReviewCountBadgeTargetsItself(t *testing.T) {
	var buf bytes.Buffer
	if err := Layout("Test").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()

	// (a) Layout body must not carry hx-target or hx-swap attrs.
	// (templ writes them as `hx-target=` / `hx-swap=` on the body tag.)
	const bodyOpenTag = `<body class="min-h-screen"`
	bodyIdx := strings.Index(content, bodyOpenTag)
	if bodyIdx < 0 {
		t.Fatalf("layout body open tag not found")
	}
	bodyCloseIdx := strings.Index(content[bodyIdx:], `>`)
	if bodyCloseIdx < 0 {
		t.Fatalf("layout body close not found")
	}
	bodyTag := content[bodyIdx : bodyIdx+bodyCloseIdx+1]
	if strings.Contains(bodyTag, "hx-target") {
		t.Fatalf("layout <body> must not declare hx-target; nested elements would inherit it and target body instead of themselves. Got body tag:\n%s", bodyTag)
	}
	if strings.Contains(bodyTag, "hx-swap") {
		t.Fatalf("layout <body> must not declare hx-swap; nested elements would inherit it. Got body tag:\n%s", bodyTag)
	}

	// (b) Badge wrapper must declare an explicit hx-target.
	const openTag = `<span data-layout-review-count hx-get="`
	idx := strings.Index(content, openTag)
	if idx < 0 {
		t.Fatalf("layout should render the review-count badge wrapper %q", openTag)
	}
	closeIdx := strings.Index(content[idx:], `></span>`)
	if closeIdx < 0 {
		t.Fatalf("wrapper close tag not found")
	}
	wrapperTag := content[idx : idx+closeIdx+len(`></span>`)]
	if !strings.Contains(wrapperTag, `hx-target="this"`) {
		t.Fatalf("review-count badge wrapper must declare hx-target=\"this\" so its innerHTML swap does not inherit hx-target from any future shell change; got wrapper:\n%s", wrapperTag)
	}
	if !strings.Contains(wrapperTag, `hx-swap="innerHTML"`) {
		t.Fatalf("review-count badge wrapper must declare hx-swap=\"innerHTML\"; got wrapper:\n%s", wrapperTag)
	}
}
