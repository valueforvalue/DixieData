package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

func TestSoldierListShowsExpandedAdvancedSearchFields(t *testing.T) {
	var buf bytes.Buffer
	err := SoldierList(nil, 1, 0, "", viewmodel.SoldierFormSuggestions{
		RankIn:           []string{"Private"},
		RankOut:          []string{"Captain"},
		Unit:             []string{"1st Texas Infantry"},
		PensionState:     []string{"Texas"},
		BuriedIn:         []string{"Oakwood Cemetery"},
		ConfederateHome:  []string{"Texas Confederate Home"},
		SourceRecordType: []string{"Pension Ledger"},
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
	err := SoldierDetail(viewmodel.Soldier{
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
	err := SoldierDetail(viewmodel.Soldier{
		ID:        42,
		DisplayID: "STC38-00001",
		FirstName: "John",
		LastName:  "Taylor",
		Images: []viewmodel.Image{
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
	err := SoldierCard(viewmodel.Soldier{
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

func TestSoldierDetailItalicizesMaidenNameAndLinksInternalReferences(t *testing.T) {
	var buf bytes.Buffer
	err := SoldierDetail(viewmodel.Soldier{
		ID:         42,
		DisplayID:  "JCM87-00042",
		EntryType:  "wife",
		FirstName:  "Sarah",
		LastName:   "Cole",
		MaidenName: "Martin",
		Notes:      "See [[JCM87-00011]] and https://example.com/report.",
		SourceRecords: []viewmodel.Record{{
			SourceRecordType: "Pension",
			Details:          "Linked to [[JCM87-00011]].",
		}},
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"<em>Martin</em>",
		`href="/soldiers/display/JCM87-00011"`,
		`data-open-external="true"`,
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("soldier detail missing %s", needle)
		}
	}
}

func TestSoldierDetailShowsResolveReviewAction(t *testing.T) {
	var buf bytes.Buffer
	err := SoldierDetail(viewmodel.Soldier{
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
	err := SoldierDetail(viewmodel.Soldier{
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
	err := SoldierDetail(viewmodel.Soldier{
		ID:              12,
		DisplayID:       "JCM87-00012",
		EntryType:       "widow",
		FirstName:       "Sarah",
		LastName:        "Cole",
		SpouseName:      "Thomas Cole",
		SpouseDisplayID: "JCM87-00011",
		LinkedSoldierID: 11,
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
	err := SoldierDetail(viewmodel.Soldier{
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

func TestSoldierDetailShowsUnitCamaraderieAction(t *testing.T) {
	var buf bytes.Buffer
	err := SoldierDetail(viewmodel.Soldier{
		ID:        18,
		DisplayID: "JCM87-00018",
		FirstName: "Andrew",
		LastName:  "Cole",
		Unit:      "Co. A, 1st Texas Infantry",
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"Unit Camaraderie",
		"/soldiers/18/camaraderie",
		"Open Unit Graph",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("soldier detail missing %s", needle)
		}
	}
}

func TestSoldierDetailShowsServiceTimelineAction(t *testing.T) {
	var buf bytes.Buffer
	err := SoldierDetail(viewmodel.Soldier{
		ID:        19,
		DisplayID: "JCM87-00019",
		FirstName: "Andrew",
		LastName:  "Cole",
		BirthDate: "05/12/1838",
		SourceRecords: []viewmodel.Record{{
			SourceRecordType: "Muster Roll",
			AppID:            "APP-19",
			Details:          "Enlisted on 03/11/1862 at Austin.",
		}},
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"Service Timeline",
		"/soldiers/19/timeline",
		"Open Timeline",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("soldier detail missing %s", needle)
		}
	}
}

func TestSoldierDetailShowsResearchLogAction(t *testing.T) {
	var buf bytes.Buffer
	err := SoldierDetail(viewmodel.Soldier{
		ID:        20,
		DisplayID: "JCM87-00020",
		FirstName: "Andrew",
		LastName:  "Cole",
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"Research Log",
		"/soldiers/20/research-log",
		"Open Research Log",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("soldier detail missing %s", needle)
		}
	}
}

func TestSoldierDetailShowsConflictLedgerAction(t *testing.T) {
	var buf bytes.Buffer
	err := SoldierDetail(viewmodel.Soldier{
		ID:        21,
		DisplayID: "JCM87-00021",
		FirstName: "Andrew",
		LastName:  "Cole",
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"Merge Review Ledger",
		"/soldiers/21/conflict-ledger",
		"Open Merge Review Ledger",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("soldier detail missing %s", needle)
		}
	}
}

func TestSoldierDetailShowsResearchPackActions(t *testing.T) {
	var buf bytes.Buffer
	err := SoldierDetail(viewmodel.Soldier{
		ID:           22,
		DisplayID:    "JCM87-00022",
		FirstName:    "Andrew",
		LastName:     "Cole",
		PensionState: "Texas",
		BirthInfo:    "Born 1838 in Orange County, Texas.",
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"Research Packs",
		"/soldiers/22/research-pack/state",
		"/soldiers/22/research-pack/county",
		"Open State Pack",
		"Open County Pack",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("soldier detail missing %s", needle)
		}
	}
}

func TestSoldierDetailShowsResearchCollectionsAction(t *testing.T) {
	var buf bytes.Buffer
	err := SoldierDetail(viewmodel.Soldier{
		ID:        23,
		DisplayID: "JCM87-00023",
		FirstName: "Andrew",
		LastName:  "Cole",
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"Research Collections",
		"/research-collections?from=23",
		"Manage Collections",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("soldier detail missing %s", needle)
		}
	}
}

func TestSoldierDetailGroupsAdvancedToolsUnderAccordion(t *testing.T) {
	var buf bytes.Buffer
	err := SoldierDetail(viewmodel.Soldier{
		ID:        24,
		DisplayID: "JCM87-00024",
		FirstName: "Andrew",
		LastName:  "Cole",
		Unit:      "Co. A, 1st Texas Infantry",
		BirthDate: "05/12/1838",
		SourceRecords: []viewmodel.Record{{
			SourceRecordType: "Muster Roll",
			AppID:            "APP-24",
			Details:          "Enlisted on 03/11/1862 at Austin.",
		}},
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"Advanced Research &amp; Review",
		"Collections, packs, ledgers, timelines, and review actions stay tucked away until you need them.",
		"Review Queue",
		"Research Log",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("soldier detail missing %s", needle)
		}
	}
}
