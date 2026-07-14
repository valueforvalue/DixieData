package templates

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

func TestEntryFormOmitsInlineScratchPadLauncher(t *testing.T) {
	var buf bytes.Buffer
	err := EntryForm(viewmodel.Soldier{DisplayID: "DXD-00001"}, nil, viewmodel.SoldierFormSuggestions{}, false).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	if strings.Contains(content, "Open Scratch Pad") {
		t.Fatalf("entry form should not render an inline scratch pad button")
	}
	if !strings.Contains(content, `name="birth_date"`) || !strings.Contains(content, `name="death_date"`) {
		t.Fatalf("entry form missing canonical date fields")
	}
	if !strings.Contains(content, `data-scratchpad-display-id="DXD-00001"`) {
		t.Fatalf("entry form should surface a page-level scratch pad display id")
	}
}

func TestEntryFormKeepsDisplayIDReadonlyOnEdit(t *testing.T) {
	var buf bytes.Buffer
	err := EntryForm(viewmodel.Soldier{DisplayID: "DXD-00001"}, nil, viewmodel.SoldierFormSuggestions{}, true).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	if !strings.Contains(content, `name="display_id"`) || !strings.Contains(content, `readonly`) {
		t.Fatalf("entry form should render display_id as readonly on edit")
	}
	if !strings.Contains(content, `data-draft-key="edit-soldier-0"`) || !strings.Contains(content, `data-record-persistence`) {
		t.Fatalf("entry form should render edit draft persistence metadata")
	}
}

func TestEntryFormEditIncludesDraftVersionAndStaleDraftControls(t *testing.T) {
	var buf bytes.Buffer
	err := EntryForm(viewmodel.Soldier{
		ID:        42,
		DisplayID: "DXD-00042",
		UpdatedAt: "2026-06-07T18:00:00Z",
	}, nil, viewmodel.SoldierFormSuggestions{}, true).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		`data-draft-record-version="2026-06-07T18:00:00Z|42"`,
		`data-draft-reset-path="/soldiers/42/edit"`,
		`data-record-persistence-preview`,
		`data-record-persistence-preview-list`,
		`data-reapply-stale-draft`,
		`data-clear-draft-trigger="base"`,
		`data-clear-draft-confirm="base"`,
		`data-clear-draft-trigger="stale"`,
		`data-clear-draft-confirm="stale"`,
		`data-undo-cleared-draft`,
		"Review older saved local changes",
		"Reapply older saved local changes",
		"Delete saved local draft",
		"Confirm delete",
		"Undo delete",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("entry form missing stale draft affordance %s", needle)
		}
	}
}

func TestEntryFormIncludesSpouseFields(t *testing.T) {
	var buf bytes.Buffer
	err := EntryForm(viewmodel.Soldier{EntryType: "wife", LinkedSoldierID: 7}, []viewmodel.Soldier{
		{ID: 7, DisplayID: "TDM65-DXD-00007", FirstName: "John", LastName: "Smith"},
	}, viewmodel.SoldierFormSuggestions{}, false).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	if !strings.Contains(content, `name="entry_type"`) || !strings.Contains(content, `data-entry-type-select`) {
		t.Fatalf("entry form missing entry type selector")
	}
	if !strings.Contains(content, `Person Record`) {
		t.Fatalf("entry form missing person record label")
	}
	if !strings.Contains(content, `name="spouse_soldier_id"`) || !strings.Contains(content, `name="maiden_name"`) {
		t.Fatalf("entry form missing spouse-specific fields")
	}
	if !strings.Contains(content, `John`) || !strings.Contains(content, `TDM65-DXD-00007`) {
		t.Fatalf("entry form missing spouse candidate option")
	}
}

func TestEntryFormShowsPrefixVisibilityToggle(t *testing.T) {
	var buf bytes.Buffer
	err := EntryForm(viewmodel.Soldier{
		Prefix:               "Capt.",
		ShowPrefixBeforeName: true,
	}, nil, viewmodel.SoldierFormSuggestions{}, false).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		`name="show_prefix_before_name"`,
		`value="1"`,
		"Show prefix before name",
		`checked`,
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("entry form missing prefix visibility control %s", needle)
		}
	}
}

func TestEntryFormSeparatesBiographyAndInternalNotes(t *testing.T) {
	var buf bytes.Buffer
	err := EntryForm(viewmodel.Soldier{DisplayID: "DXD-00001"}, nil, viewmodel.SoldierFormSuggestions{}, false).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"name=\"biography\"",
		"Advanced PDF Excerpt Override",
		`name="pdf_excerpt_override"`,
		"Target 1200 chars",
		`data-live-count-display="pdf-excerpt"`,
		"Internal Notes",
		"Quick reference:",
		"[[DISPLAY-ID]]",
		"[[STC38-00007]]",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("entry form missing biography/internal notes content %s", needle)
		}
	}
}

func TestEntryFormUsesMobileSafeSourceRecordAndActionLayouts(t *testing.T) {
	var buf bytes.Buffer
	err := EntryForm(viewmodel.Soldier{ID: 42, DisplayID: "DXD-00042"}, nil, viewmodel.SoldierFormSuggestions{}, true).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	// The Button primitive emits data-* attrs with empty string values
	// (data-record-add="") while the legacy inline form omitted the
	// value (data-record-add). Both are semantically equivalent HTML.
	// Normalize the content to accept both forms.
	contentNormalized := strings.ReplaceAll(content, `data-record-add=""`, `data-record-add`)
	contentNormalized = strings.ReplaceAll(contentNormalized, `data-record-remove=""`, `data-record-remove`)
	for _, needle := range []string{
		`data-record-add class="ghost-link w-full px-4 py-2 sm:w-auto"`,
		`data-record-remove class="pill-link w-full justify-center sm:w-auto"`,
		// Issue #401: the import button is now a <label> wrapping
		// a hidden <input type="file"> inside a multipart <form>.
		// Assert against the label's class + literal text rather than
		// the legacy </button> closing tag.
		`class="primary-button w-full sm:w-auto cursor-pointer">Add Images From Computer`,
		`class="flex flex-col gap-2 pt-2 sm:flex-row sm:flex-wrap"`,
		`class="ghost-link w-full px-4 py-2 sm:w-auto"`,
	} {
		if !strings.Contains(contentNormalized, needle) {
			t.Fatalf("entry form missing mobile-safe layout fragment %s", needle)
		}
	}
}

// TestShareLandingIsSubOverview (issue #284) verifies the
// /share landing is now a sub-overview: 4 Quick Action tiles
// linking to the 3 subpages + Share Queue, plus the
// Support & Diagnostics card. The Export & Backup, Import &
// Restore, and Google Integration surfaces moved to
// /share/exports, /share/imports, and /share/sync
// respectively (each has its own test in
// share_subpages_handlers_test.go via the handler). The
// per-section assertions from the pre-#284 version of this
// test (TestShareViewIncludesSeparatedImportAndExportActions)
// moved with the sections.
func TestShareLandingIsSubOverview(t *testing.T) {
	var buf bytes.Buffer
	err := ShareView(nil, viewmodel.ArchiveCounts{}, nil).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	// Header + page summary.
	for _, needle := range []string{
		"Share Archive",
	} {
		if !strings.Contains(content, needle) {
			t.Errorf("/share landing missing %q", needle)
		}
	}
	// Quick Actions tiles link to the 3 subpages + Share
	// Queue (4 tiles). The Build Share Archive menu item
	// was folded into /share/exports per the locked
	// decision.
	for _, needle := range []string{
		`href="/share/exports"`,
		`href="/share/imports"`,
		`href="/share/sync"`,
		`href="/share/queue"`,
	} {
		if !strings.Contains(content, needle) {
			t.Errorf("/share landing missing Quick Action link %q", needle)
		}
	}
	// Support & Diagnostics moved off /share to /settings
	// (issue #255). The "/share" landing now stays focused
	// on Exports / Imports / Sync / Merge Review. The
	// diagnostic actions are still served at the same URLs
	// but render in /settings now.
	for _, forbidden := range []string{
		"Support & Diagnostics",
		"Troubleshooting bundle",
		"/export/feedback-log",
		"/export/bug-report",
	} {
		if strings.Contains(content, forbidden) {
			t.Errorf("/share landing should not contain %q (moved to /settings per issue #255)", forbidden)
		}
	}
	// The old inline Export/Import/Google sections are
	// gone from the landing. The new subpages own them.
	for _, forbidden := range []string{
		"Export & Backup",
		"Import & Restore",
		"Google Integration",
		"/import/backup",         // on the imports subpage now
		"/export/shared-archive",  // on the exports subpage now
		"/export/static-archive",  // on the exports subpage now
		"/import/shared-archive",  // on the imports subpage now
		"data-print-config-modal", // on the exports subpage now
	} {
		if strings.Contains(content, forbidden) {
			t.Errorf("/share landing should not contain %q (moved to subpage)", forbidden)
		}
	}
}

func TestShareViewShowsMergeReviewStatus(t *testing.T) {
	var buf bytes.Buffer
	// Issue #284: the Merge Review section is gated on
	// len(conflicts) > 0 in the new slim landing. Pass a
	// single conflict so the section renders. Pre-#284 the
	// section rendered unconditionally.
	err := ShareView([]viewmodel.MergeReviewConflict{
		{
			ID:                7,
			ConflictType:      "soldier-update",
			IncomingDisplayID: "STC38-00007",
			Reason:            "Shared record changed notes.",
			IncomingRecord:    viewmodel.Soldier{DisplayID: "STC38-00007", FirstName: "John", LastName: "Taylor"},
		},
	}, viewmodel.ArchiveCounts{}, nil).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		`id="merge-review-section"`,
		`Data Loaded: 1 Conflicts Found`,
		`data-merge-review-action`,
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("share view missing merge review status UI: %s", needle)
		}
	}
}

func TestShareViewUsesManagedGoogleCalendarActions(t *testing.T) {
	var buf bytes.Buffer
	// Issue #284: the Google Integration card moved to
	// /share/sync. The /share landing is now a sub-overview
	// that does NOT include the card or its controls. The
	// full Google surface is tested via the new subpage
	// handler in share_subpages_handlers_test.go.
	err := ShareView(nil, viewmodel.ArchiveCounts{}, nil).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	// Issue #284: the Google Integration card moved to
	// /share/sync. The /share landing is now a sub-overview
	// that does NOT include the card or its controls. The
	// full Google surface is tested via the new subpage
	// handler in share_subpages_handlers_test.go.
	for _, forbidden := range []string{
		"Google Integration",
		"/integrations/google/calendar/use-managed",
		"/integrations/google/calendar/sync-managed",
		"/integrations/google/calendar/unsync-managed",
		"/integrations/google/calendar/use-test",
		"/integrations/google/calendar/sync-test",
		"/integrations/google/calendar/unsync-test",
		"data-action=\"/integrations/google/connect\"",
		"data-action=\"/integrations/google/backup\"",
		"data-action=\"/integrations/google/sheets/export\"",
		"data-google-calendar-preferences-open",
		`data-busy-group="google-calendar-actions"`,
		"Compact flow:",
		"Out of sync",
		"DixieData Calendar ID",
		"DixieData Test Calendar ID",
		"Last synced:",
	} {
		if strings.Contains(content, forbidden) {
			t.Errorf("/share landing should not contain %q (moved to /share/sync)", forbidden)
		}
	}
	if strings.Contains(content, "/integrations/google/calendar/sync\"") || strings.Contains(content, "Sync Google Calendar") {
		t.Fatalf("share view should not render legacy generic calendar sync actions")
	}
}

func TestInitialSetupViewHasSurfaceInventoryID(t *testing.T) {
	var buf bytes.Buffer
	err := InitialSetupView(viewmodel.InitialSetupForm{}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	if !strings.Contains(content, "First Launch Setup") {
		t.Fatalf("initial setup should render the page heading")
	}
}

func TestSettingsViewShowsResponsiveLayoutControls(t *testing.T) {
	var buf bytes.Buffer
	err := SettingsView("RESET", viewmodel.UpdateSettings{}, "default", "jobs-page", "").Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		`data-layout-mode-option="auto"`,
		`data-layout-mode-option="relaxed"`,
		`data-layout-mode-option="split-screen"`,
		`data-layout-mode-status`,
		`data-layout-mode-preference-label`,
		`Initialize Data`,
		`Check for Updates`,
		`w-full sm:w-auto`,
		`w-full px-4 py-2 sm:w-auto`,
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("settings view missing responsive layout control %s", needle)
		}
	}
}

func TestShareViewMergeReviewUsesSharedSummaryFormatting(t *testing.T) {
	var buf bytes.Buffer
	err := ShareView([]viewmodel.MergeReviewConflict{
		{
			ID:                7,
			ConflictType:      "soldier-update",
			IncomingDisplayID: "STC38-00007",
			Reason:            "Shared record changed notes.",
			LocalRecord: &viewmodel.PersonRecord{
				DisplayID:            "STC38-00007",
				Prefix:               "Dr.",
				ShowPrefixBeforeName: false,
				FirstName:            "John",
				LastName:             "Taylor",
				EntryType:            "soldier",
				RankOut:              "Captain",
				Unit:                 "Co. B, 1st Texas",
			},
			IncomingRecord: viewmodel.Soldier{
				DisplayID:            "STC38-00007",
				Prefix:               "Mrs.",
				ShowPrefixBeforeName: true,
				FirstName:            "Jane",
				LastName:             "Taylor",
				EntryType:            "wife",
			},
		},
	}, viewmodel.ArchiveCounts{}, nil).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"John Taylor",
		"Captain Co. B, 1st Texas",
		"Mrs. Jane Taylor",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("share view missing %s", needle)
		}
	}
	if strings.Contains(content, "Captain John Taylor") || strings.Contains(content, ">Unit: Co. B, 1st Texas<") {
		t.Fatalf("share merge review should use shared summary formatting: %s", content)
	}
}

func TestInitialSetupViewIncludesIdentityFields(t *testing.T) {
	var buf bytes.Buffer
	err := InitialSetupView(viewmodel.InitialSetupForm{
		FirstName:     "Samuel",
		MiddleName:    "Thomas",
		LastName:      "Carter",
		BirthYear:     "1838",
		PrefixPreview: "STC38",
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	if !strings.Contains(content, `action="/setup"`) || !strings.Contains(content, `name="birth_year"`) {
		t.Fatalf("initial setup view missing setup form fields")
	}
	if !strings.Contains(content, "STC38") {
		t.Fatalf("initial setup view missing prefix preview")
	}
}

func TestSettingsViewIncludesSoftwareUpdatePanel(t *testing.T) {
	var buf bytes.Buffer
	err := SettingsView("INITIALIZE", viewmodel.UpdateSettings{
		CurrentVersion:     "1.2.23",
		BuildIdentity:      "DixieData v1.2.23",
		EffectiveSourceURL: "https://api.github.com/repos/valueforvalue/DixieData/releases/latest",
		UsingDefaultSource: true,
		CanApply:           true,
		LastApply: &viewmodel.UpdateApplyStatus{
			Status:    "failed",
			Version:   "1.2.22",
			Message:   "Download checksum mismatch.",
			AppliedAt: "2026-05-30T03:00:00Z",
		},
	}, "default", "jobs-page", "").Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"Software Updates",
		"/settings/updates/source",
		"/settings/updates/check",
		"/export/backup",
		"Export Backup (.ddbak)",
		"/settings/updates/apply",
		"Last update attempt failed",
		"https://api.github.com/repos/valueforvalue/DixieData/releases/latest",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("settings view missing updater UI: %s", needle)
		}
	}
}

func TestSettingsViewIncludesDataQualityPanel(t *testing.T) {
	var buf bytes.Buffer
	err := SettingsView("INITIALIZE", viewmodel.UpdateSettings{}, "default", "jobs-page", "").Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"Data Quality Scan",
		"/settings/quality/scan",
		`name="quality_mode" value="high-confidence"`,
		`name="quality_mode" value="advanced"`,
		"Run Data Quality Scan",
		`id="settings-quality-results"`,
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("settings view missing quality scan UI: %s", needle)
		}
	}
}

// TestSettingsViewIncludesSupportDiagnosticsPanel (issue #255 +
// #545 slice 3) asserts that the Support & Diagnostics card lives
// on /settings and renders the Export Feedback Log + Export Bug
// Report Bundle affordances. After #545 slice 3, the bug-report
// button is wrapped in a <form> carrying the Include-images
// checkbox (covered by TestSettingsViewIncludesBugReportImageCheckbox);
// this test pins the section identity + button copy + the two
// action URLs.
func TestSettingsViewIncludesSupportDiagnosticsPanel(t *testing.T) {
	var buf bytes.Buffer
	err := SettingsView("INITIALIZE", viewmodel.UpdateSettings{}, "default", "jobs-page", "").Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	for _, needle := range []string{
		// Section identity
		`id="settings-diagnostics-panel"`,
		"Support &amp; Diagnostics",
		"Troubleshooting bundle",
		// The two button labels
		"Export Feedback Log",
		"Export Bug Report Bundle",
		// The two action URLs. After #545 slice 3, the
		// bug-report endpoint is reached through a <form
		// action="/export/bug-report"> (so the checkbox can
		// travel in the POST body); the feedback-log button
		// remains a data-action button.
		`data-action="/export/feedback-log"`,
		`action="/export/bug-report"`,
	} {
		if !strings.Contains(content, needle) {
			t.Errorf("/settings missing Support & Diagnostics card element %q", needle)
		}
	}
}

func TestSettingsQualityScanResultsGroupsFindings(t *testing.T) {
	var buf bytes.Buffer
	err := SettingsQualityScanResults(viewmodel.DataQualityScanResult{
		Mode:           "advanced",
		ScannedRecords: 12,
		IssueCount:     2,
		Groups: []viewmodel.DataQualityIssueGroup{
			{
				Group: "Identity",
				Count: 2,
				Issues: []viewmodel.DataQualityIssue{
					{PersonRecordID: 7, DisplayID: "DXD-00007", Name: "John Doe", Summary: "Missing last name", Detail: "Name has placeholder values."},
					{PersonRecordID: 8, DisplayID: "DXD-00008", Name: "Jane Doe", Summary: "Malformed display ID"},
				},
			},
		},
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"/settings/quality/apply",
		"Identity (2)",
		`name="selected_ids" value="7"`,
		`name="selected_ids" value="8"`,
		"Move Selected to Review Queue",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("quality scan results missing content %s", needle)
		}
	}
}

func TestNewEntryFormIncludesLocalDraftIndicator(t *testing.T) {
	var buf bytes.Buffer
	err := EntryForm(viewmodel.Soldier{DisplayID: "STC38-00001", PensionState: "N/A", ConfederateHomeStatus: "N/A"}, nil, viewmodel.SoldierFormSuggestions{
		RankIn:           []string{"Private", "Sergeant"},
		RankOut:          []string{"Corporal", "Sergeant"},
		Unit:             []string{"Co. A, 1st Texas Infantry"},
		Prefix:           []string{"Capt."},
		Suffix:           []string{"Jr."},
		PensionState:     []string{"N/A", "Texas"},
		BuriedIn:         []string{"Oakwood Cemetery"},
		ConfederateHome:  []string{},
		SourceRecordType: []string{"Pension"},
	}, false).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	if !strings.Contains(content, "Local draft only.") || !strings.Contains(content, `data-draft-key="new-soldier"`) {
		t.Fatalf("new entry form missing local draft status indicator")
	}
	if !strings.Contains(content, `name="confederate_home_status"`) || !strings.Contains(content, `name="confederate_home_name"`) {
		t.Fatalf("new entry form missing confederate home fields")
	}
	if !strings.Contains(content, `name="pension_state" value="N/A"`) {
		t.Fatalf("new entry form should default pension state to N/A")
	}
	if !strings.Contains(content, `<option value="N/A" selected>N/A</option>`) {
		t.Fatalf("new entry form should default confederate home status to N/A")
	}
	if !strings.Contains(content, `list="rank-in-suggestions"`) || !strings.Contains(content, `list="record-type-suggestions"`) {
		t.Fatalf("new entry form missing datalist attributes")
	}
	if !strings.Contains(content, `name="prefix"`) || !strings.Contains(content, `list="prefix-suggestions"`) || !strings.Contains(content, `name="suffix"`) || !strings.Contains(content, `list="suffix-suggestions"`) {
		t.Fatalf("new entry form missing prefix/suffix datalist fields")
	}
	if !strings.Contains(content, `<datalist id="record-type-suggestions">`) {
		t.Fatalf("new entry form missing datalist markup")
	}
	if !strings.Contains(content, `<datalist id="prefix-suggestions">`) || !strings.Contains(content, `value="Capt."`) || !strings.Contains(content, `<datalist id="suffix-suggestions">`) || !strings.Contains(content, `value="Jr."`) {
		t.Fatalf("new entry form missing prefix/suffix suggestion markup")
	}
	if !strings.Contains(content, `list="confederate-home-name-suggestions"`) || !strings.Contains(content, `<datalist id="confederate-home-name-suggestions">`) {
		t.Fatalf("new entry form missing confederate home name datalist")
	}
	if !strings.Contains(content, `value="Co. A, 1st Texas Infantry"`) || !strings.Contains(content, `value="Oakwood Cemetery"`) {
		t.Fatalf("new entry form missing suggestion values")
	}
}

func TestShareViewIncludesMergeReviewPanel(t *testing.T) {
	var buf bytes.Buffer
	// Issue #284: Merge Review section is gated on
	// len(conflicts) > 0. Pass a conflict so the panel
	// renders and the per-conflict keep-shared action is
	// present.
	err := ShareView([]viewmodel.MergeReviewConflict{{
		ID:                42,
		ConflictType:      "soldier-update",
		IncomingDisplayID: "STC38-00042",
		Reason:            "Shared record changed notes.",
		IncomingRecord:    viewmodel.Soldier{DisplayID: "STC38-00042", FirstName: "John", LastName: "Taylor"},
	}}, viewmodel.ArchiveCounts{}, nil).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	if !strings.Contains(content, "Merge Review") || !strings.Contains(content, "/merge-review/42/keep-shared") {
		t.Fatalf("share view missing merge review actions")
	}
	if !strings.Contains(content, "remembers that mapping for future imports from the same source archive") {
		t.Fatalf("share view missing remembered mapping copy")
	}
}

func TestSearchResultsShowMatchSnippet(t *testing.T) {
	var buf bytes.Buffer
	err := SearchResults([]viewmodel.Soldier{{
		ID:                 7,
		DisplayID:          "PENSION-4242",
		FirstName:          "Nathan",
		LastName:           "Forrest",
		Unit:               "Forrest's Cavalry",
		BuriedIn:           "Memphis",
		Notes:              "Known for his cavalry leadership in Tennessee.",
		SearchMatchField:   "Unit",
		SearchMatchSnippet: "Forrest's Cavalry",
	}}, viewmodel.SoldierSearch{Mode: "basic", Query: "Forrest"}, 1, 1, 50).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	if !strings.Contains(content, "Matched on Unit") || !strings.Contains(content, "Forrest&#39;s Cavalry") {
		t.Fatalf("search results missing quick match snippet")
	}
}

func TestSearchResultsCompareButtonDescribedByHelp(t *testing.T) {
	var buf bytes.Buffer
	err := SearchResults([]viewmodel.Soldier{{
		ID:        7,
		DisplayID: "PENSION-4242",
		FirstName: "Nathan",
		LastName:  "Forrest",
	}}, viewmodel.SoldierSearch{Mode: "basic", Query: "Forrest"}, 1, 1, 50).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	// Compare Selected button + section heading must both render.
	// The verbose narration paragraph was removed in issue #561
	// slice 16 (R5 — verbose body under heading); the button's
	// visible label + disabled state are the affordance now.
	if !strings.Contains(content, "Compare Selected") {
		t.Fatalf("Search results must render Compare Selected button")
	}
	if !strings.Contains(content, `id="search-compare-selection-status"`) {
		t.Fatalf("Compare Selected section heading must have an id for aria wiring")
	}
}

func TestSearchPreviewContentShowsResearchOnlyDetails(t *testing.T) {
	var buf bytes.Buffer
	err := SearchPreviewContent(viewmodel.Soldier{
		ID:                 7,
		DisplayID:          "PENSION-4242",
		EntryType:          "widow",
		FirstName:          "Nathan",
		LastName:           "Forrest",
		Unit:               "Forrest's Cavalry",
		BuriedIn:           "Memphis",
		Notes:              "Known for his cavalry leadership in Tennessee.",
		SearchMatchField:   "Unit",
		SearchMatchSnippet: "Forrest's Cavalry",
		LinkedSoldierID:    8,
		SpouseDisplayID:    "PENSION-4243",
		SourceRecordCount:  3,
		ImageCount:         2,
		LastEditedBy:       "STC38",
		LastEditedAt:       "2026-05-16T18:05:00Z",
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"Local Archive Signals",
		"Research Context",
		"Family &amp; Links",
		"Source Records",
		"PENSION-4243",
		"Open Linked Soldier",
		"Compare Family Person Records",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("search preview missing %s", needle)
		}
	}
}

func TestSearchPreviewContentShowsUnknownForBlankDates(t *testing.T) {
	var buf bytes.Buffer
	err := SearchPreviewContent(viewmodel.Soldier{
		ID:        8,
		DisplayID: "PENSION-4244",
		FirstName: "Blank",
		LastName:  "Dates",
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	if strings.Count(content, ">Unknown<") < 2 {
		t.Fatalf("search preview should show Unknown for blank birth and death dates: %s", content)
	}
}

func TestSearchResultsShowsRecentAccessBanner(t *testing.T) {
	var buf bytes.Buffer
	err := SearchResults([]viewmodel.Soldier{{
		ID:        7,
		DisplayID: "PENSION-4242",
		FirstName: "Nathan",
		LastName:  "Forrest",
	}}, viewmodel.SoldierSearch{Mode: "basic", Recent: true}, 1, 1, 10).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"Recently Accessed",
		"Your ten most recently opened person records.",
		"Compare Selected",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("recent results missing %s", needle)
		}
	}
}

func TestSearchResultsPaginationUsesNavLandmark(t *testing.T) {
	var buf bytes.Buffer
	// 75 results at pageSize 50 -> two pages, so both prev/next fire.
	err := SearchResults([]viewmodel.Soldier{{
		ID:        7,
		DisplayID: "PENSION-4242",
		FirstName: "Nathan",
		LastName:  "Forrest",
	}}, viewmodel.SoldierSearch{Mode: "basic", Query: "Forrest"}, 1, 75, 50).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	for _, needle := range []string{
		`<nav aria-label="Search results pagination"`,
		`aria-current="page"`,
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("search pagination missing %s", needle)
		}
	}
}

func TestShareViewIncludesKeepBothForDisplayIDCollision(t *testing.T) {
	var buf bytes.Buffer
	// Issue #284: Merge Review section is gated on
	// len(conflicts) > 0. Pass a display-id-collision
	// conflict so the per-conflict keep-both + keep-shared
	// actions render.
	err := ShareView([]viewmodel.MergeReviewConflict{{
		ID:                99,
		ConflictType:      "display-id-collision",
		IncomingDisplayID: "STC38-00099",
		Reason:            "Incoming share uses an existing local display ID.",
		IncomingRecord:    viewmodel.Soldier{DisplayID: "STC38-00099", FirstName: "John", LastName: "Taylor"},
	}}, viewmodel.ArchiveCounts{}, nil).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	if !strings.Contains(content, "/merge-review/99/keep-both") || !strings.Contains(content, "Keep Both") {
		t.Fatalf("share view missing keep-both action")
	}
	if !strings.Contains(content, "/merge-review/99/keep-shared") {
		t.Fatalf("display-id collision should show keep-shared action")
	}
}

// TestEntryFormHelpersEntryTypesOmitsEvent pins issue #362's decision:
// /soldiers/new no longer exposes Event Records in the entry-type dropdown
// because /events/new owns the dedicated create surface. The defensive
// server-side dispatch for entry_type=event POSTs stays (see
// TestHandleCreateSoldierDispatchesToNewEvent) so hand-crafted curls and
// debug tools can't accidentally create a Soldier row with entry_type=event.
func TestEntryFormHelpersEntryTypesOmitsEvent(t *testing.T) {
	for _, option := range entryTypes() {
		if option.Value == "event" {
			t.Fatalf("/soldiers/new entry-type dropdown must not expose Event (issue #362); entryTypes() returned %+v", option)
		}
		if strings.EqualFold(option.Label, "Event") {
			t.Fatalf("/soldiers/new entry-type dropdown must not show an Event label (issue #362); entryTypes() returned %+v", option)
		}
	}

	// Negative belt-and-suspenders: also render the form and confirm the
	// <option value="event"> tag never appears in the HTML. This catches
	// any future regression where entryTypes() loses the Event entry but a
	// hardcoded <option> creeps back into entry_form.templ.
	var buf bytes.Buffer
	err := EntryForm(viewmodel.Soldier{DisplayID: "DXD-00001"}, nil, viewmodel.SoldierFormSuggestions{}, false).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("EntryForm render: %v", err)
	}
	if strings.Contains(buf.String(), `value="event"`) {
		t.Fatalf("/soldiers/new HTML must not contain an option with value=\"event\" (issue #362)")
	}
}

// === Issue #416 UI test additions ===
// TestSettingsQualityScanResultsRendersGenerateDisplayIDButton proves
// the per-row "Generate Display ID" button renders for identity-missing
// rows and does NOT render for other issue codes. (Issue #416.)

func TestSettingsQualityScanResultsRendersGenerateDisplayIDButton(t *testing.T) {
	var buf bytes.Buffer
	err := SettingsQualityScanResults(viewmodel.DataQualityScanResult{
		Mode:           "advanced",
		ScannedRecords: 12,
		IssueCount:     2,
		Groups: []viewmodel.DataQualityIssueGroup{
			{
				Group: "Identity",
				Count: 2,
				Issues: []viewmodel.DataQualityIssue{
					// identity-missing: button MUST render.
					{PersonRecordID: 7, Code: "identity-missing", DisplayID: "", Name: "John Doe", Summary: "Missing display ID"},
					// non-identity-missing: button MUST NOT render.
					{PersonRecordID: 8, Code: "identity-malformed", DisplayID: "DXD-00008", Name: "Jane Doe", Summary: "Malformed display ID"},
				},
			},
		},
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	// The identity-missing row's button is present.
	if !strings.Contains(content, `data-recover-display-id="7"`) {
		t.Fatalf("identity-missing row should render the Generate Display ID button; got content: %s", content)
	}
	if !strings.Contains(content, "Generate Display ID") {
		t.Fatalf("button label not found in render; got: %s", content)
	}
	if !strings.Contains(content, `/soldiers/7/display-id/recover`) {
		t.Fatalf("button formaction URL not found; got: %s", content)
	}
	// The non-identity-missing row's button is NOT present.
	if strings.Contains(content, `data-recover-display-id="8"`) {
		t.Fatalf("non-identity-missing row should NOT render the Generate Display ID button")
	}
	// The Move Selected to Review Queue form is unchanged.
	if !strings.Contains(content, "Move Selected to Review Queue") {
		t.Fatalf("Move Selected to Review Queue button should still render (unchanged)")
	}
	// data-reload-on-success is present so the page refreshes after
	// a successful recover (issue #493: without it the toast is
	// queued for next nav and never surfaces).
	if !strings.Contains(content, `data-reload-on-success="true"`) {
		t.Fatalf("Generate Display ID button must carry data-reload-on-success=\"true\" so the page refreshes after a 200 (issue #493); got: %s", content)
	}
	// Issue #493: there must be NO nested <form> inside the Move
	// Selected form. Per the new design the per-row recover affordance
	// is a plain submit button with formaction="..." overriding the
	// outer form's action — no inner <form> tag at all.
	if strings.Contains(content, `<form method="post" action="/soldiers/`) {
		t.Fatalf("found nested <form> for per-row recover — issue #493 fix uses button[formaction] not a nested <form>; got: %s", content)
	}
	// The Move Selected form's id must be present so the form has a
	// stable anchor for future test selectors.
	if !strings.Contains(content, `id="settings-quality-apply"`) {
		t.Fatalf("Move Selected form must have id=\"settings-quality-apply\"; got: %s", content)
	}
}

// TestSettingsViewIncludesExportSurfacePanel (issue #534) pins the
// new "After export" radio group in the Settings -> Appearance
// panel. The user picks "Jobs page" or "Toast only"; the chosen
// value flows through to the dispatcher's post-export navigation
// decision via <html data-export-surface="...">. This test asserts
// the panel renders both radio options + the right one is
// `checked` for each input.
func TestSettingsViewIncludesExportSurfacePanel(t *testing.T) {
	for _, surface := range []string{"jobs-page", "toast-only"} {
		surface := surface
		t.Run(surface, func(t *testing.T) {
			var buf bytes.Buffer
			err := SettingsView("INITIALIZE", viewmodel.UpdateSettings{}, "default", surface, "").Render(context.Background(), &buf)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			content := buf.String()
			// Form + action endpoint.
			if !strings.Contains(content, `action="/settings/export-surface"`) {
				t.Errorf("settings view missing export-surface form action (issue #534)")
			}
			// Both radio options.
			for _, needle := range []string{
				`name="export_surface" value="jobs-page"`,
				`name="export_surface" value="toast-only"`,
			} {
				if !strings.Contains(content, needle) {
					t.Errorf("settings view missing radio option %q (issue #534)", needle)
				}
			}
			// The picked surface carries `checked`; the other doesn't.
			checkedNeedle := `name="export_surface" value="` + surface + `" checked`
			if !strings.Contains(content, checkedNeedle) {
				t.Errorf("settings view missing checked radio for surface %q (issue #534)", surface)
			}
		})
	}
}

// TestSettingsViewIncludesBugReportImageCheckbox (issue #545 slice 3)
// pins the new "Include images" checkbox on the Bug Report Bundle
// form. The checkbox must:
//   - live inside the bug-report form (not a standalone widget),
//   - render as an <input type="checkbox" name="include_images" value="true">,
//   - default to `checked` so users who don't touch it get the
//     legacy real-bytes behavior,
//   - carry a `data-include-images-checkbox` data attribute so
//     audit probes (audit/smoke_settings_diagnostics.mjs) can
//     pin its presence without depending on the label copy.
//
// The bug-report form must also carry a method="post" action so
// the dispatcher dispatches it through dispatchDixieDataForm
// (rather than treating it as a `data-action` POST with no body).
func TestSettingsViewIncludesBugReportImageCheckbox(t *testing.T) {
	var buf bytes.Buffer
	err := SettingsView("INITIALIZE", viewmodel.UpdateSettings{}, "default", "jobs-page", "").Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()

	for _, needle := range []string{
		// Form wraps the bug-report button + checkbox.
		`<form action="/export/bug-report"`,
		`name="include_images"`,
		`value="true"`,
		`type="checkbox"`,
		`data-include-images-checkbox`,
	} {
		if !strings.Contains(content, needle) {
			t.Errorf("/settings Support & Diagnostics card missing bug-report form element %q", needle)
		}
	}

	// The checkbox must default to checked (real-bytes legacy).
	// Look for the include_images input + checked attribute on
	// the same element. We allow the checked attribute to appear
	// either before or after the value attribute (templ renders
	// them in declaration order; either is fine).
	checkboxRe := regexp.MustCompile(`<input[^>]*name="include_images"[^>]*>`)
	match := checkboxRe.FindString(content)
	if match == "" {
		t.Fatal("could not locate the include_images input via regex")
	}
	if !strings.Contains(match, `checked`) {
		t.Errorf("include_images checkbox missing `checked` attribute; got %q", match)
	}
}
