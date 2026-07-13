// RED-first regression net for issue #370 (chrome surfaces
// for the codename + branch). Pins the contract that every
// chrome reader (window title, CLI --version, /settings/build
// panel) includes the codename + branch so a user always
// knows what build they're looking at.
package main

import (
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/buildinfo"
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

// TestWindowTitleIncludesEmDashBetweenAppAndCodename (issue #542)
// pins the typographic polish on the OS window title. The user
// reported that the OS title bar read "DixieData First Manassas ·
// v1.2.7 · dev" — a single space between the app name and the
// codename — and asked for an em dash to match the polish
// already applied (per issue #462) to the per-page <title>,
// the top-shell brand pill, and the footer.
//
// The test pins three contracts:
//   1. The title contains "DixieData — First Manassas" with
//      a U+2014 em dash, not a space.
//   2. The title is not the legacy "DixieData First Manassas"
//      shape (with a space).
//   3. The middle-dot chain (codename · version · branch)
//      survives — the em-dash polish was scoped to the brand /
//      codename boundary, not the metadata chain.
func TestWindowTitleIncludesEmDashBetweenAppAndCodename(t *testing.T) {
	title := windowTitle()

	// Pin: the em-dash form is present.
	const wantSubstring = "DixieData \u2014 First Manassas"
	if !strings.Contains(title, wantSubstring) {
		t.Errorf("windowTitle() missing em-dash form %q:\n  got: %q", wantSubstring, title)
	}

	// Pin: the legacy space-separated form is gone. A bare
	// space between "DixieData" and "First" without the
	// em dash would mean the #462 polish was lost on this
	// surface. Look for "DixieData First" (with a space and
	// no em dash) and fail if present.
	const legacyShape = "DixieData First"
	if strings.Contains(title, legacyShape) {
		t.Errorf("windowTitle() contains legacy space-separated form %q; the em-dash polish regressed:\n  got: %q", legacyShape, title)
	}

	// Pin: the middle-dot metadata chain survives. The em-dash
	// polish was scoped to the brand/codename boundary, not
	// the codename/version/branch chain (per #462's mid-dot
	// contract). The title must still contain a `\u00b7` (·)
	// after the codename to bind the metadata.
	if !strings.Contains(title, "First Manassas \u00b7") {
		t.Errorf("windowTitle() missing mid-dot separator after codename; the metadata chain regressed:\n  got: %q", title)
	}

	// Pin: branch + version are still present (the existing
	// contract from #370). The new shape must not have dropped
	// them. buildinfo.AppVersion is the bare semver (e.g. "1.1.1",
	// no leading "v" — the Wails title format puts the "v" on the
	// first element via the AppLabel elsewhere; here we use the
	// bare form for the middle-dot chain).
	if !strings.Contains(title, buildinfo.AppVersion) {
		t.Errorf("windowTitle() missing app version %q:\n  got: %q", buildinfo.AppVersion, title)
	}
	if !strings.Contains(title, "dev") {
		t.Errorf("windowTitle() missing branch 'dev':\n  got: %q", title)
	}
}