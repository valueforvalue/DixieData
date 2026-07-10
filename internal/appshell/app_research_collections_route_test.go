package appshell

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/testtemp"
)

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

// Plus the GET case stays healthy after the route change.
func TestHandleResearchCollections_GETStillRenders(t *testing.T) {
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

	req := httptest.NewRequest(http.MethodGet, "/research-collections", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%q", rec.Code, rec.Body.String())
	}
}
