// route_wildcard_test.go catches the bug class where a chi
// wildcard route (e.g. /soldiers/*) shadows a more-specific route
// registered before it (e.g. /soldiers/search). The symptom is
// that a request to /soldiers/search returns a 405 (because the
// wildcard handler rejects the method) or wrong content (because
// the wildcard handler does the wrong thing for the path).
//
// Chi matches routes in registration order, so specific paths must
// be registered before wildcards. This test enforces that order by
// firing one GET per (specific, wildcard) pair and asserting the
// specific path returns 200 (or a non-405 response that indicates
// the right handler was reached).
//
// The pairs below are hand-curated from the current routes.go.
// Add new entries when adding new specific/wildcard siblings.
package appshell

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// wildcardShadowPairs enumerates (specific path, shadowing
// wildcard pattern) pairs. For each pair, the test fires a GET at
// the specific path and asserts the response is not 405 — a 405
// means the wildcard handler was reached and rejected the method,
// which is the bug class we're guarding against.
var wildcardShadowPairs = []struct {
	specific string
	wildcard string
}{
	// /jobs/* must not shadow /jobs/active
	{"/jobs/active", "/jobs/*"},
	// /soldiers/* must not shadow /soldiers/search* / soldiers/display/* / soldiers/new
	{"/soldiers/search", "/soldiers/*"},
	{"/soldiers/search/recent", "/soldiers/*"},
	{"/soldiers/search/advanced", "/soldiers/*"},
	{"/soldiers/display/DXD-00001", "/soldiers/*"},
	{"/soldiers/new", "/soldiers/*"},
	// /review-queue/* must not shadow /review-queue/compare/*
	{"/review-queue/compare/42", "/review-queue/*"},
	// /research-collections/* must not shadow any sibling
	{"/research-collections/123", "/research-collections/*"},
	// /soldiers/{id}/tags[/...] (issue #183) must not be shadowed
	// by /soldiers/*; /tags/{id} must not be shadowed by /tags/*.
	{"/soldiers/42/tags", "/soldiers/*"},
	{"/soldiers/42/tags/7", "/soldiers/*"},
	{"/tags", "/tags/*"},
	{"/tags/7", "/tags/*"},
	// Issue #182: /share/queue/modal is GET only and must not be
	// shadowed by the /share/* branch. The /share/queue/preview
	// + /share/queue/clear POSTs aren't asserted here because
	// route_wildcard_test does GETs (chi returns 405 for
	// GET-on-POST endpoints; that's not a shadow indicator).
	{"/share/queue/modal", "/share"},
}

// TestWildcardRoutesDoNotShadowSpecific guards against route-order
// regressions. Chi matches in registration order; if a wildcard is
// registered before its more-specific sibling, the wildcard wins
// and the specific handler never runs.
func TestWildcardRoutesDoNotShadowSpecific(t *testing.T) {
	app := NewApp()
	app.setupRoutes()

	for _, pair := range wildcardShadowPairs {
		pair := pair
		t.Run("GET "+pair.specific, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, pair.specific, nil)
			rec := httptest.NewRecorder()
			app.ServeHTTP(rec, req)

			// 200/202/3xx/4xx-other-than-405 all indicate the
			// request reached the right handler. 405 means the
			// wildcard handler rejected the method — the bug
			// class we're guarding against.
			if rec.Code == http.StatusMethodNotAllowed {
				t.Errorf("GET %s returned 405; this means %s (registered later?) is shadowing the specific handler",
					pair.specific, pair.wildcard)
			}
		})
	}
}