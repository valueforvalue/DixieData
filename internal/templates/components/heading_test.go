// heading_test.go — issue #577. Byte-stability snapshot +
// behavioral pins for the Heading primitive.
//
//   - Per-level snapshot: the rendered HTML matches the legacy
//     inline class string byte-for-byte (no trailing whitespace,
//     no reordered attrs). The drift target — `recovery.templ`'s
//     h1 missing `font-display` and `--ink` — is what happens
//     when there's no primitive: future contributors copy
//     whichever shape they find first. Pinning the byte-stable
//     rendering here makes the drift impossible at the call
//     site level.
//   - ExtraClass appends after the canonical class (so a call
//     site can add page-specific spacing without rewriting the
//     level contract).
//   - templ.Attributes (data-*, id, aria-*) land in the tag at
//     the position the legacy inline form had them — class
//     first, attrs after, matching the round-trip the existing
//     snapshot tests expect.
//   - The h1 + h2 + h3 branches each emit their semantic tag
//     (verified via the snapshot's `<h1`, `<h2`, `<h3` prefix).
package components

import (
	"bytes"
	"strings"
	"testing"
)

func TestHeading_H1RendersCanonicalClass(t *testing.T) {
	component := Heading(H1, "Articles", "", nil)
	var buf bytes.Buffer
	if err := component.Render(t.Context(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	got := strings.TrimSpace(buf.String())
	want := `<h1 class="font-display text-3xl font-semibold text-[color:var(--ink)]">Articles</h1>`
	if got != want {
		t.Errorf("Heading(H1) rendered differs:\n got:  %s\n want: %s", got, want)
	}
}

func TestHeading_H2RendersCanonicalClass(t *testing.T) {
	component := Heading(H2, "Person Records", "", nil)
	var buf bytes.Buffer
	if err := component.Render(t.Context(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	got := strings.TrimSpace(buf.String())
	want := `<h2 class="font-display text-xl font-semibold text-[color:var(--ink)]">Person Records</h2>`
	if got != want {
		t.Errorf("Heading(H2) rendered differs:\n got:  %s\n want: %s", got, want)
	}
}

func TestHeading_H3RendersCanonicalClass(t *testing.T) {
	component := Heading(H3, "Sub-section", "", nil)
	var buf bytes.Buffer
	if err := component.Render(t.Context(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	got := strings.TrimSpace(buf.String())
	want := `<h3 class="text-lg font-semibold text-[color:var(--ink)]">Sub-section</h3>`
	if got != want {
		t.Errorf("Heading(H3) rendered differs:\n got:  %s\n want: %s", got, want)
	}
}

// TestHeading_ExtraClassAppendsAfterCanonical covers the page-
// specific spacing pattern. The extraClass goes AFTER the
// canonical class so a future contributor who adds a new
// heading can't accidentally shadow the level contract by
// reordering Tailwind purge groups.
func TestHeading_ExtraClassAppendsAfterCanonical(t *testing.T) {
	component := Heading(H1, "Articles", "mt-8", nil)
	var buf bytes.Buffer
	if err := component.Render(t.Context(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	got := strings.TrimSpace(buf.String())
	want := `<h1 class="font-display text-3xl font-semibold text-[color:var(--ink)] mt-8">Articles</h1>`
	if got != want {
		t.Errorf("Heading(H1, _, \"mt-8\") rendered differs:\n got:  %s\n want: %s", got, want)
	}
}

// TestHeading_AttrsAfterClass pins the attribute order:
// class first (the level contract), attrs after (the caller's
// data-* / id / aria-*). Existing snapshot tests grep for
// `<h1 class="..." data-article-title` as the trailing fragment
// so reversing this order would silently break them.
func TestHeading_AttrsAfterClass(t *testing.T) {
	component := Heading(H1, "Articles", "", map[string]any{
		"data-section": "articles-index",
		"id":           "page-title",
	})
	var buf bytes.Buffer
	if err := component.Render(t.Context(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	got := strings.TrimSpace(buf.String())
	want := `<h1 class="font-display text-3xl font-semibold text-[color:var(--ink)]" data-section="articles-index" id="page-title">Articles</h1>`
	if got != want {
		t.Errorf("Heading with attrs rendered differs:\n got:  %s\n want: %s", got, want)
	}
}
