package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

// TestArticlesListShellRendersBodyExcerpt pins issue #532 slice 2:
// each row on /articles carries a <p data-article-preview> element
// with the plain-text body excerpt when the article has a body.
// The excerpt is computed in viewmodel.ArticleFromModel (so the
// templ + the viewmodel stay aligned).
func TestArticlesListShellRendersBodyExcerpt(t *testing.T) {
	articles := []viewmodel.Article{
		{
			ID:        1,
			DisplayID: "ART-0001",
			Title:     "A Sample Article",
			Subtitle:  "Test subtitle",
			// BodyExcerpt is the precomputed plain-text excerpt.
			BodyExcerpt: "This is the body excerpt the user sees on the list page.",
		},
	}
	var buf bytes.Buffer
	if err := ArticlesListShell(articles).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	if !strings.Contains(content, `data-article-preview`) {
		t.Errorf("articles list missing data-article-preview element; got body[0..300]: %s", content[:min(300, len(content))])
	}
	if !strings.Contains(content, "This is the body excerpt the user sees on the list page.") {
		t.Errorf("articles list missing excerpt text; got body[0..300]: %s", content[:min(300, len(content))])
	}
}

// TestArticlesListShellOmitsPreviewWhenBodyExcerptEmpty pins the
// "no excerpt" contract: articles without a body (BodyExcerpt="")
// don't render an empty <p data-article-preview> placeholder. The
// row still shows Title + Subtitle + DisplayID so the list layout
// stays uniform; the preview line just doesn't appear.
func TestArticlesListShellOmitsPreviewWhenBodyExcerptEmpty(t *testing.T) {
	articles := []viewmodel.Article{
		{
			ID:        1,
			DisplayID: "ART-0001",
			Title:     "Bodyless Article",
			// BodyExcerpt intentionally empty.
		},
	}
	var buf bytes.Buffer
	if err := ArticlesListShell(articles).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	if strings.Contains(content, `data-article-preview`) {
		t.Errorf("articles list must NOT render data-article-preview when BodyExcerpt is empty; got body[0..300]: %s", content[:min(300, len(content))])
	}
	// Row layout still intact.
	if !strings.Contains(content, "Bodyless Article") {
		t.Errorf("articles list missing title; got body[0..300]: %s", content[:min(300, len(content))])
	}
}