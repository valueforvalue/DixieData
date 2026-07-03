package appshell

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
)

// TestHandleInitialSetupPostSetsDixieRedirectAndToast is the
// regression net for issue #263. Before the fix, the POST
// branch of handleInitialSetup issued a silent 303 to
// /calendar with no toast, no busy feedback, and no
// redirect-header contract — so the data-dixie-submit JS
// intercept could not show an "Identity saved" toast and
// duplicate rapid clicks fired multiple ConfigureUserIdentity
// calls. After the fix the handler must respond with:
//
//   - 200 (Option C: the dispatcher reads X-DixieData-Redirect
//     and navigates, instead of relying on the browser to
//     follow a 303)
//   - X-DixieData-Redirect: /calendar
//   - X-DixieData-Toast: a user-visible success message
//
// Together with the data-dixie-submit attribute already on
// the form (which disables the button + sets aria-busy),
// this closes the silent-submit UX hole.
func TestHandleInitialSetupPostSetsDixieRedirectAndToast(t *testing.T) {
	dataDir := t.TempDir()
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
	app.setupRoutes()

	if !app.setupRequired {
		t.Fatal("expected fresh database to require initial setup")
	}

	form := url.Values{}
	form.Set("first_name", "Test")
	form.Set("middle_name", "H")
	form.Set("last_name", "User")
	form.Set("birth_year", "1900")

	req := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d (Option C: dispatcher navigates from X-DixieData-Redirect); body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != "" {
		t.Fatalf("expected no Location header (no native redirect); got %q", got)
	}
	if got := rec.Header().Get("X-DixieData-Redirect"); got != "/calendar" {
		t.Fatalf("X-DixieData-Redirect=%q want %q (dispatchDixieDataForm navigates from this header)", got, "/calendar")
	}
	if got := rec.Header().Get("X-DixieData-Toast"); got == "" {
		t.Fatalf("expected X-DixieData-Toast header so the dispatcher can show the success message on landing; headers=%v", rec.Header())
	}
	if got := rec.Header().Get("X-DixieData-Toast-Type"); got != "success" {
		t.Fatalf("X-DixieData-Toast-Type=%q want %q", got, "success")
	}
	if app.setupRequired {
		t.Fatal("setupRequired must be cleared on successful POST")
	}
}

// TestHandleInitialSetupGetStillRedirectsToCalendarWhenNotRequired
// guards the GET branch against drift: when setupRequired is
// already false, the GET handler continues to 303 to /calendar
// (it's the only branch that still uses StatusSeeOther, which
// is why redirect_headers_test.go's exemptFunctions comment
// points here). If a future refactor accidentally migrates the
// GET branch to the 200 + X-DixieData-Redirect contract, this
// test pins the legacy behaviour and the exemption comment in
// redirect_headers_test.go must be updated in the same commit.
func TestHandleInitialSetupGetStillRedirectsToCalendarWhenNotRequired(t *testing.T) {
	app := NewApp()
	app.setupRequired = false
	app.setupRoutes()

	req := httptest.NewRequest(http.MethodGet, "/setup", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusSeeOther)
	}
	if got := rec.Header().Get("Location"); got != "/calendar" {
		t.Fatalf("Location=%q want /calendar", got)
	}
}
