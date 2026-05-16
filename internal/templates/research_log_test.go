package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/services"
)

func TestResearchLogViewRendersTasksAndSuggestions(t *testing.T) {
	var buf bytes.Buffer
	err := ResearchLogView(services.ResearchLog{
		Central: models.Soldier{
			ID:        14,
			DisplayID: "RLG-0014",
			FirstName: "Andrew",
			LastName:  "Cole",
		},
		OpenCount:     1,
		ResolvedCount: 1,
		Suggestions: []services.ResearchTaskSuggestion{{
			Title:        "Locate pension or application file",
			Notes:        "No pension ID or application ID is attached yet.",
			EvidenceType: "pension",
		}},
		Tasks: []services.ResearchTask{{
			ID:           8,
			Title:        "Locate pension file",
			Notes:        "Check state archive holdings.",
			EvidenceType: "pension",
			Status:       "open",
			CreatedAt:    "2026-05-16 18:12:00",
		}},
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"Research Log &amp; Missing Evidence",
		`data-history-back`,
		"Missing-Evidence Suggestions",
		"Locate pension file",
		"Mark Resolved",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("research log view missing %s", needle)
		}
	}
}
