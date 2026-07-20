package appshell

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/valueforvalue/DixieData/internal/jobs"
)

// TestRespondDuplicateInFlightReturnsToastAndRedirect is the updated
// regression test for the TryClaim-based guard (issue #615). When
// a duplicate request hits the claim guard, respondDuplicateInFlight
// returns 200 + X-DixieData-Redirect + toast. The legacy JobID
// redirect was dropped — the claim key alone is sufficient for dedup.
func TestRespondDuplicateInFlightReturnsToastAndRedirect(t *testing.T) {
	app := NewApp()
	app.jobs = jobs.NewWithConcurrency(1)
	t.Cleanup(func() { _ = app.jobs.Shutdown(context.Background()) })

	dupKey := "test-claim-key"
	release, ok := app.jobs.TryClaim(dupKey)
	if !ok {
		t.Fatal("first TryClaim must succeed")
	}
	defer release()

	req := httptest.NewRequest(http.MethodPost, "/soldiers/1/pdf", nil)
	req.Header.Set("Referer", "http://example.test/soldiers/1")
	rec := httptest.NewRecorder()
	app.respondDuplicateInFlight(rec, req, dupKey)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (Option C contract), got status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-DixieData-Redirect"); got == "" {
		t.Fatalf("expected X-DixieData-Redirect, got empty")
	}
	if got := rec.Header().Get("X-DixieData-Toast"); got == "" {
		t.Fatalf("expected a toast header so the user sees feedback; got none")
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("expected empty body; got %q", rec.Body.String())
	}
}

// TestRespondDuplicateInFlightWithoutClaimReturnsUserMessage covers
// the safety path: when the key is not claimed at all,
// respondDuplicateInFlight must still respond with toast + redirect
// without panicking.
func TestRespondDuplicateInFlightWithoutClaimReturnsUserMessage(t *testing.T) {
	app := NewApp()
	app.jobs = jobs.NewWithConcurrency(1)
	t.Cleanup(func() { _ = app.jobs.Shutdown(context.Background()) })

	dupKey := "unclaimed-key"

	req := httptest.NewRequest(http.MethodPost, "/soldiers/1/pdf", nil)
	req.Header.Set("Referer", "http://example.test/soldiers/1")
	rec := httptest.NewRecorder()
	app.respondDuplicateInFlight(rec, req, dupKey)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (Option C contract), got status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-DixieData-Redirect"); got == "" {
		t.Fatalf("expected X-DixieData-Redirect, got empty")
	}
	if got := rec.Header().Get("X-DixieData-Toast"); got == "" {
		t.Fatalf("expected toast header; got none")
	}
}
