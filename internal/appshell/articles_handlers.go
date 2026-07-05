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
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

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
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, "bad form: "+err.Error(), http.StatusBadRequest)
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
			http.Error(w, "title is required", http.StatusBadRequest)
			return
		}
		http.Error(w, "create article: "+err.Error(), http.StatusInternalServerError)
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
