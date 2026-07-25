// markdown_typst_test.go -- pins the markdown -> typst conversion
// contract introduced for the Article Record PDF export (see
// markdown_typst.go for the implementation). The conversion
// walks the goldmark AST directly so the PDF path stays
// aligned with the web render (both consume the same Markdown
// source via goldmark).
//
// Each test case pins one element so a future refactor of the
// walker doesn't silently drop or duplicate an element. The
// full test list mirrors the coverage comment in
// markdown_typst.go.

package records

import (
	"strings"
	"testing"
)

func TestMarkdownRenderer_RenderTypst(t *testing.T) {
	r := NewMarkdownRenderer()

	cases := []struct {
		name     string
		source   string
		mustHave []string
		mustNot  []string
	}{
		{
			name:     "empty source returns empty string",
			source:   "",
			mustHave: []string{},
			mustNot:  []string{"#par", "#list", "#quote"},
		},
		{
			name:     "h1 renders as typst heading level 1",
			source:   "# Hello",
			mustHave: []string{"= Hello"},
			mustNot:  []string{"<h1>"},
		},
		{
			name:     "h2 renders as typst heading level 2",
			source:   "## Subhead",
			mustHave: []string{"== Subhead"},
		},
		{
			name:     "h3 renders as typst heading level 3",
			source:   "### Tertiary",
			mustHave: []string{"=== Tertiary"},
		},
		{
			name:     "h6 renders as typst heading level 6",
			source:   "###### Bottom",
			mustHave: []string{"====== Bottom"},
		},
		{
			name:     "paragraph wraps in #par[]",
			source:   "Just a paragraph.",
			mustHave: []string{"#par[", "Just a paragraph.", "]"},
		},
		{
			name:     "emphasis level 1 renders as italic underscore",
			source:   "*italic text*",
			mustHave: []string{"_italic text_"},
		},
		{
			name:     "emphasis level 2 renders as bold asterisk",
			source:   "**bold text**",
			mustHave: []string{"*bold text*"},
			// The single '*' pairs (italic) must not appear
			// around the bold span.
			mustNot: []string{"*bold*"},
		},
		{
			name:     "code span renders in backticks",
			source:   "Inline `code` here.",
			mustHave: []string{"`code`"},
		},
		{
			name:     "code span is emitted ONCE (no double emit)",
			source:   "`code`",
			mustHave: []string{"`code`"},
			mustNot:  []string{"`code`code`"},
		},
		{
			name:     "link renders as typst #link()",
			source:   "[label](https://example.com)",
			mustHave: []string{`#link("https://example.com")[label]`},
		},
		{
			name:     "image renders as typst #image() with alt text",
			source:   "![alt text](pic.png)",
			mustHave: []string{`#image("pic.png", alt: "alt text")`},
		},
		{
			name:     "image is emitted ONCE (no double emit)",
			source:   "![alt text](pic.png)",
			mustHave: []string{`#image("pic.png", alt: "alt text")`},
			// The walker must skip image children so the
			// alt text isn't re-emitted as plain text after
			// the #image() call.
			mustNot: []string{"alt text]alt text"},
		},
		{
			name:     "unordered list renders as typst #list[]",
			source:   "- a\n- b\n- c",
			mustHave: []string{"#list[", "- a", "- b", "- c"},
		},
		{
			name:     "ordered list renders as typst #enum(numbering: '1.')",
			source:   "1. a\n2. b",
			mustHave: []string{"#enum(", `numbering: "1."`, "- a", "- b"},
		},
		{
			// Issue #433: PDF body list markers rendered as
			// U+FFFD because the default typst #enum marker uses
			// a private-use glyph the bundled font lacks. The
			// converter now passes `numbering: "1."` as a
			// named arg so every font renders plain ASCII
			// digit-period instead of the typst-private glyph.
			name:     "ordered list pins ASCII numbering: '1.' for font-independence (#433)",
			source:   "1. a\n2. b",
			mustHave: []string{`numbering: "1."`},
		},
		{
			name:     "blockquote renders with #quote()",
			source:   "> a quoted line",
			mustHave: []string{"#quote(block: true)["},
		},
		{
			name:     "fenced code block renders as raw block",
			source:   "```\ncode here\nmore code\n```",
			mustHave: []string{"#raw("},
			mustNot:  []string{"#list[", "#quote", "#par"},
		},
		{
			name:     "thematic break renders as line",
			source:   "before\n\n---\n\nafter",
			mustHave: []string{"#line(length: 100%"},
		},
		{
			name:     "soft line break in paragraph renders as space",
			source:   "line one\nline two",
			mustHave: []string{"line one line two"},
		},
		{
			name:     "typst reserved characters are escaped",
			source:   "has # hash and * star and _ underscore",
			mustHave: []string{`\#`, `\*`, `\_`},
		},
		{
			name:     "mixed paragraph with multiple inlines",
			source:   "A *em* then **strong** then `code` end.",
			mustHave: []string{"A _em_ then *strong* then `code` end."},
		},
		{
			name:     "links inside paragraph",
			source:   "Visit [our site](https://example.com) for more.",
			mustHave: []string{"#par[", `#link("https://example.com")[our site]`, "]"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := r.RenderTypst(tc.source)
			if err != nil {
				t.Fatalf("RenderTypst: %v", err)
			}
			for _, s := range tc.mustHave {
				if !strings.Contains(got, s) {
					t.Errorf("missing %q in output:\n%s", s, got)
				}
			}
			for _, s := range tc.mustNot {
				if strings.Contains(got, s) {
					t.Errorf("unexpected %q in output:\n%s", s, got)
				}
			}
		})
	}
}
