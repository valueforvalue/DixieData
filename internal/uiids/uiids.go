// Package uiids declares the stable string identifiers for every UI page, tab, and panel.
package uiids

const (
// PageCalendar is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PageCalendar                = "page.calendar"
// PageInitialSetup is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PageInitialSetup            = "page.setup"
// PanelCalendarQuote is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PanelCalendarQuote          = "panel.calendar.quote"
// PanelCalendarGrid is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PanelCalendarGrid           = "panel.calendar.grid"
// PanelCalendarDetails is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PanelCalendarDetails        = "panel.calendar.details"
// PageSoldiersList is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PageSoldiersList            = "page.soldiers.list"
// PageBrowse is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PageBrowse                  = "page.browse"
// TabSoldiersSearchBasic is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	TabSoldiersSearchBasic      = "tab.soldiers.search.basic"
// PanelSoldiersSearchBasic is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PanelSoldiersSearchBasic    = "panel.soldiers.search.basic"
// TabSoldiersSearchAdvanced is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	TabSoldiersSearchAdvanced   = "tab.soldiers.search.advanced"
// PanelSoldiersSearchAdvanced is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PanelSoldiersSearchAdvanced = "panel.soldiers.search.advanced"
// PanelSoldiersResults is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PanelSoldiersResults        = "panel.soldiers.results"
// PanelBrowseResults is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PanelBrowseResults          = "panel.browse.results"
// PageSoldierDetail is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PageSoldierDetail           = "page.soldier.detail"
// PanelSoldierDetailSummary is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PanelSoldierDetailSummary   = "panel.soldier.detail.summary"
// PanelSoldierDetailRecords is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PanelSoldierDetailRecords   = "panel.soldier.detail.records"
// PanelSoldierDetailImages is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PanelSoldierDetailImages    = "panel.soldier.detail.images"
// PageSoldierNew is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PageSoldierNew              = "page.soldier.new"
// PageSoldierEdit is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PageSoldierEdit             = "page.soldier.edit"
// PanelSoldierFormScratchpad is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PanelSoldierFormScratchpad  = "panel.soldier.form.scratchpad"
// PanelSoldierFormRecords is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PanelSoldierFormRecords     = "panel.soldier.form.records"
// PanelSoldierFormImages is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PanelSoldierFormImages      = "panel.soldier.form.images"
// PageExport is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PageExport                  = "page.export"
// PanelExportActions is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PanelExportActions          = "panel.export.actions"
// PanelJobStatus is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PanelJobStatus              = "panel.job.status"
// PanelExportGoogle is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PanelExportGoogle           = "panel.export.google"
	// Issue #284: dedicated subpage IDs for the three
	// subpages that replaced the inline sections on the
	// pre-#284 /share landing. Each subpage is its own
	// route + page (Page*); the section inside is its own
	// panel (Panel*).
	PageShareExports            = "page.share.exports"
// PageShareImports is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PageShareImports            = "page.share.imports"
// PageShareSync is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PageShareSync               = "page.share.sync"
// PageShareLanding is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PageShareLanding            = "page.share.landing"
// PageInsights is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PageInsights                = "page.insights"
// PanelInsightsOverview is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PanelInsightsOverview       = "panel.insights.overview"
// PanelInsightsCemeteries is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PanelInsightsCemeteries     = "panel.insights.cemeteries"
// PanelInsightsHomes is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PanelInsightsHomes          = "panel.insights.homes"
// PanelInsightsPensions is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PanelInsightsPensions       = "panel.insights.pensions"
// PanelInsightsUnits is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PanelInsightsUnits          = "panel.insights.units"
// PanelInsightsChronology is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PanelInsightsChronology     = "panel.insights.chronology"
// PanelInsightsDuplicateAudit is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PanelInsightsDuplicateAudit = "panel.insights.duplicate-audit"
// PageReviewQueue is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PageReviewQueue             = "page.review-queue"
// PanelReviewQueueList is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PanelReviewQueueList        = "panel.review-queue.list"
// PageReviewQueueCompare is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PageReviewQueueCompare      = "page.review-queue.compare"
// PanelReviewQueueCompare is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PanelReviewQueueCompare     = "panel.review-queue.compare"
// PageResearchCollectionsHub is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PageResearchCollectionsHub  = "page.research-collections.hub"
// PageResearchCollection is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PageResearchCollection      = "page.research-collections.detail"
// PageResearchLog is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PageResearchLog             = "page.research-log"
// PageResearchPack is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PageResearchPack            = "page.research-pack"
// PageServiceTimeline is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PageServiceTimeline         = "page.service-timeline"
// PageUnitCamaraderie is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PageUnitCamaraderie         = "page.unit-camaraderie"
// PageMergeReviewLedger is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
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
// PageInsightsDrilldown is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PageInsightsDrilldown       = "page.insights.drilldown"
// PageSettings is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PageSettings                = "page.settings"
// PanelSettingsLayout is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PanelSettingsLayout         = "panel.settings.layout"
// PanelSettingsInitialize is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PanelSettingsInitialize     = "panel.settings.initialize"
// PanelSettingsUpdates is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PanelSettingsUpdates        = "panel.settings.updates"
// PanelSettingsDebug is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PanelSettingsDebug          = "panel.settings.debug"
// OverlayFloatingMenu is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	OverlayFloatingMenu         = "overlay.floating.menu"
// OverlayFeedbackModal is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	OverlayFeedbackModal        = "overlay.feedback.modal"
// OverlayPrintConfigModal is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	OverlayPrintConfigModal     = "overlay.print-config.modal"
// OverlayGoogleCalendarPrefs is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	OverlayGoogleCalendarPrefs  = "overlay.google-calendar-prefs.modal"
// OverlayImageViewer is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
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
// PanelTagsList is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PanelTagsList            = "panel.tags.list"
// PanelTagDetail is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PanelTagDetail           = "panel.tags.detail"
// OverlayTagPicker is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	OverlayTagPicker         = "overlay.tag.picker"
	// Issue #193 / #310: Share Queue surfaces are now page-mounted.
	// PanelShareQueueList hosts the per-row table on /share/queue;
	// PanelShareQueuePresets hosts the Saved Queues card (PR 3).
// PanelShareQueueList is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PanelShareQueueList       = "panel.share-queue.list"
// PanelShareQueuePresets is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PanelShareQueuePresets    = "panel.share-queue.presets"
	// Issue #264: Share top-nav foldout. The trigger is a
	// <button data-foldout-trigger="layout.share.menu"> + the
	// panel is the <ul data-foldout-panel="layout.share.menu">
	// with role="menu". The two are linked by aria-controls on
	// the trigger. The foldout pattern is generic; if a
	// future nav item (Browse, Review Queue) adopts it, the
	// trigger/panel pair should be wired through the same
	// data-foldout-* attributes so installFoldout() picks it
	// up uniformly.
	LayoutShareMenu       = "layout.share.menu"
// LayoutShareMenuTrigger is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	LayoutShareMenuTrigger = "layout.share.menu.trigger"
	// Issue #265: /share landing sections. The page is
	// reorganised into a top-of-page stack (Quick Actions,
	// Recent activity) above the existing Export/Import/Sync
	// cards. Panel IDs are the card-rooted landmarks a11y
	// tools and the smoke probe can target.
	PanelShareQuickActions = "panel.share.quick-actions"
// PanelShareRecent is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PanelShareRecent      = "panel.share.recent"
// PanelShareAllExports is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PanelShareAllExports  = "panel.share.all-exports"
// PanelShareAllImports is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PanelShareAllImports  = "panel.share.all-imports"
// PanelShareSync is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PanelShareSync        = "panel.share.sync"
// PanelShareSupport is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PanelShareSupport     = "panel.share.support"
	// Issue #284: panel IDs for the dedicated /share/exports,
	// /share/imports, /share/sync subpages. Distinct from
	// PanelShareAllExports etc. above (which are reserved
	// for the future "view all" surface on /share).
	PanelShareExports = "panel.share.exports"
// PanelShareImports is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PanelShareImports = "panel.share.imports"
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
	// Issue #284: dedicated subpage IDs. See
	// PanelShareExports / PanelShareImports above for the
	// per-section panel IDs.
	{ID: PageShareLanding, Kind: "page", Description: "Share landing sub-overview (Quick Actions + Recent + Support & Diagnostics + conditional Merge Review)."},
	{ID: PageShareExports, Kind: "page", Description: "Share Exports subpage (Export & Backup surface + Build Share Archive button)."},
	{ID: PageShareImports, Kind: "page", Description: "Share Imports subpage (Import & Restore surface)."},
	{ID: PageShareSync, Kind: "page", Description: "Share Sync subpage (Google Integration surface + Calendar Preferences modal)."},
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
	{ID: PanelShareQueueList, Kind: "panel", Description: "Per-row queued Person Records table on the /share/queue management page (issue #193); each row carries a remove button + a per-row checkbox for bulk actions."},
	{ID: PanelShareQueuePresets, Kind: "panel", Description: "Saved Queues card on the /share/queue management page (issue #310 PR 3, ported from the Share Build modal in issue #192) listing named presets with Load + Delete per row."},
	{ID: LayoutShareMenu, Kind: "nav", Description: "Top-nav foldout panel under the Share trigger; lists Export / Import / Share Queue / Build Share Archive menu items (issue #264)."},
	{ID: LayoutShareMenuTrigger, Kind: "nav", Description: "Top-nav Share foldout trigger button (issue #264); clicking opens LayoutShareMenu. aria-controls points at the panel's id."},
	{ID: PanelShareQuickActions, Kind: "panel", Description: "Quick Actions card on /share (issue #265). Three large tiles: Export JSON, Import .ddbak, Share Queue. Above the fold."},
	{ID: PanelShareRecent, Kind: "panel", Description: "Recent activity card on /share (issue #265). Last 3 terminal jobs sorted by StartedAt desc. Empty state when no jobs exist."},
	{ID: PanelShareAllExports, Kind: "panel", Description: "All Exports card on /share (issue #265). Below the fold. Renamed from 'Export & Backup'."},
	{ID: PanelShareAllImports, Kind: "panel", Description: "All Imports card on /share (issue #265). Below the fold. Renamed from 'Import & Restore'."},
	{ID: PanelShareSync, Kind: "panel", Description: "Sync card on /share (issue #265). Google Integration card wrapped in a section header. Below the fold."},
	{ID: PanelShareSupport, Kind: "panel", Description: "Support & Diagnostics card on /share (issue #265). Below the fold. Moved from 'Export & Backup' to its own section."},
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
