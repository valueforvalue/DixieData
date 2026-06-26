package uiids

import (
	"os"
	"strings"
)

const DebugEnvVar = "DIXIEDATA_DEBUG_UI_IDS"
const DebugArg = "--debug-ui-ids"

const (
	PageCalendar                = "page.calendar"
	PageInitialSetup            = "page.setup"
	PanelCalendarQuote          = "panel.calendar.quote"
	PanelCalendarGrid           = "panel.calendar.grid"
	PanelCalendarDetails        = "panel.calendar.details"
	PageSoldiersList            = "page.soldiers.list"
	PageBrowse                  = "page.browse"
	TabSoldiersSearchBasic      = "tab.soldiers.search.basic"
	PanelSoldiersSearchBasic    = "panel.soldiers.search.basic"
	TabSoldiersSearchAdvanced   = "tab.soldiers.search.advanced"
	PanelSoldiersSearchAdvanced = "panel.soldiers.search.advanced"
	PanelSoldiersResults        = "panel.soldiers.results"
	PanelBrowseResults          = "panel.browse.results"
	PageSoldierDetail           = "page.soldier.detail"
	PanelSoldierDetailSummary   = "panel.soldier.detail.summary"
	PanelSoldierDetailRecords   = "panel.soldier.detail.records"
	PanelSoldierDetailImages    = "panel.soldier.detail.images"
	PageSoldierNew              = "page.soldier.new"
	PageSoldierEdit             = "page.soldier.edit"
	PanelSoldierFormScratchpad  = "panel.soldier.form.scratchpad"
	PanelSoldierFormRecords     = "panel.soldier.form.records"
	PanelSoldierFormImages      = "panel.soldier.form.images"
	PageExport                  = "page.export"
	PanelExportActions          = "panel.export.actions"
	PanelJobStatus              = "panel.job.status"
	PanelExportGoogle           = "panel.export.google"
	PageInsights                = "page.insights"
	PanelInsightsOverview       = "panel.insights.overview"
	PanelInsightsCemeteries     = "panel.insights.cemeteries"
	PanelInsightsHomes          = "panel.insights.homes"
	PanelInsightsPensions       = "panel.insights.pensions"
	PanelInsightsUnits          = "panel.insights.units"
	PanelInsightsChronology     = "panel.insights.chronology"
	PanelInsightsDuplicateAudit = "panel.insights.duplicate-audit"
	PageReviewQueue             = "page.review-queue"
	PanelReviewQueueList        = "panel.review-queue.list"
	PageReviewQueueCompare      = "page.review-queue.compare"
	PanelReviewQueueCompare     = "panel.review-queue.compare"
	PageResearchCollectionsHub  = "page.research-collections.hub"
	PageResearchCollection      = "page.research-collections.detail"
	PageResearchLog             = "page.research-log"
	PageResearchPack            = "page.research-pack"
	PageServiceTimeline         = "page.service-timeline"
	PageUnitCamaraderie         = "page.unit-camaraderie"
	PageMergeReviewLedger       = "page.merge-review-ledger"
	PageInsightsDrilldown       = "page.insights.drilldown"
	PageSettings                = "page.settings"
	PanelSettingsLayout         = "panel.settings.layout"
	PanelSettingsInitialize     = "panel.settings.initialize"
	PanelSettingsUpdates        = "panel.settings.updates"
	PanelSettingsDebug          = "panel.settings.debug"
	OverlayFloatingMenu         = "overlay.floating.menu"
	OverlayFeedbackModal        = "overlay.feedback.modal"
	OverlayPrintConfigModal     = "overlay.print-config.modal"
	OverlayGoogleCalendarPrefs  = "overlay.google-calendar-prefs.modal"
	OverlayImageViewer          = "overlay.image.viewer"
)

type Surface struct {
	ID          string
	Kind        string
	Description string
}

var Registry = []Surface{
	{ID: PageCalendar, Kind: "page", Description: "Calendar landing page."},
	{ID: PageInitialSetup, Kind: "page", Description: "First launch setup page."},
	{ID: PanelCalendarQuote, Kind: "panel", Description: "Quote of the Day panel on the calendar page."},
	{ID: PanelCalendarGrid, Kind: "panel", Description: "Month grid panel on the calendar page."},
	{ID: PanelCalendarDetails, Kind: "panel", Description: "Calendar day detail panel that shows custom items and anniversary results."},
	{ID: PageSoldiersList, Kind: "page", Description: "Main soldier list and search page."},
	{ID: PageBrowse, Kind: "page", Description: "Dedicated local archive browse page."},
	{ID: TabSoldiersSearchBasic, Kind: "tab", Description: "Quick Search tab trigger on the soldier list page."},
	{ID: PanelSoldiersSearchBasic, Kind: "panel", Description: "Quick Search tab panel on the soldier list page."},
	{ID: TabSoldiersSearchAdvanced, Kind: "tab", Description: "Advanced Search tab trigger on the soldier list page."},
	{ID: PanelSoldiersSearchAdvanced, Kind: "panel", Description: "Advanced Search tab panel on the soldier list page."},
	{ID: PanelSoldiersResults, Kind: "panel", Description: "Search results panel on the soldier list page."},
	{ID: PanelBrowseResults, Kind: "panel", Description: "Browse results table on the browse page."},
	{ID: PageSoldierDetail, Kind: "page", Description: "Soldier detail page."},
	{ID: PanelSoldierDetailSummary, Kind: "panel", Description: "Summary and actions panel on the soldier detail page."},
	{ID: PanelSoldierDetailRecords, Kind: "panel", Description: "Records section on the soldier detail page."},
	{ID: PanelSoldierDetailImages, Kind: "panel", Description: "Images section on the soldier detail page."},
	{ID: PageSoldierNew, Kind: "page", Description: "New soldier record form page."},
	{ID: PageSoldierEdit, Kind: "page", Description: "Edit soldier record form page."},
	{ID: PanelSoldierFormScratchpad, Kind: "panel", Description: "Scratch pad launcher section inside the soldier form."},
	{ID: PanelSoldierFormRecords, Kind: "panel", Description: "Records editor section inside the soldier form."},
	{ID: PanelSoldierFormImages, Kind: "panel", Description: "Image upload section inside the soldier form."},
	{ID: PageExport, Kind: "page", Description: "Export page."},
	{ID: PanelJobStatus, Kind: "panel", Description: "Background-job status page panel."},
	{ID: PanelExportActions, Kind: "panel", Description: "Main export and import actions panel."},
	{ID: PanelExportGoogle, Kind: "panel", Description: "Google integration panel on the export page."},
	{ID: PageInsights, Kind: "page", Description: "Archive insights dashboard page."},
	{ID: PanelInsightsOverview, Kind: "panel", Description: "Overview card on the insights page."},
	{ID: PanelInsightsCemeteries, Kind: "panel", Description: "Top cemeteries analytics card."},
	{ID: PanelInsightsHomes, Kind: "panel", Description: "Confederate home analytics card."},
	{ID: PanelInsightsPensions, Kind: "panel", Description: "Pension distribution analytics card."},
	{ID: PanelInsightsUnits, Kind: "panel", Description: "Unit representation analytics card."},
	{ID: PanelInsightsChronology, Kind: "panel", Description: "Chronological decade analytics card."},
	{ID: PanelInsightsDuplicateAudit, Kind: "panel", Description: "Duplicate audit card on the insights page."},
	{ID: PageReviewQueue, Kind: "page", Description: "Review queue page for flagged records."},
	{ID: PanelReviewQueueList, Kind: "panel", Description: "Main list of records awaiting review."},
	{ID: PageReviewQueueCompare, Kind: "page", Description: "Duplicate audit and manual person-record comparison page."},
	{ID: PanelReviewQueueCompare, Kind: "panel", Description: "Side-by-side duplicate comparison panel."},
	{ID: PageResearchCollectionsHub, Kind: "page", Description: "Research collections hub page."},
	{ID: PageResearchCollection, Kind: "page", Description: "Research collection detail page."},
	{ID: PageResearchLog, Kind: "page", Description: "Research log page for a person record."},
	{ID: PageResearchPack, Kind: "page", Description: "Research pack page for county or state context."},
	{ID: PageServiceTimeline, Kind: "page", Description: "Service timeline page for a person record."},
	{ID: PageUnitCamaraderie, Kind: "page", Description: "Unit camaraderie page for a person record."},
	{ID: PageMergeReviewLedger, Kind: "page", Description: "Merge review ledger page for a person record."},
	{ID: PageInsightsDrilldown, Kind: "page", Description: "Insights drilldown results page."},
	{ID: PageSettings, Kind: "page", Description: "Settings page."},
	{ID: PanelSettingsLayout, Kind: "panel", Description: "Responsive layout mode controls on the settings page."},
	{ID: PanelSettingsInitialize, Kind: "panel", Description: "Initialize Data panel on the settings page."},
	{ID: PanelSettingsUpdates, Kind: "panel", Description: "Software Updates panel on the settings page."},
	{ID: PanelSettingsDebug, Kind: "panel", Description: "Debug mode toggle on the settings page."},
	{ID: OverlayFloatingMenu, Kind: "overlay", Description: "Floating quick-navigation menu overlay."},
	{ID: OverlayFeedbackModal, Kind: "overlay", Description: "Global feedback modal overlay."},
	{ID: OverlayPrintConfigModal, Kind: "overlay", Description: "Printable export settings modal overlay."},
	{ID: OverlayGoogleCalendarPrefs, Kind: "overlay", Description: "Google managed calendar event preferences modal overlay."},
	{ID: OverlayImageViewer, Kind: "overlay", Description: "Full-screen image preview overlay."},
}

func DebugEnabled() bool {
	return truthy(os.Getenv(DebugEnvVar))
}

func EnableFromArgs(args []string) bool {
	for _, arg := range args {
		if strings.TrimSpace(arg) == DebugArg {
			_ = os.Setenv(DebugEnvVar, "1")
			return true
		}
	}
	return false
}

func truthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
