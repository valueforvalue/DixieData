package appshell

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/uiids"
)

// Issue #455 slice 2: picker tests reanchored from cookie
// context to ?person=ID query param. The picker is now purely a
// search surface + Continue shortcut; the selected Person ID
// rides in the URL of the redirect, not in a cookie.

func TestHandleResearchPickerRendersShell(t *testing.T) {
	app := newStressApp(t)
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

func TestHandleResearchPickerRendersContinueFromPersonQuery(t *testing.T) {
	app := newStressApp(t)
	// Seed the sentinel Person Record that newPickerApp used to
	// seed (issue #378).
	if _, err := app.soldiers.Create(models.Soldier{
		DisplayID: "CSA-PICKER-411",
		SyncID:    "picker-411",
		FirstName: "Test",
		LastName:  "PersonFourEleven",
	}); err != nil {
		t.Fatalf("seed person: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/research?person=1", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Continue") {
		t.Fatalf("expected Continue shortcut in body when ?person=ID matches a row")
	}
}

func TestHandleResearchSelectRedirectsWithoutSettingCookie(t *testing.T) {
	app := newStressApp(t)
	if _, err := app.soldiers.Create(models.Soldier{
		DisplayID: "CSA-PICKER-411",
		SyncID:    "picker-411",
		FirstName: "Test",
		LastName:  "PersonFourEleven",
	}); err != nil {
		t.Fatalf("seed person: %v", err)
	}
	form := url.Values{}
	form.Set("person_id", "1")
	form.Set("next", "camaraderie")
	req := httptest.NewRequest(http.MethodPost, "/research/select", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	if got, want := rec.Header().Get("X-DixieData-Redirect"), "/soldiers/1/camaraderie"; got != want {
		t.Fatalf("X-DixieData-Redirect = %q; want %q", got, want)
	}
	// Issue #455 slice 2: no dd_person_ctx cookie may be emitted.
	for _, c := range rec.Result().Cookies() {
		if c.Name == "dd_person_ctx" {
			t.Fatalf("picker must NOT emit dd_person_ctx cookie post-slice-2")
		}
	}
}

func TestHandleResearchSelectRejectsMissingPersonID(t *testing.T) {
	app := newStressApp(t)
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
	app := newStressApp(t)
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

func TestHandleResearchClearNoLongerEmitsCookie(t *testing.T) {
	app := newStressApp(t)
	req := httptest.NewRequest(http.MethodPost, "/research/clear", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	if got := rec.Header().Get("X-DixieData-Redirect"); got != "/research" {
		t.Fatalf("X-DixieData-Redirect = %q; want /research", got)
	}
	// Issue #455 slice 2: no dd_person_ctx cookie at all (was a
	// MaxAge=-1 clear cookie pre-slice-2).
	for _, c := range rec.Result().Cookies() {
		if c.Name == "dd_person_ctx" {
			t.Fatalf("/research/clear must NOT emit dd_person_ctx cookie post-slice-2")
		}
	}
}

func TestRouteOrderResearchBeatsSoldiersWildcard(t *testing.T) {
	app := newStressApp(t)
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

func TestPickerSearchReturnsPartialFragment(t *testing.T) {
	app := newStressApp(t)
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
	app := newStressApp(t)
	req := httptest.NewRequest(http.MethodGet, "/research?next=timeline", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `name="next" value="timeline"`) {
		t.Fatalf("picker did not forward ?next=timeline to its hidden fields")
	}
}

// TestSubRouteLoadsDirectDeepLink (issue #455 slice 2) replaces
// the old TestSubRouteRedirectsThroughPickerNoCookie +
// TestSubRouteLoadsDirectWithCookie pair. After slice 2 the
// picker is not a gate; soldier-scoped sub-pages load directly
// without ANY cookie or query context (the request URL itself
// carries the id).
func TestSubRouteLoadsDirectDeepLink(t *testing.T) {
	app := newStressApp(t)
	seedTimelinePerson(t, app, 511)
	req := httptest.NewRequest(http.MethodGet, "/soldiers/511/timeline", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 (direct load, no picker gate)", rec.Code)
	}
	if loc := rec.Header().Get("Location"); strings.HasPrefix(loc, "/research?") {
		t.Fatalf("Location = %q; picker redirect fired post-slice-2 (gate removed)", loc)
	}
}

func TestPickerRejectsUnknownNextStillAfterForward(t *testing.T) {
	app := newStressApp(t)
	req := httptest.NewRequest(http.MethodGet, "/research?next=evil-payload", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("picker GET status = %d; want 200", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, `name="next" value="evil-payload"`) {
		t.Fatalf("picker echoed unknown next; should fallback to default")
	}
}

func TestResearchSelectHonorsForwardedNext(t *testing.T) {
	app := newStressApp(t)
	if _, err := app.soldiers.Create(models.Soldier{
		DisplayID: "CSA-PICKER-411",
		SyncID:    "picker-411",
		FirstName: "Test",
		LastName:  "PersonFourEleven",
	}); err != nil {
		t.Fatalf("seed person: %v", err)
	}
	form := url.Values{}
	form.Set("person_id", "1")
	form.Set("next", "timeline")
	req := httptest.NewRequest(http.MethodPost, "/research/select", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	if got, want := rec.Header().Get("X-DixieData-Redirect"), "/soldiers/1/timeline"; got != want {
		t.Fatalf("X-DixieData-Redirect = %q; want %q", got, want)
	}
}

// seedTimelinePerson inserts a sentinel Person Record at the
// supplied id so handleSoldierByID's timeline branch has a row
// to render. Mirrors the newPickerApp sentinel pattern from
// issue #378.
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
