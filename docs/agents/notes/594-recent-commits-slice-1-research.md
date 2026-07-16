# Issue #594 Slice 1 — Research + data shape decision

Records the data shape decision for the `/about` "Recent commits"
section. No code in this slice; pure research + paper design, like
the #585 + #586 slice-1 research notes that preceded them.

## Goal (recap)
Add a "Recent commits" section to `/about` showing the most recent
25 commits to `dev`, NEWER than the current release. Each row:
short hash + relative date + author + subject, all linked to the
GitHub commit permalink. Section sits between Release history and
Repository activity (per Open Question 1 default).

## Data shape decision

**Add a new `parse.RecentCommit` struct + `Snapshot.RecentCommits []RecentCommit` field, do NOT extend the existing `parse.GitLogEntry`.**

Rationale:

- `parse.GitLogEntry` is a per-line-of-`git-log` projection used by
  the heatmap aggregation (`PerDayFromGitLog`). It carries
  `Date` + `Name` only — the minimum needed to bucket commits by
  day for the heatmap. Adding `Subject` + `Hash` to that struct
  would inflate the per-line memory footprint for every commit in
  the rolling 52-week window (≈ hundreds of lines) when the new
  use case only needs the last 25.
- A new `parse.RecentCommit` struct carries exactly the fields the
  templ needs: `Hash, ShortHash, Date, Author, Subject`. The
  bake-script's git-log call only happens once per bake (not on
  every request), so two separate calls — one for the heatmap,
  one for recent commits — is fine.
- This matches the existing pattern in `parse.go`: `ReleaseActivity`,
  `ContributorCount`, `IssueLabel`, `IssuesSummary` are all
  specialized structs with their own `parse` definitions. Adding
  one more is consistent.

## Type sketch (Go, to land in Slice 2)

```go
// RecentCommit is one parsed line of `git log --format=%H|%aI|%an|%s`,
// projected to the /about page's Recent commits section. The bake
// takes the last 25 (by commit date) and stores them in
// Snapshot.RecentCommits; the templ reads the slice as-is. Hash
// is the full 40-char SHA1; ShortHash is the 7-char prefix the
// UI renders as the visible hash text. Date is the YYYY-MM-DD
// slice of the ISO timestamp; Author is the git user.name; Subject
// is the first line of the commit message.
type RecentCommit struct {
    Hash      string
    ShortHash string
    Date      string
    Author    string
    Subject   string
}
```

`Snapshot.RecentCommits []RecentCommit` is added to the `Snapshot`
struct, alongside `PerDay`, `TopContributors`, `PerRelease`, and
`IssuesClosed`.

The `internal/activityhistory` parent package re-exports the type
via `type RecentCommit = parse.RecentCommit` (same pattern as
`IssueLabel` and `IssuesSummary` per the #588 slice).

## Git command sketch (bake-script, to land in Slice 2)

`scripts/bake-activity/main.go` gains a new helper `gitLogRecentCommits(root, cap)`:

```bash
git log -n 25 --format=%H|%aI|%an|%s dev
```

The `dev` branch is the source-of-truth (matches the existing
`git log` usage in the bake script). `cap=25` matches
`recentCommitsCap` const. The output is line-oriented, one
commit per line, fields pipe-separated; the parser splits on
`|` (subject cannot contain `|` in a clean git log, but if a
subject ever did, splitting on the first 3 pipes + taking the
rest as the subject is the safe form).

## Frontend relative-date helper

The probe `audit/smoke_about.mjs` does not currently pin the
relative-date rendering. The plan defers that to the templ +
JS rendering (Slice 4). The decision recorded here:

- Commits ≤ 30 days old → relative ("2 days ago", "yesterday",
  "5 hours ago").
- Commits > 30 days old → ISO date (`2026-07-12`).
- Implementation: `Intl.RelativeTimeFormat('en', { numeric: 'auto' })`
  in a small `formatRelativeDate(isoDate, now)` helper in
  `frontend/app.js` (added in Slice 4).

## Locked decisions (from the plan, confirmed)
1. **Source**: `internal/activityhistory` snapshot, NOT a new bake
   package. (No new `make` target, no new script in `scripts/bake-*`.)
2. **Cap**: 25 rows. Tweak via `recentCommitsCap` const.
3. **Hash format**: full 40-char hash in the data, short 7-char
   prefix in the rendered link text.
4. **Permalink**: `https://github.com/valueforvalue/DixieData/commit/<hash>`.
5. **Section order**: between Release history and Repository activity.
6. **Author display**: full `user.name` (not the `Top contributors`
   projection).

## Open questions resolved
- ~~Add `Subject` + `Hash` to `GitLogEntry` vs new `RecentCommit` struct~~
  → **New struct** (rationale above).
- ~~One bake call vs two~~ → **Two calls** (heatmap uses the existing
  `gitLogRolling`; recent commits use the new `gitLogRecentCommits`).
  Two calls are cheaper than one call + per-line filtering because
  the format strings are different.
- ~~Subject parsing on `|`~~ → **Split on the first 3 pipes**;
  anything after the third `|` is the subject. Defensive against
  the rare commit message that contains `|`.

## Files this slice touches
- `docs/agents/notes/594-recent-commits-slice-1-research.md` (this file).

## Files Slice 2 will touch
- `internal/activityhistory/parse/parse.go` — new `RecentCommit`
  struct + `Snapshot.RecentCommits` field.
- `internal/activityhistory/activity.go` — re-export alias.
- `scripts/bake-activity/main.go` — `gitLogRecentCommits` helper.
- `internal/activityhistory/parse/parse_test.go` — RED test.