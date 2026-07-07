package appshell

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

// === Issue #368 slice 2 PATCH event source handler tests ===

// createEventWithSources seeds an event with N event_source rows
// and returns the event id.
func createEventWithSources(t *testing.T, app *App, id int64, count int) int64 {
	t.Helper()
	conn := app.database.Conn()
	if _, err := conn.Exec(
		`INSERT INTO soldiers (id, display_id, sync_id, entry_type, kind, first_name, last_name, begin_date, end_date, created_at, updated_at)
		 VALUES (?, ?, ?, 'event', 'battle', '', '', '1862-09-17', '1862-09-17', ?, ?)`,
		id, "EVT-MV-"+strconv.FormatInt(id, 10), "evt-mv-"+strconv.FormatInt(id, 10),
		"2026-07-07T00:00:00Z", "2026-07-07T00:00:00Z",
	); err != nil {
		t.Fatalf("seed event %d: %v", id, err)
	}
	for i := 0; i < count; i++ {
		if _, err := conn.Exec(
			`INSERT INTO event_sources (sync_id, event_id, event_sync_id, record_type, app_id, details, sort_order)
			 VALUES (?, ?, ?, 'Roster', ?, 'row', ?)`,
			"evt-src-"+strconv.FormatInt(int64(i), 10)+"-"+strconv.FormatInt(id, 10),
			id, "evt-mv-"+strconv.FormatInt(id, 10),
			"APP-E"+strconv.Itoa(i), int64(i),
		); err != nil {
			t.Fatalf("seed event source %d: %v", i, err)
		}
	}
	return id
}

func TestHandleMoveEventSource_Success(t *testing.T) {
	app := newPickerApp(t)
	eid := createEventWithSources(t, app, 2001, 3)
	var firstSourceID int64
	if err := app.database.Conn().QueryRow(
		`SELECT id FROM event_sources WHERE event_id = ? ORDER BY sort_order LIMIT 1`, eid,
	).Scan(&firstSourceID); err != nil {
		t.Fatalf("get first source id: %v", err)
	}
	req := httptest.NewRequest(http.MethodPatch,
		"/events/"+strconv.FormatInt(eid, 10)+"/sources/"+strconv.FormatInt(firstSourceID, 10)+"/position",
		strings.NewReader("position=3"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200; body = %s", rec.Code, rec.Body.String())
	}
	var sortOrder int64
	if err := app.database.Conn().QueryRow(
		`SELECT sort_order FROM event_sources WHERE id = ?`, firstSourceID,
	).Scan(&sortOrder); err != nil {
		t.Fatalf("read sort_order: %v", err)
	}
	if sortOrder != 3 {
		t.Errorf("sort_order = %d; want 3", sortOrder)
	}
}

func TestHandleMoveEventSource_404ForMissingSource(t *testing.T) {
	app := newPickerApp(t)
	eid := createEventWithSources(t, app, 2002, 2)
	req := httptest.NewRequest(http.MethodPatch,
		"/events/"+strconv.FormatInt(eid, 10)+"/sources/99999/position",
		strings.NewReader("position=1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d; want 404", rec.Code)
	}
}

func TestHandleMoveEventSource_404ForForeignSource(t *testing.T) {
	app := newPickerApp(t)
	eA := createEventWithSources(t, app, 2003, 1)
	eB := createEventWithSources(t, app, 2004, 1)
	var aSourceID int64
	app.database.Conn().QueryRow(
		`SELECT id FROM event_sources WHERE event_id = ? LIMIT 1`, eA,
	).Scan(&aSourceID)
	// Try to move A's source under B.
	req := httptest.NewRequest(http.MethodPatch,
		"/events/"+strconv.FormatInt(eB, 10)+"/sources/"+strconv.FormatInt(aSourceID, 10)+"/position",
		strings.NewReader("position=1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d; want 404 (foreign source)", rec.Code)
	}
}

var _ = models.Soldier{} // keep models import even if helpers go away
