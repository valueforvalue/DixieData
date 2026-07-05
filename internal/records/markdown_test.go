// markdown_test.go — pins the slice-3.6 markdown render
// contract (issue #321): the MarkdownRenderer (goldmark +
// bluemonday Strict) sanitizes raw HTML while preserving
// the markdown syntax output. The Strict policy is the
// locked decision per slice-3 user approval.
package records

import (
	"strings"
	"testing"
)

func TestMarkdownRenderer_RendersBasicMarkdown(t *testing.T) {
	r := NewMarkdownRenderer()

	cases := []struct {
		name     string
		source   string
		mustHave []string
		mustNot  []string
	}{
		{
			name:     "heading renders to h1",
			source:   "# Hello",
			mustHave: []string{"<h1>", "Hello"},
			mustNot:  []string{"<script>"},
		},
		{
			name:     "paragraph renders",
			source:   "Just a paragraph of text.",
			mustHave: []string{"<p>", "Just a paragraph of text."},
		},
		{
			name:     "person token survives sanitization",
			source:   "See [John Doe](#person/D-00123) for context.",
			mustHave: []string{"<a", `href="#person/D-00123"`},
			mustNot:  []string{"<script>"},
		},
		{
			name:     "script tag is stripped by Strict",
			source:   "Hello <script>alert(1)</script> world.",
			mustNot:  []string{"<script>"},
			mustHave: []string{"Hello", "world"},
		},
		{
			name:     "iframe is stripped by Strict",
			source:   "Before <iframe src='evil'>x</iframe> after.",
			mustNot:  []string{"<iframe"},
			mustHave: []string{"Before", "after"},
		},
		{
			name:     "style tag is stripped by Strict",
			source:   "Pre <style>body{color:red}</style> post.",
			mustNot:  []string{"<style"},
			mustHave: []string{"Pre", "post"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := r.Render(tc.source)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			for _, want := range tc.mustHave {
				if !strings.Contains(out, want) {
					t.Errorf("Render output missing %q\n--- got ---\n%s\n--- end ---", want, out)
				}
			}
			for _, banned := range tc.mustNot {
				if strings.Contains(out, banned) {
					t.Errorf("Render output contains banned %q\n--- got ---\n%s\n--- end ---", banned, out)
				}
			}
		})
	}
}

func TestMarkdownRenderer_EmptySourceReturnsEmpty(t *testing.T) {
	r := NewMarkdownRenderer()
	out, err := r.Render("")
	if err != nil {
		t.Fatalf("Render empty: %v", err)
	}
	if out != "" {
		t.Errorf("Render empty = %q, want \"\"", out)
	}
}

func TestMarkdownRenderer_DoesNotErrorOnMultilineMarkdown(t *testing.T) {
	r := NewMarkdownRenderer()
	source := `# Title

First paragraph.

## Subsection

- bullet one
- bullet two
- bullet three

> A blockquote.
`
	out, err := r.Render(source)
	if err != nil {
		t.Fatalf("Render multiline: %v", err)
	}
	if !strings.Contains(out, "<h1>") {
		t.Errorf("Render multiline missing h1")
	}
	if !strings.Contains(out, "<h2>") {
		t.Errorf("Render multiline missing h2")
	}
	if !strings.Contains(out, "<ul>") {
		t.Errorf("Render multiline missing ul")
	}
}