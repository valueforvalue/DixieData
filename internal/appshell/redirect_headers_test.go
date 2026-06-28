// redirect_headers_test.go enforces that every 303 See Other
// response in appshell writes HX-Redirect alongside Location, so
// htmx 2.x with hx-swap="none" does not silently swallow the
// redirect.
//
// Background — the bug class:
//
//   DixieData share-page buttons (and others) submit via htmx with
//   hx-swap="none" so the originating page stays put while a
//   background worker runs. A 303 + Location alone is NOT enough
//   for htmx 2.x to navigate the browser in that configuration;
//   htmx 2.x suppresses both the swap and the redirect handling.
//   The handler must also set the HX-Redirect response header.
//
//   Commit 70878ac shipped per-kind stats on /jobs/{id} and the
//   buttons were re-pointed at it. The original six share-page
//   exports were fixed by 3612dab, but the Google handlers
//   (handleGoogleBackup, handleGoogleSheetsExport) and the
//   printable-PDF modal flow were missed. The same bug shipped
//   again in the bulk-review, orphan-cleanup, duplicate-audit,
//   and image-import handlers. This test walks every Go function
//   in internal/appshell/, finds every StatusSeeOther write, and
//   asserts the enclosing function also sets HX-Redirect.
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
//   display-ID lookup) do NOT need HX-Redirect because no htmx
//   button reaches them. They are listed in exemptFunctions below
//   with a one-line reason. Adding a function to the allow-list
//   requires a comment explaining why no htmx button hits it; this
//   forces the author to think about the bug class instead of
//   silently bypassing the guard.
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
// StatusSeeOther but legitimately do not need HX-Redirect because
// no htmx button reaches them. Each entry must carry a one-line
// reason so the next reader understands the exemption.
var exemptFunctions = map[string]string{
	"handleRecovery":                 "GET handler / early-out — no htmx button reaches it; recovery is server-initiated middleware",
	"handleInitialSetup":             "setup flow — POST from a plain <form method=\"post\"> without htmx; HX-Redirect not needed",
	"handleLegacyExportRedirect":     "GET-only URL rename (/export -> /share); no body, no htmx",
	"handleSoldierByDisplayID":       "GET-only display-ID lookup; the two redirects are URL canonicalisation, no form submit",
	"cancelJob":                      "POST /jobs/{id}/cancel is a plain <form method=\"post\"> (jobs.templ:153), no hx-swap=\"none\"; native browser follows Location",
}

// filesExempt lists files where every function may write
// StatusSeeOther without HX-Redirect. These are pure
// middleware/setup helpers — no htmx button ever hits a route in
// them.
var filesExempt = map[string]bool{
	"app_recovery.go":   true, // recovery middleware; redirect-only
	"lifecycle.go":      true, // startup/setup middleware; redirect-only
	"calendar_handlers.go": false, // mixed: handleInitialSetup redirects exempt; calendar view is GET
	"app_feedback.go":   false, // mixed: handleSoldierByDisplayID exempt; handleFeedbackSubmit returns 200
}

// TestAll303sWriteHXRedirect is the regression net for the
// "export options status pages not landing" bug class. It walks
// every .go file in internal/appshell/, parses each top-level
// function declaration, and asserts:
//
//   1. If the function body contains a WriteHeader(StatusSeeOther)
//      or http.Redirect(...StatusSeeOther) call, AND
//   2. The function is not in exemptFunctions, AND
//   3. The file is not fully exempt under filesExempt,
//
//   THEN the function body must also contain a Set("HX-Redirect"...)
//   (or equivalent: w.Header().Set(`HX-Redirect`, ...) with
//   backticks or double-quoted strings).
//
// The failing output cites file:line:function so the next debugger
// can jump to the offender.
func TestAll303sWriteHXRedirect(t *testing.T) {
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
		// Whole-file exemption covers files where every StatusSeeOther
		// write is server-initiated middleware with no htmx caller.
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
			name := fn.Name.Name
			if _, exempt := exemptFunctions[name]; exempt {
				continue
			}
			if setsHXRedirect(bodySrc) {
				continue
			}
			pos := fset.Position(fn.Pos())
			offenders = append(offenders, formatOffender(pos, name))
		}
	}

	if len(offenders) > 0 {
		t.Fatalf("%d handler(s) write StatusSeeOther without HX-Redirect; htmx hx-swap=\"none\" will silently swallow the redirect and strand the user on the originating page. Add `w.Header().Set(\"HX-Redirect\", target)` alongside the Location header, or add the function to exemptFunctions with a one-line reason:\n  %s",
			len(offenders), strings.Join(offenders, "\n  "))
	}
}

// writesStatusSeeOther returns true if the function body contains
// either a WriteHeader(http.StatusSeeOther) call or an
// http.Redirect(...StatusSeeOther) call. The scanner is
// deliberately permissive: any substring match on the canonical
// token is enough. False positives would require a handler to
// mention StatusSeeOther in a comment or string literal without
// actually writing it — accepted as the price of the simple
// source-scan approach. If a future edit needs to evade the
// scanner, that's a smell; discuss in code review.
func writesStatusSeeOther(body string) bool {
	return strings.Contains(body, "StatusSeeOther")
}

// setsHXRedirect returns true if the function body sets the
// HX-Redirect response header. Matches both quoted-string and
// backtick raw-string forms:
//   w.Header().Set("HX-Redirect", target)
//   w.Header().Set(`HX-Redirect`, target)
// The match is on the substring "HX-Redirect" within ~40 chars
// of a Set call. This is permissive enough to accept both quoted
// and backtick forms without false negatives.
func setsHXRedirect(body string) bool {
	if !strings.Contains(body, "HX-Redirect") {
		return false
	}
	const window = 40
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