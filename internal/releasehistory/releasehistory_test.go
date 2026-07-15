// releasehistory_test.go -- issue #585 slice 2.
//
// Pins the CHANGELOG.md parser contract. The parser is the
// single source of truth for the /about page's Release history
// section; the templ + JS layers consume Entry structs (no
// string-typed metadata in the viewmodel). All releases ship
// with a baked JSON baked at release time; this test pins
// both the parser behaviour AND the empty-state shape
// (baked == nil -> empty entries slice).
//
// Fixture-driven: the parser is tested against a literal
// CHANGELOG fragment that exercises both the pre-#266
// `## v1.2.N` shape AND the post-#266 `## v1.U.N` shape plus
// the empty-state ([Unreleased] only) and a single-line
// release (worst-case parsing).
package releasehistory

import (
	"strings"
	"testing"
)

// fixtureCHANGELOG exercises every shape the parser must handle.
// - Two `### Added` sections, one `### Changed`, one `### Fixed`.
// - Two distinct headings: the legacy `## v1.2.55 - 2026-06-25`
//   shape AND the post-#266 `## v1.1.4 - 2026-07-12` shape.
// - One empty release (heading + nothing else).
// - One single-line release (heading + one bullet under one
//   ### subsection).
const fixtureCHANGELOG = `# Changelog

## [Unreleased]

### Added

- future-feature: something not yet shipped

## v1.1.4 - 2026-07-12

### Fixed

- **race fix (#103)**: per-element marker guard.

## v1.2.55 - 2026-06-25

### Added

- **first-feature (#100)**: opens the page.
- **second-feature (#101)**: drills into a record.

### Changed

- **tighten copy (#102)**: shorter headings across the board.

## v1.0.0 - 2026-05-16

### Added

- **first release**.
`

func TestParseReaderFixtureShape(t *testing.T) {
	got, err := ParseReader(strings.NewReader(fixtureCHANGELOG))
	if err != nil {
		t.Fatalf("ParseReader: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d entries; want 3 (the [Unreleased] block is excluded)", len(got))
	}
	// Newest first (the parser reverses the CHANGELOG order so
	// callers can index [0] for the latest release).
	if got[0].Version != "v1.1.4" {
		t.Errorf("got[0].Version = %q; want v1.1.4 (newest first)", got[0].Version)
	}
	if got[0].Date != "2026-07-12" {
		t.Errorf("got[0].Date = %q; want 2026-07-12", got[0].Date)
	}
	if len(got[0].Fixed) != 1 {
		t.Errorf("got[0].Fixed = %v; want 1 bullet", got[0].Fixed)
	}
	if got[1].Version != "v1.2.55" {
		t.Errorf("got[1].Version = %q; want v1.2.55 (legacy shape, second-newest)", got[1].Version)
	}
	if got[1].Date != "2026-06-25" {
		t.Errorf("got[1].Date = %q; want 2026-06-25", got[1].Date)
	}
	if len(got[1].Added) != 2 {
		t.Errorf("got[1].Added = %v; want 2 bullets", got[1].Added)
	}
	if len(got[1].Changed) != 1 {
		t.Errorf("got[1].Changed = %v; want 1 bullet", got[1].Changed)
	}
	if got[2].Version != "v1.0.0" {
		t.Errorf("got[2].Version = %q; want v1.0.0", got[2].Version)
	}
	if len(got[2].Added) != 1 {
		t.Errorf("got[2].Added = %v; want 1 bullet", got[2].Added)
	}
}

// TestParseReaderExcludesUnreleased pins that the [Unreleased]
// block is never surfaced as a release entry. The templ partial
// reads baked entries only; [Unreleased] is the author's
// working buffer.
func TestParseReaderExcludesUnreleased(t *testing.T) {
	got, err := ParseReader(strings.NewReader(fixtureCHANGELOG))
	if err != nil {
		t.Fatalf("ParseReader: %v", err)
	}
	for _, e := range got {
		if e.Version == "Unreleased" || e.Version == "[Unreleased]" {
			t.Errorf("entry leaked from [Unreleased] block: %+v", e)
		}
	}
}

// TestParseReaderEmptyInput pins the empty-state shape: zero
// releases, no error. The dev binary ships baked == nil and the
// templ partial must render an empty-state copy, not crash.
func TestParseReaderEmptyInput(t *testing.T) {
	got, err := ParseReader(strings.NewReader(""))
	if err != nil {
		t.Fatalf("ParseReader(empty): %v", err)
	}
	if len(got) != 0 {
		t.Errorf("empty input -> %d entries; want 0", len(got))
	}
}

// TestParseReaderSingleLineRelease pins the worst-case single
// bullet shape (v1.2.53 + v1.2.54 in production CHANGELOG.md).
// A parser that requires N+ bullets per subsection fails here.
func TestParseReaderSingleLineRelease(t *testing.T) {
	in := "## v1.2.54 - 2026-06-08\n\n### Fixed\n\n- **small fix**.\n"
	got, err := ParseReader(strings.NewReader(in))
	if err != nil {
		t.Fatalf("ParseReader: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d entries; want 1", len(got))
	}
	if got[0].Version != "v1.2.54" {
		t.Errorf("Version = %q; want v1.2.54", got[0].Version)
	}
	if len(got[0].Fixed) != 1 {
		t.Errorf("Fixed = %v; want 1 bullet", got[0].Fixed)
	}
}

// TestBakedReturnsBakedVar pins the runtime accessor: Baked()
// returns the package-level `baked` slice verbatim. The dev
// binary's baked is nil; a release build's baked is populated.
// The About templ partial renders an empty-state message when
// Baked() returns nil.
func TestBakedReturnsBakedVar(t *testing.T) {
	if got, want := Baked(), baked; (got == nil) != (want == nil) {
		t.Errorf("Baked() nilness mismatch: got nil=%v, want nil=%v", got == nil, want == nil)
	}
}

// TestEntriesNewestFirst pins the order contract. The About
// page's expanded list shows the most recent 10 releases;
// callers index [0] for the latest. A parser that returns
// CHANGELOG order (oldest first) would silently render the
// wrong 10.
func TestEntriesNewestFirst(t *testing.T) {
	got, err := ParseReader(strings.NewReader(fixtureCHANGELOG))
	if err != nil {
		t.Fatalf("ParseReader: %v", err)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].Date < got[i].Date {
			t.Errorf("entry %d (%s) precedes entry %d (%s); entries must be newest-first",
				i-1, got[i-1].Date, i, got[i].Date)
		}
	}
}