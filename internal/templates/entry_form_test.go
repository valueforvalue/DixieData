package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

func TestEntryFormOmitsInlineScratchPadLauncher(t *testing.T) {
	var buf bytes.Buffer
	err := EntryForm(viewmodel.Soldier{DisplayID: "DXD-00001"}, nil, viewmodel.SoldierFormSuggestions{}, viewmodel.FindAGraveScrapeState{}, false).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	if strings.Contains(content, "Open Scratch Pad") {
		t.Fatalf("entry form should not render an inline scratch pad button")
	}
	if !strings.Contains(content, `name="birth_date"`) || !strings.Contains(content, `name="death_date"`) {
		t.Fatalf("entry form missing canonical date fields")
	}
	if !strings.Contains(content, `data-scratchpad-display-id="DXD-00001"`) {
		t.Fatalf("entry form should surface a page-level scratch pad display id")
	}
}

func TestEntryFormKeepsDisplayIDReadonlyOnEdit(t *testing.T) {
	var buf bytes.Buffer
	err := EntryForm(viewmodel.Soldier{DisplayID: "DXD-00001"}, nil, viewmodel.SoldierFormSuggestions{}, viewmodel.FindAGraveScrapeState{}, true).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	if !strings.Contains(content, `name="display_id"`) || !strings.Contains(content, `readonly`) {
		t.Fatalf("entry form should render display_id as readonly on edit")
	}
	if !strings.Contains(content, `data-draft-key="edit-soldier-0"`) || !strings.Contains(content, `data-record-persistence`) {
		t.Fatalf("entry form should render edit draft persistence metadata")
	}
}

func TestEntryFormIncludesSpouseFields(t *testing.T) {
	var buf bytes.Buffer
	err := EntryForm(viewmodel.Soldier{EntryType: "wife", LinkedSoldierID: 7}, []viewmodel.Soldier{
		{ID: 7, DisplayID: "TDM65-DXD-00007", FirstName: "John", LastName: "Smith"},
	}, viewmodel.SoldierFormSuggestions{}, viewmodel.FindAGraveScrapeState{}, false).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	if !strings.Contains(content, `name="entry_type"`) || !strings.Contains(content, `data-entry-type-select`) {
		t.Fatalf("entry form missing entry type selector")
	}
	if !strings.Contains(content, `Generic Linked Person`) {
		t.Fatalf("entry form missing generic linked person label")
	}
	if !strings.Contains(content, `name="spouse_soldier_id"`) || !strings.Contains(content, `name="maiden_name"`) {
		t.Fatalf("entry form missing spouse-specific fields")
	}
	if !strings.Contains(content, `John`) || !strings.Contains(content, `TDM65-DXD-00007`) {
		t.Fatalf("entry form missing spouse candidate option")
	}
}

func TestShareViewIncludesSeparatedImportAndExportActions(t *testing.T) {
	var buf bytes.Buffer
	err := ShareView(viewmodel.GoogleStatus{}, nil).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	if !strings.Contains(content, "Export & Backup") || !strings.Contains(content, "Import & Restore") {
		t.Fatalf("share view missing separated import/export sections")
	}
	if !strings.Contains(content, "/import/backup") || !strings.Contains(content, "Load Backup (.ddbak)") {
		t.Fatalf("share view missing backup import action")
	}
	if !strings.Contains(content, "/export/shared-archive") || !strings.Contains(content, "Export Shared Archive (.ddshare)") {
		t.Fatalf("share view missing shared archive export action")
	}
	if !strings.Contains(content, "/export/static-archive") || !strings.Contains(content, "Export Static Web Archive") {
		t.Fatalf("share view missing static web archive export action")
	}
	if !strings.Contains(content, "/export/database-pdf") || !strings.Contains(content, "Full Database Printable PDF") {
		t.Fatalf("share view missing full database PDF export action")
	}
	if !strings.Contains(content, "/import/shared-archive") || !strings.Contains(content, "Import Shared Archive (.ddshare)") {
		t.Fatalf("share view missing shared archive import action")
	}
	if !strings.Contains(content, "/export/bug-report") || !strings.Contains(content, "Support & Diagnostics") {
		t.Fatalf("share view missing diagnostics section")
	}
	if !strings.Contains(content, "/export/feedback-log") || !strings.Contains(content, "Export Feedback Log") {
		t.Fatalf("share view missing feedback log export action")
	}
	if !strings.Contains(content, ".ddbak") || !strings.Contains(content, ".ddshare") {
		t.Fatalf("share view missing custom archive extension copy")
	}
}

func TestShareViewShowsMergeReviewStatus(t *testing.T) {
	var buf bytes.Buffer
	err := ShareView(viewmodel.GoogleStatus{}, []viewmodel.MergeReviewConflict{
		{
			ID:                7,
			ConflictType:      "soldier-update",
			IncomingDisplayID: "STC38-00007",
			Reason:            "Shared record changed notes.",
			IncomingRecord:    viewmodel.Soldier{DisplayID: "STC38-00007", FirstName: "John", LastName: "Taylor"},
		},
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		`id="merge-review-section"`,
		`Data Loaded: 1 Conflicts Found`,
		`data-merge-review-action`,
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("share view missing merge review status UI: %s", needle)
		}
	}
}

func TestInitialSetupViewIncludesIdentityFields(t *testing.T) {
	var buf bytes.Buffer
	err := InitialSetupView(viewmodel.InitialSetupForm{
		FirstName:     "Samuel",
		MiddleName:    "Thomas",
		LastName:      "Carter",
		BirthYear:     "1838",
		PrefixPreview: "STC38",
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	if !strings.Contains(content, `action="/setup"`) || !strings.Contains(content, `name="birth_year"`) {
		t.Fatalf("initial setup view missing setup form fields")
	}
	if !strings.Contains(content, "STC38") {
		t.Fatalf("initial setup view missing prefix preview")
	}
}

func TestNewEntryFormIncludesLocalDraftIndicator(t *testing.T) {
	var buf bytes.Buffer
	err := EntryForm(viewmodel.Soldier{DisplayID: "STC38-00001", PensionState: "None", ConfederateHomeStatus: "None"}, nil, viewmodel.SoldierFormSuggestions{
		RankIn:           []string{"Private", "Sergeant"},
		RankOut:          []string{"Corporal", "Sergeant"},
		Unit:             []string{"Co. A, 1st Texas Infantry"},
		Prefix:           []string{"Capt."},
		Suffix:           []string{"Jr."},
		PensionState:     []string{"None", "Texas"},
		BuriedIn:         []string{"Oakwood Cemetery"},
		ConfederateHome:  []string{},
		SourceRecordType: []string{"Pension"},
	}, viewmodel.FindAGraveScrapeState{}, false).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	if !strings.Contains(content, "Local draft only.") || !strings.Contains(content, `data-draft-key="new-soldier"`) {
		t.Fatalf("new entry form missing local draft status indicator")
	}
	if !strings.Contains(content, `<details class="card mb-5 rounded-3xl p-4">`) {
		t.Fatalf("new entry form should collapse the scrape panel by default")
	}
	if !strings.Contains(content, `name="confederate_home_status"`) || !strings.Contains(content, `name="confederate_home_name"`) {
		t.Fatalf("new entry form missing confederate home fields")
	}
	if !strings.Contains(content, `name="pension_state" value="None"`) {
		t.Fatalf("new entry form should default pension state to None")
	}
	if !strings.Contains(content, `list="rank-in-suggestions"`) || !strings.Contains(content, `list="record-type-suggestions"`) {
		t.Fatalf("new entry form missing datalist attributes")
	}
	if !strings.Contains(content, `name="prefix"`) || !strings.Contains(content, `list="prefix-suggestions"`) || !strings.Contains(content, `name="suffix"`) || !strings.Contains(content, `list="suffix-suggestions"`) {
		t.Fatalf("new entry form missing prefix/suffix datalist fields")
	}
	if !strings.Contains(content, `<datalist id="record-type-suggestions">`) {
		t.Fatalf("new entry form missing datalist markup")
	}
	if !strings.Contains(content, `<datalist id="prefix-suggestions">`) || !strings.Contains(content, `value="Capt."`) || !strings.Contains(content, `<datalist id="suffix-suggestions">`) || !strings.Contains(content, `value="Jr."`) {
		t.Fatalf("new entry form missing prefix/suffix suggestion markup")
	}
	if !strings.Contains(content, `list="confederate-home-name-suggestions"`) || !strings.Contains(content, `<datalist id="confederate-home-name-suggestions">`) {
		t.Fatalf("new entry form missing confederate home name datalist")
	}
	if !strings.Contains(content, `value="Co. A, 1st Texas Infantry"`) || !strings.Contains(content, `value="Oakwood Cemetery"`) {
		t.Fatalf("new entry form missing suggestion values")
	}
}

func TestNewEntryFormIncludesFindAGraveScrapeWarning(t *testing.T) {
	var buf bytes.Buffer
	err := EntryForm(viewmodel.Soldier{DisplayID: "STC38-00001"}, nil, viewmodel.SoldierFormSuggestions{}, viewmodel.FindAGraveScrapeState{
		Input:        "https://www.findagrave.com/memorial/11523031/elbert_dixon-anderson",
		SourceLabel:  "Parsed from pasted HTML",
		WarningLines: []string{"Verify all scraped data manually before saving."},
		Spouses: []viewmodel.ScrapedRelative{{
			Name:       "Harriet Clement Anderson",
			MemorialID: "11523035",
			URL:        "https://www.findagrave.com/memorial/11523035/harriet-anderson",
		}},
	}, false).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	if !strings.Contains(content, "Scrape Find a Grave") || !strings.Contains(content, `name="findagrave_source"`) {
		t.Fatalf("entry form missing Find a Grave scrape UI")
	}
	if !strings.Contains(content, "Parsed from pasted HTML") || !strings.Contains(content, "1 warning(s)") || !strings.Contains(content, "1 spouse record memorial(s)") {
		t.Fatalf("entry form missing compact scrape summary badges")
	}
	if !strings.Contains(content, "Review scraped data carefully before saving.") {
		t.Fatalf("entry form missing scrape review warning")
	}
	if !strings.Contains(content, "Harriet Clement Anderson") || !strings.Contains(content, "Memorial ID 11523035") {
		t.Fatalf("entry form missing scraped spouse preview")
	}
}

func TestShareViewIncludesMergeReviewPanel(t *testing.T) {
	var buf bytes.Buffer
	err := ShareView(viewmodel.GoogleStatus{}, []viewmodel.MergeReviewConflict{{
		ID:                42,
		ConflictType:      "soldier-update",
		Reason:            "Shared archive changed notes.",
		IncomingDisplayID: "TDM65-00042",
		LocalRecord: &viewmodel.Soldier{
			DisplayID: "TDM65-00042",
			FirstName: "Local",
			LastName:  "Version",
		},
		IncomingRecord: viewmodel.Soldier{
			DisplayID: "TDM65-00042",
			FirstName: "Shared",
			LastName:  "Version",
		},
	}}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	if !strings.Contains(content, "Merge Review") || !strings.Contains(content, "/merge-review/42/keep-shared") {
		t.Fatalf("share view missing merge review actions")
	}
}

func TestSearchResultsShowMatchSnippet(t *testing.T) {
	var buf bytes.Buffer
	err := SearchResults([]viewmodel.Soldier{{
		ID:                 7,
		DisplayID:          "PENSION-4242",
		FirstName:          "Nathan",
		LastName:           "Forrest",
		Unit:               "Forrest's Cavalry",
		BuriedIn:           "Memphis",
		Notes:              "Known for his cavalry leadership in Tennessee.",
		SearchMatchField:   "Unit",
		SearchMatchSnippet: "Forrest's Cavalry",
	}}, viewmodel.SoldierSearch{Mode: "basic", Query: "Forrest"}, 1, 1, 50).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	if !strings.Contains(content, "Matched on Unit") || !strings.Contains(content, "Forrest&#39;s Cavalry") {
		t.Fatalf("search results missing quick match snippet")
	}
}

func TestSearchPreviewContentShowsResearchOnlyDetails(t *testing.T) {
	var buf bytes.Buffer
	err := SearchPreviewContent(viewmodel.Soldier{
		ID:                 7,
		DisplayID:          "PENSION-4242",
		EntryType:          "widow",
		FirstName:          "Nathan",
		LastName:           "Forrest",
		Unit:               "Forrest's Cavalry",
		BuriedIn:           "Memphis",
		Notes:              "Known for his cavalry leadership in Tennessee.",
		SearchMatchField:   "Unit",
		SearchMatchSnippet: "Forrest's Cavalry",
		LinkedSoldierID:    8,
		SpouseDisplayID:    "PENSION-4243",
		SourceRecordCount:  3,
		ImageCount:         2,
		LastEditedBy:       "STC38",
		LastEditedAt:       "2026-05-16T18:05:00Z",
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"Local Archive Signals",
		"Research Context",
		"Family &amp; Links",
		"Source Records",
		"PENSION-4243",
		"Open Linked Soldier",
		"Compare Family Person Records",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("search preview missing %s", needle)
		}
	}
}

func TestSearchResultsShowsRecentAccessBanner(t *testing.T) {
	var buf bytes.Buffer
	err := SearchResults([]viewmodel.Soldier{{
		ID:        7,
		DisplayID: "PENSION-4242",
		FirstName: "Nathan",
		LastName:  "Forrest",
	}}, viewmodel.SoldierSearch{Mode: "basic", Recent: true}, 1, 1, 10).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"Recently Accessed",
		"Your ten most recently opened person records.",
		"Compare Selected",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("recent results missing %s", needle)
		}
	}
}

func TestShareViewIncludesKeepBothForDisplayIDCollision(t *testing.T) {
	var buf bytes.Buffer
	err := ShareView(viewmodel.GoogleStatus{}, []viewmodel.MergeReviewConflict{{
		ID:                99,
		ConflictType:      "display-id-collision",
		Reason:            "Shared record collides on display ID.",
		IncomingDisplayID: "TDM65-LOCAL-COLLIDE",
		LocalRecord: &viewmodel.Soldier{
			DisplayID: "TDM65-LOCAL-COLLIDE",
			FirstName: "Thomas",
			LastName:  "Lewis",
		},
		IncomingRecord: viewmodel.Soldier{
			DisplayID: "TDM65-LOCAL-COLLIDE",
			FirstName: "Andrew",
			LastName:  "Morris",
		},
	}}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	if !strings.Contains(content, "/merge-review/99/keep-both") || !strings.Contains(content, "Keep Both") {
		t.Fatalf("share view missing keep-both action")
	}
	if !strings.Contains(content, "/merge-review/99/keep-shared") {
		t.Fatalf("display-id collision should show keep-shared action")
	}
}
