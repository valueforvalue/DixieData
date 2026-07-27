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

	"github.com/valueforvalue/DixieData/internal/config"
)

func TestMarkdownRenderer_RenderTypst(t *testing.T) {
	cases := []struct {
		name     string
		source   string
		setup    func(r *MarkdownRenderer)
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
			mustHave: []string{"#enum(", `numbering: "1."`, "a", "b"},
			mustNot:  []string{"- a", "- b"},
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
		// Issue #669 + #670 amendment 2: typst 0.15 removed
		// the `stroke:` arg from `#quote(...)` AND rejects
		// nested `rgb("rgb(...))` forms. The fix emits
		// `#block(inset: (left: 1em), stroke: (left: 2pt + #hex))`
		// + `#set par(first-line-indent: 0pt)`. typstColorExpr
		// returns the bare color literal (no `rgb("...")` wrapper)
		// so the stroke arg is a clean typst expression. Pin
		// the new shape so a future refactor doesn't reintroduce
		// either regression. This subtest wires a theme so the
		// themed code path (the one real PDF exports exercise)
		// is exercised; the fall-through path is covered by the
		// next subtest.
		{
			name:   "blockquote uses #block with left stroke when theme is wired (typst 0.15+, issue #669 + #670)",
			source: "> a quote\n> line two",
			setup: func(r *MarkdownRenderer) {
				r.SetTheme(&config.ThemeConfig{BlockquoteBorder: "#8d7440"})
			},
			mustHave: []string{`#block(inset: (left: 1em), stroke: (left: 2pt + rgb("#8d7440"))`, "#set par(first-line-indent: 0pt)"},
			mustNot:  []string{"#quote(block: true, stroke:"},
		},
		{
			name:     "blockquote without theme falls back to plain #quote (no stroke arg)",
			source:   "> a quote without theme",
			mustHave: []string{"#quote(block: true)["},
			mustNot:  []string{"stroke:"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Each subtest starts from a fresh renderer so
			// the theme is deterministic per case. Without
			// the reset, a prior case that called SetTheme
			// would leak into the next case (the fallback
			// "no theme" subtest would see a theme and take
			// the themed branch).
			r := NewMarkdownRenderer()
			if tc.setup != nil {
				tc.setup(r)
			}
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

// TestTypstColorExpr_NormalizesForTypst015 (issue #670 amendment 2) pins
// the typstColorExpr normalizer. The pre-typst-0.15 code passed the
// `rgb(36 48 61 / 0.06)` form through verbatim, which the markdown
// converter then wrapped in `rgb("rgb(36 48 61 / 0.06)")` — the
// nested rgb() form that typst 0.15 rejects with
// "color string contains non-hexadecimal letters". The fix converts
// the input to a typst-0.15-compatible literal:
//   - "rgb(R G B / A)"   → "color.rgb(R, G, B, A*255)"
//   - "rgb(R, G, B, A)"  → "color.rgb(R, G, B, A*255)"
//   - "rgb(R, G, B)"     → "#rrggbb"   (6-char hex, alpha omitted)
//   - "#hex"             → "#hex"      (pass through)
func TestTypstColorExpr_NormalizesForTypst015(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		// Hex passthrough.
		{"hex 6-char wraps in rgb()", "#8d7440", `rgb("#8d7440")`},
		{"hex 3-char wraps in rgb()", "#abc", `rgb("#abc")`},
		// CSS rgb() with space separator + slash alpha.
		// The default CodeBackground is this form:
		// "rgb(36 48 61 / 0.06)". Convert to typst's own
		// color.rgb() literal so the outer rgb() wrapper
		// (added by the caller) becomes color.rgb(36, 48, 61, 15)
		// which typst 0.15 accepts.
		{
			"css rgb() with space + slash alpha",
			"rgb(36 48 61 / 0.06)",
			"color.rgb(36, 48, 61, 15)",
		},
		// Comma-separated with alpha.
		{
			"css rgb() comma alpha",
			"rgb(36, 48, 61, 0.06)",
			"color.rgb(36, 48, 61, 15)",
		},
		// No alpha — fall back to 6-char hex.
		{
			"css rgb() no alpha wraps in rgb()",
			"rgb(36, 48, 61)",
			`rgb("#24303d")`,
		},
		{
			"css rgb() space no alpha wraps in rgb()",
			"rgb(36 48 61)",
			`rgb("#24303d")`,
		},
		// Named color passthrough.
		{"named color passes through", "red", "red"},
		// Empty falls back to black.
		{"empty falls back to black (wrapped)", "", `rgb("#000000")`},
		{"whitespace falls back to black (wrapped)", "   ", `rgb("#000000")`},
		// Edge: alpha 1.0 = fully opaque. Should still emit
		// color.rgb() with alpha=255, not a hex. (The form
		// is lossy on conversion but the alpha is preserved.)
		{
			"alpha 1.0 emits color.rgb with alpha 255",
			"rgb(255, 0, 0, 1.0)",
			"color.rgb(255, 0, 0, 255)",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := typstColorExpr(c.in)
			if got != c.want {
				t.Errorf("typstColorExpr(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestTypstEscape_EscapesDollarInMarkup (issue #670 amendment 3)
// pins the dollar-sign escape. typst 0.15 rejected a raw `$` in
// markup mode (it starts math mode + requires a matching `]` or
// `)` to close), so the pre-0.15 code that passed `~$5` through
// unchanged failed at compile time. The fix adds `$` to the
// typstEscape switch so the body becomes `\~$5` (both
// characters escaped, both legal in markup mode).
func TestTypstEscape_EscapesDollarInMarkup(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"dollar alone", "$", `\$`},
		{"dollar in text", "~$5 million", `\~\$5 million`},
		{"dollar in list item", "Property damage: ~$5 million", `Property damage\: \~\$5 million`},
		// Already-escaped source: goldmark strips the backslash,
		// so the walker sees raw text. The escape is applied
		// once.
		{"dollar at end of word", "end$", `end\$`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := typstEscape(c.in)
			if got != c.want {
				t.Errorf("typstEscape(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
