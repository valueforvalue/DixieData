// Issue #414: the hx-guard test used to false-positive on the
// string `hx-confirm` inside a `//` doc comment
// (soldier_card.templ:691 in the original commit). The scanner
// is line-by-line; without a comment filter, every line that
// contains the literal hx-confirm/hx-post/hx-delete substring
// matches, even when the line is documentation.
//
// RED-first regression net: this test file pins the new
// filter's behaviour by exercising scanTemplFile (the helper
// extracted from TestNoPostThenNavigateHXXAttrs in hx_guard_test.go)
// directly. Two cases:
//
//   1. Synthetic .templ with a comment + a real attr: the
//      comment line is filtered, the attr line is flagged.
//   2. The two known-real hx-post offender files
//      (entry_form.templ + soldier_card.templ) still report
//      offenders. If a future refactor filters too aggressively
//      and silently drops real offenders, this case fails.
//
// scanTemplFile lives in hx_guard_test.go (same package); if
// a future refactor removes the helper, this file fails to
// build, which is the intended RED signal.
package templates

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestNoPostThenNavigateHXXAttrsCommentFalsePositivesGone
// verifies the comment filter via the production scanner
// helper. Writes a temp .templ with two `//` comment lines
// (one carrying hx-confirm, one carrying hx-post) + one
// real hx-post attribute line. Asserts only the real
// attribute line is flagged.
func TestNoPostThenNavigateHXXAttrsCommentFalsePositivesGone(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "synthetic.templ")
	content := "// dependency. hx-confirm preserves the dialog UX on a real <button>\n" +
		"// and hx-post was the previous sentinel, now replaced by data-dixie-submit.\n" +
		`hx-post={ templ.SafeURL("/soldiers/1/images/import") }` + "\n"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	offenders, err := scanTemplFile(tmp)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(offenders) != 1 {
		t.Fatalf("expected 1 offender (real hx-post on line 3); got %d:\n%s",
			len(offenders), strings.Join(offenders, "\n"))
	}
	if !strings.Contains(offenders[0], ":3") {
		t.Errorf("expected offender to cite line 3; got: %s", offenders[0])
	}
}

// TestNoPostThenNavigateHXXAttrsGuardFileStillFlagsImageUploads
// is the safety net: the 2 real hx-post offenders on the
// image-upload forms (entry_form.templ + soldier_card.templ)
// must STILL be flagged after the comment filter. A future
// refactor that filters too aggressively (e.g. also skips
// hx-* attrs on real attribute lines) would let those two
// real offenders hide behind the filter; this test catches
// that regression by asserting the live production scanner
// reports at least one hx-post offender in the repo.
func TestNoPostThenNavigateHXXAttrsGuardFileStillFlagsImageUploads(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	thisDir := filepath.Dir(thisFile)

	knownReal := []string{"entry_form.templ", "soldier_card.templ"}
	for _, file := range knownReal {
		path := filepath.Join(thisDir, file)
		offenders, err := scanTemplFile(path)
		if err != nil {
			t.Errorf("scan %s: %v", file, err)
			continue
		}
		if len(offenders) == 0 {
			t.Errorf("%s: expected at least one hx-post offender (image upload form not yet migrated to Option C); got 0", file)
		}
	}
}