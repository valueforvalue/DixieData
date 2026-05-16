package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/services"
)

func TestInsightsViewRendersAnalyticsCards(t *testing.T) {
	var buf bytes.Buffer
	err := InsightsView(services.AnalyticsSnapshot{
		RecordTypes: models.ArchiveCounts{
			TotalSoldiers:    12,
			TotalWivesWidows: 4,
		},
		CemeteryDensity:         []services.AnalyticsCount{{Label: "Oak Hill Cemetery", Count: 7}},
		ConfederateHomeStatus:   []services.AnalyticsCount{{Label: "Inmate", Count: 3}},
		ConfederateHomeNames:    []services.AnalyticsCount{{Label: "Texas Confederate Home", Count: 2}},
		PensionDistribution:     []services.AnalyticsCount{{Label: "Texas", Count: 5}},
		UnitRepresentation:      []services.AnalyticsCount{{Label: "1st Texas Infantry", Count: 4}},
		BirthDecadeDistribution: []services.AnalyticsCount{{Label: "1830s", Count: 6}},
		DeathDecadeDistribution: []services.AnalyticsCount{{Label: "1900s", Count: 2}},
		DuplicateAudit: services.DuplicateAuditSummary{
			OpenFindings:        3,
			ResolvedFindings:    1,
			LastRunAt:           "2026-05-16 15:00:00",
			SimilarityThreshold: 2,
		},
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"Archive Insights",
		"/insights/report/pdf",
		"Top Cemeteries",
		"Oak Hill Cemetery",
		"Confederate Home Census",
		"Pension Distribution",
		"1st Texas Infantry",
		"Birth and Death Decades",
		"Advanced Duplicate Discovery",
		"Audit Now",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("insights view missing %s", needle)
		}
	}
}
