// markdown.go — Markdown -> sanitized HTML renderer for
// Article Records (issue #321 slice 3.6 + slice 4). Wraps
// yuin/goldmark (CommonMark parser) + microcosm-cc/bluemonday
// (Strict policy sanitizer) so the slice-1 verbatim md
// storage can be rendered safely on the /articles/{id}
// detail page + the slice-3.6 editor's live preview.
//
// License acknowledgements (carried in the slice-3.6 commit
// message):
//   - github.com/yuin/goldmark v1.8.2 is MIT licensed
//     (https://github.com/yuin/goldmark/blob/master/LICENSE).
//   - github.com/microcosm-cc/bluemonday v1.0.27 is BSD-3-Clause
//     licensed
//     (https://github.com/microcosm-cc/bluemonday/blob/master/LICENSE.md).
//
// Both libraries are pinned in go.mod and live under their
// own indirect chain; the MarkdownRenderer type wraps them
// so the slice-4 export paths can reuse it without coupling
// to goldmark or bluemonday directly.
package records

import (
	"bytes"
	"fmt"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

// MarkdownRenderer renders markdown source to a sanitized
// HTML string using goldmark + a custom bluemonday policy.
// The renderer is stateless after construction; a single
// instance is safe for concurrent use across goroutines.
//
// Policy choice (locked decision in slice-3.6 user approval):
// goldmark renders the markdown to HTML; the custom policy
// allows goldmark's safe output tags (h1-h6, p, ul, ol, li,
// a, blockquote, code, pre, em, strong, hr, img) and
// strips all raw HTML the author slipped in
// (script, iframe, style, object, embed, form, input, etc.).
// This matches the user-approved intent of "no HTML allowed"
// in the slice-3.6 surface: authors cannot inject raw HTML,
// but markdown syntax does render to HTML output.
//
// bluemonday.StrictPolicy() is too aggressive for this use
// case -- it strips ALL HTML, including the markdown render.
// The custom policy below is the right balance for the
// Article Records use case.
//
// Exported as part of issue #523 so the seed-data CLI can
// reuse the same goldmark pipeline the Wails app uses on
// save. Callers that need a wrapper around markdown render
// (e.g. the static archive exporter, future email templates)
// can import records.NewMarkdownRenderer directly.
type MarkdownRenderer struct {
	md       goldmark.Markdown
	sanitizer *bluemonday.Policy
}

// NewMarkdownRenderer constructs a renderer with goldmark's
// defaults + the custom bluemonday policy. The goldmark
// instance is reusable across calls; the bluemonday Policy
// is a pointer-type wrapper that maintains internal state,
// so reusing it is correct (and faster than re-allocating
// per call).
//
// Issue #525: enable the GFM extension so tables, autolinks,
// strikethrough, and task lists render in the article preview /
// detail surface (the slice-3.6 editor + /articles/{id} detail
// + the static archive article page). The custom bluemonday
// policy below already allow-lists every tag GFM emits
// (table/thead/tbody/tr/th/td/a/del/hr) plus the attribute
// rules GFM uses (href on <a>), so no sanitizer changes
// are needed alongside the extension toggle.
func NewMarkdownRenderer() *MarkdownRenderer {
	p := bluemonday.NewPolicy()
	// Allow goldmark's safe output tags.
	p.AllowElements("h1", "h2", "h3", "h4", "h5", "h6",
		"p", "ul", "ol", "li", "a", "blockquote",
		"code", "pre", "em", "strong", "hr", "img",
		"br", "del", "table", "thead", "tbody", "tr", "th", "td")
	// Allow href on anchors (Person Record tokens rely on
	// markdown link syntax producing an <a href="...">).
	p.AllowAttrs("href").OnElements("a")
	// Allow src + alt on images (markdown ![alt](src) syntax).
	p.AllowAttrs("src", "alt").OnElements("img")
	// Allow task list checkbox rendering. GFM task list
	// output uses <input type="checkbox" disabled> with no
	// other attributes; allow type + disabled + checked so
	// the checkbox state survives sanitization.
	p.AllowAttrs("type", "disabled", "checked").OnElements("input")
	return &MarkdownRenderer{
		md: goldmark.New(
			goldmark.WithExtensions(extension.GFM),
		),
		sanitizer: p,
	}
}

// Render converts markdown source to a sanitized HTML
// string. Returns the empty string when source is blank
// (the slice-1 verbatim path can store "" for an empty
// body without forcing a markdown render).
//
// Implementation: goldmark renders to a bytes.Buffer; the
// buffer is then passed through bluemonday's StrictPolicy
// sanitizer so any raw HTML the author slipped in is
// stripped. The output is safe to render via templ.Raw.
//
// Note: the caller is responsible for the surrounding
// container markup (e.g. <div class="prose">); this method
// returns only the rendered HTML fragment.
func (r *MarkdownRenderer) Render(source string) (string, error) {
	if source == "" {
		return "", nil
	}
	var buf bytes.Buffer
	if err := r.md.Convert([]byte(source), &buf); err != nil {
		return "", fmt.Errorf("markdown render: %w", err)
	}
	return r.sanitizer.Sanitize(buf.String()), nil
}