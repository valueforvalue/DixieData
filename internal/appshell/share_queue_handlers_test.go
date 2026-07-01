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
// right Soldiers / Source Records / Images counts.
func TestShareQueuePreview(t *testing.T) {
	app := newTagTestApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	pid1 := seedPersonRecord(t, app)
	pid2 := seedPersonRecord(t, app)
	pid3 := seedPersonRecord(t, app)

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
	if !strings.Contains(content, "3 soldiers will ship") {
		t.Errorf("preview body missing 3-soldiers line; got %s", content)
	}
	if !strings.Contains(content, "Source Records:") {
		t.Errorf("preview body missing Source Records line")
	}
	if !strings.Contains(content, "Images:") {
		t.Errorf("preview body missing Images line")
	}
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
// renders the templ (currently the c4 stub) with a 200.
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
