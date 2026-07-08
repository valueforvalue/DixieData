// Issue #431: /articles/{id} detail page had no Edit CTA. The
// route /articles/{id}/edit + the edit surface + the URL helper
// already existed; only the link in the detail page header was
// missing. This test pins the acceptance criterion: the detail
// page renders an Edit link next to the DisplayID, the link
// points at /articles/{id}/edit, and it carries a stable
// data-article-edit-link selector for the smoke probe.
package templates

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

func TestArticleDetailShellRendersEditLink(t *testing.T) {
	cases := []struct {
		name string
		id   int64
	}{
		{"seed-data article id=1", 1},
		{"fresh article id=42", 42},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			view := viewmodel.Article{
				ID:        tc.id,
				DisplayID: fmt.Sprintf("ART-%05d", tc.id),
				Title:     "Sample",
				Body:      "body",
			}

			var buf bytes.Buffer
			err := ArticleDetailShell(view).Render(context.Background(), &buf)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			content := buf.String()

			// The link must carry the canonical data-article-edit-link
			// selector so the smoke probe can target it.
			idx := strings.Index(content, "data-article-edit-link")
			if idx < 0 {
				t.Fatalf("Edit link missing on /articles/{id} detail page; expected data-article-edit-link selector")
			}

			// Walk back to the enclosing <a tag and forward to its
			// closing > so the href + label are inspectable.
			preceding := content[:idx]
			aStart := strings.LastIndex(preceding, "<a")
			if aStart < 0 {
				t.Fatalf("Edit link element not found before selector")
			}
			aEnd := strings.Index(content[aStart:], ">")
			if aEnd < 0 {
				t.Fatalf("Edit link element not closed")
			}
			anchor := content[aStart : aStart+aEnd+1]

			// The href must point at /articles/{id}/edit so the
			// existing edit surface renders.
			wantHref := fmt.Sprintf(`href="/articles/%d/edit"`, tc.id)
			if !strings.Contains(anchor, wantHref) {
				t.Errorf("Edit link href missing or wrong; want %q, got: %s", wantHref, anchor)
			}

			// The link must carry visible "Edit" label so users
			// know what it does.
			if !strings.Contains(content, ">Edit<") {
				t.Errorf("Edit link visible label missing; expected >Edit< somewhere in rendered page")
			}
		})
	}
}