package appshell

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Issue #422 slice 2 picker intelligence tests, reanchored for
// issue #455 slice 2 (?person=ID query in place of dd_person_ctx
// cookie). The Continue shortcut behavior assertions are unchanged;
// only the input context switched from cookie to query.

// emptyFieldSoldier inserts a soldier at id with empty birth_info
// + unit so the camaraderie's HasUnitForCamaraderie predicate is
// false and the picker research-pack sub-screen hides the county
// option.
func emptyFieldSoldier(t *testing.T, app *App, id int64) {
	t.Helper()
	conn := app.database.Conn()
	if _, err := conn.Exec(
		`INSERT INTO soldiers (id, display_id, sync_id, entry_type, first_name, last_name, created_at, updated_at)
		 VALUES (?, ?, ?, 'soldier', ?, ?, ?, ?)`,
		id,
		"CSA-EMPTY-"+itoaID(id),
		"empty-"+itoaID(id),
		"Test",
		"Empty"+itoaID(id),
		"2026-07-07T00:00:00Z",
		"2026-07-07T00:00:00Z",
	); err != nil {
		t.Fatalf("seed empty soldier %d: %v", id, err)
	}
}

// unitSoldier inserts a soldier at id with the supplied unit so
// HasUnitForCamaraderie is true.
func unitSoldier(t *testing.T, app *App, id int64, unit string) {
	t.Helper()
	conn := app.database.Conn()
	if _, err := conn.Exec(
		`INSERT INTO soldiers (id, display_id, sync_id, entry_type, first_name, last_name, unit, created_at, updated_at)
		 VALUES (?, ?, ?, 'soldier', ?, ?, ?, ?, ?)`,
		id,
		"CSA-UNIT-"+itoaID(id),
		"unit-"+itoaID(id),
		"Test",
		"Unit"+itoaID(id),
		unit,
		"2026-07-07T00:00:00Z",
		"2026-07-07T00:00:00Z",
	); err != nil {
		t.Fatalf("seed unit soldier %d: %v", id, err)
	}
}

// itoaID is a tiny strconv-free int formatter used by the
// picker test helpers. Renamed from "itoa" to avoid collision
// with the gold_master_test.go itoa in the same package.
func itoaID(id int64) string {
	if id < 10 {
		return string(rune('0' + id))
	}
	out := []byte{}
	for id > 0 {
		out = append([]byte{byte('0' + id%10)}, out...)
		id /= 10
	}
	return string(out)
}

// addPersonQuery appends ?person={id} to a request URL (issue #455
// slice 2 context shape).
func addPersonQuery(req *http.Request, personID int64) {
	q := req.URL.Query()
	q.Set("person", itoaID(personID))
	req.URL.RawQuery = q.Encode()
}

func TestPickerContinueShortcutHidesCamaraderieForUnitlessSoldier(t *testing.T) {
	app := newPickerApp(t)
	emptyFieldSoldier(t, app, 801)
	req := httptest.NewRequest(http.MethodGet, "/research", nil)
	addPersonQuery(req, 801)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, `value="camaraderie"`) && strings.Contains(body, "Continue to Camaraderie") {
		t.Fatalf("Continue shortcut should NOT show Camaraderie for unit-less soldier")
	}
	for _, action := range []string{"timeline", "research-log", "conflict-ledger", "research-pack"} {
		if !strings.Contains(body, `name="next" value="`+action+`"`) {
			t.Fatalf("Continue shortcut should show Continue-to-%s for any soldier", action)
		}
	}
}

func TestPickerContinueShortcutShowsCamaraderieForSoldierWithUnit(t *testing.T) {
	app := newPickerApp(t)
	unitSoldier(t, app, 802, "3rd Arkansas Mounted Rifles")
	req := httptest.NewRequest(http.MethodGet, "/research", nil)
	addPersonQuery(req, 802)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `name="next" value="camaraderie"`) {
		t.Fatalf("Continue shortcut should show Camaraderie for soldier with unit")
	}
}

func TestPickerContinueShortcutShowsAllActionsForFullyPopulatedSoldier(t *testing.T) {
	app := newPickerApp(t)
	conn := app.database.Conn()
	if _, err := conn.Exec(
		`INSERT INTO soldiers (id, display_id, sync_id, entry_type, first_name, last_name, unit, birth_info, pension_state, created_at, updated_at)
		 VALUES (?, ?, ?, 'soldier', ?, ?, ?, ?, '', ?, ?)`,
		int64(803),
		"CSA-FULL-803",
		"full-803",
		"Test",
		"FullSoldier803",
		"5th Texas Cavalry",
		"Born in Nacogdoches County, Texas",
		"2026-07-07T00:00:00Z",
		"2026-07-07T00:00:00Z",
	); err != nil {
		t.Fatalf("seed full soldier 803: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/research", nil)
	addPersonQuery(req, 803)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, action := range []string{"camaraderie", "timeline", "research-log", "conflict-ledger", "research-pack"} {
		if !strings.Contains(body, `name="next" value="`+action+`"`) {
			t.Fatalf("Continue shortcut should show Continue-to-%s for fully populated soldier", action)
		}
	}
}

func TestPickerContinueShortcutAbsentWhenNoPersonQuery(t *testing.T) {
	app := newPickerApp(t)
	// No ?person query — no current person — the Continue shortcut
	// should not render at all (no buttons, no soldier name).
	req := httptest.NewRequest(http.MethodGet, "/research", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "Continue to Camaraderie") {
		t.Fatalf("no-person picker should not show any Continue button")
	}
	if strings.Contains(body, "Continue:") {
		t.Fatalf("no-person picker should not show the Continue shortcut at all")
	}
}

func TestPickerSubScreenHidesCountyForSoldierWithoutBirthInfo(t *testing.T) {
	app := newPickerApp(t)
	emptyFieldSoldier(t, app, 804)
	req := httptest.NewRequest(http.MethodGet, "/research?next=research-pack", nil)
	addPersonQuery(req, 804)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "research-pack-state") {
		t.Fatalf("sub-screen should show state option even for soldier without birth_info")
	}
	if strings.Contains(body, "research-pack-county") {
		t.Fatalf("sub-screen should hide county option for soldier without birth_info")
	}
}

func TestPickerSubScreenShowsBothForSoldierWithCounty(t *testing.T) {
	app := newPickerApp(t)
	conn := app.database.Conn()
	if _, err := conn.Exec(
		`INSERT INTO soldiers (id, display_id, sync_id, entry_type, first_name, last_name, unit, birth_info, pension_state, created_at, updated_at)
		 VALUES (?, ?, ?, 'soldier', ?, ?, '', ?, '', ?, ?)`,
		int64(805),
		"CSA-COUNTY-805",
		"county-805",
		"Test",
		"CountySoldier805",
		"Born in Shelby County, Tennessee",
		"2026-07-07T00:00:00Z",
		"2026-07-07T00:00:00Z",
	); err != nil {
		t.Fatalf("seed county soldier 805: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/research?next=research-pack", nil)
	addPersonQuery(req, 805)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "research-pack-state") {
		t.Fatalf("sub-screen should show state option")
	}
	if !strings.Contains(body, "research-pack-county") {
		t.Fatalf("sub-screen should show county option for soldier with county in birth_info")
	}
}
