// about_handlers_test.go -- issue #585 slice 3.
//
// Pins the buildAboutView mapper contract. The mapper is the
// pure function the GET /about handler delegates to; testing
// the mapper directly avoids the App-construction overhead
// (the App holds a db handle + a dozen service pointers and
// has no constructor suitable for unit testing).
//
// The mapper pulls from buildinfo (identity) + releasehistory
// (release entries) and produces a viewmodel.AboutView the
// templ partial renders.

package appshell

import (
	"testing"

	"github.com/valueforvalue/DixieData/internal/buildinfo"
)

// TestBuildAboutViewIdentityFields pins the identity surface:
// AppName, Version, Codename, Schema, Commit, Branch, BuiltAt
// all populated from the buildinfo package.
func TestBuildAboutViewIdentityFields(t *testing.T) {
	view := buildAboutView("abc123def", "stable", "2026-07-15T00:00:00Z")
	if view.AppName != buildinfo.AppName {
		t.Errorf("AppName = %q; want %q", view.AppName, buildinfo.AppName)
	}
	if view.Version != buildinfo.AppVersion {
		t.Errorf("Version = %q; want %q", view.Version, buildinfo.AppVersion)
	}
	if view.Codename != buildinfo.Codename() {
		t.Errorf("Codename = %q; want %q", view.Codename, buildinfo.Codename())
	}
	if view.Schema != buildinfo.SchemaVersion {
		t.Errorf("Schema = %d; want %d", view.Schema, buildinfo.SchemaVersion)
	}
	if view.Commit != "abc123def" {
		t.Errorf("Commit = %q; want abc123def", view.Commit)
	}
	if view.Branch != "stable" {
		t.Errorf("Branch = %q; want stable", view.Branch)
	}
	if view.BuiltAt != "2026-07-15T00:00:00Z" {
		t.Errorf("BuiltAt = %q; want 2026-07-15T00:00:00Z", view.BuiltAt)
	}
}

// TestBuildAboutViewLicenseURL pins the LicenseURL field.
// Dev builds point at /blob/dev/LICENSE; tagged builds point at
// /blob/{commit}/LICENSE.
func TestBuildAboutViewLicenseURL(t *testing.T) {
	// Tagged build.
	view := buildAboutView("abc123", "stable", "2026-07-15T00:00:00Z")
	want := "https://github.com/valueforvalue/DixieData/blob/abc123/LICENSE"
	if view.LicenseURL != want {
		t.Errorf("LicenseURL (tagged) = %q; want %q", view.LicenseURL, want)
	}
	// Dev build.
	view = buildAboutView("dev", "dev", "")
	want = "https://github.com/valueforvalue/DixieData/blob/dev/LICENSE"
	if view.LicenseURL != want {
		t.Errorf("LicenseURL (dev) = %q; want %q", view.LicenseURL, want)
	}
}

// (Issue #598: TestBuildAboutViewCodenameOnlyOnCurrent,
// TestBuildAboutViewReleasesFlatten, and
// TestBuildAboutViewEmptyBakedKeepsNilReleases are removed.
// The /about page no longer renders a Release history
// section; the recent-commits section is the replacement.
// The releasehistory package itself is still consumed by
// scripts/bake-activity for the per-release Repository
// activity rollup (#586).)

// TestBuildAboutViewRecentCommitsShape pins the issue #594
// projection: the new RecentCommits slice on AboutView carries
// the same field shape as activityhistory.RecentCommit. In
// dev builds (no bake) the slice is nil; the templ renders an
// empty-state notice in that case. The contract: every field
// the templ needs is on the view, in a form the templ can
// render without importing activityhistory directly.
func TestBuildAboutViewRecentCommitsShape(t *testing.T) {
	view := buildAboutView("dev", "dev", "")
	if view.RecentCommits == nil {
		// Dev build (no bake). Shape is nil-slice, not
		// zero-length slice. The templ handles nil safely.
		return
	}
	// Baked build: at least one entry (in practice the
	// recentCommitsCap is 25; the test asserts the shape
	// of the first entry).
	if len(view.RecentCommits) == 0 {
		// Edge: bake ran but produced no commits (rare,
		// but defensible on a fresh repo).
		return
	}
	first := view.RecentCommits[0]
	if first.Hash == "" {
		t.Errorf("RecentCommits[0].Hash = empty; want 40-char SHA")
	}
	if first.ShortHash == "" {
		t.Errorf("RecentCommits[0].ShortHash = empty; want 7-char prefix")
	}
	if first.Date == "" {
		t.Errorf("RecentCommits[0].Date = empty; want YYYY-MM-DD")
	}
	if first.Author == "" {
		t.Errorf("RecentCommits[0].Author = empty; want user.name")
	}
	if first.Subject == "" {
		t.Errorf("RecentCommits[0].Subject = empty; want commit subject")
	}
	// ShortHash must be a prefix of Hash. Defensive: a
	// future refactor might derive ShortHash incorrectly
	// (e.g. via a separate field rather than a slice).
	if len(first.Hash) >= 7 && first.ShortHash != first.Hash[:7] {
		t.Errorf("RecentCommits[0].ShortHash = %q; want Hash[:7] = %q",
			first.ShortHash, first.Hash[:7])
	}
}
