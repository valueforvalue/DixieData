package appshell

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGoogleManagedCalendarRoutesRequireConnection(t *testing.T) {
	app := newStressApp(t)
	for _, path := range []string{
		"/integrations/google/calendar/use-managed",
		"/integrations/google/calendar/sync-managed",
		"/integrations/google/calendar/unsync-managed",
		"/integrations/google/calendar/use-test",
		"/integrations/google/calendar/sync-test",
		"/integrations/google/calendar/unsync-test",
	} {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%q", path, rec.Code, rec.Body.String())
		}
		if !strings.Contains(strings.ToLower(rec.Body.String()), "failed") {
			t.Fatalf("%s expected failure message, got %q", path, rec.Body.String())
		}
	}
}
