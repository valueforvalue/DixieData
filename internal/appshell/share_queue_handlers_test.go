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

// TestShareQueuePage (issue #193) asserts the
// /share/queue management page renders with a 200 and the
// expected empty-state copy when the queue is empty.
// ?ids= is optional -- the JS populates it on load.
func TestShareQueuePage_Empty(t *testing.T) {
	app := newTagTestApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	resp, err := http.Get(server.URL + "/share/queue")
	if err != nil {
		t.Fatalf("GET page: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("page status %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	content := string(body)
	for _, needle := range []string{
		"Manage your staged subset",
		"No Person Records staged",
		"Remove Selected",
		"Export Selected as .ddshare",
		// Issue #310 PR 3: Saved Queues presets card.
		"Saved Queues",
		`data-share-queue-preset-save`,
		`data-share-queue-preset-list`,
		`data-share-queue-preset-empty`,
		`data-share-queue-preset-status`,
		`name="name"`,
		"Save current queue",
	} {
		if !strings.Contains(content, needle) {
			t.Errorf("page missing %s; got %s", needle, content)
		}
	}
}

// TestShareQueuePage_WithIDs (issue #193) seeds two
// Person Records, hits /share/queue?ids=X,Y, and asserts the
// table renders both rows with Display ID + Name. The page
// drops unknown ids silently (mirrors ByIDs's
// ErrNoRows-tolerance).
func TestShareQueuePage_WithIDs(t *testing.T) {
	app := newTagTestApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	pid1 := seedPersonRecord(t, app)
	pid2 := seedPersonRecord(t, app)

	resp, err := http.Get(server.URL + fmt.Sprintf("/share/queue?ids=%d,%d", pid1, pid2))
	if err != nil {
		t.Fatalf("GET page: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("page status %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	content := string(body)
	for _, needle := range []string{
		fmt.Sprintf("data-share-queue-page-row-id=\"%d\"", pid1),
		fmt.Sprintf("data-share-queue-page-row-id=\"%d\"", pid2),
		"Order",
		"Source Records",
		"Images",
	} {
		if !strings.Contains(content, needle) {
			t.Errorf("page missing %s; got %s", needle, content)
		}
	}
}

// TestShareQueuePage_DropsUnknownIDs (issue #193) asserts the
// handler silently drops ids that don't resolve to soldiers
// rather than 500-ing.
func TestShareQueuePage_DropsUnknownIDs(t *testing.T) {
	app := newTagTestApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	resp, err := http.Get(server.URL + "/share/queue?ids=99998,99999")
	if err != nil {
		t.Fatalf("GET page: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("page status %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "No Person Records staged") {
		t.Errorf("page should render empty state for all-unknown ids")
	}
}

// TestShareQueuePage_RouteWildcardNotShadowed (issue #193)
// asserts the new literal /share/queue route wins over the
// existing /share wildcard. A GET should return the
// management page chrome (Manage your staged subset), not
// the Share page chrome.
func TestShareQueuePage_RouteWildcardNotShadowed(t *testing.T) {
	app := newTagTestApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	resp, err := http.Get(server.URL + "/share/queue")
	if err != nil {
		t.Fatalf("GET page: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Manage your staged subset") {
		t.Errorf("page is being shadowed by /share wildcard; got %s", string(body))
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
