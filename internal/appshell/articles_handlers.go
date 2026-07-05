// articles_handlers.go covers the slice-1 Article Record HTTP
// handlers (issue #321 slice 1). The slice-1 surface is the
// minimum needed to flip the RED test green:
//
//   GET  /articles            -- list page (empty for slice 1;
//                                pinned by future slice 2's
//                                "list non-empty" test).
//   GET  /articles/new        -- editor form page (slice 1: a
//                                minimal form; slice 3 swaps in
//                                the markdown source + sanitized
//                                preview + local-draft-persistence
//                                block).
//   POST /articles/new        -- create handler. The headline slice-1
//                                surface: form posts here, the
//                                handler mints a Display ID via
//                                ArticleService.Create, then writes
//                                the X-DixieData-Redirect header
//                                (per the #341 / Option C convention)
//                                and returns 200 with an empty body.
//                                The client JS then navigates to
//                                /articles/{row-id}.
//   GET  /articles/{id}       -- detail page. The slice-1 surface
//                                renders the title + body verbatim
//                                via the body_html column. Slice 3
//                                adds the Refs panel + the
//                                "Cited in" reverse-lookup.
//   POST /articles/{id}       -- alias POST for any inline form
//                                that posts back here (slice 3+).
//
// Slice 1 deliberately does NOT ship:
//   - /articles/{id}/edit (slice 3)
//   - /articles/{id}/refs/* (slice 3 picker)
//   - /articles/{id}/snapshot, /restore, /delete (slice 2.5)
//   - /articles/{id}/pdf, /raw (slice 4 exports)
package appshell

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/presentation"
	"github.com/valueforvalue/DixieData/internal/records"
	"github.com/valueforvalue/DixieData/internal/routebuilder"
	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

// handleArticles serves GET /articles -- the list page.
// slice 1 ships an empty-state placeholder; slice 2 fills
// in the query + the per-row card list.
func (a *App) handleArticles(w http.ResponseWriter, r *http.Request) {
	if err := presentation.ArticlesListShell().Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
// surface is the title + body verbatim; the slice-3 Refs
// panel + the Reverse-lookup "Cited in" link land later.
// ErrArticleNotFound -> 404 with a short message; other
// read errors -> 500.
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
	if err := presentation.ArticleDetailShell(view).Render(r.Context(), w); err != nil {
		respondInternal(w, r, fmt.Sprintf("Could not render article %d.", id), err)
	}
}
