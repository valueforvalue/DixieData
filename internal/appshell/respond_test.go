package appshell

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRespondErrorEnvelope validates that every kind produces the right
// HTTP status, toast kind, and that the raw error is never written to
// the body. This is the contract audit findings 1.1 / 1.2 / 1.3 / 1.9
// rely on for the 2026-06-24 audit (issue #88).
func TestRespondErrorEnvelope(t *testing.T) {
	cases := []struct {
		name         string
		kind         ErrorKind
		wantStatus   int
		wantToastKind string
	}{
		{"validation", KindValidation, http.StatusBadRequest, "warning"},
		{"not_found", KindNotFound, http.StatusNotFound, "error"},
		{"conflict", KindConflict, http.StatusConflict, "warning"},
		{"forbidden", KindForbidden, http.StatusForbidden, "error"},
		{"unavailable", KindUnavailable, http.StatusServiceUnavailable, "error"},
		{"internal", KindInternal, http.StatusInternalServerError, "error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/test", nil)
			respondError(rec, req, tc.kind, "boom", errors.New("sqlite: constraint failed"))

			if rec.Code != tc.wantStatus {
				t.Fatalf("status: got %d, want %d", rec.Code, tc.wantStatus)
			}
			if got := rec.Header().Get("X-DixieData-Toast-Type"); got != tc.wantToastKind {
				t.Fatalf("toast kind: got %q, want %q", got, tc.wantToastKind)
			}
			if got := rec.Header().Get("X-DixieData-Toast"); got != "boom" {
				t.Fatalf("toast message: got %q, want %q", got, "boom")
			}
			body := rec.Body.String()
			if body != "boom" {
				t.Fatalf("body: got %q, want %q", body, "boom")
			}
			// Raw error MUST NOT leak.
			if strings.Contains(body, "sqlite") || strings.Contains(body, "constraint") {
				t.Fatalf("raw error leaked into body: %q", body)
			}
		})
	}
}

// TestRespondErrorFallsBackToDefaultMessage verifies that callers who
// pass an empty userMessage get the kind-specific generic copy rather
// than an empty toast.
func TestRespondErrorFallsBackToDefaultMessage(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	respondError(rec, req, KindNotFound, "", nil)

	if got := rec.Header().Get("X-DixieData-Toast"); got == "" {
		t.Fatalf("expected non-empty default toast, got empty")
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// TestRespondErrorNilSafe covers the rare path where a deferred recover
// or background goroutine calls respondError without a request or
// response writer. The helper must not panic.
func TestRespondErrorNilSafe(t *testing.T) {
	t.Run("nil writer", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("respondError panicked with nil writer: %v", r)
			}
		}()
		respondError(nil, nil, KindInternal, "boom", nil)
	})
	t.Run("nil request", func(t *testing.T) {
		rec := httptest.NewRecorder()
		respondError(rec, nil, KindInternal, "boom", errors.New("inner"))
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status: got %d, want %d", rec.Code, http.StatusInternalServerError)
		}
	})
}

// TestRespondShorthandHelpers checks the per-kind shorthand helpers
// produce the same envelope as the full respondError call.
func TestRespondShorthandHelpers(t *testing.T) {
	cases := []struct {
		name       string
		fn         func(http.ResponseWriter, *http.Request, string, error)
		wantStatus int
		wantToast  string
	}{
		{"validation", respondValidation, http.StatusBadRequest, "warning"},
		{"not_found", respondNotFound, http.StatusNotFound, "error"},
		{"conflict", respondConflict, http.StatusConflict, "warning"},
		{"unavailable", respondUnavailable, http.StatusServiceUnavailable, "error"},
		{"internal", respondInternal, http.StatusInternalServerError, "error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/test", nil)
			tc.fn(rec, req, "boom", errors.New("inner"))
			if rec.Code != tc.wantStatus {
				t.Fatalf("status: got %d, want %d", rec.Code, tc.wantStatus)
			}
			if got := rec.Header().Get("X-DixieData-Toast-Type"); got != tc.wantToast {
				t.Fatalf("toast kind: got %q, want %q", got, tc.wantToast)
			}
		})
	}
}