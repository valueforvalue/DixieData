package appshell

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// === Issue #422 slice 2 RED tests ===
//
// The picker page should hide unsupported sub-pages from the
// Continue shortcut when the current person lacks the required
// data. Camaraderie is hidden when the soldier has no unit; the
// picker sub-screen for research-pack hides the County option
// when the soldier has no county in birth_info.

func TestPickerContinueShortcutHidesCamaraderieForUnitlessSoldier(t *testing.T) {
	app := newPickerApp(t)
	emptyFieldSoldier(t, app, 801)
	req := httptest.NewRequest(http.MethodGet, "/research", nil)
	setCookie(t, req, app, 801)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, `value="camaraderie"`) && strings.Contains(body, "Continue to Camaraderie") {
		t.Fatalf("Continue shortcut should NOT show Camaraderie for unit-less soldier; got a Camaraderie button")
	}
	// Timeline / Research Log / Conflict Ledger / Research Pack
	// must still be present (they always work).
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
	setCookie(t, req, app, 802)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `name="next" value="camaraderie"`) {
		t.Fatalf("Continue shortcut should show Camaraderie for soldier with unit; got body: %s", body[:min(500, len(body))])
	}
}

func TestPickerContinueShortcutShowsAllActionsForFullyPopulatedSoldier(t *testing.T) {
	app := newPickerApp(t)
	// Seed a soldier with unit AND birth_info county.
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
	setCookie(t, req, app, 803)
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

func TestPickerContinueShortcutHidesCamaraderieWhenNoCookie(t *testing.T) {
	app := newPickerApp(t)
	// No cookie set — no current person — the Continue shortcut
	// should not render at all (no buttons, no soldier name).
	req := httptest.NewRequest(http.MethodGet, "/research", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "Continue to Camaraderie") {
		t.Fatalf("no-cookie picker should not show any Continue button")
	}
	// The Continue shortcut section should be absent.
	if strings.Contains(body, "Continue:") {
		t.Fatalf("no-cookie picker should not show the Continue shortcut at all")
	}
}

func TestPickerSubScreenHidesCountyForSoldierWithoutBirthInfo(t *testing.T) {
	app := newPickerApp(t)
	emptyFieldSoldier(t, app, 804)
	req := httptest.NewRequest(http.MethodGet, "/research?next=research-pack", nil)
	setCookie(t, req, app, 804)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	body := rec.Body.String()
	// State option should still be present (always available via
	// pension_state N/A default).
	if !strings.Contains(body, "research-pack-state") {
		t.Fatalf("sub-screen should show state option even for soldier without birth_info; got body: %s", body[:min(500, len(body))])
	}
	// County option should be hidden when birth_info is empty.
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
	setCookie(t, req, app, 805)
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
