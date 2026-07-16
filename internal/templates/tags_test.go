// tags_test.go -- /tags page render pinning.
//
// The TagsManagementPage templ renders the /tags
// landing page. This file pins the visible / a11y
// contract for the Merge picker placeholder text
// (issue #605). The original "Pick a survivor tag…"
// placeholder relied on SQL-MERGE "survivor" jargon
// the user-visible copy couldn't rely on; the post-fix
// "Merge into which tag…" phrasing reads as a
// continuation of the Merge button label.
package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/records"
	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

// TestTagsManagementPageMergePickerPlaceholder pins the
// issue #605 placeholder text. The Merge picker's
// visible placeholder option reads "Merge into which
// tag…". Defensive: the old jargon ("Pick a survivor
// tag…") must not return.
func TestTagsManagementPageMergePickerPlaceholder(t *testing.T) {
	tags := []records.Tag{
		{ID: 1, Name: "Gettysburg", MemberCount: 5},
		{ID: 2, Name: "1st Texas", MemberCount: 3},
		{ID: 3, Name: "virtual-cemetery", MemberCount: 2},
	}
	counts := viewmodel.ArchiveCounts{TagCount: 3}
	var buf bytes.Buffer
	if err := TagsManagementPage(tags, counts).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	// The new phrasing must be present.
	if !strings.Contains(content, `Merge into which tag…`) {
		t.Errorf(`Merge picker placeholder text is missing "Merge into which tag…" (issue #605 — replace SQL-MERGE jargon with action-aligned language)`)
	}
	// The old jargon must not return.
	if strings.Contains(content, `Pick a survivor tag`) {
		t.Errorf(`Merge picker placeholder text re-introduced the SQL-MERGE jargon ("Pick a survivor tag…") — issue #605 (placeholder clarity) regressed`)
	}
	// The accessible label on the parent <select>
	// stays as written — the screen-reader
	// announcement is unchanged.
	if !strings.Contains(content, `aria-label="Merge tag`) {
		t.Errorf(`Merge picker aria-label is missing the "Merge tag <X> into which tag" canonical screen-reader announcement`)
	}
}
