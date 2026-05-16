package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

func TestSoldierListShowsExpandedAdvancedSearchFields(t *testing.T) {
	var buf bytes.Buffer
	err := SoldierList(nil, 1, 0, "", models.SoldierFormSuggestions{
		RankIn:              []string{"Private"},
		RankOut:             []string{"Captain"},
		Unit:                []string{"1st Texas Infantry"},
		PensionState:        []string{"Texas"},
		BuriedIn:            []string{"Oakwood Cemetery"},
		ConfederateHomeName: []string{"Texas Confederate Home"},
		RecordType:          []string{"Pension Ledger"},
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		`name="maiden_name"`,
		`name="rank_in"`,
		`name="rank_out"`,
		`name="record_type"`,
		`name="confederate_home_status"`,
		`name="confederate_home_name"`,
		`name="birth_year"`,
		`name="birth_year_to"`,
		`name="death_year_to"`,
		`name="review_status"`,
		`list="advanced-record-type-suggestions"`,
		`list="advanced-rank-in-suggestions"`,
		`list="advanced-rank-out-suggestions"`,
		`list="advanced-pension-state-suggestions"`,
		`list="advanced-confederate-home-name-suggestions"`,
		`<datalist id="advanced-record-type-suggestions">`,
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("advanced search form missing %s", needle)
		}
	}
}

func TestSoldierDetailShowsMetadataHistoryPanel(t *testing.T) {
	var buf bytes.Buffer
	err := SoldierDetail(models.Soldier{
		ID:               42,
		DisplayID:        "STC38-00001",
		FirstName:        "John",
		LastName:         "Taylor",
		AddedBy:          "STC38",
		LastEditedBy:     "MDC42",
		LastEditedAt:     "2026-05-16T18:05:00Z",
		LastEditedFields: "Unit changed from \"4th OK Inf.\" to \"1st OK Cav.\".\nRecords updated.",
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"Record Metadata &amp; History",
		"Created By",
		"Last Updated By",
		"Last Update Time",
		"STC38",
		"MDC42",
		"May 16, 2026",
		"Unit changed from &#34;4th OK Inf.&#34; to &#34;1st OK Cav.&#34;.",
		"Records updated.",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("metadata/history panel missing %s", needle)
		}
	}
}

func TestSoldierDetailShowsPrimaryImageControls(t *testing.T) {
	var buf bytes.Buffer
	err := SoldierDetail(models.Soldier{
		ID:        42,
		DisplayID: "STC38-00001",
		FirstName: "John",
		LastName:  "Taylor",
		Images: []models.Image{
			{ID: 7, FileName: "front.png", FilePath: `images\front.png`, Caption: "Front", IsPrimary: true},
			{ID: 8, FileName: "side.png", FilePath: `images\side.png`, Caption: "Side"},
		},
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"Primary Image",
		"/soldiers/42/images/primary/8",
		"Set as Primary",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("primary image controls missing %s", needle)
		}
	}
}

func TestSoldierCardShowsReviewBadge(t *testing.T) {
	var buf bytes.Buffer
	err := SoldierCard(models.Soldier{
		ID:           7,
		DisplayID:    "JCM87-00007",
		FirstName:    "Flagged",
		LastName:     "Record",
		NeedsReview:  true,
		ReviewReason: "Potential duplicate from JCM87 import",
	}, false).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"Needs Review",
		"Potential duplicate from JCM87 import",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("soldier card missing %s", needle)
		}
	}
}

func TestSoldierDetailShowsResolveReviewAction(t *testing.T) {
	var buf bytes.Buffer
	err := SoldierDetail(models.Soldier{
		ID:           42,
		DisplayID:    "JCM87-00042",
		FirstName:    "Review",
		LastName:     "Needed",
		NeedsReview:  true,
		ReviewReason: "Potential duplicate from import",
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"Review Status",
		"Review Reason",
		"Mark as Resolved",
		"/soldiers/42/review/resolve",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("soldier detail missing %s", needle)
		}
	}
}

func TestSoldierDetailShowsManualReviewFlagAction(t *testing.T) {
	var buf bytes.Buffer
	err := SoldierDetail(models.Soldier{
		ID:        41,
		DisplayID: "JCM87-00041",
		FirstName: "Manual",
		LastName:  "Review",
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"Send to Review Queue",
		"Review Note",
		"/soldiers/41/review/flag",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("soldier detail missing %s", needle)
		}
	}
}

func TestSoldierDetailConsolidatesRelationshipDisplay(t *testing.T) {
	var buf bytes.Buffer
	err := SoldierDetail(models.Soldier{
		ID:              12,
		DisplayID:       "JCM87-00012",
		EntryType:       "widow",
		FirstName:       "Sarah",
		LastName:        "Cole",
		SpouseName:      "Thomas Cole",
		SpouseDisplayID: "JCM87-00011",
		SpouseSoldierID: 11,
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	if strings.Count(content, "Married To") != 1 {
		t.Fatalf("expected a single Married To label, got %d", strings.Count(content, "Married To"))
	}
	for _, needle := range []string{
		"Family &amp; Relationships",
		"Thomas Cole (JCM87-00011)",
		"View Husband",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("soldier detail missing %s", needle)
		}
	}
}

func TestSoldierDetailSecondaryBackActionUsesSmartBack(t *testing.T) {
	var buf bytes.Buffer
	err := SoldierDetail(models.Soldier{
		ID:            12,
		DisplayID:     "JCM87-00012",
		FirstName:     "Sarah",
		LastName:      "Cole",
		BackLinkURL:   "/review-queue",
		BackLinkLabel: "Back to Review Queue",
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		`data-history-back`,
		`data-fallback-href="/review-queue"`,
		"Back to Review Queue",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("soldier detail secondary back action missing %s", needle)
		}
	}
}
