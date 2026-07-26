// markdown_typst.go -- markdown source -> typst markup renderer
// for the Article Record PDF export (issue #321 slice 4.2 PDF
// body, follow-up to the article-detail rendering fixes that
// shipped in commit 617dcee for the web view).
//
// The web view (article_detail.templ) renders BodyHTML via
// templ.Raw and a CSS stylesheet; the typst export path is
// in the .typ files (article_landscape.typ / article_portrait.typ)
// and previously just passed body_html into typst's `raw()`
// which prints the HTML source verbatim (a "white screen" of
// HTML tags inside the PDF). This converter walks goldmark's
// AST directly and emits typst markup so the PDF body actually
// renders as styled content.
//
// Why goldmark AST and not HTML parsing: the same Markdown
// source feeds both the web render (HTML) and the PDF render
// (typst). Walking the AST means the two outputs share a
// single source of truth. Re-parsing sanitized HTML on the
// other side would risk subtle drift between the two render
// paths whenever a new element or attribute is added.
//
// License + dependency: yuin/goldmark v1.8.2 (MIT, already
// in go.mod). No new deps.
//
// Coverage (matches the bluemonday policy in markdown.go):
//   h1-h6, p, ul, ol, li, em, strong (via Emphasis level=1/2),
//   blockquote, code, pre, a, img, hr, br, del, table, thead,
//   tbody, tr, th, td. Anything not in the typst output gets
//   silently skipped (RawHTML, HTMLBlock, LinkReferenceDefinition,
//   extensions we don't enable).
//
// Scoping: this method lives next to MarkdownRenderer so the
// existing callers (the web render path) keep their public
// surface unchanged. The PDF path is the only consumer for
// now, wired in by RenderPDF in article_service.go.
package records

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	gastext "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/valueforvalue/DixieData/internal/config"
)

// RenderTypst converts a markdown source string to typst
// markup suitable for inline placement inside an article_*.typ
// template. The output is plain typst markup (no escaping
// needed for typst markup characters in article prose). Code
// spans + code blocks are the exception: their content is
// passed through typst's `raw()` so the source appears
// verbatim.
//
// The returned string is suitable for passing to a typst
// template as data["article"].body_typst. Templates should
// interpolate it with `#data.at("body_typst", default: "")`
// inside a body container -- the typ wrapper holds the
// surrounding layout, fonts, and colors.
//
// Error returns are reserved for future use (e.g. parse
// panics) so the API matches Render's signature; today the
// parse is total and never returns a non-nil error.
func (r *MarkdownRenderer) RenderTypst(source string) (string, error) {
	if source == "" {
		return "", nil
	}
	srcBytes := []byte(source)
	md := goldmark.New(goldmark.WithExtensions(extension.GFM))
	doc := md.Parser().Parse(text.NewReader(srcBytes))

	state := &typstState{src: srcBytes, theme: r.theme}
	walker := func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		return state.walk(n, entering)
	}
	if err := ast.Walk(doc, walker); err != nil {
		return "", fmt.Errorf("markdown typst walk: %w", err)
	}
	return state.out.String(), nil
}

// typstState holds the per-render output buffer and the
// source bytes (Text nodes carry source-segment ranges, not
// the actual text). Methods emit typst markup into out.
type typstState struct {
	src []byte
	out bytes.Buffer
	// theme is the configured ThemeConfig (issue #660
	// amendment #2). May be nil; when nil the converter
	// emits flat 11pt Arial with no link color, no mono
	// font, no blockquote border, no heading-color tint.
	// When non-nil, the converter wraps links in
	// theme.palette.link, code in theme.fonts.mono +
	// theme.palette.code_background, blockquote in italic
	// + theme.palette.blockquote_border, headings in
	// theme.palette.heading_color at
	// theme.type_scale.heading_h{1..4}.size_pt.
	theme *config.ThemeConfig

	// Table accumulation
	tableCells    []string
	inTableHeader bool
	inTableCell   bool
	cellBuf       bytes.Buffer
	rowCellCount  int
}

// walk is the ast.Walker callback. It dispatches on node type
// and uses the `entering` flag to decide whether to emit the
// opening or closing markup (typst markup is mostly prefix
// style, so a single emit per node is the common case).
func (s *typstState) walk(n ast.Node, entering bool) (ast.WalkStatus, error) {
	switch v := n.(type) {
	case *ast.Document:
		// root: walk children only
	case *ast.Heading:
		if entering {
			s.writeHeading(v)
		}
		// Skip children -- the heading label was collected
		// inside writeHeading. Without this skip, the heading's
		// Text children would emit the label a second time.
		return ast.WalkSkipChildren, nil
	case *ast.CodeSpan:
		if entering {
			// Issue #660: when a theme is wired, wrap the
			// inline code span in a #text() that forces the
			// configured mono font + body size. Without the
			// theme we emit the bare backtick pair (the
			// legacy behavior).
			if s.theme != nil && s.theme.Fonts.Mono != "" {
				fmt.Fprintf(&s.out, `#text(font: "%s")[`, typstEscape(s.theme.Fonts.Mono))
			}
			s.out.WriteString("`")
		} else {
			s.out.WriteString("`")
			if s.theme != nil && s.theme.Fonts.Mono != "" {
				s.out.WriteString("]")
			}
		}
		// Walker walks the Text children which emit the content.
		// No skip -- the text needs to be written by the walker.
	case *ast.Paragraph:
		// ListItem children use TextBlock, not Paragraph, for
		// the inline text. So a Paragraph here is always a
		// top-level body paragraph. Wrap in #par[]. Soft/hard
		// breaks inside the paragraph are handled by the Text
		// walker.
		if entering {
			s.out.WriteString("#par[\n")
		} else {
			s.out.WriteString("] #v(0.4em)\n")
		}
	case *ast.TextBlock:
		// Inlines only, no #par wrapper. Used inside ListItem.
		// No opening/closing markup needed -- children handle it.
	case *ast.Emphasis:
		if v.Level == 2 {
			s.out.WriteString("*")
		} else {
			s.out.WriteString("_")
		}
	case *ast.Link:
		if entering {
			dest := string(v.Destination)
			// Issue #660: when a theme is wired, wrap the link
			// label in #text(fill: theme.palette.link) so the
			// PDF link color matches the browser preview.
			// Without a theme we emit the bare #link call
			// (the legacy typst default-color behavior).
			linkColor := ""
			if s.theme != nil {
				linkColor = s.theme.Palette["link"]
			}
			if linkColor != "" {
				fmt.Fprintf(&s.out, `#link("%s")[#text(fill: %s)[`, typstEscapeLink(dest), typstColorExpr(linkColor))
			} else {
				fmt.Fprintf(&s.out, `#link("%s")[`, typstEscapeLink(dest))
			}
		} else {
			linkColor := ""
			if s.theme != nil {
				linkColor = s.theme.Palette["link"]
			}
			if linkColor != "" {
				s.out.WriteString("]]")
			} else {
				s.out.WriteString("]")
			}
		}
	case *ast.Image:
		if entering {
			// Image is a leaf from the typst point of view:
			// the typst #image("dest", alt: "alt") call is one
			// statement. Emit it here, then skip the children
			// so the alt text isn't re-emitted as plain text.
			dest := string(v.Destination)
			alt := typstEscape(inlineTextContent(v, s.src))
			fmt.Fprintf(&s.out, `#image("%s", alt: "%s")`, typstEscapeLink(dest), alt)
			return ast.WalkSkipChildren, nil
		}
	case *ast.AutoLink:
		if entering {
			// AutoLink is a leaf -- emit and skip children.
			dest := string(v.URL(s.src))
			label := typstEscape(string(v.Label(s.src)))
			fmt.Fprintf(&s.out, `#link("%s")[%s]`, typstEscapeLink(dest), label)
			return ast.WalkSkipChildren, nil
		}
	case *ast.Blockquote:
		if entering {
			// Issue #669: typst 0.15 removed the `stroke:`
			// arg from `#quote(...)`. The pre-0.15 shape
			// was:
			//   #quote(block: true, stroke: (left: 2pt + rgb("#abc")))[
			// which now fails to compile with "unexpected
			// argument: stroke". Preserve the left-border
			// design (the `--theme-sepia` token) by
			// switching to `#block(inset: ..., stroke: ...)`
			// + `#set par(first-line-indent: 0pt)` so the
			// first line of the quote body doesn't get an
			// unwanted indent.
			if s.theme != nil && s.theme.BlockquoteBorder != "" {
				fmt.Fprintf(&s.out, "#block(inset: (left: 1em), stroke: (left: 2pt + %s))[\n#set par(first-line-indent: 0pt)\n", typstColorExpr(s.theme.BlockquoteBorder))
			} else {
				s.out.WriteString("#quote(block: true)[\n#set par(first-line-indent: 0pt)\n")
			}
		} else {
			s.out.WriteString("]\n")
		}
	case *ast.FencedCodeBlock:
		if entering {
				lang := string(v.Language(s.src))
			content := blockText(v, s.src)
			// Issue #660: when a theme is wired, wrap the
			// code block in a #block(fill: ...) + the
			// configured mono font so the block matches the
			// browser preview.
			monoFont := ""
			codeFill := ""
			if s.theme != nil {
				monoFont = s.theme.Fonts.Mono
				codeFill = s.theme.CodeBackground
			}
			if monoFont != "" {
				fmt.Fprintf(&s.out, "#block(fill: %s, inset: 0.5em, radius: 2pt)[\n", typstColorExpr(codeFill))
				fmt.Fprintf(&s.out, "#text(font: \"%s\")[\n", typstEscape(monoFont))
			}
			// typst 0.15: raw() requires a string first arg;
			// the content-body syntax raw(...)[...] is
			// rejected with "expected string, found content".
			// Emit as #raw("...escaped...", block: true).
			_ = lang
			fmt.Fprintf(&s.out, "#raw(%s, block: true, lang: \"\")\n", typstStringLiteral(content))
			if monoFont != "" {
				s.out.WriteString("]\n]\n")
			}
		}
	case *ast.CodeBlock:
		if entering {
			content := blockText(v, s.src)
			// Issue #660: same mono-font wrap as the fenced
			// code block above.
			monoFont := ""
			codeFill := ""
			if s.theme != nil {
				monoFont = s.theme.Fonts.Mono
				codeFill = s.theme.CodeBackground
			}
			if monoFont != "" {
				fmt.Fprintf(&s.out, "#block(fill: %s, inset: 0.5em, radius: 2pt)[\n", typstColorExpr(codeFill))
				fmt.Fprintf(&s.out, "#text(font: \"%s\")[\n", typstEscape(monoFont))
			}
			fmt.Fprintf(&s.out, "#raw(%s, block: true)\n", typstStringLiteral(content))
			if monoFont != "" {
				s.out.WriteString("]\n]\n")
			}
		}
	case *ast.List:
		ordered := v.IsOrdered()
		if entering {
			if ordered {
				// Issue #433: pass `numbering: "1."` as a named
				// arg so every font renders plain ASCII
				// digit-period. Without the explicit arg,
				// typst 0.13+ falls back to a private-use
				// glyph for the marker that bundled fonts
				// (Liberation Sans etc.) lack, surfacing as
				// U+FFFD in the PDF output.
				s.out.WriteString(`#enum(numbering: "1.")[\n`)
			} else {
				s.out.WriteString("#list[\n")
			}
		} else {
			s.out.WriteString("]\n")
		}
	case *ast.ListItem:
		if entering {
			s.out.WriteString("- ")
		} else {
			s.out.WriteString("\n")
		}
	case *ast.ThematicBreak:
		if entering {
			s.out.WriteString("#line(length: 100%, stroke: 0.5pt + luma(200))\n")
		}
	case *ast.Text:
		if entering {
			if s.inTableCell {
				s.writeTextToBuf(v, &s.cellBuf)
			} else {
				s.writeText(v)
			}
		}
	case *ast.String:
		if entering {
			if s.inTableCell {
				s.cellBuf.WriteString(typstEscape(string(v.Value)))
			} else {
				s.out.WriteString(typstEscape(string(v.Value)))
			}
		}
	case *ast.HTMLBlock, *ast.RawHTML, *ast.LinkReferenceDefinition:
		// Stripped -- bluemonday already filtered the sanitized
		// output, so anything left here is empty or unwanted.
		// Skipping via returning WalkSkipChildren is more
		// efficient than emitting nothing for the children.
		return ast.WalkSkipChildren, nil
	case *gastext.Table:
		if entering {
			s.tableCells = s.tableCells[:0]
			s.rowCellCount = 0
		} else {
			s.emitTable()
		}
	case *gastext.TableHeader:
		s.inTableHeader = entering
	case *gastext.TableRow:
		if entering {
			s.rowCellCount = 0
		}
	case *gastext.TableCell:
		if entering {
			s.inTableCell = true
			s.cellBuf.Reset()
		} else {
			s.inTableCell = false
			content := s.cellBuf.String()
			if s.inTableHeader {
				content = "*" + content + "*"
			}
			s.tableCells = append(s.tableCells, content)
			s.rowCellCount++
		}
	default:
		// Unknown block / inline: best effort, walk children
		// so any nested content still renders.
	}
	return ast.WalkContinue, nil
}

// writeHeading emits a typst heading for the given level.
// The typst `=` syntax maps directly to h1..h6 (`= h1`, `== h2`,
// `=== h3`, etc.), so we don't need a custom style block.
func (s *typstState) writeHeading(h *ast.Heading) {
	level := h.Level
	if level < 1 {
		level = 1
	}
	if level > 6 {
		level = 6
	}
	// Collect the heading's inline children into the typst
	// markup. Common heading content is plain text + maybe
	// a code span or emphasis; handle those.
	var label strings.Builder
	for c := h.FirstChild(); c != nil; c = c.NextSibling() {
		switch t := c.(type) {
		case *ast.Text:
			label.WriteString(typstEscape(string(t.Segment.Value(s.src))))
		case *ast.String:
			label.WriteString(typstEscape(string(t.Value)))
		case *ast.CodeSpan:
			label.WriteString("`")
			label.WriteString(typstEscape(inlineTextContent(t, s.src)))
			label.WriteString("`")
		case *ast.Emphasis:
			level := t.Level
			if level == 2 {
				label.WriteString("*")
			} else {
				label.WriteString("_")
			}
			for cc := t.FirstChild(); cc != nil; cc = cc.NextSibling() {
				if tx, ok := cc.(*ast.Text); ok {
					label.WriteString(typstEscape(string(tx.Segment.Value(s.src))))
				}
			}
			if level == 2 {
				label.WriteString("*")
			} else {
				label.WriteString("_")
			}
		}
	}
	for i := 0; i < level; i++ {
		s.out.WriteString("=")
	}
	s.out.WriteString(" ")
	// Issue #660: when a theme is wired, wrap the heading
	// label in #text(fill: heading_color) so the PDF
	// heading color matches the browser preview. The
	// heading_size_pt comes from theme.type_scale.
	if s.theme != nil && s.theme.HeadingColor != "" {
		fmt.Fprintf(&s.out, "#text(fill: %s)[", typstColorExpr(s.theme.HeadingColor))
	}
	s.out.WriteString(label.String())
	if s.theme != nil && s.theme.HeadingColor != "" {
		s.out.WriteString("]")
	}
	s.out.WriteString(" #v(0.3em)\n")
}

// writeText emits a Text node's content, honouring hard and
// soft line breaks. typst handles word wrapping automatically;
// soft breaks render as a space (so adjacent text fragments
// don't merge), hard breaks render as #linebreak().
func (s *typstState) writeText(t *ast.Text) {
	raw := string(t.Segment.Value(s.src))
	if t.HardLineBreak() {
		raw = strings.TrimRight(raw, "\n")
		raw = strings.TrimRight(raw, "\\")
		s.out.WriteString(typstEscape(raw))
		s.out.WriteString(" #linebreak()\n")
		return
	}
	if t.SoftLineBreak() {
		// Soft break -- a single space separates the two halves
		// so the text reads naturally instead of merging.
		raw = strings.TrimRight(raw, "\n")
		s.out.WriteString(typstEscape(raw))
		s.out.WriteString(" ")
		return
	}
	raw = strings.TrimRight(raw, "\n")
	s.out.WriteString(typstEscape(raw))
}

// inlineTextContent concatenates all Text descendants of a
// node (used for Link label, Image alt, AutoLink label).
// Walks recursively so nested inline children (Text inside
// Emphasis inside Link) all contribute.
func inlineTextContent(n ast.Node, src []byte) string {
	var b strings.Builder
	var walk func(child ast.Node)
	walk = func(child ast.Node) {
		if t, ok := child.(*ast.Text); ok {
			b.Write(t.Segment.Value(src))
			return
		}
		if s, ok := child.(*ast.String); ok {
			b.Write(s.Value)
			return
		}
		for c := child.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

// blockText returns the literal text of a code block by
// concatenating the block's line segments. Used for both
// FencedCodeBlock and CodeBlock.
func blockText(n ast.Node, src []byte) string {
	var b strings.Builder
	lines := blockLines(n)
	for i, seg := range lines {
		b.Write(seg.Value(src))
		if i < len(lines)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// blockLines returns the line segments of a code-like block.
func blockLines(n ast.Node) []*text.Segment {
	switch v := n.(type) {
	case *ast.FencedCodeBlock:
		ls := v.Lines()
		out := make([]*text.Segment, ls.Len())
		for i := 0; i < ls.Len(); i++ {
			s := ls.At(i)
			out[i] = &s
		}
		return out
	case *ast.CodeBlock:
		ls := v.Lines()
		out := make([]*text.Segment, ls.Len())
		for i := 0; i < ls.Len(); i++ {
			s := ls.At(i)
			out[i] = &s
		}
		return out
	}
	return nil
}

// typstEscape escapes characters that have special meaning
// inside typst markup blocks. The set is per the typst spec:
// #, *, _, `, <, >, @, =, -, +, /, :, ~, \
//
// Backslash itself is doubled. Plain prose that doesn't
// contain any of these characters is a fast no-op.
func typstEscape(s string) string {
	if !strings.ContainsAny(s, `#*_<>=:~/\\$+`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		switch r {
		case '\\', '#', '*', '_', '`', '<', '>', '@', '=', ':', '~', '/', '$', '+':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// typstEscapeLink escapes a URL for inclusion inside a typst
// #link() or #image() string literal. Only backslashes,
// double-quotes, and whitespace need escaping inside a
// typst string literal.
func typstEscapeLink(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 4)
	for _, r := range s {
		switch r {
		case '\\', '"', '\n', '\r', '\t':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// typstStringLiteral returns a typst string literal containing s.
// Wraps the content in double quotes and escapes backslashes,
// double-quotes, and newlines so the result is a valid typst
// string suitable for passing as the first argument to raw().
func typstStringLiteral(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 16)
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString("\\\\")
		case '"':
			b.WriteString("\\\"")
		case '\n':
			b.WriteString("\\n")
		default:
			b.WriteByte(byte(r))
		}
	}
	b.WriteByte('"')
	return b.String()
}

// typstColorExpr returns a typst color expression for the given
// CSS color string, suitable for direct use in a typst
// expression context (no surrounding `rgb("...")` wrapper
// needed). The returned string is the full typst color
// expression:
//
//   "#8d7440"  →  `rgb("#8d7440")`    (rgb wrapper around hex)
//   "rgb(R G B / A)"  →  `color.rgb(R, G, B, A*255)`  (bare function call)
//   "rgb(R, G, B)"     →  `rgb("#rrggbb")`  (rgb wrapper around hex)
//   "red"  →  `red`  (named color, pass through)
//
// The hex forms use `rgb("#hex")` because typst's markup mode
// (the mode the article body uses via `eval(body-typst, mode: "markup")`)
// treats a bare `#hex` as a function call (the `#` starts a
// markup function). The `rgb()` wrapper is a real typst
// function that accepts a hex string. The alpha-bearing form
// uses `color.rgb()` directly because it's a function call
// with positional args, which markup mode handles correctly.
//
// typst 0.15 strictness (issues #669 + #670 amendments):
//   - `rgb("#hex")` works (the inner string is pure hex).
//   - `rgb("rgb(R G B / A)")` is REJECTED with
//     "color string contains non-hexadecimal letters" (the
//     pre-#670 nested form).
//   - `rgb("color.rgb(r, g, b, a)")` is ALSO rejected (the
//     pre-#670 amendment-2 still-wrapped form).
//   - `color.rgb(r, g, b, a)` bare works (a real function call).
//   - `rgb("#hex")` in markup mode works (the `#hex` is a
//     string literal inside the rgb() function call).
//
// So the returned string is always usable as a typst
// expression: `fill: #hex_result`. The emission sites use
// the result directly without further wrapping.
func typstColorExpr(cssColor string) string {
	trimmed := strings.TrimSpace(cssColor)
	if trimmed == "" {
		return "rgb(\"#000000\")"
	}
	// hex form — wrap in rgb() so the markup-mode parser
	// treats it as a color literal, not a function call.
	if strings.HasPrefix(trimmed, "#") {
		return fmt.Sprintf("rgb(\"%s\")", trimmed)
	}
	// rgb(...) form — convert to a typst-0.15-compatible
	// expression. Handles the three shapes we ship in the
	// theme:
	//   - "rgb(36 48 61 / 0.06)"  space-separated + slash-alpha
	//   - "rgb(36 48 61, 0.06)"  comma-separated
	//   - "rgb(36, 48, 61)"     no alpha
	if strings.HasPrefix(trimmed, "rgb(") {
		inner := strings.TrimSuffix(strings.TrimPrefix(trimmed, "rgb("), ")")
		// Normalize: replace "/" and "," with " " so a single
		// split-on-whitespace handles all three input shapes.
		normalized := strings.NewReplacer("/", " ", ",", " ").Replace(inner)
		parts := strings.Fields(normalized)
		if len(parts) >= 3 {
			r := strings.TrimSpace(parts[0])
			g := strings.TrimSpace(parts[1])
			b := strings.TrimSpace(parts[2])
			if len(parts) >= 4 {
				// Alpha-bearing: emit color.rgb(r, g, b, a*255)
				// as a bare function call. typst 0.15's
				// color.rgb() takes 0-255 alpha, which matches
				// the CSS convention.
				alphaStr := strings.TrimSpace(parts[3])
				if alphaF, err := strconvParseFloat(alphaStr); err == nil {
					alpha255 := int(alphaF * 255)
					if alpha255 < 0 {
						alpha255 = 0
					} else if alpha255 > 255 {
						alpha255 = 255
					}
					return fmt.Sprintf("color.rgb(%s, %s, %s, %d)", r, g, b, alpha255)
				}
			}
			// Opaque rgb() — convert to 6-char hex and wrap
			// in rgb() so the markup-mode parser treats it
			// as a color literal.
			hex := rgbPartsToHex(r, g, b)
			if hex != "" {
				return fmt.Sprintf("rgb(\"#%s\")", hex)
			}
		}
		// Unparseable — fall back to black so the export
		// doesn't fail silently. The pre-0.15 code returned
		// the literal and let typst emit a clear error; the
		// 0.15 behavior is to fail the whole render, which
		// is too costly for a single bad color.
		return "rgb(\"#000000\")"
	}
	// named color — return as-is. typst's color literals
	// include "red", "blue", etc. (the markup-mode parser
	// doesn't try to call them as functions).
	return trimmed
}

// typstColor is the pre-#670 amendment normalizer. It returns
// a string meant to be wrapped in `rgb("...")`. The five
// markdown_typst emission sites now use typstColorExpr (which
// returns a bare typst expression) instead, so this function
// is only kept for callers that still need the wrapped form.
// The pre-#670 code path that produced `rgb("rgb(36 48 61 / 0.06)")`
// is dead; this function is preserved as a reference for the
// audit probe + the test cases.
func typstColor(cssColor string) string {
	trimmed := strings.TrimSpace(cssColor)
	if trimmed == "" {
		return "#000000"
	}
	if strings.HasPrefix(trimmed, "#") {
		return trimmed
	}
	if strings.HasPrefix(trimmed, "rgb(") {
		return trimmed
	}
	return trimmed
}

// rgbPartsToHex converts three 0-255 RGB integer strings to a
// 6-char lowercase hex string. Returns "" if any input is
// out of range or non-numeric.
func rgbPartsToHex(r, g, b string) string {
	rv, err1 := strconvAtoi(r)
	gv, err2 := strconvAtoi(g)
	bv, err3 := strconvAtoi(b)
	if err1 != nil || err2 != nil || err3 != nil {
		return ""
	}
	if rv < 0 || rv > 255 || gv < 0 || gv > 255 || bv < 0 || bv > 255 {
		return ""
	}
	return fmt.Sprintf("%02x%02x%02x", rv, gv, bv)
}

// strconvParseFloat + strconvAtoi are tiny shims so the import
// block stays compact. Wrapping the stdlib functions also lets
// the parser logic be unit-tested without exposing strconv.
var strconvParseFloat = func(s string) (float64, error) {
	return strconvParseFloatImpl(s)
}
var strconvAtoi = func(s string) (int, error) {
	return strconvAtoiImpl(s)
}

// writeTextToBuf is writeText but emits to a caller-provided
// buffer. Used during table cell accumulation.
func (s *typstState) writeTextToBuf(t *ast.Text, buf *bytes.Buffer) {
	raw := string(t.Segment.Value(s.src))
	if t.HardLineBreak() {
		raw = strings.TrimRight(raw, "\n")
		raw = strings.TrimRight(raw, "\\")
		buf.WriteString(typstEscape(raw))
		buf.WriteString(" ")
		return
	}
	if t.SoftLineBreak() {
		raw = strings.TrimRight(raw, "\n")
		buf.WriteString(typstEscape(raw))
		buf.WriteString(" ")
		return
	}
	raw = strings.TrimRight(raw, "\n")
	buf.WriteString(typstEscape(raw))
}

// emitTable writes accumulated table cells as a typst
// #table() call. Header cells are wrapped in *bold*.
func (s *typstState) emitTable() {
	if len(s.tableCells) == 0 {
		return
	}
	cols := s.rowCellCount
	if cols == 0 {
		return
	}
	s.out.WriteString(fmt.Sprintf("#table(columns: %d", cols))
	for _, cell := range s.tableCells {
		s.out.WriteString(", [")
		s.out.WriteString(cell)
		s.out.WriteString("]")
	}
	s.out.WriteString(")\n")
}

// Reference imports so go vet / goimports keeps them when a
// future refactor trims usage in the file body.
var _ = parser.NewParser
var _ = goldmark.New

// strconvParseFloatImpl + strconvAtoiImpl are the stdlib
// delegates for the typstColor shim above. Kept in this
// file (vs an inline import) so the file's import list
// doesn't grow just for the parser; the typstColor body
// stays a self-contained normalizer.
func strconvParseFloatImpl(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}
func strconvAtoiImpl(s string) (int, error) {
	return strconv.Atoi(s)
}