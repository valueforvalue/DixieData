package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/services"
)

func TestUnitCamaraderieViewRendersConnections(t *testing.T) {
	var buf bytes.Buffer
	err := UnitCamaraderieView(services.UnitCamaraderieGraph{
		Central: models.Soldier{
			ID:        9,
			DisplayID: "CAM-0009",
			FirstName: "Andrew",
			LastName:  "Cole",
			Unit:      "Co. A, 1st Texas Infantry",
		},
		UnitLabel:     "Co. A, 1st Texas Infantry",
		RegimentLabel: "1st Texas Infantry",
		CompanyLabel:  "Company A",
		SameUnit: []services.UnitCamaraderieConnection{{
			Soldier: models.Soldier{
				ID:        10,
				DisplayID: "CAM-0010",
				FirstName: "Thomas",
				LastName:  "Reed",
				Unit:      "Co. A, 1st Texas Infantry",
			},
			Relation:     "Same recorded unit",
			StrengthText: "Strong",
		}},
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"Unit Camaraderie Graph",
		`data-history-back`,
		"CAM-0009",
		"CAM-0010",
		"Same Recorded Unit",
		"Compare Records",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("unit camaraderie view missing %s", needle)
		}
	}
}
