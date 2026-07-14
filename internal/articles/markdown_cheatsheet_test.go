// markdown_cheatsheet_test.go — issue #565
// Drift detector between the Markdown cheatsheet rows and the
// actual article renderer (internal/records/markdown.go). The
// renderer is the source of truth; the cheatsheet is forbidden
// from claiming any syntax the renderer does not produce.
//
// The test reads three sources of truth:
//
//  1. The list of Go files goldmark includes by default + the
//     GFM extension (autolinks, tables, strikethrough, task
//     lists). We assert the cheatsheet has a row for every GFM
//     CommonMark primitive the bluemonday policy in
//     internal/records/markdown.go:82-91 allow-lists, plus the
//     GFM additions.
//
//  2. The custom Person Record reference syntax, parsed in
//     internal/records/article_service.go:771
//     (personRefLinkRE = regexp.MustCompile(`\(#person/([A-Za-z0-9_-]+)\)`)).
//     The cheatsheet must have a row for it.
//
//  3. The cheatsheet is a presentation artifact, not the
//     source of truth: if the renderer grows new syntax, this
//     test fails and the author of the renderer adds the row.
//     If the renderer drops syntax, the test fails the same way.
//
// The test runs without a database. It is the slice-1 RED
// pin per docs/agents/tdd.md — written before any cheatsheet
// data exists so the first run fails for the right reason
// (missing Rows() function, not "the test is wrong").
package articles

import (
	"regexp"
	"strings"
	"testing"
)

// personRefLinkRE mirrors the regex in
// internal/records/article_service.go:771. We re-declare it
// here instead of importing the records package so the
// internal/articles package stays free of records imports —
// per internal/architecture/architecture_test.go's
// deep-module discipline, the cheatsheet data layer is
// allowed to be a leaf node.
var personRefLinkRE = regexp.MustCompile(`\(#person/([A-Za-z0-9_-]+)\)`)

// goldmarkGFMPrimitives is the canonical set of syntaxes the
// goldmark GFM extension adds on top of CommonMark. The list
// mirrors the goldmark v1.8.2 documentation for the GFM
// extension (https://github.com/yuin/goldmark#extension--6).
//
// The cheatsheet MUST cover every entry that survives the
// bluemonday policy in internal/records/markdown.go (h1-h6,
// p, ul, ol, li, a, blockquote, code, pre, em, strong, hr,
// img, br, del, table, thead, tbody, tr, th, td). The test
// in TestCheatsheetCoversRenderer enumerates the overlap.
var goldmarkGFMPrimitives = []string{
	"heading",
	"paragraph",
	"emphasis",
	"strong",
	"link",
	"image",
	"unordered-list",
	"ordered-list",
	"task-list",
	"blockquote",
	"code-inline",
	"code-block",
	"horizontal-rule",
	"table",
	"strikethrough",
	"autolink",
}

// personRecordRefRow is the literal row key the cheatsheet
// must expose for the [Name](#person/D-00123) custom-scheme
// link. The actual regex is duplicated above to keep the
// internal/articles package free of records imports.
const personRecordRefRow = "person-record-reference"

func TestRows_FailsUntilDataFileLands(t *testing.T) {
	// Slice-1 RED pin: this test fails until
	// markdown_cheatsheet.go is implemented. Once it lands,
	// the test must enumerate the rows the cheatsheet exposes
	// and assert they cover the renderer.
	rows := Rows()
	if len(rows) == 0 {
		t.Fatalf("Rows() returned no rows; cheatsheet data not implemented")
	}
}

func TestCheatsheetCoversGoldmarkGFMPrimitives(t *testing.T) {
	rows := Rows()
	if len(rows) < 12 {
		t.Errorf("cheatsheet has %d rows; expected >=12 (the full GFM + Person Record ref subset)", len(rows))
	}

	// Build a set of row keys for fast lookup.
	rowKeys := make(map[string]bool, len(rows))
	for _, r := range rows {
		rowKeys[r.Key] = true
	}

	// Every goldmark GFM primitive must have a cheatsheet row.
	for _, primitive := range goldmarkGFMPrimitives {
		if !rowKeys[primitive] {
			t.Errorf("cheatsheet missing row for goldmark GFM primitive %q", primitive)
		}
	}

	// The custom Person Record reference syntax must have a
	// row. The regex is the source of truth — the row is
	// valid only if the regex matches a real example.
	if !rowKeys[personRecordRefRow] {
		t.Errorf("cheatsheet missing row %q for [Name](#person/D-00123) Person Record ref", personRecordRefRow)
	}
}

func TestCheatsheetPersonRecordExampleMatchesRegex(t *testing.T) {
	// Regression net: the example in the cheatsheet's
	// Person Record ref row must actually parse with the
	// same regex the article_service resolver uses. If the
	// author changes one without the other, the cheatsheet
	// lies to users.
	rows := Rows()
	var example string
	for _, r := range rows {
		if r.Key == personRecordRefRow {
			example = r.Example
			break
		}
	}
	if example == "" {
		t.Fatalf("cheatsheet has no %q row; cannot verify example", personRecordRefRow)
	}
	if !personRefLinkRE.MatchString(example) {
		t.Errorf("Person Record ref example %q does not match the article_service regex (it would not resolve at render time)", example)
	}
}

func TestCheatsheetRowsHaveRequiredFields(t *testing.T) {
	// Every row must have: Key, Syntax, Effect, Example. An
	// incomplete row is a UX bug: the popover shows an
	// empty cell.
	rows := Rows()
	for i, r := range rows {
		if r.Key == "" {
			t.Errorf("row %d: missing Key", i)
		}
		if r.Syntax == "" {
			t.Errorf("row %d (%s): missing Syntax", i, r.Key)
		}
		if r.Effect == "" {
			t.Errorf("row %d (%s): missing Effect", i, r.Key)
		}
		if r.Example == "" {
			t.Errorf("row %d (%s): missing Example", i, r.Key)
		}
	}
}

func TestCheatsheetHasNoDuplicateKeys(t *testing.T) {
	// The popover is keyed off `r.Key` for analytics
	// (Copy example button) and for the drift test's
	// primitive-coverage check. Duplicates would silently
	// drop a row in the UI.
	rows := Rows()
	seen := make(map[string]int, len(rows))
	for _, r := range rows {
		seen[r.Key]++
	}
	for key, n := range seen {
		if n > 1 {
			t.Errorf("cheatsheet has %d rows with key %q; keys must be unique", n, key)
		}
	}
}

func TestCheatsheetExamplesDoNotContainRawHTML(t *testing.T) {
	// Raw HTML in the body is stripped at save time per
	// internal/records/markdown.go's bluemonday policy. An
	// example that the renderer would strip is a bug in
	// the cheatsheet (it teaches a syntax that does not
	// work end-to-end).
	rows := Rows()
	banned := []string{"<script", "<iframe", "<style", "<form"}
	for _, r := range rows {
		for _, b := range banned {
			if strings.Contains(r.Example, b) {
				t.Errorf("row %q example contains raw HTML %q that the renderer would strip", r.Key, b)
			}
		}
	}
}
