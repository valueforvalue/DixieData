package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

func TestResearchCollectionsHubViewRendersCurrentRecordContext(t *testing.T) {
	var buf bytes.Buffer
	err := ResearchCollectionsHubView(viewmodel.ResearchCollectionHub{
		Current: &viewmodel.Soldier{ID: 30, DisplayID: "COL-0030", FirstName: "Andrew", LastName: "Cole"},
		Collections: []viewmodel.ResearchCollection{{
			ID:              7,
			Name:            "Orange County Cluster",
			Description:     "County-focused follow-up list.",
			ItemCount:       2,
			ContainsCurrent: false,
		}},
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"Named Research Collections",
		"/research-collections/7?from=30",
		"Add Current Record",
		`data-history-back`,
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("research collections hub missing %s", needle)
		}
	}
}

func TestResearchCollectionDetailViewRendersMembers(t *testing.T) {
	var buf bytes.Buffer
	err := ResearchCollectionDetailView(viewmodel.ResearchCollectionDetail{
		Collection: viewmodel.ResearchCollection{
			ID:              7,
			Name:            "Orange County Cluster",
			Description:     "County-focused follow-up list.",
			ItemCount:       1,
			ContainsCurrent: true,
		},
		Current: &viewmodel.Soldier{ID: 30, DisplayID: "COL-0030", FirstName: "Andrew", LastName: "Cole"},
		Members: []viewmodel.Soldier{{
			ID:        30,
			DisplayID: "COL-0030",
			FirstName: "Andrew",
			LastName:  "Cole",
		}},
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"Research Collection",
		"Orange County Cluster",
		"Current record included",
		"Open Record",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("research collection detail missing %s", needle)
		}
	}
}
