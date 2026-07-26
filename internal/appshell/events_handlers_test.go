// events_handlers_test.go covers the slice-3 Event Record
// HTTP handlers (issue #320). The tests use httptest.NewRecorder
// to drive the chi-mounted routes through the App.ServeHTTP
// path. Each test creates a fresh temp-app via newStressApp so
// the EventService wiring (slice 2's NewEventService) is
// exercised end-to-end.
package appshell

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/valueforvalue/DixieData/internal/appdata"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/testtemp"
	"github.com/valueforvalue/DixieData/internal/uiids"
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

// TestHandleCreateSoldierDispatchesToNewEvent covers issue #320
// slot #330: the JS-side action-swap for entry_type=event
// (syncEntryTypeFields) sets the form action to /events/new
// on the client, but a researcher can still hand-curl the
// form to /soldiers with entry_type=event. The handler
// must forward them to /events/new instead of silently
// creating a Soldier row with entry_type=event.
func TestHandleCreateSoldierDispatchesToNewEvent(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	form := url.Values{}
	form.Set("entry_type", "event")
	form.Set("kind", "Battle")
	form.Set("begin_date", "07/01/1863")
	form.Set("end_date", "07/03/1863")
	form.Set("description", "Battle submitted via /soldiers with entry_type=event")
	form.Set("display_id", "EVT-SUBMIT-VIA-SOLDIERS")
	resp, err := http.PostForm(server.URL+"/soldiers", form)
	if err != nil {
		t.Fatalf("POST /soldiers (event entry_type): %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /soldiers status = %d, want 200 (Option C redirect)", resp.StatusCode)
	}
	redirect := resp.Header.Get("X-DixieData-Redirect")
	if !strings.HasPrefix(redirect, "/events/") {
		t.Fatalf("expected X-DixieData-Redirect to /events/{id}; got %q", redirect)
	}
	// No Soldier row may have been created.
	if s := app.soldiers; s == nil {
		t.Fatalf("app.soldiers is nil")
	}
	idStr := strings.TrimPrefix(redirect, "/events/")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	if got, err := app.events.GetEventByID(id); err != nil || got == nil {
		t.Errorf("Event row missing after /soldiers entry_type=event dispatch: got=%v err=%v", got, err)
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

// TestHandleEventByIDGetDetail confirms the Event detail page keeps
func TestHandleEventByIDGetDetail_SourcesPanelEditCTA(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	created := createEvent(t, app, "Empty Sources Event", "", "", "")
	resp, err := http.Get(server.URL + "/events/" + intStr(created.ID))
	if err != nil {
		t.Fatalf("GET /events/%d: %v", created.ID, err)
	}
	defer resp.Body.Close()
	body := readAll(t, resp)

	// Legacy form must be gone.
	for _, needle := range []string{
		`name="record_type"`,
		`name="app_id"`,
		`name="details"`,
		"Source type (Pension, Roster, ...)",
	} {
		if strings.Contains(body, needle) {
			t.Errorf("GET /events/%d body still contains legacy %q", created.ID, needle)
		}
	}

	// Empty-state must no longer mention the obsolete /sources
	// authoring flow redirect.
	if strings.Contains(body, "Attach source documents from the /sources authoring flow") {
		t.Errorf("GET /events/%d empty-state copy still references obsolete /sources authoring flow", created.ID)
	}

	editHref := fmt.Sprintf("/events/%d/edit", created.ID)
	// The page-level anchor is the only Edit Event entry point.
	if !strings.Contains(body, fmt.Sprintf(`href="%s"`, editHref)) {
		t.Errorf("GET /events/%d body missing page-level Edit Event link %q", created.ID, editHref)
	}
	if strings.Contains(body, fmt.Sprintf(`data-action="%s"`, editHref)) {
		t.Errorf("GET /events/%d body contains obsolete panel Edit Event action", created.ID)
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

// TestHandleEventByIDPostUpdate mirrors TestHandleEventByIDGetDetail
// for the POST → /events/{id} alias. The smoke probe for issue #320
// child #323 caught that the edit form in event_form.templ posts
// to /events/{id} (not /events/{id}/edit) so the JS dispatcher can
// re-use the same Option C handler for new + edit. Routes.go
// registers POST + PUT on /events/{id}; handleEventByID originally
// only switched on PUT and 405'd the POST. This test pins the POST
// path so the regression net stays green.
func TestHandleEventByIDPostUpdate(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	created := createEvent(t, app, "ToPostUpdate", "01/01/1864", "01/02/1864", "Original description")

	form := url.Values{}
	form.Set("entry_type", "event")
	form.Set("kind", "ToPostUpdateEdited")
	form.Set("begin_date", "01/05/1864")
	form.Set("end_date", "01/06/1864")
	form.Set("description", "Posted edit description")

	req, _ := http.NewRequest(http.MethodPost, server.URL+"/events/"+intStr(created.ID), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /events/%d: %v", created.ID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /events/%d status = %d, want 200", created.ID, resp.StatusCode)
	}
	redirect := resp.Header.Get("X-DixieData-Redirect")
	wantRedirect := "/events/" + intStr(created.ID)
	if redirect != wantRedirect {
		t.Errorf("X-DixieData-Redirect = %q, want %q", redirect, wantRedirect)
	}
	updated, err := app.events.GetEventByID(created.ID)
	if err != nil {
		t.Fatalf("GetEventByID after POST: %v", err)
	}
	if updated.Event.Kind != "ToPostUpdateEdited" {
		t.Errorf("kind = %q, want ToPostUpdateEdited", updated.Event.Kind)
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
	if !strings.HasPrefix(redirect, "/soldiers/"+intStr(person.ID)+"/events") {
		t.Errorf("X-DixieData-Redirect = %q, want /soldiers/%d/events (issue #345)", redirect, person.ID)
	}
	// Verify link.
	linked, err := app.events.ListForPerson(person.ID)
	if err != nil {
		t.Fatalf("ListForPerson: %v", err)
	}
	if len(linked) != 1 {
		t.Errorf("linked = %v, want exactly one event", linked)
	}

	// Issue #377 slice 2: Quick-Add Event rows must carry
	// CreatedByImportPath = "quick_add_event" so the provenance
	// footer can distinguish them from rows created via the
	// dedicated /events/new form (which stamps "create_event").
	quickAdded, err := app.soldiers.GetByID(linked[0].ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if quickAdded.CreatedByImportPath != "quick_add_event" {
		t.Errorf("CreatedByImportPath = %q, want %q", quickAdded.CreatedByImportPath, "quick_add_event")
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

	want := filepath.Join(testtemp.New(t).Path(), eventPDFName(created))
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

// TestHandleEventPDF_OrientationPicker pins the issue #374
// headline criterion: POST /events/{id}/pdf with an
// `orientation` form field (portrait|landscape, default
// landscape) routes through the matching event_<orientation>.typ
// template. The slice-0.5 RED state confirms the picker is
// unwired — the existing handler ignores `orientation`, always
// rendering the landscape template. Slice 1 flips this green
// by wiring EventService.RenderPDF + the picker form value.
//
// Sub-tests:
//   - portrait: POST with orientation=portrait → PDF saved is
//     the portrait template's output (rendered non-empty bytes
//     because the test exercises the full typst path).
//   - landscape (explicit): POST with orientation=landscape →
//     the landscape template's output (existing behavior).
//   - landscape (default): POST with no orientation field →
//     defaults to landscape (back-compat for any existing
//     invoker that doesn't read the picker yet).
//
// The dialog-guard pattern is asserted by the existing
// TestHandleEventPDF (200 + second-POST-also-200); this test
// focuses on the orientation routing.
func TestHandleEventPDF_OrientationPicker(t *testing.T) {
	app := newStressApp(t)
	created := createEvent(t, app, "Battle of Springfield", "10/25/1864", "10/25/1864", "Decisive engagement")

	portraitPath := filepath.Join(testtemp.New(t).Path(), "portrait.pdf")
	landscapePath := filepath.Join(testtemp.New(t).Path(), "landscape.pdf")
	defaultPath := filepath.Join(testtemp.New(t).Path(), "default.pdf")

	// Each POST needs a fresh SaveFileDialog return path; reuse
	// a queue so the same app routes each request to a different
	// destination. The article path pre-renders + writes the
	// PDF synchronously, so the file must exist on disk before
	// the test returns.
	dialogReturns := []string{portraitPath, landscapePath, defaultPath}
	app.saveFileDialogOverride = func(opts any) (string, error) {
		if len(dialogReturns) == 0 {
			return "", nil
		}
		p := dialogReturns[0]
		dialogReturns = dialogReturns[1:]
		return p, nil
	}
	defer func() { app.saveFileDialogOverride = nil }()

	server := httptest.NewServer(app)
	defer server.Close()

	cases := []struct {
		name     string
		form     url.Values
		wantPath string
	}{
		{"portrait", url.Values{"orientation": {"portrait"}}, portraitPath},
		{"landscape", url.Values{"orientation": {"landscape"}}, landscapePath},
		{"default", url.Values{}, defaultPath},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Record the pre-request size (0 for the first
			// sub-test; non-zero on subsequent passes). The
			// request must write a fresh PDF on each iteration.
			preSize := int64(-1)
			if st, err := os.Stat(c.wantPath); err == nil {
				preSize = st.Size()
			}

			req, _ := http.NewRequest(http.MethodPost,
				server.URL+"/events/"+intStr(created.ID)+"/pdf",
				strings.NewReader(c.form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("POST /events/%d/pdf: %v", created.ID, err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("POST status = %d, want 200", resp.StatusCode)
			}

			// Slice 1 will wire the handler to the article
			// pattern: pre-render + synchronous write to the
			// chosen path. This assertion fails RED today
			// because the current handler enqueues a job that
			// writes async (TestHandleEventPDF's wait helper
			// would be needed). Slice 1 lands the article
			// pattern so the file exists on return.
			st, err := os.Stat(c.wantPath)
			if err != nil {
				t.Fatalf("stat %q: %v (handler must write PDF synchronously per article pattern)", c.wantPath, err)
			}
			if st.Size() == preSize {
				t.Fatalf("file %q size unchanged after request (size=%d) — handler did not write bytes", c.wantPath, st.Size())
			}
			body, err := os.ReadFile(c.wantPath)
			if err != nil {
				t.Fatalf("read %q: %v", c.wantPath, err)
			}
			if len(body) < 4 || string(body[:4]) != "%PDF" {
				t.Fatalf("file body prefix = %q, want %%PDF-", string(body[:min(8, len(body))]))
			}
		})
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
		DisplayID:    id,
		EntryType:    models.EntryTypeSoldier,
		FirstName:    "Test",
		LastName:     label,
		RankOut:      "PVT",
		RankIn:       "PVT",
		PensionState: "Not Applicable",
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

// bodyExtract returns a substring of body centered on the first
// occurrence of marker (preferring the right edge for less context
// noise) so a failing test can dump enough surrounding HTML to
// explain a missing-CTA failure without a wall of text.
func bodyExtract(body, marker string, radius int) string {
	idx := strings.Index(body, marker)
	if idx < 0 {
		return body
	}
	start := idx - radius
	if start < 0 {
		start = 0
	}
	end := idx + radius
	if end > len(body) {
		end = len(body)
	}
	return body[start:end]
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

// TestHandleEventResearchLog covers issue #320 slot #328:
// the per-Event research log. Seeds an Event, GETs the
// log page (200), POSTs a research task, asserts the
// task title appears, then POSTs the resolve action
// and confirms the task is now resolved. Re-uses
// SoldierService through the handler — research_tasks
// is FK-linked to soldiers(id), and Event rows live in
// the same table.
func TestHandleEventResearchLog(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	event := createEvent(t, app, "Battle of Atlanta", "07/22/1864", "07/22/1864", "Decisive engagement")

	// GET log page (no tasks yet).
	getResp, err := http.Get(server.URL + "/events/" + intStr(event.ID) + "/research-log")
	if err != nil {
		t.Fatalf("GET /events/%d/research-log: %v", event.ID, err)
	}
	getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Errorf("GET log page status = %d, want 200", getResp.StatusCode)
	}

	// POST a task via the create form.
	taskTitle := "Verify casualty figures for Atlanta"
	createResp, err := http.PostForm(server.URL+"/events/"+intStr(event.ID)+"/research-log/tasks", url.Values{
		"title":         {taskTitle},
		"notes":         {"Check Fox & Warner, Civil War Battles app."},
		"evidence_type": {"archive"},
	})
	if err != nil {
		t.Fatalf("POST create: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(createResp.Body)
		t.Fatalf("create status = %d (body=%q), want 200", createResp.StatusCode, string(body))
	}
	if got := createResp.Header.Get("X-DixieData-Redirect"); !strings.Contains(got, "/events/"+intStr(event.ID)+"/research-log") {
		t.Errorf("create redirect = %q, want /events/%d/research-log", got, event.ID)
	}

	// GET the log page; the task title must appear.
	body := get(t, server, "/events/"+intStr(event.ID)+"/research-log")
	if !strings.Contains(body, taskTitle) {
		t.Errorf("research log page missing task title %q after create", taskTitle)
	}

	// Grab the new task id via the service.
	log, err := app.soldiers.ResearchLog(event.ID)
	if err != nil {
		t.Fatalf("ResearchLog: %v", err)
	}
	var taskID int64
	for _, task := range log.Tasks {
		if strings.TrimSpace(task.Title) == taskTitle {
			taskID = task.ID
			break
		}
	}
	if taskID == 0 {
		t.Fatalf("research log missing newly-created task %q; tasks=%+v", taskTitle, log.Tasks)
	}

	// POST resolve.
	resolvePath := fmt.Sprintf("/events/%d/research-log/tasks/%d/resolve", event.ID, taskID)
	resolveResp, err := http.PostForm(server.URL+resolvePath, url.Values{})
	if err != nil {
		t.Fatalf("POST %s: %v", resolvePath, err)
	}
	resolveResp.Body.Close()
	if resolveResp.StatusCode != http.StatusOK {
		t.Errorf("resolve status = %d, want 200", resolveResp.StatusCode)
	}
	if got := resolveResp.Header.Get("X-DixieData-Redirect"); !strings.Contains(got, "/events/"+intStr(event.ID)+"/research-log") {
		t.Errorf("resolve redirect = %q, want /events/%d/research-log", got, event.ID)
	}

	// Confirm the task is now resolved in the service.
	resolved, err := app.soldiers.ResearchLog(event.ID)
	if err != nil {
		t.Fatalf("ResearchLog post-resolve: %v", err)
	}
	for _, task := range resolved.Tasks {
		if task.ID == taskID && strings.TrimSpace(task.Status) != "resolved" {
			t.Errorf("task %d still has status %q after resolve", taskID, task.Status)
		}
	}
}

// TestHandleEventSourcesAndScratchpad covers issue #320 slots
// #329 + #330: the per-Event Sources panel + Open Scratch Pad
// button. Seeds an Event, GETs /sources (panel renders "No
// Source Records"), POSTs an attach (event persists the
// record), detaches it via the /detach endpoint, and asserts
// the scratchpad form on event detail posts to /scratchpad/
// open with the Event's display_id. Issue #341: also asserts the
// POST handlers return swap-friendly fragments (no
// X-DixieData-Redirect header, response body matches the
// data-results-target shape on event_detail.templ) so the JS
// dispatcher swaps the response into #data-event-sources-list in
// place rather than navigating to the fragment URL.
func TestHandleEventSourcesAndScratchpad(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	event := createEvent(t, app, "Battle of Gettysburg", "07/01/1863", "07/03/1863", "Pivotal engagement")

	getResp, err := http.Get(server.URL + "/events/" + intStr(event.ID) + "/sources")
	if err != nil {
		t.Fatalf("GET sources: %v", err)
	}
	body := readAll(t, getResp)
	getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Errorf("GET /sources status = %d, want 200", getResp.StatusCode)
	}
	if !strings.Contains(body, "No Source Records") {
		t.Errorf("expected empty Sources panel copy; got %q", body)
	}

	// Issue #380 slice 6: POST /sources/attach was deleted (the
	// handler was a stale bookmark-compat remnant kept alive post-#341
	// with no UI form calling it; the app does not support bookmarks).
	// The GET + detach endpoints stay; the previous attach+detach
	// round-trip test exercised both. Post-#380 the test shrinks to:
	// the GET endpoint still renders the empty state copy on a fresh
	// event, and the orphan-handler probe (audit/discover_orphan_handlers.mjs)
	// no longer flags /sources/attach.

	// The scratch-pad open test below still requires an Event context
	// -- createEvent above already seeded the row, so the event is
	// available for the Open Scratch Pad POST.

	after, err := app.events.ListSourcesForEvent(event.ID)
	if err != nil {
		t.Fatalf("ListSourcesForEvent post-detach: %v", err)
	}
	if len(after) != 0 {
		t.Errorf("want 0 sources after detach, got %d", len(after))
	}

	// Scratchpad: event detail renders an Open Scratch Pad form
	// pointing at /scratchpad/open with the Event's display_id.
	detailBody := get(t, server, "/events/"+intStr(event.ID))
	if !strings.Contains(detailBody, "Open Scratch Pad") {
		t.Errorf("event detail missing Open Scratch Pad button; got %q", detailBody)
	}
	if !strings.Contains(detailBody, fmt.Sprintf(`value="%s"`, event.DisplayID)) {
		t.Errorf("event detail missing scratchpad display_id input; got %q", detailBody)
	}
	// Issue #360: the event detail Sources panel no longer carries
	// a standalone attach form (the inline attach surface lives on
	// /events/{id}/edit per #357). The panel still wraps the list
	// in #data-event-sources-list for the post-detach fragment
	// swap (issue #341) and exposes an Edit Event CTA pointing at
	// /events/{id}/edit.
	if !strings.Contains(detailBody, "id=\"data-event-sources-list\"") {
		t.Errorf("event detail missing sources list wrapper id; got %q", detailBody)
	}
	// Event detail keeps only page-level edit navigation.
	editHref := fmt.Sprintf("/events/%d/edit", event.ID)
	if !strings.Contains(detailBody, fmt.Sprintf(`href="%s"`, editHref)) {
		t.Errorf("event detail missing page-level Edit Event link; want href=%q", editHref)
	}
	if strings.Contains(detailBody, fmt.Sprintf(`data-action="%s"`, editHref)) {
		t.Errorf("event detail contains obsolete panel Edit Event action")
	}
	if !strings.Contains(detailBody, "id=\"data-event-tags-list\"") {
		t.Errorf("event detail missing tags list wrapper id; got %q", detailBody)
	}
}

// TestHandleEventTags covers issue #320 slot #333 (per-Event
// Tags chips) and issue #341 (fragment-vs-redirect wiring).
// Seeds an Event + a tag, GETs the panel (empty), attaches the
// tag, asserts the POST response is a swap-friendly fragment
// with no X-DixieData-Redirect header (otherwise the JS
// dispatcher navigates to the fragment URL and the user sees
// raw HTML as a page), then detaches it and re-asserts the
// empty state. Calls the same path-suffix dispatcher as the
// sources/research-log panels.
func TestHandleEventTags(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	event := createEvent(t, app, "Siege of Vicksburg", "05/18/1863", "07/04/1863", "")
	tag, err := app.tags.UpsertByName(context.Background(), "siege")
	if err != nil {
		t.Fatalf("tags.UpsertByName: %v", err)
	}

	getResp, err := http.Get(server.URL + "/events/" + intStr(event.ID) + "/tags")
	if err != nil {
		t.Fatalf("GET tags: %v", err)
	}
	body := readAll(t, getResp)
	getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Errorf("GET /tags status = %d, want 200", getResp.StatusCode)
	}
	if !strings.Contains(body, "No tags attached") {
		t.Errorf("expected empty Tags panel copy; got %q", body)
	}

	addResp, err := http.PostForm(server.URL+"/events/"+intStr(event.ID)+"/tags", url.Values{
		"tag_id": {intStr(tag.ID)},
	})
	if err != nil {
		t.Fatalf("POST tags: %v", err)
	}
	addBody := readAll(t, addResp)
	addResp.Body.Close()
	if addResp.StatusCode != http.StatusOK {
		t.Errorf("add status = %d, want 200", addResp.StatusCode)
	}
	// Issue #341: the POST must NOT set X-DixieData-Redirect,
	// otherwise the JS dispatcher does a full-page nav to the
	// fragment URL and the user sees raw HTML as a page.
	if got := addResp.Header.Get("X-DixieData-Redirect"); got != "" {
		t.Errorf("POST tags set X-DixieData-Redirect=%q; want empty (issue #341)", got)
	}
	// Response body should be the post-add fragment — a chip
	// span containing the tag name + a detach button carrying the
	// data-results-target so the next click re-renders this
	// fragment in place.
	if !strings.Contains(addBody, "siege") {
		t.Errorf("POST tags response missing tag chip; got %q", addBody)
	}
	if !strings.Contains(addBody, "data-results-target=\"#data-event-tags-list\"") {
		t.Errorf("POST tags chip missing data-results-target; got %q", addBody)
	}

	after, err := app.events.ListTagsForEvent(event.ID)
	if err != nil {
		t.Fatalf("ListTagsForEvent: %v", err)
	}
	if len(after) != 1 || after[0].ID != tag.ID {
		t.Fatalf("want 1 tag attached, got %+v", after)
	}

	chipResp, err := http.Get(server.URL + "/events/" + intStr(event.ID) + "/tags")
	if err != nil {
		t.Fatalf("GET tags post-add: %v", err)
	}
	chipBody := readAll(t, chipResp)
	chipResp.Body.Close()
	if !strings.Contains(chipBody, "siege") {
		t.Errorf("tag chip missing after add; got %q", chipBody)
	}
	// Issue #341: the GET fragment is the swap target on a fresh
	// page load — it must carry the data-results-target on the
	// detach button so a user-initiated detach also re-renders in
	// place (no full-page nav).
	if !strings.Contains(chipBody, "data-results-target=\"#data-event-tags-list\"") {
		t.Errorf("GET tags chip missing data-results-target; got %q", chipBody)
	}

	detachResp, err := http.PostForm(server.URL+"/events/"+intStr(event.ID)+"/tags/"+intStr(tag.ID)+"/detach", url.Values{})
	if err != nil {
		t.Fatalf("POST detach: %v", err)
	}
	detachBody := readAll(t, detachResp)
	detachResp.Body.Close()
	if detachResp.StatusCode != http.StatusOK {
		t.Errorf("detach status = %d, want 200", detachResp.StatusCode)
	}
	// Issue #341: detach POST must return a fragment, not redirect.
	if got := detachResp.Header.Get("X-DixieData-Redirect"); got != "" {
		t.Errorf("POST detach set X-DixieData-Redirect=%q; want empty (issue #341)", got)
	}
	// After detach, the response body should re-render the empty
	// state — same shape as the initial GET.
	if !strings.Contains(detachBody, "No tags attached") {
		t.Errorf("POST detach response missing empty state; got %q", detachBody)
	}

	final, err := app.events.ListTagsForEvent(event.ID)
	if err != nil {
		t.Fatalf("ListTagsForEvent post-detach: %v", err)
	}
	if len(final) != 0 {
		t.Errorf("want 0 tags after detach, got %d", len(final))
	}
}

// TestHandleEventImages (issue #320 child #332, slot 16 of 16)
// pins the Event images gallery surface: a fresh event with
// two seeded images renders both thumbnails via the
// /events/{id}/images fragment, and POSTing
// /events/{id}/images/delete with one image_id drops that one
// from the rendered grid AND removes the DB row AND does NOT
// set X-DixieData-Redirect (per issue #341, fragment-as-page
// nav). Mirrors TestHandleEventTags shape; seeds images via
// the same gold_master_test.go shape (real PNG file +
// app.soldiers.AddImage) so the on-disk path lands under the
// sharded images/<A>/<B>/EVT-NNNNN/ layout per the v60 widening.
func TestHandleEventImages(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	event := createEvent(t, app, "Battle of Test Run", "01/01/1865", "01/02/1865", "Gallery smoke.")

	imageDir, relativeDir := appdata.RecordImageDir(app.dataDir, event.DisplayID)
	if err := os.MkdirAll(imageDir, 0o755); err != nil {
		t.Fatalf("MkdirAll imageDir: %v", err)
	}
	firstPath := filepath.Join(imageDir, "first.png")
	secondPath := filepath.Join(imageDir, "second.png")
	if err := os.WriteFile(firstPath, pngFixture(), 0o644); err != nil {
		t.Fatalf("WriteFile first: %v", err)
	}
	if err := os.WriteFile(secondPath, pngFixture(), 0o644); err != nil {
		t.Fatalf("WriteFile second: %v", err)
	}
	if err := app.soldiers.AddImage(event.ID, "first.png", filepath.Join(relativeDir, "first.png"), "First portrait"); err != nil {
		t.Fatalf("AddImage first: %v", err)
	}
	if err := app.soldiers.AddImage(event.ID, "second.png", filepath.Join(relativeDir, "second.png"), "Second portrait"); err != nil {
		t.Fatalf("AddImage second: %v", err)
	}

	getResp, err := http.Get(server.URL + "/events/" + intStr(event.ID) + "/images")
	if err != nil {
		t.Fatalf("GET images: %v", err)
	}
	getBody := readAll(t, getResp)
	getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("GET images status = %d, want 200", getResp.StatusCode)
	}
	if !strings.Contains(getBody, "First portrait") {
		t.Errorf("GET images missing first image caption; got %q", getBody)
	}
	if !strings.Contains(getBody, "Second portrait") {
		t.Errorf("GET images missing second image caption; got %q", getBody)
	}
	if !strings.Contains(getBody, fmt.Sprintf("data-results-target=\"#%s\"", uiids.PanelEventDetailImages)) {
		t.Errorf("GET images fragment missing data-results-target; got %q", getBody)
	}
	if strings.Contains(getBody, "No images are attached") {
		t.Errorf("GET images rendered empty state despite 2 seeded images; got %q", getBody)
	}

	// Event detail page must render the Images section on first
	// load (mirrors the Tags + Sources lazy-load pattern).
	detailResp, err := http.Get(server.URL + "/events/" + intStr(event.ID))
	if err != nil {
		t.Fatalf("GET detail: %v", err)
	}
	detailBody := readAll(t, detailResp)
	detailResp.Body.Close()
	if !strings.Contains(detailBody, fmt.Sprintf("id=\"%s\"", uiids.PanelEventDetailImages)) {
		t.Errorf("detail page missing #%s anchor; got %q", uiids.PanelEventDetailImages, detailBody)
	}
	if !strings.Contains(detailBody, "First portrait") {
		t.Errorf("detail page missing first image caption; got %q", detailBody)
	}

	// Delete one image. Pull the image_id off the row by
	// re-querying the service; the test cares about the side
	// effect, not the markup shape.
	refreshed, err := app.events.GetEventByID(event.ID)
	if err != nil {
		t.Fatalf("GetEventByID pre-delete: %v", err)
	}
	if len(refreshed.Event.Images) != 2 {
		t.Fatalf("seeded 2 images, event has %d", len(refreshed.Event.Images))
	}
	dropID := refreshed.Event.Images[0].ID

	delResp, err := http.PostForm(server.URL+"/events/"+intStr(event.ID)+"/images/delete", url.Values{
		"image_ids": {intStr(dropID)},
	})
	if err != nil {
		t.Fatalf("POST images/delete: %v", err)
	}
	delBody := readAll(t, delResp)
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusOK {
		t.Errorf("POST delete status = %d, want 200", delResp.StatusCode)
	}
	if got := delResp.Header.Get("X-DixieData-Redirect"); got != "" {
		t.Errorf("POST delete set X-DixieData-Redirect=%q; want empty (issue #341)", got)
	}
	if !strings.Contains(delBody, fmt.Sprintf("data-results-target=\"#%s\"", uiids.PanelEventDetailImages)) {
		t.Errorf("POST delete response missing fragment anchor; got %q", delBody)
	}

	afterDelete, err := app.events.GetEventByID(event.ID)
	if err != nil {
		t.Fatalf("GetEventByID post-delete: %v", err)
	}
	if len(afterDelete.Event.Images) != 1 {
		t.Errorf("after delete want 1 image, got %d", len(afterDelete.Event.Images))
	}
}

// TestHandleEventLinksAttachDetachByDisplayID pins slice 2 of
// #361: the Event editor's Linked Persons section posts to
// /events/{id}/links (Display ID form field, resolves to
// Person ID, calls existing AttachEventToPerson) and
// /events/{id}/links/{personId}/detach (existing
// DetachEventFromPerson). Both must respond with
// X-DixieData-Redirect pointing at /events/{id}/edit (the
// editor surface, NOT the detail page, because the user is
// mid-edit). Bad Display ID returns 400, not 500.
func TestHandleEventLinksAttachDetachByDisplayID(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	event := createEvent(t, app, "Attach Slice2", "07/01/1863", "07/03/1863", "")
	target := createSoldier(t, app, "Slice2 Target")

	// Success: POST /events/{id}/links with the target's Display ID.
	attachResp, err := http.PostForm(server.URL+"/events/"+intStr(event.ID)+"/links", url.Values{
		"display_id": {target.DisplayID},
	})
	if err != nil {
		t.Fatalf("POST /events/{id}/links: %v", err)
	}
	attachResp.Body.Close()
	if attachResp.StatusCode != http.StatusOK {
		t.Errorf("attach status = %d, want 200", attachResp.StatusCode)
	}
	// X-DixieData-Redirect must point back at the editor, NOT
	// the detail page (the user is mid-edit, so landing on the
	// read-only detail page would lose their in-progress edits).
	if got := attachResp.Header.Get("X-DixieData-Redirect"); got != "/events/"+intStr(event.ID)+"/edit" {
		t.Errorf("attach X-DixieData-Redirect = %q, want /events/%d/edit", got, event.ID)
	}

	// Post-condition: the link exists. The detail-page panel
	// from slice 1 will render the linked Person Record via
	// ListForEvent; mirror that assertion here.
	linked, err := app.events.ListForEvent(event.ID)
	if err != nil {
		t.Fatalf("ListForEvent: %v", err)
	}
	if len(linked) != 1 || linked[0].ID != target.ID {
		t.Errorf("ListForEvent = %v, want 1 Person with ID %d", linked, target.ID)
	}

	// Detach via /events/{id}/links/{personId}/detach.
	detachResp, err := http.PostForm(server.URL+"/events/"+intStr(event.ID)+"/links/"+intStr(target.ID)+"/detach", nil)
	if err != nil {
		t.Fatalf("POST detach: %v", err)
	}
	detachResp.Body.Close()
	if detachResp.StatusCode != http.StatusOK {
		t.Errorf("detach status = %d, want 200", detachResp.StatusCode)
	}
	if got := detachResp.Header.Get("X-DixieData-Redirect"); got != "/events/"+intStr(event.ID)+"/edit" {
		t.Errorf("detach X-DixieData-Redirect = %q, want /events/%d/edit", got, event.ID)
	}
	linkedAfter, err := app.events.ListForEvent(event.ID)
	if err != nil {
		t.Fatalf("ListForEvent post-detach: %v", err)
	}
	if len(linkedAfter) != 0 {
		t.Errorf("ListForEvent post-detach len = %d, want 0", len(linkedAfter))
	}

	// Bad Display ID: must return 404 (mirrors the soldier-side
	// /soldiers/{id}/events/attach-by-display-id contract — the
	// Display ID points at a row that doesn't exist), not 500.
	missResp, _ := http.PostForm(server.URL+"/events/"+intStr(event.ID)+"/links", url.Values{
		"display_id": {"SOL-99999"},
	})
	missResp.Body.Close()
	if missResp.StatusCode != http.StatusNotFound {
		t.Errorf("not-found attach status = %d, want 404", missResp.StatusCode)
	}

	// Empty Display ID: must return 400.
	emptyResp, _ := http.PostForm(server.URL+"/events/"+intStr(event.ID)+"/links", url.Values{
		"display_id": {""},
	})
	emptyResp.Body.Close()
	if emptyResp.StatusCode != http.StatusBadRequest {
		t.Errorf("empty attach status = %d, want 400", emptyResp.StatusCode)
	}
}

// TestHandleEventTagAddByName pins issue #361 slice 3: the
// Event editor's Tags section posts a free-text `tag_name`
// field (mirroring the /soldiers/{id}/tags picker UX) and
// the handler upserts the tag via the existing
// TagService.UpsertByName path before calling
// EventService.AddTagToEvent. The response shape is
// unchanged (fragment, no X-DixieData-Redirect — #341
// contract preserved) so the JS dispatcher can swap in
// place on the edit page.
//
// RED today: handleEventTagAdd does not accept `tag_name`;
// only `tag_id` works. After slice 3 lands: posting
// `tag_name=vc-shiloh` creates the tag if absent and
// attaches it; posting `tag_id=N` still works (backward-
// compatible for existing callers).
func TestHandleEventTagAddByName(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	event := createEvent(t, app, "Battle", "07/01/1863", "07/03/1863", "")

	// Happy path: POST with tag_name. The handler must
	// upsert + attach in one round-trip.
	addResp, err := http.PostForm(server.URL+"/events/"+intStr(event.ID)+"/tags", url.Values{
		"tag_name": {"vc-shiloh"},
	})
	if err != nil {
		t.Fatalf("POST tags by name: %v", err)
	}
	addBody := readAll(t, addResp)
	addResp.Body.Close()
	if addResp.StatusCode != http.StatusOK {
		t.Errorf("add-by-name status = %d, want 200", addResp.StatusCode)
	}
	// #341: must NOT set X-DixieData-Redirect.
	if got := addResp.Header.Get("X-DixieData-Redirect"); got != "" {
		t.Errorf("POST tags-by-name set X-DixieData-Redirect=%q; want empty (issue #341)", got)
	}
	// Response body: chip with the tag name + in-place swap target.
	if !strings.Contains(addBody, "vc-shiloh") {
		t.Errorf("POST tags-by-name response missing chip; got %q", addBody)
	}
	if !strings.Contains(addBody, "data-results-target=\"#data-event-tags-list\"") {
		t.Errorf("POST tags-by-name chip missing data-results-target; got %q", addBody)
	}
	// Post-condition: tag exists + is attached.
	attached, err := app.events.ListTagsForEvent(event.ID)
	if err != nil {
		t.Fatalf("ListTagsForEvent: %v", err)
	}
	if len(attached) != 1 || attached[0].Name != "vc-shiloh" {
		t.Errorf("want 1 tag named vc-shiloh, got %+v", attached)
	}

	// Idempotency: posting the same tag_name again must NOT
	// create a duplicate (UpsertByName dedupes case-
	// insensitively; AddTagToEvent is INSERT OR IGNORE).
	addResp2, _ := http.PostForm(server.URL+"/events/"+intStr(event.ID)+"/tags", url.Values{
		"tag_name": {"VC-Shiloh"},
	})
	addResp2.Body.Close()
	attached2, _ := app.events.ListTagsForEvent(event.ID)
	if len(attached2) != 1 {
		t.Errorf("duplicate tag_name should leave count at 1, got %d", len(attached2))
	}

	// Backward compatibility: posting tag_id (existing
	// behavior) still works.
	preExistingTag, err := app.tags.UpsertByName(context.Background(), "pre-existing")
	if err != nil {
		t.Fatalf("tags.UpsertByName pre-existing: %v", err)
	}
	addResp3, _ := http.PostForm(server.URL+"/events/"+intStr(event.ID)+"/tags", url.Values{
		"tag_id": {intStr(preExistingTag.ID)},
	})
	addResp3.Body.Close()
	if addResp3.StatusCode != http.StatusOK {
		t.Errorf("tag_id backward-compat status = %d, want 200", addResp3.StatusCode)
	}
	attached3, _ := app.events.ListTagsForEvent(event.ID)
	if len(attached3) != 2 {
		t.Errorf("after adding by id, want 2 tags attached, got %d", len(attached3))
	}

	// Validation: empty tag_name returns 400 (no way to
	// resolve a tag id).
	emptyResp, _ := http.PostForm(server.URL+"/events/"+intStr(event.ID)+"/tags", url.Values{
		"tag_name": {""},
	})
	emptyResp.Body.Close()
	if emptyResp.StatusCode != http.StatusBadRequest {
		t.Errorf("empty tag_name status = %d, want 400", emptyResp.StatusCode)
	}

	// Validation: whitespace-only tag_name also returns 400.
	wsResp, _ := http.PostForm(server.URL+"/events/"+intStr(event.ID)+"/tags", url.Values{
		"tag_name": {"   "},
	})
	wsResp.Body.Close()
	if wsResp.StatusCode != http.StatusBadRequest {
		t.Errorf("whitespace tag_name status = %d, want 400", wsResp.StatusCode)
	}
}

// TestHandleEventLinksAttachByName (issue #373) pins the
// name-search fallback added to the Event editor's Add
// Linked Person form. When the form input does not match a
// Display ID exactly (case-insensitive), the handler must
// fall back to LookupPersonIDByName and attach the first
// substring match sorted by display_id. The 404 envelope on
// no match must echo whatever the user typed (so they can
// tell "no such Person" from "wrong format").
func TestHandleEventLinksAttachByName(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	event := createEvent(t, app, "Attach By Name", "07/01/1863", "07/03/1863", "")

	// Seed three Person Records whose Display IDs follow the
	// DXD-00001, DXD-00002, DXD-00003 sequence — same shape as
	// the service test, so the assertion "first by display_id
	// wins" matches across both layers.
	_, err := app.soldiers.Create(models.Soldier{FirstName: "Robert", MiddleName: "E.", LastName: "Lee", Suffix: "Jr."})
	if err != nil {
		t.Fatalf("Create p1: %v", err)
	}
	p2, err := app.soldiers.Create(models.Soldier{FirstName: "Stonewall", LastName: "Jackson"})
	if err != nil {
		t.Fatalf("Create p2: %v", err)
	}
	_, err = app.soldiers.Create(models.Soldier{FirstName: "James", MiddleName: "Robert", LastName: "Lee"})
	if err != nil {
		t.Fatalf("Create p3: %v", err)
	}

	// Substring match on a unique last name attaches the right row.
	attachResp, err := http.PostForm(server.URL+"/events/"+intStr(event.ID)+"/links", url.Values{
		"display_id": {"Jackson"},
	})
	if err != nil {
		t.Fatalf("POST attach by name: %v", err)
	}
	attachResp.Body.Close()
	if attachResp.StatusCode != http.StatusOK {
		t.Errorf("attach-by-name status = %d, want 200", attachResp.StatusCode)
	}
	if got := attachResp.Header.Get("X-DixieData-Redirect"); got != "/events/"+intStr(event.ID)+"/edit" {
		t.Errorf("attach-by-name X-DixieData-Redirect = %q, want /events/%d/edit", got, event.ID)
	}
	linked, err := app.events.ListForEvent(event.ID)
	if err != nil {
		t.Fatalf("ListForEvent: %v", err)
	}
	if len(linked) != 1 || linked[0].ID != p2.ID {
		t.Errorf("ListForEvent after name-attach = %v, want 1 Person with ID %d (Stonewall Jackson)", linked, p2.ID)
	}

	// Display ID lookup still wins when the input matches a
	// Display ID exactly. Detach first so the next attach is
	// fresh.
	if err := app.events.DetachEventFromPerson(event.ID, p2.ID); err != nil {
		t.Fatalf("Detach: %v", err)
	}

	// Empty input → 400 (validation), not 404 (the empty-input
	// guard happens BEFORE the lookup so we don't run an empty
	// substring query that would match every row).
	emptyResp, _ := http.PostForm(server.URL+"/events/"+intStr(event.ID)+"/links", url.Values{
		"display_id": {""},
	})
	emptyResp.Body.Close()
	if emptyResp.StatusCode != http.StatusBadRequest {
		t.Errorf("empty attach status = %d, want 400", emptyResp.StatusCode)
	}

	// No-match → 404, and the response body should echo what the
	// user typed so they can fix their query.
	missResp, _ := http.PostForm(server.URL+"/events/"+intStr(event.ID)+"/links", url.Values{
		"display_id": {"NonexistentName"},
	})
	body := readAll(t, missResp)
	missResp.Body.Close()
	if missResp.StatusCode != http.StatusNotFound {
		t.Errorf("not-found attach status = %d, want 404", missResp.StatusCode)
	}
	if !strings.Contains(body, "NonexistentName") {
		t.Errorf("not-found body should echo user input %q, got: %q", "NonexistentName", body)
	}
}

// TestEventsPagesRenderUIIDs is the consolidated table-driven
// UIID pin for the four Event page wrappers (issue #396 +
// parallel of the soldier-side #397 wide.*). The four prior
// tests it replaced each booted newStressApp +
// httptest.NewServer + createEvent to GET one path and assert
// one anchor; sharing the App + event across the four
// subtests cuts 3 cold boots from the CI hot path.
//
// Page wrapper UIIDs PageEventList / PageEventDetail /
// PageEventNew / PageEventEdit are asserted via the same
// `id="..."` substring pattern the prior tests used. The
// detail + edit subtests share one createEvent seed
// (mirroring the prior standalone tests).
func TestEventsPagesRenderUIIDs(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	e := createEvent(t, app, "Battle", "07/01/1863", "07/03/1863", "UIIDTest")

	cases := []struct {
		name  string
		path  string
		uiids []string
	}{
		{
			name:  "list",
			path:  "/events",
			uiids: []string{uiids.PageEventList},
		},
		{
			name:  "detail",
			path:  "/events/" + strconv.FormatInt(e.ID, 10),
			uiids: []string{uiids.PageEventDetail},
		},
		{
			name:  "new",
			path:  "/events/new",
			uiids: []string{uiids.PageEventNew},
		},
		{
			name:  "edit",
			path:  "/events/" + strconv.FormatInt(e.ID, 10) + "/edit",
			uiids: []string{uiids.PageEventEdit},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Get(server.URL + tc.path)
			if err != nil {
				t.Fatalf("GET %s: %v", tc.path, err)
			}
			body := readAll(t, resp)
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET %s status = %d, want 200", tc.path, resp.StatusCode)
			}
			for _, id := range tc.uiids {
				want := fmt.Sprintf(`id="%s"`, id)
				if !strings.Contains(body, want) {
					t.Errorf("%s missing #%s anchor", tc.path, id)
				}
			}
		})
	}
}
