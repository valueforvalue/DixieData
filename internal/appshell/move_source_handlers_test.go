package appshell

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

// === Issue #368 slice 2 PATCH handler tests ===

// createPersonWithRecords seeds a person with N source records
// (Pension/Parole/Letter, etc.) and returns the person id.
func createPersonWithRecords(t *testing.T, app *App, id int64, count int) int64 {
	t.Helper()
	records := make([]models.Record, count)
	for i := 0; i < count; i++ {
		records[i] = models.Record{
			RecordType: "Roster",
			AppID:      "APP-" + strconv.Itoa(i),
			Details:    "row " + strconv.Itoa(i),
		}
	}
	conn := app.database.Conn()
	if _, err := conn.Exec(
		`INSERT INTO soldiers (id, display_id, sync_id, entry_type, first_name, last_name, created_at, updated_at)
		 VALUES (?, ?, ?, 'soldier', ?, ?, ?, ?)`,
		id, "PICK-"+strconv.FormatInt(id, 10), "pick-"+strconv.FormatInt(id, 10),
		"Test", "Picker"+strconv.FormatInt(id, 10),
		"2026-07-07T00:00:00Z", "2026-07-07T00:00:00Z",
	); err != nil {
		t.Fatalf("seed person %d: %v", id, err)
	}
	for i, r := range records {
		if _, err := conn.Exec(
			`INSERT INTO records (sync_id, person_record_id, person_sync_id, record_type, app_id, details, sort_order)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			"src-"+strconv.FormatInt(int64(i), 10)+"-"+strconv.FormatInt(id, 10),
			id, "pick-"+strconv.FormatInt(id, 10),
			r.RecordType, r.AppID, r.Details, int64(i),
		); err != nil {
			t.Fatalf("seed record %d: %v", i, err)
		}
	}
	return id
}

func TestHandleMoveSoldierSource_Success(t *testing.T) {
	app := newPickerApp(t)
	pid := createPersonWithRecords(t, app, 1001, 3)
	// Get the first record's id.
	var firstRecordID int64
	if err := app.database.Conn().QueryRow(
		`SELECT id FROM records WHERE person_record_id = ? ORDER BY sort_order LIMIT 1`, pid,
	).Scan(&firstRecordID); err != nil {
		t.Fatalf("get first record id: %v", err)
	}
	req := httptest.NewRequest(http.MethodPatch,
		"/soldiers/"+strconv.FormatInt(pid, 10)+"/sources/"+strconv.FormatInt(firstRecordID, 10)+"/position",
		strings.NewReader("position=3"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200; body = %s", rec.Code, rec.Body.String())
	}
	// Verify the row's sort_order is now 3.
	var sortOrder int64
	if err := app.database.Conn().QueryRow(
		`SELECT sort_order FROM records WHERE id = ?`, firstRecordID,
	).Scan(&sortOrder); err != nil {
		t.Fatalf("read sort_order: %v", err)
	}
	if sortOrder != 3 {
		t.Errorf("sort_order = %d; want 3", sortOrder)
	}
}

func TestHandleMoveSoldierSource_400ForBadPosition(t *testing.T) {
	app := newPickerApp(t)
	pid := createPersonWithRecords(t, app, 1002, 3)
	var firstRecordID int64
	app.database.Conn().QueryRow(
		`SELECT id FROM records WHERE person_record_id = ? ORDER BY sort_order LIMIT 1`, pid,
	).Scan(&firstRecordID)
	req := httptest.NewRequest(http.MethodPatch,
		"/soldiers/"+strconv.FormatInt(pid, 10)+"/sources/"+strconv.FormatInt(firstRecordID, 10)+"/position",
		strings.NewReader("position=abc"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", rec.Code)
	}
}

func TestHandleMoveSoldierSource_404ForMissingSource(t *testing.T) {
	app := newPickerApp(t)
	pid := createPersonWithRecords(t, app, 1003, 3)
	req := httptest.NewRequest(http.MethodPatch,
		"/soldiers/"+strconv.FormatInt(pid, 10)+"/sources/99999/position",
		strings.NewReader("position=1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d; want 404", rec.Code)
	}
}

func TestHandleMoveSoldierSource_404ForForeignSource(t *testing.T) {
	app := newPickerApp(t)
	pA := createPersonWithRecords(t, app, 1004, 2)
	pB := createPersonWithRecords(t, app, 1005, 1)
	// Get the record from person B.
	var bRecID int64
	app.database.Conn().QueryRow(
		`SELECT id FROM records WHERE person_record_id = ? ORDER BY sort_order LIMIT 1`, pB,
	).Scan(&bRecID)
	// Try to move B's record under A.
	req := httptest.NewRequest(http.MethodPatch,
		"/soldiers/"+strconv.FormatInt(pA, 10)+"/sources/"+strconv.FormatInt(bRecID, 10)+"/position",
		strings.NewReader("position=1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d; want 404 (foreign record)", rec.Code)
	}
}


