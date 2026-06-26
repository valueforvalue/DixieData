package components

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

// TestButtonContent_RendersChildren verifies that ButtonContent
// renders caller-supplied children inside the button (unlike Button,
// which takes a string label).
func TestButtonContent_RendersChildren(t *testing.T) {
	var buf bytes.Buffer
	child := templ.Raw(`<span class="font-bold">Export JSON</span><span class="text-xs">Hierarchical</span>`)
	err := ButtonContent(ButtonPrimary, "justify-between text-left", templ.Attributes{
		"hx-post":   "/export/json",
		"hx-target": "this",
		"hx-swap":   "none",
	}).Render(templ.WithChildren(context.Background(), child), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	for _, needle := range []string{
		`hx-post="/export/json"`,
		`hx-target="this"`,
		`hx-swap="none"`,
		`class="primary-button justify-between text-left"`,
		`<span class="font-bold">Export JSON</span>`,
		`<span class="text-xs">Hierarchical</span>`,
	} {
		if !strings.Contains(got, needle) {
			t.Fatalf("ButtonContent missing %q\nfull: %s", needle, got)
		}
	}
}

// TestButtonContent_TypeNotDuplicatedFromAttrs mirrors
// TestButton_TypeNotDuplicatedFromAttrs for the children variant.
func TestButtonContent_TypeNotDuplicatedFromAttrs(t *testing.T) {
	var buf bytes.Buffer
	if err := ButtonContent(ButtonPrimary, "", templ.Attributes{
		"type": "submit",
	}).Render(templ.WithChildren(context.Background(), templ.Raw("Save")), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	if strings.Count(got, "type=") != 1 {
		t.Fatalf("expected exactly one type= attribute, got %d:\n%s", strings.Count(got, "type="), got)
	}
}