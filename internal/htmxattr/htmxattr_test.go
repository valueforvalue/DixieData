package htmxattr

import (
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/valueforvalue/DixieData/internal/uiids"
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
	s, ok := v.(templ.SafeURL)
	if !ok {
		t.Fatalf("hx-get should be templ.SafeURL, got %T", v)
	}
	if string(s) != "/jobs/active" {
		t.Fatalf("hx-get = %q, want /jobs/active", string(s))
	}
}

func TestMuxPostOnly(t *testing.T) {
	got := Mux{Post: "/soldiers"}.Attrs()
	v, ok := got["hx-post"]
	if !ok {
		t.Fatal("hx-post missing")
	}
	s, ok := v.(templ.SafeURL)
	if !ok {
		t.Fatalf("hx-post should be templ.SafeURL, got %T", v)
	}
	if string(s) != "/soldiers" {
		t.Fatalf("hx-post = %q, want /soldiers", string(s))
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
	got := Mux{Select: "#countsForm"}.Attrs()
	if got["hx-select"] != "#countsForm" {
		t.Fatalf("hx-select = %v", got["hx-select"])
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
	// All registry IDs must work as hx-target without panic. Loop the
	// registry to make sure none have a weird character that breaks
	// htmx.
	for _, s := range uiids.Registry {
		target := "#" + s.ID
		got := Mux{Target: target}.Attrs()
		if got["hx-target"] != target {
			t.Fatalf("registry target %q should pass through verbatim", target)
		}
	}
}

func TestMuxAdHocTargetDoesNotPanic(t *testing.T) {
	// IDs that aren't in the registry are allowed; we don't want to
	// break transient panels that don't earn a registry entry.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ad-hoc target should not panic, got %v", r)
		}
	}()
	Mux{Target: "#feedback-form"}.Attrs()
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

func TestRegistryHasKnownSurfaces(t *testing.T) {
	// Sanity: the registry must contain at least the most-used
	// surfaces; if a future rename drops one, the route builders that
	// emit these IDs will panic in TestMuxTargetFromRegistryResolvesCleanly.
	for _, id := range []string{
		uiids.PageBrowse,
		uiids.PageSoldierDetail,
		uiids.PanelBrowseResults,
		uiids.PanelJobStatus,
		uiids.OverlayFloatingMenu,
	} {
		if !uiids.Has(id) {
			t.Fatalf("registry missing expected surface %q", id)
		}
	}
}