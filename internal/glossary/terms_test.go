// Package glossary tests — issue #564 slice 1 regression net.
//
// One test (#620) asserts the registry stays in lock-step with
// the canonical glossary headings in CONTEXT.md. The other
// tests pin registry invariants (slug uniqueness, kebab case,
// related-link integrity, alphabetical order, lookup
// round-trip).
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
	"bufio"
	"bytes"
	"os"
	"regexp"
	"sort"
	"strings"
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

// contextHeadingRE matches a glossary heading in CONTEXT.md.
// The canonical shape is `**Term**:` on a line by itself
// (followed by an indented definition block). Rejects prose
// bold words like "release counter" (no colon) and "Floor
// (regression gate, enforced by CI):" (parenthetical on
// the same line).
//
// Captures the term text in group 1.
var contextHeadingRE = regexp.MustCompile(`^\*\*([^*]+)\*\*:\s*$`)

// TestRegistryMatchesContextMD — issue #620 regression net.
//
// Asserts every term in the Registry() is also a glossary
// heading in CONTEXT.md (the single source of truth per
// AGENTS.md). The reverse direction (every CONTEXT heading
// appears in the registry) is not strictly required:
// CONTEXT.md may contain prose-only references that the
// registry has not yet absorbed (slice 2+ work).
//
// The check fires when a contributor adds a Term to
// terms.go but forgets the matching heading in CONTEXT.md,
// or vice versa. Without this test the drift is silent:
// /about renders the term, but a future agent reading
// CONTEXT.md misses it, and a future glossary audit
// disagrees with the live page.
func TestRegistryMatchesContextMD(t *testing.T) {
	const contextPath = "../../CONTEXT.md"
	raw, err := os.ReadFile(contextPath)
	if err != nil {
		t.Fatalf("read CONTEXT.md: %v — the registry sync test cannot run without the source-of-truth doc", err)
	}

	contextHeadings := make(map[string]bool)
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	for scanner.Scan() {
		line := scanner.Text()
		match := contextHeadingRE.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		term := strings.TrimSpace(match[1])
		if term == "" {
			continue
		}
		contextHeadings[term] = true
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan CONTEXT.md: %v", err)
	}

	registry := Registry()
	missing := make([]string, 0)
	for _, term := range registry {
		if !contextHeadings[term.Term] {
			missing = append(missing, term.Term)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("registry contains %d term(s) not present as `**Term**:` headings in CONTEXT.md: %v — fix by adding the missing heading(s) to CONTEXT.md or removing the term from the registry (per AGENTS.md, CONTEXT.md is the single source of truth)",
			len(missing), missing)
	}

	// Informational: report CONTEXT-only headings (terms
	// mentioned in CONTEXT.md that the registry has not
	// absorbed). Not a failure — slice 2+ work absorbs
	// these. Print via t.Logf so CI surfaces the delta.
	var contextOnly []string
	for term := range contextHeadings {
		found := false
		for _, reg := range registry {
			if reg.Term == term {
				found = true
				break
			}
		}
		if !found {
			contextOnly = append(contextOnly, term)
		}
	}
	if len(contextOnly) > 0 {
		sort.Strings(contextOnly)
		t.Logf("informational: CONTEXT.md has %d heading(s) not yet in the registry (slice 2+ backlog): %v", len(contextOnly), contextOnly)
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
