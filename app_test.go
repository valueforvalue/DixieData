package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestAppServeHTTPStartupError(t *testing.T) {
	app := NewApp()
	app.startupErr = errors.New("startup failed")
	app.setupRoutes()

	req := httptest.NewRequest(http.MethodGet, "/calendar", nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(rec.Body.String(), "startup failed") {
		t.Fatalf("expected startup error in response, got %q", rec.Body.String())
	}
}

func TestParseSoldierFormRejectsInvalidMonth(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/soldiers", strings.NewReader(url.Values{
		"death_month": {"13"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	_, err := parseSoldierForm(req, 0)
	if err == nil || !strings.Contains(err.Error(), "death_month") {
		t.Fatalf("expected death_month validation error, got %v", err)
	}
}

func TestParseBoundedIntRejectsInvalidValues(t *testing.T) {
	if _, err := parseBoundedInt("0", "month", 1, 12); err == nil {
		t.Fatal("expected invalid month error")
	}
	if _, err := parseBoundedInt("abc", "month", 1, 12); err == nil {
		t.Fatal("expected invalid month parse error")
	}
}
