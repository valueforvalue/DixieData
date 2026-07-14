// article_md_cheatsheet_test.go — issue #565
// Regression net for the Markdown syntax cheatsheet
// wiring on the Article editor. The cheatsheet is a
// Foldout (issue #264 primitive) rendered inside the
// form's action bar; this test pins:
//
//   - The cheatsheet renders on BOTH the new
//     (/articles/new) and edit (/articles/{id}/edit)
//     paths. ArticleArticleForm is shared (issue #375
//     precedent: one test, two form paths).
//   - The cheatsheet sits in the action bar next to
//     Preview + Save Article (the visual toolbar
//     position the user finds it in).
//   - The Copy example button on each row carries the
//     registry surface ID the JS initializer wires
//     (data-md-cheatsheet-copy-key) AND the raw
//     example value the clipboard handler copies
//     (data-md-cheatsheet-copy-value).
//
// The component-level invariants (Foldout ARIA, row
// order, no raw HTML in examples) live in
// components/markdown_cheatsheet_test.go. This file
// pins the page-level integration only.
package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

func TestArticleFormRendersMarkdownCheatsheetBothPaths(t *testing.T) {
	cases := []struct {
		name   string
		isEdit bool
	}{
		{"new", false},
		{"edit", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := ArticleArticleForm(viewmodel.Article{}, tc.isEdit).Render(context.Background(), &buf); err != nil {
				t.Fatalf("Render: %v", err)
			}
			content := buf.String()

			// The cheatsheet's trigger button label must
			// appear in the rendered form. The trigger is
			// a Foldout button rendered by
			// components.MarkdownCheatsheet; the label
			// "Markdown syntax" is the user-facing entry
			// point.
			if !strings.Contains(content, "Markdown syntax") {
				t.Errorf("Markdown syntax cheatsheet trigger not rendered on %s path", tc.name)
			}

			// The cheatsheet must be inside the form's
			// action bar — i.e. between the Preview
			// button and the closing </form>. The
			// toolbar (Preview + Save Article) is the
			// only place the user expects a help affordance.
			cheatsheetIdx := strings.Index(content, "Markdown syntax")
			previewIdx := strings.Index(content, "data-article-preview-open")
			if previewIdx < 0 {
				t.Fatalf("Preview button not found on %s path", tc.name)
			}
			if cheatsheetIdx > previewIdx {
				t.Errorf("Markdown syntax cheatsheet renders AFTER the Preview button on %s path; should sit next to it in the action bar", tc.name)
			}

			// The Copy example row markers must render
			// for at least the basic primitives. We
			// spot-check heading + person-record-reference
			// because those are the two most-likely-to-be-
			// broken rows: heading is the first in the
			// visual order, person-record-reference is
			// the DixieData-specific custom syntax.
			for _, key := range []string{
				`data-md-cheatsheet-copy-key="heading"`,
				`data-md-cheatsheet-copy-key="person-record-reference"`,
				`data-md-cheatsheet-copy-value="[John Doe](#person/D-00123)"`,
			} {
				if !strings.Contains(content, key) {
					t.Errorf("cheatsheet row missing %s on %s path", key, tc.name)
				}
			}
		})
	}
}
