// about_handlers.go -- issue #585 slice 3.
//
// Wires the GET /about route to a templ AboutView. The handler
// reads buildinfo (for the current identity metadata) +
// releasehistory (for the baked release entries) and maps
// both into a viewmodel.AboutView the templ partial renders.
//
// Mapper lives at the appshell seam (per the inventory pattern)
// because viewmodel cannot import buildinfo or releasehistory
// without a cycle -- buildinfo depends on versioninfo, but
// viewmodel must remain independent of the build-time layer.

package appshell

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/valueforvalue/DixieData/internal/activityhistory"
	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/presentation"
	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

// handleAbout serves GET /about. Renders the templ AboutView
// inside the standard Layout. The release history is read from
// the package-level `baked` slice (nil in dev builds; the
// templ partial renders an empty-state message).
func (a *App) handleAbout(w http.ResponseWriter, r *http.Request) {
	view := buildAboutView(buildinfo.GitCommit, buildinfo.GitBranch, buildinfo.BuildTimestamp)
	if err := presentation.AboutView(view).Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// buildAboutView assembles the viewmodel.AboutView from the
// buildinfo + releasehistory + activityhistory packages. Pure
// function -- testable without an App instance.
//
// Issue #598: the releasehistory.Baked() loop is gone —
// the Release history section was removed from /about
// (the recent-commits section, #594, is the replacement).
// The releasehistory package itself is still used by
// scripts/bake-activity/main.go for the per-release
// Repository activity rollup (#586), so the import
// stays.
func buildAboutView(commit, branch, builtAt string) viewmodel.AboutView {
	view := viewmodel.AboutView{
		AppName:    buildinfo.AppName,
		Version:    buildinfo.AppVersion,
		Codename:   buildinfo.Codename(),
		Schema:     buildinfo.SchemaVersion,
		Commit:     commit,
		Branch:     branch,
		BuiltAt:    builtAt,
		LicenseURL: licenseURL(commit),
		Activity:   buildActivityView(activityhistory.Baked()),
	}
	// Issue #594: project the baked RecentCommits slice into
	// the viewmodel. nil-safe: in dev builds (no bake) the
	// slice is nil and the templ renders an empty-state
	// notice ("Recent commits will appear after the next
	// build."). The buildAboutView signature stays a pure
	// function so the test file can call it with a fixture
	// snapshot.
	snap := activityhistory.Baked()
	if snap != nil {
		for _, c := range snap.RecentCommits {
			view.RecentCommits = append(view.RecentCommits, viewmodel.RecentCommitView{
				Hash:      c.Hash,
				ShortHash: c.ShortHash,
				Date:      c.Date,
				Author:    c.Author,
				Subject:   c.Subject,
			})
		}
	}
	return view
}

// buildActivityView projects the activityhistory.Snapshot into
// the viewmodel's ActivitySnapshotView. Returns nil when no
// bake has run (dev binary).
func buildActivityView(snap *activityhistory.Snapshot) *viewmodel.ActivitySnapshotView {
	if snap == nil {
		return nil
	}
	v := &viewmodel.ActivitySnapshotView{
		GeneratedAt:       snap.GeneratedAt,
		FirstCommitDate:   snap.FirstCommitDate,
		LatestCommitDate:  snap.LatestCommitDate,
		TotalCommits:      snap.TotalCommits,
		TotalContributors: snap.TotalContributors,
	}
	// Heatmap: JSON-encode the per-day map for the JS renderer.
	b, err := json.Marshal(snap.PerDay)
	if err != nil {
		b = []byte("{}")
	}
	v.HeatmapData = string(b)
	// Top contributors.
	for _, c := range snap.TopContributors {
		v.TopContributors = append(v.TopContributors, viewmodel.Contributor{Name: c.Name, Count: c.Count})
	}
	// Per-release.
	for _, r := range snap.PerRelease {
		v.PerRelease = append(v.PerRelease, viewmodel.ActivityView{
			Version: r.Version, Date: r.Date, CommitCount: r.CommitCount,
			Contributors: r.Contributors, LinesAdded: r.LinesAdded, LinesRemoved: r.LinesRemoved,
		})
	}
	// Issues-closed: sort buckets by count desc for the stacked bar.
	for k, c := range snap.IssuesClosed.ByType {
		v.IssuesClosed.ByType = append(v.IssuesClosed.ByType, viewmodel.TypeBucket{Type: k, Count: c})
	}
	sort.Slice(v.IssuesClosed.ByType, func(i, j int) bool {
		if v.IssuesClosed.ByType[i].Count != v.IssuesClosed.ByType[j].Count {
			return v.IssuesClosed.ByType[i].Count > v.IssuesClosed.ByType[j].Count
		}
		return v.IssuesClosed.ByType[i].Type < v.IssuesClosed.ByType[j].Type
	})
	v.IssuesClosed.TotalClosed = snap.IssuesClosed.TotalClosed
	v.IssuesClosed.GeneratedAt = snap.IssuesClosed.GeneratedAt
	return v
}

// licenseURL returns the GitHub LICENSE blob URL for the
// running commit. For dev builds (commit == "dev") it points at
// the dev branch; for tagged builds it points at the tag.
func licenseURL(commit string) string {
	if commit == "" || commit == "dev" {
		return "https://github.com/valueforvalue/DixieData/blob/dev/LICENSE"
	}
	return "https://github.com/valueforvalue/DixieData/blob/" + commit + "/LICENSE"
}