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
// the /browse GET perf budget (issues #176, #234, #466). After
// issue #234 the GET no longer calls listAllSoldiers(); the
// print-config modal lazy-loads its filter panel + record picker
// from /share/print-records-fragment on first open. The bench-
// driven budget is 100ms at 5k records (was 500ms at 1k before
// the fix); if this test fails the most likely cause is a
// regression that re-introduces the full-archive load on the
// GET path. See docs/COMMON_BUGS.md §8.4 for the -race perf trap.
//
// Issue #466: skip under -race. modernc.org/sqlite (the pure-Go
// SQLite driver pinned in go.mod) is dominated by unsafe-pointer
// arithmetic, and the race detector's runtime.checkptr*
// instrumentation slows every prepared-statement parse by 10-30x.
// CPU profile of the failing race-stress run shows 38% of the
// race runtime in Xsqlite3_prepare_v2 + 16% in checkptrBase +
// 12% in checkptrStraddles — none of which is DixieData code.
// Switching to a cgo-based SQLite driver (mattn/go-sqlite3) would
// lift the -race cost back near the non-race 42ms but requires a
// C compiler at build time and touches every Wails desktop build,
// smoke harness, and gold-master path. The data-race side of #466
// was fixed in this slice (per-request state moved off package
// globals onto request context, see internal/templates/
// layout_helpers.go); only the perf-budget enforcement is skipped
// under -race. The non-race regression net is preserved.
func TestHandleBrowseResponseUnderThreshold(t *testing.T) {
	if testing.Short() {
		t.Skip("response-time test: run via `go test ./internal/appshell/...` without -short")
	}
	if raceEnabled() {
		t.Skip("response-time test: skipped under -race (see issue #466 for the runtime.checkptr cost in modernc.org/sqlite)")
	}
	app := newStressApp(t)

	const recordCount = 5000
	for i := 0; i < recordCount; i++ {
		_, err := app.soldiers.Create(models.Soldier{
			DisplayID: fmt.Sprintf("BRT-%04d", i),
			FirstName: fmt.Sprintf("Browse-%d", i),
			LastName:  fmt.Sprintf("Response-%d", i),
			Unit:      fmt.Sprintf("Regiment %d", i%50),
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