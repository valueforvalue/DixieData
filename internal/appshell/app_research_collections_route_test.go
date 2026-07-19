package appshell

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/testtemp"
)

// addRouteSeedCounter avoids displayID collisions across tests in this
// file when the global soldier counter is shared. Atomic add is cheap
// and avoids needing a sync.Mutex around the counter.
var addRouteSeedCounter int64

func seedAddRouteSoldier(t *testing.T, app *App, firstName, lastName string) struct{ ID int64 } {
	t.Helper()
	n := atomic.AddInt64(&addRouteSeedCounter, 1)
	displayID := fmt.Sprintf("DXD-ADD-%d-%s-%s", n, firstName, lastName)
	res, err := app.database.Conn().Exec(
		`INSERT INTO soldiers (display_id, first_name, last_name) VALUES (?, ?, ?)`,
		displayID, firstName, lastName)
	if err != nil {
		t.Fatalf("insert soldier %q: %v", displayID, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}
	return struct{ ID int64 }{ID: id}
}

func seedAddRouteCollection(t *testing.T, app *App, name string) struct{ ID int64 } {
	t.Helper()
	res, err := app.database.Conn().Exec(
		`INSERT INTO research_collections (name, description, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)`,
		name, "")
	if err != nil {
		t.Fatalf("insert collection %q: %v", name, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}
	return struct{ ID int64 }{ID: id}
}

// Regression net for issue #451 (or whichever issue spawns this):
// POST /research-collections must be wired (case http.MethodPost in
// handleResearchCollections). The form in research_collections.templ
// posts here on submit; without r.Post(...) chi returned 405 and users
// saw "Could not load research collections" when the page re-validated.
func TestHandleResearchCollections_POSTCreatesCollection(t *testing.T) {
	dataDir := testtemp.New(t).Path()
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	app := NewApp()
	app.dataDir = dataDir
	app.database = database
	if err := app.reloadServices(); err != nil {
		t.Fatalf("reloadServices: %v", err)
	}
	configureTestIdentity(t, app)
	app.setupRoutes()

	body := "name=Test+Collection&description=Cue+the+regression"
	req := httptest.NewRequest(http.MethodPost, "/research-collections", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code == http.StatusMethodNotAllowed {
		t.Fatalf("POST /research-collections returned 405; r.Post(...) not registered. body=%q", rec.Body.String())
	}
	if rec.Code < 200 || rec.Code >= 300 {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-DixieData-Redirect"); got == "" {
		t.Fatalf("expected X-DixieData-Redirect header, got %q", rec.Header().Get("X-DixieData-Redirect"))
	}
	respBody, _ := io.ReadAll(rec.Body)
	if !strings.Contains(string(respBody), "Collection created") {
		t.Fatalf("expected success body, got %q", string(respBody))
	}
}

// Regression net for the follow-up on issue #452: a stale "?from=<id>" that
// no longer points at an existing soldier (deleted row, merged archive, old
// bookmark) must NOT 500 the whole hub. The handler should fall back to
// fromID=0 (no current context) and render the hub as if the user landed
// there directly from top-nav.
func TestHandleResearchCollections_GETStaleFromIDFallsBackToHub(t *testing.T) {
	dataDir := testtemp.New(t).Path()
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	app := NewApp()
	app.dataDir = dataDir
	app.database = database
	if err := app.reloadServices(); err != nil {
		t.Fatalf("reloadServices: %v", err)
	}
	configureTestIdentity(t, app)
	app.setupRoutes()

	// Stale `?from=<nonexistent-id>` — was 500 with "Could not load research
	// collections." before fix; now must be 200 with the hub rendered.
	staleID := int64(999999)
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/research-collections?from=%d", staleID), nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("stale from=%d returned status=%d body=%q; expected 200 with hub fallback", staleID, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "panel.research-collections.hub") {
		t.Fatalf("expected hub panel in body; got %q", rec.Body.String())
	}
}

// Regression net for the add-to-collection route. The form action is
// /research-collections/{id}/add and POSTs soldier_id + from. Before the
// fix, the route was registered as r.Get only; POST returned 405 and the
// user saw a red toast on the hub. Now POST must succeed (200 +
// X-DixieData-Redirect + "Record added to collection" body), and a second
// POST of the same (collectionID, soldierID) must be idempotent (also
// 200, but the toast is the info-style "Already in this collection.").
//
// Mirrors TestHandleResearchCollections_POSTCreatesCollection (issue
// #452 follow-up): same root cause (route missing r.Post), different
// sub-path. The chi /research-collections/* catch-all needs both verbs.
func TestHandleResearchCollectionByIDAdd_POSTWiresRoute(t *testing.T) {
	dataDir := testtemp.New(t).Path()
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	app := NewApp()
	app.dataDir = dataDir
	app.database = database
	if err := app.reloadServices(); err != nil {
		t.Fatalf("reloadServices: %v", err)
	}
	configureTestIdentity(t, app)
	app.setupRoutes()

	// Seed a soldier + a collection so the add has work to do.
	soldier := seedAddRouteSoldier(t, app, "AddRoute", "Smoke")
	collection := seedAddRouteCollection(t, app, "Route Test")

	// First add: must succeed.
	body := fmt.Sprintf("soldier_id=%d&from=%d", soldier.ID, soldier.ID)
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/research-collections/%d/add", collection.ID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code == http.StatusMethodNotAllowed {
		t.Fatalf("POST /research-collections/%d/add returned 405; r.Post not registered. body=%q", collection.ID, rec.Body.String())
	}
	if rec.Code < 200 || rec.Code >= 300 {
		t.Fatalf("first add: status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("X-DixieData-Redirect") == "" {
		t.Fatalf("expected X-DixieData-Redirect header; got %q", rec.Header().Get("X-DixieData-Redirect"))
	}
	if got := rec.Header().Get("X-DixieData-Toast"); !strings.Contains(got, "added") {
		t.Fatalf("expected success toast on first add; got %q", got)
	}
	if got := rec.Header().Get("X-DixieData-Toast-Type"); got == "error" {
		t.Fatalf("first add must NOT be an error toast; got %q", got)
	}

	// Second add: idempotent re-add must also succeed (200, not 500)
	// and emit the info-style "Already in this collection" toast
	// (was: a red error toast before the fix).
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/research-collections/%d/add", collection.ID), strings.NewReader(body))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	app.ServeHTTP(rec2, req2)

	if rec2.Code < 200 || rec2.Code >= 300 {
		t.Fatalf("re-add: status=%d body=%q; expected 200 (idempotent no-op, NOT 500 error)",
			rec2.Code, rec2.Body.String())
	}
	if got := rec2.Header().Get("X-DixieData-Toast-Type"); got == "error" {
		t.Fatalf("re-add must NOT be an error toast; got %q (was: fmt.Errorf on already-in path, surfaced as red toast)",
			got)
	}
	if got := rec2.Header().Get("X-DixieData-Toast"); !strings.Contains(strings.ToLower(got), "already") {
		t.Fatalf("re-add toast should signal already-in; got %q", got)
	}
}
