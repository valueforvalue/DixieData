package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

func TestInsightsViewRendersAnalyticsCards(t *testing.T) {
	var buf bytes.Buffer
	err := InsightsView(viewmodel.AnalyticsSnapshot{
		PersonRecordTypes: viewmodel.ArchiveCounts{
			SoldierCount:      12,
			SpouseRecordCount: 4,
		},
		CemeteryDensity:         []viewmodel.AnalyticsCount{{Label: "Oak Hill Cemetery", Count: 7}},
		ConfederateHomeStatus:   []viewmodel.AnalyticsCount{{Label: "Inmate", Count: 3}},
		ConfederateHomeNames:    []viewmodel.AnalyticsCount{{Label: "Texas Confederate Home", Count: 2}},
		PensionDistribution:     []viewmodel.AnalyticsCount{{Label: "Texas", Count: 5}},
		UnitRepresentation:      []viewmodel.AnalyticsCount{{Label: "1st Texas Infantry", Count: 4}},
		BirthDecadeDistribution: []viewmodel.AnalyticsCount{{Label: "1830s", Count: 6}},
		DeathDecadeDistribution: []viewmodel.AnalyticsCount{{Label: "1900s", Count: 2}},
		DuplicateAudit: viewmodel.DuplicateAuditSummary{
			OpenFindings:        3,
			ResolvedFindings:    1,
			LastRunAt:           "2026-05-16 15:00:00",
			SimilarityThreshold: 2,
		},
	}, viewmodel.ArchiveCounts{SoldierCount: 12, SpouseRecordCount: 4}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"Local Archive Insights",
		"/insights/report/pdf",
		"/insights/drilldown?scope=entry_type&amp;value=soldier",
		"/insights/drilldown?scope=buried_in&amp;value=Oak+Hill+Cemetery",
		"Top Cemeteries",
		"Oak Hill Cemetery",
		"Confederate Home Census",
		"Pension Distribution",
		"1st Texas Infantry",
		"Birth and Death Decades",
		"Advanced Duplicate Discovery",
		"Audit Now",
		"xl:flex-row xl:items-start xl:justify-between",
		"xl:items-end",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("insights view missing %s", needle)
		}
	}
}

func TestInsightsDrilldownViewRendersLinkedResults(t *testing.T) {
	var buf bytes.Buffer
	err := InsightsDrilldownView(
		"Burial Drilldown",
		"Records buried in Oak Hill Cemetery.",
		[]viewmodel.Soldier{{
			ID:                9,
			DisplayID:         "JCM87-00009",
			FirstName:         "Andrew",
			LastName:          "Cole",
			BuriedIn:          "Oak Hill Cemetery",
			SourceRecordCount: 2,
			ImageCount:        1,
			NeedsReview:       true,
			ReviewReason:      "Potential duplicate from import",
		}},
		viewmodel.SoldierSearch{Mode: "advanced", BuriedIn: "Oak Hill Cemetery"},
		1,
		1,
		50,
		"buried_in",
		"Oak Hill Cemetery",
	).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"Burial Drilldown",
		`data-history-back`,
		"Compare Selected",
		"Quick View",
		"JCM87-00009",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("insights drilldown missing %s", needle)
		}
	}
}
