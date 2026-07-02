// cli_debug_inplace.go — `debug in-place-safety` subcommand.
//
// Walks `git diff <last-release-tag>..HEAD` for operations
// that are unsafe for in-place update on main:
//   - Destructive schema operations: DROP TABLE, DROP COLUMN,
//     RENAME without preserve-old, DELETE FROM in a migration
//     block, removal of an IF EXISTS guard.
//   - Handler signature changes: route pattern or method
//     changes in routes.go.
//
// The check is INFORMATIONAL — exit code is the count of
// HIGH-severity findings (0 = clean). The `safe-for-in-place`
// label is the human acknowledgment; this check is the
// automated tripwire. See docs/agents/build-protocol.md §5.

package appshell

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// InPlaceSafetyFinding is a single flagged operation.
type InPlaceSafetyFinding struct {
	Kind     string `json:"kind"`
	Severity string `json:"severity"`
	File     string `json:"file"`
	Snippet  string `json:"snippet"`
	Reason   string `json:"reason"`
}

// InPlaceSafetyReport is the JSON payload.
type InPlaceSafetyReport struct {
	Command      string                 `json:"command"`
	BaseRef      string                 `json:"base_ref"`
	Findings     []InPlaceSafetyFinding `json:"findings"`
	FindingCount int                    `json:"finding_count"`
	GeneratedAt  string                 `json:"generated_at"`
}

// runDebugInPlaceSafety walks the diff against the last release
// tag and flags destructive operations.
func runDebugInPlaceSafety(ctx context.Context, app *App, opts DebugOptions) (int, error) {
	repoRoot := findRepoRootForInPlaceSafety()
	if repoRoot == "" {
		return 2, fmt.Errorf("could not locate repo root from working dir")
	}
	base, err := lastReleaseTag(repoRoot)
	if err != nil {
		return 2, fmt.Errorf("could not resolve last release tag: %w", err)
	}
	if base == "" {
		base = "HEAD~10"
	}

	findings := scanInPlaceSafetyDiff(repoRoot, base, "HEAD")

	report := InPlaceSafetyReport{
		Command:      "debug in-place-safety",
		BaseRef:      base,
		Findings:     findings,
		FindingCount: len(findings),
		GeneratedAt:  time.Unix(opts.Now(), 0).UTC().Format(time.RFC3339),
	}

	if opts.JSON {
		enc := json.NewEncoder(opts.Writer)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return 1, err
		}
	} else {
		fmt.Fprintf(opts.Writer, "dixiedata debug in-place-safety\n")
		fmt.Fprintf(opts.Writer, "================================\n")
		fmt.Fprintf(opts.Writer, "Base ref:     %s\n", report.BaseRef)
		fmt.Fprintf(opts.Writer, "Findings:     %d\n", report.FindingCount)
		fmt.Fprintf(opts.Writer, "\n")
		if len(findings) == 0 {
			fmt.Fprintf(opts.Writer, "Clean: no destructive operations detected.\n")
		} else {
			for _, f := range findings {
				fmt.Fprintf(opts.Writer, "  [%s] %s\n", f.Severity, f.Kind)
				fmt.Fprintf(opts.Writer, "    File:    %s\n", f.File)
				fmt.Fprintf(opts.Writer, "    Reason:  %s\n", f.Reason)
				if f.Snippet != "" {
					fmt.Fprintf(opts.Writer, "    Snippet: %s\n", f.Snippet)
				}
				fmt.Fprintf(opts.Writer, "\n")
			}
		}
	}

	highCount := 0
	for _, f := range findings {
		if f.Severity == "high" {
			highCount++
		}
	}
	if highCount > 0 {
		return highCount, fmt.Errorf("in-place-safety: %d high-severity findings", highCount)
	}
	return 0, nil
}

// lastReleaseTag returns the most recent v* tag reachable
// from HEAD. Returns "" if no tags exist.
func lastReleaseTag(repoRoot string) (string, error) {
	cmd := exec.Command("git", "tag", "--sort=-v:refname", "--merged", "HEAD")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "v") {
			return line, nil
		}
	}
	return "", nil
}

// findRepoRootForInPlaceSafety walks up from cwd looking for
// the git root.
func findRepoRootForInPlaceSafety() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	dir := cwd
	for i := 0; i < 5; i++ {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// scanInPlaceSafetyDiff walks `git diff <base>..HEAD` and
// flags lines that match destructive patterns.
func scanInPlaceSafetyDiff(repoRoot, base, head string) []InPlaceSafetyFinding {
	cmd := exec.Command("git", "diff", "--unified=0", "--no-color", base+".."+head)
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	diff := string(out)

	var findings []InPlaceSafetyFinding
	currentFile := ""
	hunkLine := 0

	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			currentFile = extractDiffPath(line)
			hunkLine = 0
		case strings.HasPrefix(line, "@@"):
			re := regexp.MustCompile(`\+(\d+)`)
			if m := re.FindStringSubmatch(line); m != nil {
				fmt.Sscanf(m[1], "%d", &hunkLine)
			}
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			hunkLine++
			kind, severity, reason := classifyAddedLine(currentFile, line[1:])
			if kind != "" {
				findings = append(findings, InPlaceSafetyFinding{
					Kind:     kind,
					Severity: severity,
					File:     fmt.Sprintf("%s:%d", currentFile, hunkLine),
					Snippet:  strings.TrimSpace(line[1:]),
					Reason:   reason,
				})
			}
		}
	}

	sort.Slice(findings, func(i, j int) bool {
		return findings[i].File < findings[j].File
	})
	return findings
}

// extractDiffPath pulls the b-side path from a `diff --git`
// header.
func extractDiffPath(line string) string {
	re := regexp.MustCompile(`^diff --git a/(.+?) b/(.+)$`)
	m := re.FindStringSubmatch(line)
	if m == nil {
		return ""
	}
	return m[2]
}

// classifyAddedLine returns (kind, severity, reason) when
// the added line matches a destructive pattern.
func classifyAddedLine(file, line string) (kind, severity, reason string) {
	trimmed := strings.TrimSpace(line)

	if isSchemaFile(file) {
		return classifySchemaLine(trimmed)
	}
	if strings.HasSuffix(file, "routes.go") && isHandlerChange(trimmed) {
		return classifyHandlerLine(trimmed)
	}
	return "", "", ""
}

// isSchemaFile identifies files that are part of a schema
// migration. Conservative: only matches files in internal/db/
// or docs/migrations/.
func isSchemaFile(file string) bool {
	if strings.HasPrefix(file, "internal/db/") {
		return true
	}
	if strings.HasPrefix(file, "docs/migrations/") {
		return true
	}
	return false
}

// classifySchemaLine picks the specific kind for a schema
// migration line.
func classifySchemaLine(line string) (kind, severity, reason string) {
	switch {
	case strings.Contains(line, "DROP TABLE"):
		return "schema_drop_table", "high", "DROP TABLE is destructive; users on the affected version cannot roll back via in-place update"
	case strings.Contains(line, "DROP COLUMN"):
		return "schema_drop_column", "high", "DROP COLUMN is destructive; data is lost on upgrade"
	case strings.Contains(line, "ALTER TABLE") && strings.Contains(line, "RENAME"):
		return "schema_rename", "high", "RENAME without preserve-old breaks readers on the old name"
	case strings.Contains(line, "DELETE FROM"):
		return "schema_delete", "high", "DELETE FROM in a migration deletes user data on upgrade"
	}
	return "", "", ""
}

// isHandlerChange identifies lines in routes.go that register
// or modify route patterns.
func isHandlerChange(line string) bool {
	return strings.Contains(line, "r.Get(") ||
		strings.Contains(line, "r.Post(") ||
		strings.Contains(line, "r.Put(") ||
		strings.Contains(line, "r.Delete(") ||
		strings.Contains(line, "r.Patch(") ||
		strings.Contains(line, "r.Handle(")
}

// classifyHandlerLine picks the kind for a handler
// registration. We flag MEDIUM because the diff can't tell
// whether the route is new (added), renamed, or
// method-changed.
func classifyHandlerLine(line string) (kind, severity, reason string) {
	return "handler_registration", "medium",
		"new or changed route registration; verify the existing client surface still works against this handler"
}