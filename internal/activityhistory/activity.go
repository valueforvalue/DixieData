// Package activityhistory exposes the typed Repository
// activity snapshot the /about page renders, plus the Baked()
// accessor for the package-level data populated by
// scripts/bake-activity (issue #586).
//
// The parser + helpers (Snapshot, GitLogEntry, ContributorCount,
// ReleaseActivity, IssueLabel, IssuesSummary, PerDayFromGitLog,
// TopContributors, IssuesClosedFromLabels) live in the
// sub-package internal/activityhistory/parse. Splitting them
// out lets the bake script import the parser without
// depending on this package, which references a package-level
// `baked` variable declared in a generated file (baked.go,
// gitignored). The split fixes the chicken-egg bug
// documented in issue #588 where the bake script could not
// compile on a fresh checkout.
//
// Bake contract (issue #586 slice 2):
//   - FirstCommitDate / LatestCommitDate: ISO YYYY-MM-DD
//     boundaries of the rolling 52-week window.
//   - PerDay: map[YYYY-MM-DD]int, one entry per active day.
//   - TopContributors: top 10 by commit count (names only,
//     no email).
//   - PerRelease: per-release activity (commits, contributors,
//     lines added, lines removed) computed from `git log
//     <prev_tag>..<this_tag>` for each releasehistory entry.
//   - IssuesClosed: closed issues by Type label (bug /
//     enhancement / documentation / duplicate / question /
//     invalid / wontfix).
package activityhistory

import "github.com/valueforvalue/DixieData/internal/activityhistory/parse"

// GitLogEntry is one parsed line of `git log --format=%aI
// %an`. Aliased from the parse sub-package so existing
// consumers (templates, viewmodel, appshell handlers)
// continue to reference activityhistory.GitLogEntry
// unchanged.
type GitLogEntry = parse.GitLogEntry

// ContributorCount is a (name, commit-count) pair used for
// the top-contributors ranking.
type ContributorCount = parse.ContributorCount

// ReleaseActivity is one release's commit rollup.
type ReleaseActivity = parse.ReleaseActivity

// IssueLabel is one parsed label from a closed GitHub Issue.
type IssueLabel = parse.IssueLabel

// IssuesSummary is the closed-issues breakdown by Type label.
type IssuesSummary = parse.IssuesSummary

// Snapshot is the baked payload the /about page reads.
type Snapshot = parse.Snapshot

// PerDayFromGitLog rolls up a slice of git log entries into
// the per-day commit count map. Days not present in the
// input are absent from the map (not zero).
func PerDayFromGitLog(entries []GitLogEntry) map[string]int {
	return parse.PerDayFromGitLog(entries)
}

// TopContributors returns the top N contributors ranked by
// commit count descending. Ties keep insertion order.
// Contributors beyond the cap are dropped.
func TopContributors(in []ContributorCount, n int) []ContributorCount {
	return parse.TopContributors(in, n)
}

// IssuesClosedFromLabels aggregates a slice of issue labels
// into the per-type closed-issues summary. Returns an empty
// (non-nil) ByType map so the templ partial can iterate.
func IssuesClosedFromLabels(labels []IssueLabel) IssuesSummary {
	return parse.IssuesClosedFromLabels(labels)
}

// Baked returns the package-level baked snapshot. The dev
// binary ships baked == nil; the /about page renders an
// empty-state message in that case. The Makefile target
// `activity-history-bake` rewrites baked.go with the
// populated value.
func Baked() *Snapshot {
	return baked
}