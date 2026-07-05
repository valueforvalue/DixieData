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
