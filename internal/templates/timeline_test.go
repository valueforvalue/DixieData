package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

func TestServiceTimelineViewRendersEventsAndUndatedSources(t *testing.T) {
	var buf bytes.Buffer
	err := ServiceTimelineView(viewmodel.ServiceTimeline{
		SubjectPersonRecord: viewmodel.Soldier{
			ID:        12,
			DisplayID: "TLM-0012",
			FirstName: "Andrew",
			LastName:  "Cole",
			Unit:      "1st Texas Infantry",
		},
		StartLabel:         "May 12, 1838",
		EndLabel:           "November 3, 1904",
		ExactEventCount:    2,
		InferredEventCount: 1,
		TimelineEvents: []viewmodel.ServiceTimelineEvent{
			{
				Title:           "Birth",
				DateLabel:       "May 12, 1838",
				Category:        "life",
				ConfidenceLabel: "Exact",
				SourceLabel:     "Profile",
			},
			{
				Title:           "Pension",
				DateLabel:       "1901",
				Description:     "Filed in 1901 after moving back to Texas. See [[TLM-0012]].",
				Category:        "pension",
				ConfidenceLabel: "Inferred",
				SourceLabel:     "Pension · APP-3",
				Approximate:     true,
			},
		},
		UndatedSourceRecords: []viewmodel.Record{{
			SourceRecordType: "Letter",
			AppID:            "APP-4",
			Details:          "Family correspondence with [[TLM-0012]] and no year listed.",
		}},
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"Auto-Built Service Timeline",
		`data-history-back`,
		"TLM-0012",
		"Pension Trail",
		"Undated Source Records",
		"APP-4",
		`href="/soldiers/display/TLM-0012"`,
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("service timeline view missing %s", needle)
		}
	}
}
