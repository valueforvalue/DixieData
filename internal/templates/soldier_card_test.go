package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

func TestSoldierListShowsExpandedAdvancedSearchFields(t *testing.T) {
	var buf bytes.Buffer
	err := SoldierList(nil, 1, 0, "", models.SoldierFormSuggestions{
		RankIn:              []string{"Private"},
		RankOut:             []string{"Captain"},
		Unit:                []string{"1st Texas Infantry"},
		PensionState:        []string{"Texas"},
		BuriedIn:            []string{"Oakwood Cemetery"},
		ConfederateHomeName: []string{"Texas Confederate Home"},
		RecordType:          []string{"Pension Ledger"},
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		`name="maiden_name"`,
		`name="rank_in"`,
		`name="rank_out"`,
		`name="record_type"`,
		`name="confederate_home_status"`,
		`name="confederate_home_name"`,
		`name="birth_year"`,
		`name="birth_year_to"`,
		`name="death_year_to"`,
		`list="advanced-record-type-suggestions"`,
		`list="advanced-rank-in-suggestions"`,
		`list="advanced-rank-out-suggestions"`,
		`list="advanced-pension-state-suggestions"`,
		`list="advanced-confederate-home-name-suggestions"`,
		`<datalist id="advanced-record-type-suggestions">`,
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("advanced search form missing %s", needle)
		}
	}
}

func TestSoldierDetailShowsMetadataHistoryPanel(t *testing.T) {
	var buf bytes.Buffer
	err := SoldierDetail(models.Soldier{
		ID:               42,
		DisplayID:        "STC38-00001",
		FirstName:        "John",
		LastName:         "Taylor",
		AddedBy:          "STC38",
		LastEditedBy:     "MDC42",
		LastEditedAt:     "2026-05-16T18:05:00Z",
		LastEditedFields: "Unit changed from \"4th OK Inf.\" to \"1st OK Cav.\".\nRecords updated.",
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"Record Metadata &amp; History",
		"Created By",
		"Last Updated By",
		"Last Update Time",
		"STC38",
		"MDC42",
		"May 16, 2026",
		"Unit changed from &#34;4th OK Inf.&#34; to &#34;1st OK Cav.&#34;.",
		"Records updated.",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("metadata/history panel missing %s", needle)
		}
	}
}
