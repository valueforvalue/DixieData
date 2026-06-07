package templates

import (
	"bytes"
	"context"
	"fmt"
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
		`html[data-layout-mode="split-screen"] .floating-dock-button`,
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
	if !strings.Contains(content, `floating-nav-panel`) || !strings.Contains(content, `max-w-[min(100vw-2rem`) {
		t.Fatalf("layout should keep floating menu viewport-bounded")
	}
	if !strings.Contains(content, `.top-shell {`) || !strings.Contains(content, `position: sticky;`) || !strings.Contains(content, `class="top-shell flex`) || !strings.Contains(content, `class="app-shell"`) {
		t.Fatalf("layout should render the header as a floating top bar")
	}
}
