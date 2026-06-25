package templates

import (
	"bytes"
	"context"
	"fmt"
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

func TestBrowseViewTableHeadersDeclareScopeCol(t *testing.T) {
	var buf bytes.Buffer
	err := BrowseView(
		[]viewmodel.PersonRecord{{ID: 1, DisplayID: "STC38-00001", FirstName: "John", LastName: "Carter"}},
		viewmodel.BrowseState{Page: 1, PageSize: 100, Scope: "all", Sort: "display_id_asc"},
		viewmodel.PersonRecordFormSuggestions{},
	).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	for _, column := range []string{"display_id", "name", "entry_type", "rank_out", "unit", "pension_state", "review_status", "last_edited"} {
		needle := fmt.Sprintf(`scope="col" data-browse-column=%q`, column)
		if !strings.Contains(content, needle) {
			t.Fatalf("browse table <th> for column %s should declare scope='col'", column)
		}
	}
	if strings.Contains(content, `<th class="px-4 py-3 font-semibold uppercase tracking-[0.18em] text-xs" data-browse-column=`) {
		t.Fatalf("every data-browse-column <th> should carry scope='col'")
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
