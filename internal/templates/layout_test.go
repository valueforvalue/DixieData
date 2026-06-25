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
		`data-ui-id="overlay.floating.menu"`,
		`data-ui-id="overlay.feedback.modal"`,
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
