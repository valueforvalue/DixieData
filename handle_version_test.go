// RED-first regression net for issue #370 (chrome surfaces
// for the codename + branch). Pins the contract that every
// chrome reader (window title, CLI --version, /settings/build
// panel) includes the codename + branch so a user always
// knows what build they're looking at.
package main

import (
	"strings"
	"testing"
)

func TestHandleVersionFlagIncludesCodenameAndBranch(t *testing.T) {
	out, done := handleVersionFlag([]string{"dixiedata", "--version"})
	if !done {
		t.Fatalf("--version flag not detected")
	}
	// Must include the codename.
	if !strings.Contains(out, "First Manassas") {
		t.Errorf("handleVersionFlag output missing codename 'First Manassas':\n%s", out)
	}
	// Must include the branch (the dev default is "dev").
	if !strings.Contains(out, "dev") {
		t.Errorf("handleVersionFlag output missing branch 'dev':\n%s", out)
	}
	// Must still include the numeric app version.
	if !strings.Contains(out, "v1.") {
		t.Errorf("handleVersionFlag output missing app version 'v1.':\n%s", out)
	}
}