package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/services"
)

func TestResearchPackViewRendersLocationClusters(t *testing.T) {
	var buf bytes.Buffer
	err := ResearchPackView(services.ResearchPack{
		Central:     models.Soldier{ID: 22, DisplayID: "PACK-0022", FirstName: "Andrew", LastName: "Cole"},
		Scope:       "state",
		PlaceLabel:  "Texas",
		Description: "Records tied to Texas through pension filing or birth-place context.",
		Related: []models.Soldier{{
			ID:           23,
			DisplayID:    "PACK-0023",
			FirstName:    "Thomas",
			LastName:     "Reed",
			Unit:         "1st Texas Infantry",
			PensionState: "Texas",
			BuriedIn:     "Oak Hill Cemetery",
		}},
		TopUnits:        []services.AnalyticsCount{{Label: "1st Texas Infantry", Count: 3}},
		TopCemeteries:   []services.AnalyticsCount{{Label: "Oak Hill Cemetery", Count: 2}},
		OpenReviewCount: 1,
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"State Research Pack",
		`data-history-back`,
		"Texas",
		"1st Texas Infantry",
		"Open Record",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("research pack view missing %s", needle)
		}
	}
}
