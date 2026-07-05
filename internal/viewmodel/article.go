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
	"github.com/valueforvalue/DixieData/internal/models"
)

// Article is the UI-shaped projection of models.Article. It
// carries only the fields the slice-1 /articles list + the
// /articles/{id} detail page need; the markdown source lives
// on Body (verbatim in slice 1; slice 2 swaps to a sanitized
// HTML render). Slice 3 will add fields for the picker modal
// refs panel + the Revisions tab.
type Article struct {
	ID             int64
	DisplayID      string
	Title          string
	Subtitle       string
	Body           string // slice-1: verbatim md; slice 2: sanitized HTML
	CreatedAt      string
	UpdatedAt      string
	BackLinkURL    string
	BackLinkLabel  string
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
		CreatedAt:     input.CreatedAt,
		UpdatedAt:     input.UpdatedAt,
	}
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
