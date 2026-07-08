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
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
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
	md := goldmark.New()
	doc := md.Parser().Parse(text.NewReader(srcBytes))

	state := &typstState{src: srcBytes}
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
	case *ast.CodeSpan:
		if entering {
			s.out.WriteString("`")
		} else {
			s.out.WriteString("`")
		}
		// Walker walks the Text children which emit the content.
		// No skip -- the text needs to be written by the walker.
	case *ast.Link:
		if entering {
			dest := string(v.Destination)
			// Link emits its opening #link("dest")[ here, then
			// the children walk produces the label content, then
			// the Exit branch closes the brackets.
			fmt.Fprintf(&s.out, `#link("%s")[`, typstEscapeLink(dest))
		} else {
			s.out.WriteString("]")
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
			s.out.WriteString("#quote(block: true)[\n")
		} else {
			s.out.WriteString("]\n")
		}
	case *ast.FencedCodeBlock:
		if entering {
			lang := string(v.Language(s.src))
			content := blockText(v, s.src)
			// typst has #raw(block: true, lang: "go")[...] but
			// syntax highlighting requires a typst plugin
			// (`@preview/ctyp`) that the article template
			// doesn't ship. Render as a plain raw block for
			// now. When highlighting support lands, prepend
			// `lang: "..."` to the raw() call.
			_ = lang
			fmt.Fprintf(&s.out, "#raw(block: true, lang: \"\")[\n%s\n]\n", typstEscape(content))
		}
	case *ast.CodeBlock:
		if entering {
			content := blockText(v, s.src)
			fmt.Fprintf(&s.out, "#raw(block: true)[\n%s\n]\n", typstEscape(content))
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
			s.writeText(v)
		}
	case *ast.String:
		if entering {
			s.out.WriteString(typstEscape(string(v.Value)))
		}
	case *ast.HTMLBlock, *ast.RawHTML, *ast.LinkReferenceDefinition:
		// Stripped -- bluemonday already filtered the sanitized
		// output, so anything left here is empty or unwanted.
		// Skipping via returning WalkSkipChildren is more
		// efficient than emitting nothing for the children.
		return ast.WalkSkipChildren, nil
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
	s.out.WriteString(label.String())
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
	if !strings.ContainsAny(s, `#*_<>=:~/\\+`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		switch r {
		case '\\', '#', '*', '_', '`', '<', '>', '@', '=', ':', '~', '/', '+':
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

// Reference imports so go vet / goimports keeps them when a
// future refactor trims usage in the file body.
var _ = parser.NewParser
var _ = goldmark.New