package components

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

// TestCard_DefaultClass verifies the simplest call produces exactly
// <div class="card">. This is the byte-stability anchor for sites
// that previously wrote <div class="card"> manually.
func TestCard_DefaultClass(t *testing.T) {
	var buf bytes.Buffer
	err := Card("").Render(templ.WithChildren(context.Background(), templ.NopComponent), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got, want := buf.String(), `<div class="card"></div>`; got != want {
		t.Fatalf("default card snapshot drift:\n got: %q\nwant: %q", got, want)
	}
}

// TestCard_ExtraClass verifies that extraClass is appended after the
// "card" base class with a single space. Covers the most common
// existing pattern: `<div class="card rounded-3xl p-6">`.
func TestCard_ExtraClass(t *testing.T) {
	var buf bytes.Buffer
	err := Card("rounded-3xl p-6").Render(templ.WithChildren(context.Background(), templ.NopComponent), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got, want := buf.String(), `<div class="card rounded-3xl p-6"></div>`; got != want {
		t.Fatalf("extra class card snapshot drift:\n got: %q\nwant: %q", got, want)
	}
}

// TestCard_WithChildren verifies that caller-supplied children render
// inside the card. The snapshot shows <p>hello</p> nested in <div>.
func TestCard_WithChildren(t *testing.T) {
	var buf bytes.Buffer
	inner := templ.Raw(`<p>hello</p>`)
	err := Card("").Render(templ.WithChildren(context.Background(), inner), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, `<div class="card">`) {
		t.Fatalf("missing card wrapper:\n%s", got)
	}
	if !strings.Contains(got, `<p>hello</p>`) {
		t.Fatalf("missing child content:\n%s", got)
	}
}