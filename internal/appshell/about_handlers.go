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
	"net/http"

	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/presentation"
	"github.com/valueforvalue/DixieData/internal/releasehistory"
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
// buildinfo + releasehistory packages. Pure function -- testable
// without an App instance. The current release's Codename is
// filled from buildinfo.Codename() (the baked entries do not
// carry codenames).
func buildAboutView(commit, branch, builtAt string) viewmodel.AboutView {
	baked := releasehistory.Baked()
	view := viewmodel.AboutView{
		AppName:    buildinfo.AppName,
		Version:    buildinfo.AppVersion,
		Codename:   buildinfo.Codename(),
		Schema:     buildinfo.SchemaVersion,
		Commit:     commit,
		Branch:     branch,
		BuiltAt:    builtAt,
		LicenseURL: licenseURL(commit),
	}
	for i, e := range baked {
		re := viewmodel.ReleaseEntry{
			Version:     e.Version,
			Date:        e.Date,
			Added:       e.Added,
			Changed:     e.Changed,
			Fixed:       e.Fixed,
			Removed:     e.Removed,
			Maintenance: e.Maintenance,
			Docs:        e.Docs,
		}
		// Fill the codename for the current release only -- the
		// CHANGELOG does not record per-release codenames (the
		// codename is a property of the current version, not
		// the historical series). See docs/RELEASING.md §"codename
		// rules".
		if i == 0 {
			re.Codename = buildinfo.Codename()
		}
		view.Releases = append(view.Releases, re)
	}
	return view
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