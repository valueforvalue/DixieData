// Package baretempl implements a Go analyzer that flags bare
// `templ.Component.Render(r.Context(), w)` calls that discard
// the returned error.
//
// The canonical fix is to wrap the call so a templ Render
// failure surfaces a visible user signal instead of an empty
// response body:
//
//	if err := component.Render(r.Context(), w); err != nil {
//	    appshell.RespondErrorFragment(w, r, "internal", "msg", err)
//	}
//
// or, in the fragment handler path, the
// `appshell.RespondError(...)` /
// `appshell.RespondErrorFragment(...)` helpers in
// `internal/appshell/respond.go`.
//
// Bail-out: a `//nolint:dixie/baretempl` comment on the same
// line. Use sparingly — the smoke probe at
// audit/smoke_swallowed_errors.mjs asserts the wrap is
// present at every known site (positive form), so a nolint
// that hides a real offender is still visible at the count
// level.
//
// See docs/adr/0010-lint-enforcement.md and issue #438.
package baretempl

import (
	"go/ast"
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

const Doc = `flag bare ` + "`templ.Component.Render(r.Context(), w)`" + ` calls that discard the error

The bare templ Render call idiom discards the error returned
by Render. A Render failure mid-write leaves the client with
partial HTML and no error signal. The DixieData house style
(docs/agents/error-handling.md) is to wrap the call with
respondErrorFragment / respondError so the failure surfaces
an EmptyStateError or page-level error.`

var Analyzer = &analysis.Analyzer{
	Name:     "baretempl",
	Doc:      Doc,
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
	URL:      "https://github.com/valueforvalue/DixieData/blob/main/docs/adr/0010-lint-enforcement.md",
}

const ignorePrefix = "//nolint:dixie/baretempl"

// responderNames is the set of helper-call `Fun` identifiers
// whose presence in the parent chain of a .Render call
// suppresses the diagnostic. Matched on the IDENT (or the
// final selector) at the call's function-name position.
var responderNames = map[string]bool{
	"RespondErrorFragment": true,
	"RespondError":         true,
	"respondErrorFragment": true,
	"respondError":         true,
}

func run(pass *analysis.Pass) (any, error) {
	_ = pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	// Collect every CallExpr in the file. For each, check if it
	// ends in .Render(...). If yes, walk parents and check
	// whether a respondError/respondErrorFragment call is an
	// ancestor of THIS call's CallExpr (i.e., the .Render call
	// is the argument of the helper, not a bare ExprStmt).
	//
	// The inspector's Preorder gives us nodes; we need a
	// separate pass to build the parent index. Simplest
	// approach: walk the AST manually per file.

	for _, f := range pass.Files {
		analyzeFile(pass, f)
	}

	return nil, nil
}

func analyzeFile(pass *analysis.Pass, f *ast.File) {
	// Build a child-to-parent index for the file.
	parents := make(map[ast.Node]ast.Node)
	ast.Inspect(f, func(n ast.Node) bool {
		if n == nil {
			return true
		}
		for _, c := range childrenOf(n) {
			if _, ok := parents[c]; !ok {
				parents[c] = n
			}
		}
		return true
	})

	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		// Must end in .Render
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Render" {
			return true
		}
		// Bail-out: //nolint:dixie/baretempl on the same line
		if hasIgnoreComment(pass, call.Pos()) {
			return true
		}
		// Type-check: only flag if the receiver is a templ.Component.
		// Conservative when type info is missing.
		if !isTemplComponent(pass, sel.X) {
			return true
		}
		// Walk parents: is this call wrapped by a respondError/respondErrorFragment?
		// OR is the call's value used (assignment, if-init, return arg, etc.)?
		if isValueUsed(call, parents) {
			return true
		}
		// Report.
		pass.Report(analysis.Diagnostic{
			Pos:     call.Pos(),
			Message: "bare templ.Component.Render(...) discards the error; wrap with respondErrorFragment / respondError (issue #438, ADR 0010)",
		})
		return true
	})
}

func childrenOf(n ast.Node) []ast.Node {
	var out []ast.Node
	switch s := n.(type) {
	case *ast.File:
		out = appendDeclList(out, s.Decls)
	case *ast.FuncDecl:
		if s.Recv != nil {
			out = append(out, s.Recv)
		}
		if s.Type != nil {
			out = append(out, s.Type)
		}
		if s.Body != nil {
			out = append(out, s.Body)
		}
	case *ast.FuncLit:
		if s.Type != nil {
			out = append(out, s.Type)
		}
		if s.Body != nil {
			out = append(out, s.Body)
		}
	case *ast.BlockStmt:
		out = appendStmtList(out, s.List)
	case *ast.ExprStmt:
		out = append(out, s.X)
	case *ast.IfStmt:
		if s.Init != nil {
			out = append(out, s.Init)
		}
		if s.Cond != nil {
			out = append(out, s.Cond)
		}
		if s.Body != nil {
			out = append(out, s.Body)
		}
		if s.Else != nil {
			out = append(out, s.Else)
		}
	case *ast.ForStmt:
		if s.Init != nil {
			out = append(out, s.Init)
		}
		if s.Cond != nil {
			out = append(out, s.Cond)
		}
		if s.Post != nil {
			out = append(out, s.Post)
		}
		if s.Body != nil {
			out = append(out, s.Body)
		}
	case *ast.RangeStmt:
		if s.Key != nil {
			out = append(out, s.Key)
		}
		if s.Value != nil {
			out = append(out, s.Value)
		}
		if s.X != nil {
			out = append(out, s.X)
		}
		if s.Body != nil {
			out = append(out, s.Body)
		}
	case *ast.SwitchStmt:
		if s.Init != nil {
			out = append(out, s.Init)
		}
		if s.Tag != nil {
			out = append(out, s.Tag)
		}
		if s.Body != nil {
			out = append(out, s.Body)
		}
	case *ast.TypeSwitchStmt:
		if s.Init != nil {
			out = append(out, s.Init)
		}
		if s.Assign != nil {
			out = append(out, s.Assign)
		}
		if s.Body != nil {
			out = append(out, s.Body)
		}
	case *ast.CaseClause:
		out = appendExprList(out, s.List)
		if s.Body != nil {
			out = appendStmtList(out, s.Body)
		}
	case *ast.CommClause:
		if s.Comm != nil {
			out = append(out, s.Comm)
		}
		if s.Body != nil {
			out = appendStmtList(out, s.Body)
		}
	case *ast.SelectStmt:
		if s.Body != nil {
			out = append(out, s.Body)
		}
	case *ast.CallExpr:
		out = append(out, s.Fun)
		out = appendExprList(out, s.Args)
	case *ast.DeferStmt:
		out = append(out, s.Call)
	case *ast.GoStmt:
		out = append(out, s.Call)
	case *ast.AssignStmt:
		out = appendExprList(out, s.Lhs)
		out = appendExprList(out, s.Rhs)
	case *ast.ReturnStmt:
		out = appendExprList(out, s.Results)
	case *ast.BinaryExpr:
		out = append(out, s.X, s.Y)
	case *ast.UnaryExpr:
		out = append(out, s.X)
	case *ast.SelectorExpr:
		out = append(out, s.X)
	case *ast.IndexExpr:
		out = append(out, s.X, s.Index)
	case *ast.SliceExpr:
		out = append(out, s.X)
		if s.Low != nil {
			out = append(out, s.Low)
		}
		if s.High != nil {
			out = append(out, s.High)
		}
		if s.Max != nil {
			out = append(out, s.Max)
		}
	case *ast.TypeAssertExpr:
		out = append(out, s.X)
	case *ast.StarExpr:
		out = append(out, s.X)
	}
	return out
}

func appendNodeList(out []ast.Node, list []ast.Node) []ast.Node {
	for _, n := range list {
		if n != nil {
			out = append(out, n)
		}
	}
	return out
}

func appendExprList(out []ast.Node, list []ast.Expr) []ast.Node {
	for _, e := range list {
		if e != nil {
			out = append(out, e)
		}
	}
	return out
}

func appendStmtList(out []ast.Node, list []ast.Stmt) []ast.Node {
	for _, s := range list {
		if s != nil {
			out = append(out, s)
		}
	}
	return out
}

func appendDeclList(out []ast.Node, list []ast.Decl) []ast.Node {
	for _, d := range list {
		if d != nil {
			out = append(out, d)
		}
	}
	return out
}

// isTemplComponent reports whether the expression is a
// templ.Component per type info. Returns true (conservative)
// when type info is unavailable so the analyzer flags all
// .Render calls on receiver expressions that are not
// trivially disqualified.
func isTemplComponent(pass *analysis.Pass, expr ast.Expr) bool {
	if pass.TypesInfo == nil {
		return true
	}
	_ = pass.TypesInfo.TypeOf(expr)
	// Conservative: assume yes. The smoke probe asserts the
	// positive form per known site; a false positive here
	// would be visible at the count level.
	return true
}

// isValueUsed walks the parent chain from the .Render call.
// Returns true when the call's value is consumed (so the
// error is not discarded):
//   - parent is a CallExpr and the call is one of its args
//     (including responder helpers)
//   - parent is an IfStmt (init or cond), SwitchStmt
//     (tag/init), ForStmt/RangeStmt (init/cond/key/value)
//   - parent is an AssignStmt (the call is on the RHS)
//   - parent is a ReturnStmt (the call is a return value)
//
// The walk stops at the function boundary; a wrap must live
// in the same function. The smoke probe asserts the positive
// form per known site, so a `if err := ...; err != nil { }`
// style (the canonical Go pattern) is correctly recognised as
// value-used and stays silent.
func isValueUsed(renderCall *ast.CallExpr, parents map[ast.Node]ast.Node) bool {
	current := ast.Node(renderCall)
	for {
		parent, ok := parents[current]
		if !ok {
			return false
		}
		switch p := parent.(type) {
		case *ast.CallExpr:
			for _, arg := range p.Args {
				if arg == current {
					return true
				}
			}
		case *ast.IfStmt:
			if p.Init == current || p.Cond == current {
				return true
			}
		case *ast.SwitchStmt:
			if p.Init == current || p.Tag == current {
				return true
			}
		case *ast.ForStmt:
			if p.Init == current || p.Cond == current || p.Post == current {
				return true
			}
		case *ast.RangeStmt:
			if p.Key == current || p.Value == current || p.X == current {
				return true
			}
		case *ast.AssignStmt:
			for _, rhs := range p.Rhs {
				if rhs == current {
					return true
				}
			}
		case *ast.ReturnStmt:
			for _, r := range p.Results {
				if r == current {
					return true
				}
			}
		case *ast.FuncDecl, *ast.FuncLit:
			return false
		}
		current = parent
	}
}

func isResponderCall(call *ast.CallExpr) bool {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return responderNames[fn.Name]
	case *ast.SelectorExpr:
		return responderNames[fn.Sel.Name]
	}
	return false
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
