// route_integration_test.go is the runtime half of the route/handler
// method safety net. routes_method_guard_test.go is the AST-level
// check (catches the structural mismatch at compile-time speed).
// This file is the runtime-level check: spin up a real App, fire a
// real HTTP request against each known POST-only route, and assert
// that GET requests are rejected with 405 Method Not Allowed.
//
// The guard test caught 16 mis-registered routes after PR #1 of the
// stabilization sprint. This integration test exists so the same
// regression class is caught from both directions going forward:
//
//   - The AST guard catches "handler requires POST but route is GET".
//     It's fast (no HTTP), but it depends on the conventional guard
//     shape `if r.Method != http.MethodPost { ... return }`.
//
//   - This integration test catches any runtime path that ends in a
//     405, regardless of guard shape. It exercises the live chi
//     router and the live handler method check.
//
// Together, the two tests are belt-and-braces against the chi
// migration's biggest hazard.
package appshell

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// postOnlyPaths is the list of routes whose handlers reject
// anything except POST. Hand-curated from a recent run of
// TestRouteMethodMatchesHandler so this test stays decoupled from
// the AST walk's internals — when the handler list changes, only
// the AST guard needs updating, not this list.
//
// To regenerate: run `go test ./internal/appshell -run TestRouteMethodMatchesHandler -v`
// and copy the path lines from the failure output into the list
// below.
var postOnlyPaths = []string{
	"/export/json",
	"/export/csv",
	"/export/ical",
	"/export/static-archive",
	"/export/database-pdf",
	"/export/backup",
	"/export/shared-archive",
	"/export/bug-report",
	"/export/feedback-log",
	"/insights/report/pdf",
	"/merge-review/42/keep-local",
	"/merge-review/42/keep-shared",
	"/merge-review/42/keep-both",
	"/integrations/google/connect",
	"/integrations/google/disconnect",
	"/integrations/google/backup",
	"/integrations/google/sheets/export",
	"/images/screenshot",
	"/open-link",
	// Share Queue (issue #182).
	"/share/queue/preview",
	"/share/queue/clear",
}

// TestPostOnlyHandlersRejectGET is the integration assertion that
// backs the AST-level guard. For every route in postOnlyPaths it
// issues a real GET via the live chi router and verifies the
// handler returns 405. This catches:
//
//   - A mis-registration where the route was changed back to r.Get
//     (the original bug from PR #1).
//   - A handler whose guard shape changed in a way the AST walker
//     doesn't recognize (e.g. switched to a switch statement).
//
// The test uses NewApp() + setupRoutes() so the live chi router is
// exercised end-to-end. The handler's method check fires before
// any database access, so the test works without fixtures.
func TestPostOnlyHandlersRejectGET(t *testing.T) {
	app := NewApp()
	app.setupRoutes()

	for _, path := range postOnlyPaths {
		path := path
		t.Run("GET "+path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			app.ServeHTTP(rec, req)

			if rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("GET %s returned %d; POST-only handler should reject GET with 405",
					path, rec.Code)
			}
		})
	}
}

// TestPostOnlyHandlersAdvertiseAllowHeader ensures the chi router
// advertises the supported method via the Allow header on 405
// responses. RFC 7231 §6.5.5 requires this header so clients
// (including htmx) know which methods are valid. The header is set
// by chi's automatic method-not-allowed response, not by the handler.
//
// This catches a misconfiguration where the chi router is configured
// to suppress the Allow header (e.g. custom error handler that
// drops it) or where the route is registered under a method the
// handler can't accept.
func TestPostOnlyHandlersAdvertiseAllowHeader(t *testing.T) {
	app := NewApp()
	app.setupRoutes()

	for _, path := range postOnlyPaths {
		path := path
		t.Run("GET "+path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			app.ServeHTTP(rec, req)

			if rec.Code != http.StatusMethodNotAllowed {
				return // not the bug we're testing
			}
			allow := rec.Header().Get("Allow")
			if !strings.Contains(allow, "POST") {
				t.Errorf("GET %s returned 405 but Allow header is %q (expected to contain POST)",
					path, allow)
			}
		})
	}
}