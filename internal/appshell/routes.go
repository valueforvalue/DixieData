// routes.go holds the App.setupRoutes method. Extracted from app.go as step
// 1 of the God-class reduction tracked in issue #42. All handler methods
// referenced here stay defined on *App in their domain-specific files; the
// route table is the single point that maps URL patterns to handler methods.
//
// PR #1 (Stabilization Sprint): migrated the underlying router from
// net/http.ServeMux to github.com/go-chi/chi/v5. Chi gives us middleware
// composition and explicit pattern routing without changing any handler
// signatures. Each handler reads r.URL.Path directly, so passing through
// chi's wildcard routes preserves the existing prefix-trim logic in the
// handler bodies.
package appshell

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/valueforvalue/DixieData/internal/debug"
)

func (a *App) setupRoutes() {
	r := chi.NewRouter()

	// Standard middleware stack. Order matters: recover wraps everything
	// else so a panic in a handler produces a 500 instead of crashing the
	// process. RequestID lets handlers log with a stable ID across the
	// crash log + debug log + response header.
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	// Static frontend assets. These are served by the asset handler (see
	// app.go AssetServer) but the local-loopback HTTP server also serves
	// them when running in dev or when the embedded asset server is
	// bypassed.
	r.Get("/app.js", a.handleFrontendAsset("app.js", "text/javascript; charset=utf-8"))
	r.Get("/app.css", a.handleFrontendAsset("app.css", "text/css; charset=utf-8"))
	r.Get("/debug.js", a.handleFrontendAsset("debug.js", "text/javascript; charset=utf-8"))
	r.Get("/debug-toolbox.js", a.handleFrontendAsset("debug-toolbox.js", "text/javascript; charset=utf-8"))
	r.Get("/htmx.min.js", a.handleFrontendAsset("htmx.min.js", "text/javascript; charset=utf-8"))
	r.Get("/index.html", a.handleFrontendAsset("index.html", "text/html; charset=utf-8"))

	r.Get("/recovery", a.handleRecovery)
	r.Get("/jobs/active", a.renderActiveJob)
	r.Get("/jobs/*", a.handleJobStatus)
	r.Post("/jobs/*", a.handleJobStatus)

	r.Get("/", a.handleCalendar)
	r.Get("/calendar", a.handleCalendar)
	r.Get("/calendar/*", a.handleCalendarMonth)
	r.Post("/calendar/*", a.handleCalendarMonth)
	r.Get("/anniversary/*", a.handleAnniversary)

	r.Get("/soldiers", a.handleSoldiers)
	r.Post("/soldiers", a.handleSoldiers)
	r.Get("/browse", a.handleBrowse)
	r.Get("/browse/results", a.handleBrowseResults)
	r.Get("/soldiers/search", a.handleSearch)
	r.Get("/soldiers/search/recent", a.handleRecentSearch)
	r.Get("/soldiers/search/advanced", a.handleAdvancedSearch)
	r.Get("/soldiers/display/*", a.handleSoldierByDisplayID)
	r.Post("/soldiers/display/*", a.handleSoldierByDisplayID)
	r.Put("/soldiers/display/*", a.handleSoldierByDisplayID)
	r.Delete("/soldiers/display/*", a.handleSoldierByDisplayID)
	r.Get("/soldiers/new", a.handleNewSoldier)
	r.Post("/soldiers/new", a.handleNewSoldier)
	r.Get("/soldiers/{id:[0-9]+}/tags", a.handleTagAutocomplete)
	r.Post("/soldiers/{id:[0-9]+}/tags", a.handleAttachTag)
	r.Post("/soldiers/{id:[0-9]+}/tags/{tagId:[0-9]+}", a.handleDetachTag)
	r.Post("/browse/bulk-tag", a.handleBulkTagFromBrowse)
	r.Get("/soldiers/*", a.handleSoldierByID)
	r.Post("/soldiers/*", a.handleSoldierByID)
	r.Put("/soldiers/*", a.handleSoldierByID)
	r.Delete("/soldiers/*", a.handleSoldierByID)
	// v60 (issue #320): Event Record routes. Registered
	// before the /soldiers/* catch-all so the literal /events
	// prefix matches first. The /events/{id:[0-9]+}/edit
	// path uses chi's regex capture to bind the id; the
	// handler reads the id from the URL path itself so the
	// route stays consistent with the /soldiers/{id:[0-9]+}
	// pattern. The per-event image routes + per-event PDF
	// route are deferred to follow-up issues (out of scope
	// for the v1 landing per the RPCI spec's "Slice 4"
	// section).
	r.Get("/events", a.handleEvents)
	r.Get("/events/new", a.handleNewEvent)
	r.Post("/events/new", a.handleNewEvent)
	r.Get("/events/{id:[0-9]+}", a.handleEventByID)
	r.Post("/events/{id:[0-9]+}", a.handleEventByID)
	r.Put("/events/{id:[0-9]+}", a.handleEventByID)
	r.Delete("/events/{id:[0-9]+}", a.handleEventByID)
	// /events/{id}/edit dispatches to handleEditEvent via
	// dedicated route shims; the handler reads the id from
	// the URL path so the sub-path stays in one place.
	r.Get("/events/{id:[0-9]+}/edit", a.handleEditEventRoute)
	r.Post("/events/{id:[0-9]+}/edit", a.handleEditEventRoute)
	// /events/{id}/pdf dispatches the per-Event PDF export
	// (issue #320 v1). Mirrors the /soldiers/{id}/pdf route
	// shape; the handler uses a.saveFileDialogOverride in
	// tests so the Wails native dialog is bypassed in CI.
	r.Get("/events/{id:[0-9]+}/pdf", a.handleEventPDFRoute)
	r.Post("/events/{id:[0-9]+}/pdf", a.handleEventPDFRoute)
	// Issue #320 slice #328: per-Event research log. The
	// routes mirror the /soldiers/{id}/research-log shape
	// (GET list, POST /tasks create, POST /tasks/{id}/resolve
	// close) but dispatch through a.soldiers.ResearchLog /
	// AddResearchTask / ResolveResearchTask because the
	// research_tasks table is FK-linked to soldiers(id) and
	// Events are rows in the same table (entry_type = 'event').
	r.Get("/events/{id:[0-9]+}/research-log", a.handleEventResearchLogRoute)
	r.Post("/events/{id:[0-9]+}/research-log/tasks", a.handleEventResearchLogRoute)
	r.Post("/events/{id:[0-9]+}/research-log/tasks/{entryId:[0-9]+}/resolve", a.handleEventResearchLogRoute)
	// Issue #320 slice #329: per-Event Sources panel. Source
	// Records are rows in the `records` table keyed by
	// person_record_id (Events are soldiers rows so the FK applies
	// unchanged). The handler dispatches on r.URL.Path suffix.
	r.Get("/events/{id:[0-9]+}/sources", a.handleEventSourcesRoute)
	// Issue #360: the /events/{id}/sources/attach route has no UI
	// caller as of this commit (the Sources panel on the detail
	// page now points users to /events/{id}/edit, where #357's
	// inline RecordInputRow covers attach). The route stays
	// reachable because the attach/detach round-trip
	// TestHandleEventSourcesAndScratchpad pins the backend wiring
	// for any future programmatic attach path (e.g. .ddshare
	// replay, bulk-import). Hand-coded as 'orphan' in the probe
	// output by design — verify the route + handler still resolve
	// before deleting in a future cleanup issue.
	r.Post("/events/{id:[0-9]+}/sources/attach", a.handleEventSourcesRoute)
	r.Post("/events/{id:[0-9]+}/sources/{sourceId:[0-9]+}/detach", a.handleEventSourcesRoute)
	// Issue #320 slice #333: per-Event Tags chips. The
	// person_record_tags junction FKs soldiers(id) so the
	// same table covers Events.
	r.Get("/events/{id:[0-9]+}/tags", a.handleEventTagsRoute)
	r.Post("/events/{id:[0-9]+}/tags", a.handleEventTagsRoute)
	r.Post("/events/{id:[0-9]+}/tags/{tagId:[0-9]+}/detach", a.handleEventTagsRoute)
	// Issue #320 child #332 (slot 16 of 16): per-Event images
	// gallery. GET returns the fragment (lazy-load + post-action
	// swap target); POST /import opens the native file picker
	// and enqueues an image_import job; POST /delete re-renders
	// the fragment in place (no X-DixieData-Redirect, per issue
	// #341).
	r.Get("/events/{id:[0-9]+}/images", a.handleEventImagesRoute)
	r.Post("/events/{id:[0-9]+}/images/import", a.handleEventImagesRoute)
	r.Post("/events/{id:[0-9]+}/images/delete", a.handleEventImagesRoute)
	// Events tab. The /events sub-path on a Person Record
	// page is dispatched from a dedicated route shim so
	// the literal path wins over the generic
	// /soldiers/* catch-all. The /events/quick-add path is
	// also registered here (not dispatched from
	// handleSoldierByID) so the create+link transaction
	r.Get("/soldiers/{id:[0-9]+}/events", a.handlePersonEventsTabRoute)
	r.Post("/soldiers/{id:[0-9]+}/events/{eventId:[0-9]+}/attach", a.handleAttachEventRoute)
	r.Post("/soldiers/{id:[0-9]+}/events/{eventId:[0-9]+}/detach", a.handleDetachEventRoute)
	r.Post("/soldiers/{id:[0-9]+}/events/quick-add", a.handleQuickAddEventRoute)
	r.Post("/soldiers/{id:[0-9]+}/events/attach-by-display-id", a.handleAttachEventByDisplayIDRoute)
	r.Get("/review-queue", a.handleReviewQueue)
	r.Post("/review-queue/bulk", a.handleReviewQueueBulk)
	r.Get("/review-queue/compare/*", a.handleReviewQueueCompare)
	r.Get("/compare", a.handleCompare)

	r.Get("/setup", a.handleInitialSetup)
	r.Post("/setup", a.handleInitialSetup)
	r.Get("/version", a.handleVersion)
	r.Get("/share", a.handleShare)
	// Issue #284: dedicated subpages for the Share surface.
	// Each owns a slice of the original /share landing;
	// the landing reorg + foldout menu update land in
	// Slice 2. The /share/queue route already exists below.
	r.Get("/share/exports", a.handleShareExports)
	r.Get("/share/imports", a.handleShareImports)
	r.Get("/share/sync", a.handleShareSync)
	r.Get("/research-collections", a.handleResearchCollections)
	r.Get("/research-collections/*", a.handleResearchCollectionByID)

	r.Get("/insights", a.handleInsights)
	r.Get("/insights/drilldown", a.handleInsightsDrilldown)
	r.Post("/insights/audit/duplicates", a.handleRunDuplicateAudit)

	r.Get("/export", a.handleLegacyExportRedirect)
	r.Get("/settings", a.handleSettings)
	r.Post("/settings/initialize", a.handleSettingsInitialize)
	r.Post("/settings/updates/source", a.handleUpdateSource)
	r.Post("/settings/updates/check", a.handleCheckForUpdates)
	r.Post("/settings/updates/apply", a.handleApplyLatestUpdate)
	r.Post("/settings/updates/health/bootstrap", a.handleUpdateBootstrapHealth)
	r.Post("/settings/images/orphans/scan", a.handleScanImageOrphans)
	r.Post("/settings/images/orphans/cleanup", a.handleCleanupImageOrphans)
	r.Post("/settings/quality/scan", a.handleScanDataQuality)
	r.Post("/settings/quality/apply", a.handleApplyDataQuality)

	r.Post("/export/json", a.handleExportJSON)
	r.Post("/export/csv", a.handleExportCSV)
	r.Post("/export/ical", a.handleExportICalendar)
	r.Post("/export/static-archive", a.handleExportStaticArchive)
	r.Post("/export/database-pdf", a.handleExportDatabasePDF)
	r.Post("/export/preview", a.handleExportPreview)
	r.Get("/export/templates", a.handleListExportTemplates)
	r.Post("/export/templates", a.handleSaveExportTemplate)
	// Issue #258: the print-config modal's Save Changes button
	// fires POST /export/templates/{id} from JS (see
	// frontend/app.js: updateSelectedTemplate). The handler
	// already accepts POST as well as PATCH (see
	// handleUpdateExportTemplate), so expose both verbs at the
	// router level — chi matches the exact verb, so without
	// this alias the POST fetch lands on 405 Method Not Allowed.
	// PATCH stays canonical per REST; POST is the alias used by
	// the JS dispatcher (which keeps verb=POST so it composes
	// with the Option C X-DixieData-Redirect flow used by every
	// other form in the app).
	r.Patch("/export/templates/{id}", a.handleUpdateExportTemplate)
	r.Post("/export/templates/{id}", a.handleUpdateExportTemplate)
	r.Delete("/export/templates/{id}", a.handleDeleteExportTemplate)
	r.Post("/export/templates/{id}/apply", a.handleApplyExportTemplate)
	r.Get("/layout/review-count", a.handleLayoutReviewCount)
	r.Get("/tags", a.handleTagsManagementPage)
	r.Get("/tags/{id:[0-9]+}", a.handleTagDetailPage)
	r.Post("/tags/{id:[0-9]+}/rename", a.handleRenameTag)
	r.Post("/tags/{id:[0-9]+}/merge", a.handleMergeTag)
	r.Delete("/tags/{id:[0-9]+}", a.handleDeleteTag)
	r.Patch("/share/export-options", a.handleShareExportOptions)
	r.Post("/export/backup", a.handleExportBackup)
	r.Post("/export/shared-archive", a.handleExportSharedArchive)
	r.Post("/export/bug-report", a.handleExportBugReport)
	r.Post("/export/feedback-log", a.handleExportFeedbackLog)
	r.Post("/insights/report/pdf", a.handleExportInsightsPDF)

	r.Post("/import/backup", a.handleImportBackup)
	r.Post("/import/shared-archive", a.handleImportSharedArchive)
	r.Post("/import/memorial-json", a.handleImportMemorialJSON)

	r.Post("/merge-review/*", a.handleMergeReviewConflict)
	// Issue #234: lazy-load print-records fragment. Same wildcard
	// ordering concern as /share/queue/*: register before the
	// generic /share handler so chi resolves the literal first.
	r.Get("/share/print-records-fragment", a.handleSharePrintRecordsFragment)
	// Issue #192: saved Share Queue presets. /share/queue/presets
	// is a literal prefix; the /{id} and /{id}/apply routes
	// register below and chi matches in order so the literal
	// GET/POST/DELETE win over the wildcard /{id:[0-9]+} ones.
	r.Get("/share/queue/presets", a.handleListShareQueuePresets)
	r.Post("/share/queue/presets", a.handleSaveShareQueuePreset)
	r.Delete("/share/queue/presets/{id:[0-9]+}", a.handleDeleteShareQueuePreset)
	r.Get("/share/queue/presets/{id:[0-9]+}/apply", a.handleApplyShareQueuePreset)
	// Issue #193: /share/queue management page. The literal
	// route must register before the existing /share wildcard
	// handler so chi resolves this URL rather than the
	// generic Share page.
	r.Get("/share/queue", a.handleShareQueuePage)

	r.Post("/integrations/google/connect", a.handleGoogleConnect)
	r.Post("/integrations/google/disconnect", a.handleGoogleDisconnect)
	r.Post("/integrations/google/backup", a.handleGoogleBackup)
	r.Post("/integrations/google/sheets/export", a.handleGoogleSheetsExport)
	r.Post("/integrations/google/calendar/use-managed", a.handleGoogleCalendarUseManaged)
	r.Post("/integrations/google/calendar/preferences/save", a.handleGoogleCalendarPreferencesSave)
	r.Post("/integrations/google/calendar/sync-managed", a.handleGoogleCalendarSyncManaged)
	r.Post("/integrations/google/calendar/unsync-managed", a.handleGoogleCalendarUnsyncManaged)
	r.Post("/integrations/google/calendar/use-test", a.handleGoogleCalendarUseTest)
	r.Post("/integrations/google/calendar/sync-test", a.handleGoogleCalendarSyncTest)
	r.Post("/integrations/google/calendar/unsync-test", a.handleGoogleCalendarUnsyncTest)

	r.Post("/images/screenshot", a.handleImageScreenshot)
	r.Post("/images/rotate", a.handleImageRotate)
	r.Post("/open-link", a.handleOpenLink)
	r.Post("/feedback/submit", a.handleFeedbackSubmit)
	r.Post("/scratchpad/open", a.handleScratchpadOpen)
	r.Get("/media/*", a.handleMedia)

	// Debug endpoints (state + client-logs + toggle).
	r.Get("/debug/state", a.handleDebugState)
	r.Post("/debug/client-logs", a.handleClientLogs)
	r.Post("/settings/debug-mode", a.handleDebugModeToggle)

	// Console + folder + clear.
	r.Get("/debug/console", a.handleDebugConsole)
	r.Get("/debug/console/tail", a.handleDebugConsoleTail)
	r.Post("/debug/console/clear", a.handleDebugConsoleClear)
	r.Get("/debug/open-folder", a.handleDebugOpenFolder)

	// debug.Middleware is OUTERMOST so the request_id it generates is on
	// the context before recover runs (the crash log line carries it).
	a.muxRaw = nil
	a.mux = debug.Middleware(recoverMiddleware(r))
}