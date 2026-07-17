// adr_author_test.go — issue #617 regression net.
//
// Pins the convention that every ADR in `docs/adr/*.md` (i.e.
// the top-level ADR docs, not the companion material under
// `docs/adr/references/`) carries a `## Author` section with
// a non-empty author line. Backfill happened on 2026-07-17;
// the regression net catches a future ADR shipped without the
// field.
//
// We use a static source check (matching the precedent set by
// undo_fallback_test.go for the slice-1 undo fix): walk the
// directory, read each ADR, assert the section is present.
// Files in `docs/adr/references/` are deliberately excluded —
// those are companion documents (catalogs, reference tables)
// not decisions; they live under the parent ADR's number
// without inheriting the Author requirement.
//
// The check is intentionally narrow: it asserts the
// `^## Author` heading and a non-empty body line. It does NOT
// validate the format of the author line beyond non-empty
// (e.g. it does not enforce a `@handle` token). Stricter
// checks belong in a follow-up; right now the goal is "no
// ADR ships without an Author section."
package appshell

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// adrAuthorHeadingRE matches a top-level `## Author` heading
// at the start of a line. Anchored (^##\s+Author\s*$) so a
// `## Author bio` or `### Author` does not satisfy the
// check.
var adrAuthorHeadingRE = regexp.MustCompile(`(?m)^##\s+Author\s*$`)

// TestADRsHaveAuthorSection asserts every ADR under
// `docs/adr/*.md` carries a `## Author` section with a
// non-empty body line. Companion docs under
// `docs/adr/references/` are excluded.
//
// Failure message names the file + line number so the
// contributor can fix the ADR with one jump.
func TestADRsHaveAuthorSection(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// Walk up from `internal/appshell/` to the repo root
	// (the test runs from the package directory, not the
	// repo root). The test framework also runs from the
	// package directory so two levels up is always correct.
	root := filepath.Join(wd, "..", "..")
	adrDir := filepath.Join(root, "docs", "adr")

	entries, err := os.ReadDir(adrDir)
	if err != nil {
		t.Fatalf("read %s: %v — the ADR regression net cannot run without the docs tree", adrDir, err)
	}

	var missing []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		path := filepath.Join(adrDir, name)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read %s: %v", path, err)
			continue
		}
		if !adrAuthorHeadingRE.Match(raw) {
			missing = append(missing, path)
			continue
		}
		// Verify the section has at least one non-blank body
		// line below the heading. Catches a future drift
		// where someone adds `## Author` but leaves it empty.
		if !sectionHasBody(raw, "## Author") {
			missing = append(missing, path+" (heading present but body is empty)")
		}
	}

	if len(missing) > 0 {
		t.Fatalf("found %d ADR(s) missing a non-empty `## Author` section: %v — every ADR must carry the author per docs/adr/TEMPLATE.md. New ADRs use the template; older ADRs backfill with the form `Jeremy Morris (@jeremymorris) — backfilled YYYY-MM-DD`.",
			len(missing), missing)
	}
}

// sectionHasBody returns true when the markdown source has at
// least one non-blank line after the named `## Heading`
// section, before the next `## ` heading (or end of file).
func sectionHasBody(raw []byte, heading string) bool {
	lines := strings.Split(string(raw), "\n")
	inSection := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") && strings.TrimSpace(strings.TrimPrefix(trimmed, "## ")) == strings.TrimPrefix(heading, "## ") {
			inSection = true
			continue
		}
		if !inSection {
			continue
		}
		// Next `##` heading ends the section.
		if strings.HasPrefix(trimmed, "## ") {
			return false
		}
		// Non-blank body line found.
		if trimmed != "" {
			return true
		}
	}
	return false
}