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
	if !strings.Contains(content, "xl:flex-row xl:items-start xl:justify-between") {
		t.Fatalf("browse view should keep top summary stacked until xl widths")
	}
	for _, needle := range []string{
		"md:grid-cols-2",
		"xl:grid-cols-6",
		"md:col-span-2",
		"xl:col-span-2",
		"sm:flex-row sm:flex-wrap sm:items-end",
		"flex flex-col gap-2 sm:flex-row sm:flex-wrap sm:items-center",
		"w-full sm:w-auto",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("browse view missing responsive browse contract %s", needle)
		}
	}
}

func TestBrowseViewShowsBuriedInFilter(t *testing.T) {
	var buf bytes.Buffer
	err := BrowseView(nil, viewmodel.BrowseState{
		Page:     1,
		PageSize: 100,
		Scope:    "all",
		Sort:     "display_id_asc",
		BuriedIn: "Oak Hill Cemetery",
	}, viewmodel.PersonRecordFormSuggestions{
		BuriedIn: []string{"Oak Hill Cemetery"},
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{"name=\"buried_in\"", "browse-buried-in-suggestions", "value=\"Oak Hill Cemetery\""} {
		if !strings.Contains(content, needle) {
			t.Fatalf("browse view missing %s", needle)
		}
	}
}

func TestBrowseResultsDoNotShowBlankPensionStateAsActiveFilter(t *testing.T) {
	var buf bytes.Buffer
	err := BrowseResults(nil, viewmodel.BrowseState{
		Page:     1,
		PageSize: 100,
		Total:    320,
		Scope:    "all",
		Sort:     "display_id_asc",
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	if strings.Contains(content, "Pension State: N/A") {
		t.Fatalf("browse results should not show a blank pension state as an active filter")
	}
	if strings.Contains(content, "pension_state=N%2FA") {
		t.Fatalf("browse pager should not include a blank pension state filter")
	}
}
