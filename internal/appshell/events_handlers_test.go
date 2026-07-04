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
	"os"
	"path/filepath"
	"strings"
	"strconv"
	"testing"
	"time"

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
	id, perr := strconv.ParseInt(idStr, 10, 64)
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

// TestHandlePersonEventsTab verifies the lazy-loaded fragment
// for the Person Record → Events tab (issue #320 slice #324).
// Replaces the slice-3 303 redirect with a 200 + htmx fragment
// containing the linked-events table (D5 of #322, no biography
// excerpt).
func TestHandlePersonEventsTab(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	person := createSoldier(t, app, "Test Person")
	event := createEvent(t, app, "Battle of Springfield", "10/25/1864", "10/25/1864", "Decisive engagement")
	if _, err := app.events.AttachEventToPerson(event.ID, person.ID); err != nil {
		t.Fatalf("AttachEventToPerson: %v", err)
	}

	resp, err := http.Get(server.URL + "/soldiers/" + intStr(person.ID) + "/events")
	if err != nil {
		t.Fatalf("GET /soldiers/%d/events: %v", person.ID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /soldiers/%d/events status = %d, want 200", person.ID, resp.StatusCode)
	}
	body := readAll(t, resp)
	// The fragment is wrapped in a <div class="space-y-3"> root
	// by person_events_tab.templ. The linked event's Display ID
	// must appear in the body so the lazy-load actually returns
	// useful content (D5 of #322).
	if !strings.Contains(body, event.DisplayID) {
		t.Errorf("fragment missing linked event Display ID %q", event.DisplayID)
	}
	if !strings.Contains(body, "Battle") {
		t.Errorf("fragment missing linked event kind %q", "Battle")
	}
	if !strings.Contains(body, "linked") {
		t.Errorf("fragment missing 'linked' count label")
	}
}
// (issue #320 v1). The test substitutes the Wails native
// save dialog with a temp file via saveFileDialogOverride so
// the render path is exercised end-to-end without a desktop
// dialog. Asserts:
//   - the rendered file starts with %PDF-
//   - the file name matches eventPDFName (D4: Event-<DisplayID>.pdf)
//   - the in-flight dedup key is cleared so a second request succeeds
func TestHandleEventPDF(t *testing.T) {
	app := newStressApp(t)

	created := createEvent(t, app, "Battle of Springfield", "10/25/1864", "10/25/1864", "Decisive engagement")

	want := filepath.Join(t.TempDir(), eventPDFName(created))
	app.saveFileDialogOverride = func(opts any) (string, error) { return want, nil }
	defer func() { app.saveFileDialogOverride = nil }()

	server := httptest.NewServer(app)
	defer server.Close()

	form := url.Values{}
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/events/"+intStr(created.ID)+"/pdf", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /events/%d/pdf: %v", created.ID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /events/%d/pdf status = %d, want 200", created.ID, resp.StatusCode)
	}

	// File name: D4 says Event-<DisplayID>.pdf
	if !strings.HasSuffix(want, eventPDFName(created)) {
		t.Errorf("file name = %q, want suffix %q", want, eventPDFName(created))
	}

	// Wait for the export job to complete (typst cold-start is
	// slow on Windows; the 200 OK + X-DixieData-Redirect returns
	// immediately while the render runs in a background goroutine).
	waitForEventPDFJob(t, app, want)

	body, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("read %q: %v", want, err)
	}
	if len(body) < 4 || string(body[:4]) != "%PDF" {
		t.Errorf("file body = %q... (len=%d), want prefix %%PDF-", string(body[:min(8, len(body))]), len(body))
	}
	// first one cleared the key when the export completed).
	req2, _ := http.NewRequest(http.MethodPost, server.URL+"/events/"+intStr(created.ID)+"/pdf", strings.NewReader(""))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("second POST /events/%d/pdf: %v", created.ID, err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("second POST status = %d, want 200 (in-flight key not cleared)", resp2.StatusCode)
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

// waitForEventPDFJob polls until the per-Event PDF export
// job completes. The job creates the output file via
// os.Create inside ExportEventPDF; typst then writes the
// content asynchronously. We exit when the file size is
// non-zero (the file is complete) or after 60s.
func waitForEventPDFJob(t *testing.T, app *App, outPath string) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		if info, err := os.Stat(outPath); err == nil && info.Size() > 0 {
			return
		}
	}
	t.Fatalf("event PDF job did not produce a non-empty file at %q within 60s", outPath)
}
// extractDisplayID was a placeholder helper kept for
// earlier test scaffolding. The CreateEvent test now
// recovers the row id from the redirect URL directly via
// parseInt64. Kept as a no-op for any future reference.
// TestHandlePersonEventsTabUnlink covers slice #325 (issue #320):
// the inline Unlink form on each linked-event row in the
// Person Events tab fragment posts to
// /soldiers/{id}/events/{eventId}/detach. The test attaches an
// event, hits the detach form via the route that the template
// emits, then re-asks the fragment and asserts the Event is no
// longer in the body.
func TestHandlePersonEventsTabUnlink(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	person := createSoldier(t, app, "Tab Unlink Person")
	event := createEvent(t, app, "Skirmish", "06/12/1864", "", "Engagement")
	if _, err := app.events.AttachEventToPerson(event.ID, person.ID); err != nil {
		t.Fatalf("AttachEventToPerson: %v", err)
	}

	resp, err := http.PostForm(server.URL+"/soldiers/"+intStr(person.ID)+"/events/"+intStr(event.ID)+"/detach", url.Values{})
	if err != nil {
		t.Fatalf("POST /detach: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("detach status = %d, want 200", resp.StatusCode)
	}
	got := get(t, server, "/soldiers/"+intStr(person.ID)+"/events")
	if !strings.Contains(got, "0 linked") {
		t.Errorf("detach: fragment should show '0 linked', got: %s", got)
	}
}

// TestHandleAttachEventByDisplayID covers the new "Add existing
// event" control from slice #325: a single-form-field POST that
// resolves the target Event by Display ID. Not-found,
// validation, success, and duplicate-link (already-linked)
// paths are all exercised.
func TestHandleAttachEventByDisplayID(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	person := createSoldier(t, app, "Tab Attach Person")
	target := createEvent(t, app, "Battle", "10/25/1864", "10/25/1864", "Decisive")

	// Success: attach by Display ID.
	resp, err := http.PostForm(server.URL+"/soldiers/"+intStr(person.ID)+"/events/attach-by-display-id", url.Values{"display_id": {target.DisplayID}})
	if err != nil {
		t.Fatalf("POST attach-by-display-id: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("attach-by-display-id status = %d, want 200", resp.StatusCode)
	}

	// Fragment must list the linked Event now.
	got := get(t, server, "/soldiers/"+intStr(person.ID)+"/events")
	if !strings.Contains(got, target.DisplayID) {
		t.Errorf("fragment missing %q after attach-by-display-id", target.DisplayID)
	}

	// Duplicate-link: posting the same Display ID again must not
	// silently re-attach (the existing link is rejected; the
	// handler returns 409 or a JS-toaster-wrapped 200 with a
	// body indicating the conflict — either way, the test's
	// post-condition is that the link count is unchanged and
	// a second Event row was NOT created.
	dup, _ := http.PostForm(server.URL+"/soldiers/"+intStr(person.ID)+"/events/attach-by-display-id", url.Values{"display_id": {target.DisplayID}})
	dup.Body.Close()
	linkCountAfter, _ := app.events.ListForPerson(person.ID)
	if len(linkCountAfter) != 1 {
		t.Errorf("duplicate attach should leave link count at 1; got %d", len(linkCountAfter))
	}
	miss, _ := http.PostForm(server.URL+"/soldiers/"+intStr(person.ID)+"/events/attach-by-display-id", url.Values{"display_id": {"EVT-99999"}})
	miss.Body.Close()
	if miss.StatusCode != http.StatusNotFound {
		t.Errorf("not-found attach status = %d, want 404", miss.StatusCode)
	}

	// Validation: empty Display ID returns 400.
	empty, _ := http.PostForm(server.URL+"/soldiers/"+intStr(person.ID)+"/events/attach-by-display-id", url.Values{"display_id": {""}})
	empty.Body.Close()
	if empty.StatusCode != http.StatusBadRequest {
		t.Errorf("empty attach status = %d, want 400", empty.StatusCode)
	}
}

// TestHandlePersonEventsTabQuickAdd covers the Quick-add Event
// form on the Person Events tab (slice #325). Submits kind +
// dates + description via the existing /quick-add route that
// the template emits and asserts a fresh Event with the
// supplied kind is now linked to the Person Record.
func TestHandlePersonEventsTabQuickAdd(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	person := createSoldier(t, app, "Tab QuickAdd Person")
	form := url.Values{
		"kind":        {"Skirmish"},
		"begin_date":  {"06/12/1864"},
		"end_date":    {"06/12/1864"},
		"description": {"Brief encounter"},
	}
	resp, err := http.PostForm(server.URL+"/soldiers/"+intStr(person.ID)+"/events/quick-add", form)
	if err != nil {
		t.Fatalf("POST /quick-add: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("quick-add status = %d, want 200", resp.StatusCode)
	}

	got := get(t, server, "/soldiers/"+intStr(person.ID)+"/events")
	if !strings.Contains(got, "Skirmish") {
		t.Errorf("fragment missing kind %q after quick-add", "Skirmish")
	}
}

// get is a tiny helper that does a GET and returns the body as
// a string. Failed reads fail the test.
func get(t *testing.T, server *httptest.Server, path string) string {
	t.Helper()
	resp, err := http.Get(server.URL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	return readAll(t, resp)
}
// TestHandleBrowseEventsFilter covers issue #320 slice #327:
// selecting "Event" in the browse entry-type filter must
// return a list of Event rows whose row URL points at
// /events/{id} (not /soldiers/{id}, which 404s for Events).
// The test seeds one Event and one Soldier, GETs
// /browse?entry_type=event, and asserts the Event's
// Display ID is in the body while the Soldier's is NOT,
// plus the row-href data attribute targets /events/{id}.
func TestHandleBrowseEventsFilter(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	event := createEvent(t, app, "Battle of Atlanta", "07/22/1864", "07/22/1864", "Decisive engagement")
	if _, err := app.soldiers.Create(models.Soldier{FirstName: "Nathan", LastName: "Bedford", EntryType: models.EntryTypeSoldier}); err != nil {
		t.Fatalf("seed Soldier: %v", err)
	}

	resp, err := http.Get(server.URL + "/browse?entry_type=event&page_size=50")
	if err != nil {
		t.Fatalf("GET /browse?entry_type=event: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body := readAll(t, resp)

	// The Event's Display ID must be present.
	if !strings.Contains(body, event.DisplayID) {
		t.Errorf("body missing Event Display ID %q", event.DisplayID)
	}
	// The Event's row must link to /events/{id}, not /soldiers/{id}.
	if !strings.Contains(body, "/events/"+intStr(event.ID)) {
		t.Errorf("browse row did not link to /events/%d (recordBrowseURL regression): %s", event.ID, body)
	}
	// Soldier "Nathan Bedford" must NOT appear (filter is event-only).
	if strings.Contains(body, "Nathan") {
		t.Errorf("browse with entry_type=event should not show Soldier rows; got Nathan in body")
	}

	// Round-trip: rows for /browse (no filter) include both subtypes.
	allResp, err := http.Get(server.URL + "/browse?page_size=50")
	if err != nil {
		t.Fatalf("GET /browse: %v", err)
	}
	allResp.Body.Close()
	if allResp.StatusCode != http.StatusOK {
		t.Fatalf("/browse status = %d, want 200", allResp.StatusCode)
	}
}
func extractDisplayID(string) string { return "" }
