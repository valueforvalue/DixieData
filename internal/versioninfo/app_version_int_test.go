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
	// Catch-up value (issue #578 slice 1, 2026-07-14):
	// accounts for the documented post-2026-07-03 release
	// work — the #544 + #566 feedback chain, the #561
	// microcopy sweep, and the #570 buildinfo consolidation.
	if CurrentAppVersionInt != 7 {
		t.Fatalf("CurrentAppVersionInt = %d; want 7 (RC1 check-results fix, issue #658)", CurrentAppVersionInt)
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
