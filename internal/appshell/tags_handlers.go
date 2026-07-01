package appshell

// Person Record tagging HTTP handlers (issue #183). Mirrors the
// domain shape in internal/records/tag_service.go and the
// handler discipline in export_templates_handlers.go:
//
//   - JSON endpoints use writeExportRedirect so the post-then-
//     navigate redirect contract from issue #130 holds (see
//     TestPostThenNavigateUsesDixieRedirect).
//   - Fragments (autocomplete, attach/detach row) return HTML
//     partials the existing JS handlers already know how to
//     swap into the DOM.
//   - /tags page rendering is a follow-up (commit 5 / template
//     work). For now the management page handler is a stub that
//     501s — the spec requires the route to exist but rendering
//     waits for the templ work.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/valueforvalue/DixieData/internal/records"
)

// parseSoldierIDFromPath extracts the {id} segment from URLs of
// the form "/soldiers/{id}/tags[/...]". Returns 0 + an error when
// the URL doesn't carry a numeric id; the handler maps that to
// 400 with a caller-specific message.
func parseSoldierIDFromPath(path string) (int64, error) {
	// /soldiers/{id}/tags[/...]
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 || parts[0] != "soldiers" {
		return 0, fmt.Errorf("unexpected path shape: %s", path)
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid soldier id: %w", err)
	}
	return id, nil
}

// parseTagIDFromPath extracts {tagId} from "/soldiers/{id}/tags/{tagId}".
func parseTagIDFromPath(path string) (int64, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 4 || parts[0] != "soldiers" || parts[2] != "tags" {
		return 0, fmt.Errorf("unexpected path shape: %s", path)
	}
	id, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid tag id: %w", err)
	}
	return id, nil
}

// parseTagIDFromTagPath extracts {tagId} from "/tags/{id}[/...]".
func parseTagIDFromTagPath(path string) (int64, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 || parts[0] != "tags" {
		return 0, fmt.Errorf("unexpected path shape: %s", path)
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid tag id: %w", err)
	}
	return id, nil
}

// handleTagAutocomplete returns a small HTML fragment with up to N
// tags whose normalized name contains `q`. Used by the picker
// in commit 5; the HTMX endpoint mounts at
// GET /soldiers/{id}/tags?autocomplete={q}.
//
// The soldier id is required because the same fragment shape
// must be renderable in a per-soldier picker context. Any
// authorized caller can use the same fragment, so we accept the
// id but only use it for context (the picker shape is identical
// for every soldier).
func (a *App) handleTagAutocomplete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	soldierID, err := parseSoldierIDFromPath(r.URL.Path)
	if err != nil {
		http.Error(w, "invalid soldier id", http.StatusBadRequest)
		return
	}
	q := r.URL.Query().Get("autocomplete")
	ctx := r.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	hits, err := a.tags.Autocomplete(ctx, q, 20)
	if err != nil {
		respondInternal(w, r, "Could not list tags for autocomplete.", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Render each hit as an <option> so the picker <select> can
	// swap them in directly. Empty state: a single disabled
	// placeholder.
	if len(hits) == 0 {
		_, _ = fmt.Fprintf(w, `<option value="" disabled>%s</option>`,
			escapeTagOption(emptyStateText(q)))
		return
	}
	for _, t := range hits {
		_, _ = fmt.Fprintf(w,
			`<option value=%q data-tag-id="%d">%s</option>`,
			t.NormalizedName, t.ID, escapeTagOption(t.Name))
	}
	_ = soldierID
}

// handleAttachTag binds a tag (specified by name in the form) to
// a Person Record. POST /soldiers/{id}/tags. Idempotent — repeat
// submissions are no-ops. Returns 200 OK + X-DixieData-Redirect
// to the soldier detail page so the picker fragment can swap
// through the standard dispatcher.
func (a *App) handleAttachTag(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	soldierID, err := parseSoldierIDFromPath(r.URL.Path)
	if err != nil {
		http.Error(w, "invalid soldier id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the tag form.", err)
		return
	}
	tagName := strings.TrimSpace(r.FormValue("tag_name"))
	tagIDRaw := strings.TrimSpace(r.FormValue("tag_id"))
	ctx := r.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	var tag records.Tag
	switch {
	case tagIDRaw != "":
		id, perr := strconv.ParseInt(tagIDRaw, 10, 64)
		if perr != nil {
			respondValidation(w, r, "Invalid tag id.", perr)
			return
		}
		tag, err = a.tags.Get(ctx, id)
		if err != nil {
			respondNotFound(w, r, fmt.Sprintf("Tag %d not found.", id), err)
			return
		}
	case tagName != "":
		tag, err = a.tags.UpsertByName(ctx, tagName)
		if err != nil {
			if errors.Is(err, records.ErrTagNameTaken) {
				respondValidation(w, r, "Tag name already exists.", err)
				return
			}
			respondInternal(w, r, "Could not resolve the tag.", err)
			return
		}
	default:
		respondValidation(w, r, "Either tag_name or tag_id is required.", nil)
		return
	}
	if err := a.tags.Attach(ctx, tag.ID, soldierID); err != nil {
		respondInternal(w, r, "Could not attach the tag.", err)
		return
	}
	writeExportRedirect(w, fmt.Sprintf("/soldiers/%d", soldierID))
}

// handleDetachTag removes a binding. POST /soldiers/{id}/tags/{tagId}.
// Idempotent. Target = soldier detail page.
func (a *App) handleDetachTag(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	soldierID, err := parseSoldierIDFromPath(r.URL.Path)
	if err != nil {
		http.Error(w, "invalid soldier id", http.StatusBadRequest)
		return
	}
	tagID, err := parseTagIDFromPath(r.URL.Path)
	if err != nil {
		http.Error(w, "invalid tag id", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	if err := a.tags.Detach(ctx, tagID, soldierID); err != nil {
		respondInternal(w, r, fmt.Sprintf("Could not detach tag %d.", tagID), err)
		return
	}
	writeExportRedirect(w, fmt.Sprintf("/soldiers/%d", soldierID))
}

// handleBulkTagFromBrowse binds one tag (by name) to every
// selected_ids entry from the Browse selection toolbar. POST
// /browse/bulk-tag. The form carries selected_ids as a
// repeated field (one per row) and a single tag_name input.
func (a *App) handleBulkTagFromBrowse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the bulk-tag form.", err)
		return
	}
	tagName := strings.TrimSpace(r.FormValue("tag_name"))
	if tagName == "" {
		respondValidation(w, r, "Tag name is required.", nil)
		return
	}
	raw := r.Form["selected_ids"]
	if len(raw) == 0 {
		respondValidation(w, r, "At least one selected_ids entry is required.", nil)
		return
	}
	ids := make([]int64, 0, len(raw))
	for _, s := range raw {
		n, perr := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		if perr != nil {
			respondValidation(w, r, fmt.Sprintf("Invalid selected_ids entry: %q.", s), perr)
			return
		}
		ids = append(ids, n)
	}
	ctx := r.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	tag, err := a.tags.UpsertByName(ctx, tagName)
	if err != nil {
		if errors.Is(err, records.ErrTagNameTaken) {
			respondValidation(w, r, "Tag name already exists.", err)
			return
		}
		respondInternal(w, r, "Could not resolve the tag.", err)
		return
	}
	attached, err := a.tags.AttachMany(ctx, tag.ID, ids)
	if err != nil {
		respondInternal(w, r, "Could not apply the tag to every selection.", err)
		return
	}
	// Redirect back to Browse so the toolbar / bulk-action surface
	// can pick up the new chips via the next render. The toolbar
	// uses a hidden ?bulk_tag=1 fragment in a follow-up so users
	// see the toast without a full re-render.
	w.Header().Set("X-DixieData-Tag-Attached", strconv.Itoa(attached))
	writeExportRedirect(w, "/browse?bulk_tag="+strconv.FormatInt(tag.ID, 10))
}

// handleRenameTag renames the display name on a tag id. POST
// /tags/{id}/rename. Form carries `tag_name`. 409 on collision.
func (a *App) handleRenameTag(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tagID, err := parseTagIDFromTagPath(r.URL.Path)
	if err != nil {
		http.Error(w, "invalid tag id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the rename form.", err)
		return
	}
	tagName := strings.TrimSpace(r.FormValue("tag_name"))
	if tagName == "" {
		respondValidation(w, r, "Tag name is required.", nil)
		return
	}
	ctx := r.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	renamed, err := a.tags.Rename(ctx, tagID, tagName)
	if err != nil {
		switch {
		case errors.Is(err, records.ErrTagNotFound):
			respondNotFound(w, r, fmt.Sprintf("Tag %d not found.", tagID), err)
		case errors.Is(err, records.ErrTagNameTaken):
			http.Error(w, "Another tag already uses that name.", http.StatusConflict)
		default:
			respondInternal(w, r, fmt.Sprintf("Could not rename tag %d.", tagID), err)
		}
		return
	}
	writeExportRedirect(w, "/tags")
	_ = renamed
}

// handleMergeTag merges sourceID into survivorID. POST /tags/{id}/merge
// where {id} is the source id. Form carries `survivor_id`.
func (a *App) handleMergeTag(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sourceID, err := parseTagIDFromTagPath(r.URL.Path)
	if err != nil {
		http.Error(w, "invalid source tag id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the merge form.", err)
		return
	}
	survivorRaw := strings.TrimSpace(r.FormValue("survivor_id"))
	if survivorRaw == "" {
		respondValidation(w, r, "survivor_id is required.", nil)
		return
	}
	survivorID, err := strconv.ParseInt(survivorRaw, 10, 64)
	if err != nil {
		respondValidation(w, r, "Invalid survivor_id.", err)
		return
	}
	ctx := r.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	if _, err := a.tags.MergeInto(ctx, sourceID, survivorID); err != nil {
		switch {
		case errors.Is(err, records.ErrTagNotFound):
			respondNotFound(w, r, "Source or survivor tag not found.", err)
		case errors.Is(err, records.ErrTagMergeCollision):
			http.Error(w, "Tags would share a name after the merge. Rename one first.", http.StatusConflict)
		default:
			respondInternal(w, r, "Could not merge the tag.", err)
		}
		return
	}
	writeExportRedirect(w, "/tags")
}

// handleDeleteTag removes a tag + cascading memberships. DELETE
// /tags/{id}. Idempotent on missing rows; the handler maps that
// to 404 so the dispatch surface stays consistent with the other
// DELETE handlers.
func (a *App) handleDeleteTag(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tagID, err := parseTagIDFromTagPath(r.URL.Path)
	if err != nil {
		http.Error(w, "invalid tag id", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	if err := a.tags.Delete(ctx, tagID); err != nil {
		if errors.Is(err, records.ErrTagNotFound) {
			respondNotFound(w, r, fmt.Sprintf("Tag %d not found.", tagID), err)
			return
		}
		respondInternal(w, r, fmt.Sprintf("Could not delete tag %d.", tagID), err)
		return
	}
	writeExportRedirect(w, "/tags")
}

// handleTagsManagementPage renders the /tags management surface.
// Stubbed at 501 until commit 5 lands the templ; the route must
// exist from day one so the /share/export-options PATCH can
// link there without 404s in tests.
func (a *App) handleTagsManagementPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	http.Error(w, "tag management page is not yet rendered", http.StatusNotImplemented)
}

// handleTagDetailPage renders /tags/{id} (member list). Stub at
// 501 until commit 5.
func (a *App) handleTagDetailPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	http.Error(w, "tag detail page is not yet rendered", http.StatusNotImplemented)
}

// handleShareExportOptions toggles the archive_meta.include_tags
// row for the shared archive kind. PATCH /share/export-options.
// Form carries `include_tags` (0 or 1). On success, redirect to
// /share so the checkbox state re-renders.
func (a *App) handleShareExportOptions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the share export-options form.", err)
		return
	}
	raw := strings.TrimSpace(r.FormValue("include_tags"))
	if raw == "" {
		respondValidation(w, r, "include_tags is required.", nil)
		return
	}
	include := raw == "1" || strings.EqualFold(raw, "true")
	ctx := r.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	if _, err := a.archiveMeta.SetIncludeTags(ctx, records.ArchiveKindShared, include); err != nil {
		respondInternal(w, r, "Could not update share export options.", err)
		return
	}
	writeExportRedirect(w, "/share")
}

// emptyStateText returns the placeholder text for an empty
// autocomplete result list.
func emptyStateText(q string) string {
	if strings.TrimSpace(q) == "" {
		return "Start typing to find a tag."
	}
	return "No matching tags."
}

// escapeTagOption HTML-escapes a tag name for safe interpolation
// inside an <option> element's text content.
func escapeTagOption(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&#39;",
	)
	return r.Replace(s)
}
