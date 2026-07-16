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
// TestArticleFromModel_RendersMarkdownWhenBodyHTMLEmpty pins
// the issue #606 contract: when an article was saved before
// the slice-3.6 body_html round-trip (so body_html is empty)
// and the markdown source body_md is populated, the mapper
// must render body_md through the goldmark + bluemonday
// pipeline so the user sees formatted HTML on /articles/{id}
// instead of literal "*" / "#" characters.
//
// Prior to this fix, the mapper fell back to body_md verbatim;
// the templ renders via @templ.Raw, so the user saw "##
// Heading" + "**bold**" instead of the formatted article.
func TestArticleFromModel_RendersMarkdownWhenBodyHTMLEmpty(t *testing.T) {
	in := models.Article{
		DisplayID: "ART-0002",
		Title:     "Legacy article",
		// body_html empty (legacy save before slice 3.6);
		// body_md populated (the source row was always saved).
		BodyHTML: "",
		BodyMD:   "# Legacy heading\n\nThis is **bold** and *italic*.",
	}
	out := ArticleFromModel(in)
	// The mapper should have rendered the markdown source
	// into HTML. The mapper uses the same renderer as the
	// editor preview; goldmark's default heading is h1
	// (no # == h1) and ** == strong, * == em.
	if !strings.Contains(out.Body, "<h1>") {
		t.Errorf("Body should contain rendered <h1> heading for legacy article — got %q", out.Body)
	}
	if !strings.Contains(out.Body, "<strong>bold</strong>") {
		t.Errorf("Body should contain rendered <strong>bold</strong> for legacy article — got %q", out.Body)
	}
	if !strings.Contains(out.Body, "<em>italic</em>") {
		t.Errorf("Body should contain rendered <em>italic</em> for legacy article — got %q", out.Body)
	}
	// Defensive: the literal markdown characters must
	// not return. If the mapper silently reverts to the
	// raw-markdown fallback, the user still sees literal
	// asterisks + hashes.
	if strings.Contains(out.Body, "**bold**") {
		t.Errorf("Body still contains literal **bold** — issue #606 (Markdown render on read) regressed")
	}
	if strings.Contains(out.Body, "# Legacy heading") {
		t.Errorf("Body still contains literal '# Legacy heading' — issue #606 (Markdown render on read) regressed")
	}
}

// TestArticleFromModel_RawMarkdownUnavailableWhenBothEmpty is
// the empty-input degenerate case: when both body_html and
// body_md are empty, the mapper returns an empty Body rather
// than crashing or returning "<p></p>" — the templ's
// empty-state branch renders the guidance copy.
func TestArticleFromModel_RawMarkdownUnavailableWhenBothEmpty(t *testing.T) {
	in := models.Article{
		DisplayID: "ART-0003",
		Title:     "Empty article",
		BodyHTML:  "",
		BodyMD:    "",
	}
	out := ArticleFromModel(in)
	if out.Body != "" {
		t.Errorf("Body should be empty when both body_html + body_md are empty; got %q", out.Body)
	}
	// BodyExcerpt also empty.
	if out.BodyExcerpt != "" {
		t.Errorf("BodyExcerpt should be empty when Body is empty; got %q", out.BodyExcerpt)
	}
}
