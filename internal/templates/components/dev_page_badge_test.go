package components

import (
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

// TestDevPageBadge_RendersWithPath asserts the dev badge templ
// emits the data attributes the JS side uses to find + update it,
// plus the server-side-rendered path text (no flash before JS
// hydrates). Issue #309.
func TestDevPageBadge_RendersWithPath(t *testing.T) {
	out := new(strings.Builder)
	err := DevPageBadge("/soldiers/42").Render(context.Background(), out)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := out.String()
	for _, needle := range []string{
		`data-dixie-page-badge`,
		`data-current-path="/soldiers/42"`,
		`data-dixie-page-badge-path`,
		`/soldiers/42`,
		`data-dixie-page-badge-label`,
		`42`, // labelFromPath on /soldiers/42 -> "42"
	} {
		if !strings.Contains(content, needle) {
			t.Errorf("badge missing %q; got: %s", needle, content)
		}
	}
}

// TestDevPageBadge_DefaultsHidden asserts the badge ships with
// `class="hidden"` so JS controls the visibility. Without this
// flag the badge would always show in production.
func TestDevPageBadge_DefaultsHidden(t *testing.T) {
	out := new(strings.Builder)
	err := DevPageBadge("/").Render(context.Background(), out)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := out.String()
	if !strings.Contains(content, `class="fixed bottom-3 right-3 z-30 hidden`) {
		t.Errorf("badge must default to class=\"... hidden ...\"; got: %s", content)
	}
}

// TestBreadcrumb_Renders asserts the breadcrumb templ emits the
// data attributes the JS side reads to update itself on htmx
// swaps. The crumb labels themselves are tested in the helper
// test above; this checks the component shape.
func TestBreadcrumb_Renders(t *testing.T) {
	out := new(strings.Builder)
	err := Breadcrumb("/share/queue").Render(context.Background(), out)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := out.String()
	for _, needle := range []string{
		`data-dixie-breadcrumb`,
		`data-current-path="/share/queue"`,
		`Home`,
		`Share`,
		`Queue`,
		`aria-current="page"`,
	} {
		if !strings.Contains(content, needle) {
			t.Errorf("breadcrumb missing %q; got: %s", needle, content)
		}
	}
}

// silence the unused-import warning on templ (we keep it imported
// in case future tests need to compare against templ.Component).
var _ = templ.Component(nil)
