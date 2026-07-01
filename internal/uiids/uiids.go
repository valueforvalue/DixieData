package uiids

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
	// PanelResearchCollectionsHub is the named-research-collections
	// list/table on the Research Collections Hub page.
	PanelResearchCollectionsHub = "panel.research-collections.hub"
	// PanelResearchCollection is the items + add-row section on a
	// single Research Collection detail page.
	PanelResearchCollection     = "panel.research-collection.detail"
	// PanelResearchLog wraps the log entries + task creation form
	// on the per-soldier Research Log page.
	PanelResearchLog            = "panel.research-log"
	// PanelResearchPack wraps the pack contents on the county/state
	// scoped Research Pack page.
	PanelResearchPack           = "panel.research-pack"
	// PanelSoldierTimeline wraps the evidence-backed chronology
	// rendered inside the soldier detail page (Timeline HTMX swap).
	PanelSoldierTimeline        = "panel.soldier.timeline"
	// PanelSoldierCamaraderie wraps the unit-graph view rendered
	// inside the soldier detail page (Camaraderie HTMX swap).
	PanelSoldierCamaraderie     = "panel.soldier.camaraderie"
	// PanelSoldierConflictLedger wraps the Local-vs-Incoming merge
	// ledger rendered inside the soldier detail page (Conflict Ledger
	// HTMX swap).
	PanelSoldierConflictLedger  = "panel.soldier.conflict-ledger"
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
	// OverlayJobsProgress is the fixed-position popup region that
	// shows a progress card for the most recent active background
	// job (polled via /jobs/active every 3s). Lives once in
	// layout.templ so the card floats over every page; not a
	// per-page panel. Silent kinds in jobs.SilentKinds (e.g.
	// static_archive) are filtered out by MostRecentActive so
	// this region stays empty for them.
	OverlayJobsProgress = "overlay.jobs.progress"
	// Issue #183: Person Record tagging surfaces.
	PageTagsManagement       = "page.tags.management"
	PanelTagsList            = "panel.tags.list"
	PanelTagDetail           = "panel.tags.detail"
	OverlayTagPicker         = "overlay.tag.picker"
	// Issue #182: Share Queue surfaces.
	OverlayShareQueue         = "overlay.share-queue.modal"
	PanelShareQueueList       = "panel.share-queue.list"
	PanelShareQueuePreview    = "panel.share-queue.preview"
	PanelShareQueuePresets    = "panel.share-queue.presets"
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
	{ID: OverlayJobsProgress, Kind: "overlay", Description: "Global fixed-position popup region that renders the most recent active background job's progress card (polled via /jobs/active every 3s)."},
	{ID: PanelResearchCollectionsHub, Kind: "panel", Description: "Named research collections list and create-collection section on the Research Collections Hub page."},
	{ID: PanelResearchCollection, Kind: "panel", Description: "Items list and add-row section on a Research Collection detail page."},
	{ID: PanelResearchLog, Kind: "panel", Description: "Research log entries and task creation form on the per-soldier Research Log page."},
	{ID: PanelResearchPack, Kind: "panel", Description: "Pack contents on the county/state scoped Research Pack page."},
	{ID: PanelSoldierTimeline, Kind: "panel", Description: "Evidence-backed chronology rendered inside the soldier detail page (Timeline HTMX swap)."},
	{ID: PanelSoldierCamaraderie, Kind: "panel", Description: "Unit camaraderie graph rendered inside the soldier detail page (Camaraderie HTMX swap)."},
	{ID: PanelSoldierConflictLedger, Kind: "panel", Description: "Local vs Incoming merge ledger rendered inside the soldier detail page (Conflict Ledger HTMX swap)."},
	{ID: PageTagsManagement, Kind: "page", Description: "Tag management surface listing all Tags with rename / merge / delete actions."},
	{ID: PanelTagsList, Kind: "panel", Description: "Tag table on the /tags management page."},
	{ID: PanelTagDetail, Kind: "panel", Description: "Single tag detail page showing the membership list with Remove buttons."},
	{ID: OverlayTagPicker, Kind: "overlay", Description: "Inline tag-picker overlay used on the soldier detail page and in the Browse bulk-tag toolbar."},
	{ID: OverlayShareQueue, Kind: "overlay", Description: "Share Build modal listing the user's queued Person Records (issue #182); opens from the persistent Share Queue pill on the layout shell."},
	{ID: PanelShareQueueList, Kind: "panel", Description: "Per-row queued Person Records list inside the Share Build modal; each row carries a remove button + a per-row checkbox."},
	{ID: PanelShareQueuePreview, Kind: "panel", Description: "Live preview pane inside the Share Build modal showing the Soldiers / Source Records / Images count summary."},
	{ID: PanelShareQueuePresets, Kind: "panel", Description: "Saved Queues section inside the Share Build modal (issue #192) listing named presets with Load + Delete per row."},
}



// Has reports whether id is one of the canonical surface IDs in
// Registry. Used by htmxattr.Mux to validate hx-target selectors at
// render time and by other packages that need to know whether a
// string is a known surface.
func Has(id string) bool {
	for _, s := range Registry {
		if s.ID == id {
			return true
		}
	}
	return false
}
