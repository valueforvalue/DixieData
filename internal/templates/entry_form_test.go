package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

func TestEntryFormOmitsInlineScratchPadLauncher(t *testing.T) {
	var buf bytes.Buffer
	err := EntryForm(models.Soldier{DisplayID: "DXD-00001"}, nil, models.SoldierFormSuggestions{}, models.FindAGraveScrapeState{}, false).Render(context.Background(), &buf)
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
	err := EntryForm(models.Soldier{DisplayID: "DXD-00001"}, nil, models.SoldierFormSuggestions{}, models.FindAGraveScrapeState{}, true).Render(context.Background(), &buf)
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
	err := EntryForm(models.Soldier{EntryType: "wife", SpouseSoldierID: 7}, []models.Soldier{
		{ID: 7, DisplayID: "TDM65-DXD-00007", FirstName: "John", LastName: "Smith"},
	}, models.SoldierFormSuggestions{}, models.FindAGraveScrapeState{}, false).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	if !strings.Contains(content, `name="entry_type"`) || !strings.Contains(content, `data-entry-type-select`) {
		t.Fatalf("entry form missing entry type selector")
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
	err := ShareView(models.GoogleStatus{}, nil).Render(context.Background(), &buf)
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
	if !strings.Contains(content, "/import/shared-archive") || !strings.Contains(content, "Import Shared Archive (.ddshare)") {
		t.Fatalf("share view missing shared archive import action")
	}
	if !strings.Contains(content, "/export/bug-report") || !strings.Contains(content, "Support & Diagnostics") {
		t.Fatalf("share view missing diagnostics section")
	}
	if !strings.Contains(content, ".ddbak") || !strings.Contains(content, ".ddshare") {
		t.Fatalf("share view missing custom archive extension copy")
	}
}

func TestInitialSetupViewIncludesIdentityFields(t *testing.T) {
	var buf bytes.Buffer
	err := InitialSetupView(models.InitialSetupForm{
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
	err := EntryForm(models.Soldier{DisplayID: "STC38-00001", PensionState: "None", ConfederateHomeStatus: "None"}, nil, models.SoldierFormSuggestions{
		RankIn:              []string{"Private", "Sergeant"},
		RankOut:             []string{"Corporal", "Sergeant"},
		Unit:                []string{"Co. A, 1st Texas Infantry"},
		PensionState:        []string{"None", "Texas"},
		BuriedIn:            []string{"Oakwood Cemetery"},
		ConfederateHomeName: []string{},
		RecordType:          []string{"Pension"},
	}, models.FindAGraveScrapeState{}, false).Render(context.Background(), &buf)
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
	if !strings.Contains(content, `<datalist id="record-type-suggestions">`) {
		t.Fatalf("new entry form missing datalist markup")
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
	err := EntryForm(models.Soldier{DisplayID: "STC38-00001"}, nil, models.SoldierFormSuggestions{}, models.FindAGraveScrapeState{
		Input:        "https://www.findagrave.com/memorial/11523031/elbert_dixon-anderson",
		SourceLabel:  "Parsed from pasted HTML",
		WarningLines: []string{"Verify all scraped data manually before saving."},
		Spouses: []models.ScrapedRelative{{
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
	if !strings.Contains(content, "Parsed from pasted HTML") || !strings.Contains(content, "1 warning(s)") || !strings.Contains(content, "1 spouse memorial(s)") {
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
	err := ShareView(models.GoogleStatus{}, []models.MergeReviewConflict{{
		ID:              42,
		ConflictType:    "soldier-update",
		Reason:          "Shared archive changed notes.",
		SourceDisplayID: "TDM65-00042",
		LocalSoldier: &models.Soldier{
			DisplayID: "TDM65-00042",
			FirstName: "Local",
			LastName:  "Version",
		},
		SourceSoldier: models.Soldier{
			DisplayID: "TDM65-00042",
			FirstName: "Shared",
			LastName:  "Version",
		},
	}}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	if !strings.Contains(content, "Shared Merge Review") || !strings.Contains(content, "/merge-review/42/use-shared") {
		t.Fatalf("share view missing merge review actions")
	}
}

func TestSearchResultsShowMatchSnippet(t *testing.T) {
	var buf bytes.Buffer
	err := SearchResults([]models.Soldier{{
		ID:                 7,
		DisplayID:          "PENSION-4242",
		FirstName:          "Nathan",
		LastName:           "Forrest",
		Unit:               "Forrest's Cavalry",
		BuriedIn:           "Memphis",
		Notes:              "Known for his cavalry leadership in Tennessee.",
		SearchMatchField:   "Unit",
		SearchMatchSnippet: "Forrest's Cavalry",
	}}, models.SoldierSearch{Mode: "basic", Query: "Forrest"}, 1, 1, 50).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	if !strings.Contains(content, "Matched on Unit") || !strings.Contains(content, "Forrest&#39;s Cavalry") {
		t.Fatalf("search results missing quick match snippet")
	}
}

func TestShareViewIncludesKeepBothForDisplayIDCollision(t *testing.T) {
	var buf bytes.Buffer
	err := ShareView(models.GoogleStatus{}, []models.MergeReviewConflict{{
		ID:              99,
		ConflictType:    "display-id-collision",
		Reason:          "Shared record collides on display ID.",
		SourceDisplayID: "TDM65-LOCAL-COLLIDE",
		LocalSoldier: &models.Soldier{
			DisplayID: "TDM65-LOCAL-COLLIDE",
			FirstName: "Thomas",
			LastName:  "Lewis",
		},
		SourceSoldier: models.Soldier{
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
	if strings.Contains(content, "/merge-review/99/use-shared") {
		t.Fatalf("display-id collision should not show use-shared action")
	}
}
