// redirect_headers_test.go enforces that every 303 See Other
// response in appshell also writes X-DixieData-Redirect, so
// the Option C dispatcher (dispatchDixieDataForm) can navigate
// the browser regardless of whether the handler migrated to the
// new 200 + X-DixieData-Redirect contract yet.
//
// Background — the bug class:
//
//   DixieData share-page buttons submit via the custom dispatcher
//   in frontend/app.js (formerly request(), now dispatchDixieDataForm).
//   The dispatcher reads EITHER X-DixieData-Redirect (the new
//   contract) OR follows a 303 + Location response (legacy).
//   Handlers should converge on X-DixieData-Redirect so the
//   dispatcher has a single contract to read; legacy 303 writers
//   only work because of the dispatcher's dual-contract support,
//   which is load-bearing for the templ retag window (Commits 6–14).
//
//   Historically every "fix" commit (3612dab, 11f1c01, a6f7fa2, etc.)
//   added HX-Redirect to a 303-returning handler. HX-Redirect is
//   DEAD CODE — no code path in this codebase reads it (htmx is
//   not the dispatcher; the custom JS reads X-DixieData-Redirect).
//   Option C replaces the legacy pattern: handlers emit
//   X-DixieData-Redirect, dispatcher reads it, navigator runs.
//
//   This test inverts the legacy assertion. After all Option C
//   handler migrations (Commits 3–5) land, the new assertion is
//   satisfied; the test goes from red to green as handlers
//   migrate. Until then, the offenders list is the inventory of
//   remaining migration work.
//
// The test follows the same source-scan pattern as
// internal/templates/hx_guard_test.go: regex/AST scanning of
// source rather than runtime instrumentation, because we want to
// catch the bug at build time before it ships. The cost is that
// sufficiently exotic reflection-built handlers could evade the
// scanner; in practice DixieData handlers are written by hand and
// the trade-off is acceptable.
//
// Allow-list:
//
//   Functions whose entire purpose is a server-initiated redirect
//   (recovery/setup middleware, legacy URL renames, GET-only
//   display-ID lookup) do NOT need X-DixieData-Redirect because
//   no DixieData dispatcher button reaches them. They are listed
//   in exemptFunctions below with a one-line reason. Adding a
//   function to the allow-list requires a comment explaining why
//   no DixieData button hits it; this forces the author to think
//   about the bug class instead of silently bypassing the guard.
package appshell

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// exemptFunctions lists functions in appshell that write
// StatusSeeOther but legitimately do not need X-DixieData-Redirect
// because no DixieData dispatcher button reaches them. Each entry
// must carry a one-line reason so the next reader understands the
// exemption.
var exemptFunctions = map[string]string{
	"handleRecovery":             "GET handler / early-out — no DixieData button reaches it; recovery is server-initiated middleware",
	"handleInitialSetup":         "setup flow — POST from a plain <form method=\"post\"> without DixieData; X-DixieData-Redirect not needed",
	"handleLegacyExportRedirect": "GET-only URL rename (/export -> /share); no body, no DixieData",
	"handleSoldierByDisplayID":   "GET-only display-ID lookup; the two redirects are URL canonicalisation, no form submit",
	"cancelJob":                  "POST /jobs/{id}/cancel is a plain <form method=\"post\"> (jobs.templ:153); native browser follows Location",
	"openJobArtifact":            "POST /jobs/{id}/open is a plain <form method=\"post\"> (jobs.templ); it sets a toast header then 303's back to /jobs/{id}; native browser follows Location",
	"confirmJob":                "POST /jobs/{id}/confirm is a plain <form method=\"post\"> (jobs.templ confirmation card); 303 + Location back to /jobs/{id}; native browser follows Location",
}

// filesExempt lists files where every function may write
// StatusSeeOther without X-DixieData-Redirect. These are pure
// middleware/setup helpers — no DixieData button ever hits a route
// in them.
var filesExempt = map[string]bool{
	"app_recovery.go": true, // recovery middleware; redirect-only
	"lifecycle.go":    true, // startup/setup middleware; redirect-only
}

// TestPostThenNavigateUsesDixieRedirect is the regression net for
// the "export options status pages not landing" bug class, in its
// Option C form. It walks every .go file in internal/appshell/,
// parses each top-level function declaration, and asserts:
//
//   1. If the function body contains a WriteHeader(StatusSeeOther)
//      or http.Redirect(...StatusSeeOther) call, AND
//   2. The function is not in exemptFunctions, AND
//   3. The file is not fully exempt under filesExempt,
//
//   THEN the function body must also contain a Set("X-DixieData-Redirect"...)
//   (or equivalent with backticks or double-quoted strings).
//
// At this commit, the assertion is RED: ~22 handlers still emit
// 303 + Location + HX-Redirect without X-DixieData-Redirect. The
// offender list is the inventory for Commits 3–5. As those commits
// migrate handlers to the new contract, this test turns green.
func TestPostThenNavigateUsesDixieRedirect(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	thisDir := filepath.Dir(thisFile)
	entries, err := os.ReadDir(thisDir)
	if err != nil {
		t.Fatalf("read appshell dir: %v", err)
	}

	var offenders []string
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if filesExempt[name] {
			continue
		}
		path := filepath.Join(thisDir, name)
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil {
				continue
			}
			if fn.Body == nil {
				continue
			}
			bodySrc := string(src[fset.Position(fn.Body.Lbrace).Offset:fset.Position(fn.Body.Rbrace).Offset])
			if !writesStatusSeeOther(bodySrc) {
				continue
			}
			fnName := fn.Name.Name
			if _, exempt := exemptFunctions[fnName]; exempt {
				continue
			}
			if setsDixieRedirect(bodySrc) {
				continue
			}
			pos := fset.Position(fn.Pos())
			offenders = append(offenders, formatOffender(pos, fnName))
		}
	}

	if len(offenders) > 0 {
		t.Fatalf("%d handler(s) write StatusSeeOther without X-DixieData-Redirect; the Option C dispatcher needs X-DixieData-Redirect on every post-then-navigate response. Migrate the handler to use writeExportRedirect() (or set X-DixieData-Redirect manually before WriteHeader(StatusSeeOther)), or add the function to exemptFunctions with a one-line reason:\n  %s",
			len(offenders), strings.Join(offenders, "\n  "))
	}
}

// writesStatusSeeOther returns true if the function body contains
// either a WriteHeader(http.StatusSeeOther) call or an
// http.Redirect(...StatusSeeOther) call.
func writesStatusSeeOther(body string) bool {
	return strings.Contains(body, "StatusSeeOther")
}

// setsDixieRedirect returns true if the function body sets the
// X-DixieData-Redirect response header. Matches both quoted-string
// and backtick raw-string forms:
//   w.Header().Set("X-DixieData-Redirect", target)
//   w.Header().Set(`X-DixieData-Redirect`, target)
func setsDixieRedirect(body string) bool {
	if !strings.Contains(body, "X-DixieData-Redirect") {
		return false
	}
	const window = 60
	for i := 0; i < len(body); i++ {
		j := strings.Index(body[i:], "X-DixieData-Redirect")
		if j < 0 {
			return false
		}
		idx := i + j
		lo := idx - window
		if lo < 0 {
			lo = 0
		}
		hi := idx + window
		if hi > len(body) {
			hi = len(body)
		}
		if strings.Contains(body[lo:hi], "Set") {
			return true
		}
		i = idx
	}
	return false
}

func formatOffender(pos token.Position, fnName string) string {
	return pos.String() + " (" + fnName + ")"
}

// writesHXRedirect returns true if the function body contains a
// Set call that writes the HX-Redirect header (either quoted or
// backtick form). It does NOT match reads via Header().Get (e.g.
// a debug report that lists headers for triage), which would be
// legitimate even after Option C removes the writes.
func writesHXRedirect(body string) bool {
	const window = 60
	for i := 0; i < len(body); i++ {
		j := strings.Index(body[i:], "HX-Redirect")
		if j < 0 {
			return false
		}
		idx := i + j
		lo := idx - window
		if lo < 0 {
			lo = 0
		}
		hi := idx + window
		if hi > len(body) {
			hi = len(body)
		}
		snippet := body[lo:hi]
		// Match Set(...HX-Redirect...) with either quote style.
		if strings.Contains(snippet, ".Set(") && (strings.Contains(snippet, "\"HX-Redirect\"") || strings.Contains(snippet, "`HX-Redirect`")) {
			return true
		}
		i = idx
	}
	return false
}
// TestNoDeadHXRedirectWrites is the regression net for the
// Option C housekeeping goal: HX-Redirect is dead code (the
// custom dispatcher reads X-DixieData-Redirect, never HX-Redirect).
// After the templ retag and handler migration land, no handler in
// appshell should write HX-Redirect at all. The static archive
// carve-out legitimately writes HX-Redirect in some flows; see
// filesExempt and exemptFunctions for the carve-out list.
//
// This test enforces the post-Option-C invariant: every handler
// that needs the dispatcher reads X-DixieData-Redirect; nobody
// reaches for the dead header.
func TestNoDeadHXRedirectWrites(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	thisDir := filepath.Dir(thisFile)
	entries, err := os.ReadDir(thisDir)
	if err != nil {
		t.Fatalf("read appshell dir: %v", err)
	}

	var offenders []string
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if filesExempt[name] {
			continue
		}
		path := filepath.Join(thisDir, name)
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil {
				continue
			}
			if fn.Body == nil {
				continue
			}
			bodySrc := string(src[fset.Position(fn.Body.Lbrace).Offset:fset.Position(fn.Body.Rbrace).Offset])
			if !writesHXRedirect(bodySrc) {
				continue
			}
			if _, exempt := exemptFunctions[fn.Name.Name]; exempt {
				continue
			}
			pos := fset.Position(fn.Pos())
			offenders = append(offenders, formatOffender(pos, fn.Name.Name))
		}
	}

	if len(offenders) > 0 {
		t.Fatalf("%d handler(s) still write HX-Redirect, which is dead code in this codebase (dispatchDixieDataForm reads X-DixieData-Redirect, never HX-Redirect). Either migrate to writeExportRedirect(w, target) or add the function to exemptFunctions with a one-line reason:\n  %s",
			len(offenders), strings.Join(offenders, "\n  "))
	}
}
