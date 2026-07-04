package htmxattr

import (
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/htmlids"
)

func TestMuxZeroValueEmitsNothing(t *testing.T) {
	got := Mux{}.Attrs()
	if len(got) != 0 {
		t.Fatalf("zero-value Mux should emit no attributes, got %v", got)
	}
}

func TestMuxGetOnly(t *testing.T) {
	got := Mux{Get: "/jobs/active"}.Attrs()
	if len(got) != 1 {
		t.Fatalf("expected 1 attribute, got %d: %v", len(got), got)
	}
	v, ok := got["hx-get"]
	if !ok {
		t.Fatal("hx-get missing")
	}
	// Plain string, NOT templ.SafeURL. SafeURL is silently dropped
	// by templ.RenderAttributes' type switch — see Attrs() comment.
	s, ok := v.(string)
	if !ok {
		t.Fatalf("hx-get should be string, got %T", v)
	}
	if s != "/jobs/active" {
		t.Fatalf("hx-get = %q, want /jobs/active", s)
	}
}

func TestMuxPostOnly(t *testing.T) {
	got := Mux{Post: "/soldiers"}.Attrs()
	v, ok := got["hx-post"]
	if !ok {
		t.Fatal("hx-post missing")
	}
	s, ok := v.(string)
	if !ok {
		t.Fatalf("hx-post should be string, got %T", v)
	}
	if s != "/soldiers" {
		t.Fatalf("hx-post = %q, want /soldiers", s)
	}
}

func TestMuxTargetEmittedVerbatim(t *testing.T) {
	got := Mux{Target: "#browse-results"}.Attrs()
	v, ok := got["hx-target"]
	if !ok {
		t.Fatal("hx-target missing")
	}
	if v != "#browse-results" {
		t.Fatalf("hx-target = %q, want #browse-results", v)
	}
}

func TestMuxSwapAllowedValues(t *testing.T) {
	for _, swap := range []string{"innerHTML", "outerHTML", "beforebegin", "afterbegin", "beforeend", "afterend", "delete", "none"} {
		t.Run(swap, func(t *testing.T) {
			got := Mux{Swap: swap}.Attrs()
			if got["hx-swap"] != swap {
				t.Fatalf("hx-swap = %v, want %q", got["hx-swap"], swap)
			}
		})
	}
}

func TestMuxSwapEmptyOmitsAttribute(t *testing.T) {
	got := Mux{Get: "/x", Swap: ""}.Attrs()
	if _, ok := got["hx-swap"]; ok {
		t.Fatalf("hx-swap should be omitted when empty")
	}
}

func TestMuxSwapInvalidPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for invalid swap value")
		} else if !strings.Contains(r.(string), "invalid hx-swap") {
			t.Fatalf("panic message = %q, want substring 'invalid hx-swap'", r)
		}
	}()
	Mux{Swap: "nonsense"}.Attrs()
}

func TestMuxTriggerEmitted(t *testing.T) {
	got := Mux{Trigger: "load, every 3s"}.Attrs()
	if got["hx-trigger"] != "load, every 3s" {
		t.Fatalf("hx-trigger = %v, want 'load, every 3s'", got["hx-trigger"])
	}
}

func TestMuxConfirmEmitted(t *testing.T) {
	got := Mux{Confirm: "Are you sure?"}.Attrs()
	if got["hx-confirm"] != "Are you sure?" {
		t.Fatalf("hx-confirm = %v", got["hx-confirm"])
	}
}

func TestMuxSelectEmitted(t *testing.T) {
	// Use a selector that's registered in htmlids so the dev-build
	// panic does not fire — this test verifies Select rendering,
	// not target validation.
	got := Mux{Select: "#browse-results"}.Attrs()
	if got["hx-select"] != "#browse-results" {
		t.Fatalf("hx-select = %v, want #browse-results", got["hx-select"])
	}
}

func TestMuxEmptyValuesOmitted(t *testing.T) {
	got := Mux{
		Get:     "/x",
		Post:    "",  // empty
		Target:  "",  // empty
		Swap:    "",  // empty
		Trigger: "",  // empty
		Select:  "",  // empty
		Confirm: "",  // empty
	}.Attrs()
	if len(got) != 1 {
		t.Fatalf("expected 1 attribute, got %d: %v", len(got), got)
	}
	if _, ok := got["hx-get"]; !ok {
		t.Fatal("hx-get missing")
	}
}

func TestMuxWhitespaceTreatedAsEmpty(t *testing.T) {
	got := Mux{Get: "   "}.Attrs()
	if _, ok := got["hx-get"]; ok {
		t.Fatalf("hx-get should be omitted when whitespace-only")
	}
}

func TestMuxTargetFromRegistryResolvesCleanly(t *testing.T) {
	// All htmlids registry selectors must work as hx-target without
	// panic. Loop the registry to make sure none have a weird
	// character that breaks htmx.
	for _, s := range htmlids.Registry {
		target := "#" + s.ID
		got := Mux{Target: target}.Attrs()
		if got["hx-target"] != target {
			t.Fatalf("registry target %q should pass through verbatim", target)
		}
	}
}

func TestMuxPanicsOnUnknownRegistryTarget(t *testing.T) {
	// #typo is the canonical case: a Target id that's not in the
	// htmlids registry. Dev-build panic fires (issue #316 slice 4).
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for unknown registry target, got none")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("panic value = %T (%v), want string", r, r)
		}
		if !strings.Contains(msg, "htmlids registry") {
			t.Fatalf("panic message = %q, want substring 'htmlids registry'", msg)
		}
		if !strings.Contains(msg, "#typo") {
			t.Fatalf("panic message = %q, want substring '#typo'", msg)
		}
	}()
	Mux{Target: "#typo"}.Attrs()
}

// TestMuxAcceptsRegisteredSelectors covers every selector
// currently in the htmlids registry; mirrors the registry's
// TestRegistryHasKnownSelectors test so a future addition to one
// forces the other to be updated.
func TestMuxAcceptsRegisteredSelectors(t *testing.T) {
	for _, id := range htmlids.Registry {
		target := "#" + id.ID
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("registered selector %q should not panic, got %v", target, r)
				}
			}()
			got := Mux{Target: target}.Attrs()
			if got["hx-target"] != target {
				t.Fatalf("registered selector %q should pass through verbatim", target)
			}
		}()
	}
}

// TestMuxTargetNonHashSelectorsPass verifies that non-# selectors
// (body, [data-...], .cls, this) never trigger the htmlids
// validation. Symmetric with the audit probe slice-2 walker; both
// treat the same set of selectors as legitimate.
func TestMuxTargetNonHashSelectorsPass(t *testing.T) {
	cases := []string{"body", "[data-jobs-progress-region]", ".cls", "this"}
	for _, sel := range cases {
		t.Run(sel, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("non-# selector %q should not panic, got %v", sel, r)
				}
			}()
			got := Mux{Target: sel}.Attrs()
			if got["hx-target"] != sel {
				t.Fatalf("non-# selector %q should pass through verbatim", sel)
			}
		})
	}
}

func TestMuxFullFields(t *testing.T) {
	m := Mux{
		Get:     "/jobs/active",
		Target:  "#browse-results",
		Swap:    "outerHTML",
		Trigger: "load, every 3s",
	}
	got := m.Attrs()
	for _, key := range []string{"hx-get", "hx-target", "hx-swap", "hx-trigger"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("expected %s in attrs", key)
		}
	}
	if len(got) != 4 {
		t.Fatalf("expected 4 attributes, got %d: %v", len(got), got)
	}
}

// TestRegistryHasKnownSurfaces moved to internal/uiids/uiids_test.go
// as TestRegistryIncludesResponsiveFoundationSurfaces. The
// uiids.Set membership check lives with the registry it tests,
// not in a downstream package's test file.