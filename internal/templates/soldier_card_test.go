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
