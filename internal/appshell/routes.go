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
	// Issue #391 Slice B.1: dedicated chi route for
	// /soldiers/{id}/images (soldier-side fragment-swap
	// spine, mirroring event-side #332). MUST register
	// before the /soldiers/* wildcard below — the
	// regex route is more specific, so chi matches it
	// first when both patterns cover the same path.
	// Per-card Delete + Set-Primary forms land in B.2.
	r.Get("/soldiers/{id:[0-9]+}/images", a.handleSoldierImagesRoute)
	// Issue #416: per-row "Generate Display ID" affordance on
	// the data-quality scan results. Dedicated chi route so
	// the per-row POST has a stable URL; the picker-guard
	// doesn't apply here (this is an admin/recovery path,
	// not a foldout entry).
	r.Post("/soldiers/{id:[0-9]+}/display-id/recover", a.handleRecoverDisplayID)
	// Issue #368 slice 2: PATCH endpoint for reordering a
	// Source Record within a Person Record. Top-level chi
	// route (not a sub-dispatch in handleSoldierByID) so
	// the picker-guard doesn't apply and the method is
	// method-specific (chi 405s other methods automatically).
	r.Patch("/soldiers/{id:[0-9]+}/sources/{sourceId:[0-9]+}/position", a.handleMoveSoldierSource)
	r.Post("/browse/bulk-tag", a.handleBulkTagFromBrowse)
	r.Get("/soldiers/*", a.handleSoldierByID)
	r.Post("/soldiers/*", a.handleSoldierByID)
	r.Put("/soldiers/*", a.handleSoldierByID)
	r.Delete("/soldiers/*", a.handleSoldierByID)
	// v62 (issue #321 slice 1): Article Record routes.
	// Registered between the /soldiers catch-alls and the
	// /events routes so the literal /articles and /articles/{id}
	// prefixes match before any catch-all. The slice-1 surface
	// is the Get + Post alias on /articles/{id:[0-9]+}; the
	// /edit, /refs/*, /snapshot, /restore, /pdf, /raw paths
	// land in slices 2.5 / 3 / 4. The /articles/* catch-all is
	// deliberately omitted in slice 1 — adding it would let the
	// orphan-handler probe miss a future missing-invoker route
	// (the same anti-pattern issue #257 sweeps).
	r.Get("/articles", a.handleArticles)
	r.Get("/articles/new", a.handleNewArticle)
	r.Post("/articles/new", a.handleNewArticle)
	r.Get("/articles/{id:[0-9]+}", a.handleArticleByID)
	r.Post("/articles/{id:[0-9]+}", a.handleArticleByID)
	// v62 slice 2 (issue #321): ref attach/detach routes.
	// Picker UI (slice 3) posts to /articles/{id}/refs; the
	// inline Refs panel renders a Delete button that posts
	// (via hx-delete) to /articles/{id}/refs/{personId}.
	// Idempotent detach so a UI double-click is a no-op.
	r.Post("/articles/{id:[0-9]+}/refs", a.handleArticleRefsAttach)
	r.Delete("/articles/{id:[0-9]+}/refs/{personId:[0-9]+}", a.handleArticleRefsDetach)
	// v62 slice 2.5 (issue #321): snapshot lifecycle.
	// POST /articles/{id}/snapshot creates a fresh row with
	// is_snapshot = 1 + a new ART-NNNNN Display ID. POST
	// /articles/{id}/restore overwrites the live row the
	// snapshot refers to. DELETE /articles/{id}/snapshot/
	// {snapshotID} removes only the snapshot row.
	// v62 slice 3.3 (issue #321): picker modal. The picker
	// renders an inline panel (search + results) that
	// posts to the existing /articles/{id}/refs attach
	// route. No native dialog -- uses an htmx-swapped
	// fragment per docs/agents/dialog-guard.md.
	r.Get("/articles/{id:[0-9]+}/picker", a.handleArticlePicker)
	// v62 slice 3.4 (issue #321): Revisions tab fragment.
	// The fragment lists every snapshot pointing at the
	// live article, with per-snapshot Restore + Delete
	// affordances. Renders only the <ul> + actions; the
	// tab UI lives on the article_detail.templ shell so
	// the fragment is a drop-in for the tab's data panel.
	r.Get("/articles/{id:[0-9]+}/revisions", a.handleArticleRevisions)
	// v62 slice 3.5 (issue #321): /articles/{id}/edit route
	// shell. GET renders the minimal form (slice 3.7 swaps
	// in the full markdown editor); POST calls Update via
	// the existing slice-2 path with 404 + 400 + 409
	// error mappings. Slice 3.5 deliberately ships the
	// minimal round-trip surface before the editor UX lands.
	r.Get("/articles/{id:[0-9]+}/edit", a.handleEditArticle)
	r.Post("/articles/{id:[0-9]+}/edit", a.handleEditArticle)
	// v62 slice 3.6 (issue #321): live preview endpoint.
	// POST /articles/preview takes a form-encoded body
	// field and returns the sanitized HTML render. The
	// editor's preview pane POSTs on every keystroke
	// (250ms debounce) and swaps the response innerHTML.
	// Registered at /articles/preview (no id; the
	// endpoint is editor-scoped, not article-scoped).
	r.Post("/articles/preview", a.handleArticlePreview)
	// v62 slice 4.2 (issue #321): PDF download + raw-md
	// download. POST /articles/{id}/pdf opens a guarded
	// SaveFileDialog and writes the pre-rendered PDF body
	// to the user's chosen path. GET /articles/{id}/raw
	// returns the body_md verbatim with text/markdown
	// + Content-Disposition: attachment.
	r.Post("/articles/{id:[0-9]+}/pdf", a.handleArticlePDF)
	r.Get("/articles/{id:[0-9]+}/raw", a.handleArticleRaw)
	r.Post("/articles/{id:[0-9]+}/snapshot", a.handleArticleSnapshot)
	r.Post("/articles/{id:[0-9]+}/restore", a.handleArticleRestore)
	r.Delete("/articles/{id:[0-9]+}/snapshot/{snapshotID:[0-9]+}", a.handleArticleSnapshotDelete)
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
	// Issue #320 + #343 finding #3: per-Event panel routes are
	// dispatched by handleEventPanelRoute (appshell/event_panel.go)
	// through the eventPanels registry. Each chi route below
	// covers one panel name + its (method, sub-path) variants;
	// adding a new panel = one chi route + one registry entry.
	// The collapse replaces four near-identical dispatcher shims
	// (research-log, sources, tags, images) with one shared
	// handler that walks a (panel, method, subPath) → handler
	// table. The architecture-review body flagged the prior
	// shape as a deletion-test signal: "delete the dispatcher,
	// do the panels keep working? Yes. The handler functions
	// are the real work; the dispatcher is glue."
	r.Route("/events/{id:[0-9]+}/research-log", func(r chi.Router) {
		r.Get("/*", a.handleEventPanelRoute)
		r.Post("/*", a.handleEventPanelRoute)
	})
	r.Route("/events/{id:[0-9]+}/sources", func(r chi.Router) {
		// GET /events/{id}/sources + POST /sources/{id}/detach funnel through
		// the dispatcher; PATCH /sources/{sourceId}/position stays
		// registered as a top-level handler. POST /sources/attach was
		// deleted in issue #380 slice 6 (stale bookmark-compat remnant,
		// no remaining callers).
		// on its dedicated handler (handleMoveEventSource is a
		// separate concern, not a "panel" route).
		r.Get("/*", a.handleEventPanelRoute)
		r.Post("/*", a.handleEventPanelRoute)
	})
	// Issue #368 slice 2: PATCH endpoint for reordering an
	// Event Source. Same shape as the soldier-side route.
	r.Patch("/events/{id:[0-9]+}/sources/{sourceId:[0-9]+}/position", a.handleMoveEventSource)
	r.Route("/events/{id:[0-9]+}/tags", func(r chi.Router) {
		r.Get("/*", a.handleEventPanelRoute)
		r.Post("/*", a.handleEventPanelRoute)
	})
	// Issue #361 slice 2: Event editor's Linked Persons section.
	// POST /events/{id}/links takes a `display_id` form field
	// and resolves it to a Person Record ID via the new
	// LookupPersonIDByDisplayID helper, then delegates to the
	// existing AttachEventToPerson. POST /events/{id}/links/
	// {personId}/detach delegates to DetachEventFromPerson.
	// Both respond with X-DixieData-Redirect pointing at
	// /events/{id}/edit so the user lands back on the editor
	// (not the detail page) after attaching/detaching.
	r.Post("/events/{id:[0-9]+}/links", a.handleEventLinksAttachRoute)
	r.Post("/events/{id:[0-9]+}/links/{personId:[0-9]+}/detach", a.handleEventLinksDetachRoute)
	r.Route("/events/{id:[0-9]+}/images", func(r chi.Router) {
		r.Get("/*", a.handleEventPanelRoute)
		r.Post("/*", a.handleEventPanelRoute)
	})
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
	r.Post("/research-collections", a.handleResearchCollections)
	r.Get("/research-collections/*", a.handleResearchCollectionByID)
	r.Post("/research-collections/*", a.handleResearchCollectionByID)
	// Issue #378 slice 1: Research & Review Person picker landing
	// + select/clear cookie actions. The /research route MUST
	// register before any /soldiers/* catch-all (see
	// TestRouteOrderResearchBeatsSoldiersWildcard) — chi resolves
	// the literal pattern first when both could match, but
	// documenting the order here so a future agent who reorders
	// the routes file lands on this constraint.
	r.Get("/research", a.handleResearchPicker)
	r.Post("/research/select", a.handleResearchSelect)
	r.Post("/research/clear", a.handleResearchClear)
	// Issue #378 slice 3 placeholder: /research/recent fragment
	// endpoint registered here so the routebuilder is stable at
	// slice 2 land; the handler returns an empty recents list
	// until slice 3 wires the localStorage hydration.
	r.Get("/research/recent", a.handleResearchRecent)

	r.Get("/insights", a.handleInsights)
	r.Get("/insights/drilldown", a.handleInsightsDrilldown)
	r.Post("/insights/audit/duplicates", a.handleRunDuplicateAudit)

	r.Get("/export", a.handleLegacyExportRedirect)
	r.Get("/settings", a.handleSettings)
	r.Post("/settings/theme", a.handleSettingsTheme)
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