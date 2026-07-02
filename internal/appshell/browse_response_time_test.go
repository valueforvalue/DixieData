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
// the /browse GET perf budget (issues #176, #234). After issue #234
// the GET no longer calls listAllSoldiers(); the print-config modal
// lazy-loads its filter panel + record picker from
// /share/print-records-fragment on first open. The bench-driven
// budget is 100ms at 5k records (was 500ms at 1k before the fix);
// if this test fails the most likely cause is a regression that
// re-introduces the full-archive load on the GET path. See
// docs/COMMON_BUGS.md §4.x for the bug class.
func TestHandleBrowseResponseUnderThreshold(t *testing.T) {
	if testing.Short() {
		t.Skip("response-time test: run via `go test ./internal/appshell/...` without -short")
	}
	app := newStressApp(t)

	const recordCount = 5000
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

	// Issue #234: tightened from 500ms after the lazy-load fix
	// dropped listAllSoldiers() from the /browse GET. Bench
	// numbers (pre-fix vs post-fix at 5k records): 261ms vs 36ms.
	const threshold = 100 * time.Millisecond
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