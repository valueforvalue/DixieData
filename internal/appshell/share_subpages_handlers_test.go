package appshell

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Issue #284 regression net: the three new /share subpages
// (exports, imports, sync) must each render 200 with the
// expected content. These tests don't exercise the data
// loading (exportRecords, Google status) in depth — they
// are the smoke-level "did the route work" assertions,
// matching the bar set by TestShareQueuePage (issue #193). The
// handler-level tests for the existing /share route
// continue to cover the data path; the new subpages are
// thin extractions of inline sections and the data path
// is the same as the pre-split landing.

// TestShareExportsSubpage_Renders asserts GET /share/exports
// returns 200 and carries the section id + breadcrumb that
// the locked decisions in #284 require. Also asserts the
// Open Share Queue link stays on this page (the foldout
// collapses the 4-item menu to 3 items; Build moves into
// the page). Updated issue #310: the link now navigates
// to /share/queue rather than opening the Share Build modal.
func TestShareExportsSubpage_Renders(t *testing.T) {
	app := newTagTestApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	resp, err := http.Get(server.URL + "/share/exports")
	if err != nil {
		t.Fatalf("GET /share/exports: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	content := string(body)

	mustContain(t, content, []string{
		`href="/share"`,      // breadcrumb back to /share
		"Share Exports",      // page header
		"Create files to share or preserve", // section heading (issue #579: was "Export & Backup" pre-#561)
		"Export JSON",        // the whole-archive export
		"Export Shared Archive (.ddshare)",
		"Open Share Queue",   // the navigate-to-queue link (issue #310)
		`href="/share/queue"`,  // the link target (issue #310)
		"data-share-include-tags", // the include-tags checkbox
	})
	mustNotContain(t, content, []string{
		"Import Shared Archive", // belongs to /share/imports
		"Google Integration",     // belongs to /share/sync
		"Build Share Archive",    // pre-#310 modal button (issue #310 removed it)
		`data-share-queue-open="`, // pre-#310 modal trigger attribute (issue #310 removed it)
	})
}

// TestShareImportsSubpage_Renders asserts GET /share/imports
// returns 200 and carries the three import modes. The imports
// subpage is a static launchpad — no handler-side data — so
// the test is content-only.
func TestShareImportsSubpage_Renders(t *testing.T) {
	app := newTagTestApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	resp, err := http.Get(server.URL + "/share/imports")
	if err != nil {
		t.Fatalf("GET /share/imports: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	content := string(body)

	mustContain(t, content, []string{
		`href="/share"`, // breadcrumb
		"Share Imports",
		"Bring data back into this local archive", // section heading (issue #579: was "Import & Restore" pre-#561)
		"Collaborative Merge",
		"Import Shared Archive (.ddshare)",
		"Memorial JSON Import",
		"Replace Local Archive", // destructive path stays visually isolated
	})
	mustNotContain(t, content, []string{
		"Export JSON",     // belongs to /share/exports
		"Google Integration", // belongs to /share/sync
	})
}

// TestShareSyncSubpage_Renders asserts GET /share/sync
// returns 200 and carries the Google Integration surface.
// The Google status data path is the same as the pre-split
// /share landing (a.google.Status() + CalendarDriftStatus)
// so a fresh / empty archive still renders the page with
// "Not connected" status.
func TestShareSyncSubpage_Renders(t *testing.T) {
	app := newTagTestApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	resp, err := http.Get(server.URL + "/share/sync")
	if err != nil {
		t.Fatalf("GET /share/sync: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	content := string(body)

	mustContain(t, content, []string{
		`href="/share"`, // breadcrumb
		"Share Sync",
		"Google Integration",
		"Connect Google Account",
		"DixieData Calendar",
		"DixieData Test Calendar",
		"Not connected", // fresh archive: not yet connected
	})
	mustNotContain(t, content, []string{
		"Export JSON",          // belongs to /share/exports
		"Import Shared Archive", // belongs to /share/imports
	})
}

// TestShareSubpages_MethodNotAllowed asserts each subpage
// rejects non-GET with 405 — the existing handleShare
// behaviour. Catches a regression where the new handlers
// silently accept POST/PUT.
func TestShareSubpages_MethodNotAllowed(t *testing.T) {
	app := newTagTestApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	for _, path := range []string{"/share/exports", "/share/imports", "/share/sync"} {
		req, err := http.NewRequest(http.MethodPost, server.URL+path, nil)
		if err != nil {
			t.Fatalf("NewRequest %s: %v", path, err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("POST %s status %d, want 405", path, resp.StatusCode)
		}
	}
}

// mustContain is a tiny assertion helper that fails the
// test with a single line per missing substring. Kept
// inline so the share_subpages test file doesn't grow
// a parallel test-helper file for a 6-line function.
func mustContain(t *testing.T, haystack string, needles []string) {
	t.Helper()
	for _, n := range needles {
		if !strings.Contains(haystack, n) {
			t.Errorf("expected to find %q in body", n)
		}
	}
}

func mustNotContain(t *testing.T, haystack string, needles []string) {
	t.Helper()
	for _, n := range needles {
		if strings.Contains(haystack, n) {
			t.Errorf("did not expect to find %q in body", n)
		}
	}
}
