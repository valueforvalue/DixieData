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
// PanelSoldierDetailProvenance is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PanelSoldierDetailProvenance = "panel.soldier.detail.provenance"
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
// PageInventory is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	PageInventory               = "page.inventory"
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
	// Issue #380: top-nav mega-menu panels (slice 2 + 3). The
	// Records mega-menu and the Share & Review mega-menu both
	// replace the prior flat pill + foldout pattern. The UIID
	// string is the same shape as the foldout UIIDs above
	// (layout.<surface>.menu + layout.<surface>.menu.trigger)
	// so the JS dispatcher + audit harness can use a uniform
	// lookup helper.
	LayoutRecordsMenu       = "layout.records.menu"
// LayoutRecordsMenuTrigger is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	LayoutRecordsMenuTrigger = "layout.records.menu.trigger"
	LayoutShareReviewMenu       = "layout.share-review.menu"
// LayoutShareReviewMenuTrigger is the canonical UI surface identifier (string ID). See the Registry entry below for the human-readable description.
	LayoutShareReviewMenuTrigger = "layout.share-review.menu.trigger"
	// Issue #380 slice 3: LayoutShareMenu + LayoutShareMenuTrigger
	// + LayoutResearchMenu + LayoutResearchMenuTrigger are RETIRED.
	// The 2 pre-#380 top-nav foldouts (Share + Research &
	// Review) collapsed into the single LayoutShareReviewMenu /
	// LayoutShareReviewMenuTrigger mega-menu (declared above).

	// Issue #378: Research & Review picker (slice 1 — picker landing).
	// PageResearchPicker wraps the page-level main content area on
	// /research. PanelResearchPickerSearch is the search-input +
	// results region (target of the htmx search swap). PanelResearchPickerRecent
	// is the localStorage-backed recent-persons list region. PanelResearchPickerContinue
	// is the "Continue: ..." shortcut to the most-recent-scoped sub-page.
	PageResearchPicker            = "page.research.picker"
	PanelResearchPickerSearch    = "panel.research.picker.search"
	PanelResearchPickerResults    = "panel.research.picker.results"
	PanelResearchPickerRecent     = "panel.research.picker.recent"
	PanelResearchPickerContinue   = "panel.research.picker.continue"
	// Issue #378 slice 3: research-pack picker sub-screen that asks
	// the user to choose state vs county before redirecting to the
	// sub-page. Only renders when ?next=research-pack is set.
	PanelResearchPickerPackSubScreen = "panel.research.picker.pack-sub-screen"
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
	// PanelEventDetailImages is the canonical UI surface identifier
	// for the images gallery container on the Event detail page
	// (/events/{id}). Issue #390: smoke steps #387 + #386 used to
	// target an inline `id="data-event-images-list"` literal in
	// event_detail.templ plus per-card `data-image-*` selectors.
	// Promoting this to a canonical UIID lets the goquery invariant
	// tests pin against it the same way the soldier-side
	// PanelSoldierDetailImages does.
	PanelEventDetailImages = "panel.event.detail.images"

	// PageEventList is the canonical UI surface identifier for the
	// /events browse page wrapper (issue #396). Wraps the
	// EventList templ's main content area (header + grid of
	// EventCard). Mirrors the soldier-side PageSoldiersList shape.
	PageEventList = "page.event.list"
	// PageEventDetail is the canonical UI surface identifier for
	// the /events/{id} detail page wrapper. Wraps the
	// EventDetail templ's main content area (back button +
	// summary card + linked persons + tags + images + research log).
	PageEventDetail = "page.event.detail"
	// PageEventNew is the canonical UI surface identifier for
	// the /events/new page wrapper. Wraps the EventFormFragment
	// body when isEdit=false (i.e. event creation form).
	PageEventNew = "page.event.new"
	// PageEventEdit is the canonical UI surface identifier for
	// the /events/{id}/edit page wrapper. Wraps the
	// EventFormFragment body when isEdit=true (i.e. event update
	// form). Same templ as PageEventNew, different UIID — mutual
	// exclusion via templ's if isEdit { ... } else { ... } block.
	PageEventEdit = "page.event.edit"

	// Issue #342: Event detail sub-panels (sources / tags /
	// linked-persons). The literal DOM IDs `data-event-sources-list`
	// and `data-event-tags-list` live in event_detail.templ
	// (rendered by EventSourcesListFragment + EventTagsListFragment);
	// the post-action fragment swap targets are these surfaces.
	// PanelEventDetailLinkedPersons is the <ul> of linked
	// Person Records on the event detail page.
	PanelEventDetailSources     = "panel.event.detail.sources"
	PanelEventDetailTags        = "panel.event.detail.tags"
	PanelEventDetailLinkedPersons = "panel.event.detail.linked-persons"

	// Issue #342: Event form sub-panels (sources editor / linked
	// persons / tags). The sources list is the same RecordInputRow
	// region the soldier form uses; the linked-persons and tags
	// sections sit OUTSIDE the main edit <form> to avoid HTML-
	// invalid nested <form> (per the comment block in
	// event_form.templ). Each gets its own panel so the fragment
	// post-action swap target is canonical.
	PanelEventFormSources     = "panel.event.form.sources"
	PanelEventFormLinkedPersons = "panel.event.form.linked-persons"
	PanelEventFormTags        = "panel.event.form.tags"

	// Issue #342: floating-dock + floating-nav-panel + scratchpad
	// status pill. The dock is rendered once in layout.templ and
	// persists on every page (issue #283 / #289 / #313). The
	// scratchpad status region (`data-floating-scratchpad-status`)
	// is its own surface so live-region announcements don't
	// collide with the dock buttons.
	PanelFloatingDock           = "panel.floating.dock"
	PanelFloatingNavPanel       = "panel.floating.nav-panel"
	PanelFloatingScratchpadStatus = "panel.floating.scratchpad-status"

	// Issue #342: persistent Share Queue status pill (issue
	// #182). Fixed-position, hidden when the queue is empty so
	// the layout stays quiet for users who never touched it.
	// Lives outside the floating dock per the issue spec to avoid
	// z-index collision with the recent overlay fix.
	PanelShareQueuePill = "panel.share-queue.pill"

	// Issue #380 slice 2: Tags top-nav pill was moved into
	// the Records mega-menu (People group). LayoutTagsLink is
	// retired -- the new menuitem lives at
	// data-marker="records-tags" inside the Records mega-menu,
	// so no separate top-nav surface ID is needed.
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
	{ID: PanelSoldierDetailProvenance, Kind: "panel", Description: "Row provenance footer (Created by DixieData v1.2.N via <path> + Restored at <timestamp>) on the soldier detail page. Hidden when both fields are empty."},
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
	{ID: PageInventory, Kind: "page", Description: "Archive inventory page (issue #491): full DB rollup — Person Record subtypes + Event Records + Articles + Tags at a basic level than Insights."},
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
	// Issue #380: top-nav mega-menus (slice 2 + 3). The Records
	// mega-menu collapses 5 flat pills (Search/Browse/Events/
	// Articles/Tags) into one 2D panel. The Share & Review
	// mega-menu absorbs the Research & Review foldout + the
	// Share foldout + the standalone Insights pill into one
	// 2D panel.
	{ID: LayoutRecordsMenu, Kind: "nav", Description: "Top-nav mega-menu panel under the Records trigger (issue #380 slice 2); 2D grid with two groups (People: Search/Events/Tags; More records: Browse/Articles). Replaces 5 flat pills."},
	{ID: LayoutRecordsMenuTrigger, Kind: "nav", Description: "Top-nav Records mega-menu trigger button (issue #380 slice 2); clicking opens LayoutRecordsMenu. aria-controls points at the panel's id."},
	{ID: LayoutShareReviewMenu, Kind: "nav", Description: "Top-nav mega-menu panel under the Share & Review trigger (issue #380 slice 3); 2D grid with two groups (Review & Research: Review Queue with badge + Timeline + Research Log + Collections + Insights; Share: Landing + Export + Import + Share Queue + Sync)."},
	{ID: LayoutShareReviewMenuTrigger, Kind: "nav", Description: "Top-nav Share & Review mega-menu trigger button (issue #380 slice 3); clicking opens LayoutShareReviewMenu. aria-controls points at the panel's id."},
	{ID: PageResearchPicker, Kind: "page", Description: "Research & Review Person picker landing page (issue #378 slice 1). Search + recents + continue shortcut; honors dd_person_ctx cookie for sticky person context."},
	{ID: PanelResearchPickerSearch, Kind: "panel", Description: "Search input region on the Research picker page; htmx target for the live results swap."},
	{ID: PanelResearchPickerResults, Kind: "panel", Description: "Live search results region on the Research picker page; htmx swap target for the search fragment."},
	{ID: PanelResearchPickerRecent, Kind: "panel", Description: "Recent-persons region on the Research picker page; populated via localStorage + /soldiers/search/recent?ids=."},
	{ID: PanelResearchPickerContinue, Kind: "panel", Description: "Continue shortcut on the Research picker page when dd_person_ctx cookie is set; links to the most-recent-scoped sub-page."},
	{ID: PanelResearchPickerPackSubScreen, Kind: "panel", Description: "Research-pack picker sub-screen on /research?next=research-pack (issue #378 slice 3); offers a state vs county geography choice before the user picks a Person Record."},
	{ID: PanelShareQuickActions, Kind: "panel", Description: "Quick Actions card on /share (issue #265). Three large tiles: Export JSON, Import .ddbak, Share Queue. Above the fold."},
	{ID: PanelShareRecent, Kind: "panel", Description: "Recent activity card on /share (issue #265). Last 3 terminal jobs sorted by StartedAt desc. Empty state when no jobs exist."},
	{ID: PanelShareAllExports, Kind: "panel", Description: "All Exports card on /share (issue #265). Below the fold. Renamed from 'Export & Backup'."},
	{ID: PanelShareAllImports, Kind: "panel", Description: "All Imports card on /share (issue #265). Below the fold. Renamed from 'Import & Restore'."},
	{ID: PanelShareSync, Kind: "panel", Description: "Sync card on /share (issue #265). Google Integration card wrapped in a section header. Below the fold."},
	{ID: PanelShareSupport, Kind: "panel", Description: "Support & Diagnostics card on /share (issue #265). Below the fold. Moved from 'Export & Backup' to its own section."},
	{ID: PanelEventDetailImages, Kind: "panel", Description: "Images gallery section on the event detail page (/events/{id}); wraps the per-card grid plus empty state and is targeted by the post-delete fragment swap (issue #390)."},
	{ID: PageEventList, Kind: "page", Description: "Event Record browse page on /events; wraps the main content area (header + list of EventCard). Mirrors PageSoldiersList (issue #396)."},
	{ID: PageEventDetail, Kind: "page", Description: "Event Record detail page on /events/{id}; wraps the main content area (back button + summary card + linked persons + tags + images + research log). Mirrors PageSoldierDetail (issue #396)."},
	{ID: PageEventNew, Kind: "page", Description: "Event Record create page on /events/new; wraps the EventFormFragment body when isEdit=false. Mutually exclusive with PageEventEdit (issue #396)."},
	{ID: PageEventEdit, Kind: "page", Description: "Event Record edit page on /events/{id}/edit; wraps the EventFormFragment body when isEdit=true. Same templ as PageEventNew (issue #396)."},
	{ID: PanelEventDetailSources, Kind: "panel", Description: "Source Records list on the event detail page (/events/{id}); wraps the <div id=data-event-sources-list> swap target rendered by EventSourcesListFragment (issue #342)."},
	{ID: PanelEventDetailTags, Kind: "panel", Description: "Tags chips on the event detail page (/events/{id}); wraps the <div id=data-event-tags-list> swap target rendered by EventTagsListFragment (issue #342)."},
	{ID: PanelEventDetailLinkedPersons, Kind: "panel", Description: "Linked Person Records list on the event detail page (/events/{id}); the <ul> of person records attached to this event (issue #342)."},
	{ID: PanelEventFormSources, Kind: "panel", Description: "Source Records editor section on the event form (/events/new + /events/{id}/edit); wraps the RecordInputRow list rendered inside the main <form> (issue #342)."},
	{ID: PanelEventFormLinkedPersons, Kind: "panel", Description: "Linked Persons section on the event edit page (/events/{id}/edit); rendered OUTSIDE the main <form> to avoid HTML-invalid nested forms; Add Link + Unlink actions target this panel (issue #342, #361)."},
	{ID: PanelEventFormTags, Kind: "panel", Description: "Tags section on the event edit page (/events/{id}/edit); rendered OUTSIDE the main <form> for the same nested-form reason; Add Tag form posts to /events/{id}/tags and swaps into #data-event-tags-list (issue #342, #361)."},
	{ID: PanelFloatingDock, Kind: "panel", Description: "Persistent bottom dock rendered once in layout.templ (issue #283 / #289 / #313); hosts Scratch Pad + Feedback + Menu buttons. z-40."},
	{ID: PanelFloatingNavPanel, Kind: "panel", Description: "Slide-out nav panel toggled by the Menu button via data-floating-nav-toggle (issue #283). Duplicates top-nav links + renders the layout-mode picker; positioned bottom-right, z-50."},
	{ID: PanelFloatingScratchpadStatus, Kind: "panel", Description: "Live region in the floating dock (data-floating-scratchpad-status, aria-live=polite) for scratchpad open / save status announcements; mirrors the aria-live contract used by jobs-progress-overlay (issue #283)."},
	{ID: PanelShareQueuePill, Kind: "panel", Description: "Persistent Share Queue status pill (issue #182); fixed bottom-center, hidden when the queue is empty. Wraps data-share-queue-pill + data-share-queue-pill-label + data-share-queue-pill-count."},
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
