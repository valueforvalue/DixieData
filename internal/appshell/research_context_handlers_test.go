package appshell

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/uiids"
)

func TestHandleResearchPickerRendersShellNoCookie(t *testing.T) {
	app := newPickerApp(t)
	req := httptest.NewRequest(http.MethodGet, "/research", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `id="`+uiids.PageResearchPicker+`"`) {
		t.Fatalf("picker page wrapper missing")
	}
	if !strings.Contains(body, `id="`+uiids.PanelResearchPickerSearch+`"`) {
		t.Fatalf("picker search panel missing")
	}
	if !strings.Contains(body, `id="`+uiids.PanelResearchPickerRecent+`"`) {
		t.Fatalf("picker recent panel missing")
	}
}

func TestHandleResearchPickerRendersContinueFromCookie(t *testing.T) {
	app := newPickerApp(t)
	req := httptest.NewRequest(http.MethodGet, "/research", nil)
	writePersonCtxCookie(t, req, app, 411)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Continue") {
		t.Fatalf("expected Continue shortcut in body")
	}
}

func TestHandleResearchSelectWritesCookieAndRedirects(t *testing.T) {
	app := newPickerApp(t)
	form := url.Values{}
	form.Set("person_id", "411")
	form.Set("next", "camaraderie")
	req := httptest.NewRequest(http.MethodPost, "/research/select", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	if got, want := rec.Header().Get("X-DixieData-Redirect"), "/soldiers/411/camaraderie"; got != want {
		t.Fatalf("X-DixieData-Redirect = %q; want %q", got, want)
	}
	found := false
	for _, c := range rec.Result().Cookies() {
		if c.Name == "dd_person_ctx" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("dd_person_ctx cookie not emitted")
	}
}

func TestHandleResearchSelectRejectsMissingPersonID(t *testing.T) {
	app := newPickerApp(t)
	form := url.Values{}
	form.Set("next", "camaraderie")
	req := httptest.NewRequest(http.MethodPost, "/research/select", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", rec.Code)
	}
}

func TestHandleResearchSelectRejectsBadNext(t *testing.T) {
	app := newPickerApp(t)
	form := url.Values{}
	form.Set("person_id", "1")
	form.Set("next", "../../etc/passwd")
	req := httptest.NewRequest(http.MethodPost, "/research/select", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", rec.Code)
	}
}

func TestHandleResearchClearEmitsMaxAgeMinusOne(t *testing.T) {
	app := newPickerApp(t)
	req := httptest.NewRequest(http.MethodPost, "/research/clear", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	if got := rec.Header().Get("X-DixieData-Redirect"); got != "/research" {
		t.Fatalf("X-DixieData-Redirect = %q; want /research", got)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "dd_person_ctx" && c.MaxAge >= 0 {
			t.Fatalf("dd_person_ctx cookie not cleared")
		}
	}
}

func TestRouteOrderResearchBeatsSoldiersWildcard(t *testing.T) {
	app := newPickerApp(t)
	req := httptest.NewRequest(http.MethodGet, "/research", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "Unknown soldier") {
		t.Fatalf("dispatched to handleSoldierByID")
	}
	if !strings.Contains(body, `id="`+uiids.PageResearchPicker+`"`) {
		t.Fatalf("did not render picker")
	}
}

// === Issue #378 slice 2 RED tests ===
// The tests below prove slice-2 behavior before any slice-2 code
// exists. They MUST fail until the slice-2 handler + guard + templ
// changes land.

func TestPickerSearchReturnsPartialFragment(t *testing.T) {
	app := newPickerApp(t)
	req := httptest.NewRequest(http.MethodGet, "/research?q=Person&partial=1", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `id="`+uiids.PanelResearchPickerResults+`"`) {
		t.Fatalf("partial response missing results panel; full layout should be suppressed")
	}
	if strings.Contains(body, `id="`+uiids.PageResearchPicker+`"`) {
		t.Fatalf("partial response unexpectedly included the full-page wrapper")
	}
}

func TestPickerForwardsNextFromQuery(t *testing.T) {
	app := newPickerApp(t)
	req := httptest.NewRequest(http.MethodGet, "/research?next=timeline", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	body := rec.Body.String()
	// The picker shell must echo the next value into hidden fields
	// on every "Open"/"Continue" form so the post-redirect lands
	// on the right sub-page.
	if !strings.Contains(body, `name="next" value="timeline"`) {
		t.Fatalf("picker did not forward ?next=timeline to its hidden fields")
	}
}

func TestSubRouteRedirectsThroughPickerNoCookie(t *testing.T) {
	app := newPickerApp(t)
	req := httptest.NewRequest(http.MethodGet, "/soldiers/411/timeline", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d; want 303 (redirect through picker)", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/research?next=timeline") {
		t.Fatalf("Location = %q; want prefix /research?next=timeline", loc)
	}
}

func TestSubRouteLoadsDirectWithCookie(t *testing.T) {
	app := newPickerApp(t)
	// Seed a Person Record and give the request a valid
	// dd_person_ctx cookie pointing at it. The handleSoldierByID
	// branch for /timeline should now load directly without
	// redirecting through /research.
	seedTimelinePerson(t, app, 511)
	req := httptest.NewRequest(http.MethodGet, "/soldiers/511/timeline", nil)
	writePersonCtxCookie(t, req, app, 511)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 (direct load with cookie)", rec.Code)
	}
	if loc := rec.Header().Get("Location"); strings.HasPrefix(loc, "/research?") {
		t.Fatalf("Location = %q; picker redirect fired despite cookie present", loc)
	}
}

func TestPickerRejectsUnknownNextStillAfterForward(t *testing.T) {
	app := newPickerApp(t)
	// Forwarding a payload the picker allowed through (rendered as
	// hidden fields) MUST still 400 at select time. The allowlist
	// is the only gate; never trust URL flow.
	req := httptest.NewRequest(http.MethodGet, "/research?next=evil-payload", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("picker GET status = %d; want 200", rec.Code)
	}
	// Picker did not echo the unknown next into hidden fields
	// (echo is allowlisted too, by design).
	body := rec.Body.String()
	if strings.Contains(body, `name="next" value="evil-payload"`) {
		t.Fatalf("picker echoed unknown next; should fallback to default")
	}
}

func TestResearchSelectHonorsForwardedNext(t *testing.T) {
	app := newPickerApp(t)
	form := url.Values{}
	form.Set("person_id", "411")
	form.Set("next", "timeline")
	req := httptest.NewRequest(http.MethodPost, "/research/select", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	if got, want := rec.Header().Get("X-DixieData-Redirect"), "/soldiers/411/timeline"; got != want {
		t.Fatalf("X-DixieData-Redirect = %q; want %q", got, want)
	}
}

// seedTimelinePerson inserts a sentinel Person Record at the
// supplied id so handleSoldierByID's timeline branch has a row
// to render. Mirrors newPickerApp's sentinel pattern.
func seedTimelinePerson(t *testing.T, app *App, id int64) {
	t.Helper()
	conn := app.database.Conn()
	if _, err := conn.Exec(
		`INSERT INTO soldiers (id, display_id, sync_id, entry_type, first_name, last_name, created_at, updated_at)
		 VALUES (?, ?, ?, 'soldier', ?, ?, ?, ?)`,
		id,
		"CSA-TIMELINE-"+strconv.FormatInt(id, 10),
		"timeline-"+strconv.FormatInt(id, 10),
		"Test",
		"Person"+strconv.FormatInt(id, 10),
		"2026-07-07T00:00:00Z",
		"2026-07-07T00:00:00Z",
	); err != nil {
		t.Fatalf("seed sentinel Person Record %d: %v", id, err)
	}
}
