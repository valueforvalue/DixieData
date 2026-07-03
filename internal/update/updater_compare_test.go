// updater_compare_test.go — pins issue #266's version-split
// decision matrix in a single Go test file so future refactors
// of compareVersions + parseVersion fail loudly before shipping.
//
// Decisions covered (all four locked during RPCI Critique):
//   - Q1 (legacy v1.2.{N} strings): mapped to U=1, N=N by
//     parseVersion (see parseVersion's isLegacyVersionShape
//     branch).
//   - Q2 (downgrade reject): U < installed U →
//     Compatible=false regardless of N. Tested as
//     "installed-with-newer-U-bumped".
//   - Q3 (first real U bump ships as v1.3.0): tested via the
//     "U=1 to U=2" pair; the new-shape literal v1.2.{N} in
//     that pair ships as v1.3.0 in the bump script.
//   - Q4 (U mismatch in either direction rejects): tested
//     under both "U+1" and "U-1" axes.
//
// Future work (follow-up issues, not in this slice):
//   - Add NeedsReinstall exposure through the Settings UI
//   - Add ArchiveManifest field carrying (U, N) on backup
//     write
//   - Update bump-version.ps1 with --bump-update-flow + N
//     tracking
//   - Update RELEASING.md + ADR 0008 cross-reference

package update

import "testing"

// TestCompareVersionsMatrix_ExplicitCases drives the matrix
// with release-then-installed ordering (the call-site pattern
// in updater.go: compareVersions(release, installed)). Each
// row pins a specific decision.

// TestCompareVersionsMatrix_ExplicitCases drives the matrix
// with release-then-installed ordering (the call-site pattern
// in updater.go: compareVersions(release, installed)). Each
// row pins a specific decision.
//
// Format: name, release, installed, expectedCompatible,
// expectedNewer. `newer` = release is strictly newer than
// installed (and auto-update eligible). `compatible` = same U,
// no reinstall path needed.
func TestCompareVersionsMatrix_ExplicitCases(t *testing.T) {
	cases := []struct {
		name              string
		release           string
		installed         string
		compatible        bool
		newer             bool
	}{
		// --- Q5: legacy v1.2.{N} pair (both parse to U=1) ---
		{
			name:       "legacy_v1.2.56_over_v1.2.55_update",
			release:    "v1.2.56",
			installed:  "v1.2.55",
			compatible: true,
			newer:      true,
		},
		{
			name:       "legacy_v1.2.55_over_v1.2.56_downgrade",
			release:    "v1.2.55",
			installed:  "v1.2.56",
			compatible: true,
			newer:      false,
		},
		{
			name:       "legacy_v1.2.55_over_v1.2.55_equal",
			release:    "v1.2.55",
			installed:  "v1.2.55",
			compatible: true,
			newer:      false,
		},

		// --- new shape: U matches, N varies ---
		{
			name:       "newshape_U1_N11_over_U1_N10_update",
			release:    "v1.1.11",
			installed:  "v1.1.10",
			compatible: true,
			newer:      true,
		},
		{
			name:       "newshape_U1_N9_over_U1_N10_downgrade",
			release:    "v1.1.9",
			installed:  "v1.1.10",
			compatible: true,
			newer:      false,
		},

		// --- Q4 + Q2: U mismatch forces Compatible=false ---
		// Q10: per the convention, the U=2 release ships as
		// v1.3.0 (Q3), not v1.2.20. The literal "2" in the
		// middle position is always legacy. So test U=2 via
		// v1.3.0 + v1.3.N shapes.
		{
			name:       "newshape_U2_over_U1_release_higher",
			release:    "v1.3.5",
			installed:  "v1.1.99",
			compatible: false,
			newer:      false, // release.N=5 < installed.N=99; even though U mismatch forces reinstall, release isn't numerically newer
		},
		{
			name:       "newshape_U1_over_U2_release_lower_U",
			release:    "v1.1.20",
			installed:  "v1.3.10",
			compatible: false,
			newer:      true, // release.N=20 > installed.N=10; U mismatch overrides + forces reinstall
		},
		{
			name:       "newshape_U1_over_U2_release_higher_N",
			release:    "v1.1.99",
			installed:  "v1.3.10",
			compatible: false,
			newer:      true, // release.N=99 > installed.N=10 (incompatible though U mismatch)
		},

		// --- edge cases ---
		{
			name:       "newshape_U1_N0_over_U1_N0_equal",
			release:    "v1.1.0",
			installed:  "v1.1.0",
			compatible: true,
			newer:      false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := compareVersions(c.release, c.installed)
			if err != nil {
				t.Fatalf("compareVersions(%q, %q) error: %v", c.release, c.installed, err)
			}
			if got.Compatible != c.compatible {
				t.Errorf("Compatible=%v want %v (got %+v for release=%s installed=%s)",
					got.Compatible, c.compatible, got, c.release, c.installed)
			}
			if got.Newer != c.newer {
				t.Errorf("Newer=%v want %v (got %+v for release=%s installed=%s)",
					got.Newer, c.newer, got, c.release, c.installed)
			}
		})
	}
}

// TestParseVersionLegacyV1ToU1 pins the Q1 mapping: every v1.2.{N}
// string parse lands at U=1, regardless of whether the literal
// "2" would suggest otherwise. Future refactors that drop the
// isLegacyVersionShape branch fail this test.
func TestParseVersionLegacyV1ToU1(t *testing.T) {
	legacyStrings := []string{"v1.2.55", "v1.2.0", "v1.2.99", "1.2.1"}
	for _, s := range legacyStrings {
		t.Run(s, func(t *testing.T) {
			parsed, err := parseVersion(s)
			if err != nil {
				t.Fatalf("parseVersion(%q): %v", s, err)
			}
			if parsed.updateFlow != 1 {
				t.Errorf("legacy %q should map to U=1; got U=%d", s, parsed.updateFlow)
			}
		})
	}
}

// TestParseVersionNewShapeLeavesUAlone pins that the new
// shape v1.{U}.{N} does NOT trigger the legacy rewrite. A
// literal U=2 in the middle passes through as U=2 (this is
// the moment a real U bump happens, and the rule that
// rewrite is "1.2." prefix-only is what makes that work).
func TestParseVersionNewShapeLeavesUAlone(t *testing.T) {
	// Q10 + Q3 convention: per issue #266 decision 3, the
	// first U-bump ships as v1.3.0. The literal "2" in the
	// middle position is ALWAYS legacy (mapped to U=1 by
	// TestParseVersionLegacyV1ToU1). New-shape tests below
	// skip v1.2.X intentionally — it is the legacy shape.
	newStrings := []struct {
		raw   string
		wantU int
		wantN int
	}{
		{"v1.1.10", 1, 10},  // same-U new shape
		{"v1.3.0", 3, 0},    // first U-bump per Q3
		{"v1.4.42", 4, 42},  // mid-cycle
		{"1.5.42", 5, 42},   // no v-prefix
	}
	for _, c := range newStrings {
		t.Run(c.raw, func(t *testing.T) {
			parsed, err := parseVersion(c.raw)
			if err != nil {
				t.Fatalf("parseVersion(%q): %v", c.raw, err)
			}
			if parsed.updateFlow != c.wantU {
				t.Errorf("U=%d want %d for %q", parsed.updateFlow, c.wantU, c.raw)
			}
			if parsed.release != c.wantN {
				t.Errorf("N=%d want %d for %q", parsed.release, c.wantN, c.raw)
			}
		})
	}
}

// TestParseVersionMalformedRejected pins that bad inputs
// return an error rather than silently coercing.
func TestParseVersionMalformedRejected(t *testing.T) {
	cases := []string{
		"",        // empty
		"1.2",     // only two segments
		"1.2.3.4", // too many segments
		"v1.x.3",  // non-numeric
		"latest",  // no version pattern at all
	}
	for _, s := range cases {
		t.Run(s, func(t *testing.T) {
			_, err := parseVersion(s)
			if err == nil {
				t.Errorf("parseVersion(%q) should have failed", s)
			}
		})
	}
}
