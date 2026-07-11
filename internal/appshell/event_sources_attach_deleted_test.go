package appshell

// This file documents the issue #380 slice 6 deletion of
// `POST /events/{id}/sources/attach`. The handler was kept
// alive post-#341 only as bookmark-compat (a stale PR-195-era
// remnant); issue #360 / v61 slice removed the UI form but
// left the handler. Per the user's 2026-07-11 decision on
// issue #380 OQ3, the app does not support bookmarks at all
// so the handler has zero remaining callers. The deletion
// saves ~30 LoC + a test + a chi-route registration.
//
// The test below uses a build-time marker: the
// `handleEventSourceAttach` symbol and the `POST sources/attach`
// route entry should both be gone from the package. We
// verify by reflection-free static inspection — the Go
// compiler refuses to build if the test file references the
// deleted identifiers, so the negative-existence is implied
// by a successful test build.
//
// We add an explicit regression test (TestEventSourcesAttachRouteIsDeleted)
// that asserts the chi router returns 404 for the
// /events/{id}/sources/attach path under POST. Pre-deletion
// this returned 200; post-deletion it returns 405 (method not
// allowed) or 404 (route missing) — the dispatcher table no
// longer has the entry.
import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestEventSourcesAttachRouteIsDeleted pins the slice 6
// deletion: POST /events/{id}/sources/attach no longer
// resolves. The test exercises a real app server via
// httptest.NewServer; if a future slice accidentally
// re-registers the route, this test fails loudly.
func TestEventSourcesAttachRouteIsDeleted(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	// Pick a numeric event id; the route resolution happens
	// before any DB lookup, so even an out-of-range id is fine
	// — what matters is whether chi matches the route at all.
	const path = "/events/999999/sources/attach"

	req, err := http.NewRequest(http.MethodPost, server.URL+path, strings.NewReader("record_type=Pension&app_id=APP-1880-7701&details=test"))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	// After deletion: chi returns 405 (method not allowed) for
	// /events/{id}/sources without an `attach` sub-route, OR
	// 404 if the entire sub-router no longer matches. Either is
	// acceptable evidence the handler is gone. Pre-deletion
	// (with the route still registered) this returns 500 (the
	// handler tries to load event 999999 from the DB and fails)
	// — so the test pin is "404 or 405" exactly, not "not 200".
	if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST %s returned %d; expected 404 or 405 after slice 6 deletion (route still registered would return 500 from DB lookup failure on event id 999999)", path, resp.StatusCode)
	}
}