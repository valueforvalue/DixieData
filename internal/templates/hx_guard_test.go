package templates

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// hxSubmitRe matches hx-post, hx-put, hx-delete, or hx-confirm
// literal strings in a templ file (used to forbid these attributes
// outside the polling fragments, after the Option C templ retag).
var hxSubmitRe = regexp.MustCompile(`hx-(post|put|delete|confirm)`)

// pollingFiles lists the .templ files where hx-get on a polling
// element is allowed. Everything else must use Option C's
// data-dixie-submit / data-action convention.
//
// Why these files specifically: layout.templ includes the global
// jobs-progress overlay that polls /jobs/active every 3s; jobs.templ
// and job_slot_fragment.templ poll /jobs/{id} every 2s for the
// status pill. Polling is a GET that returns an HTML fragment; it
// is the only legitimate hx-get use in this codebase. hx-post /
// hx-put / hx-delete are never legitimate after the templ retag.
var pollingFiles = map[string]bool{
	"layout.templ":            true,
	"jobs.templ":              true,
	"job_slot_fragment.templ": true,
}

// scanTemplFile scans one .templ file for hx-post / hx-put /
// hx-delete / hx-confirm attributes. Returns the offenders
// found, each as a "file:line — attrs — src" string ready for
// t.Errorf inclusion. Extracted from TestNoPostThenNavigateHXXAttrs
// so the regression net (hxm_guard_comment_filter_test.go) can
// exercise the scanner logic directly without re-running the
// production test.
func scanTemplFile(path string) ([]string, error) {
	base := filepath.Base(path)
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var offenders []string
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		// Issue #414: skip `//` comment lines. The earlier
		// scanner matched hx-confirm inside a doc comment
		// (soldier_card.templ:691) as a real offender. Comment
		// text is documentation, not a templ attribute, so it
		// cannot be a real htmx call.
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		if matches := hxSubmitRe.FindAllString(line, -1); len(matches) > 0 {
			offenders = append(offenders, formatTemplOffender(base, lineNum, matches, line))
		}
	}
	return offenders, scanner.Err()
}

// TestNoPostThenNavigateHXXAttrs walks every .templ file and
// fails if any file contains hx-post / hx-put / hx-delete /
// hx-confirm attributes. After the Option C templ retag, every
// POST-then-navigate flow uses action= or data-action +
// data-dixie-submit. hx-confirm is replaced by data-confirm.
//
// Polling fragments (layout.templ, jobs.templ, job_slot_fragment.templ)
// are allowed to retain hx-get for fragment polling; they never
// carry hx-post / hx-put / hx-delete / hx-confirm so they're
// unaffected by this check.
//
// The regression net is a source-scan because it has to fire
// before any code runs. If a future contributor adds hx-post /
// hx-delete / hx-confirm to a non-polling file, this test fails
// the build with a file:line citation pointing at the offender.
//
// Allowed escapes:
//   - hx-get / hx-trigger / hx-target / hx-swap on polling
//     elements (handled by the pollingFiles allow-list; they
//     can stay because Option C kept htmx for GET polling).
func TestNoPostThenNavigateHXXAttrs(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	thisDir := filepath.Dir(thisFile)

	templFiles, err := filepath.Glob(filepath.Join(thisDir, "*.templ"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}

	var offenders []string
	for _, file := range templFiles {
		fileOffenders, err := scanTemplFile(file)
		if err != nil {
			t.Errorf("scan %s: %v", filepath.Base(file), err)
			continue
		}
		offenders = append(offenders, fileOffenders...)
	}

	if len(offenders) > 0 {
		t.Errorf("found %d post-then-navigate hx-* attrs in non-conforming templates. After Option C, every POST-then-navigate flow uses data-dixie-submit + data-action (forms) or data-dixie-submit + data-action (bare buttons). hx-confirm is replaced by data-confirm.\n  %s",
			len(offenders), strings.Join(offenders, "\n  "))
	}
	_ = pollingFiles // documented above; reserved for future hx-get enforcement.
}

func formatTemplOffender(file string, line int, matches []string, src string) string {
	trimmed := strings.TrimSpace(src)
	if len(trimmed) > 100 {
		trimmed = trimmed[:100] + "..."
	}
	return file + ":" + itoa(line) + " — " + strings.Join(matches, ",") + " — " + trimmed
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if negative {
		return "-" + string(digits)
	}
	return string(digits)
}