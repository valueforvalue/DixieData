// Package bare_templ holds the fixtures for the baretempl
// analyzer. The package-level doc intentionally does NOT use
// the magic comment keyword to avoid analysistest confusion.
package bare_templ

import (
	"context"
	"io"
)

// Component is a stand-in for a templ.Component. The analyzer
// flags it conservatively (no type info available in this
// isolated test) so the test mirrors real-world conditions.
type Component struct{}

func (c Component) Render(ctx context.Context, w io.Writer) error {
	return nil
}

// Bad: a bare .Render call as an ExprStmt. The analyzer must
// report this site.
func badBare(c Component, w io.Writer) {
	c.Render(context.Background(), w) // want `bare templ\.Component\.Render`
}

// Bad: a bare .Render call that is the only statement in a
// function. The analyzer must report this site.
func badBare2(c Component, w io.Writer) {
	c.Render(context.Background(), w) // want `bare templ\.Component\.Render`
}

// Good: the .Render call is wrapped in an if err := ... check.
func goodIfErrAssign(c Component, w io.Writer) {
	if err := c.Render(context.Background(), w); err != nil {
		return
	}
}

// Good: the .Render call is the argument of a
// respondErrorFragment helper. (We use a no-op function name
// that matches the analyzer's responder-name set.)
func goodWrapped(c Component, w io.Writer) {
	respondErrorFragment(c.Render(context.Background(), w))
}

// Good: the bail-out. The //nolint:dixie/baretempl comment
// suppresses the diagnostic.
func goodBailOut(c Component, w io.Writer) {
	c.Render(context.Background(), w) //nolint:dixie/baretempl
}

func respondErrorFragment(_ error) {}
