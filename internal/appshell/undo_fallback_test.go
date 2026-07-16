// undo_fallback_test.go — issue #611 slice 1 regression net.
//
// Pins the contract that the 3 JS-side fallback paths in
// frontend/app.js (initializeMarkdownCheatsheet +
// initializeEditorToolbar + initializeTableBuilder) do NOT
// assign to textarea.value (which would clobber the
// browser's native undo stack). The slice-1 fix replaces
// those assignments with textarea.setRangeText(...) — a
// browser API that preserves the undo stack — so the
// fallback path is no worse than the happy path.
//
// We use a static source check (not a runtime test) because
// the fallback code is inlined in three call sites in
// app.js; extracting it into a shared helper would create
// a new failure mode (the helper itself might fail to load,
// the very class of bug #610 just fixed). The source check
// is brittle to refactors (renaming `textarea.value` to
// `ta.value`, for example) but those refactors are also
// opportunities to revisit the undo contract.
//
// The check is intentionally narrow: we look for the
// specific anti-pattern
//   textarea.value = textarea.value.slice(0, ...
// inside the three fallback paths. We do NOT grep the
// whole app.js for `textarea.value =` (that would catch
// the legitimate initialization in the happy path, which
// is also fine because it's the value of a NEW textarea
// not the existing one).
package appshell

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

// TestFrontend_FallbackPathsPreserveUndo pins the slice-1
// fix: the 3 inlined fallback paths in app.js must NOT
// contain the `textarea.value = textarea.value.slice(...)`
// anti-pattern. The fix replaces those with
// `textarea.setRangeText(...)` so the browser undo stack
// is preserved when the helper is missing.
func TestFrontend_FallbackPathsPreserveUndo(t *testing.T) {
	// Locate the repo root (the test runs from internal/appshell).
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root := wd
	for i := 0; i < 5; i += 1 {
		if _, err := os.Stat(root + "/go.mod"); err == nil {
			break
		}
		root = root + "/.."
	}

	appJSPath := root + "/frontend/app.js"
	f, err := os.Open(appJSPath)
	if err != nil {
		t.Skipf("app.js not found at %s: %v", appJSPath, err)
	}
	defer f.Close()

	// Scan app.js line by line. We're looking for the
	// `textarea.value = textarea.value.slice(...)` pattern
	// which appears in 3 places (one per fallback). The
	// happy path uses `textarea.setRangeText(...)` (we
	// don't check for that here — covered by the slice-2
	// helper test in insert_text_at_cursor.test.mjs).
	scanner := bufio.NewScanner(f)
	lineNum := 0
	antiPattern := "textarea.value = textarea.value.slice"
	foundAntiPattern := []int{}
	for scanner.Scan() {
		lineNum += 1
		line := scanner.Text()
		if strings.Contains(line, antiPattern) {
			foundAntiPattern = append(foundAntiPattern, lineNum)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}

	if len(foundAntiPattern) > 0 {
		t.Errorf("found %d occurrences of the undo-clobbering anti-pattern at lines %v. The 3 fallback paths in app.js must use textarea.setRangeText(...) instead of textarea.value = textarea.value.slice(...) so the browser undo stack is preserved (issue #611 slice 1).", len(foundAntiPattern), foundAntiPattern)
	}
}