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

// ClosedIssue is one parsed closed GitHub issue (issue #650).
// The /issues endpoint returns both issues and pull requests;
// the bake sets IsPullRequest from the upstream payload so the
// aggregation can exclude PRs.
type ClosedIssue = parse.ClosedIssue

// IssuesSummary is the closed-issues breakdown by Type label.
type IssuesSummary = parse.IssuesSummary

// Snapshot is the baked payload the /about page reads.
type Snapshot = parse.Snapshot

// RecentCommit is one parsed line of `git log -n <cap>`,
// projected to the /about page's Recent commits section.
// Aliased from the parse sub-package so existing consumers
// (appshell handler) continue to reference
// activityhistory.RecentCommit unchanged. Issue #594.
type RecentCommit = parse.RecentCommit

// RecentCommitsFromGitLog parses the bake-script's
// `git log -n <cap> --format=%H|%aI|%an|%s` output into a
// slice of RecentCommit. Aliased from the parse sub-package.
// Issue #594.
func RecentCommitsFromGitLog(lines []string, capN int) []RecentCommit {
	return parse.RecentCommitsFromGitLog(lines, capN)
}

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

// IssuesClosedFromIssues aggregates a slice of ClosedIssue
// records into the per-type closed-issues summary. Each
// issue is counted exactly once; issues with no canonical
// Type label land in UncategorizedCount; pull requests are
// excluded. Issue #650.
func IssuesClosedFromIssues(issues []ClosedIssue) IssuesSummary {
	return parse.IssuesClosedFromIssues(issues)
}

// IssuesClosedFromLabels aggregates a slice of issue labels
// into the per-type closed-issues summary. Legacy aggregation
// (issue #650); retained for the parse_test.go fixture that
// pins its shape. New callers should use IssuesClosedFromIssues.
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