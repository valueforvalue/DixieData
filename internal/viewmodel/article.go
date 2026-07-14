// Package viewmodel continues here -- article.go carries the
// slice-1 Article UI projection + mapper (issue #321).
//
// Article is its own viewmodel type (parallel to PersonRecord),
// NOT a field on PersonRecord; this matches locked decision #2
// (parallel primary entity) and aligns with #343 candidate #1's
// recommendation (don't pile Events into PersonRecord; Articles
// get their own type from day 1).
package viewmodel

import (
	"html"
	"regexp"
	"strings"

	"github.com/valueforvalue/DixieData/internal/models"
)

// bodyExcerptCap is the issue #532 slice-2 contract: the
// list-page preview shows the first ~280 characters of the
// sanitized body so the user gets a meaningful excerpt of
// the article's content without the full body overwhelming
// the row. Tune via this constant; the test pins the intent
// (a meaningful excerpt + ellipsis suffix) without a brittle
// exact-length match.
const bodyExcerptCap = 280

// tagStripper matches HTML tags for the body excerpt
// computation. Cheap regex is fine -- the input is
// already-sanitized BodyHTML (post-bluemonday) so there's
// no script / event-handler risk to worry about; we just
// want a clean plain-text excerpt.
var tagStripper = regexp.MustCompile(`<[^>]*>`)

// whitespaceCollider collapses runs of whitespace (including
// newlines from the markdown source) into a single space so
// the excerpt reads as prose on the list page.
var whitespaceCollider = regexp.MustCompile(`\s+`)

// Article is the UI-shaped projection of models.Article. It
// carries only the fields the slice-1 /articles list + the
// /articles/{id} detail page need; the markdown source lives
// on Body (verbatim in slice 1; slice 2 swaps to a sanitized
// HTML render). Slice 3 adds fields for the picker modal refs
// panel + the Revisions tab (slices 3.2 / 3.4); slice 3.8
// adds the CitedIn inverse projection on the Person Record
// detail page (see cited_in_articles.templ).
type Article struct {
	ID             int64
	DisplayID      string
	Title          string
	Subtitle       string
	Body           string // slice-1: verbatim md; slice 2: sanitized HTML
	BodyMD         string // slice-3.6: the raw markdown source (for the editor's source panel)
	// Issue #532 slice 2: plain-text excerpt of the body for
	// the /articles list page row. Computed in ArticleFromModel
	// by stripping HTML tags + collapsing whitespace +
	// truncating to bodyExcerptCap runes with a trailing
	// ellipsis when the body exceeds the cap. Empty when the
	// article has no body to excerpt.
	BodyExcerpt    string
	CreatedAt      string
	UpdatedAt      string
	BackLinkURL    string
	BackLinkLabel  string

	// Refs lists the Person Records attached to this article
	// via the article_refs junction (slice 2). The detail-page
	// Refs panel renders one row per ref with an Unlink button
	// (slice 3.2). Token / Position live on ArticleRef if the
	// slice-3.x reorder surface ever lands (per the Position
	// comment in records.ArticleRef).
	Refs []ArticleRef

	// ResolvedRefs lists the in-body markdown tokens parsed
	// out of Body via ArticleService.ResolveRefs. The detail
	// page renders each token as either a link to the
	// resolved Person Record or a fail-loud "⚠ Unknown: <id>"
	// marker per locked decision #6.
	ResolvedRefs []ArticleRef

	// Snapshots lists the snapshot rows for this article's
	// Revisions tab (slice 3.4). Empty for live branches;
	// service returns empty slice (not nil) so the templ
	// loop renders cleanly.
	Snapshots []Article
}

// ArticleRef is the UI-shaped projection of records.ArticleRef
// (the article_refs junction row). The viewmodel decouples the
// template layer from the service row shape so a future service
// field rename doesn't break the templ.
type ArticleRef struct {
	ID                int64
	ArticleID         int64
	PersonRecordID    int64
	PersonDisplayID   string
	PersonSyncID      string
	Position          int
	Resolved          bool
	Token             string
}

// ArticleFromModel maps a models.Article row to the UI-shaped
// viewmodel.Article projection. Body carries the source verbatim
// in slice 1 (body_html column is set to body_md in Create so the
// first read can render without a markdown library); slice 2's
// mapper swap reads BodyHTML instead.
func ArticleFromModel(input models.Article) Article {
	body := input.BodyHTML
	if body == "" {
		body = input.BodyMD
	}
	return Article{
		ID:            input.ID,
		DisplayID:     input.DisplayID,
		Title:         input.Title,
		Subtitle:      input.Subtitle,
		Body:          body,
		BodyMD:        input.BodyMD,
		BodyExcerpt:   buildBodyExcerpt(body),
		CreatedAt:     input.CreatedAt,
		UpdatedAt:     input.UpdatedAt,
	}
}

// buildBodyExcerpt strips HTML tags + collapses whitespace +
// truncates to bodyExcerptCap runes with a trailing ellipsis
// when the body exceeds the cap. Empty input returns "".
// Operates rune-aware so a multi-byte unicode char at the cap
// boundary isn't split mid-codepoint.
func buildBodyExcerpt(body string) string {
	plain := tagStripper.ReplaceAllString(body, "")
	plain = html.UnescapeString(plain)
	plain = whitespaceCollider.ReplaceAllString(strings.TrimSpace(plain), " ")
	if plain == "" {
		return ""
	}
	runes := []rune(plain)
	if len(runes) <= bodyExcerptCap {
		return plain
	}
	return string(runes[:bodyExcerptCap]) + "\u2026"
}

// ArticlePtrFromModel maps a *models.Article to a *viewmodel.Article,
// returning nil when the input is nil. Used by the /articles/{id}
// detail-page path where GetByID returns *models.Article.
func ArticlePtrFromModel(input *models.Article) *Article {
	if input == nil {
		return nil
	}
	out := ArticleFromModel(*input)
	return &out
}

// ArticlesFromModels maps a []models.Article to []viewmodel.Article
// for the /articles list page. Empty input returns an empty slice
// (not nil) so the .templ loop renders cleanly without a nil guard.
func ArticlesFromModels(inputs []models.Article) []Article {
	out := make([]Article, 0, len(inputs))
	for _, in := range inputs {
		out = append(out, ArticleFromModel(in))
	}
	return out
}
