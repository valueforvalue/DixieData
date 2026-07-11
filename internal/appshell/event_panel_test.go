package appshell

// Tests for the path-suffix dispatcher collapse (#343 finding #3).
// Each test pins a single dispatch contract; together they
// pin the full eventPanels registry + matchSubPath parser.

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestMatchSubPathLiteralMatch pins the literal sub-path branch
// of matchSubPath: empty matches empty; "attach" matches
// "attach"; trailing/leading slashes are tolerated.
func TestMatchSubPathLiteralMatch(t *testing.T) {
	cases := []struct {
		suffix, subPath string
		wantSubID       int64
		want            bool
	}{
		{"", "", -1, true},
		{"attach", "attach", -1, true},
		{"attach", "/attach", -1, true},
		{"attach", "attach/", -1, true},
		{"attach", "/attach/", -1, true},
		{"", "attach", -1, false},
		{"detach", "attach", -1, false},
		{"attach/extra", "attach", -1, false},
	}
	for _, tc := range cases {
		t.Run(tc.suffix+"|"+tc.subPath, func(t *testing.T) {
			var subID int64 = -99
			got := matchSubPath(tc.suffix, tc.subPath, &subID)
			if got != tc.want {
				t.Errorf("matchSubPath(%q, %q) = %v, want %v", tc.suffix, tc.subPath, got, tc.want)
			}
			if got && subID != tc.wantSubID {
				t.Errorf("subID = %d, want %d", subID, tc.wantSubID)
			}
		})
	}
}

// TestMatchSubPathParametricMatch pins the {id} placeholder
// branch: subPath "{id}/detach" matches suffix "7/detach" with
// subID=7; non-numeric segments fail; length mismatches fail.
func TestMatchSubPathParametricMatch(t *testing.T) {
	cases := []struct {
		suffix, subPath string
		wantSubID       int64
		want            bool
	}{
		{"7/detach", "{id}/detach", 7, true},
		{"42/detach", "{id}/detach", 42, true},
		{"abc/detach", "{id}/detach", -1, false},
		{"7/attach", "{id}/detach", -1, false},
		{"7", "{id}/detach", -1, false},
		{"7/detach/extra", "{id}/detach", -1, false},
		{"", "{id}/detach", -1, false},
	}
	for _, tc := range cases {
		t.Run(tc.suffix+"|"+tc.subPath, func(t *testing.T) {
			var subID int64 = -99
			got := matchSubPath(tc.suffix, tc.subPath, &subID)
			if got != tc.want {
				t.Errorf("matchSubPath(%q, %q) = %v, want %v", tc.suffix, tc.subPath, got, tc.want)
			}
			if got && subID != tc.wantSubID {
				t.Errorf("subID = %d, want %d", subID, tc.wantSubID)
			}
		})
	}
}

// TestEventPanelsRegistryParityWithRoutes asserts the registry
// contains every panel the chi router registers. A future
// panel that gets added to routes.go without a matching
// registry entry would route through the dispatcher as
// "not found" — this test catches that drift.
func TestEventPanelsRegistryParityWithRoutes(t *testing.T) {
	wantPanels := map[string]bool{
		"research-log": true,
		"sources":      true,
		"tags":         true,
		"images":       true,
	}
	for _, panel := range eventPanels {
		if !wantPanels[panel.name] {
			t.Errorf("registry contains unknown panel %q (add to wantPanels or remove from eventPanels)", panel.name)
		}
		delete(wantPanels, panel.name)
	}
	for name := range wantPanels {
		t.Errorf("registry missing panel %q (chi route shim would 404)", name)
	}
}

// TestEventPanelRouteUnknownPanelReturns404 pins the dispatcher's
// behavior on a suffix that doesn't match any panel name.
// /events/42/nonexistent → 404, not 405.
func TestEventPanelRouteUnknownPanelReturns404(t *testing.T) {
	app := &App{}
	req := httptest.NewRequest(http.MethodGet, "/events/42/nonexistent", nil)
	rr := httptest.NewRecorder()
	app.handleEventPanelRoute(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

// TestEventPanelRouteKnownPanelUnknownMethodReturns405 pins the
// 405-vs-404 split: a known panel with a method the panel
// doesn't expose (e.g. PATCH /events/42/sources) is 405, not
// 404. Clients can probe the allowed-method set.
func TestEventPanelRouteKnownPanelUnknownMethodReturns405(t *testing.T) {
	app := &App{}
	req := httptest.NewRequest(http.MethodPatch, "/events/42/sources", nil)
	rr := httptest.NewRecorder()
	app.handleEventPanelRoute(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rr.Code)
	}
}

// TestEventPanelRouteKnownPanelUnknownSubPathReturns404 pins
// that a known panel + known method + unknown sub-path is
// 404, not 405. /events/42/sources/foobar on POST is 404 —
// the sources panel accepts POST only on "attach" and
// "{id}/detach".
func TestEventPanelRouteKnownPanelUnknownSubPathReturns404(t *testing.T) {
	app := &App{}
	req := httptest.NewRequest(http.MethodPost, "/events/42/sources/foobar", nil)
	rr := httptest.NewRecorder()
	app.handleEventPanelRoute(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

// TestEventPanelRouteMissingEventIDReturns400 pins the
// dispatcher bails on /events/{not-a-number}/... with 400.
func TestEventPanelRouteMissingEventIDReturns400(t *testing.T) {
	app := &App{}
	req := httptest.NewRequest(http.MethodGet, "/events/notanumber/sources", nil)
	rr := httptest.NewRecorder()
	app.handleEventPanelRoute(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// TestEventPanelRouteMissingPanelReturns404 pins /events/42
// with no panel segment returns 404 (the dispatcher needs at
// least one segment after the eventID).
func TestEventPanelRouteMissingPanelReturns404(t *testing.T) {
	app := &App{}
	req := httptest.NewRequest(http.MethodGet, "/events/42", nil)
	rr := httptest.NewRecorder()
	app.handleEventPanelRoute(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

// TestEventPanelRoutePrefixCollisionIsRejected pins that
// /events/42/sourcesXxx doesn't accidentally match the
// "sources" panel. The prefix check requires either an exact
// match on the suffix or a following "/" — "sourcesXxx"
// would otherwise be parsed as panel="sources", rest="Xxx".
func TestEventPanelRoutePrefixCollisionIsRejected(t *testing.T) {
	app := &App{}
	req := httptest.NewRequest(http.MethodGet, "/events/42/sourcesXxx", nil)
	rr := httptest.NewRecorder()
	app.handleEventPanelRoute(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (sourcesXxx must not match the 'sources' panel)", rr.Code)
	}
}