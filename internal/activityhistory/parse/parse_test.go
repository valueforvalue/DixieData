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
	"time"
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

// TestParseRecentCommitsFromGitLog pins the Recent commits
// section's data shape. The bake-script captures
// `git log -n <cap> --format=%H|%aI|%an|%s dev` and parses
// each line into a RecentCommit. The parser splits on the
// first 3 pipes (defensive against commit subjects that
// contain `|`); anything after the third pipe is the subject.
//
// RED (before Slice 2 lands): RecentCommit does not exist
// and the bake-parser function does not exist — build fails.
// GREEN (after Slice 2 lands): the fixture parses to the
// expected slice.
func TestParseRecentCommitsFromGitLog(t *testing.T) {
	in := []string{
		"abcdef1234567890abcdef1234567890abcdef12|2026-07-15T10:00:00-05:00|Jeremy Morris|fix: foo",
		"1234567890abcdef1234567890abcdef12345678|2026-07-14T18:30:00-05:00|Jane Doe|feat: bar",
		"fedcba0987654321fedcba0987654321fedcba09|2026-07-13T09:15:00-05:00|Bob|chore: baz",
		// Subject contains a pipe (rare but defensible) — must
		// split on the first 3 pipes and take the rest as
		// the subject.
		"deadbeefdeadbeefdeadbeefdeadbeefdeadbeef|2026-07-12T08:00:00-05:00|Carol|merge: branch | conflict",
	}
	got := RecentCommitsFromGitLog(in, 25)
	if len(got) != 4 {
		t.Fatalf("got %d recent commits; want 4", len(got))
	}
	if got[0].Hash != "abcdef1234567890abcdef1234567890abcdef12" {
		t.Errorf("got[0].Hash = %q; want full 40-char SHA", got[0].Hash)
	}
	if got[0].ShortHash != "abcdef1" {
		t.Errorf("got[0].ShortHash = %q; want 7-char prefix", got[0].ShortHash)
	}
	if got[0].Date != "2026-07-15" {
		t.Errorf("got[0].Date = %q; want YYYY-MM-DD slice", got[0].Date)
	}
	if got[0].Author != "Jeremy Morris" {
		t.Errorf("got[0].Author = %q; want full user.name", got[0].Author)
	}
	if got[0].Subject != "fix: foo" {
		t.Errorf("got[0].Subject = %q; want commit subject", got[0].Subject)
	}
	// Pipe-in-subject edge case.
	if got[3].Subject != "merge: branch | conflict" {
		t.Errorf("got[3].Subject = %q; want %q", got[3].Subject, "merge: branch | conflict")
	}
	// Cap: requesting 2 returns the first 2.
	got2 := RecentCommitsFromGitLog(in, 2)
	if len(got2) != 2 {
		t.Errorf("cap=2 returned %d; want 2", len(got2))
	}
	// Empty input → empty slice (not nil) so the templ can
	// range over it without nil-checks.
	got3 := RecentCommitsFromGitLog(nil, 25)
	if got3 == nil {
		t.Errorf("RecentCommitsFromGitLog(nil) = nil; want empty slice")
	}
	if len(got3) != 0 {
		t.Errorf("RecentCommitsFromGitLog(nil) length = %d; want 0", len(got3))
	}
	// Malformed line (no pipes) is dropped silently.
	got4 := RecentCommitsFromGitLog([]string{"not a real line"}, 25)
	if len(got4) != 0 {
		t.Errorf("malformed line should be dropped; got %d entries", len(got4))
	}
}

// TestParseRecentCommitsSnapshotField pins the Snapshot
// integration: Snapshot.RecentCommits carries the parsed
// slice; the templ reads it directly. Mirrors the existing
// snapshot-shape tests.
func TestParseRecentCommitsSnapshotField(t *testing.T) {
	snap := Snapshot{
		GeneratedAt:      "2026-07-15T10:00:00Z",
		FirstCommitDate:  "2024-01-01",
		LatestCommitDate: "2026-07-15",
		TotalCommits:     3,
		RecentCommits: []RecentCommit{
			{Hash: "abcdef1234567890abcdef1234567890abcdef12", ShortHash: "abcdef1", Date: "2026-07-15", Author: "Jeremy Morris", Subject: "fix: foo"},
		},
	}
	if len(snap.RecentCommits) != 1 {
		t.Fatalf("snap.RecentCommits length = %d; want 1", len(snap.RecentCommits))
	}
	if snap.RecentCommits[0].ShortHash != "abcdef1" {
		t.Errorf("ShortHash = %q; want abcdef1", snap.RecentCommits[0].ShortHash)
	}
}

// TestParseRecentCommitsSubjectPinnedAtRedLight pins the
// defensiveness of RecentCommitsFromGitLog: the function
// is not date-relative, so pinning a date here is a
// regression tripwire. If a future refactor adds a
// time-based filter, this test fails. (Per docs/agents/tdd.md:
// "Test assumptions as well as code.")
func TestParseRecentCommitsNoTimeFilter(t *testing.T) {
	in := []string{
		"abcdef1234567890abcdef1234567890abcdef12|2024-01-01T00:00:00Z|Old Author|very old",
		"1234567890abcdef1234567890abcdef12345678|2099-12-31T23:59:59Z|Future Author|very new",
	}
	got := RecentCommitsFromGitLog(in, 25)
	if len(got) != 2 {
		t.Errorf("RecentCommitsFromGitLog should NOT filter by date; got %d entries", len(got))
	}
	// Sanity: ensure time package usage stays trivial so
	// the test compiles (time is reserved for future
	// date-relative filtering).
	_ = time.Time{}
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