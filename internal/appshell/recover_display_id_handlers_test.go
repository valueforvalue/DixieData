package appshell

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"testing"
)

// blankDisplayIDSoldier seeds a soldier with an empty display_id
// (simulating the pre-#376 corruption that #416 recovers). Inserts
// via raw SQL so the service-layer Create-time auto-generation
// doesn't mint a fresh id — we want a row that's blank from the
// start.
func blankDisplayIDSoldier(t *testing.T, app *App, id int64) {
	t.Helper()
	conn := app.database.Conn()
	if _, err := conn.Exec(
		`INSERT INTO soldiers (id, display_id, sync_id, entry_type, first_name, last_name, created_at, updated_at)
		 VALUES (?, '', ?, 'soldier', ?, ?, ?, ?)`,
		id,
		"recover-"+strconv.FormatInt(id, 10),
		"Test",
		"BlankID"+strconv.FormatInt(id, 10),
		"2026-07-07T00:00:00Z",
		"2026-07-07T00:00:00Z",
	); err != nil {
		t.Fatalf("seed blank-id soldier %d: %v", id, err)
	}
}

// generatedDisplayIDPattern matches a freshly-minted display id:
// e.g. DXD-00001, THU00-00001. We don't pin the exact prefix
// because the test app's node prefix differs from production.
var generatedDisplayIDPattern = regexp.MustCompile(`^[A-Z0-9]{3,6}-\d{5}$`)

func TestHandleRecoverDisplayID_Success(t *testing.T) {
	app := newPickerApp(t)
	blankDisplayIDSoldier(t, app, 901)
	req := httptest.NewRequest(http.MethodPost, "/soldiers/901/display-id/recover", nil)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200; body = %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		DisplayID string `json:"display_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal body: %v; body = %s", err, rec.Body.String())
	}
	if resp.DisplayID == "" {
		t.Errorf("display_id is empty")
	}
	if !generatedDisplayIDPattern.MatchString(resp.DisplayID) {
		t.Errorf("display_id = %q; want match for %s", resp.DisplayID, generatedDisplayIDPattern)
	}
}

func TestHandleRecoverDisplayID_404ForMissingRow(t *testing.T) {
	app := newPickerApp(t)
	req := httptest.NewRequest(http.MethodPost, "/soldiers/99999/display-id/recover", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d; want 404", rec.Code)
	}
}

func TestHandleRecoverDisplayID_409ForAlreadyHasID(t *testing.T) {
	app := newPickerApp(t)
	unitSoldier(t, app, 902, "Some Unit")
	req := httptest.NewRequest(http.MethodPost, "/soldiers/902/display-id/recover", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d; want 409; body = %s", rec.Code, rec.Body.String())
	}
}
