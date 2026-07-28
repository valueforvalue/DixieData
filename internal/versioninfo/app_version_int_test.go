// app_version_int_test.go — issue #578 slice 1.
// Pins the post-#266 release counter (N) at the value the
// slice-1 catch-up bump sets it to. After the catch-up, N=4
// reflects the body of post-cutover release-aimed work on
// dev (the #544 + #566 feedback chain, the #561 microcopy
// sweep, and the #570 buildinfo consolidation). RC1 cohort
// bump (issue #658, 2026-07-26): N=5 so the RC manifest can
// advertise 1.1.5-rc1, making compareVersions see Newer=true
// against the installed 1.1.4 binary.
//
// RC cohort N progression on the rc/v1.1 branch:
//
//	N=5  (2026-07-26, #658)     1.1.5-rc1
//	N=25 (2026-07-27, #674)     1.1.25-rc1
//	N=26 (2026-07-27, #674)     1.1.26-rc1
//	N=28 (2026-07-27, #674)     1.1.28-rc2
//	N=29 (2026-07-28, rc/v1.1)  1.1.29-rc1 — v1.1.29-rc1.zip rebuild
//	N=30 (2026-07-28, rc/v1.1)  1.1.30-rc2 — bug-fix RC2 (Wails cache reuse shipped stale binary)
//	N=31 (2026-07-28, rc/v1.1)  1.1.31-rc3 — clean rebuild, re-bump so updater offers it
//
// Going forward, the slice-2 CI gate in
// .github/workflows/test.yml enforces a +1 bump on every
// release-time PR (issue #578, ADR 0009 stable-promotion
// flow). The +1 lockstep guard at scripts/bump-version.ps1
// (line ~365) is the human side; this test pin is the
// machine side that catches reset regressions.
//
// When the catch-up value changes (e.g. another historical
// catch-up is needed) update this constant and the slice-1
// PR's commit message must explain why.
package versioninfo

import "testing"

func TestCurrentAppVersionIntReflectsPostCutoverWork(t *testing.T) {
	// rc/v1.1 RC1: N=32 bumps after method-override elimination
	// across all form/button delete and update paths. The previous
	// RC3 (N=31) shipped with silently-broken save + delete buttons
	// in the Wails desktop build due to data-method + Clone issues.
	if CurrentAppVersionInt != 32 {
		t.Fatalf("CurrentAppVersionInt = %d; want 32 (v1.1.32-rc1)", CurrentAppVersionInt)
	}
}

// TestAppVersionUsesReleaseCounterNotSchema guards against
// a regression where AppVersion() recomputes from schema (the
// pre-#266 shape, v1.2.{schema}). After #266 the two
// counters are decoupled and AppVersion must use N directly.
func TestAppVersionUsesReleaseCounterNotSchema(t *testing.T) {
	if AppVersion() == AppVersionForSchema(CurrentSchemaVersion) {
		t.Errorf("AppVersion() = %q; must NOT match the pre-#266 shape %q (decoupled per CONTEXT.md \"Release counter N != schema version\" law)",
			AppVersion(), AppVersionForSchema(CurrentSchemaVersion))
	}
}
