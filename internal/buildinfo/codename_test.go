// Package buildinfo regression net for issue #462 (chrome polish).
//
// Codename() is the codename-only chrome string re-exported from
// versioninfo.CurrentReleaseName so the top-shell brand pill can
// pair "DixieData — First Manassas" without rendering "DixieData
// DixieData First Manassas" (which is what happens if callers
// pair the literal "DixieData" with ReleaseLabel's "DixieData First
// Manassas" form). The helper mirrors ReleaseLabel's re-export
// pattern, so this test pins the same v1 contract.
package buildinfo

import (
	"testing"

	"github.com/valueforvalue/DixieData/internal/versioninfo"
)

func TestCodenameReturnsCurrentReleaseName(t *testing.T) {
	if got, want := Codename(), versioninfo.CurrentReleaseName; got != want {
		t.Errorf("Codename() = %q; want %q (mirrors versioninfo.CurrentReleaseName)", got, want)
	}
	if got := Codename(); got == "" {
		t.Errorf("Codename() returned empty string; chrome sites would render \"DixieData — \" with no codename")
	}
}
