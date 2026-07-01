package appshell

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/valueforvalue/DixieData/internal/models"
)

// TestHandleExportPreview_AllScope verifies the preview handler
// returns a count + first-5 list when scope=all is selected.
func TestHandleExportPreview_AllScope(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	for i := 0; i < 10; i++ {
		_, err := app.soldiers.Create(models.Soldier{
			DisplayID: fmt.Sprintf("PRV-%03d", i),
			FirstName: fmt.Sprintf("Preview-%d", i),
			LastName:  "Worker",
		})
		if err != nil {
			t.Fatalf("seed Create %d: %v", i, err)
		}
	}

	form := url.Values{}
	form.Set("scope", "all")
	resp, err := http.PostForm(server.URL+"/export/preview", form)
	if err != nil {
		t.Fatalf("POST /export/preview: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	content := string(body)
	for _, needle := range []string{
		"10 records match this configuration",
		"Scope: All records",
		"Sort: Alphabetical by Last Name",
		"Group By: No grouping",
	} {
		if !strings.Contains(content, needle) {
			t.Errorf("preview body missing %q", needle)
		}
	}
}

// TestHandleExportPreview_SelectedScopeWithNoIDs verifies the
// preview handler falls back to "0 records" + "Scope: All records"
// when scope=selected arrives with no IDs. PrintSettings.Normalize
// silently downgrades scope=selected+empty IDs to scope=all (see
// pkg/render/render.go), so the preview reports an empty result
// set rather than blocking on the form. The frontend treats this as
// a hint to the user that they should select records.
func TestHandleExportPreview_SelectedScopeWithNoIDs(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	form := url.Values{}
	form.Set("scope", "selected")
	resp, err := http.PostForm(server.URL+"/export/preview", form)
	if err != nil {
		t.Fatalf("POST /export/preview: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	content := string(body)
	for _, needle := range []string{
		"0 records match this configuration",
		"Scope: All records",
	} {
		if !strings.Contains(content, needle) {
			t.Errorf("preview body missing %q in:\n%s", needle, content)
		}
	}
}

// TestHandleExportPreview_StaleFilterWarning (issue #185) asserts
// the preview fragment surfaces a stale-filter warning when the
// user submits a filter value that does not exist on any row.
// Mirrors what the Load handler emits so the preview counter and
// the eventual Generate cannot silently disagree.
func TestHandleExportPreview_StaleFilterWarning(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	for i := 0; i < 3; i++ {
		_, err := app.soldiers.Create(models.Soldier{
			DisplayID: fmt.Sprintf("PRV-STALE-%03d", i),
			FirstName: fmt.Sprintf("Stale-%d", i),
			LastName:  "Worker",
			Unit:      "5th Virginia Infantry",
		})
		if err != nil {
			t.Fatalf("seed Create %d: %v", i, err)
		}
	}

	form := url.Values{}
	form.Set("scope", "filtered")
	form.Set("filter_unit", "No-Such-Unit") // does not exist
	resp, err := http.PostForm(server.URL+"/export/preview", form)
	if err != nil {
		t.Fatalf("POST /export/preview: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	content := string(body)
	if !strings.Contains(content, "stale filter value") {
		t.Errorf("preview body missing stale warning line:\n%s", content)
	}
}

// TestHandleExportPreview_FilteredScope verifies the preview
// filter logic returns only matching records.
func TestHandleExportPreview_FilteredScope(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	// Seed two units: Alpha (3 records) and Bravo (2 records).
	for i, unit := range []string{"Alpha", "Alpha", "Alpha", "Bravo", "Bravo"} {
		_, err := app.soldiers.Create(models.Soldier{
			DisplayID: fmt.Sprintf("FLT-%02d", i),
			FirstName: fmt.Sprintf("Filter-%d", i),
			LastName:  "Tester",
			Unit:      unit,
		})
		if err != nil {
			t.Fatalf("seed Create %d: %v", i, err)
		}
	}

	form := url.Values{}
	form.Set("scope", "filtered")
	form.Set("filter_unit", "Alpha")
	resp, err := http.PostForm(server.URL+"/export/preview", form)
	if err != nil {
		t.Fatalf("POST /export/preview: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	content := string(body)
	for _, needle := range []string{
		"3 records match this configuration",
		"Scope: Filtered records",
	} {
		if !strings.Contains(content, needle) {
			t.Errorf("preview body missing %q", needle)
		}
	}
}

// TestHandleExportPreviewResponseUnderThreshold (issue #188) is a
// measurement-only stress test. It seeds 5,000 rows (the chosen
// upper bound for a v1 DixieData archive), warms up a single POST
// /export/preview, then measures the second request. If this
// ever fails, that's the signal to invest in caching
// (listAllSoldiers keyed by archive etag, or push preview to a
// background worker). The test exists to catch the regression
// early, not to ship with a brittle threshold.
//
// Threshold rationale (from issue #188): preview is debounced
// 150ms. A 500ms response ceiling means rapid changes collapse
// to ~3 requests/sec before queueing. UX is acceptable but
// borderline.
func TestHandleExportPreviewResponseUnderThreshold(t *testing.T) {
	if testing.Short() {
		t.Skip("response-time test: run via `go test ./internal/appshell/...` without -short")
	}
	app := newStressApp(t)

	const recordCount = 5000
	for i := 0; i < recordCount; i++ {
		_, err := app.soldiers.Create(models.Soldier{
			DisplayID: fmt.Sprintf("PRT-%05d", i),
			FirstName: fmt.Sprintf("Preview-%d", i),
			LastName:  fmt.Sprintf("Response-%d", i),
			Unit:      fmt.Sprintf("Regiment %d", i%50),
		})
		if err != nil {
			t.Fatalf("seed Create %d: %v", i, err)
		}
	}

	server := httptest.NewServer(app)
	defer server.Close()

	// Warm up once — first request pays any one-time costs
	// (sqlite plan cache, etc.). The second request is the
	// measurement that matches what the user perceives.
	form := url.Values{}
	form.Set("scope", "filtered")
	form.Set("filter_unit", fmt.Sprintf("Regiment %d", 7))
	warmup, err := http.PostForm(server.URL+"/export/preview", form)
	if err != nil {
		t.Fatalf("warmup POST /export/preview: %v", err)
	}
	warmup.Body.Close()

	const threshold = 500 * time.Millisecond
	start := time.Now()
	resp, err := http.PostForm(server.URL+"/export/preview", form)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("POST /export/preview: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /export/preview: status %d, want 200", resp.StatusCode)
	}
	if elapsed > threshold {
		t.Fatalf("POST /export/preview with %d records took %v, want < %v (see issue #188 for the perf budget)", recordCount, elapsed, threshold)
	}
	t.Logf("POST /export/preview with %d records: %v (threshold %v)", recordCount, elapsed, threshold)
}