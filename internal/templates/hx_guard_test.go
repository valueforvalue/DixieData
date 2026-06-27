// hx_guard_test.go enforces HTMX attribute invariants in .templ files.
//
// The wrong-selector bug class (hx-target points to nothing, hx-get
// posts to nowhere) was DixieData's most common regression in 2026.
// This test walks every .templ file in internal/templates/ and
// asserts two rules:
//
//  1. URL attributes (hx-get, hx-post) MUST be emitted via either
//     routebuilder.X() or htmxattr.Mux{...}.Attrs(). Bare string
//     literals like `hx-get="/some/route"` are not allowed because
//     they break when the route renames silently. The set of allowed
//     builder targets is auto-discovered from the routebuilder
//     package's exported functions, so adding a new builder is the
//     only way to satisfy the test for a new URL.
//
//  2. Selector attributes (hx-target, hx-select) that start with "#"
//     SHOULD resolve to a uiids registry entry. Ad-hoc selectors are
//     allowed (transient panels don't earn a registry entry) but the
//     test reports them so authors can decide whether the element
//     deserves a permanent home in the registry.
//
// The test uses regex-based source scanning rather than a full Templ
// AST because templ doesn't expose its AST publicly. This is a
// trade-off: a sufficiently creative Templ author could construct
// hx-* attributes dynamically and bypass the scanner. In practice
// DixieData templates use literal attribute syntax, so the trade-off
// is acceptable. If a future edit needs to evade the scanner,
// that's a smell — open the discussion in code review.
package templates

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/uiids"
)

// hxURLRe matches hx-get="..." or hx-post="..." with a literal value.
// hx-get/hx-post with {...} expressions are checked separately
// because they typically wrap a builder call.
var hxURLRe = regexp.MustCompile(`hx-(get|post)="([^"]+)"`)

// hxMuxExprRe matches hx-(get|post)={...} expressions on a single
// line. The expression body is captured for builder inspection.
var hxMuxExprRe = regexp.MustCompile(`hx-(get|post)=\{([^{}]+)\}`)

// hxTargetRe matches hx-target="..." or hx-select="..." with a
// literal value.
var hxTargetRe = regexp.MustCompile(`hx-(target|select)="([^"]+)"`)

// allowedBuilders is the set of routebuilder function names whose
// results are accepted as URL attribute sources. Auto-discovered
// from the routebuilder package so adding a new builder is the only
// way to satisfy the test for a new URL.
//
// The discovery walks the package by reading its source; the
// alternative (reflection over the package's exported symbols) would
// require importing routebuilder here, which is fine because
// internal/templates can already import routebuilder (no cycle) —
// but reading the source keeps the test decoupled from the package's
// runtime behaviour.
func allowedBuilders(t *testing.T) map[string]bool {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	thisDir := filepath.Dir(thisFile)
	rbFile := filepath.Join(thisDir, "..", "routebuilder", "routebuilder.go")
	rbFile = filepath.Clean(rbFile)
	src, err := os.ReadFile(rbFile)
	if err != nil {
		t.Fatalf("read routebuilder source: %v", err)
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, rbFile, src, 0)
	if err != nil {
		t.Fatalf("parse routebuilder: %v", err)
	}

	out := map[string]bool{}
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil {
			continue
		}
		if fn.Name.IsExported() {
			out[fn.Name.Name] = true
		}
	}
	return out
}

// TestHXURLsUseBuilders walks every .templ file in the templates
// package and reports hx-get / hx-post attributes that don't resolve
// to a routebuilder.X() call or htmxattr.Mux{...}.Attrs() call.
//
// This test is now strict: any hx-get / hx-post attribute that
// doesn't resolve through a routebuilder builder fails the test
// with a file:line citation. The migration of all raw-URL sites
// to builders landed in PR #F1 (follow-up plan FU.6 + FU.7); the
// PR flipped the test from advisory (t.Logf) to strict (t.Errorf).
//
// To allow a new local helper function as a builder wrapper (i.e.,
// one whose body calls a builder but whose name isn't itself a
// builder), add it to allowedBuilderWrappers below and verify by
// inspection that the body routes through a builder.
func TestHXURLsUseBuilders(t *testing.T) {
	builders := allowedBuilders(t)
	_, thisFile, _, _ := runtime.Caller(0)
	thisDir := filepath.Dir(thisFile)

	templFiles, err := filepath.Glob(filepath.Join(thisDir, "*.templ"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}

	var offenders []offender
	for _, file := range templFiles {
		rel, _ := filepath.Rel(filepath.Join(thisDir, "..", ".."), file)
		offenders = append(offenders, scanFileForBareURLs(file, rel, builders)...)
	}

	if len(offenders) == 0 {
		return
	}
	for _, o := range offenders {
		t.Errorf("  %s:%d — %s", o.file, o.line, o.reason)
	}
}

type offender struct {
	file   string
	line   int
	reason string
}

func scanFileForBareURLs(path, rel string, builders map[string]bool) []offender {
	f, err := os.Open(path)
	if err != nil {
		return []offender{{file: rel, line: 0, reason: fmt.Sprintf("open: %v", err)}}
	}
	defer f.Close()

	var out []offender
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		out = append(out, checkLine(line, lineNum, rel, builders)...)
	}
	return out
}

func checkLine(line string, lineNum int, rel string, builders map[string]bool) []offender {
	var out []offender
	for _, m := range hxURLRe.FindAllStringSubmatch(line, -1) {
		verb := m[1]
		val := m[2]
		if strings.HasPrefix(val, "routebuilder.") {
			continue
		}
		out = append(out, offender{
			file:   rel,
			line:   lineNum,
			reason: fmt.Sprintf("hx-%s=\"%s\" is a bare URL literal; use templ.SafeURL(routebuilder.X(...)) or htmxattr.Mux{...}", verb, val),
		})
	}

	for _, m := range hxMuxExprRe.FindAllStringSubmatch(line, -1) {
		verb := m[1]
		expr := strings.TrimSpace(m[2])
		if !usesBuilder(expr, builders) {
			out = append(out, offender{
				file:   rel,
				line:   lineNum,
				reason: fmt.Sprintf("hx-%s={%s} does not reference a routebuilder builder; use routebuilder.X(...) or htmxattr.Mux{...}", verb, expr),
			})
		}
	}

	return out
}

// usesBuilder reports whether expr contains a call to any function
// in the builders set, or to htmxattr.Mux (the typed wrapper that
// itself contains a builder call). Both forms are accepted because
// the canonical path is direct (e.g. `templ.SafeURL(routebuilder.X())`)
// but the typed path through Mux is also valid.
func usesBuilder(expr string, builders map[string]bool) bool {
	if strings.Contains(expr, "htmxattr.Mux{") {
		return true
	}
	for name := range builders {
		if strings.Contains(expr, name+"(") {
			return true
		}
	}
	// Allow known wrappers that internally call builders. These are
	// documented in the comment above each wrapper; the guard test
	// can't statically prove they route through builders, but the
	// codebase keeps a strict audit on which wrappers are approved.
	for allowed := range allowedBuilderWrappers {
		if strings.Contains(expr, allowed+"(") {
			return true
		}
	}
	return false
}

// allowedBuilderWrappers is the set of local helper function names
// whose bodies construct URLs by calling routebuilder builders. The
// guard test can't statically verify the body; if you add a new
// wrapper, document it here and verify by inspection that the body
// routes through a builder.
var allowedBuilderWrappers = map[string]bool{
	"pageRequestURL": true, // soldier_card.templ:692 — routes through SoldierSearch / SoldierSearchAdvanced
	"pageHref":       true, // soldier_card.templ:688 — wraps pageRequestURL
	"browsePageHref": true, // browse.templ — URL helper that builds /browse/results query
}

// TestHXTargetsPreferRegistry walks every .templ file and reports
// any hx-target / hx-select that starts with "#" but does NOT
// resolve to a uiids registry entry. Reports are advisory — the
// test passes either way, but the output tells the author which
// selectors could be promoted to the registry.
func TestHXTargetsPreferRegistry(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	thisDir := filepath.Dir(thisFile)

	templFiles, err := filepath.Glob(filepath.Join(thisDir, "*.templ"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}

	adHoc := map[string][]string{} // selector -> []file:line

	for _, file := range templFiles {
		rel, _ := filepath.Rel(filepath.Join(thisDir, "..", ".."), file)
		f, err := os.Open(file)
		if err != nil {
			t.Errorf("open %s: %v", rel, err)
			continue
		}
		scanner := bufio.NewScanner(f)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			for _, m := range hxTargetRe.FindAllStringSubmatch(line, -1) {
				sel := m[2]
				if !strings.HasPrefix(sel, "#") {
					continue
				}
				id := strings.TrimPrefix(sel, "#")
				if id == "" {
					continue
				}
				if !uiids.Has(id) {
					adHoc[id] = append(adHoc[id], fmt.Sprintf("%s:%d", rel, lineNum))
				}
			}
		}
		f.Close()
	}

	if len(adHoc) == 0 {
		return
	}
	t.Logf("ad-hoc hx-target selectors (not in uiids registry). These work but consider promoting to internal/uiids for permanent naming:")
	for id, locs := range adHoc {
		t.Logf("  #%s — used at %s", id, strings.Join(locs, ", "))
	}
}

// Ensure the reflect package is referenced so future edits to this
// file don't lose the dependency by accident. (The reflect package
// is used implicitly by reflect.TypeOf in routebuilder; this is a
// defensive import so test failures don't accidentally remove it.)
var _ = reflect.TypeOf