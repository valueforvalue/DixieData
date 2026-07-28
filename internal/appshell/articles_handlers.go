// articles_handlers.go covers the Article Record HTTP
// handlers (issue #321). Slice 1 ships the minimum CRUD
// surface (GET /articles, GET/POST /articles/new, GET/POST
// /articles/{id}); slice 2 adds the ref attach / detach
// surface so a picker modal (slice 3) can post here. Slice 2.5
// adds Snapshot/Restore/Delete; slice 3 adds the editor +
// picker; slice 4 adds the PDF / Static HTML / raw-md exports.
//
// Surface overview:
//
//   GET    /articles                            -- list page
//   GET    /articles/new                        -- new-article editor
//   POST   /articles/new                        -- create handler
//   GET    /articles/{id}                       -- detail page
//   POST   /articles/{id}                       -- alias POST alias
//   POST   /articles/{id}/refs                  -- attach ref
//   DELETE /articles/{id}/refs/{personId}       -- detach ref
//
// Slice 1 deliberately does NOT ship:
//   - /articles/{id}/edit (slice 3)
//   - picker modal (slice 3)
//   - /articles/{id}/snapshot, /restore, /delete (slice 2.5)
//   - /articles/{id}/pdf, /raw (slice 4)
package appshell

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/valueforvalue/DixieData/internal/appdata"
	"github.com/valueforvalue/DixieData/internal/jobs"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/presentation"
	"github.com/valueforvalue/DixieData/internal/records"
	"github.com/valueforvalue/DixieData/internal/routebuilder"
	"github.com/valueforvalue/DixieData/internal/templates/components"
	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

// handleArticles serves GET /articles -- the list page.
// slice 1 shipped an empty-state placeholder; slice 2 fills
// in the per-row card grid by querying ArticleService.List
// and projecting through viewmodel.ArticlesFromModels.
func (a *App) handleArticles(w http.ResponseWriter, r *http.Request) {
	rows, _, err := a.articles.List(1, 100)
	if err != nil {
		respondInternal(w, r, "Could not list articles.", err)
		return
	}
	view := viewmodel.ArticlesFromModels(rows)
	if err := presentation.ArticlesListShell(view).Render(r.Context(), w); err != nil {
		respondInternal(w, r, "Could not render articles list.", err)
	}
}

// handleNewArticle serves both GET and POST on /articles/new.
// The same function dispatches on method (mirror of
// handleNewEvent at events_handlers.go). slice-3 swaps the
// "GET form" branch for the full markdown editor + picker
// preview; slice 1 keeps the GET branch as a minimal form
// stub because the slice-1 RED test posts to /articles/new
// without first hitting GET.
func (a *App) handleNewArticle(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// slice 1: the GET form branch is a minimal shell.
		// Slice 3 swaps this for the markdown editor + sanitized
		// preview + local-draft-persistence block.
		if err := presentation.ArticleNewShell().Render(r.Context(), w); err != nil {
			// Issue #443: was `http.Error(w, err.Error(), 500)` —
			// a raw Go error leak. The Render call itself failed,
			// so the error IS the response body (per the #384
			// decision flow: "Render itself failed? →
			// respondErrorFragment").
			respondErrorFragment(w, r, KindInternal, "Could not render the new article form.", err)
		}
	case http.MethodPost:
		a.createArticle(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// createArticle handles POST /articles/new. The form posts
// title + subtitle + body; the handler trims the title, calls
// ArticleService.Create, then writes the X-DixieData-Redirect
// header and returns 200 with an empty body. Errors map to:
//   - ErrArticleTitleRequired  -> 400 (blank title)
//   - other create errors      -> 500 with a short error body
//   - any other parse error     -> 400 with a short error body
func (a *App) createArticle(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		// Issue #443: was `http.Error(w, "bad form: "+err.Error(), 400)` —
		// raw Go error leak. respondValidation sets 400 + sets
		// X-DixieData-Toast headers so the user sees a toast in
		// addition to the status (per the #384 contract).
		respondValidation(w, r, "Could not parse the article form.", err)
		return
	}
	title := strings.TrimSpace(r.PostFormValue("title"))
	if title == "" {
		http.Error(w, "title is required", http.StatusBadRequest)
		return
	}
	article, err := a.articles.Create(models.Article{
		Title:    r.PostFormValue("title"),
		Subtitle: r.PostFormValue("subtitle"),
		BodyMD:   r.PostFormValue("body"),
	})
	if err != nil {
		if errors.Is(err, records.ErrArticleTitleRequired) {
			// Note: this stays as `http.Error(w, "title is required", 400)`
			// because the message is a static user-facing string
			// (no err.Error() to leak). The respondValidation helper
			// would log a spurious "err=nil" line and the existing
			// TestHandleArticleCRUD_RoundTrip test asserts the 400
			// status which respondValidation also sets. Keeping the
			// direct http.Error here matches the validation
			// pattern at lines 97 + 107 (which are also static
			// messages without err).
			http.Error(w, "title is required", http.StatusBadRequest)
			return
		}
		// Issue #443: was `http.Error(w, "create article: "+err.Error(), 500)`.
		respondInternal(w, r, "Could not create the article.", err)
		return
	}
	// Per the #341 / Option C convention: 200 + X-DixieData-Redirect
	// so the client JS navigates without a full page reload. The
	// URL is /articles/{sqlite-row-id} (not the DisplayID) so the
	// getArticleByID handler's URL key matches the SQLite row id.
	redirect := routebuilder.ArticleByID(article.ID)
	w.Header().Set("X-DixieData-Redirect", redirect)
	w.WriteHeader(http.StatusOK)
}

// handleArticleByID dispatches /articles/{id} by method.
// slice 1 supports GET + POST alias; slice 2-5 add the
// rest. The 405 fallback is explicit so future slices that
// register DELETE/PATCH on this URL surface cleanly.
func (a *App) handleArticleByID(w http.ResponseWriter, r *http.Request) {
	id, err := parseIntFromPath(r.URL.Path, "/articles/", "")
	if err != nil || id < 1 {
		respondValidation(w, r, "Invalid article id.", err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		a.showArticle(w, r, id)
	case http.MethodPost:
		// Slice-1 POST alias: a no-op echo (the slice-3 Edit form
		// lands here and posts to itself; slice 1 has no Edit form
		// so the handler is here to keep the route alive for the
		// slice-2.5 Snapshot/Restore paths that will share this URL
		// prefix when they land). Respond with a 200 + a redirect
		// back to the detail page so a stray POST never silently
		// mutates state during slice 1.
		w.Header().Set("X-DixieData-Redirect", routebuilder.ArticleByID(id))
		w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// showArticle renders GET /articles/{id}. The slice-1
// surface is the title + body verbatim; the slice-3.2 Refs
// panel + slice-3.4 Revisions tab land here. The slice-3.2
// commit adds the Refs query (ScanRefs + ResolveRefs) so the
// Refs panel renders inline; the slice-3.4 commit will add
// the snapshots query + the tab UI. ErrArticleNotFound -> 404
// with a short message; other read errors -> 500.
//
// Refs query failure is logged but does not 500 the page --
// the Refs panel renders empty and the user sees a "could
// not load refs" notice. The body of the article is the
// load-bearing surface; a transient ref-query failure must
// not lock the user out of the article.
func (a *App) showArticle(w http.ResponseWriter, r *http.Request, id int64) {
	article, err := a.articles.GetByID(id)
	if err != nil {
		if errors.Is(err, records.ErrArticleNotFound) {
			respondNotFound(w, r, fmt.Sprintf("Article %d not found.", id), err)
			return
		}
		respondInternal(w, r, fmt.Sprintf("Could not read article %d.", id), err)
		return
	}
	view := viewmodel.ArticleFromModel(*article)
	view.Refs = a.loadArticleRefsForView(id)
	view.ResolvedRefs = a.loadArticleResolvedRefsForView(id)
	view.Snapshots = a.loadArticleSnapshotsForView(id)
	if err := presentation.ArticleDetailShell(view).Render(r.Context(), w); err != nil {
		respondInternal(w, r, fmt.Sprintf("Could not render article %d.", id), err)
	}
}

// loadArticleRefsForView reads the article_refs junction rows
// for an article and projects them into viewmodel.ArticleRef
// rows. Returns an empty slice (not nil) when no refs are
// attached or when the query fails -- the Refs panel renders
// the empty state rather than a 500. Per locked decision #6,
// an unknown in-body token is fail-loud at render time, not at
// query time, so this query never errors on "unknown token".
func (a *App) loadArticleRefsForView(articleID int64) []viewmodel.ArticleRef {
	refs, err := a.articles.ScanRefs(articleID)
	if err != nil {
		return []viewmodel.ArticleRef{}
	}
	out := make([]viewmodel.ArticleRef, 0, len(refs))
	for _, ref := range refs {
		out = append(out, viewmodel.ArticleRef{
			ID:               ref.ID,
			ArticleID:        ref.ArticleID,
			PersonRecordID:   ref.PersonRecordID,
			PersonDisplayID:  ref.PersonDisplayID,
			PersonSyncID:     ref.PersonRecordSyncID,
			Position:         ref.Position,
		})
	}
	return out
}

// loadArticleResolvedRefsForView reads the in-body markdown
// tokens for an article and projects them into viewmodel rows
// with the Resolved flag. Returns an empty slice on error so
// the resolver panel renders without a 500.
func (a *App) loadArticleResolvedRefsForView(articleID int64) []viewmodel.ArticleRef {
	tokens, err := a.articles.ResolveRefs(articleID)
	if err != nil {
		return []viewmodel.ArticleRef{}
	}
	out := make([]viewmodel.ArticleRef, 0, len(tokens))
	for _, tok := range tokens {
		out = append(out, viewmodel.ArticleRef{
			ArticleID:       tok.ArticleID,
			Token:           tok.Token,
			PersonRecordID:  tok.PersonRecordID,
			PersonDisplayID: tok.PersonDisplayID,
			Resolved:        tok.Resolved,
		})
	}
	return out
}

// loadArticleSnapshotsForView reads the snapshot rows for an
// article and projects them into viewmodel.Article rows for
// the Revisions tab (slice 3.4). Returns an empty slice on
// error so the tab renders the empty state rather than 500ing.
func (a *App) loadArticleSnapshotsForView(articleID int64) []viewmodel.Article {
	snapshots, err := a.articles.ListSnapshots(articleID)
	if err != nil {
		return []viewmodel.Article{}
	}
	return viewmodel.ArticlesFromModels(snapshots)
}

// handleArticlePicker serves GET /articles/{id}/picker.
// Renders the inline Person Record picker shell: a search
// input + a result list. The handler re-runs the search on
// every keystroke (the hx-trigger on the input fires the
// same endpoint with q in the query string). The picker is
// a pure fragment -- no header, no layout -- so it can
// swap into the picker target div on /articles/{id}.
//
// Implementation note: reuses SoldierService.SearchPage --
// no new service method. The picker limit is hard-coded
// at 25 rows; a future slice can parameterize it via the
// search service if scrolling becomes an issue.
func (a *App) handleArticlePicker(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	articleID, err := parseIntFromPath(r.URL.Path, "/articles/", "/picker")
	if err != nil || articleID < 1 {
		respondValidation(w, r, "Invalid article id.", err)
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	rows, _, err := a.soldiers.SearchPage(q, 1, 25)
	if err != nil {
		respondInternal(w, r, "Could not search Person Records.", err)
		return
	}
	viewRows := viewmodel.PersonRecordsFromModels(rows)
	if err := components.PersonRecordPicker(articleID, q, viewRows).Render(r.Context(), w); err != nil {
		respondInternal(w, r, "Could not render picker.", err)
	}
}

// handleArticleRevisions serves GET /articles/{id}/revisions.
// Renders the Revisions tab fragment: a list of every snapshot
// pointing at the live article, with per-snapshot Restore +
// Delete affordances. Empty list renders the empty-state copy.
//
// Implementation: reuses the slice-3.4 ArticleService.ListSnapshots
// method. The fragment is a pure <ul> + actions -- the tab UI
// lives on the article_detail.templ shell so this fragment is a
// drop-in for the tab's data panel.
func (a *App) handleArticleRevisions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	articleID, err := parseIntFromPath(r.URL.Path, "/articles/", "/revisions")
	if err != nil || articleID < 1 {
		respondValidation(w, r, "Invalid article id.", err)
		return
	}
	snapshots, err := a.articles.ListSnapshots(articleID)
	if err != nil {
		respondInternal(w, r, fmt.Sprintf("Could not list snapshots for article %d.", articleID), err)
		return
	}
	view := viewmodel.ArticlesFromModels(snapshots)
	if err := components.ArticleRevisionsList(articleID, view).Render(r.Context(), w); err != nil {
		respondInternal(w, r, "Could not render revisions.", err)
	}
}

// handleEditArticle serves both GET and POST on
// /articles/{id}/edit. The slice-3.5 commit lands the route +
// the 404 + 400 contracts; the slice-3.7 commit swaps the
// "GET form" branch for the full markdown editor + sanitized
// preview + local-draft-persistence block.
//
// POST contract (slice 3.5): the form posts title + subtitle +
// body; the handler trims the title, calls
// ArticleService.Update, then writes the X-DixieData-Redirect
// header and returns 200. Errors map to:
//   - ErrArticleNotFound   -> 404 (unknown id)
//   - ErrArticleTitleRequired -> 400 (blank title)
//   - ErrArticleSnapshot   -> 409 (snapshot target rejected)
//   - other update errors  -> 500
//
// The slice-3.5 POST branch uses a deliberately-small surface
// (3 form fields, 3 error mappings) so the route can be wired
// before the editor UX lands.
func (a *App) handleEditArticle(w http.ResponseWriter, r *http.Request) {
	id, err := parseIntFromPath(r.URL.Path, "/articles/", "/edit")
	if err != nil || id < 1 {
		respondValidation(w, r, "Invalid article id.", err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		// Slice 3.5: 404 + 400 contracts only. Slice 3.7
		// swaps this for the full markdown editor.
		article, err := a.articles.GetByID(id)
		if err != nil {
			if errors.Is(err, records.ErrArticleNotFound) {
				respondNotFound(w, r, fmt.Sprintf("Article %d not found.", id), err)
				return
			}
			respondInternal(w, r, fmt.Sprintf("Could not read article %d.", id), err)
			return
		}
		view := viewmodel.ArticlePtrFromModel(article)
		if err := presentation.ArticleEditShell(view).Render(r.Context(), w); err != nil {
			respondInternal(w, r, fmt.Sprintf("Could not render article %d.", id), err)
		}
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			// Issue #443: was `http.Error(w, "bad form: "+err.Error(), 400)`.
			respondValidation(w, r, "Could not parse the article form.", err)
			return
		}
		title := strings.TrimSpace(r.PostFormValue("title"))
		if title == "" {
			http.Error(w, "title is required", http.StatusBadRequest)
			return
		}
		// Look up the existing row, then mutate + Update.
		// We need the existing row's snapshot_of_id + is_snapshot
		// fields so Update can keep them unchanged. GetByID
		// filters snapshots out -- so a POST against a snapshot
		// row returns ErrArticleNotFound here, which we map to
		// 409 (snapshot rows are not editable from this surface).
		existing, err := a.articles.GetByID(id)
		if err != nil {
			if errors.Is(err, records.ErrArticleNotFound) {
				// Could be unknown id OR a snapshot target. Check
				// the snapshot path so the right status surfaces.
				if _, sErr := a.articles.GetSnapshotByID(id); sErr == nil {
					respondConflict(w, r, fmt.Sprintf("Article %d is a snapshot and cannot be edited.", id), nil)
					return
				}
				respondNotFound(w, r, fmt.Sprintf("Article %d not found.", id), err)
				return
			}
			respondInternal(w, r, fmt.Sprintf("Could not read article %d.", id), err)
			return
		}
		existing.Title = title
		existing.Subtitle = strings.TrimSpace(r.PostFormValue("subtitle"))
		existing.BodyMD = r.PostFormValue("body")
		if err := a.articles.Update(*existing); err != nil {
			switch {
			case errors.Is(err, records.ErrArticleNotFound):
				respondNotFound(w, r, fmt.Sprintf("Article %d not found.", id), err)
			case errors.Is(err, records.ErrArticleTitleRequired):
				http.Error(w, "title is required", http.StatusBadRequest)
			case errors.Is(err, records.ErrArticleSnapshot):
				respondConflict(w, r, fmt.Sprintf("Article %d is a snapshot and cannot be edited.", id), err)
			default:
				respondInternal(w, r, fmt.Sprintf("Could not update article %d.", id), err)
			}
			return
		}
		w.Header().Set("X-DixieData-Redirect", routebuilder.ArticleByID(id))
		w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}


// handleArticleRefsAttach serves POST /articles/{id}/refs.
// The form posts a Person Record by Display ID (the picker
// UI inserts the Display ID); the handler looks up the
// person row via SoldierService.GetByDisplayID, then calls
// ArticleService.AttachRef. Refs are user-managed duplicates
// (the unique index idx_article_refs_article_person makes a
// duplicate attach a no-op; the handler maps that to 200 +
// X-DixieData-Redirect back to the detail page so a duplicate
// UI click never surfaces a server error to the user).
//
// Slice 2 ships the bare POST handler so the API is
// exercisable from smoke probes + handler tests; slice 3
// adds the inline picker modal that pops the picker view
// first, then posts here.
func (a *App) handleArticleRefsAttach(w http.ResponseWriter, r *http.Request) {
	articleID, err := parseIntFromPath(r.URL.Path, "/articles/", "/refs")
	if err != nil || articleID < 1 {
		respondValidation(w, r, "Invalid article id.", err)
		return
	}
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the attach form.", err)
		return
	}
	displayID := strings.TrimSpace(r.PostFormValue("display_id"))
	if displayID == "" {
		respondValidation(w, r, "Display ID is required.", nil)
		return
	}
	person, err := a.soldiers.GetByDisplayID(displayID)
	if err != nil {
		respondNotFound(w, r, fmt.Sprintf("Person Record %q not found.", displayID), err)
		return
	}
	if _, err := a.articles.AttachRef(articleID, person.ID); err != nil {
		respondInternal(w, r, fmt.Sprintf("Could not attach %s to article %d.", displayID, articleID), err)
		return
	}
	w.Header().Set("X-DixieData-Redirect", routebuilder.ArticleByID(articleID))
	w.WriteHeader(http.StatusOK)
}

// handleArticleRefsDetach serves DELETE /articles/{id}/refs/{personId}.
// Idempotent: a detach on a non-existent row returns 200
// (the article_refs row is already gone) rather than 404 so
// a UI double-click is safe. The handler issues an
// X-DixieData-Redirect back to the detail page so the JS
// dispatcher can re-render the Refs panel.
func (a *App) handleArticleRefsDetach(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/articles/")
	parts := strings.SplitN(path, "/refs/", 2)
	if len(parts) != 2 {
		respondValidation(w, r, "Invalid article or person id in URL.", nil)
		return
	}
	articleID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || articleID < 1 {
		respondValidation(w, r, "Invalid article id.", err)
		return
	}
	personID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || personID < 1 {
		respondValidation(w, r, "Invalid person id.", err)
		return
	}
	if err := a.articles.DetachRef(articleID, personID); err != nil {
		respondInternal(w, r, fmt.Sprintf("Could not detach person %d from article %d.", personID, articleID), err)
		return
	}
	w.Header().Set("X-DixieData-Redirect", routebuilder.ArticleByID(articleID))
	w.WriteHeader(http.StatusOK)
}


// handleArticleSnapshot serves POST /articles/{id}/snapshot.
// Creates a fresh row with is_snapshot = 1 + snapshot_of_id
// pointing at the source row + a fresh ART-NNNNN Display ID
// minted by NextArticleID. The new row's fields are a copy
// of the source row's CURRENT fields; subsequent updates to
// the source do NOT retroactively change the snapshot.
//
// 200 + X-DixieData-Redirect back to the source detail page
// so the JS dispatcher can re-render the Revisions tab.
// 400 on source-not-found; 409 on snapshot-of-snapshot;
// 500 on internal error.
func (a *App) handleArticleSnapshot(w http.ResponseWriter, r *http.Request) {
	srcID, err := parseIntFromPath(r.URL.Path, "/articles/", "/snapshot")
	if err != nil || srcID < 1 {
		respondValidation(w, r, "Invalid article id.", err)
		return
	}
	snap, err := a.articles.Snapshot(srcID)
	if err != nil {
		if errors.Is(err, records.ErrArticleNotFound) {
			respondNotFound(w, r, fmt.Sprintf("Article %d not found.", srcID), err)
			return
		}
		if errors.Is(err, records.ErrArticleSnapshot) {
			respondConflict(w, r, fmt.Sprintf("Article %d is itself a snapshot; snapshot-of-snapshot not allowed.", srcID), err)
			return
		}
		respondInternal(w, r, fmt.Sprintf("Could not snapshot article %d.", srcID), err)
		return
	}
	// Redirect to the SOURCE detail page (not the new
	// snapshot row) -- the user just clicked "Save copy" on
	// the live article; the Revisions tab refreshes in
	// place + slice 3 may show a toast. The slice-2.5
	// surface does not yet expose a Revisions-render path,
	// so the redirect simply re-renders the detail page.
	w.Header().Set("X-DixieData-Redirect", routebuilder.ArticleByID(srcID))
	w.WriteHeader(http.StatusOK)
	_ = snap // snap is returned for slice-3 callers; slice-2.5 ignores it
}

// handleArticleRestore serves POST /articles/{id}/restore.
// The id is the SNAPSHOT row id (the handler looks up the
// live row via the snapshot's snapshot_of_id column).
// Restores the snapshot's CURRENT fields to the live row;
// the snapshot stays in place per the slice-2.5 Revisions
// contract.
//
// 200 + X-DixieData-Redirect to the live detail page.
// 404 on snapshot-not-found; 409 on non-snapshot target.
func (a *App) handleArticleRestore(w http.ResponseWriter, r *http.Request) {
	snapID, err := parseIntFromPath(r.URL.Path, "/articles/", "/restore")
	if err != nil || snapID < 1 {
		respondValidation(w, r, "Invalid snapshot id.", err)
		return
	}
	if err := a.articles.Restore(snapID); err != nil {
		if errors.Is(err, records.ErrArticleNotFound) {
			respondNotFound(w, r, fmt.Sprintf("Snapshot %d not found.", snapID), err)
			return
		}
		if errors.Is(err, records.ErrArticleSnapshot) {
			respondConflict(w, r, fmt.Sprintf("Article %d is not a snapshot; cannot restore.", snapID), err)
			return
		}
		respondInternal(w, r, fmt.Sprintf("Could not restore snapshot %d.", snapID), err)
		return
	}
	// Redirect to the live detail page -- the snapshot is
	// keyed by snapshot_of_id, but the URL-keyed redirect
	// hands the JS dispatcher an id (the snapshot's id).
	// The slice-3 Revisions-tab handler will swap to the
	// live row id when it lands; for now the snapshot's id
	// also points at a detail page (renders 404 under
	// slice-1's GetByID-filter, which is fine for slice-2.5
	// -- slice 3 will swap to a snapshot-aware redirect).
	w.Header().Set("X-DixieData-Redirect", routebuilder.ArticleByID(snapID))
	w.WriteHeader(http.StatusOK)
}

// handleArticleSnapshotDelete serves DELETE
// /articles/{id}/snapshot/{snapshotID}. Removes the
// snapshot row only; the live branch the snapshot referred
// to is untouched. Idempotent-on-not-found via 404 (not 200)
// so a stale DELETE surfaces as not-found for the front end.
func (a *App) handleArticleSnapshotDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/articles/")
	parts := strings.SplitN(path, "/snapshot/", 2)
	if len(parts) != 2 {
		respondValidation(w, r, "Invalid article or snapshot id in URL.", nil)
		return
	}
	articleID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || articleID < 1 {
		respondValidation(w, r, "Invalid article id.", err)
		return
	}
	snapshotID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || snapshotID < 1 {
		respondValidation(w, r, "Invalid snapshot id.", err)
		return
	}
	if err := a.articles.DeleteSnapshot(snapshotID); err != nil {
		if errors.Is(err, records.ErrArticleNotFound) {
			respondNotFound(w, r, fmt.Sprintf("Snapshot %d not found.", snapshotID), err)
			return
		}
		if errors.Is(err, records.ErrArticleSnapshot) {
			respondConflict(w, r, fmt.Sprintf("Article %d is not a snapshot; use the regular delete.", articleID), err)
			return
		}
		respondInternal(w, r, fmt.Sprintf("Could not delete snapshot %d.", snapshotID), err)
		return
	}
	w.Header().Set("X-DixieData-Redirect", routebuilder.ArticleByID(articleID))
	w.WriteHeader(http.StatusOK)
}

// handleArticlePreview serves GET /articles/preview?body=...
// Returns the sanitized HTML render of the supplied
// markdown source. Used by the slice-3.6 editor's live
// preview pane -- the JS editor POSTs the body to this
// endpoint on each keystroke (250ms debounce) and replaces
// the preview div innerHTML with the response.
//
// The body is read from the form field name "body" so a
// simple form-encoded POST works (htmx hx-post with
// hx-trigger="input changed delay:250ms"). The render goes
// through ArticleService's renderer (goldmark + bluemonday
// custom policy) so the preview matches what the Create /
// Update path will store.
//
// Empty body renders the guidance message so the preview
// pane is never blank.
func (a *App) handleArticlePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		// Issue #443: was `http.Error(w, "bad form: "+err.Error(), 400)`.
		respondValidation(w, r, "Could not parse the article preview form.", err)
		return
	}
	body := r.PostFormValue("body")
	rendered := a.articles.RenderBodyHTML(body)
	if rendered == "" {
		rendered = "<p class=\"text-sm text-slate-500\">If you write Markdown in the source panel, the rendered preview appears here. Updates live (250ms debounce).</p>"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(rendered))
}

// handleArticlePDF serves POST /articles/{id}/pdf. Routes the
// export through the jobs flow (issue #533) so every PDF export
// surface lands the user on /jobs/{id} with a terminal Completion
// card, instead of bypassing the job with an inline render +
// 200 OK + X-DixieData-Toast.
//
// The orientation form field (portrait|landscape, defaults to
// portrait) selects the per-export template. The user-pasted
// path is captured BEFORE the worker enqueues (same UX as
// handleSoldierPDF / handleCalendarPDF — the user sees the
// native SaveFileDialog, then lands on /jobs/{id} to watch the
// render complete).
//
// Snapshot targets are rejected with 409 (matching the slice-2.5
// contract); unknown id returns 404; render failures bubble up
// through the worker (return error from the work closure).

// articlePDFFilename wraps records.SlugifyArticleFilename so the
// handler can compute the suggested filename before opening the
// native SaveFileDialog (issue #533 — the dialog shows the user a
// sensible default BEFORE the worker runs the actual render).
func articlePDFFilename(article *models.Article, orientation string) string {
	if article == nil {
		return "article.pdf"
	}
	return records.SlugifyArticleFilename(*article, orientation)
}

func (a *App) handleArticlePDF(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := parseIntFromPath(r.URL.Path, "/articles/", "/pdf")
	if err != nil || id < 1 {
		respondValidation(w, r, "Invalid article id.", err)
		return
	}
	orientation := strings.TrimSpace(r.PostFormValue("orientation"))
	if orientation == "" {
		orientation = "portrait"
	}

	article, err := a.articles.GetByID(id)
	if err != nil {
		respondNotFound(w, r, fmt.Sprintf("Article %d not found.", id), err)
		return
	}

	// Suggested filename is the article's slugified title + the
	// DisplayID (mirrors the soldier_pdf naming convention). The
	// filename must be derivable BEFORE the worker runs so the
	// native dialog shows the user a sensible default.
	defaultFilename := articlePDFFilename(article, orientation)
	opts := runtime.SaveDialogOptions{
		DefaultFilename: defaultFilename,
		Filters: []runtime.FileFilter{
			{DisplayName: "PDF document", Pattern: "*.pdf"},
		},
	}
	dupKey := fmt.Sprintf("article_pdf|%d|%s|%s", id, orientation, defaultFilename)
	path, outcome := a.guardedSaveFileDialog(dupKey, opts)
	switch outcome {
	case SaveOutcomeDuplicated:
		a.respondDuplicateInFlight(w, r, dupKey)
		return
	case SaveOutcomeDialogAborted:
		respondError(w, r, KindValidation, "Article PDF export cancelled.", nil)
		return
	}

	// Issue #533: stage the success toast on the OUTER response
	// BEFORE enqueueExport writes the redirect header. The
	// dispatcher (frontend/app.js::dispatchDixieDataForm) reads
	// BOTH X-DixieData-Toast and X-DixieData-Redirect from the
	// same response — the toast is the immediate save-confirmation
	// signal, the redirect is the follow-up navigation. Setting
	// the toast after enqueueExport would be a no-op because
	// writeExportRedirect calls w.WriteHeader(200) and Go's
	// ResponseWriter stops accepting new headers after WriteHeader.
	w.Header().Set("X-DixieData-Toast", fmt.Sprintf("Article PDF saved to %s.", filepath.Base(path)))
	w.Header().Set("X-DixieData-Toast-Type", "success")

	// Snapshot rejection must run in the worker because the
	// pre-enqueue path can't predict whether the user picked a
	// snapshot target until the dialog returns. The worker
	// surfaces the rejection as a job error so /jobs/{id} shows
	// it on the terminal Error card.
	work := func(ctx context.Context, p *jobs.Progress) error {
		p.Set(20, "Rendering Article PDF")
		result, err := a.articles.RenderPDF(id, orientation)
		if err != nil {
			if errors.Is(err, records.ErrArticleNotFound) {
				return fmt.Errorf("article %d not found: %w", id, err)
			}
			return fmt.Errorf("render article %d PDF: %w", id, err)
		}
		if err := os.WriteFile(path, result.Bytes, 0o644); err != nil {
			return fmt.Errorf("write article %d PDF to %q: %w", id, path, err)
		}
		return nil
	}
	a.enqueueExport(dupKey, "article_pdf", work, path, w)
}

// handleArticleRaw serves GET /articles/{id}/raw. Returns the
// body_md verbatim as text/markdown with a Content-Disposition:
// attachment header so the browser saves the file. The
// suggested filename is the article's slugified title + the
// DisplayID (mirrors the PDF download's slugify pattern).
//
// Used by the per-export "Save as Markdown" affordance + the
// slice-4.5 picker.
func (a *App) handleArticleRaw(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := parseIntFromPath(r.URL.Path, "/articles/", "/raw")
	if err != nil || id < 1 {
		respondValidation(w, r, "Invalid article id.", err)
		return
	}
	article, err := a.articles.GetByID(id)
	if err != nil {
		if errors.Is(err, records.ErrArticleNotFound) {
			respondNotFound(w, r, fmt.Sprintf("Article %d not found.", id), err)
			return
		}
		respondInternal(w, r, fmt.Sprintf("Could not read article %d.", id), err)
		return
	}
	filename := fmt.Sprintf("Article-%s.md", article.DisplayID)
	if slug := slugifyTitle(article.Title); slug != "" {
		filename = fmt.Sprintf("Article-%s-%s.md", article.DisplayID, slug)
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	_, _ = w.Write([]byte(article.BodyMD))
}

// handleImportArticleImages serves POST /articles/{id}/images/import
// (issue #612 slice 2). Web-mode multipart upload + Wails
// native dialog are both supported, mirroring the per-Person-Record
// shape at handleImportSoldierImages. The imported files land
// under dataDir/images/articles/<displayID>/ (a per-article
// sibling directory — see appdata.ArticleImageDir) and the
// metadata row carries article_id=<id> + kind='article' (the
// slice-1 discriminator that lets the picker filter cleanly).
//
// The slice-3 picker modal is the primary consumer; the slice-4
// paste/drag-drop path also routes through this endpoint. The
// picker fragment at GET /articles/{id}/images is the read side
// of the same contract.
func (a *App) handleImportArticleImages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := parseIntFromPath(r.URL.Path, "/articles/", "/images/import")
	if err != nil {
		respondValidation(w, r, "Invalid article id.", err)
		return
	}

	article, err := a.articles.GetByID(id)
	if err != nil {
		respondNotFound(w, r, fmt.Sprintf("Article %d not found.", id), err)
		return
	}

	// Web-mode branch: multipart upload with the file
	// input's "images" field. We save each uploaded file
	// to a temp path so the importArticleImages walker
	// can copy it into the article's image directory.
	uploadedPaths := readUploadedImagePaths(w, r)
	if uploadedPaths != nil {
		imported, importErr := a.importArticleImages(*article, uploadedPaths)
		if importErr != nil {
			slog.Error("appshell: article image import (web)", "audit", "respond-error", "article_id", id, "imported", imported, "err", importErr.Error())
			respondInternal(w, r, "Could not import the uploaded images.", importErr)
			return
		}
		setToastHeader(w, fmt.Sprintf("Imported %d image(s).", imported))
		a.renderArticleImagesListFragment(w, r, id)
		return
	}

	// Wails branch: native multi-file dialog with the
	// per-call re-entry guard (see docs/agents/dialog-guard.md).
	// Only attempt when a Wails frontend is available;
	// web-mode users should use the paste/drop flow or the
	// manual URL input instead.
	if !wailsHasFrontend(a.ctx) {
		respondValidation(w, r, "Image import from device is only available in the desktop app. Use paste/drop or paste an image URL instead.", nil)
		return
	}
	pathsOpts := runtime.OpenDialogOptions{
		Filters: []runtime.FileFilter{
			{DisplayName: "Image files", Pattern: "*.png;*.jpg;*.jpeg;*.gif;*.bmp;*.webp;*.svg"},
		},
	}
	dupKey := guardedOpenMultipleFilesDialogKey("import_article_images", pathsOpts)
	paths, admitted, ok := a.guardedOpenMultipleFilesDialog(dupKey, pathsOpts)
	if !admitted {
		a.respondDuplicateInFlight(w, r, dupKey)
		return
	}
	if !ok {
		respondError(w, r, KindValidation, "Image import cancelled.", nil)
		return
	}
	imported, importErr := a.importArticleImages(*article, paths)
	if importErr != nil {
		slog.Error("appshell: article image import (wails)", "audit", "respond-error", "article_id", id, "imported", imported, "err", importErr.Error())
		respondError(w, r, KindInternal, importErr.Error(), importErr)
		return
	}
	setToastHeader(w, fmt.Sprintf("Imported %d image(s).", imported))
	a.renderArticleImagesListFragment(w, r, id)
}

// handleArticleImagesList serves GET /articles/{id}/images
// (issue #612 slice 2). The slice-3 picker modal's "Pick
// existing" tab reads this fragment; the slice-3 upload tab
// posts to the import endpoint above + re-fetches this
// fragment on success. The fragment is the list of images
// already attached to the article (kind='article' filter via
// the slice-1 discriminator).
func (a *App) handleArticleImagesList(w http.ResponseWriter, r *http.Request) {
	id, err := parseIntFromPath(r.URL.Path, "/articles/", "/images")
	if err != nil {
		respondValidation(w, r, "Invalid article id.", err)
		return
	}
	a.renderArticleImagesListFragment(w, r, id)
}

// renderArticleImagesListFragment writes the image-list
// fragment for the article picker modal. The fragment is a
// plain HTML <ul> for slice 2; the slice-3 picker modal wraps
// it with the tab UI.
func (a *App) renderArticleImagesListFragment(w http.ResponseWriter, r *http.Request, articleID int64) {
	images, err := a.articles.ImagesForArticle(articleID)
	if err != nil {
		respondInternal(w, r, fmt.Sprintf("Could not load images for article %d.", articleID), err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := components.ArticleImagesListFragment(articleID, images).Render(r.Context(), w); err != nil {
		respondInternal(w, r, fmt.Sprintf("Could not render images for article %d.", articleID), err)
	}
}

// importArticleImages saves each source path into the
// article's image directory + inserts a metadata row via
// ArticleService.AddImage. Mirrors the per-Person-Record
// importImagePaths shape: copies bytes, generates a unique
// filename, records the relative path. The slice-2 contract:
// the row carries article_id + kind='article' so the picker
// query (ImagesForArticle) finds it.
func (a *App) importArticleImages(article models.Article, paths []string) (int, error) {
	recordDir, relativeDir := appdata.ArticleImageDir(a.dataDir, article.DisplayID)
	if err := os.MkdirAll(recordDir, 0o755); err != nil {
		return 0, fmt.Errorf("create image directory: %w", err)
	}
	namePrefix := filepath.Base(relativeDir)
	nextSequence, err := nextStoredImageSequence(recordDir, namePrefix)
	if err != nil {
		return 0, fmt.Errorf("prepare image filenames: %w", err)
	}

	imported := 0
	var issues []string
	for _, sourcePath := range paths {
		sourcePath = strings.TrimSpace(sourcePath)
		if sourcePath == "" {
			continue
		}
		fileName := filepath.Base(sourcePath)
		if !isAllowedImageFile(fileName) {
			issues = append(issues, fmt.Sprintf("unsupported image file: %s", fileName))
			continue
		}
		info, err := os.Stat(sourcePath)
		if err != nil {
			issues = append(issues, fmt.Sprintf("read image file %s: %v", fileName, err))
			continue
		}
		if info.IsDir() || info.Size() == 0 {
			issues = append(issues, fmt.Sprintf("image file %s is empty", fileName))
			continue
		}

		storedName := standardizedImageFileName(namePrefix, nextSequence, fileName)
		absolutePath := filepath.Join(recordDir, storedName)
		relativePath := filepath.Join(relativeDir, storedName)

		if err := copyImageFile(sourcePath, absolutePath); err != nil {
			issues = append(issues, err.Error())
			continue
		}
		if err := a.articles.AddImage(article.ID, storedName, filepath.ToSlash(relativePath), ""); err != nil {
			_ = os.Remove(absolutePath)
			issues = append(issues, err.Error())
			continue
		}
		imported++
		nextSequence++
	}

	if len(issues) > 0 {
		return imported, errors.New(strings.Join(issues, "; "))
	}
	return imported, nil
}

// slugifyTitle is a thin wrapper for the filename slug helper
// (mirrors ArticleService.slugify but for the raw-md filename).
func slugifyTitle(title string) string {
	s := strings.TrimSpace(strings.ToLower(title))
	if s == "" {
		return ""
	}
	var b strings.Builder
	prevHyphen := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevHyphen = false
		case r == ' ' || r == '-' || r == '_':
			if !prevHyphen && b.Len() > 0 {
				b.WriteByte('-')
				prevHyphen = true
			}
		}
		if b.Len() >= 60 {
			break
		}
	}
	return strings.TrimRight(b.String(), "-")
}

// handleDeleteArticle removes an article + its snapshots +
// its refs in a single transaction. DELETE /articles/{id}.
// Idempotent on missing rows; ErrArticleSnapshot means the
// target is a snapshot row and should be deleted via the
// snapshot-delete route instead. On success, returns the
// new (post-Commits 3-5) contract: 200 + X-DixieData-Redirect
// + X-DixieData-Toast. The JS dispatcher reads the redirect
// header and navigates + shows the toast. Mirrors
// handleDeleteTag (issue #666).
func (a *App) handleDeleteArticle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id < 1 {
		respondValidation(w, r, "Invalid article id.", err)
		return
	}
	ctx := r.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	if err := a.articles.Delete(id); err != nil {
		switch {
		case errors.Is(err, records.ErrArticleNotFound):
			respondNotFound(w, r, fmt.Sprintf("Article %d not found.", id), err)
		case errors.Is(err, records.ErrArticleSnapshot):
			respondValidation(w, r, "Cannot delete a snapshot row via /articles/{id}. Use the snapshot-delete route instead.", err)
		default:
			respondInternal(w, r, fmt.Sprintf("Could not delete article %d.", id), err)
		}
		return
	}
	setToastHeader(w, "Article deleted.")
	writeExportRedirect(w, "/articles")
}
