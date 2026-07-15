// parse_test.go -- issue #586 slice 2.
//
// Pins the rolling 52-week commit activity snapshot contract.
// The /about page's Repository activity section (issue #586)
// reads the baked `Snapshot` from this package.
//
// Contract:
//   - FirstCommitDate / LatestCommitDate: ISO YYYY-MM-DD
//     boundaries of the rolling window.
//   - PerDay: map[YYYY-MM-DD]int, rolling 52 weeks back from
//     today (or the latest commit date for static builds).
//   - TopContributors: top 10 by commit count across the
//     window. Names are git user.name strings only; no
//     email, no avatar URL.
//   - PerRelease: one entry per release in releasehistory.
//     Each carries commit count + contributor count + lines
//     added/removed for that release's tag range.
//   - IssuesClosed: closed issues broken down by triage Type
//     (bug, enhancement, documentation, duplicate, question,
//     invalid, wontfix per docs/agents/triage-labels.md).
//
// All counts are pure function outputs of the parser; tests
// pin shape + invariants, not exact counts (those depend on
// the repo state at bake time).
package parse

import (
	"testing"
)

// TestParseGitLogPinpointDates pins the per-day map shape.
// 52 weeks = 364 days. The fixture below exercises a tight
// commit cluster; the map should have one key per commit
// date.
func TestParseGitLogPinpointDates(t *testing.T) {
	out := PerDayFromGitLog([]GitLogEntry{
		{Date: "2026-07-15"},
		{Date: "2026-07-15"},
		{Date: "2026-07-14"},
	})
	if out["2026-07-15"] != 2 {
		t.Errorf("2026-07-15 = %d; want 2", out["2026-07-15"])
	}
	if out["2026-07-14"] != 1 {
		t.Errorf("2026-07-14 = %d; want 1", out["2026-07-14"])
	}
}

// TestTopContributorsRanksByCount pins the ranking contract.
// 10 contributors max; tied commits keep insertion order.
func TestTopContributorsRanksByCount(t *testing.T) {
	in := []ContributorCount{
		{Name: "Alice", Count: 5},
		{Name: "Bob", Count: 10},
		{Name: "Carol", Count: 3},
	}
	got := TopContributors(in, 10)
	if len(got) != 3 {
		t.Fatalf("got %d contributors; want 3", len(got))
	}
	if got[0].Name != "Bob" {
		t.Errorf("got[0] = %q; want Bob (highest count)", got[0].Name)
	}
	if got[2].Name != "Carol" {
		t.Errorf("got[2] = %q; want Carol (lowest count)", got[2].Name)
	}
}

// TestTopContributorsCapsAt10 pins the slice cap.
// Contributors beyond the 10th are dropped.
func TestTopContributorsCapsAt10(t *testing.T) {
	in := make([]ContributorCount, 0, 20)
	for i := 0; i < 20; i++ {
		in = append(in, ContributorCount{Name: "c" + string(rune('a'+i)), Count: i})
	}
	got := TopContributors(in, 10)
	if len(got) != 10 {
		t.Errorf("got %d contributors; want 10 (cap)", len(got))
	}
	// Sorted descending; the highest commit count is 19.
	if got[0].Count != 19 {
		t.Errorf("got[0].Count = %d; want 19", got[0].Count)
	}
}

// TestIssuesClosedBucketShape pins the closed-issues shape:
// one bucket per canonical Type label. Empty archive ->
// empty map (not nil) so the templ partial can iterate.
func TestIssuesClosedBucketShape(t *testing.T) {
	got := IssuesClosedFromLabels([]IssueLabel{
		{Name: "bug"},
		{Name: "bug"},
		{Name: "enhancement"},
		{Name: "documentation"},
		{Name: "wontfix"},
		{Name: "area:frontend"},
		{Name: "needs-triage"},
	})
	if got.TotalClosed != 5 {
		t.Errorf("TotalClosed = %d; want 5 (non-Type labels excluded)", got.TotalClosed)
	}
	if got.ByType["bug"] != 2 {
		t.Errorf("ByType[bug] = %d; want 2", got.ByType["bug"])
	}
	if got.ByType["enhancement"] != 1 {
		t.Errorf("ByType[enhancement] = %d; want 1", got.ByType["enhancement"])
	}
	if got.ByType["documentation"] != 1 {
		t.Errorf("ByType[documentation] = %d; want 1", got.ByType["documentation"])
	}
	if got.ByType["wontfix"] != 1 {
		t.Errorf("ByType[wontfix] = %d; want 1", got.ByType["wontfix"])
	}
	if _, ok := got.ByType["area:frontend"]; ok {
		t.Errorf("ByType[area:frontend] present; should have been excluded (not a Type label)")
	}
}