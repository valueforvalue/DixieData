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
	r.Post("/soldiers/scrape-findagrave", a.handleScrapeFindAGrave)
	r.Get("/soldiers/*", a.handleSoldierByID)
	r.Post("/soldiers/*", a.handleSoldierByID)
	r.Put("/soldiers/*", a.handleSoldierByID)
	r.Delete("/soldiers/*", a.handleSoldierByID)

	r.Get("/review-queue", a.handleReviewQueue)
	r.Post("/review-queue/bulk", a.handleReviewQueueBulk)
	r.Get("/review-queue/compare/*", a.handleReviewQueueCompare)
	r.Get("/compare", a.handleCompare)

	r.Get("/setup", a.handleInitialSetup)
	r.Post("/setup", a.handleInitialSetup)
	r.Get("/version", a.handleVersion)
	r.Get("/share", a.handleShare)
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
	r.Delete("/export/templates/{id}", a.handleDeleteExportTemplate)
	r.Post("/export/templates/{id}/apply", a.handleApplyExportTemplate)
	r.Get("/layout/review-count", a.handleLayoutReviewCount)
	r.Post("/export/backup", a.handleExportBackup)
	r.Post("/export/shared-archive", a.handleExportSharedArchive)
	r.Post("/export/bug-report", a.handleExportBugReport)
	r.Post("/export/feedback-log", a.handleExportFeedbackLog)
	r.Post("/insights/report/pdf", a.handleExportInsightsPDF)

	r.Post("/import/backup", a.handleImportBackup)
	r.Post("/import/shared-archive", a.handleImportSharedArchive)
	r.Post("/import/memorial-json", a.handleImportMemorialJSON)

	r.Post("/merge-review/*", a.handleMergeReviewConflict)

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