package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

func TestReviewQueueViewShowsFlaggedRecords(t *testing.T) {
	var buf bytes.Buffer
	err := ReviewQueueView([]viewmodel.ReviewQueueEntry{{
		PersonRecord: viewmodel.Soldier{
			ID:           12,
			DisplayID:    "JCM87-00012",
			FirstName:    "Andrew",
			LastName:     "Morris",
			EntryType:    "wife",
			MaidenName:   "Carter",
			NeedsReview:  true,
			ReviewReason: "Potential duplicate from JCM87 import",
		},
		DuplicateFindings: []viewmodel.DuplicateAuditFindingSummary{{
			ID:                  9,
			OtherPersonRecordID: 18,
			OtherDisplayID:      "JCM87-00018",
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
		"<em>Carter</em>",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("review queue missing %s", needle)
		}
	}
}

func TestReviewQueueCompareViewShowsSideBySideFields(t *testing.T) {
	var buf bytes.Buffer
	err := ReviewQueueCompareView(viewmodel.DuplicateAuditComparison{
		FindingID:         4,
		FindingType:       "fuzzy-first-name",
		Reason:            `Duplicate Audit: Fuzzy match: "John" and "Jon" share the same last name and birth year.`,
		LeftPersonRecord:  viewmodel.Soldier{DisplayID: "JCM87-00004", FirstName: "John", LastName: "Kerns", BirthDate: "01/01/1840", Unit: "4th OK Inf."},
		RightPersonRecord: viewmodel.Soldier{DisplayID: "JCM87-00008", FirstName: "Jon", LastName: "Kerns", BirthDate: "01/01/1840", Unit: "4th OK Inf."},
		Fields: []viewmodel.DuplicateAuditComparisonField{
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
		"differing field(s)",
		"matching field(s)",
		"Differences to Review First",
		"Open Left Person Record",
		"Open Right Person Record",
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

func TestReviewQueueCompareViewSupportsManualComparison(t *testing.T) {
	var buf bytes.Buffer
	err := ReviewQueueCompareView(viewmodel.DuplicateAuditComparison{
		PageTitle:        "Person Record Comparison",
		BackHref:         "/soldiers",
		BackLabel:        "Back",
		Reason:           "Manual side-by-side comparison of two selected person records.",
		Status:           "manual",
		LeftPersonRecord: viewmodel.Soldier{DisplayID: "JCM87-00004", FirstName: "John", LastName: "Kerns"},
		RightPersonRecord: viewmodel.Soldier{
			DisplayID: "JCM87-00008",
			FirstName: "Jon",
			LastName:  "Kerns",
		},
		Fields: []viewmodel.DuplicateAuditComparisonField{
			{Label: "First Name", LeftValue: "John", RightValue: "Jon", Highlighted: true},
		},
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"Person Record Comparison",
		"data-history-back",
		"Back",
		"Manual side-by-side comparison of two selected person records.",
		"Open Left Person Record",
		"Open Right Person Record",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("manual comparison view missing %s", needle)
		}
	}
	if strings.Contains(content, "Mark Match Resolved") {
		t.Fatalf("manual comparison should not show resolve action: %s", content)
	}
}
