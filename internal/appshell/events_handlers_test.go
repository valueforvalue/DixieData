// events_handlers_test.go covers the slice-3 Event Record
// HTTP handlers (issue #320). The tests use httptest.NewRecorder
// to drive the chi-mounted routes through the App.ServeHTTP
// path. Each test creates a fresh temp-app via newStressApp so
// the EventService wiring (slice 2's NewEventService) is
// exercised end-to-end.
package appshell

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
 )
func TestHandleEventsEmptyList(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	resp, err := http.Get(server.URL + "/events")
	if err != nil {
		t.Fatalf("GET /events: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /events status = %d, want 200", resp.StatusCode)
	}
	body := readAll(t, resp)
	if !strings.Contains(body, "Event Records") {
		t.Errorf("GET /events body missing 'Event Records' heading: %q", body)
	}
}

// TestHandleNewEventGetForm verifies the /events/new GET
// renders the form with a pre-allocated EVT-NNNNN Display
// ID and the entry_type hidden field set to "event".
func TestHandleNewEventGetForm(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	resp, err := http.Get(server.URL + "/events/new")
	if err != nil {
		t.Fatalf("GET /events/new: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /events/new status = %d, want 200", resp.StatusCode)
	}
	body := readAll(t, resp)
	if !strings.Contains(body, `name="entry_type" value="event"`) {
		t.Errorf("GET /events/new body missing entry_type=event hidden field: %q", body)
	}
	if !strings.Contains(body, "EVT-") {
		t.Errorf("GET /events/new body missing pre-allocated EVT-NNNNN Display ID: %q", body)
	}
}

// TestHandleNewEventPostCreatesEvent verifies the /events/new
// POST creates a row and redirects to the new event's
// detail page with X-DixieData-Redirect.
func TestHandleNewEventPostCreatesEvent(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	form := url.Values{}
	form.Set("entry_type", "event")
	form.Set("kind", "Battle")
	form.Set("begin_date", "07/01/1863")
	form.Set("end_date", "07/03/1863")
	form.Set("description", "The Battle of Gettysburg")
	form.Set("notes", "Decisive engagement")
	resp, err := http.PostForm(server.URL+"/events/new", form)
	if err != nil {
		t.Fatalf("POST /events/new: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /events/new status = %d, want 200 (Option C redirect)", resp.StatusCode)
	}
	redirect := resp.Header.Get("X-DixieData-Redirect")
	if !strings.HasPrefix(redirect, "/events/") {
		t.Errorf("X-DixieData-Redirect = %q, want /events/{id} prefix", redirect)
	}
	// Verify the event landed in the database by looking
	// up the row id from the redirect URL. The id in
	// /events/{id} is the SQLite row id; the DisplayID
	// is the EVT-NNNNN string allocated by NextEventID.
	idStr := strings.TrimPrefix(redirect, "/events/")
	id, perr := parseInt64(idStr)
	if perr != nil {
		t.Fatalf("parse id %q: %v", idStr, perr)
	}
	if _, err := app.events.GetEventByID(id); err != nil {
		t.Errorf("new event %d not found in DB: %v", id, err)
	}
}

// TestHandleEventByIDGetDetail verifies GET /events/{id}
// renders the detail page for an existing event.
func TestHandleEventByIDGetDetail(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	created := createEvent(t, app, "Skirmish", "06/15/1863", "", "A small engagement")
	resp, err := http.Get(server.URL + "/events/" + intStr(created.ID))
	if err != nil {
		t.Fatalf("GET /events/%d: %v", created.ID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /events/%d status = %d, want 200", created.ID, resp.StatusCode)
	}
	body := readAll(t, resp)
	if !strings.Contains(body, "Skirmish") {
		t.Errorf("GET /events/%d body missing kind: %q", created.ID, body)
	}
	if !strings.Contains(body, created.DisplayID) {
		t.Errorf("GET /events/%d body missing display ID %q", created.ID, created.DisplayID)
	}
}

// TestHandleEventByIDDelete removes an event and verifies
// the redirect back to /events.
func TestHandleEventByIDDelete(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	created := createEvent(t, app, "ToDelete", "01/01/1864", "", "")

	req, _ := http.NewRequest(http.MethodDelete, server.URL+"/events/"+intStr(created.ID), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /events/%d: %v", created.ID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE /events/%d status = %d, want 200", created.ID, resp.StatusCode)
	}
	redirect := resp.Header.Get("X-DixieData-Redirect")
	if redirect != "/events" {
		t.Errorf("X-DixieData-Redirect = %q, want /events", redirect)
	}
	// Verify the event is gone.
	if _, err := app.events.GetEventByID(created.ID); err == nil {
		t.Errorf("event %d still present after DELETE", created.ID)
	}
}

// TestHandleAttachEventAndDetachEvent verifies the link
// CRUD round-trips through the route handlers and the
// junction table.
func TestHandleAttachEventAndDetachEvent(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	// Seed one Person Record and one Event.
	person := createSoldier(t, app, "S")
	event := createEvent(t, app, "Battle", "07/01/1863", "", "")

	// Attach.
	resp, err := http.PostForm(server.URL+"/soldiers/"+intStr(person.ID)+"/events/"+intStr(event.ID)+"/attach", url.Values{})
	if err != nil {
		t.Fatalf("POST attach: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST attach status = %d", resp.StatusCode)
	}

	// Verify link exists.
	linked, err := app.events.ListForPerson(person.ID)
	if err != nil {
		t.Fatalf("ListForPerson: %v", err)
	}
	if len(linked) != 1 || linked[0].ID != event.ID {
		t.Fatalf("attach: linked = %v, want exactly the seeded event", linked)
	}

	// Detach.
	resp, err = http.PostForm(server.URL+"/soldiers/"+intStr(person.ID)+"/events/"+intStr(event.ID)+"/detach", url.Values{})
	if err != nil {
		t.Fatalf("POST detach: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST detach status = %d", resp.StatusCode)
	}

	// Verify link is gone.
	linked, err = app.events.ListForPerson(person.ID)
	if err != nil {
		t.Fatalf("ListForPerson after detach: %v", err)
	}
	if len(linked) != 0 {
		t.Errorf("detach: linked = %v, want empty", linked)
	}
}

// TestHandleAttachEventDuplicateReturnsConflict verifies the
// duplicate-link path returns a 409 conflict response.
func TestHandleAttachEventDuplicateReturnsConflict(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	person := createSoldier(t, app, "S")
	event := createEvent(t, app, "Battle", "07/01/1863", "", "")

	// First attach succeeds.
	resp, _ := http.PostForm(server.URL+"/soldiers/"+intStr(person.ID)+"/events/"+intStr(event.ID)+"/attach", url.Values{})
	resp.Body.Close()
	// Second attach returns 409.
	resp, _ = http.PostForm(server.URL+"/soldiers/"+intStr(person.ID)+"/events/"+intStr(event.ID)+"/attach", url.Values{})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("duplicate attach status = %d, want 409", resp.StatusCode)
	}
}

// TestHandleQuickAddEvent creates a new event + link in
// one POST. Verifies the event row exists, the link row
// exists, and the response is a redirect to the new event
// detail page.
func TestHandleQuickAddEvent(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	person := createSoldier(t, app, "S")
	form := url.Values{}
	form.Set("kind", "Skirmish")
	form.Set("begin_date", "05/10/1862")
	form.Set("description", "A small but bloody affair")
	resp, err := http.PostForm(server.URL+"/soldiers/"+intStr(person.ID)+"/events/quick-add", form)
	if err != nil {
		t.Fatalf("POST quick-add: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST quick-add status = %d", resp.StatusCode)
	}
	redirect := resp.Header.Get("X-DixieData-Redirect")
	if !strings.HasPrefix(redirect, "/events/") {
		t.Errorf("X-DixieData-Redirect = %q, want /events/{id}", redirect)
	}
	// Verify link.
	linked, err := app.events.ListForPerson(person.ID)
	if err != nil {
		t.Fatalf("ListForPerson: %v", err)
	}
	if len(linked) != 1 {
		t.Errorf("linked = %v, want exactly one event", linked)
	}
}

// TestHandleUpdateEvent verifies PUT /events/{id} updates
// the kind and description fields.
func TestHandleUpdateEvent(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	created := createEvent(t, app, "Before", "01/01/1864", "", "Original description")
	form := url.Values{}
	form.Set("entry_type", "event")
	form.Set("kind", "After")
	form.Set("description", "Updated description")
	req, _ := http.NewRequest(http.MethodPut, server.URL+"/events/"+intStr(created.ID), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /events/%d: %v", created.ID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT /events/%d status = %d", created.ID, resp.StatusCode)
	}
	// Re-fetch and verify.
	updated, err := app.events.GetEventByID(created.ID)
	if err != nil {
		t.Fatalf("GetEventByID: %v", err)
	}
	if updated.Event.Kind != "After" {
		t.Errorf("kind = %q, want %q", updated.Event.Kind, "After")
	}
	if updated.Event.Description != "Updated description" {
		t.Errorf("description = %q, want %q", updated.Event.Description, "Updated description")
	}
}

// --- test helpers ---

// createSoldier seeds a minimal Person Record (entry_type
// defaults to soldier) and returns it. Used by the
// Person-Record → Events tab tests below.
func createSoldier(t *testing.T, app *App, label string) models.Soldier {
	t.Helper()
	id, err := app.database.NextDXDID()
	if err != nil {
		t.Fatalf("NextDXDID: %v", err)
	}
	s, err := app.soldiers.Create(models.Soldier{
		DisplayID:     id,
		EntryType:     models.EntryTypeSoldier,
		FirstName:     "Test",
		LastName:      label,
		RankOut:       "PVT",
		RankIn:        "PVT",
		PensionState:  "Not Applicable",
	})
	if err != nil {
		t.Fatalf("soldier.Create: %v", err)
	}
	return *s
}

// createEvent seeds an Event Record and returns it. Uses
// the events facade so the EventService.CreateEvent path is
// exercised.
func createEvent(t *testing.T, app *App, kind, begin, end, description string) models.Soldier {
	t.Helper()
	event, err := app.events.CreateEvent(models.Soldier{
		EntryType:   models.EntryTypeEvent,
		Kind:        kind,
		BeginDate:   begin,
		EndDate:     end,
		Description: description,
	})
	if err != nil {
		t.Fatalf("events.CreateEvent: %v", err)
	}
	return *event
}

// readAll reads the response body to a string. Used by the
// assertions that grep for substrings.
func readAll(t *testing.T, resp *http.Response) string {
	t.Helper()
	var b strings.Builder
	buf := make([]byte, 1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			b.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
	return b.String()
}
// intStr formats an int64 as a string.
func intStr(n int64) string {
	if n == 0 {
		return "0"
	}
	var digits [20]byte
	i := len(digits)
	for n > 0 {
		i--
		digits[i] = byte('0' + n%10)
		n /= 10
	}
	return string(digits[i:])
}

// parseInt64 parses a string to int64. Used by the
// CreateEvent test to recover the row id from the
// /events/{id} redirect.
func parseInt64(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}
// extractDisplayID was a placeholder helper kept for
// earlier test scaffolding. The CreateEvent test now
// recovers the row id from the redirect URL directly via
// parseInt64. Kept as a no-op for any future reference.
func extractDisplayID(string) string { return "" }
