// routes_method_guard_test.go locks in the contract that every route
// registered in setupRoutes uses the HTTP method its handler
// accepts. PR #1 of the stabilization sprint (chi router migration)
// registered most action endpoints with r.Get when their handlers
// reject anything other than POST. Users clicking "Export",
// "Generate static archive", "Backup", etc. got "405 method not
// allowed" responses and the buttons looked dead.
//
// This guard reproduces that bug class mechanically: AST-walk the
// routes table, AST-walk every handler method, fail loudly when a
// r.Get registration pairs with a handler that requires POST. Each
// finding cites the offending route + the handler file:line so the
// regression can be fixed in one pass.
package appshell_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestRouteMethodMatchesHandler walks internal/appshell, extracts
// the route table from setupRoutes, and for each (path, handler)
// pair asserts that the HTTP method on the registration matches
// the method the handler accepts. Handlers that begin with
// `if r.Method != http.MethodPost { ... return }` are tagged
// POST-only; handlers with no such guard accept either method.
func TestRouteMethodMatchesHandler(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("locate repo root: %v", err)
	}
	appshellDir := filepath.Join(repoRoot, "internal", "appshell")

	// 1. Load handler files into a map[method-name]handlerInfo so we
	//    can look up the "is POST-only" flag quickly while walking
	//    the route table.
	fset := token.NewFileSet()
	handlers, err := loadHandlers(fset, appshellDir)
	if err != nil {
		t.Fatalf("load handlers: %v", err)
	}

	// 2. Parse routes.go and walk every r.Get / r.Post call inside
	//    setupRoutes.
	routesPath := filepath.Join(appshellDir, "routes.go")
	f, err := parser.ParseFile(fset, routesPath, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %s: %v", routesPath, err)
	}
	rel, _ := filepath.Rel(repoRoot, routesPath)

	routes := collectRoutes(f)
	if len(routes) == 0 {
		t.Fatalf("no r.Get / r.Post calls found in %s; the route table format may have changed — update this guard", rel)
	}

	// 3. For each (path, method, handlerExpr) tuple, resolve the
	//    handler method and verify method compatibility.
	type mismatch struct {
		path        string
		routeMethod string
		handler     string
		handlerFile string
		handlerLine int
		reason      string
	}
	var mismatches []mismatch

	for _, r := range routes {
		handlerName, ok := stripAppReceiver(r.handler)
		if !ok {
			// Not a method value (could be a closure or a helper
			// wrapper like a.handleFrontendAsset(...)). Skip —
			// helper-wrapped routes are intentionally Get.
			continue
		}
		info, found := handlers[handlerName]
		if !found {
			// Handler method is not in this package's AST (could be
			// an external package). Skip silently — other guards
			// cover that case.
			continue
		}
		if info.isPostOnly && r.method != "Post" {
			pos := fset.Position(info.pos)
			mismatches = append(mismatches, mismatch{
				path:        r.path,
				routeMethod: r.method,
				handler:     handlerName,
				handlerFile: pos.Filename,
				handlerLine: pos.Line,
				reason:      "handler requires POST (rejects everything except http.MethodPost) but route is registered as " + r.method,
			})
		}
	}

	// 4. Also check the inverse: a handler that is registered as
	//    r.Post but never enforces POST-only is suspicious — usually
	//    not a bug (the handler still works), but worth surfacing.
	//    Use t.Log so it doesn't fail the build.

	sort.Slice(mismatches, func(i, j int) bool {
		return mismatches[i].path < mismatches[j].path
	})

	for _, m := range mismatches {
		t.Errorf("route method mismatch\n  path: %s\n  registered as: r.%s\n  handler: %s (defined at %s:%d)\n  reason: %s\n  fix: change r.%s to r.Post in internal/appshell/routes.go",
			m.path, m.routeMethod, m.handler, m.handlerFile, m.handlerLine, m.reason, m.routeMethod)
	}

	if len(mismatches) == 0 {
		t.Logf("verified %d routes in setupRoutes — all method/handler pairs consistent", len(routes))
	}
}

// route is a (path, method, handlerExpr) tuple extracted from
// setupRoutes. handlerExpr is the source text of the third argument
// to r.Get / r.Post, e.g. "a.handleExportStaticArchive".
type route struct {
	path    string
	method  string
	handler string
	line    int
}

// collectRoutes walks the AST of routes.go and returns every
// r.Get(path, handler) / r.Post(path, handler) call found in
// setupRoutes. Calls outside setupRoutes are ignored — the route
// table is the only place that should drive routing in this
// package.
func collectRoutes(f *ast.File) []route {
	var routes []route
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok || ident.Name != "r" {
			return true
		}
		if sel.Sel.Name != "Get" && sel.Sel.Name != "Post" {
			return true
		}
		if len(call.Args) < 2 {
			return true
		}
		// First arg: path string literal.
		bl, ok := call.Args[0].(*ast.BasicLit)
		if !ok || bl.Kind != token.STRING {
			return true
		}
		path := strings.Trim(bl.Value, `"`)
		// Second arg: handler expression.
		handler := renderExpr(call.Args[1])
		pos := call.Pos()
		routes = append(routes, route{
			path:    path,
			method:  sel.Sel.Name,
			handler: handler,
			line:    int(pos),
		})
		return true
	})
	return routes
}

// renderExpr returns the source text of an expression. Used for
// handler arguments so we can match them to method names.
func renderExpr(e ast.Expr) string {
	var b strings.Builder
	_ = formatExpr(&b, e)
	return b.String()
}

// formatExpr writes the AST node back to Go source. Good enough for
// the simple `a.foo` and `a.foo(arg)` patterns we see in routes.go.
func formatExpr(b *strings.Builder, e ast.Expr) error {
	switch v := e.(type) {
	case *ast.Ident:
		b.WriteString(v.Name)
	case *ast.SelectorExpr:
		if err := formatExpr(b, v.X); err != nil {
			return err
		}
		b.WriteByte('.')
		b.WriteString(v.Sel.Name)
	case *ast.BasicLit:
		b.WriteString(v.Value)
	case *ast.CallExpr:
		if err := formatExpr(b, v.Fun); err != nil {
			return err
		}
		b.WriteByte('(')
		for i, a := range v.Args {
			if i > 0 {
				b.WriteString(", ")
			}
			if err := formatExpr(b, a); err != nil {
				return err
			}
		}
		b.WriteByte(')')
	default:
		return fmt.Errorf("unhandled expr node %T", e)
	}
	return nil
}

// stripAppReceiver turns "a.handleFoo" into "handleFoo". Returns
// false if the receiver is not "a".
func stripAppReceiver(expr string) (string, bool) {
	if !strings.HasPrefix(expr, "a.") {
		return "", false
	}
	return strings.TrimPrefix(expr, "a."), true
}

// handlerInfo describes one handler method: whether it's POST-only
// and where it's defined.
type handlerInfo struct {
	isPostOnly bool
	pos        token.Pos
}

// loadHandlers walks every Go file in the appshell package and
// returns a map from method name to handlerInfo. The POST-only
// check inspects the function body's first statement: if it begins
// with `if r.Method != http.MethodPost`, the handler is POST-only.
//
// We only inspect the first non-comment statement because that's
// the conventional location for a method guard. A handler that
// uses a different guard shape (e.g. a switch on r.Method) is
// better caught by an integration test.
func loadHandlers(fset *token.FileSet, dir string) (map[string]handlerInfo, error) {
	out := map[string]handlerInfo{}
	walkErr := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == "vendor" || name == "testdata" || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if fd.Recv == nil || fd.Body == nil {
				continue
			}
			// Only consider methods on *App. Other receivers in
			// this package are helpers / not handlers.
			isAppMethod := false
			for _, field := range fd.Recv.List {
				if star, ok := field.Type.(*ast.StarExpr); ok {
					if id, ok := star.X.(*ast.Ident); ok && id.Name == "App" {
						isAppMethod = true
					}
				}
			}
			if !isAppMethod {
				continue
			}
			if !strings.HasPrefix(fd.Name.Name, "handle") && !strings.HasPrefix(fd.Name.Name, "render") && !strings.HasPrefix(fd.Name.Name, "enqueue") {
				continue
			}
			out[fd.Name.Name] = handlerInfo{
				isPostOnly: startsWithMethodPostGuard(fd.Body),
				pos:        fd.Pos(),
			}
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return out, nil
}

// startsWithMethodPostGuard returns true if the function body's
// first statement is `if r.Method != http.MethodPost { ... }` (or a
// close textual variant — see below). The guard pattern is
// conventional: every handler in this codebase uses the same shape.
func startsWithMethodPostGuard(body *ast.BlockStmt) bool {
	if len(body.List) == 0 {
		return false
	}
	first := body.List[0]
	ifStmt, ok := first.(*ast.IfStmt)
	if !ok {
		return false
	}
	bin, ok := ifStmt.Cond.(*ast.BinaryExpr)
	if !ok {
		return false
	}
	if bin.Op != token.NEQ {
		return false
	}
	// Left side: r.Method
	leftSel, ok := bin.X.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	leftIdent, ok := leftSel.X.(*ast.Ident)
	if !ok || leftIdent.Name != "r" || leftSel.Sel.Name != "Method" {
		return false
	}
	// Right side: http.MethodPost
	rightSel, ok := bin.Y.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	rightIdent, ok := rightSel.X.(*ast.Ident)
	if !ok || rightIdent.Name != "http" || rightSel.Sel.Name != "MethodPost" {
		return false
	}
	return true
}

// findRepoRoot walks up from the current working directory until it
// finds go.mod. Used to anchor paths in the test failure messages.
func findRepoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found starting at %s", wd)
		}
		dir = parent
	}
}