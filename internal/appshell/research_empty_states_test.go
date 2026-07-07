package appshell

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/cookies"
)

// === Issue #422 slice 1 RED tests ===
//
// These tests prove the handler empty-state contract BEFORE the
// fix lands. Currently each test that asserts 200 or 404/400
// should FAIL because the existing handlers return 500 for
// missing-data scenarios.
//
// Note: research-pack/state is NOT affected — PensionState
// normalizes to "N/A" on read so it's never truly empty.

func emptyFieldSoldier(t *testing.T, app *App, id int64) {
	t.Helper()
	conn := app.database.Conn()
	if _, err := conn.Exec(
		`INSERT INTO soldiers (id, display_id, sync_id, entry_type, first_name, last_name, unit, birth_info, pension_state, created_at, updated_at)
		 VALUES (?, ?, ?, 'soldier', ?, ?, '', '', '', ?, ?)`,
		id,
		"CSA-EMPTY-"+strconv.FormatInt(id, 10),
		"empty-"+strconv.FormatInt(id, 10),
		"Test",
		"EmptySoldier"+strconv.FormatInt(id, 10),
		"2026-07-07T00:00:00Z",
		"2026-07-07T00:00:00Z",
	); err != nil {
		t.Fatalf("seed empty-field soldier %d: %v", id, err)
	}
}

func unitSoldier(t *testing.T, app *App, id int64, unit string) {
	t.Helper()
	conn := app.database.Conn()
	if _, err := conn.Exec(
		`INSERT INTO soldiers (id, display_id, sync_id, entry_type, first_name, last_name, unit, birth_info, pension_state, created_at, updated_at)
		 VALUES (?, ?, ?, 'soldier', ?, ?, ?, '', '', ?, ?)`,
		id,
		"CSA-UNIT-"+strconv.FormatInt(id, 10),
		"unit-"+strconv.FormatInt(id, 10),
		"Test",
		"UnitSoldier"+strconv.FormatInt(id, 10),
		unit,
		"2026-07-07T00:00:00Z",
		"2026-07-07T00:00:00Z",
	); err != nil {
		t.Fatalf("seed unit-soldier %d: %v", id, err)
	}
}

func setCookie(t *testing.T, req *http.Request, app *App, personID int64) {
	t.Helper()
	rec := httptest.NewRecorder()
	if err := cookies.WritePersonCtx(rec, app.personCtxKey, personID); err != nil {
		t.Fatalf("WritePersonCtx: %v", err)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == cookies.CookieName {
			req.AddCookie(c)
			return
		}
	}
	t.Fatal("cookie not set")
}

// --- Empty-state 200 tests ---

func TestHandleUnitCamaraderieRendersEmptyStateForUnitlessSoldier(t *testing.T) {
	app := newPickerApp(t)
	emptyFieldSoldier(t, app, 701)
	req := httptest.NewRequest(http.MethodGet, "/soldiers/701/camaraderie", nil)
	setCookie(t, req, app, 701)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 (empty-state page, not 500)", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "EmptySoldier701") {
		t.Fatalf("empty-state page must mention the soldier's name; got preview: %s", body[:min(300, len(body))])
	}
	if strings.Contains(body, "Could not build") {
		t.Fatalf("empty-state page must not contain the 500 error string")
	}
}

func TestHandleUnitCamaraderieRendersGraphForSoldierWithUnit(t *testing.T) {
	app := newPickerApp(t)
	unitSoldier(t, app, 702, "3rd Arkansas Mounted Rifles")
	req := httptest.NewRequest(http.MethodGet, "/soldiers/702/camaraderie", nil)
	setCookie(t, req, app, 702)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 (full graph)", rec.Code)
	}
}

func TestHandleResearchPackCountyRendersEmptyStateWhenNoBirthInfo(t *testing.T) {
	app := newPickerApp(t)
	emptyFieldSoldier(t, app, 704)
	req := httptest.NewRequest(http.MethodGet, "/soldiers/704/research-pack/county", nil)
	setCookie(t, req, app, 704)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 (empty-state page)", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "Could not build") {
		t.Fatalf("empty-state page must not contain the 500 error string")
	}
}

// --- Error dispatch normalisation tests ---

func TestHandleUnitCamaraderieReturns404ForMissingSoldier(t *testing.T) {
	app := newPickerApp(t)
	req := httptest.NewRequest(http.MethodGet, "/soldiers/99999/camaraderie", nil)
	// Set a cookie so the picker-guard doesn't redirect us — we
	// want to exercise the handler's own 404 path for a genuinely
	// missing soldier ID (not the guard redirect).
	setCookie(t, req, app, 99999)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d; want 404 for a genuinely missing soldier", rec.Code)
	}
}

func TestHandleAddResearchTaskRejectsBlankTitleWith400(t *testing.T) {
	app := newPickerApp(t)
	emptyFieldSoldier(t, app, 705)
	req := httptest.NewRequest(http.MethodPost, "/soldiers/705/research-log/tasks", strings.NewReader("title=&notes=&evidence_type="))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-DixieData-Submit", "true")
	setCookie(t, req, app, 705)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code == http.StatusInternalServerError {
		t.Fatalf("status = 500; want 400 (validation error for blank title)")
	}
	if rec.Code != http.StatusOK && rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400 or 200 with toast", rec.Code)
	}
}

func TestHandleResolveResearchTaskReturns404ForMissingTask(t *testing.T) {
	app := newPickerApp(t)
	emptyFieldSoldier(t, app, 706)
	req := httptest.NewRequest(http.MethodPost, "/soldiers/706/research-log/tasks/999999/resolve", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-DixieData-Submit", "true")
	setCookie(t, req, app, 706)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d; want 404 for a genuinely missing task", rec.Code)
	}
}
