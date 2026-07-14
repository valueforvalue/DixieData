package jobs

import (
	"strings"
	"testing"
)

// TestKindLabelRoutesThroughRegistry pins the canonical entry
// point for the "kind → label" mapping (issue #575). All three
// call paths — the (Job).DisplayLabel method, the templ
// jobLabel wrapper, and direct callers reading from jobs.KindLabel —
// must agree on the same string for both registered kinds and
// the humanizeKind fallback. Adding a new kind to KindRegistry
// is the single source-of-truth edit; this test guards both
// paths from drifting.
func TestKindLabelRoutesThroughRegistry(t *testing.T) {
	cases := map[string]string{
		"static_archive": "Static web archive",
		"database_pdf":   "Printable archive PDF",
		"unknown_kind":   "Unknown Kind",
		"":               "",
	}
	for kind, want := range cases {
		if got := KindLabel(kind); got != want {
			t.Errorf("KindLabel(%q) = %q, want %q", kind, got, want)
		}
		if got := (Job{Kind: kind}).DisplayLabel(); got != want {
			t.Errorf("Job{%q}.DisplayLabel() = %q, want %q (must agree with KindLabel)", kind, got, want)
		}
	}
}

// TestKindLabelUnknownFallsBackToHumanize guarantees the
// fail-soft shape: an unregistered kind still returns a readable
// label (via humanizeKind), never a raw snake_case string. This
// pins the #556 slice 1 invariant at the KindLabel entry point
// so a regression in humanizeKind can't slip in unnoticed.
func TestKindLabelUnknownFallsBackToHumanize(t *testing.T) {
	got := KindLabel("snake_case_kind_not_in_registry")
	if got == "" {
		t.Fatal("KindLabel must return a non-empty fallback for unknown kinds")
	}
	if strings.Contains(got, "_") {
		t.Errorf("KindLabel fallback %q contains an underscore (must humanize)", got)
	}
}
