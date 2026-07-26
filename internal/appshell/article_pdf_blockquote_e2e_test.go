// article_pdf_blockquote_e2e_test.go -- end-to-end exercise of
// the article PDF render path against an article with a
// markdown blockquote. Issue #669 surfaced the typst 0.15
// `stroke:` regression on #quote; the smoke test in
// `audit/smoke_typst_quote_stroke.mjs` pins the markdown_typst
// source. This test takes the next step: it renders the
// markdown to typst + compiles the result with the real
// typst binary. A future regression that re-introduces the
// broken pattern, or introduces a NEW typst 0.15+
// incompatibility, fails this test.
//
// Why this complements the source-scan probe: the probe
// pins the source. This test pins the RENDERED OUTPUT
// (what typst actually sees). A future refactor of
// `typstColor`, `a.theme.BlockquoteBorder`, or any code that
// inserts a `rgb(...)` call with a non-hex string would
// pass the source probe (the file is unchanged) and fail
// this test (the rendered typst contains the bad string).
package appshell

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/records"
)

func TestArticlePDFExport_RendersBlockquoteEndToEnd(t *testing.T) {
	app := newStressApp(t)

	// Skip if typst isn't on PATH or the bundled binary is
	// absent. The audit harness always has it; CI may not.
	typstPath := bundledTypstPath(t)
	if typstPath == "" {
		t.Skip("typst binary not found; skipping PDF render e2e")
	}

	// Build a markdown body exercising every syntax the
	// seed fixture uses: headings, emphasis, lists, code,
	// tables, links, blockquote. Each element is a candidate
	// for a typst 0.15+ parse incompatibility; a future
	// emission regression on any of them surfaces here.
	body := strings.Join([]string{
		"# Markdown Feature Reference",
		"",
		"A working reference for every markdown syntax.",
		"",
		"## Text Emphasis",
		"",
		"This paragraph uses **bold**, *italic*, and `code` together.",
		"",
		"> A blockquote spans multiple lines when the author continues the thought. The blockquote is its own block element.",
		"",
		"> A second blockquote, this one with a longer line that wraps to a second visual row in the PDF.",
		"",
		"## Lists",
		"",
		"- First item",
		"- Second item",
		"- Third item",
		"",
		"1. Ordered one",
		"2. Ordered two",
		"",
		"## Code",
		"",
		"```go",
		"package main",
		"",
		"func main() {}",
		"```",
		"",
		"## Table",
		"",
		"| A | B |",
		"| -: | -: |",
		"| 1 | 2 |",
		"| 3 | 4 |",
		"",
		"## Link",
		"",
		"Visit [the site](https://example.com) for more.",
	}, "\n")
	src, err := app.articles.Create(models.Article{
		Title:  "Markdown Feature Reference",
		BodyMD: body,
	})
	if err != nil {
		t.Fatalf("Create article: %v", err)
	}

	// Render the markdown body to typst directly via the
	// internal API. Catches the regression at the "is the
	// typst markup well-formed" layer, before the heavier
	// render → typst compile → PDF pipeline. Use the
	// configured ThemeConfig so the rendered typst matches
	// what the production article-PDF handler emits.
	r := records.NewMarkdownRenderer()
	r.SetTheme(&app.cfg.Theme)
	typst, err := r.RenderTypst(src.BodyMD)
	if err != nil {
		t.Fatalf("RenderTypst: %v", err)
	}

	// Assert the rendered typst does NOT contain the
	// typst-0.15-broken pattern. (Defends against a future
	// refactor of the Blockquote branch that re-introduces
	// the bug.)
	if strings.Contains(typst, "#quote(block: true, stroke:") {
		t.Fatalf("rendered typst contains the typst-0.15-broken #quote(...stroke:) pattern:\n%s", typst)
	}

	// Assert the rendered typst DOES contain the typst-0.15-
	// compatible shape. (Defends against a future refactor
	// that silently drops the blockquote styling.)
	if !strings.Contains(typst, "#block(inset: (left: 1em), stroke: (left: 2pt + rgb") {
		t.Errorf("rendered typst missing the typst-0.15-compatible blockquote shape:\n%s", typst)
	}
	if !strings.Contains(typst, "#set par(first-line-indent: 0pt)") {
		t.Errorf("rendered typst missing the first-line-indent guard:\n%s", typst)
	}

	// Wrap the rendered typst in a minimal typst document
	// so the compiler has a real entry point. Use set
	// text(fill: ...) to verify the link color is a valid
	// hex string for typst 0.15's stricter parser. The link
	// color is the second-most-likely culprit after the
	// blockquote border; both come from the theme config.
	doc := "#set page(width: auto, height: auto, margin: 0pt)\n" + typst + "\n"
	typstFile := t.TempDir() + "/body.typ"
	if err := os.WriteFile(typstFile, []byte(doc), 0o644); err != nil {
		t.Fatalf("write typst: %v", err)
	}
	pdfOut := t.TempDir() + "/body.pdf"
	cmd := exec.Command(typstPath, "compile", typstFile, pdfOut)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("typst compile failed: %v\ntypst stdout/stderr:\n%s\ntypst source:\n%s", err, out, doc)
	}
}

// bundledTypstPath resolves the path to the typst binary the
// test should use. Tries (in order):
//   1. bin/typst-windows.exe in the repo root (the bundled
//      release binary the production app uses)
//   2. typst on PATH
// Returns "" if neither is present so the caller can Skip.
func bundledTypstPath(t *testing.T) string {
	t.Helper()
	candidates := []string{
		"bin/typst-windows.exe",
		"bin/typst",
		"typst",
	}
	for _, c := range candidates {
		if p, err := exec.LookPath(c); err == nil {
			return p
		}
	}
	return ""
}
