package appshell

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestSettingsOrphanScanEndpointRendersResults covers the regression
// net for issue #134: clicking "Scan for Orphaned Images" on /settings
// now writes the response body into #settings-orphan-results via the
// data-results-target convention added to dispatchDixieDataForm. The
// server contract is unchanged — handler still returns 200 + HTML —
// but the body MUST contain the orphan summary so the dispatcher has
// something to inject into the results div.
func TestSettingsOrphanScanEndpointRendersResults(t *testing.T) {
	app := newStressApp(t)

	req := httptest.NewRequest(http.MethodPost, "/settings/images/orphans/scan", nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	// Empty-archive case renders this sentence verbatim. If the
	// handler regressed to an empty body or a redirect, the
	// dispatcher's data-results-target write would still land an
	// empty string and the UI would look broken.
	if !strings.Contains(body, "No orphaned image files were found.") {
		t.Fatalf("scan body missing empty-state marker: %q", body)
	}
	// Defence-in-depth: handler must NOT emit a redirect header —
	// that would race the data-results-target write and leave the
	// target div empty after navigation.
	if rec.Header().Get("X-DixieData-Redirect") != "" || rec.Header().Get("Location") != "" {
		t.Fatalf("scan handler must not redirect; got headers %v", rec.Header())
	}
}