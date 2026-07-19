package htmxattr

import (
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/htmlids"
)

// TestMuxAttributes consolidates the per-field Mux tests into one
// table-driven case. Each row exercises one Mux field → one
// attribute mapping. Rows that need richer assertions (panic
// recovery, len/typed value checks, attribute-omission checks)
// use the optional check function.
func TestMuxAttributes(t *testing.T) {
	cases := []struct {
		name string
		mux  Mux
		// wantAttr is the attribute name to inspect in got.
		// Empty for rows that check overall behavior (zero-value,
		// omission, panic).
		wantAttr string
		// wantValue is the expected string value for wantAttr.
		// Empty when the assertion is non-value-shaped.
		wantValue string
		// check runs after the basic wantAttr/wantValue comparison
		// when the row needs richer assertions (typed value,
		// omission, panic recovery). It receives the Attrs() map.
		check func(t *testing.T, got map[string]any)
	}{
		{
			name: "zero_value_emits_nothing",
			mux:  Mux{},
			check: func(t *testing.T, got map[string]any) {
				if len(got) != 0 {
					t.Fatalf("zero-value Mux should emit no attributes, got %v", got)
				}
			},
		},
		{
			name:      "get_only",
			mux:       Mux{Get: "/jobs/active"},
			wantAttr:  "hx-get",
			wantValue: "/jobs/active",
			check: func(t *testing.T, got map[string]any) {
				if len(got) != 1 {
					t.Fatalf("expected 1 attribute, got %d: %v", len(got), got)
				}
				// Plain string, NOT templ.SafeURL. SafeURL is silently dropped
				// by templ.RenderAttributes' type switch — see Attrs() comment.
				if _, ok := got["hx-get"].(string); !ok {
					t.Fatalf("hx-get should be string, got %T", got["hx-get"])
				}
			},
		},
		{
			name:      "post_only",
			mux:       Mux{Post: "/soldiers"},
			wantAttr:  "hx-post",
			wantValue: "/soldiers",
			check: func(t *testing.T, got map[string]any) {
				if _, ok := got["hx-post"].(string); !ok {
					t.Fatalf("hx-post should be string, got %T", got["hx-post"])
				}
			},
		},
		{
			name:      "target_emitted_verbatim",
			mux:       Mux{Target: "#browse-results"},
			wantAttr:  "hx-target",
			wantValue: "#browse-results",
		},
		{
			name:      "trigger_emitted",
			mux:       Mux{Trigger: "load, every 3s"},
			wantAttr:  "hx-trigger",
			wantValue: "load, every 3s",
		},
		{
			name:      "confirm_emitted",
			mux:       Mux{Confirm: "Are you sure?"},
			wantAttr:  "hx-confirm",
			wantValue: "Are you sure?",
		},
		{
			// Use a selector that's registered in htmlids so the
			// dev-build panic does not fire — this row verifies
			// Select rendering, not target validation.
			name:      "select_emitted",
			mux:       Mux{Select: "#browse-results"},
			wantAttr:  "hx-select",
			wantValue: "#browse-results",
		},
		{
			name: "swap_empty_omits_attribute",
			mux:  Mux{Get: "/x", Swap: ""},
			check: func(t *testing.T, got map[string]any) {
				if _, ok := got["hx-swap"]; ok {
					t.Fatalf("hx-swap should be omitted when empty")
				}
			},
		},
		{
			name: "empty_values_omitted",
			mux: Mux{
				Get:     "/x",
				Post:    "", // empty
				Target:  "", // empty
				Swap:    "", // empty
				Trigger: "", // empty
				Select:  "", // empty
				Confirm: "", // empty
			},
			check: func(t *testing.T, got map[string]any) {
				if len(got) != 1 {
					t.Fatalf("expected 1 attribute, got %d: %v", len(got), got)
				}
				if _, ok := got["hx-get"]; !ok {
					t.Fatal("hx-get missing")
				}
			},
		},
		{
			name: "whitespace_treated_as_empty",
			mux:  Mux{Get: "   "},
			check: func(t *testing.T, got map[string]any) {
				if _, ok := got["hx-get"]; ok {
					t.Fatalf("hx-get should be omitted when whitespace-only")
				}
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.mux.Attrs()
			if c.wantAttr != "" {
				if got[c.wantAttr] != c.wantValue {
					t.Fatalf("%s = %v, want %q", c.wantAttr, got[c.wantAttr], c.wantValue)
				}
			}
			if c.check != nil {
				c.check(t, got)
			}
		})
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
