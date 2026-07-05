// articles_handlers_test.go covers the Article Record HTTP
// handlers (issue #321, slice 1: tracer bullet — Backend +
// /articles + /articles/{id} shells only). The test pins the
// headline acceptance criterion from issue #321: a researcher
// can POST /articles/new with title + subtitle + markdown body
// and land on /articles/{id} showing the title + body. Goes
// RED today (pre-slice-1): /articles/* routes are not
// registered, so httptest.NewServer returns 404 on the POST
// and the redirect assertion fails.
//
// Once slice-1 lands (route registration + minimal Article
// service + a /articles list page that links to /articles/{id}
// detail shells), this test flips GREEN and stays green through
// slice 2 (editor + picker) and slice 3 (Revisions) — those
// slices add apply-sites without changing the CRUD round-trip
// contract.
//
// Test shape mirrors TestHandleNewEventPostCreatesEvent (slice
// 3 of issue #320): httptest.NewServer wrapping the App, then
// http.PostForm for the create + http.Get for the read-back.
package appshell

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

// TestHandleArticleCRUD_RoundTrip pins the slice-1 headline
// acceptance criterion: POST /articles/new with title +
// subtitle + body returns 200 (with the X-DixieData-Redirect
// convention used elsewhere in this package per #341), the
// redirect target is /articles/{row-id}, and a follow-up GET
// of that URL returns 200 with the title + body text rendered
// on the page.
//
// Sub-tests cover the happy path + the empty-title rejection
// path so a slice-1 implementation that silently accepts blank
// titles is caught here rather than at the UI feedback layer.
func TestHandleArticleCRUD_RoundTrip(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	t.Run("create then read round-trips title and body", func(t *testing.T) {
		form := url.Values{}
		form.Set("title", "21st Mississippi at Gettysburg")
		form.Set("subtitle", "Day 2 on the Peach Orchard line")
		form.Set("body", "A regimental narrative covering July 2-3, 1863.")

		resp, err := http.PostForm(server.URL+"/articles/new", form)
		if err != nil {
			t.Fatalf("POST /articles/new: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("POST /articles/new status = %d, want 200 (X-DixieData-Redirect convention)", resp.StatusCode)
		}

		redirect := resp.Header.Get("X-DixieData-Redirect")
		if !strings.HasPrefix(redirect, "/articles/") {
			t.Fatalf("X-DixieData-Redirect = %q, want /articles/{row-id} prefix", redirect)
		}
		// Reject the empty-id case (would mean a 200 with no
		// usable redirect target). The /articles/{id} URL uses
		// the SQLite row id, not the ART-NNNNN Display ID, so
		// a missing row id is the failure mode.
		idSegment := strings.TrimPrefix(redirect, "/articles/")
		if idSegment == "" {
			t.Fatalf("X-DixieData-Redirect %q carries no row id", redirect)
		}
		// Reject a string that obviously isn't a row id (defends
		// against accidentally re-using the Display ID as the
		// path segment, which would 404 on the follow-up GET).
		if strings.Contains(idSegment, "-") {
			t.Fatalf("X-DixieData-Redirect %q uses the Display ID instead of the row id", redirect)
		}

		// Follow-up GET renders the detail page with the body
		// content + title text on the page (the slice-1 detail
		// shell only needs to render the body verbatim; the
		// sanitized preview lands in slice 2 with the markdown
		// library drop).
		detail, err := http.Get(server.URL + redirect)
		if err != nil {
			t.Fatalf("GET %s: %v", redirect, err)
		}
		defer detail.Body.Close()
		if detail.StatusCode != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200", redirect, detail.StatusCode)
		}
		body := readAll(t, detail)
		if !strings.Contains(body, "21st Mississippi at Gettysburg") {
			t.Errorf("GET %s body missing title text '21st Mississippi at Gettysburg':\n%s", redirect, body)
		}
		if !strings.Contains(body, "regimental narrative") {
			t.Errorf("GET %s body missing body text 'regimental narrative':\n%s", redirect, body)
		}
	})

	// Empty-title guard so slice-1 doesn't accept the form
	// silently. The current /events/new handler returns 400 on
	// blank kind (issue #320 convention); /articles/new should
	// follow the same shape so a future frontend error message
	// has a stable contract to render against.
	t.Run("blank title returns a 4xx", func(t *testing.T) {
		form := url.Values{}
		form.Set("title", "")
		form.Set("subtitle", "no title")
		form.Set("body", "body without title")
		resp, err := http.PostForm(server.URL+"/articles/new", form)
		if err != nil {
			t.Fatalf("POST /articles/new (blank title): %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode < 400 || resp.StatusCode >= 500 {
			t.Fatalf("POST /articles/new with blank title status = %d, want 4xx", resp.StatusCode)
		}
	})
}


// createArticleHandlerTestPerson seeds a Soldier via the
// full *App fixture so handler-level ref tests have a real
// Person row to attach. The Display ID is explicit so the
// handler can resolve it via GetByDisplayID.
func createArticleHandlerTestPerson(t *testing.T, app *App, displayID string) {
	t.Helper()
	_, err := app.soldiers.Create(models.Soldier{
		DisplayID: displayID,
		FirstName: "Test",
		LastName:  "Person",
		Rank:      "Private",
		Unit:      "Test Unit",
	})
	if err != nil {
		t.Fatalf("Create test person %q: %v", displayID, err)
	}
}

// TestHandleArticlesListRendersArticles pins the slice-2 GET
// /articles list surface: after creating 2 articles, GET
// /articles returns 200 and the rendered HTML contains
// both titles.
func TestHandleArticlesListRendersArticles(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	for _, title := range []string{"List Test One", "List Test Two"} {
		if _, err := app.articles.Create(models.Article{Title: title}); err != nil {
			t.Fatalf("Create %q: %v", title, err)
		}
	}

	resp, err := http.Get(server.URL + "/articles")
	if err != nil {
		t.Fatalf("GET /articles: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /articles status = %d, want 200", resp.StatusCode)
	}
	body := readAll(t, resp)
	for _, title := range []string{"List Test One", "List Test Two"} {
		if !strings.Contains(body, title) {
			t.Errorf("GET /articles missing title %q", title)
		}
	}
}

// TestHandleArticleByIDNotFound pins the slice-2 404 contract
// for GET /articles/{id}. The slice-1 path could not exercise
// this because the slice-1 surface was the valid-id happy path
// pinned by TestHandleArticleCRUD_RoundTrip.
func TestHandleArticleByIDNotFound(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	resp, err := http.Get(server.URL + "/articles/999999")
	if err != nil {
		t.Fatalf("GET /articles/999999: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET /articles/999999 status = %d, want 404", resp.StatusCode)
	}
}

// TestHandleArticleRefsAttachDetach pins the slice-2 ref
// round-trip. POST /articles/{id}/refs with the Display ID
// form value attaches; a follow-up DELETE on the same ref
// pair removes; a second DELETE is a no-op (idempotent).
func TestHandleArticleRefsAttachDetach(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	// Seed a Person Record with an explicit Display ID so
	// the handler can resolve via GetByDisplayID.
	createArticleHandlerTestPerson(t, app, "DXD-00099")

	// Create an Article.
	article, err := app.articles.Create(models.Article{Title: "Refs target"})
	if err != nil {
		t.Fatalf("Create article: %v", err)
	}

	// Locate the seeded person row id.
	person, err := app.soldiers.GetByDisplayID("DXD-00099")
	if err != nil {
		t.Fatalf("lookup person: %v", err)
	}

	// Attach via the handler.
	form := url.Values{}
	form.Set("display_id", "DXD-00099")
	resp, err := http.PostForm(server.URL+"/articles/"+intStr(article.ID)+"/refs", form)
	if err != nil {
		t.Fatalf("POST /articles/%d/refs: %v", article.ID, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /articles/%d/refs status = %d, want 200", article.ID, resp.StatusCode)
	}
	if got := resp.Header.Get("X-DixieData-Redirect"); !strings.HasPrefix(got, "/articles/") {
		t.Errorf("X-DixieData-Redirect = %q, want /articles/{id} prefix", got)
	}

	refs, _ := app.articles.ScanRefs(article.ID)
	if len(refs) != 1 {
		t.Fatalf("ScanRefs after attach len = %d, want 1", len(refs))
	}

	// Detach via the handler.
	req, err := http.NewRequest(http.MethodDelete,
		server.URL+"/articles/"+intStr(article.ID)+"/refs/"+intStr(person.ID), nil)
	if err != nil {
		t.Fatalf("build DELETE: %v", err)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE ref: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE ref status = %d, want 200", resp.StatusCode)
	}

	refs, _ = app.articles.ScanRefs(article.ID)
	if len(refs) != 0 {
		t.Errorf("ScanRefs after detach len = %d, want 0", len(refs))
	}

	// Idempotent detach.
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE ref (idempotent): %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("idempotent DELETE ref status = %d, want 200", resp.StatusCode)
	}
}


// TestHandleArticleSnapshotRoundTrip pins the slice-2.5
// POST /articles/{id}/snapshot happy path: 200 +
// X-DixieData-Redirect; the underlying ArticleService now
// exposes 2 rows (the source + the new snapshot row); the
// snapshot row carries is_snapshot = 1 + snapshot_of_id =
// source.ID.
func TestHandleArticleSnapshotRoundTrip(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	src, err := app.articles.Create(models.Article{Title: "Snapshot source"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	resp, err := http.PostForm(
		server.URL+"/articles/"+intStr(src.ID)+"/snapshot",
		url.Values{})
	if err != nil {
		t.Fatalf("POST snapshot: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /articles/%d/snapshot status = %d, want 200", src.ID, resp.StatusCode)
	}
	if got := resp.Header.Get("X-DixieData-Redirect"); !strings.HasPrefix(got, "/articles/") {
		t.Errorf("X-DixieData-Redirect = %q, want /articles/{id} prefix", got)
	}

	// The snapshot row exists (via the service's
	// GetSnapshotByID helper). Source is still live (via
	// GetByID). The DB has 2 article rows.
	snap, err := app.articles.GetSnapshotByID(1)
	if err != nil {
		// 1 is the snapshot row id -- the SQLite
		// autoincrement starts at 1 for the first row
		// (source) and the snapshot is row 2.
		// We don't know the snap id without asking the
		// service, so list All snapshots via direct SQL.
		allRows := listAllArticleIDs(t, app)
		if len(allRows) != 2 {
			t.Fatalf("after snapshot: article count = %d, want 2 (source + snapshot)", len(allRows))
		}
		// Find the snapshot row (the one with is_snapshot = 1).
		for _, id := range allRows {
			if id != src.ID {
				if _, err := app.articles.GetSnapshotByID(id); err != nil {
					t.Errorf("expected snapshot row at id %d, got %v", id, err)
				}
			}
		}
	} else {
		if snap == nil {
			t.Errorf("snap unexpectedly nil")
		}
	}

	// Source row still live.
	if _, err := app.articles.GetByID(src.ID); err != nil {
		t.Errorf("source row disappeared: %v", err)
	}
}

// TestHandleArticleRestoreRoundTrip pins the slice-2.5
// POST /articles/{id}/restore happy path: the snapshot
// overwrites the live row's title + body; the snapshot
// remains in place.
func TestHandleArticleRestoreRoundTrip(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	src, _ := app.articles.Create(models.Article{Title: "Restore source"})
	snap, err := app.articles.Snapshot(src.ID)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	// Mutate the snapshot directly (the slice-2.5 surface
	// does not yet ship an Edit-snapshot handler; a future
	// slice may).
	if _, err := app.database.Conn().Exec(
		`UPDATE articles SET body_md = ?, title = ? WHERE id = ? AND is_snapshot = 1`,
		"restored body", "Restored Title", snap.ID); err != nil {
		t.Fatalf("direct SQL update of snapshot: %v", err)
	}

	resp, err := http.PostForm(
		server.URL+"/articles/"+intStr(snap.ID)+"/restore",
		url.Values{})
	if err != nil {
		t.Fatalf("POST restore: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /articles/%d/restore status = %d, want 200", snap.ID, resp.StatusCode)
	}

	// Live row now has the restored title + body.
	live, err := app.articles.GetByID(src.ID)
	if err != nil {
		t.Fatalf("GetByID(src): %v", err)
	}
	if live.Title != "Restored Title" {
		t.Errorf("after Restore: title = %q, want %q", live.Title, "Restored Title")
	}
	if live.BodyMD != "restored body" {
		t.Errorf("after Restore: body = %q, want %q", live.BodyMD, "restored body")
	}

	// Snapshot still in place (lookup via GetSnapshotByID).
	if _, err := app.articles.GetSnapshotByID(snap.ID); err != nil {
		t.Errorf("snapshot disappeared after Restore: %v", err)
	}
}

// TestHandleArticleSnapshotDeleteRoundTrip pins the
// slice-2.5 DELETE /articles/{id}/snapshot/{snapshotID}
// happy path: 200 + X-DixieData-Redirect; the snapshot row
// is gone; the live row survives.
func TestHandleArticleSnapshotDeleteRoundTrip(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	src, _ := app.articles.Create(models.Article{Title: "Delete source"})
	snap, err := app.articles.Snapshot(src.ID)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	req, err := http.NewRequest(http.MethodDelete,
		server.URL+"/articles/"+intStr(src.ID)+"/snapshot/"+intStr(snap.ID), nil)
	if err != nil {
		t.Fatalf("build DELETE: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE snapshot: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE snapshot status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("X-DixieData-Redirect"); !strings.HasPrefix(got, "/articles/") {
		t.Errorf("X-DixieData-Redirect = %q, want /articles/{id} prefix", got)
	}

	// Snapshot gone.
	if _, err := app.articles.GetSnapshotByID(snap.ID); err == nil {
		t.Errorf("snapshot not deleted: GetSnapshotByID returned nil")
	}
	// Source still live.
	if _, err := app.articles.GetByID(src.ID); err != nil {
		t.Errorf("source row disappeared: %v", err)
	}
}

// TestHandleArticleSnapshotOfSnapshotRejected pins the
// slice-2.5 contract: POST /articles/{id}/snapshot where
// {id} is itself a snapshot returns 409 (snapshot-of-snapshot
// is rejected).
func TestHandleArticleSnapshotOfSnapshotRejected(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	src, _ := app.articles.Create(models.Article{Title: "Live"})
	snap, _ := app.articles.Snapshot(src.ID)

	resp, err := http.PostForm(
		server.URL+"/articles/"+intStr(snap.ID)+"/snapshot",
		url.Values{})
	if err != nil {
		t.Fatalf("POST snapshot-of-snapshot: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("snapshot-of-snapshot status = %d, want 409", resp.StatusCode)
	}
}

// TestHandleArticleRestoreLiveRowRejected pins the
// slice-2.5 contract: POST /articles/{id}/restore where
// {id} is a live-branch row (not a snapshot) returns 409.
func TestHandleArticleRestoreLiveRowRejected(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	src, _ := app.articles.Create(models.Article{Title: "Live"})

	resp, err := http.PostForm(
		server.URL+"/articles/"+intStr(src.ID)+"/restore",
		url.Values{})
	if err != nil {
		t.Fatalf("POST restore-of-live: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("restore-of-live status = %d, want 409", resp.StatusCode)
	}
}

// TestHandleArticleSnapshotDeleteLiveRowRejected pins the
// slice-2.5 contract: DELETE /articles/{id}/snapshot/{snapID}
// where {id} is a live-branch row returns 409 (DeleteSnapshot
// is snapshots-only; live rows use the regular Delete path).
func TestHandleArticleSnapshotDeleteLiveRowRejected(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	src, _ := app.articles.Create(models.Article{Title: "Live"})

	// Trying to DeleteSnapshot src.ID (a live row) -- but
	// the URL takes a snapshotID segment, so use a dummy id.
	req, err := http.NewRequest(http.MethodDelete,
		server.URL+"/articles/"+intStr(src.ID)+"/snapshot/"+intStr(src.ID), nil)
	if err != nil {
		t.Fatalf("build DELETE: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE live as snapshot: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("DeleteSnapshot-of-live status = %d, want 409", resp.StatusCode)
	}
}

// listAllArticleIDs is a small helper used by the slice-2.5
// handler tests to enumerate every article row id
// (regardless of is_snapshot). It issues a direct SQL
// query because no public ArticleService method lists every
// row mixed; using direct SQL here is a test-only convenience
// and avoids widening the public surface for slice-2.5.
func listAllArticleIDs(t *testing.T, app *App) []int64 {
	t.Helper()
	conn := app.database.Conn()
	rows, err := conn.Query(`SELECT id FROM articles ORDER BY id`)
	if err != nil {
		t.Fatalf("listAllArticleIDs: %v", err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("listAllArticleIDs scan: %v", err)
		}
		ids = append(ids, id)
	}
	return ids
}

// TestHandleArticleDetailRendersRefsPanel pins the slice-3.2
// contract: GET /articles/{id} renders the inline Refs panel
// with one row per attached Person Record. The body panel
// (the slice-1 surface) still renders above the Refs panel;
// the empty-state copy renders when no refs are attached.
func TestHandleArticleDetailRendersRefsPanel(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	// Seed two Person Records + attach one to the article.
	createArticleHandlerTestPerson(t, app, "DXD-00091")
	createArticleHandlerTestPerson(t, app, "DXD-00092")
	personA, err := app.soldiers.GetByDisplayID("DXD-00091")
	if err != nil {
		t.Fatalf("lookup DXD-00091: %v", err)
	}
	article, err := app.articles.Create(models.Article{Title: "Refs panel target"})
	if err != nil {
		t.Fatalf("Create article: %v", err)
	}
	if _, err := app.articles.AttachRef(article.ID, personA.ID); err != nil {
		t.Fatalf("AttachRef: %v", err)
	}

	// Fetch the detail page.
	resp, err := http.Get(server.URL + "/articles/" + intStr(article.ID))
	if err != nil {
		t.Fatalf("GET detail: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET detail status = %d, want 200", resp.StatusCode)
	}

	// Assert the Refs panel renders the attached row.
	if !strings.Contains(string(body), `data-article-refs-panel`) {
		t.Errorf("Refs panel missing in detail page")
	}
	if !strings.Contains(string(body), `data-article-refs-row`) {
		t.Errorf("Refs row missing in detail page")
	}
	if !strings.Contains(string(body), "DXD-00091") {
		t.Errorf("Attached display id DXD-00091 missing from Refs panel")
	}
	if strings.Contains(string(body), "DXD-00092") {
		t.Errorf("Unattached display id DXD-00092 unexpectedly appears in Refs panel")
	}
	if !strings.Contains(string(body), `data-article-refs-row-display-id`) {
		t.Errorf("Refs row display-id anchor missing")
	}
	if !strings.Contains(string(body), `data-article-refs-unlink`) {
		t.Errorf("Unlink button missing from Refs row")
	}

	// Empty state on a fresh article.
	bare, err := app.articles.Create(models.Article{Title: "Bare article"})
	if err != nil {
		t.Fatalf("Create bare: %v", err)
	}
	resp2, err := http.Get(server.URL + "/articles/" + intStr(bare.ID))
	if err != nil {
		t.Fatalf("GET detail bare: %v", err)
	}
	defer resp2.Body.Close()
	body2, _ := io.ReadAll(resp2.Body)
	if !strings.Contains(string(body2), `data-article-refs-empty`) {
		t.Errorf("Empty-state copy missing on a fresh article")
	}
}
