// Package parse implements the CHANGELOG.md parser for the
// releasehistory package. It is a separate sub-package so
// that bake scripts (scripts/bake-release-notes) can import
// the parser without depending on the parent releasehistory
// package, which references a package-level `baked` variable
// declared in a generated file (internal/releasehistory/baked.go,
// gitignored). Without this split, the bake script's compile
// fails with `undefined: baked` on a fresh checkout — the
// chicken-egg bug documented in issue #588.
//
// The parent releasehistory package re-exports the Entry type
// via `type Entry = parse.Entry` so existing consumers do not
// change.
package parse

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/valueforvalue/DixieData/internal/debug"
)

// Entry is one release as parsed from CHANGELOG.md. Date is
// the ISO `YYYY-MM-DD` value from the heading. Codename is
// not in the CHANGELOG (it lives in
// internal/versioninfo/versioninfo.go's CurrentReleaseName),
// so it stays empty here and the viewmodel layer fills it
// from buildinfo for the current release only.
type Entry struct {
	Version     string
	Codename    string
	Date        string
	Added       []string
	Changed     []string
	Fixed       []string
	Removed     []string
	Maintenance []string
	Docs        []string
}

// headingRe matches `## v{VERSION} - YYYY-MM-DD`. VERSION
// accepts digits + dots (the post-#266 shape has the letter
// `v` then `MAJOR.UPDATEFLOW.RELEASE`).
var headingRe = regexp.MustCompile(`^## v([0-9]+(?:\.[0-9]+){1,2}) - (\d{4}-\d{2}-\d{2})`)

// subsectionRe matches `### Subsection Name` and captures
// the name. The trailing text is mapped to the canonical
// field (Added / Changed / Fixed / Removed / Maintenance /
// Documentation).
var subsectionRe = regexp.MustCompile(`^### (.+?)\s*$`)

// ParseFile reads path and returns the parsed entries
// newest-first. The [Unreleased] block is excluded.
func ParseFile(path string) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("releasehistory/parse: open %s: %w", path, err)
	}
	defer debug.DeferCloseLog(f, "releasehistory-changelog")
	return ParseReader(f)
}

// ParseReader reads r and returns the parsed entries in
// CHANGELOG order (newest-first per the project's CHANGELOG
// convention). The [Unreleased] block is excluded.
func ParseReader(r io.Reader) ([]Entry, error) {
	scanner := bufio.NewScanner(r)
	// CHANGELOG.md bullets can be very long (the prose wraps
	// to multiple lines via line-continuation, though we
	// don't currently use that). 1 MiB per line is plenty.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var out []Entry
	var cur *Entry
	var curSubsection *[]string
	flush := func() {
		if cur != nil {
			out = append(out, *cur)
			cur = nil
			curSubsection = nil
		}
	}
	for scanner.Scan() {
		line := scanner.Text()
		if m := headingRe.FindStringSubmatch(line); m != nil {
			flush()
			cur = &Entry{
				Version: "v" + m[1],
				Date:    m[2],
			}
			curSubsection = nil
			continue
		}
		if cur == nil {
			// Outside any release section. Lines before the
			// first `## v...` heading (preamble, [Unreleased])
			// are intentionally ignored.
			continue
		}
		if m := subsectionRe.FindStringSubmatch(line); m != nil {
			curSubsection = subsectionField(cur, m[1])
			if curSubsection == nil {
				// Unknown subsection -- treat as no-op so a
				// future CHANGELOG convention doesn't break
				// the parser. Bullets under the unknown
				// heading are dropped.
				continue
			}
			continue
		}
		if curSubsection == nil {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "- ") {
			continue
		}
		bullet := strings.TrimPrefix(trimmed, "- ")
		bullet = strings.TrimSpace(bullet)
		if bullet == "" {
			continue
		}
		*curSubsection = append(*curSubsection, bullet)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("releasehistory/parse: scan: %w", err)
	}
	flush()
	// CHANGELOG.md is authored newest-first by project
	// convention; the parser preserves that order so callers
	// can index [0] for the latest release. Tests pin this
	// invariant (see TestEntriesNewestFirst).
	return out, nil
}

// subsectionField returns a pointer to the matching bullet
// slice for the given subsection name, or nil if the name is
// not one of the canonical CHANGELOG subsections.
func subsectionField(e *Entry, name string) *[]string {
	switch name {
	case "Added":
		return &e.Added
	case "Changed":
		return &e.Changed
	case "Fixed":
		return &e.Fixed
	case "Removed":
		return &e.Removed
	case "Maintenance":
		return &e.Maintenance
	case "Documentation":
		return &e.Docs
	}
	return nil
}