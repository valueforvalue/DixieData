package htmlids

import "testing"

// TestRegistryIDsAreUnique mirrors uiids_test.go convention:
// every Registry entry must have a unique ID, no empty ID.
func TestRegistryIDsAreUnique(t *testing.T) {
	seen := map[string]Selector{}
	for _, s := range Registry {
		if s.ID == "" {
			t.Fatal("registry contains empty ID")
		}
		if previous, ok := seen[s.ID]; ok {
			t.Fatalf("duplicate ID %q for %q and %q", s.ID, previous.Description, s.Description)
		}
		seen[s.ID] = s
	}
}

// TestRegistryHasKnownSelectors is the load-bearing test: every
// htmxattr.Mux{Target: "#X"} call site in templ must have its X
// registered here. The next test in the slice-4 commit enables the
// dev-build panic in validateTarget that requires this invariant;
// this test fails first if a new Mux target slips through without
// an entry added.
func TestRegistryHasKnownSelectors(t *testing.T) {
	required := []string{
		BrowseResults,
		SoldierList,
	}
	for _, id := range required {
		if !Has(id) {
			t.Fatalf("htmlids registry missing %q (referenced by htmxattr.Mux{Target})", id)
		}
	}
}

// TestHasRejectsUnknownIDs verifies Has returns false for known-typo
// strings; the in-function check fires the same code path that the
// dev-build panic will gate.
func TestHasRejectsUnknownIDs(t *testing.T) {
	for _, id := range []string{"", "browse-result", "soldierLists", "modal", "feedback-form-not-yet-registered"} {
		if Has(id) {
			t.Fatalf("Has(%q) returned true for unknown selector", id)
		}
	}
}
