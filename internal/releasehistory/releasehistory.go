// Package releasehistory exposes the typed release-history
// shape the /about page renders, plus the Baked() accessor
// for the package-level data populated by
// scripts/bake-release-notes (issue #585 slice 2).
//
// The parser and Entry struct live in the sub-package
// internal/releasehistory/parse. Splitting them out lets the
// bake script import the parser without depending on this
// package, which references a package-level `baked` variable
// declared in a generated file (baked.go, gitignored). The
// split fixes the chicken-egg bug documented in issue #588
// where the bake script could not compile on a fresh checkout.
//
// Parser contract (issue #585 slice 2):
//   - Each `## v{VERSION} - YYYY-MM-DD` heading starts a new
//     release entry. Headings without a date are skipped
//     (defensive).
//   - Each `### {Subsection}` heading splits bullets into the
//     matching field: Added / Changed / Fixed / Removed /
//     Maintenance / Documentation.
//   - The [Unreleased] block is never surfaced as an entry.
//   - Entries are returned in CHANGELOG order (newest-first
//     per the project's CHANGELOG convention). Callers can
//     index [0] for the latest release.
package releasehistory

import "github.com/valueforvalue/DixieData/internal/releasehistory/parse"

// Entry is one release as parsed from CHANGELOG.md. The type
// is aliased from the parse sub-package so existing consumers
// (templates, viewmodel, appshell handlers) continue to
// reference releasehistory.Entry unchanged.
type Entry = parse.Entry

// Baked returns the release entries baked at build time. The
// dev binary ships with baked == nil; the partial renders an
// empty-state message. The bake step (Makefile target
// `release-notes-bake`) regenerates internal/releasehistory/baked.go.
func Baked() []Entry {
	return baked
}