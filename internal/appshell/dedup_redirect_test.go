package appshell

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/valueforvalue/DixieData/internal/jobs"
)

// TestRespondDuplicateInFlightRedirectsToExistingJob is the unit-level
// regression test for issue #130. When a duplicate request hits the
// in-flight guard and a background job has already been started
// under the same dupKey, the handler must redirect the user to the
// job's /jobs/{id} status page instead of returning the error
// body that left them stranded.
func TestRespondDuplicateInFlightRedirectsToExistingJob(t *testing.T) {
	app := NewApp()
	app.jobs = jobs.NewWithConcurrency(1)
	t.Cleanup(func() { _ = app.jobs.Shutdown(context.Background()) })
	// Start a real background job so we have a valid JobID.
	jobID := app.jobs.Start("soldier_pdf", func(_ context.Context, _ *jobs.Progress) error {
		return nil
	})
	dupKey := "soldier-pdf|1|L|john-doe.pdf"
	app.inFlight.Store(dupKey, &inFlightEntry{JobID: jobID})

	req := httptest.NewRequest(http.MethodPost, "/soldiers/1/pdf", nil)
	rec := httptest.NewRecorder()
	app.respondDuplicateInFlight(rec, req, dupKey)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != "/jobs/"+jobID {
		t.Fatalf("expected Location=/jobs/%s, got %q", jobID, got)
	}
}

// TestRespondDuplicateInFlightWithoutJobIDReturnsUserMessage is the
// companion test for issue #130. When the duplicate request arrives
// while the save dialog is still open (no JobID yet), the handler
// must respond with HX-Redirect + a toast header so the originating
// page stays put. Returning the error body would replace the modal
// and strand the user.
func TestRespondDuplicateInFlightWithoutJobIDReturnsUserMessage(t *testing.T) {
	app := NewApp()
	app.jobs = jobs.NewWithConcurrency(1)
	t.Cleanup(func() { _ = app.jobs.Shutdown(context.Background()) })
	dupKey := "soldier-pdf|1|L|john-doe.pdf"
	app.inFlight.Store(dupKey, &inFlightEntry{JobID: ""})

	req := httptest.NewRequest(http.MethodPost, "/soldiers/1/pdf", nil)
	req.Header.Set("Referer", "http://example.test/soldiers/1")
	rec := httptest.NewRecorder()
	app.respondDuplicateInFlight(rec, req, dupKey)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 HX-Redirect, got status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("HX-Redirect"); got != "/soldiers/1" {
		t.Fatalf("expected HX-Redirect=/soldiers/1, got %q", got)
	}
	if got := rec.Header().Get("X-DixieData-Toast"); got == "" {
		t.Fatalf("expected a toast header so the user sees feedback; got none")
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("expected empty body (toast + HX-Redirect should not include error page text); got %q", rec.Body.String())
	}
}

// TestRespondDuplicateInFlightWithUnknownKeyReturnsUserMessage covers
// the safety path: when the dupKey is not present in inFlight at all,
// respondDuplicateInFlight must not panic and must still respond with
// the toast + HX-Redirect fallback.
func TestRespondDuplicateInFlightWithUnknownKeyReturnsUserMessage(t *testing.T) {
	app := NewApp()
	app.jobs = jobs.NewWithConcurrency(1)
	t.Cleanup(func() { _ = app.jobs.Shutdown(context.Background()) })
	req := httptest.NewRequest(http.MethodPost, "/soldiers/1/pdf", nil)
	req.Header.Set("Referer", "http://example.test/soldiers/1")
	rec := httptest.NewRecorder()
	app.respondDuplicateInFlight(rec, req, "never-stored-key")

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 HX-Redirect, got status=%d", rec.Code)
	}
	if got := rec.Header().Get("HX-Redirect"); got != "/soldiers/1" {
		t.Fatalf("expected HX-Redirect=/soldiers/1, got %q", got)
	}
}

// TestEnqueueExportRecordsJobIDOnEntry verifies the wiring between
// the in-flight dedup key and the background-job registry. After
// enqueueExport writes the 303, the entry's JobID must be populated
// so a duplicate request can be redirected to the same /jobs/{id}
// status page.
func TestEnqueueExportRecordsJobIDOnEntry(t *testing.T) {
	app := NewApp()
	app.jobs = jobs.NewWithConcurrency(1)
	t.Cleanup(func() { _ = app.jobs.Shutdown(context.Background()) })
	app.jobs.Start("soldier_pdf", func(_ context.Context, _ *jobs.Progress) error { return nil })

	dupKey := "soldier-pdf|1|L|john-doe.pdf"
	admitted, entry := app.enterInFlight(dupKey)
	if !admitted || entry == nil {
		t.Fatalf("expected admission for fresh dupKey")
	}

	rec := httptest.NewRecorder()
	app.enqueueExport(dupKey, "soldier_pdf", func(_ *jobs.Progress) error {
		return nil
	}, "/tmp/example.pdf", rec)

	if entry.JobID == "" {
		t.Fatalf("expected JobID populated on entry after enqueueExport; got empty")
	}
	if loc := rec.Header().Get("Location"); loc != "/jobs/"+entry.JobID {
		t.Fatalf("expected Location=/jobs/%s, got %q", entry.JobID, loc)
	}
	// htmx 2.x with hx-swap="none" needs HX-Redirect to navigate;
	// without it the user sits on the originating page while the
	// export runs invisibly. Pinning it down here so a future
	// refactor that drops HX-Redirect fails this test.
	if hx := rec.Header().Get("HX-Redirect"); hx != "/jobs/"+entry.JobID {
		t.Fatalf("expected HX-Redirect=/jobs/%s so htmx hx-swap=none buttons land on the status page; got %q", entry.JobID, hx)
	}
}