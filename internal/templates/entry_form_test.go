package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

func TestEntryFormIncludesScratchPadLauncher(t *testing.T) {
	var buf bytes.Buffer
	err := EntryForm(models.Soldier{DisplayID: "DXD-00001"}, nil, false).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	if !strings.Contains(content, "Open Scratch Pad") {
		t.Fatalf("entry form missing scratch pad button")
	}
	if !strings.Contains(content, `name="birth_date"`) || !strings.Contains(content, `name="death_date"`) {
		t.Fatalf("entry form missing canonical date fields")
	}
	if !strings.Contains(content, `data-ui-id="panel.soldier.form.scratchpad"`) {
		t.Fatalf("entry form missing scratch pad surface id")
	}
}

func TestEntryFormKeepsDisplayIDReadonlyOnEdit(t *testing.T) {
	var buf bytes.Buffer
	err := EntryForm(models.Soldier{DisplayID: "DXD-00001"}, nil, true).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	if !strings.Contains(content, `name="display_id"`) || !strings.Contains(content, `readonly`) {
		t.Fatalf("entry form should render display_id as readonly on edit")
	}
}

func TestEntryFormIncludesSpouseFields(t *testing.T) {
	var buf bytes.Buffer
	err := EntryForm(models.Soldier{EntryType: "wife", SpouseSoldierID: 7}, []models.Soldier{
		{ID: 7, DisplayID: "TDM65-DXD-00007", FirstName: "John", LastName: "Smith"},
	}, false).Render(context.Background(), &buf)
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

func TestExportViewIncludesSharedDatabaseImport(t *testing.T) {
	var buf bytes.Buffer
	err := ExportView(models.GoogleStatus{}, nil).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	if !strings.Contains(content, "/import/backup") || !strings.Contains(content, "Load Backup") {
		t.Fatalf("export view missing backup import action")
	}
	if !strings.Contains(content, "/export/shared-archive") || !strings.Contains(content, "Export Shared Archive") {
		t.Fatalf("export view missing shared archive export action")
	}
	if !strings.Contains(content, "/import/shared-archive") || !strings.Contains(content, "Import Shared Archive") {
		t.Fatalf("export view missing shared backup import action")
	}
	if !strings.Contains(content, ".ddbak") || !strings.Contains(content, ".ddshare") {
		t.Fatalf("export view missing custom archive extension copy")
	}
}

func TestInitialSetupViewIncludesIdentityFields(t *testing.T) {
	var buf bytes.Buffer
	err := InitialSetupView(models.InitialSetupForm{
		FirstName:     "Samuel",
		MiddleName:    "Thomas",
		LastName:      "Carter",
		BirthYear:     "1838",
		PrefixPreview: "STC1838",
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	if !strings.Contains(content, `action="/setup"`) || !strings.Contains(content, `name="birth_year"`) {
		t.Fatalf("initial setup view missing setup form fields")
	}
	if !strings.Contains(content, "STC1838") {
		t.Fatalf("initial setup view missing prefix preview")
	}
}

func TestExportViewIncludesMergeReviewPanel(t *testing.T) {
	var buf bytes.Buffer
	err := ExportView(models.GoogleStatus{}, []models.MergeReviewConflict{{
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
		t.Fatalf("export view missing merge review actions")
	}
}

func TestExportViewIncludesKeepBothForDisplayIDCollision(t *testing.T) {
	var buf bytes.Buffer
	err := ExportView(models.GoogleStatus{}, []models.MergeReviewConflict{{
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
		t.Fatalf("export view missing keep-both action")
	}
	if strings.Contains(content, "/merge-review/99/use-shared") {
		t.Fatalf("display-id collision should not show use-shared action")
	}
}
