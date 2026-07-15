// Package parse implements the git-log + GitHub-Issues parser
// for the activityhistory package. It is a separate sub-package
// so that bake scripts (scripts/bake-activity) can import the
// parser + helpers without depending on the parent
// activityhistory package, which references a package-level
// `baked` variable declared in a generated file
// (internal/activityhistory/baked.go, gitignored). Without
// this split, the bake script's compile fails with `undefined:
// baked` on a fresh checkout — the chicken-egg bug documented
// in issue #588.
//
// The parent activityhistory package re-exports the types via
// aliases so existing consumers (templates, viewmodel,
// appshell handlers) continue to reference activityhistory.X
// unchanged.
package parse

import (
	"sort"
)

// GitLogEntry is one parsed line of `git log --format=%aI
// %an`. Date is the YYYY-MM-DD slice of the ISO timestamp;
// Name is the git user.name.
type GitLogEntry struct {
	Date string
	Name string
}

// ContributorCount is a (name, commit-count) pair used for
// the top-contributors ranking.
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
// label. ByType is keyed by the canonical Type name;
// issues without a Type label are excluded from the per-type
// counts but still counted in TotalClosed iff they have a
// Status label (defensive; the bake source typically carries
// the Type axis).
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
// the per-day commit count map. Days not present in the input
// are absent from the map (not zero).
func PerDayFromGitLog(entries []GitLogEntry) map[string]int {
	out := make(map[string]int)
	for _, e := range entries {
		out[e.Date]++
	}
	return out
}

// TopContributors returns the top N contributors ranked by
// commit count descending. Ties keep insertion order (Go's
// sort.SliceStable). Contributors beyond the cap are
// dropped.
//
// Public so the bake script's inline top-N slicing can
// delegate to the canonical implementation; tests pin the
// cap + ordering invariants on this function.
func TopContributors(in []ContributorCount, n int) []ContributorCount {
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
// area / priority / meta) are ignored for the bucket
// counts.
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