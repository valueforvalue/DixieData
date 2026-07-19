package components

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

// TestCardVariants consolidates the per-shape card snapshot tests into
// one table-driven case. Each row pins a different Card call shape
// (default class, extra class, children) against its expected render.
// This is the byte-stability anchor for sites that previously wrote
// <div class="card ..."> manually.
func TestCardVariants(t *testing.T) {
	cases := []struct {
		name      string
		extraCls  string
		children  templ.Component
		wantExact string   // when set, asserts byte-equality
		wantHas   []string // when set, asserts each substring is present
	}{
		{
			// Simplest call produces exactly <div class="card"></div>.
			name:      "default_class",
			wantExact: `<div class="card"></div>`,
		},
		{
			// Extra class is appended after the "card" base class with
			// a single space. Covers `<div class="card rounded-3xl p-6">`.
			name:      "extra_class",
			extraCls:  "rounded-3xl p-6",
			wantExact: `<div class="card rounded-3xl p-6"></div>`,
		},
		{
			// Caller-supplied children render inside the card.
			name:     "with_children",
			children: templ.Raw(`<p>hello</p>`),
			wantHas:  []string{`<div class="card">`, `<p>hello</p>`},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			child := c.children
			if child == nil {
				child = templ.NopComponent
			}
			var buf bytes.Buffer
			err := Card(c.extraCls).Render(templ.WithChildren(context.Background(), child), &buf)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			got := buf.String()
			if c.wantExact != "" {
				if got != c.wantExact {
					t.Fatalf("%s snapshot drift:\n got: %q\nwant: %q", c.name, got, c.wantExact)
				}
			}
			for _, needle := range c.wantHas {
				if !strings.Contains(got, needle) {
					t.Fatalf("%s missing %q\nfull: %s", c.name, needle, got)
				}
			}
		})
	}
}
