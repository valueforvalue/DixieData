package appshell

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/jobs"
)

func TestHandleJobStatusUnknownJobReturns404(t *testing.T) {
	app := newStressApp(t)
	req := httptest.NewRequest(http.MethodGet, "/jobs/does-not-exist", nil)
	rec := httptest.NewRecorder()
	app.handleJobStatus(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestHandleJobStatusFragmentRoute(t *testing.T) {
	app := newStressApp(t)
	id := app.jobs.Start("unit", func(ctx context.Context, p *jobs.Progress) error { return nil })
	req := httptest.NewRequest(http.MethodGet, "/jobs/"+id+"/status", nil)
	rec := httptest.NewRecorder()
	app.handleJobStatus(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "job-status-body") {
		t.Fatalf("status fragment should render the job-status-body wrapper; got:\n%s", body)
	}
}

func TestHandleJobCancelUnknownJobReturns404(t *testing.T) {
	app := newStressApp(t)
	req := httptest.NewRequest(http.MethodPost, "/jobs/missing/cancel", nil)
	rec := httptest.NewRecorder()
	app.handleJobStatus(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}