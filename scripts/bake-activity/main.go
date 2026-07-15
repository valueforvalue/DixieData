// bake-activity -- Issue #586 slice 2/3.
//
// Parses git log + GitHub Issues into a Snapshot the /about
// page's Repository activity section renders. Writes
// internal/activityhistory/baked.go (gitignored, regenerated
// by `make tpl`).
//
// Usage:
//   go run ./scripts/bake-activity
//
// Wired into `make tpl` alongside `release-notes-bake`.
//
// Source: `git log` for the commit heatmap + top contributors
// + per-release activity (via `git log <prev_tag>..<this_tag>`);
// `gh api /repos/.../issues?state=closed` for the issues-closed
// breakdown by Type label.

package main

import (
	"bytes"
	"fmt"
	"go/format"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/valueforvalue/DixieData/internal/activityhistory/parse"
	"github.com/valueforvalue/DixieData/internal/releasehistory"
)

// rollingWindowDays is the size of the heatmap window (52
// weeks = 364 days). Per issue #586 locked decision.
const rollingWindowDays = 364

// topContributorsCap is the maximum number of contributors
// the snapshot retains. Per issue #586 locked decision.
const topContributorsCap = 10

func main() {
	root, err := repoRoot()
	if err != nil {
		log.Fatalf("bake-activity: %v", err)
	}
	// Git log: rolling window + per-release.
	gitEntries, err := gitLogRolling(root, rollingWindowDays)
	if err != nil {
		log.Fatalf("bake-activity: git log: %v", err)
	}
	releases := releasehistory.Baked()
	perRelease, err := perReleaseActivity(root, releases)
	if err != nil {
		log.Fatalf("bake-activity: per-release: %v", err)
	}
	// GitHub Issues: closed-by-Type.
	issues, err := fetchClosedIssues()
	if err != nil {
		// Non-fatal -- the bake still works without the issues
		// section. Log and continue with an empty summary.
		log.Printf("bake-activity: gh issues fetch failed (continuing without): %v", err)
		issues = nil
	}
	snap := buildSnapshot(gitEntries, perRelease, issues)
	out := filepath.Join(root, "internal/activityhistory/baked.go")
	src, err := render(out, snap)
	if err != nil {
		log.Fatalf("bake-activity: render: %v", err)
	}
	if err := os.WriteFile(out, src, 0o644); err != nil {
		log.Fatalf("bake-activity: write %s: %v", out, err)
	}
	fmt.Printf("bake-activity: wrote snapshot (%d commits, %d releases, %d issues) to %s\n",
		snap.TotalCommits, len(snap.PerRelease), snap.IssuesClosed.TotalClosed, out)
}

func repoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, "CHANGELOG.md")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find CHANGELOG.md walking up from %s", cwd)
		}
		dir = parent
	}
}

// gitLogRolling returns the rolling N-day git log as a slice
// of GitLogEntry. Uses `git log --format=%aI %an` and slices
// the timestamp to YYYY-MM-DD.
func gitLogRolling(root string, days int) ([]parse.GitLogEntry, error) {
	cmd := exec.Command("git", "log",
		fmt.Sprintf("--since=%d days ago", days),
		"--format=%aI %an",
		"--no-merges",
	)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
	}
	var entries []parse.GitLogEntry
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		// Format: "YYYY-MM-DDTHH:MM:SS-ZZ Firstname Lastname"
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}
		date := parts[0][:10] // YYYY-MM-DD slice
		entries = append(entries, parse.GitLogEntry{Date: date, Name: parts[1]})
	}
	return entries, nil
}

// perReleaseActivity computes commit counts + contributors +
// LOC stats for each release in the given slice, using
// `git log <prev_tag>..<this_tag>` for each. The oldest release
// has no `prev_tag`; its range starts from the empty tree.
//
// Releases whose tag does not exist (e.g. CHANGELOG entries
// added in dev between release tags) are emitted with zero
// counts -- the templ partial renders "N/A" for the per-release
// row in that case (the about page surfaces "this release was
// amended in dev; activity data not yet baked").
func perReleaseActivity(root string, releases []releasehistory.Entry) ([]parse.ReleaseActivity, error) {
	if len(releases) == 0 {
		return nil, nil
	}
	tagExists := make(map[string]bool)
	for _, r := range releases {
		tagExists[r.Version] = gitTagExists(root, r.Version)
	}
	var out []parse.ReleaseActivity
	for i, rel := range releases {
		if !tagExists[rel.Version] {
			// Tag does not exist -- emit a zero row so the
			// templ partial can render an N/A marker.
			out = append(out, parse.ReleaseActivity{
				Version:      rel.Version,
				Date:         rel.Date,
				CommitCount:  0,
				Contributors: 0,
				LinesAdded:   0,
				LinesRemoved: 0,
			})
			continue
		}
		var rangeSpec string
		if i == 0 {
			rangeSpec = rel.Version
		} else {
			// Skip per-release counts when the previous tag is
			// missing -- the range spec would error.
			if !tagExists[releases[i-1].Version] {
				out = append(out, parse.ReleaseActivity{
					Version:      rel.Version,
					Date:         rel.Date,
					CommitCount:  0,
					Contributors: 0,
					LinesAdded:   0,
					LinesRemoved: 0,
				})
				continue
			}
			rangeSpec = releases[i-1].Version + ".." + rel.Version
		}
		count, added, removed, err := gitLogShortstat(root, rangeSpec)
		if err != nil {
			return nil, fmt.Errorf("per-release %s: %w", rel.Version, err)
		}
		contribs, err := gitLogContributors(root, rangeSpec)
		if err != nil {
			return nil, fmt.Errorf("per-release contribs %s: %w", rel.Version, err)
		}
		out = append(out, parse.ReleaseActivity{
			Version:      rel.Version,
			Date:         rel.Date,
			CommitCount:  count,
			Contributors: len(contribs),
			LinesAdded:   added,
			LinesRemoved: removed,
		})
	}
	return out, nil
}

// gitTagExists reports whether the given tag is present in
// the local repo. Tags not yet created (e.g. CHANGELOG entries
// added in dev) return false.
func gitTagExists(root, tag string) bool {
	cmd := exec.Command("git", "tag", "-l", tag)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == tag
}

// gitLogShortstat returns the commit count + added/removed
// lines for a range. Uses `git log --shortstat` and parses the
// "N files changed, M insertions(+), K deletions(-)" line.
func gitLogShortstat(root, rangeSpec string) (int, int, int, error) {
	cmd := exec.Command("git", "log", rangeSpec, "--shortstat", "--no-merges", "--format=")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return 0, 0, 0, err
	}
	count := 0
	added := 0
	removed := 0
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "files changed") {
			count++
			a, r := parseShortstat(line)
			added += a
			removed += r
		}
	}
	return count, added, removed, nil
}

var shortstatRe = regexp.MustCompile(`(\d+) insertion.*?(\d+) deletion`)

func parseShortstat(line string) (int, int) {
	m := shortstatRe.FindStringSubmatch(line)
	if m == nil {
		return 0, 0
	}
	var a, r int
	fmt.Sscanf(m[1], "%d", &a)
	fmt.Sscanf(m[2], "%d", &r)
	return a, r
}

// gitLogContributors returns the unique contributor names for
// a range. Uses `git shortlog -sn` (counts, then names).
func gitLogContributors(root, rangeSpec string) ([]string, error) {
	cmd := exec.Command("git", "shortlog", "-sn", "--no-merges", rangeSpec)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		// Format: "  <count>\t<name>"
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		names = append(names, strings.TrimSpace(parts[1]))
	}
	return names, nil
}

// fetchClosedIssues fetches all closed GitHub Issues via the
// gh CLI, paginating until exhausted. The bake accepts that
// this requires `gh auth login` -- a non-authenticated dev
// environment bakes with an empty issues summary.
func fetchClosedIssues() ([]parse.IssueLabel, error) {
	all := []parse.IssueLabel{}
	page := 1
	for {
		// gh api paginates automatically with --paginate; we use
		// explicit page+per_page for the per-call JSON parsing.
		cmd := exec.Command("gh", "api",
			fmt.Sprintf("repos/valueforvalue/DixieData/issues?state=closed&per_page=100&page=%d", page),
			"--jq", ".[].labels[].name",
		)
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("gh api page %d: %w", page, err)
		}
		text := strings.TrimSpace(string(out))
		if text == "" || text == "null" {
			break
		}
		for _, line := range strings.Split(text, "\n") {
			if line == "" {
				continue
			}
			all = append(all, parse.IssueLabel{Name: line})
		}
		page++
		if page > 50 {
			// Safety: 50 pages * 100 = 5000 issues, way more than
			// the repo has today (500). Bail if we somehow hit it.
			break
		}
	}
	return all, nil
}

// buildSnapshot assembles the typed Snapshot from the inputs.
func buildSnapshot(entries []parse.GitLogEntry, perRelease []parse.ReleaseActivity, issueLabels []parse.IssueLabel) *parse.Snapshot {
	perDay := parse.PerDayFromGitLog(entries)
	// Compute top contributors from the rolling entries.
	contribCounts := make(map[string]int)
	for _, e := range entries {
		contribCounts[e.Name]++
	}
	var counts []parse.ContributorCount
	for name, c := range contribCounts {
		counts = append(counts, parse.ContributorCount{Name: name, Count: c})
	}
	counts = parse.TopContributors(counts, topContributorsCap)
	// Find first / latest commit dates.
	first := ""
	latest := ""
	for _, e := range entries {
		if first == "" || e.Date < first {
			first = e.Date
		}
		if latest == "" || e.Date > latest {
			latest = e.Date
		}
	}
	// Issues-closed.
	issues := parse.IssuesClosedFromLabels(issueLabels)
	issues.GeneratedAt = time.Now().UTC().Format("2006-01-02T15:04:05Z")
	return &parse.Snapshot{
		GeneratedAt:        time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		FirstCommitDate:    first,
		LatestCommitDate:   latest,
		TotalCommits:       len(entries),
		TotalContributors:  len(contribCounts),
		PerDay:             perDay,
		TopContributors:    counts,
		PerRelease:         perRelease,
		IssuesClosed:       issues,
	}
}

// render emits the Go source for baked.go. gofmt-formatted for
// stable diffs.
func render(path string, snap *parse.Snapshot) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(`// Code generated by scripts/bake-activity. DO NOT EDIT.
//
// Regenerated by ` + "`make tpl`" + ` before every release. The
// dev binary (no bake) ships baked == nil; the /about page
// renders an empty-state message in that case. See
// docs/agents/notes/about-slice-1-research.md for the bake
// contract.

package activityhistory

var baked = &Snapshot{
`)
		writeField(&buf, "GeneratedAt", snap.GeneratedAt)
	writeField(&buf, "FirstCommitDate", snap.FirstCommitDate)
	writeField(&buf, "LatestCommitDate", snap.LatestCommitDate)
	buf.WriteString(fmt.Sprintf("\tTotalCommits: %d,\n", snap.TotalCommits))
	buf.WriteString(fmt.Sprintf("\tTotalContributors: %d,\n", snap.TotalContributors))
	// PerDay map.
	buf.WriteString("\tPerDay: map[string]int{\n")
	keys := make([]string, 0, len(snap.PerDay))
	for k := range snap.PerDay {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		buf.WriteString(fmt.Sprintf("\t\t%q: %d,\n", k, snap.PerDay[k]))
	}
	buf.WriteString("\t},\n")
	// TopContributors slice.
	buf.WriteString("\tTopContributors: []ContributorCount{\n")
	for _, c := range snap.TopContributors {
		buf.WriteString(fmt.Sprintf("\t\t{Name: %q, Count: %d},\n", c.Name, c.Count))
	}
	buf.WriteString("\t},\n")
	// PerRelease slice.
	buf.WriteString("\tPerRelease: []ReleaseActivity{\n")
	for _, r := range snap.PerRelease {
		buf.WriteString(fmt.Sprintf("\t\t{Version: %q, Date: %q, CommitCount: %d, Contributors: %d, LinesAdded: %d, LinesRemoved: %d},\n",
			r.Version, r.Date, r.CommitCount, r.Contributors, r.LinesAdded, r.LinesRemoved))
	}
	buf.WriteString("\t},\n")
	// IssuesClosed.
	buf.WriteString("\tIssuesClosed: IssuesSummary{\n")
	buf.WriteString(fmt.Sprintf("\t\tTotalClosed: %d,\n", snap.IssuesClosed.TotalClosed))
	buf.WriteString(fmt.Sprintf("\t\tGeneratedAt: %q,\n", snap.IssuesClosed.GeneratedAt))
	buf.WriteString("\t\tByType: map[string]int{\n")
	keys = keys[:0]
	for k := range snap.IssuesClosed.ByType {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		buf.WriteString(fmt.Sprintf("\t\t\t%q: %d,\n", k, snap.IssuesClosed.ByType[k]))
	}
	buf.WriteString("\t\t},\n")
	buf.WriteString("\t},\n")
	buf.WriteString("}\n")
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		os.Stderr.WriteString("--- unformatted source ---\n")
		os.Stderr.Write(buf.Bytes())
		os.Stderr.WriteString("--- end unformatted source ---\n")
		return nil, fmt.Errorf("gofmt: %w", err)
	}
	return formatted, nil
}

func writeField(buf *bytes.Buffer, name, value string) {
	buf.WriteString(fmt.Sprintf("\t%s: %q,\n", name, value))
}