package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

func TestBrowseViewShowsSelectionHelperAndPrintAction(t *testing.T) {
	var buf bytes.Buffer
	err := BrowseView(nil, viewmodel.BrowseState{
		Page:     1,
		PageSize: 100,
		Scope:    "all",
		Sort:     "display_id_asc",
	}, viewmodel.PersonRecordFormSuggestions{}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{"Print/Export Selected", "Use the Select checkboxes to build a working set across pages", "/share?openPrintConfig=1", "data-browse-reset"} {
		if !strings.Contains(content, needle) {
			t.Fatalf("browse view missing %s", needle)
		}
	}
}
