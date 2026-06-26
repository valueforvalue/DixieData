// routes.go holds the App.setupRoutes method. Extracted from app.go as step
// 1 of the God-class reduction tracked in issue #42. All handler methods
// referenced here stay defined on *App in their domain-specific files; the
// route table is the single point that maps URL patterns to handler methods.
package appshell

import (
	"net/http"

	"github.com/valueforvalue/DixieData/internal/debug"
)

func (a *App) setupRoutes() {
	mux := http.NewServeMux()

	mux.HandleFunc("/app.js", a.handleFrontendAsset("app.js", "text/javascript; charset=utf-8"))
	mux.HandleFunc("/app.css", a.handleFrontendAsset("app.css", "text/css; charset=utf-8"))
	mux.HandleFunc("/debug.js", a.handleFrontendAsset("debug.js", "text/javascript; charset=utf-8"))
	mux.HandleFunc("/htmx.min.js", a.handleFrontendAsset("htmx.min.js", "text/javascript; charset=utf-8"))
	mux.HandleFunc("/index.html", a.handleFrontendAsset("index.html", "text/html; charset=utf-8"))
	mux.HandleFunc("/recovery", a.handleRecovery)
	mux.HandleFunc("/jobs/", a.handleJobStatus)
	mux.HandleFunc("/", a.handleCalendar)
	mux.HandleFunc("/calendar", a.handleCalendar)
	mux.HandleFunc("/calendar/", a.handleCalendarMonth)
	mux.HandleFunc("/anniversary/", a.handleAnniversary)
	mux.HandleFunc("/soldiers", a.handleSoldiers)
	mux.HandleFunc("/browse", a.handleBrowse)
	mux.HandleFunc("/browse/results", a.handleBrowseResults)
	mux.HandleFunc("/soldiers/search", a.handleSearch)
	mux.HandleFunc("/soldiers/search/recent", a.handleRecentSearch)
	mux.HandleFunc("/soldiers/search/advanced", a.handleAdvancedSearch)
	mux.HandleFunc("/soldiers/display/", a.handleSoldierByDisplayID)
	mux.HandleFunc("/soldiers/new", a.handleNewSoldier)
	mux.HandleFunc("/soldiers/scrape-findagrave", a.handleScrapeFindAGrave)
	mux.HandleFunc("/soldiers/", a.handleSoldierByID)
	mux.HandleFunc("/review-queue", a.handleReviewQueue)
	mux.HandleFunc("/review-queue/bulk", a.handleReviewQueueBulk)
	mux.HandleFunc("/review-queue/compare/", a.handleReviewQueueCompare)
	mux.HandleFunc("/compare", a.handleCompare)
	mux.HandleFunc("/setup", a.handleInitialSetup)
	mux.HandleFunc("/version", a.handleVersion)
	mux.HandleFunc("/share", a.handleShare)
	mux.HandleFunc("/research-collections", a.handleResearchCollections)
	mux.HandleFunc("/research-collections/", a.handleResearchCollectionByID)
	mux.HandleFunc("/insights", a.handleInsights)
	mux.HandleFunc("/insights/drilldown", a.handleInsightsDrilldown)
	mux.HandleFunc("/insights/audit/duplicates", a.handleRunDuplicateAudit)
	mux.HandleFunc("/export", a.handleLegacyExportRedirect)
	mux.HandleFunc("/settings", a.handleSettings)
	mux.HandleFunc("/settings/initialize", a.handleSettingsInitialize)
	mux.HandleFunc("/settings/updates/source", a.handleUpdateSource)
	mux.HandleFunc("/settings/updates/check", a.handleCheckForUpdates)
	mux.HandleFunc("/settings/updates/apply", a.handleApplyLatestUpdate)
	mux.HandleFunc("/settings/updates/health/bootstrap", a.handleUpdateBootstrapHealth)
	mux.HandleFunc("/settings/images/orphans/scan", a.handleScanImageOrphans)
	mux.HandleFunc("/settings/images/orphans/cleanup", a.handleCleanupImageOrphans)
	mux.HandleFunc("/settings/quality/scan", a.handleScanDataQuality)
	mux.HandleFunc("/settings/quality/apply", a.handleApplyDataQuality)
	mux.HandleFunc("/export/json", a.handleExportJSON)
	mux.HandleFunc("/export/csv", a.handleExportCSV)
	mux.HandleFunc("/export/ical", a.handleExportICalendar)
	mux.HandleFunc("/export/static-archive", a.handleExportStaticArchive)
	mux.HandleFunc("/export/database-pdf", a.handleExportDatabasePDF)
	mux.HandleFunc("/export/backup", a.handleExportBackup)
	mux.HandleFunc("/export/shared-archive", a.handleExportSharedArchive)
	mux.HandleFunc("/export/bug-report", a.handleExportBugReport)
	mux.HandleFunc("/export/feedback-log", a.handleExportFeedbackLog)
	mux.HandleFunc("/insights/report/pdf", a.handleExportInsightsPDF)
	mux.HandleFunc("/import/backup", a.handleImportBackup)
	mux.HandleFunc("/import/shared-archive", a.handleImportSharedArchive)
	mux.HandleFunc("/import/memorial-json/preview", a.handlePreviewMemorialJSONImport)
	mux.HandleFunc("/import/memorial-json/confirm", a.handleConfirmMemorialJSONImport)
	mux.HandleFunc("/merge-review/", a.handleMergeReviewConflict)
	mux.HandleFunc("/integrations/google/connect", a.handleGoogleConnect)
	mux.HandleFunc("/integrations/google/disconnect", a.handleGoogleDisconnect)
	mux.HandleFunc("/integrations/google/backup", a.handleGoogleBackup)
	mux.HandleFunc("/integrations/google/sheets/export", a.handleGoogleSheetsExport)
	mux.HandleFunc("/integrations/google/calendar/use-managed", a.handleGoogleCalendarUseManaged)
	mux.HandleFunc("/integrations/google/calendar/preferences/save", a.handleGoogleCalendarPreferencesSave)
	mux.HandleFunc("/integrations/google/calendar/sync-managed", a.handleGoogleCalendarSyncManaged)
	mux.HandleFunc("/integrations/google/calendar/unsync-managed", a.handleGoogleCalendarUnsyncManaged)
	mux.HandleFunc("/integrations/google/calendar/use-test", a.handleGoogleCalendarUseTest)
	mux.HandleFunc("/integrations/google/calendar/sync-test", a.handleGoogleCalendarSyncTest)
	mux.HandleFunc("/integrations/google/calendar/unsync-test", a.handleGoogleCalendarUnsyncTest)
	mux.HandleFunc("/images/screenshot", a.handleImageScreenshot)
	mux.HandleFunc("/images/rotate", a.handleImageRotate)
	mux.HandleFunc("/open-link", a.handleOpenLink)
	mux.HandleFunc("/feedback/submit", a.handleFeedbackSubmit)
	mux.HandleFunc("/scratchpad/open", a.handleScratchpadOpen)
	mux.HandleFunc("/media/", a.handleMedia)

	// Phase 4: debug endpoints (state + client-logs + toggle).
	mux.HandleFunc("/debug/state", a.handleDebugState)
	mux.HandleFunc("/debug/client-logs", a.handleClientLogs)
	mux.HandleFunc("/settings/debug-mode", a.handleDebugModeToggle)

	// Phase 6: console + folder + clear (handlers defined in debug_handlers.go).
	mux.HandleFunc("/debug/console", a.handleDebugConsole)
	mux.HandleFunc("/debug/console/tail", a.handleDebugConsoleTail)
	mux.HandleFunc("/debug/console/clear", a.handleDebugConsoleClear)
	mux.HandleFunc("/debug/open-folder", a.handleDebugOpenFolder)

	a.muxRaw = mux
	// debug.Middleware is OUTERMOST so the request_id it generates is on
	// the context before recover runs (the crash log line carries it).
	a.mux = debug.Middleware(recoverMiddleware(mux))
}
