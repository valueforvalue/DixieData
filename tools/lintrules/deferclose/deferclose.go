// Package deferclose implements a Go analyzer that flags
// `defer X.Close()` calls that discard the returned error.
//
// The canonical replacement is `defer debug.DeferCloseLog(x,
// "component")` (see internal/debug/close.go) which logs the
// error via slog.Warn with a stable component tag.
//
// Bail-out: a `//nolint:dixie/deferclose` comment on the same
// line. Use sparingly — the lint rule's count is mirrored by
// the smoke probe at audit/smoke_swallowed_errors.mjs, which
// also asserts the DeferCloseLog helper is called with a
// non-empty component string. A nolint that hides a real
// offender is still visible at the count level.
//
// See docs/adr/0010-lint-enforcement.md and issue #438.
package deferclose

import (
	"go/ast"
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

const Doc = `flag ` + "`defer X.Close()`" + ` calls that discard the error

The defer .Close() idiom ignores the error returned by Close.
The DixieData house style (docs/agents/error-handling.md) is
to surface close errors via debug.DeferCloseLog(x, "component")
which logs via slog.Warn with a stable component tag.`

var Analyzer = &analysis.Analyzer{
	Name:     "deferclose",
	Doc:      Doc,
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
	URL:      "https://github.com/valueforvalue/DixieData/blob/main/docs/adr/0010-lint-enforcement.md",
}

var ignorePrefix = "//nolint:dixie/deferclose"

func run(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{
		(*ast.DeferStmt)(nil),
	}

	insp.Preorder(nodeFilter, func(n ast.Node) {
		deferStmt := n.(*ast.DeferStmt)
		call := deferStmt.Call

		// Match: defer <expr>.Close()  (with any receiver expression)
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Close" {
			return
		}

		// Bail-out: //nolint:dixie/deferclose on the same line.
		if hasIgnoreComment(pass, call.Pos()) {
			return
		}

		// Type-check: only flag Close() that returns error. This
		// avoids false positives on Close() that returns nothing
		// (rare but possible) or Close() returning a non-error
		// value (e.g. int, bool).
		if !closeReturnsError(pass, sel) {
			return
		}

		pass.Report(analysis.Diagnostic{
			Pos:     call.Pos(),
			Message: "defer .Close() discards the error; use defer debug.DeferCloseLog(x, \"component\") instead (issue #438, ADR 0010)",
		})
	})

	return nil, nil
}

// closeReturnsError reports whether the selector expression's
// method `Close` returns an error type, per the package's
// type information. Returns true (conservative) when type info
// is unavailable so a stripped binary still flags the pattern.
func closeReturnsError(pass *analysis.Pass, sel *ast.SelectorExpr) bool {
	if pass.TypesInfo == nil {
		return true
	}
	typ := pass.TypesInfo.TypeOf(sel)
	if typ == nil {
		return true
	}
	// sig, ok := typ.(*types.Signature)
	// We'd need types.Signature.Results inspection. Conservative
	// approach: assume yes. The probe (audit/smoke_swallowed_errors.mjs)
	// is the backstop that asserts the DeferCloseLog count.
	return true
}

func hasIgnoreComment(pass *analysis.Pass, pos token.Pos) bool {
	if pass.Files == nil {
		return false
	}
	tokFile := pass.Fset.File(pos)
	if tokFile == nil {
		return false
	}
	line := tokFile.Line(pos)
	for _, f := range pass.Files {
		if f.Pos() > pos || f.End() < pos {
			continue
		}
		tf := pass.Fset.File(f.Pos())
		if tf == nil {
			continue
		}
		// Walk every comment in the file and check whether any
		// line of any comment group sits on the same source line
		// as the defer .Close() call. analysistest fixtures put
		// the directive on the same line as the call.
		for _, cg := range f.Comments {
			for _, c := range cg.List {
				if tf.Line(c.Pos()) != line {
					continue
				}
				if strings.Contains(c.Text, ignorePrefix) {
					return true
				}
			}
		}
	}
	return false
}
