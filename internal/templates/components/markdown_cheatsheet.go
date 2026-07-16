package components

import (
	"github.com/valueforvalue/DixieData/internal/records"
)

// markdownPreviewRenderer is the shared goldmark renderer
// the cheatsheet uses to produce the live preview cells.
// It is package-scoped (not exported) because every call
// site is the cheatsheet; the renderer itself is stateless
// after construction so reusing a single instance is safe
// and cheaper than re-allocating per row.
var markdownPreviewRenderer = records.NewMarkdownRenderer()

// RenderPreview returns the goldmark-rendered HTML for a
// Markdown source string. Used by the MarkdownCheatsheet
// component (issue #610) to ship a live, render-accurate
// preview of each syntax row alongside the raw Syntax +
// Effect + Example text fields.
//
// Input contract: any non-empty Markdown source. Empty
// input returns empty string (matches the
// records.MarkdownRenderer.Render contract — the slice-1
// verbatim path can store "" for an empty body without
// forcing a markdown render).
//
// Output contract: goldmark output sanitized through the
// custom bluemonday policy in records.NewMarkdownRenderer.
// The cheatsheet renders the result via templ.Raw; the
// bluemonday pass guarantees no raw <script>/<iframe>/etc.
// survives, so the per-row preview is XSS-safe by
// construction.
func RenderPreview(source string) string {
	if source == "" {
		return ""
	}
	rendered, err := markdownPreviewRenderer.Render(source)
	if err != nil {
		return ""
	}
	return rendered
}