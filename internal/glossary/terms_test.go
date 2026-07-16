// Package glossary tests — issue #564 slice 1 regression net.
//
// Pins the registry's invariants that every slice in this
// issue (and every future contributor) relies on:
//
//   - Every term has a unique Slug (no collisions; the
//     disclosure popover uses Slug as the lookup key).
//   - Every term has non-empty Short + Full + Term (no
//     half-populated rows).
//   - Every Slug is kebab-case and matches the Anchor
//     prefix (a refactor that drops the kebab escape
//     fails the test, not the user).
//   - Every Related slug is itself a registered term (no
//     dangling "see also" links).
//   - Registry() returns the terms in a stable alphabetical
//     order (deterministic render order — the /about
//     page must render in the same sequence every time).
//   - LookupBySlug round-trips: every Slug in the
//     registry is findable.
package glossary

import (
	"regexp"
	"testing"
)

// TestRegistryUniqueSlugs ensures no two terms share a slug.
// Adding a duplicate slug (e.g. "person-record" twice by
// accident) trips this test, not the production disclosure
// popover.
func TestRegistryUniqueSlugs(t *testing.T) {
	reg := Registry()
	seen := make(map[string]bool, len(reg))
	for _, term := range reg {
		if seen[term.Slug] {
			t.Errorf("duplicate slug %q on term %q — the disclosure popover uses Slug as the lookup key; collisions would misroute clicks", term.Slug, term.Term)
		}
		seen[term.Slug] = true
	}
}

// TestRegistryNonEmptyFields ensures every term carries all
// three text fields. A term with an empty Short would render
// a blank disclosure; an empty Full would render a blank
// /about row; an empty Term would render an unanchored row.
func TestRegistryNonEmptyFields(t *testing.T) {
	for _, term := range Registry() {
		if term.Slug == "" {
			t.Errorf("term %q has empty Slug — Slug is required for the kebab anchor", term.Term)
		}
		if term.Term == "" {
			t.Errorf("slug %q has empty display Term — /about would render an unanchored row", term.Slug)
		}
		if term.Short == "" {
			t.Errorf("slug %q (%s) has empty Short — disclosures would show a blank popover", term.Slug, term.Term)
		}
		if term.Full == "" {
			t.Errorf("slug %q (%s) has empty Full — /about would render a blank row", term.Slug, term.Term)
		}
	}
}

// kebabSlugRE matches lowercase letters + hyphens only.
// Slugs are path-safe and lowercase by convention (the
// HTML id derives directly from the slug).
var kebabSlugRE = regexp.MustCompile(`^[a-z]+(-[a-z]+)*$`)

// TestRegistrySlugsAreKebabCase ensures every Slug matches
// the kebab-case + URL-safe contract. A slug like
// "Person Record" would break /about#glossary-<slug>
// because spaces aren't valid in fragment identifiers.
func TestRegistrySlugsAreKebabCase(t *testing.T) {
	for _, term := range Registry() {
		if !kebabSlugRE.MatchString(term.Slug) {
			t.Errorf("slug %q on term %q is not kebab-case — fragment identifiers (URL #about.glossary-<slug>) require URL-safe slugs", term.Slug, term.Term)
		}
	}
}

// TestRegistryRelatedSlugsAreRegistered ensures every
// slug in Related is itself a slug in the registry.
// A dangling "see also" link would 404 the user's click.
func TestRegistryRelatedSlugsAreRegistered(t *testing.T) {
	for _, term := range Registry() {
		for _, rel := range term.Related {
			if _, ok := LookupBySlug(rel); !ok {
				t.Errorf("term %q has Related slug %q which is not in the registry — the /about 'See also' link would 404", term.Term, rel)
			}
		}
	}
}

// TestRegistryIsAlphabetical ensures the registry returns
// the same sequence every call — /about must render
// deterministically. Sorting by Term name (case-insensitive)
// is the locked ordering.
func TestRegistryIsAlphabetical(t *testing.T) {
	first := Registry()
	second := Registry()
	for i := range first {
		if first[i].Slug != second[i].Slug {
			t.Errorf("registry returned a non-deterministic order between calls — call #1 slot %d is %q, call #2 is %q. The package must sort every time it returns the slice.", i, first[i].Slug, second[i].Slug)
		}
		// Verify each call is sorted itself.
		if i == 0 {
			continue
		}
		prev := first[i-1]
		cur := first[i]
		prevName := lower(prev.Term)
		curName := lower(cur.Term)
		if curName < prevName {
			t.Errorf("registry not sorted alphabetically: %q (slot %d) comes before %q (slot %d)", cur.Term, i, prev.Term, i-1)
		}
	}
}

// TestLookupBySlugRoundTrip ensures every slug in the
// registry can be resolved by LookupBySlug. The disclosure
// popover (slice 2) calls LookupBySlug on click; if the
// lookup misses, the popover shows an empty state instead
// of the term's Short.
func TestLookupBySlugRoundTrip(t *testing.T) {
	for _, term := range Registry() {
		got, ok := LookupBySlug(term.Slug)
		if !ok {
			t.Errorf("LookupBySlug(%q) returned ok=false — every registered slug must be resolvable", term.Slug)
			continue
		}
		if got.Slug != term.Slug {
			t.Errorf("LookupBySlug(%q) returned wrong term (got %q)", term.Slug, got.Slug)
		}
	}
}

// TestLookupBySlugUnknownReturnsMiss ensures unknown slugs
// miss cleanly. A future apply site that mistypes a slug
// must NOT silently render a blank popover.
func TestLookupBySlugUnknownReturnsMiss(t *testing.T) {
	if _, ok := LookupBySlug("definitely-not-a-real-term"); ok {
		t.Errorf("LookupBySlug(definitely-not-a-real-term) returned ok=true — unknown slugs must miss cleanly so the disclosure popover can render a friendly fallback")
	}
	if _, ok := LookupBySlug(""); ok {
		t.Errorf("LookupBySlug(\"\") returned ok=true — empty slug must miss")
	}
}

// TestTermAnchorFull pins the anchor prefix. The /about
// section navigates to /about#about.glossary-<slug>; a
// future refactor that changes the prefix would break every
// cross-link.
func TestTermAnchorFull(t *testing.T) {
	term := Term{Slug: "person-record"}
	got := term.AnchorFull()
	want := "about.glossary-person-record"
	if got != want {
		t.Errorf("AnchorFull() = %q, want %q — a refactor that changes this prefix breaks every /about#glossary cross-link", got, want)
	}
}

// lower returns a case-insensitive comparison key.
//
// Inlined here rather than imported as strings.ToLower so
// the test reads at one glance.
func lower(s string) string {
	out := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		out[i] = c
	}
	return string(out)
}
