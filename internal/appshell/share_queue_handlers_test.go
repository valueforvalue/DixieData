package appshell

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

// TestShareQueuePreview (issue #182) seeds three soldiers via
// the facade, POSTs /share/queue/preview with the corresponding
// selected_ids, and asserts the returned fragment carries the
// right Soldiers / Source Records / Images counts. Issue #190
// hardens the assertion: with seedPersonRecordWithCounts
// attaching a known number of records/images to each staged
// row, the preview fragment must report the *exact* sum (not
// the prior substring check that passed even with the
// pre-enrichment stubbed counts of 0).
func TestShareQueuePreview(t *testing.T) {
	app := newTagTestApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	// Mix: one row with records only, one with images only,
	// one with both. Confirms per-row attribution is summed
	// across all three categories -- a missing join would
	// surface as a 0 in at least one column.
	pid1 := seedPersonRecordWithCounts(t, app, 2, 0)
	pid2 := seedPersonRecordWithCounts(t, app, 0, 3)
	pid3 := seedPersonRecordWithCounts(t, app, 1, 4)
	wantSoldiers := 3
	wantRecords := 3 // 2 + 0 + 1
	wantImages := 7  // 0 + 3 + 4

	form := url.Values{}
	form.Add("selected_ids", fmt.Sprintf("%d", pid1))
	form.Add("selected_ids", fmt.Sprintf("%d", pid2))
	form.Add("selected_ids", fmt.Sprintf("%d", pid3))
	resp, err := http.PostForm(server.URL+"/share/queue/preview", form)
	if err != nil {
		t.Fatalf("POST preview: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("preview status %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	content := string(body)

	wantLines := []string{
		fmt.Sprintf("%d soldiers will ship", wantSoldiers),
		fmt.Sprintf("Source Records: %d", wantRecords),
		fmt.Sprintf("Images: %d", wantImages),
	}
	for _, want := range wantLines {
		if !strings.Contains(content, want) {
			t.Errorf("preview body missing line %q; got %s", want, content)
		}
	}
}

// seedPersonRecordWithCounts creates a fresh soldier row plus
// `recordCount` Source Record rows and `imageCount` Image rows
// linked back to the soldier. Used by issue #190's hardened
// TestShareQueuePreview so the assertion can target real
// counts rather than the prior 0-by-default behavior.
func seedPersonRecordWithCounts(t *testing.T, app *App, recordCount, imageCount int) int64 {
	t.Helper()
	soldierID := seedPersonRecord(t, app)
	conn := app.database.Conn()
	for i := 0; i < recordCount; i++ {
		if _, err := conn.Exec(
			`INSERT INTO records (soldier_id, record_type, app_id, details) VALUES (?, ?, ?, ?)`,
			soldierID, "pension", fmt.Sprintf("TEST-RECORD-%d", i), fmt.Sprintf("details %d", i),
		); err != nil {
			t.Fatalf("insert record: %v", err)
		}
	}
	for i := 0; i < imageCount; i++ {
		if _, err := conn.Exec(
			`INSERT INTO images (soldier_id, file_name, file_path, caption) VALUES (?, ?, ?, ?)`,
			soldierID, fmt.Sprintf("img-%d.jpg", i), fmt.Sprintf("/tmp/img-%d.jpg", i), fmt.Sprintf("caption %d", i),
		); err != nil {
			t.Fatalf("insert image: %v", err)
		}
	}
	return soldierID
}

// TestShareQueuePreview_NoIDs (issue #182) asserts the preview
// falls back to a friendly zero-state fragment when no ids are
// submitted rather than 400-ing. Lets the JS layer retry
// without a hard error.
func TestShareQueuePreview_NoIDs(t *testing.T) {
	app := newTagTestApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	resp, err := http.PostForm(server.URL+"/share/queue/preview", url.Values{})
	if err != nil {
		t.Fatalf("POST preview empty: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("preview empty status %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Pick at least one Person Record") {
		t.Errorf("preview body should hint 'Pick at least one Person Record'; got %s", string(body))
	}
}

// TestShareQueueClear (issue #182) asserts the clear handler
// returns 200 + X-DixieData-Redirect: /share so the dispatcher's
// single anchor path covers the no-op server-side branch.
func TestShareQueueClear(t *testing.T) {
	app := newTagTestApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	resp, err := http.PostForm(server.URL+"/share/queue/clear", url.Values{})
	if err != nil {
		t.Fatalf("POST clear: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("clear status %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("X-DixieData-Redirect"); got != "/share" {
		t.Errorf("X-DixieData-Redirect = %q, want /share", got)
	}
}

// TestShareQueueModal (issue #182) asserts the modal route
// renders the templ with a 200. Commit 5 replaced the c4 stub
// with the full Share Build modal carrying the Export Selected
// CTA + the persistent queue list shell.
func TestShareQueueModal(t *testing.T) {
	app := newTagTestApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	resp, err := http.Get(server.URL + "/share/queue/modal")
	if err != nil {
		t.Fatalf("GET modal: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("modal status %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Share Queue") {
		t.Errorf("modal body missing 'Share Queue' heading; got %s", string(body))
	}
	if !strings.Contains(string(body), "Export Selected as .ddshare") {
		t.Errorf("modal body missing the Export Selected CTA; got %s", string(body))
	}
}

// TestExportSharedArchiveSubset_Roundtrip (issue #182) seeds
// multiple soldiers, POSTs /export/shared-archive?subset=1 with
// selected_ids, and verifies the X-DixieData-Redirect target
// is /jobs/{id} (Option C dispatcher contract).
func TestExportSharedArchiveSubset_Roundtrip(t *testing.T) {
	app := newTagTestApp(t)
	// Native SaveFileDialog is unavailable in the test harness;
	// set the override so the handler reaches the export path.
	app.saveFileDialogOverride = func(_ any) (string, error) {
		return filepath.Join(t.TempDir(), "subset.ddshare"), nil
	}
	server := httptest.NewServer(app)
	defer server.Close()

	pid1 := seedPersonRecord(t, app)
	pid2 := seedPersonRecord(t, app)

	form := url.Values{}
	form.Add("selected_ids", fmt.Sprintf("%d", pid1))
	form.Add("selected_ids", fmt.Sprintf("%d", pid2))
	resp, err := http.PostForm(server.URL+"/export/shared-archive?subset=1", form)
	if err != nil {
		t.Fatalf("POST subset export: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body := make([]byte, 4096)
		n, _ := resp.Body.Read(body)
		t.Fatalf("subset export status %d, want 200; body: %s", resp.StatusCode, string(body[:n]))
	}
	redirect := resp.Header.Get("X-DixieData-Redirect")
	if !strings.HasPrefix(redirect, "/jobs/") {
		t.Errorf("X-DixieData-Redirect = %q, want /jobs/{id}", redirect)
	}
	_ = context.Background
	_ = models.Soldier{}
}

// TestExportSharedArchiveSubset_EmptyIDs (issue #182) asserts
// the subset branch with no selected_ids returns 400 rather
// than silently exporting an empty archive.
func TestExportSharedArchiveSubset_EmptyIDs(t *testing.T) {
	app := newTagTestApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	resp, err := http.PostForm(server.URL+"/export/shared-archive?subset=1", url.Values{})
	if err != nil {
		t.Fatalf("POST empty subset: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("empty-subset status %d, want 400", resp.StatusCode)
	}
}
