package components

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

// TestPill_DefaultSnapshot verifies the simplest call: a label, an
// href, no extras. Matches the legacy
// `<a href="..." class="pill-link">label</a>` form.
func TestPill_DefaultSnapshot(t *testing.T) {
	var buf bytes.Buffer
	if err := Pill("Calendar", "/calendar", "", nil).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	if !strings.HasPrefix(got, `<a href="/calendar" class="pill-link">Calendar</a>`) {
		t.Fatalf("default pill snapshot drift:\n got: %q", got)
	}
}

// TestPill_ExtraClass verifies extra class tokens append after "pill-link".
func TestPill_ExtraClass(t *testing.T) {
	var buf bytes.Buffer
	if err := Pill("Next →", "/browse?page=2", "ml-2", nil).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, `class="pill-link ml-2"`) {
		t.Fatalf("pill extra-class drift:\n%s", got)
	}
	if !strings.Contains(got, `href="/browse?page=2"`) || !strings.Contains(got, `>Next →</a>`) {
		t.Fatalf("pill href/label drift:\n%s", got)
	}
}

// TestPill_AttrsPassThrough verifies hx-* and aria-* attributes appear
// on the rendered anchor. The browse pager uses hx-get + hx-target +
// aria-label extensively.
func TestPill_AttrsPassThrough(t *testing.T) {
	var buf bytes.Buffer
	err := Pill("Next →", "/browse?page=2", "", templ.Attributes{
		"hx-get":    "/browse?page=2",
		"hx-target": "#browse-results",
		"aria-label": "Next page",
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	for _, needle := range []string{
		`hx-get="/browse?page=2"`,
		`hx-target="#browse-results"`,
		`aria-label="Next page"`,
		`class="pill-link"`,
	} {
		if !strings.Contains(got, needle) {
			t.Fatalf("pill missing %q\nfull: %s", needle, got)
		}
	}
}