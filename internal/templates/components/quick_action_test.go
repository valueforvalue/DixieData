package components

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

// TestQuickActionRendersAsAnchor locks the a11y contract: the
// tile MUST be a real <a> element so screen readers, middle-
// click, and cmd-click all work without extra JS wiring.
func TestQuickActionRendersAsAnchor(t *testing.T) {
	var buf bytes.Buffer
	if err := QuickAction("Export JSON", "Hierarchical w/ version metadata", "/share#export-section", "", nil).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, `<a href="/share#export-section"`) {
		t.Errorf("expected <a href> with the deep-link anchor; got:\n%s", got)
	}
	if !strings.Contains(got, `Export JSON`) {
		t.Errorf("expected label to render; got:\n%s", got)
	}
	if !strings.Contains(got, `Hierarchical w/ version metadata`) {
		t.Errorf("expected description to render; got:\n%s", got)
	}
	if !strings.Contains(got, `aria-hidden="true"`) {
		t.Errorf("expected decorative arrow to be aria-hidden; got:\n%s", got)
	}
}

// TestQuickActionAttrsPassThrough asserts the caller's
// attrs (data-action, data-dixie-submit, etc.) render on
// the <a> element, minus the class attribute which the
// primitive owns.
func TestQuickActionAttrsPassThrough(t *testing.T) {
	var buf bytes.Buffer
	attrs := templ.Attributes{
		"data-action":        "/export/json",
		"data-dixie-submit":  "true",
		"data-progress-label": "Exporting JSON…",
	}
	if err := QuickAction("Export JSON", "", "/export/json", "", attrs).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	for _, want := range []string{`data-action="/export/json"`, `data-dixie-submit="true"`, `data-progress-label="Exporting JSON…"`} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered tile missing attr %q; got:\n%s", want, got)
		}
	}
}