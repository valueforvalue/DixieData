// Package buildinfo regression net for issue #462 (chrome polish)
// + issue #570 (consolidation).
//
// Codename() is the codename-only chrome string re-exported from
// versioninfo.CurrentReleaseName so the top-shell brand pill can
// pair "DixieData — First Manassas" without rendering "DixieData
// DixieData First Manassas" (which is what happens if callers
// pair the literal "DixieData" with ReleaseLabel's "DixieData First
// Manassas" form). The helper mirrors ReleaseLabel's re-export
// pattern, so this test pins the same v1 contract.
//
// FeedbackIdentity() + CombinedVersionString() (issue #570)
// ship the consolidated version values the feedback modal
// disclosure and the window title both read from. Pinning the
// exact string shape here keeps a future change to the
// disclosure copy from silently drifting one surface but not
// the other.
package buildinfo

import (
	"fmt"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/versioninfo"
)

func TestCodenameReturnsCurrentReleaseName(t *testing.T) {
	if got, want := Codename(), versioninfo.CurrentReleaseName; got != want {
		t.Errorf("Codename() = %q; want %q (mirrors versioninfo.CurrentReleaseName)", got, want)
	}
	if got := Codename(); got == "" {
		t.Errorf("Codename() returned empty string; chrome sites would render \"DixieData \u2014 \" with no codename")
	}
}

// TestFeedbackIdentityNamesAllThreeFields pins the contract
// for the slice 1 helper (issue #570). The feedback modal's
// Send-to-support disclosure uses these three strings
// (app_version + build_identity + schema_version) and the
// feedback JSON payload serialises them \u2014 both surfaces
// must see the same values. The helper is the single source
// of truth for the three values.
func TestFeedbackIdentityNamesAllThreeFields(t *testing.T) {
	app, build, schema := FeedbackIdentity()
	if app != AppVersion {
		t.Errorf("FeedbackIdentity app = %q; want %q (must equal AppVersion)", app, AppVersion)
	}
	if app != versioninfo.AppVersion() {
		t.Errorf("FeedbackIdentity app = %q; want %q (must equal versioninfo.AppVersion())", app, versioninfo.AppVersion())
	}
	if build != BuildIdentity() {
		t.Errorf("FeedbackIdentity build = %q; want %q (must equal BuildIdentity())", build, BuildIdentity())
	}
	if schema != "v"+fmt.Sprint(SchemaVersion) {
		t.Errorf("FeedbackIdentity schema = %q; want %q (must render SchemaVersion as v<N>)", schema, "v"+fmt.Sprint(SchemaVersion))
	}
}

// TestCombinedVersionStringMentionsAppSchemaBuild pins the
// sentence shape the layout footer renders. Future disclosure
// changes should keep all three fields named; the test guards
// against "dropped the schema" or "dropped the build" drift.
func TestCombinedVersionStringMentionsAppSchemaBuild(t *testing.T) {
	got := CombinedVersionString()
	for _, want := range []string{AppVersion, "v" + fmt.Sprint(SchemaVersion), BuildIdentity()} {
		if !strings.Contains(got, want) {
			t.Errorf("CombinedVersionString() = %q; missing %q", got, want)
		}
	}
}

// TestVersionStructAlignsWithDirectAccessors pins the
// one-liner struct surface: callers that want a single
// return value (rather than three function calls) read from
// here. The struct must stay in sync with the direct
// accessor outputs so a future change to AppVersion() or
// SchemaVersion can't leave one surface stale.
func TestVersionStructAlignsWithDirectAccessors(t *testing.T) {
	if Version.App != AppVersion {
		t.Errorf("Version.App = %q; want %q", Version.App, AppVersion)
	}
	if Version.Schema != SchemaVersion {
		t.Errorf("Version.Schema = %d; want %d", Version.Schema, SchemaVersion)
	}
}

// TestDisclosureSentenceWrapsCombined pins the slice-2
// helper (issue #570) that the layout disclosure paragraph
// renders: the version values must be wrapped in parens so
// the existing layout disclosure copy (which puts the values
// inline between "the app version" and "to a third-party
// support service") reads naturally. Future disclosure copy
// revisions that drop the parens are a deliberate decision
// rather than an accidental drift.
func TestDisclosureSentenceWrapsCombined(t *testing.T) {
	got := DisclosureSentence()
	if !strings.HasPrefix(got, "(") || !strings.HasSuffix(got, ")") {
		t.Errorf("DisclosureSentence() = %q; must be wrapped in parens", got)
	}
	if got != "("+CombinedVersionString()+")" {
		t.Errorf("DisclosureSentence() = %q; want %q", got, "("+CombinedVersionString()+")")
	}
}
