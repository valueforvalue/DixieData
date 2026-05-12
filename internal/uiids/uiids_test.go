package uiids

import "testing"

func TestRegistryIDsAreUnique(t *testing.T) {
	seen := map[string]Surface{}
	for _, surface := range Registry {
		if surface.ID == "" {
			t.Fatal("registry contains empty ID")
		}
		if previous, ok := seen[surface.ID]; ok {
			t.Fatalf("duplicate ID %q for %q and %q", surface.ID, previous.Description, surface.Description)
		}
		seen[surface.ID] = surface
	}
}

func TestDebugEnabledUsesEnvVar(t *testing.T) {
	t.Setenv(DebugEnvVar, "1")
	if !DebugEnabled() {
		t.Fatalf("%s=1 should enable debug UI IDs", DebugEnvVar)
	}

	t.Setenv(DebugEnvVar, "false")
	if DebugEnabled() {
		t.Fatalf("%s=false should disable debug UI IDs", DebugEnvVar)
	}
}

func TestEnableFromArgsRecognizesDebugArg(t *testing.T) {
	t.Setenv(DebugEnvVar, "")
	if !EnableFromArgs([]string{DebugArg}) {
		t.Fatalf("expected %s to be recognized", DebugArg)
	}
	if !DebugEnabled() {
		t.Fatalf("%s should enable debug UI IDs", DebugArg)
	}
}

func TestEnableFromArgsIgnoresUnknownArgs(t *testing.T) {
	t.Setenv(DebugEnvVar, "")
	if EnableFromArgs([]string{"--not-a-real-flag"}) {
		t.Fatal("unexpected debug enable for unknown flag")
	}
	if DebugEnabled() {
		t.Fatal("unknown flags should not enable debug UI IDs")
	}
}
