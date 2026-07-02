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

func TestSoldierListUsesResponsiveContractForSearchControls(t *testing.T) {
	var buf bytes.Buffer
	err := SoldierList(nil, 1, 0, "", viewmodel.SoldierFormSuggestions{}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		`grid grid-cols-1 gap-2 sm:flex sm:flex-wrap`,
		`flex w-full items-center justify-between rounded-xl`,
		`flex flex-col gap-2 sm:flex-row sm:flex-wrap sm:items-center`,
		`<label class="block text-sm text-slate-500 mb-1" for="sc-buried_in">Buried In</label>`,
		`<label class="block text-sm text-slate-500 mb-1" for="sc-review_status">Status</label>`,
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("search/quick view surface missing responsive control contract %s", needle)
		}
	}
	if strings.Contains(content, `<div class="col-span-2">
						<label class="block text-sm text-slate-500 mb-1">Buried In</label>`) {
		t.Fatalf("advanced search should not use a raw col-span-2 wrapper for Buried In")
	}
	if strings.Contains(content, `<div class="col-span-2">
						<label class="block text-sm text-slate-500 mb-1">Status</label>`) {
		t.Fatalf("advanced search should not use a raw col-span-2 wrapper for Status")
	}
}

func TestSoldierListSearchInputHasMeaningfulAriaLabel(t *testing.T) {
	var buf bytes.Buffer
	err := SoldierList(nil, 1, 0, "", viewmodel.SoldierFormSuggestions{}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	if !strings.Contains(content, `aria-label="Quick search across the Local Archive"`) {
		t.Fatalf("search input should have a meaningful aria-label, not a single-letter abbreviation")
	}
	if strings.Contains(content, `aria-label="q"`) {
		t.Fatalf("search input must not use aria-label=\"q\"; screen readers read it literally as the letter cue")
	}
}

func TestSoldierDetailImageAltUsesFallbackForBlankCaption(t *testing.T) {
	var buf bytes.Buffer
	err := SoldierDetail(viewmodel.Soldier{
		ID:        17,
		DisplayID: "JCM87-00017",
		FirstName: "Blank",
		LastName:  "Caption",
		Images: []viewmodel.Image{
			{ID: 1, FilePath: "images/a.png"},
		},
	}, nil).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	if !strings.Contains(content, `alt="Image for Person Record JCM87-00017"`) {
		t.Fatalf("blank-caption image should fall back to Person Record alt text; got:\n%s", content)
	}
}

func TestSoldierDetailImageAltStripsHTMLFromCaption(t *testing.T) {
	var buf bytes.Buffer
	err := SoldierDetail(viewmodel.Soldier{
		ID:        18,
		DisplayID: "JCM87-00018",
		FirstName: "Markup",
		LastName:  "Caption",
		Images: []viewmodel.Image{
			{ID: 2, FilePath: "images/b.png", Caption: `Found at <a href="x">Smithville</a>`},
		},
	}, nil).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	if strings.Contains(content, `alt="Found at <a`) {
		t.Fatalf("image alt should not contain raw HTML from caption")
	}
	if !strings.Contains(content, `alt="Found at Smithville"`) {
		t.Fatalf("image alt should strip HTML and keep caption text")
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
	}, nil).Render(context.Background(), &buf)
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

func TestSoldierDetailSeparatesBiographyFromInternalNotes(t *testing.T) {
	var buf bytes.Buffer
	err := SoldierDetail(viewmodel.Soldier{
		ID:        42,
		DisplayID: "STC38-00001",
		FirstName: "John",
		LastName:  "Taylor",
		Biography: "Public-facing life sketch with [[STC38-00002]] link.",
		Notes:     "Private research note.",
	}, nil).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"Biography",
		"Internal Notes",
		"Public-facing life sketch",
		"Private research note.",
		`href="/soldiers/display/STC38-00002"`,
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("detail view missing biography/internal notes content %s", needle)
		}
	}
}

func TestSoldierDetailShowsPDFAndJPGExportActions(t *testing.T) {
	var buf bytes.Buffer
	err := SoldierDetail(viewmodel.Soldier{
		ID:        42,
		DisplayID: "STC38-00001",
		FirstName: "John",
		LastName:  "Taylor",
	}, nil).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"/soldiers/42/pdf",
		"/soldiers/42/jpg",
		"Export Record",
		"Export PDF",
		"Export JPG",
		"Which single-record report should I use?",
		"Portrait PDF/JPG",
		"Landscape PDF/JPG",
		"Internal notes and scratch pad content stay out of these exports.",
		"xl:flex-row xl:items-start xl:justify-between",
		"absolute left-0",
		"xl:left-auto xl:right-0",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("detail view missing export action %s", needle)
		}
	}
}

func TestSoldierDetailShowsNameOnlyHeadingAndServiceLine(t *testing.T) {
	var buf bytes.Buffer
	err := SoldierDetail(viewmodel.Soldier{
		ID:                   42,
		DisplayID:            "STC38-00001",
		Prefix:               "Capt.",
		ShowPrefixBeforeName: false,
		FirstName:            "John",
		LastName:             "Taylor",
		RankOut:              "Captain",
		Unit:                 "Co. A, 1st Texas Infantry",
	}, nil).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	if strings.Contains(content, "Captain John Taylor") {
		t.Fatalf("detail view should not prepend rank to the heading: %s", content)
	}
	for _, needle := range []string{
		">John Taylor<",
		"Captain Co. A, 1st Texas Infantry",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("detail view missing %s", needle)
		}
	}
}

func TestSoldierDetailShowsPrefixWhenEnabled(t *testing.T) {
	var buf bytes.Buffer
	err := SoldierDetail(viewmodel.Soldier{
		ID:                   42,
		DisplayID:            "STC38-00001",
		Prefix:               "Capt.",
		ShowPrefixBeforeName: true,
		FirstName:            "John",
		LastName:             "Taylor",
		RankOut:              "Captain",
	}, nil).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	if !strings.Contains(content, "Capt. John Taylor") {
		t.Fatalf("detail view should respect prefix visibility: %s", content)
	}
}

func TestSoldierDetailServiceLineFallsBackToSingleValue(t *testing.T) {
	var buf bytes.Buffer
	err := SoldierDetail(viewmodel.Soldier{
		ID:        42,
		DisplayID: "STC38-00001",
		FirstName: "John",
		LastName:  "Taylor",
		Unit:      "Co. A, 1st Texas Infantry",
	}, nil).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	if !strings.Contains(content, "Co. A, 1st Texas Infantry") {
		t.Fatalf("detail view should show a unit-only service line: %s", content)
	}
	if strings.Contains(content, " - Co. A, 1st Texas Infantry") {
		t.Fatalf("detail view should not render a dangling service-line separator: %s", content)
	}
}

func TestSoldierDetailUsesFieldSpecificEmptyStates(t *testing.T) {
	var buf bytes.Buffer
	err := SoldierDetail(viewmodel.Soldier{
		ID:                    42,
		DisplayID:             "STC38-00001",
		FirstName:             "John",
		LastName:              "Taylor",
		PensionState:          "N/A",
		ConfederateHomeStatus: "N/A",
	}, nil).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"Birth Date</dt><dd>Unknown</dd>",
		"Death</dt><dd>Unknown</dd>",
		"Confederate Home Status</dt><dd>N/A</dd>",
		"Confederate Home Name</dt><dd>N/A</dd>",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("detail view missing %s", needle)
		}
	}
	for _, needle := range []string{
		"Middle Name</dt><dd>N/A</dd>",
		"Rank In</dt><dd>N/A</dd>",
		"Rank Out</dt><dd>N/A</dd>",
		"Unit</dt><dd>N/A</dd>",
	} {
		if strings.Contains(content, needle) {
			t.Fatalf("detail view should leave %s blank: %s", needle, content)
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
	}, nil).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"Primary Image",
		"/soldiers/42/images/primary/8",
		"Set as Primary",
		`class="mb-4 flex flex-col gap-3 rounded-2xl border border-slate-200 bg-white/70 px-4 py-3 text-sm text-slate-600 lg:flex-row lg:items-center lg:justify-between"`,
		`class="flex w-full flex-col items-stretch gap-2 sm:w-auto sm:flex-row sm:flex-wrap sm:items-center sm:justify-end"`,
		`class="pill-link w-full justify-center sm:w-auto"`,
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("primary image controls missing %s", needle)
		}
	}
}

func TestSoldierDetailUsesMobileSafeSummaryActions(t *testing.T) {
	var buf bytes.Buffer
	err := SoldierDetail(viewmodel.Soldier{
		ID:              42,
		DisplayID:       "STC38-00001",
		FirstName:       "John",
		LastName:        "Taylor",
		LinkedSoldierID: 7,
	}, nil).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		`class="flex w-full flex-col items-stretch gap-2 sm:w-auto sm:flex-row sm:flex-wrap sm:items-center sm:justify-end"`,
		`class="pill-link w-full justify-center sm:w-auto"`,
		`class="secondary-button w-full list-none cursor-pointer justify-center gap-2 sm:w-auto"`,
		`class="secondary-button w-full justify-center sm:w-auto"`,
		`class="primary-button w-full sm:w-auto">Export PDF`,
		`class="secondary-button w-full sm:w-auto">Export JPG`,
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("detail view missing mobile-safe summary action fragment %s", needle)
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

func TestSoldierCardHighlightedPlainMeta(t *testing.T) {
	var buf bytes.Buffer
	err := SoldierCard(viewmodel.Soldier{
		ID:        42,
		DisplayID: "JCM87-00042",
		EntryType: "soldier",
		FirstName: "John",
		LastName:  "Carter",
		DeathDate: "1864-04-12",
		BuriedIn:  "Memphis, Tennessee",
	}, true).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	// Plain meta must be present.
	for _, needle := range []string{
		"Soldier",
		"1864-04-12",
		"Memphis, Tennessee",
		"<dl",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("highlighted soldier card missing plain meta %s", needle)
		}
	}
	// Highlighted-pill class strings must NOT be present.
	for _, forbidden := range []string{
		`rounded-full border-[rgba(141,116,64,0.55)]`,
		`Death: `,
		`Buried In: `,
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("highlighted soldier card should not render pill row; found %s", forbidden)
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
	}, nil).Render(context.Background(), &buf)
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
	}, nil).Render(context.Background(), &buf)
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
	}, nil).Render(context.Background(), &buf)
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
	}, nil).Render(context.Background(), &buf)
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
	}, nil).Render(context.Background(), &buf)
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
	}, nil).Render(context.Background(), &buf)
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
	}, nil).Render(context.Background(), &buf)
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
	}, nil).Render(context.Background(), &buf)
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
	}, nil).Render(context.Background(), &buf)
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
	}, nil).Render(context.Background(), &buf)
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
	}, nil).Render(context.Background(), &buf)
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
	}, nil).Render(context.Background(), &buf)
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

// TestSoldierDetailHasShareQueueButton (issue #191) asserts the
// Person Record detail page header now exposes a [+] Queue
// button next to the existing Edit / Export Record actions.
// The button uses the same data-share-queue-add hook as the
// Browse row button so frontend/app.js needs no new wiring.
func TestSoldierDetailHasShareQueueButton(t *testing.T) {
	var buf bytes.Buffer
	err := SoldierDetail(viewmodel.PersonRecord{
		ID:        87,
		DisplayID: "JCM87-00087",
		FirstName: "James",
		LastName:  "Carter",
	}, nil).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	for _, needle := range []string{
		`data-share-queue-add="87"`,
		"+ Queue",
		"Add JCM87-00087 to the Share Queue",
	} {
		if !strings.Contains(content, needle) {
			t.Errorf("soldier detail missing %s; got %s", needle, content)
		}
	}
}
