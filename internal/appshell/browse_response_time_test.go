package appshell

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/valueforvalue/DixieData/internal/models"
)

// TestHandleBrowseResponseUnderThreshold is the regression net for
// issue #176. BrowseView now also loads the full archive via
// listAllSoldiers() so the in-place print-config modal can populate
// its filter dropdowns. We assert that even with 1000 records the
// /browse GET stays well under the UX-acceptable threshold so users
// don't perceive a slowdown.
//
// Per the issue critique, if this ever fails we revisit (lazy-load
// the filter options via AJAX on modal open, or cache the export
// list keyed by archive hash). The test exists to catch the
// regression early, not to ship with a brittle threshold.
func TestHandleBrowseResponseUnderThreshold(t *testing.T) {
	if testing.Short() {
		t.Skip("response-time test: run via `go test ./internal/appshell/...` without -short")
	}
	app := newStressApp(t)

	const recordCount = 1000
	for i := 0; i < recordCount; i++ {
		_, err := app.soldiers.Create(models.Soldier{
			DisplayID: fmt.Sprintf("BRT-%04d", i),
			FirstName: fmt.Sprintf("Browse-%d", i),
			LastName:  fmt.Sprintf("Response-%d", i),
			Unit:      fmt.Sprintf("Regiment %d", i%50), // 50 distinct units for filter scroll
		})
		if err != nil {
			t.Fatalf("seed Create %d: %v", i, err)
		}
	}

	server := httptest.NewServer(app)
	defer server.Close()

	// Warm up once (first request can pay one-time costs like
	// template compilation caches if any).
	resp, err := http.Get(server.URL + "/browse")
	if err != nil {
		t.Fatalf("warmup GET /browse: %v", err)
	}
	resp.Body.Close()

	const threshold = 500 * time.Millisecond
	start := time.Now()
	resp, err = http.Get(server.URL + "/browse")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("GET /browse: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /browse: status %d, want 200", resp.StatusCode)
	}
	if elapsed > threshold {
		t.Fatalf("GET /browse with %d records took %v, want < %v (see issue #176 for the perf budget)", recordCount, elapsed, threshold)
	}
	t.Logf("GET /browse with %d records: %v (threshold %v)", recordCount, elapsed, threshold)
}