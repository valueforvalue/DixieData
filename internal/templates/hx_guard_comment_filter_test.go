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
// directly.
//
// As of issue #414 final slice (entry_form.templ + soldier_card.templ
// both migrated to real <form enctype="multipart/form-data">
// shapes), zero production .templ files carry hx-* attrs.
// TestNoPostThenNavigateHXXAttrsCommentFalsePositivesGone below
// remains the active regression net: synthetic .templ with a
// comment + a real attr asserts the comment line is filtered AND
// the real attr line is flagged. If a future refactor filters
// too aggressively, this synthetic case catches it; if a future
// PR adds a new hx-post offender, TestNoPostThenNavigateHXXAttrs
// (in hx_guard_test.go) catches it first.
//
// An earlier form of this file also asserted
// TestNoPostThenNavigateHXXAttrsGuardFileStillFlagsImageUploads
// as a safety net pinning "real hx-post offender files still get
// flagged". Slice 1/2 + Slice 2/2 of the #414 migration removed
// both real offender files; the synthetic case above is the only
// assertion left because the safety net has no production offender
// to anchor against. Future migrations that re-introduce hx-* on
// a real form will surface again via TestNoPostThenNavigateHXXAttrs
// itself — and the next slice of #414-style work should re-introduce
// the safety-net test against the new offender list.
//
// scanTemplFile lives in hx_guard_test.go (same package); if
// a future refactor removes the helper, this file fails to
// build, which is the intended RED signal.
package templates

import (
	"os"
	"path/filepath"
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
