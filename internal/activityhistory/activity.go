// Package activityhistory parses git log + GitHub Issues into
// a typed snapshot the /about page's Repository activity
// section renders (issue #586).
//
// The data is baked at release time via
// `go run ./scripts/bake-activity` (Makefile target
// `activity-history-bake`). The dev binary ships with baked
// == nil; the templ partial renders an empty-state message.
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

import (
	"sort"
)

// GitLogEntry is one parsed line of `git log --format=%aI %an`.
// Date is the YYYY-MM-DD slice of the ISO timestamp; Name is
// the git user.name.
type GitLogEntry struct {
	Date string
	Name string
}

// ContributorCount is a (name, commit-count) pair used for the
// top-contributors ranking.
type ContributorCount struct {
	Name  string
	Count int
}

// ReleaseActivity is one release's commit rollup.
type ReleaseActivity struct {
	Version      string
	Date         string
	CommitCount  int
	Contributors int
	LinesAdded   int
	LinesRemoved int
}

// IssueLabel is one parsed label from a closed GitHub Issue.
// The bake fetches /repos/{owner}/{repo}/issues?state=closed
// and extracts the label names.
type IssueLabel struct {
	Name string
}

// IssuesSummary is the closed-issues breakdown by Type label.
// TotalClosed counts issues with at least one canonical Type
// label. ByType is keyed by the canonical Type name; issues
// without a Type label are excluded from the per-type counts
// but still counted in TotalClosed iff they have a Status
// label (defensive; the bake source typically carries the
// Type axis).
type IssuesSummary struct {
	TotalClosed int
	ByType      map[string]int
	GeneratedAt string
}

// Snapshot is the baked payload the /about page reads.
type Snapshot struct {
	GeneratedAt       string
	FirstCommitDate   string
	LatestCommitDate  string
	TotalCommits      int
	TotalContributors int
	PerDay            map[string]int
	TopContributors   []ContributorCount
	PerRelease        []ReleaseActivity
	IssuesClosed      IssuesSummary
}

// PerDayFromGitLog rolls up a slice of git log entries into
// the per-day commit count map. Days not present in the
// input are absent from the map (not zero).
func PerDayFromGitLog(entries []GitLogEntry) map[string]int {
	out := make(map[string]int)
	for _, e := range entries {
		out[e.Date]++
	}
	return out
}

// topContributors returns the top N contributors ranked by
// commit count descending. Ties keep insertion order (Go's
// sort.SliceStable). Contributors beyond the cap are dropped.
func topContributors(in []ContributorCount, n int) []ContributorCount {
	sorted := make([]ContributorCount, len(in))
	copy(sorted, in)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Count > sorted[j].Count
	})
	if n > len(sorted) {
		n = len(sorted)
	}
	return sorted[:n]
}

// canonicalTypeLabels is the set of Type axis labels per
// docs/agents/triage-labels.md. Only labels in this set are
// counted in IssuesClosed.ByType. Other labels (status /
// area / priority / meta) are ignored for the bucket counts.
var canonicalTypeLabels = map[string]bool{
	"bug":           true,
	"enhancement":   true,
	"documentation": true,
	"duplicate":     true,
	"question":      true,
	"invalid":       true,
	"wontfix":       true,
}

// IssuesClosedFromLabels aggregates a slice of issue labels
// into the per-type closed-issues summary. Returns an empty
// (non-nil) ByType map so the templ partial can iterate.
func IssuesClosedFromLabels(labels []IssueLabel) IssuesSummary {
	byType := make(map[string]int)
	total := 0
	for _, l := range labels {
		if canonicalTypeLabels[l.Name] {
			byType[l.Name]++
			total++
		}
	}
	return IssuesSummary{
		TotalClosed: total,
		ByType:      byType,
	}
}

// Baked returns the package-level baked snapshot. The dev
// binary ships baked == nil; the /about page renders an
// empty-state message in that case. The Makefile target
// `activity-history-bake` rewrites baked.go with the
// populated value.
func Baked() *Snapshot {
	return baked
}