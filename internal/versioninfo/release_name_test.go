// RED-first regression net for issue #370 (named releases).
// Pins the surface area the rest of the codebase depends on:
//
//   - CurrentReleaseName constant exists, equals "First Manassas"
//     (the first codename, chosen by the user).
//   - ReleaseLabel() helper renders "DixieData <codename>" so
//     every chrome surface (footer, window title, CLI banner)
//     reads from one source.
//
// Before this slice the codename didn't exist in the repo at
// all; this test pins the v1 state so a future maintainer
// can't accidentally blank the constant or rename the helper.
package versioninfo

import (
	"strings"
	"testing"
)

func TestCurrentReleaseNameIsFirstManassas(t *testing.T) {
	want := "First Manassas"
	if CurrentReleaseName != want {
		t.Fatalf("CurrentReleaseName = %q; want %q", CurrentReleaseName, want)
	}
	// Single English word(s) only; the bump-version.ps1
	// -BumpCodename switch validates this but the runtime
	// constant is the last line of defence. No spaces between
	// words beyond what "First Manassas" already uses; no
	// hyphens; no special characters.
	if strings.ContainsAny(CurrentReleaseName, "-_!@#$%^&*") {
		t.Errorf("CurrentReleaseName contains characters the bump script would reject: %q", CurrentReleaseName)
	}
}

func TestReleaseLabelFormat(t *testing.T) {
	// ReleaseLabel is the chrome helper. Test only the
	// contract that matters: it includes the codename so the
	// footer, window title, and CLI banner can read from
	// one source.
	got := ReleaseLabel()
	if !strings.Contains(got, CurrentReleaseName) {
		t.Errorf("ReleaseLabel = %q; expected it to include the codename %q", got, CurrentReleaseName)
	}
}