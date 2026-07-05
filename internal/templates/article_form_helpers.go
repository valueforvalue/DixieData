// article_form_helpers.go — Helper functions for
// article_form.templ + article_new.templ + article_edit.templ
// (issue #321 slice 3.6 + 3.7). Mirrors the
// entry_form_helpers.go (Soldier-typed) pattern but is
// Article-typed so the local-draft-persistence block +
// markdown editor surface can reference Article-shaped
// helpers without coupling to the Soldier viewmodel.
//
// Per the slice-3.6 user-approved decision: replicate the
// per-entity pattern (decision (a) from the handoff) rather
// than refactor entry_form_helpers.go to a generic shape
// (decision (b)). Slice 3 is ship-the-feature-at-all-costs
// scope; the generic refactor is a follow-up.
package templates

import (
	"fmt"
	"strings"

	"github.com/valueforvalue/DixieData/internal/routebuilder"
	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

// articleDraftKey returns the localStorage key for the
// article form's draft persistence. New articles use a
// stable "new-article" key; edits use "edit-article-{id}".
// Mirrors entry_form_helpers.go:40-44 (draftKey).
func articleDraftKey(article viewmodel.Article, isEdit bool) string {
	if isEdit {
		return fmt.Sprintf("edit-article-%d", article.ID)
	}
	return "new-article"
}

// articleDraftMode returns the kind label that the JS
// auto-wire uses to switch between "Local draft only"
// (new) and "Committed to database" (edit) copy. Mirrors
// entry_form_helpers.go:46-51.
func articleDraftMode(isEdit bool) string {
	if isEdit {
		return "edit"
	}
	return "new"
}

// articleDraftRecordVersion returns the per-edit record
// version sentinel used to invalidate stale drafts. Joins
// UpdatedAt | ID so an Update that bumps updated_at will
// discard any older draft. The new-article path returns
// "" (no version to track pre-Create).
func articleDraftRecordVersion(article viewmodel.Article, isEdit bool) string {
	if !isEdit {
		return ""
	}
	parts := []string{}
	if ts := strings.TrimSpace(article.UpdatedAt); ts != "" {
		parts = append(parts, ts)
	}
	parts = append(parts, fmt.Sprintf("%d", article.ID))
	return strings.Join(parts, "|")
}

// articleDraftResetPath returns the URL the JS dispatcher
// uses to clear a stale draft. New articles reset to
// /articles/new; edits reset to /articles/{id}/edit.
func articleDraftResetPath(article viewmodel.Article, isEdit bool) string {
	if isEdit {
		return routebuilder.ArticleEdit(article.ID)
	}
	return routebuilder.ArticleNew()
}

// articleRecordPersistenceClass returns the CSS class for
// the persistence-status pill: amber for new (local draft
// only), emerald for edit (committed to database).
func articleRecordPersistenceClass(isEdit bool) string {
	if isEdit {
		return "rounded-2xl border border-emerald-700/40 bg-emerald-50/80 px-4 py-3 text-sm text-emerald-900"
	}
	return "rounded-2xl border border-amber-700/40 bg-amber-50/80 px-4 py-3 text-sm text-amber-900"
}

// articleRecordPersistenceHeading returns the bold heading
// text for the persistence-status pill. Mirrors
// entry_form_helpers.go:73-78.
func articleRecordPersistenceHeading(isEdit bool) string {
	if isEdit {
		return "Committed to database."
	}
	return "Local draft only."
}

// articleRecordPersistenceMessage returns the small body
// text under the persistence-status heading.
func articleRecordPersistenceMessage(isEdit bool) string {
	if isEdit {
		return "This article currently matches the primary database until you make new local edits."
	}
	return "This new article is cached in localStorage until you create it in the database."
}