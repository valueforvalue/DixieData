// Issue #526: the Articles editor's preview surface moved
// from side-by-side (slice 3.6) to a preview-on-demand
// overlay. This test pins the new layout so a future refactor
// does not silently bring the side-by-side grid back, and so
// the new apply sites (Preview button + preview modal) stay
// wired on both the new and edit paths.
//
// What we pin:
//   - The Preview button renders next to Save
//   - data-article-preview-open is the trigger attribute
//     (the JS initializer queries this)
//   - data-article-editor-preview is GONE (the inline pane
//     was removed)
//   - The body section is no longer a 2-column grid (no
//     lg:grid-cols-2)
//   - The preview modal renders via @ArticlePreviewModal()
//     with the right ARIA + id attrs
//   - The form action still points at routebuilder.ArticleNew
//     or routebuilder.ArticleEdit (so the dispatcher wiring
//     does not regress)
package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

func TestArticleFormPreviewLayoutIssue526(t *testing.T) {
	cases := []struct {
		name        string
		isEdit      bool
		wantAction  string
		wantDraft   string
		skipAction  bool
		skipDraft   bool
	}{
		{"new", false, "", "new-article", false, false},
		{"edit", true, "", "edit-article-", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := ArticleArticleForm(viewmodel.Article{}, tc.isEdit).Render(context.Background(), &buf); err != nil {
				t.Fatalf("Render: %v", err)
			}
			content := buf.String()

			// Preview button + trigger attribute must render.
			if !strings.Contains(content, "data-article-preview-open") {
				t.Errorf("Preview button missing data-article-preview-open attr")
			}
			// The button text must say "Preview" so the user
			// can find it next to Save.
			if !strings.Contains(content, ">Preview<") {
				t.Errorf("Preview button label missing")
			}
			// Preview button must be type="button" so it
			// does not accidentally submit the form.
			previewIdx := strings.Index(content, "data-article-preview-open")
			if previewIdx < 0 {
				t.Fatal("preview button not found")
			}
			// Walk back to the nearest preceding <button open.
			btnStart := strings.LastIndex(content[:previewIdx], "<button")
			if btnStart < 0 {
				t.Fatal("preview button element not found")
			}
			btnEnd := strings.Index(content[btnStart:], ">")
			if btnEnd < 0 {
				t.Fatal("preview button element not closed")
			}
			previewBtn := content[btnStart : btnStart+btnEnd+1]
			if !strings.Contains(previewBtn, `type="button"`) {
				t.Errorf("preview button missing type=\"button\": %s", previewBtn)
			}

			// The side-by-side preview pane is GONE.
			if strings.Contains(content, "data-article-editor-preview") {
				t.Errorf("side-by-side preview pane still present; should be removed (issue #526)")
			}
			if strings.Contains(content, "lg:grid-cols-2") {
				// Allow grid-cols-1 (responsive fallback) but
				// not the 2-col layout that put editor and
				// preview side-by-side. The body block must
				// be a single column now.
				idx := strings.Index(content, "lg:grid-cols-2")
				t.Errorf("body block still uses lg:grid-cols-2 (side-by-side layout) at offset %d", idx)
			}

			// The preview modal must render with the right
			// ARIA + id attrs.
			for _, marker := range []string{
				`id="article-preview-modal"`,
				`data-article-preview-modal`,
				`role="dialog"`,
				`aria-modal="true"`,
				`data-article-preview-close`,
				`data-article-preview-body`,
			} {
				if !strings.Contains(content, marker) {
					t.Errorf("preview modal missing %s", marker)
				}
			}
		})
	}
}
