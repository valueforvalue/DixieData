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
		CurrentPersonRecord: &viewmodel.Soldier{ID: 30, DisplayID: "COL-0030", FirstName: "Andrew", LastName: "Cole"},
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
		"Add Current Person Record",
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
		CurrentPersonRecord: &viewmodel.Soldier{ID: 30, DisplayID: "COL-0030", FirstName: "Andrew", LastName: "Cole"},
		PersonRecords: []viewmodel.Soldier{{
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
		"Current person record included",
		"Open Person Record",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("research collection detail missing %s", needle)
		}
	}
}

// TestResearchCollectionDetailViewRendersAddFormForMissingCurrent
// pins the "Add Current Person Record" button on the detail page
// (the /research-collections/{id} route) so users can add the
// soldier in their cookie context to a collection without having
// to back out to the hub. The button is gated on
// `CurrentPersonRecord != nil && !Collection.ContainsCurrent` —
// the same condition the hub uses. Before the fix the detail
// page had no Add affordance at all; users coming from top-nav
// (no ?from=) had no way to add anyone.
func TestResearchCollectionDetailViewRendersAddFormForMissingCurrent(t *testing.T) {
	var buf bytes.Buffer
	err := ResearchCollectionDetailView(viewmodel.ResearchCollectionDetail{
		Collection: viewmodel.ResearchCollection{
			ID:              7,
			Name:            "Orange County Cluster",
			ItemCount:       1,
			ContainsCurrent: false,
		},
		CurrentPersonRecord: &viewmodel.Soldier{ID: 30, DisplayID: "COL-0030", FirstName: "Andrew", LastName: "Cole"},
		PersonRecords: []viewmodel.Soldier{{
			ID: 99, DisplayID: "COL-0099", FirstName: "Other", LastName: "Person",
		}},
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	if !strings.Contains(content, "Add Current Person Record") {
		t.Fatalf("detail page missing Add Current Person Record form when current is not yet in the collection; got:\n%s", content)
	}
	if !strings.Contains(content, `action="/research-collections/7/add"`) {
		t.Fatalf("Add form action should target /research-collections/7/add; got:\n%s", content)
	}
	if !strings.Contains(content, `name="soldier_id" value="30"`) {
		t.Fatalf("Add form should carry the current soldier_id (30); got:\n%s", content)
	}
	if !strings.Contains(content, `data-dixie-submit="true"`) {
		t.Fatalf("Add form must opt into the JS dispatcher (data-dixie-submit); got:\n%s", content)
	}
}

// When the current person is already in the collection, the Add
// form must NOT render (the user is already a member; offering
// "Add" again is the path that triggered the redundant-click
// idempotency change).
func TestResearchCollectionDetailViewHidesAddFormWhenAlreadyInCollection(t *testing.T) {
	var buf bytes.Buffer
	err := ResearchCollectionDetailView(viewmodel.ResearchCollectionDetail{
		Collection: viewmodel.ResearchCollection{
			ID:              7,
			Name:            "Already In",
			ItemCount:       1,
			ContainsCurrent: true,
		},
		CurrentPersonRecord: &viewmodel.Soldier{ID: 30, DisplayID: "COL-0030", FirstName: "Andrew", LastName: "Cole"},
		PersonRecords: []viewmodel.Soldier{{
			ID: 30, DisplayID: "COL-0030", FirstName: "Andrew", LastName: "Cole",
		}},
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	if strings.Contains(content, "Add Current Person Record") {
		t.Fatalf("detail page should NOT render Add form when current is already in the collection; got:\n%s", content)
	}
	if !strings.Contains(content, "Current person record included") {
		t.Fatalf("expected 'Current person record included' pill; got:\n%s", content)
	}
}
