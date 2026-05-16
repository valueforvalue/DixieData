package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/services"
)

func TestReviewQueueViewShowsFlaggedRecords(t *testing.T) {
	var buf bytes.Buffer
	err := ReviewQueueView([]services.ReviewQueueEntry{{
		Soldier: models.Soldier{
			ID:           12,
			DisplayID:    "JCM87-00012",
			FirstName:    "Andrew",
			LastName:     "Morris",
			NeedsReview:  true,
			ReviewReason: "Potential duplicate from JCM87 import",
		},
		DuplicateFindings: []services.DuplicateAuditFindingSummary{{
			ID:             9,
			OtherSoldierID: 18,
			OtherDisplayID: "JCM87-00018",
		}},
	}}, 1, 1, 50).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"Review Queue",
		"Potential duplicate from JCM87 import",
		"/soldiers/12",
		"/soldiers/12/review/resolve?context=queue",
		"/review-queue/compare/9",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("review queue missing %s", needle)
		}
	}
}

func TestReviewQueueCompareViewShowsSideBySideFields(t *testing.T) {
	var buf bytes.Buffer
	err := ReviewQueueCompareView(services.DuplicateAuditComparison{
		FindingID:    4,
		FindingType:  "fuzzy-first-name",
		Reason:       `Duplicate Audit: Fuzzy match: "John" and "Jon" share the same last name and birth year.`,
		LeftSoldier:  models.Soldier{DisplayID: "JCM87-00004", FirstName: "John", LastName: "Kerns", BirthDate: "01/01/1840", Unit: "4th OK Inf."},
		RightSoldier: models.Soldier{DisplayID: "JCM87-00008", FirstName: "Jon", LastName: "Kerns", BirthDate: "01/01/1840", Unit: "4th OK Inf."},
		Fields: []services.DuplicateAuditComparisonField{
			{Label: "First Name", LeftValue: "John", RightValue: "Jon", Highlighted: true},
			{Label: "Birth Year", LeftValue: "1840", RightValue: "1840", Highlighted: true},
		},
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"Duplicate Comparison",
		"Mark Match Resolved",
		"First Name",
		"John",
		"Jon",
		"/review-queue/compare/4/resolve",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("comparison view missing %s", needle)
		}
	}
}
