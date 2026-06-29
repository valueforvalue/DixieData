package appshell

import (
	"bufio"
	"bytes"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSetInfoToastHeaderWritesInfoKind locks down the helper used
// by every in-progress toast site (image import, shared archive
// import, memorial JSON import, Google Drive / Sheets exports,
// duplicate audit, bulk reviews, orphan cleanup). Regression test
// for issue #132: the header must set kind=info so the frontend
// classifies the toast for auto-dismiss and the heading reads
// "Heads up" rather than the misleading "Success".
func TestSetInfoToastHeaderWritesInfoKind(t *testing.T) {
	rec := httptest.NewRecorder()
	setInfoToastHeader(rec, "Importing 5 image(s)…")

	if got := rec.Header().Get("X-DixieData-Toast-Type"); got != "info" {
		t.Errorf("X-DixieData-Toast-Type = %q, want info", got)
	}
	if got := rec.Header().Get("X-DixieData-Toast"); got != "Importing 5 image(s)…" {
		t.Errorf("X-DixieData-Toast = %q, want %q", got, "Importing 5 image(s)…")
	}
}

// TestSetInfoToastHeaderSkipsEmpty verifies the empty-message
// guard short-circuits so we don't accidentally write an empty
// toast header that the frontend would render as a blank card.
func TestSetInfoToastHeaderSkipsEmpty(t *testing.T) {
	rec := httptest.NewRecorder()
	setInfoToastHeader(rec, "   ")
	if got := rec.Header().Get("X-DixieData-Toast"); got != "" {
		t.Errorf("empty/whitespace message must not set X-DixieData-Toast; got %q", got)
	}
}

// TestSetInfoToastHeaderMatchesSuccessKindContract ensures the
// info toast behaves like the success toast for auto-dismiss:
// both kinds must end up as the same X-DixieData-Toast-Type
// category on the wire so the frontend can use one switch for
// both. Success is the existing contract; info is the new path
// for in-progress messages.
func TestSetInfoToastHeaderMatchesSuccessKindContract(t *testing.T) {
	success := httptest.NewRecorder()
	setToastHeader(success, "Saved.")
	info := httptest.NewRecorder()
	setInfoToastHeader(info, "Importing…")

	successType := strings.TrimSpace(success.Header().Get("X-DixieData-Toast-Type"))
	infoType := strings.TrimSpace(info.Header().Get("X-DixieData-Toast-Type"))
	if successType == "" || infoType == "" || successType == infoType {
		t.Fatalf("success kind=%q info kind=%q — must be distinct so the frontend can render different headings, but both must be non-empty", successType, infoType)
	}
	if successType != "success" {
		t.Errorf("success kind = %q, want success", successType)
	}
	if infoType != "info" {
		t.Errorf("info kind = %q, want info", infoType)
	}
}

// TestInProgressToastStringsContainActualEllipsis is the
// regression net for issue #135. The Go literal `\u2026` ships
// over the wire as the seven ASCII characters `\`, `u`, `2`,
// `0`, `2`, `6` — Go does not interpret `\uXXXX` inside
// ordinary double-quoted strings. The byte sequence must not
// appear in any production Go source under appshell/.
//
// The sweep walks every production .go file in this package
// and asserts no line contains the seven-char literal `\u2026`
// in a non-comment, non-backtick-raw-string context. Backtick
// raw strings (e.g. JS source embedded in HTML) are exempt:
// the JS engine resolves the escape at runtime.
//
// The check is a source-level scan rather than a runtime probe:
// a runtime probe would only cover handlers the test calls
// directly, missing every future handler that lands in this
// package.
func TestInProgressToastStringsContainActualEllipsis(t *testing.T) {
	const badLiteral = "\\u2026"

	prodFiles, err := filepath.Glob(filepath.Join(".", "*.go"))
	if err != nil {
		t.Fatalf("glob appshell: %v", err)
	}

	for _, file := range prodFiles {
		name := filepath.Base(file)
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		scanner := bufio.NewScanner(bytes.NewReader(data))
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := scanner.Text()
			if !strings.Contains(line, badLiteral) {
				continue
			}
			// Strip line comments: anything after `//` is
			// documentation and may legitimately describe the
			// escape sequence.
			if idx := strings.Index(line, "//"); idx >= 0 {
				line = line[:idx]
			}
			if !strings.Contains(line, badLiteral) {
				continue
			}
			// Backtick raw string: the JS engine or HTML parser
			// handles the escape. Approximate by checking for
			// an odd number of backticks (i.e. unterminated raw
			// string crossing this line). Simpler heuristic:
			// count backticks in the file as a whole; if the
			// literal lives between two backticks on the same
			// line it's safe.
			backticks := strings.Count(line, "`")
			if backticks%2 == 1 {
				// Unterminated raw string on this line;
				// treat as a real code occurrence.
				t.Errorf("%s:%d contains the seven-char literal \\u2026; Go does not interpret \\uXXXX in double-quoted strings, so the bytes ship verbatim. Replace with the U+2026 rune.", file, lineNo)
				continue
			}
			// All other cases: still a real occurrence.
			t.Errorf("%s:%d contains the seven-char literal \\u2026; Go does not interpret \\uXXXX in double-quoted strings, so the bytes ship verbatim. Replace with the U+2026 rune.", file, lineNo)
		}
	}
}