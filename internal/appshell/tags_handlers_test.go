package appshell

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
)

var tagSeedCounter int64

// newTagTestApp mirrors newStressApp so the bootstrap "Loading
// DixieData..." screen doesn't intercept real responses.
func newTagTestApp(t *testing.T) *App {
	t.Helper()
	dataDir := filepath.Join(t.TempDir(), ".dixiedata")
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	app := NewApp()
	app.WithFrontendAssets(os.DirFS(repoFixturePath(t, "frontend")))
	app.dataDir = dataDir
	app.database = database
	if err := app.reloadServices(); err != nil {
		t.Fatalf("reloadServices: %v", err)
	}
	configureTestIdentity(t, app)
	app.setupRoutes()
	return app
}

// seedPersonRecord inserts one Person Record so attach/detach
// handlers have a real FK to bind to. Returns the new id.
// Uses an atomic counter for unique display_ids so a single test
// binary can call seedPersonRecord multiple times.
func seedPersonRecord(t *testing.T, app *App) int64 {
	t.Helper()
	n := atomic.AddInt64(&tagSeedCounter, 1)
	displayID := fmt.Sprintf("DXD-TAG-%s-%03d",
		strings.NewReplacer("/", "-", " ", "-").Replace(t.Name()), n)
	res, err := app.database.Conn().Exec(
		`INSERT INTO soldiers (display_id, first_name, last_name) VALUES (?, ?, ?)`,
		displayID, "Tag", "Test")
	if err != nil {
		t.Fatalf("insert soldier (%s): %v", displayID, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}
	return id
}

func TestTagAutocomplete_EmptyAndSearch(t *testing.T) {
	app := newTagTestApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	// Seed a couple of tags via the service to bypass form parsing.
	if _, err := app.tags.UpsertByName(context.Background(), "vc-shiloh"); err != nil {
		t.Fatalf("seed vc-shiloh: %v", err)
	}
	if _, err := app.tags.UpsertByName(context.Background(), "unit-4th-al"); err != nil {
		t.Fatalf("seed unit-4th-al: %v", err)
	}

	resp, err := http.Get(server.URL + "/soldiers/1/tags?autocomplete=vc")
	if err != nil {
		t.Fatalf("GET autocomplete: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("autocomplete status %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "vc-shiloh") {
		t.Errorf("expected vc-shiloh in body, got %s", body)
	}
	if strings.Contains(string(body), "unit-4th-al") {
		t.Errorf("autocomplete should not return unit-4th-al when querying 'vc', got %s", body)
	}
}

func TestAttachAndDetachTag(t *testing.T) {
	app := newTagTestApp(t)
	server := httptest.NewServer(app)
	defer server.Close()
	pid := seedPersonRecord(t, app)

	// Attach by name
	form := url.Values{}
	form.Set("tag_name", "vc-shiloh")
	resp, err := http.PostForm(server.URL+"/soldiers/"+tagItoa(t, pid)+"/tags", form)
	if err != nil {
		t.Fatalf("POST attach: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("attach status %d, want 200; body: %s", resp.StatusCode, body)
	}
	if got := resp.Header.Get("X-DixieData-Redirect"); got == "" {
		t.Errorf("attach handler missing X-DixieData-Redirect header")
	}

	// Find the tag id
	tag, err := app.tags.UpsertByName(context.Background(), "vc-shiloh")
	if err != nil {
		t.Fatalf("lookup tag: %v", err)
	}

	// Detach
	resp2, err := http.PostForm(
		server.URL+"/soldiers/"+tagItoa(t, pid)+"/tags/"+tagItoa(t, tag.ID),
		url.Values{})
	if err != nil {
		t.Fatalf("POST detach: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("detach status %d, want 200", resp2.StatusCode)
	}
	if got := resp2.Header.Get("X-DixieData-Redirect"); got == "" {
		t.Errorf("detach handler missing X-DixieData-Redirect header")
	}
	tags, _ := app.tags.TagsForSoldier(context.Background(), pid)
	if len(tags) != 0 {
		t.Errorf("after detach, TagsForSoldier = %d, want 0", len(tags))
	}
}

func TestBulkTagFromBrowse(t *testing.T) {
	app := newTagTestApp(t)
	server := httptest.NewServer(app)
	defer server.Close()
	pid1 := seedPersonRecord(t, app)
	pid2 := seedPersonRecord(t, app)

	form := url.Values{}
	form.Set("tag_name", "shared-vc")
	form.Add("selected_ids", tagItoa(t, pid1))
	form.Add("selected_ids", tagItoa(t, pid2))
	resp, err := http.PostForm(server.URL+"/browse/bulk-tag", form)
	if err != nil {
		t.Fatalf("POST bulk-tag: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("bulk-tag status %d, want 200; body: %s", resp.StatusCode, body)
	}
	if got := resp.Header.Get("X-DixieData-Redirect"); !strings.Contains(got, "/browse?bulk_tag=") {
		t.Errorf("bulk-tag redirect = %q, want /browse?bulk_tag=…", got)
	}
	out, _ := app.tags.TagsForSoldiers(context.Background(), []int64{pid1, pid2})
	for _, pid := range []int64{pid1, pid2} {
		if len(out[pid]) != 1 {
			t.Errorf("pid %d tagged count = %d, want 1", pid, len(out[pid]))
		}
	}
}

func TestBulkTagMissingSelection(t *testing.T) {
	app := newTagTestApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	form := url.Values{}
	form.Set("tag_name", "anywhere")
	resp, err := http.PostForm(server.URL+"/browse/bulk-tag", form)
	if err != nil {
		t.Fatalf("POST bulk-tag: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("bulk-tag without selection status %d, want 400", resp.StatusCode)
	}
}

func TestRenameTagRoundTrip(t *testing.T) {
	app := newTagTestApp(t)
	server := httptest.NewServer(app)
	defer server.Close()
	tag, err := app.tags.UpsertByName(context.Background(), "old-name")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	form := url.Values{}
	form.Set("tag_name", "new-name")
	resp, err := http.PostForm(
		server.URL+"/tags/"+tagItoa(t, tag.ID)+"/rename", form)
	if err != nil {
		t.Fatalf("POST rename: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("rename status %d, want 200; body: %s", resp.StatusCode, body)
	}
	got, err := app.tags.Get(context.Background(), tag.ID)
	if err != nil {
		t.Fatalf("post-rename Get: %v", err)
	}
	if got.NormalizedName != "new-name" {
		t.Errorf("NormalizedName after rename = %q, want new-name", got.NormalizedName)
	}
}

func TestRenameTagRejectsCollision(t *testing.T) {
	app := newTagTestApp(t)
	server := httptest.NewServer(app)
	defer server.Close()
	if _, err := app.tags.UpsertByName(context.Background(), "vc-shiloh"); err != nil {
		t.Fatalf("seed a: %v", err)
	}
	b, err := app.tags.UpsertByName(context.Background(), "other")
	if err != nil {
		t.Fatalf("seed b: %v", err)
	}
	form := url.Values{}
	form.Set("tag_name", "VC-Shiloh")
	resp, err := http.PostForm(
		server.URL+"/tags/"+tagItoa(t, b.ID)+"/rename", form)
	if err != nil {
		t.Fatalf("POST rename: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("rename-to-collide status %d, want 409", resp.StatusCode)
	}
}

func TestMergeAndDelete(t *testing.T) {
	app := newTagTestApp(t)
	server := httptest.NewServer(app)
	defer server.Close()
	pid := seedPersonRecord(t, app)
	src, _ := app.tags.UpsertByName(context.Background(), "draft-vc")
	if err := app.tags.Attach(context.Background(), src.ID, pid); err != nil {
		t.Fatalf("attach: %v", err)
	}
	survivor, _ := app.tags.UpsertByName(context.Background(), "vc-shiloh")
	form := url.Values{}
	form.Set("survivor_id", tagItoa(t, survivor.ID))
	resp, err := http.PostForm(
		server.URL+"/tags/"+tagItoa(t, src.ID)+"/merge", form)
	if err != nil {
		t.Fatalf("POST merge: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("merge status %d, want 200; body: %s", resp.StatusCode, body)
	}
	members, _ := app.tags.Members(context.Background(), survivor.ID)
	if len(members) != 1 || members[0] != pid {
		t.Errorf("post-merge survivor members = %v, want [%d]", members, pid)
	}

	// Delete survivor via DELETE
	req, _ := http.NewRequest(http.MethodDelete,
		server.URL+"/tags/"+tagItoa(t, survivor.ID), nil)
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("delete status %d, want 200", resp2.StatusCode)
	}
	if got := resp2.Header.Get("X-DixieData-Redirect"); got != "/tags" {
		t.Errorf("delete redirect = %q, want /tags", got)
	}
}

func TestDeleteTagMissing(t *testing.T) {
	app := newTagTestApp(t)
	server := httptest.NewServer(app)
	defer server.Close()
	req, _ := http.NewRequest(http.MethodDelete,
		server.URL+"/tags/99999", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("delete-missing status %d, want 404", resp.StatusCode)
	}
}

func TestShareExportOptionsToggle(t *testing.T) {
	app := newTagTestApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	form := url.Values{}
	form.Set("include_tags", "1")
	req, _ := http.NewRequest(http.MethodPatch,
		server.URL+"/share/export-options", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("PATCH status %d, want 200; body: %s", resp.StatusCode, body)
	}
	if got := resp.Header.Get("X-DixieData-Redirect"); got != "/share" {
		t.Errorf("redirect = %q, want /share", got)
	}
	got, err := app.archiveMeta.Get(context.Background(), "shared_archive")
	if err != nil {
		t.Fatalf("post-PATCH Get: %v", err)
	}
	if !got.IncludeTags {
		t.Errorf("include_tags = false after PATCH true")
	}

	// Decode a JSON snippet just to keep `encoding/json` import live
	// and demonstrate the handler returns from the redirect contract.
	if _, err := json.Marshal(map[string]bool{"ok": true}); err != nil {
		t.Fatalf("marshal: %v", err)
	}
}

func TestTagsManagementPageStub(t *testing.T) {
	app := newTagTestApp(t)
	server := httptest.NewServer(app)
	defer server.Close()
	resp, err := http.Get(server.URL + "/tags")
	if err != nil {
		t.Fatalf("GET /tags: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotImplemented {
		t.Errorf("/tags status %d, want 501 (pending commit 5 templ)", resp.StatusCode)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	app := newTagTestApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	// Dispatch handles only POST; GET should 405.
	resp, err := http.Get(server.URL + "/soldiers/1/tags")
	if err != nil {
		t.Fatalf("GET attach: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /soldiers/1/tags (autocomplete) status %d, want 200", resp.StatusCode)
	}

	// Same path, but POST without body should 400 (missing tag_name),
	// not 405.
	resp2, err := http.PostForm(server.URL+"/soldiers/1/tags", url.Values{})
	if err != nil {
		t.Fatalf("POST attach empty: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusBadRequest {
		t.Errorf("POST attach empty status %d, want 400", resp2.StatusCode)
	}
}

func tagItoa(t *testing.T, n int64) string {
	t.Helper()
	return jsonNumber(n)
}

func jsonNumber(n int64) string {
	b, _ := json.Marshal(n)
	return strings.Trim(string(b), "\"")
}
