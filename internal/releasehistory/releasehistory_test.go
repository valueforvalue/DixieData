// releasehistory_test.go -- issue #585 slice 2.
//
// Pins the Baked() accessor contract on the parent package.
// The parser tests live in parse/parse_test.go (sub-package)
// after the issue #588 split that broke the chicken-egg
// between this package's `baked` var (declared in the
// gitignored generated baked.go) and the bake script that
// produces baked.go.
//
// All releases ship with a baked JSON baked at release time;
// this test pins the empty-state shape (baked == nil -> empty
// entries slice).
package releasehistory

import (
	"testing"
)

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