package viewmodel

import (
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

// TestArticleFromModel_BodyExcerptStripsHTML pins issue #532
// slice 2: the list-page body excerpt must strip HTML tags
// before truncation so the user sees plain prose, not
// "<p>Hello <strong>world</strong></p>" truncated mid-tag.
// Models.Article stores BodyHTML (post-bluemonday) so the
// excerpt logic operates on already-sanitized HTML.
func TestArticleFromModel_BodyExcerptStripsHTML(t *testing.T) {
	in := models.Article{
		DisplayID: "ART-0001",
		Title:     "Sample Article",
		BodyHTML:  "<p>Hello <strong>world</strong> from the <em>archive</em>.</p>",
		BodyMD:    "Hello **world** from the *archive*.",
	}
	out := ArticleFromModel(in)
	if strings.Contains(out.BodyExcerpt, "<") {
		t.Errorf("BodyExcerpt still contains HTML tags: %q", out.BodyExcerpt)
	}
	if !strings.Contains(out.BodyExcerpt, "Hello") || !strings.Contains(out.BodyExcerpt, "world") {
		t.Errorf("BodyExcerpt missing core text: %q", out.BodyExcerpt)
	}
}

// TestArticleFromModel_BodyExcerptTruncatesWithEllipsis pins
// the truncation contract: bodies longer than the per-page
// cap get an ellipsis suffix so the user knows there's more
// content on the detail page. The cap is the issue's "first
// ~280 chars" guidance; if a future change tunes it, this
// test pins the intent (a meaningful excerpt + ellipsis).
func TestArticleFromModel_BodyExcerptTruncatesWithEllipsis(t *testing.T) {
	long := strings.Repeat("a", 600)
	in := models.Article{
		DisplayID: "ART-0001",
		Title:     "Sample Article",
		BodyHTML:  "<p>" + long + "</p>",
	}
	out := ArticleFromModel(in)
	if !strings.HasSuffix(out.BodyExcerpt, "\u2026") {
		t.Errorf("BodyExcerpt truncation missing ellipsis suffix; got %q (length %d)", out.BodyExcerpt, len(out.BodyExcerpt))
	}
	if len(out.BodyExcerpt) > 320 {
		t.Errorf("BodyExcerpt length = %d; want <= 320 (cap + ellipsis + slack)", len(out.BodyExcerpt))
	}
}

// TestArticleFromModel_BodyExcerptNoEllipsisForShortBody pins
// the no-ellipsis contract: bodies that fit in the cap don't
// get a trailing ellipsis (the reader sees the whole excerpt
// and assumes nothing was cut).
func TestArticleFromModel_BodyExcerptNoEllipsisForShortBody(t *testing.T) {
	in := models.Article{
		DisplayID: "ART-0001",
		Title:     "Sample Article",
		BodyHTML:  "<p>Short body.</p>",
	}
	out := ArticleFromModel(in)
	if strings.HasSuffix(out.BodyExcerpt, "\u2026") {
		t.Errorf("BodyExcerpt has ellipsis for short body: %q", out.BodyExcerpt)
	}
}

// TestArticleFromModel_BodyExcerptCollapsesWhitespace pins the
// whitespace-collapsing contract: HTML entities like &nbsp; or
// consecutive newlines / tabs / multiple spaces from the
// markdown source don't render as visual whitespace runs in
// the excerpt. The CSS line-clamp on the list page masks
// extra whitespace visually but the raw string stays clean
// for screen readers + copy/paste.
func TestArticleFromModel_BodyExcerptCollapsesWhitespace(t *testing.T) {
	in := models.Article{
		DisplayID: "ART-0001",
		Title:     "Sample Article",
		BodyHTML:  "<p>Hello\n\n\n   world</p>",
	}
	out := ArticleFromModel(in)
	if strings.Contains(out.BodyExcerpt, "  ") {
		t.Errorf("BodyExcerpt has consecutive spaces: %q", out.BodyExcerpt)
	}
	if strings.Contains(out.BodyExcerpt, "\n") {
		t.Errorf("BodyExcerpt has newlines: %q", out.BodyExcerpt)
	}
}

// TestArticleFromModel_BodyExcerptFallsBackToMarkdown pins the
// pre-render fallback: when BodyHTML is empty (the article
// was created but never re-rendered, or the renderer was
// off when the row was written), the excerpt must compute
// from BodyMD so the list page still shows something useful
// instead of an empty excerpt cell.
func TestArticleFromModel_BodyExcerptFallsBackToMarkdown(t *testing.T) {
	in := models.Article{
		DisplayID: "ART-0001",
		Title:     "Sample Article",
		BodyMD:    "Body content from markdown source.",
		// BodyHTML intentionally empty.
	}
	out := ArticleFromModel(in)
	if !strings.Contains(out.BodyExcerpt, "Body content from markdown source.") {
		t.Errorf("BodyExcerpt missing fallback markdown text: %q", out.BodyExcerpt)
	}
}