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
	"strings"

	"github.com/valueforvalue/DixieData/internal/activityhistory"
	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/glossary"
	"github.com/valueforvalue/DixieData/internal/presentation"
	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

var _ viewmodel.AboutView = viewmodel.AboutView{}

// handleAbout serves GET /about. Renders the templ AboutView
// inside the standard Layout. The release history is read from
// the package-level `baked` slice (nil in dev builds; the
// templ partial renders an empty-state message).
func (a *App) handleAbout(w http.ResponseWriter, r *http.Request) {
	view := buildAboutView(buildinfo.GitCommit, buildinfo.GitBranch, buildinfo.BuildTimestamp, a.cfg.Services.RepositoryURL)
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
func buildAboutView(commit, branch, builtAt, repoURL string) viewmodel.AboutView {
	view := viewmodel.AboutView{
		AppName:    buildinfo.AppName,
		Version:    buildinfo.AppVersion,
		Codename:   buildinfo.Codename(),
		Schema:     buildinfo.SchemaVersion,
		Commit:     commit,
		Branch:     branch,
		BuiltAt:    builtAt,
		LicenseURL:    licenseURL(commit, repoURL),
		RepositoryURL: repoURL,
		Activity:   buildActivityView(activityhistory.Baked()),
		Glossary:   buildGlossaryView(),
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
	v.IssuesClosed.UncategorizedCount = snap.IssuesClosed.UncategorizedCount
	v.IssuesClosed.GeneratedAt = snap.IssuesClosed.GeneratedAt
	return v
}

// licenseURL returns the GitHub LICENSE blob URL for the
// running commit (issue #660: base URL from
// cfg.Services.RepositoryURL). For dev builds (commit == "dev")
// it points at the dev branch; for tagged builds it points at
// the tag. repoURL is the configured repository base URL
// (default https://github.com/valueforvalue/DixieData); an
// empty repoURL falls back to the built-in default so callers
// without config wiring still work.
func licenseURL(commit, repoURL string) string {
	if strings.TrimSpace(repoURL) == "" {
		repoURL = "https://github.com/valueforvalue/DixieData"
	}
	if commit == "" || commit == "dev" {
		return repoURL + "/blob/dev/LICENSE"
	}
	return repoURL + "/blob/" + commit + "/LICENSE"
}

// commitURL builds the GitHub commit URL for a single commit
// hash (issue #660). The viewmodel package's CommitURL helper
// is the canonical implementation used by the templ; this
// thin alias exists so older callers that imported commitURL
// from appshell keep working.
func commitURL(repoURL, hash string) string {
	return viewmodel.CommitURL(repoURL, hash)
}

// buildGlossaryView projects the canonical term registry
// into the viewmodel slice the templ partial consumes
// (issue #564 slice 1). The glossary package is the
// single source of truth; the viewmodel is a typed seam
// because the templates package cannot import internal/
// glossary without a cycle.
//
// The slice copies Term-for-Term (no transformation) so
// the registry's contract (unique slugs, kebab anchors,
// non-empty fields, related slugs all registered) is
// preserved on the wire. /about renders every entry the
// registry returns; the (future) disclosure popover on
// a verified apply site reads the entry's Short via
// glossary.LookupBySlug(slug).
func buildGlossaryView() []viewmodel.GlossaryEntry {
	reg := glossary.Registry()
	out := make([]viewmodel.GlossaryEntry, 0, len(reg))
	for _, t := range reg {
		out = append(out, viewmodel.GlossaryEntry{
			Slug:    t.Slug,
			Term:    t.Term,
			Short:   t.Short,
			Full:    t.Full,
			Related: t.Related,
		})
	}
	return out
}
