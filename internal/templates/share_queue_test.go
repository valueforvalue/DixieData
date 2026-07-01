package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

// TestShareQueuePageRendersEmptyState (issue #193) asserts
// the management page renders the empty-state copy when the
// rows slice is nil/empty.
func TestShareQueuePageRendersEmptyState(t *testing.T) {
	var buf bytes.Buffer
	err := ShareQueuePage(nil).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	for _, needle := range []string{
		"Manage your staged subset",
		"No Person Records staged",
		"Remove Selected",
		"Export Selected as .ddshare",
	} {
		if !strings.Contains(content, needle) {
			t.Errorf("page missing %s", needle)
		}
	}
}

// TestShareQueuePageRendersRows (issue #193) asserts the
// table populates with one row per ShareQueueRow and shows
// Display ID + Name + Unit + counts.
func TestShareQueuePageRendersRows(t *testing.T) {
	var buf bytes.Buffer
	rows := []viewmodel.ShareQueueRow{
		{
			Order: 1,
			PersonRecord: viewmodel.PersonRecord{
				ID:                42,
				DisplayID:         "DXD-MGT-42",
				Unit:              "4th Alabama",
				SourceRecordCount: 2,
				ImageCount:        5,
			},
		},
		{
			Order: 2,
			PersonRecord: viewmodel.PersonRecord{
				ID:        87,
				DisplayID: "DXD-MGT-87",
				Unit:      "5th Virginia",
			},
		},
	}
	err := ShareQueuePage(rows).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	for _, needle := range []string{
		`data-share-queue-page-row-id="42"`,
		`data-share-queue-page-row-id="87"`,
		"DXD-MGT-42",
		"DXD-MGT-87",
		"4th Alabama",
		"5th Virginia",
		`data-share-queue-page-remove-id="42"`,
		`data-share-queue-page-remove-id="87"`,
		`data-share-queue-page-select-all`,
	} {
		if !strings.Contains(content, needle) {
			t.Errorf("page missing %s; got %s", needle, content)
		}
	}
}