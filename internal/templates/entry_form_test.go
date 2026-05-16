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
