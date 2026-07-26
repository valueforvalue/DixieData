package appshell

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"io/fs"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "embed"
	"github.com/valueforvalue/DixieData/internal/appdata"
	"github.com/valueforvalue/DixieData/internal/archive"
	"github.com/valueforvalue/DixieData/internal/confederatehomestatus"
	"github.com/valueforvalue/DixieData/internal/config"
	"github.com/valueforvalue/DixieData/internal/dates"
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/debug"
	"github.com/valueforvalue/DixieData/internal/debug/trace"
	"github.com/valueforvalue/DixieData/internal/integrations"
	"github.com/valueforvalue/DixieData/internal/jobs"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/pensionstate"
	"github.com/valueforvalue/DixieData/internal/presentation"
	"github.com/valueforvalue/DixieData/internal/records"
	"github.com/valueforvalue/DixieData/internal/scratchpad"
	"github.com/valueforvalue/DixieData/internal/supportuploader"
	"github.com/valueforvalue/DixieData/internal/templates"
	"github.com/valueforvalue/DixieData/internal/update"
	"github.com/valueforvalue/DixieData/internal/viewmodel"
	"github.com/valueforvalue/DixieData/pkg/render"
	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/versioninfo"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed quotes.json
var embeddedQuotes []byte

// App is the DixieData application shell: it owns the SQLite database
// connection, every service facade (backup, export, research, settings,
// etc.), the htmx + REST request router, the frontend-asset filesystem,
// and the in-process job registry. Constructed by NewApp, configured
// by WithFrontendAssets, and started by Startup. Every UI handler in
// internal/appshell is a method on *App; the Wails runtime and the
// CLI runner both receive an *App.
//
// App is the central seam between the Wails frontend, the headless CLI
// runner, and the domain layer. Nothing in internal/{records,models,
// viewmodel,archive,...} imports appshell; appshell imports them.
type App struct {
	// cfg is the user-tunable application configuration loaded
	// from config.json at startup (issues #636-#639). Missing
	// file → Defaults(). Read-only after Startup; handlers
	// read from a.cfg.*, not from hard-coded constants.
	cfg         config.Config
	ctx         context.Context
	database    *db.DB
	soldiers    *records.SoldierService
	anniversary *records.AnniversaryService
	// v60 (issue #320): Event Record service. Wired in
	// reloadServices() alongside soldiers (it borrows the same
	// database handle). Handlers route Event-only operations
	// through a.events, not a.soldiers, so the focused facade
	// stays decoupled from the full SoldierService surface.
	// Issue #343 finding #4: facade interfaces deleted; the App
	// holds the concrete service types directly per the
	// two-adapter rule (interfaces live at the consumer).
	events *records.EventService
	// v62 (issue #321): Article Record service. Wired in
	// reloadServices() alongside soldiers + events. The slice-1
	// surface is minimal (Create + GetByID); the facade debate
	// (#343 candidate #4) deliberately deferred, so Article
	// stays direct for v1.
	articles                        *records.ArticleService
	calendar                        *records.CalendarService
	analytics                       *records.AnalyticsService
	audit                           *records.AuditService
	exportTemplates                 *records.ExportTemplateService
	shareQueuePresets               *records.ShareQueuePresetService
	tags                            *records.TagService
	archiveMeta                     *records.ArchiveMetaService
	images                          *archive.ImageService
	export                          *archive.ExportService
	backup                          *archive.BackupService
	diagnostics                     *archive.DiagnosticsService
	google                          *integrations.GoogleService
	updater                         *update.Service
	// updateProgress is the thread-safe progress state the apply
	// handler writes to from its prepare goroutine; the
	// /settings/updates/progress GET endpoint reads it on each
	// poll to render the live progress fragment (issue #661).
	updateProgress                  *updateProgressState
	restorePoints                   *update.RestorePointManager
	quotes                          []models.Quote
	mux                             http.Handler
	muxRaw                          *http.ServeMux
	saveFileDialogOverride          func(opts any) (string, error)
	openFileDialogOverride          func(opts any) (string, error)
	openMultipleFilesDialogOverride func(opts any) ([]string, error)
	openDirectoryDialogOverride     func(opts any) (string, error)
	browserOpenURLOverride          func(rawURL string) error
	manualCallbacks                 sync.Map // map[string]*manualCallbackEntry — release/cancel callbacks for jobs.Registry.StartManual confirm-before-run jobs
	startupErr                      error
	setupRequired                   bool
	debugMode                       atomic.Bool  // Phase 4: gated by DIXIEDATA_DEBUG=1 or settings toggle
	theme                           atomic.Value // string; resolved theme name (default/high-contrast/soft). Issue #474.
	// exportSurface (issue #534) is the per-user preference for
	// what to do after a successful export: "jobs-page" (the
	// historical default -- navigate to /jobs/{id} via the
	// dispatcher's X-DixieData-Redirect header) or "toast-only"
	// (stay on the source page + show a toast -- the job still
	// runs, the user just doesn't navigate). Stored as a string
	// matching records.LocalSettings.ExportSurface so the
	// dispatcher + the settings page + the per-request
	// <html data-export-surface> attribute all read the same
	// value. The default-empty case resolves to "jobs-page"
	// via records.ResolvedExportSurface so fresh installs keep
	// today's behavior.
	exportSurface           atomic.Value
	pendingLaunchStateClear bool
	pendingRecovery         *update.RestorePointRecord
	recoveryFailure         string
	dataDir                 string
	scratchpads             scratchpadOpener
	frontendAssets          fs.FS
	jobs                    *jobs.Registry
}

func shouldAttemptPostUpdateHealthClear(r *http.Request) bool {
	if r == nil || r.Method != http.MethodGet || r.URL == nil {
		return false
	}
	return isPostUpdateHealthTrustPath(r.URL.Path)
}

func isPostUpdateHealthTrustPath(path string) bool {
	switch {
	case path == "/", path == "/calendar":
		return true
	case strings.HasPrefix(path, "/browse"):
		return true
	case strings.HasPrefix(path, "/settings"):
		return true
	case strings.HasPrefix(path, "/insights"):
		return true
	case strings.HasPrefix(path, "/soldiers"):
		return true
	case strings.HasPrefix(path, "/review-queue"):
		return true
	case strings.HasPrefix(path, "/research-collections"):
		return true
	case strings.HasPrefix(path, "/export"):
		return true
	case strings.HasPrefix(path, "/compare"):
		return true
	default:
		return false
	}
}

func (a *App) clearPendingLaunchState() error {
	if !a.pendingLaunchStateClear {
		return nil
	}
	if a.restorePoints == nil {
		return fmt.Errorf("restore point manager unavailable")
	}
	if err := a.restorePoints.ClearLaunchState(); err != nil {
		return fmt.Errorf("failed to clear restore point launch state: %w", err)
	}
	a.pendingLaunchStateClear = false
	return nil
}

// inFlightEntry is the value stored under each dedup key. Retained
// as a marker type for test files that reference it; the inFlight
// sync.Map was replaced by a.jobs.TryClaim (issue #615).
type inFlightEntry struct {
	JobID string
}

// guardDialog wraps a.jobs.TryClaim for the native-dialog dedup
// guard. Returns a release function and true if the caller is the
// active owner. When false, another caller already holds the key
// (second click on an export button while the dialog is still open).
// Falls back to a package-level sync.Map when a.jobs is nil (test
// harnesses that create App without initializing the jobs registry).
func (a *App) guardDialog(dupKey string) (release func(), admitted bool) {
	if a.jobs == nil {
		var done atomic.Bool
		if _, loaded := guardFallback.LoadOrStore(dupKey, struct{}{}); loaded {
			return nil, false
		}
		release = func() {
			if done.CompareAndSwap(false, true) {
				guardFallback.Delete(dupKey)
			}
		}
		return release, true
	}
	release, ok := a.jobs.TryClaim(dupKey)
	return release, ok
}

// guardFallback is the in-flight dedup map used when a.jobs is nil
// (test-only path — production always has a.jobs set).
var guardFallback sync.Map

// rememberManualCallback stores the release/cancel callbacks for a
// jobs.Registry.StartManual confirm-before-run job in a sync.Map
// keyed by job ID. The /jobs/{id}/confirm and /jobs/{id}/cancel
// endpoints look these up. Entries are kept until the job
// transitions to a terminal status (worker goroutine clears it
// on exit). /confirm and /cancel return jobs.ErrAlreadyTerminal
// semantics if the entry has been cleared.
func (a *App) rememberManualCallback(jobID string, release func() error, cancel func() error) {
	a.manualCallbacks.Store(jobID, &manualCallbackEntry{release: release, cancel: cancel})
}

// forgetManualCallback drops the callbacks after the job reaches a
// terminal status, so /confirm and /cancel can return a meaningful
// "already terminal" response. Safe to call multiple times.
func (a *App) forgetManualCallback(jobID string) {
	a.manualCallbacks.Delete(jobID)
}

// releaseManualCallback invokes the StartManual release callback for
// the given job ID, transitioning it from StatusQueued to
// StatusRunning. Returns jobs.ErrNotFound if no manual-job entry
// exists (e.g. the job was already released or never was manual).
func (a *App) releaseManualCallback(jobID string) error {
	v, ok := a.manualCallbacks.Load(jobID)
	if !ok {
		return jobs.ErrNotFound
	}
	entry := v.(*manualCallbackEntry)
	return entry.release()
}

// cancelManualCallback invokes the StartManual cancel callback for the
// given job ID, flipping StatusQueued to StatusCancelled without
// running the worker.
func (a *App) cancelManualCallback(jobID string) error {
	v, ok := a.manualCallbacks.Load(jobID)
	if !ok {
		// Not a manual job (or already terminal). Fall through to
		// the registry's standard Cancel path.
		return a.jobs.Cancel(jobID)
	}
	entry := v.(*manualCallbackEntry)
	return entry.cancel()
}

// manualCallbackEntry bundles the two callbacks a StartManual job
// exposes. The release/cancel fields are exactly-once; calling
// them twice returns jobs.ErrAlreadyTerminal from the registry.
type manualCallbackEntry struct {
	release func() error
	cancel  func() error
}

// respondDuplicateInFlight writes the response for a duplicate
// request that hit the in-flight guard. When a background job
// has already been started under dupKey, redirect the user to
// its /jobs/{id} status page so they can watch the real job
// progress (the legacy error page left them stranded). When no
// JobID exists yet (the save dialog is still open), redirect
// back to the originating page with a toast so the user's
// modal/page stays put instead of being replaced by an error
// body.
func (a *App) respondDuplicateInFlight(w http.ResponseWriter, r *http.Request, dupKey string) {
	// Issue #615: with TryClaim, the claim itself is the guard.
	// A duplicate request that arrives while the dialog is still
	// open gets a friendly toast on the current page. The legacy
	// JobID redirect was dropped because TryClaim does not carry
	// a JobID — the claim key alone is sufficient for dedup.
	debug.FromContext(r.Context()).Debug("duplicate request rejected", "dupKey", dupKey)
	// Dialog is still open — bounce the user back to their page so
	// the modal is not replaced by an error body. Option C: 200 +
	// X-DixieData-Redirect so dispatchDixieDataForm navigates
	// client-side without reloading the modal form. Fall back to "/"
	// if the referrer is absent or off-origin.
	redirectTo := "/"
	if r != nil {
		if referer := strings.TrimSpace(r.Referer()); referer != "" {
			if u, err := url.Parse(referer); err == nil && u.Path != "" {
				redirectTo = u.Path
			}
		}
	}
	setToastHeaderWithType(w, "Export already in progress; please wait for the save dialog.", "info")
	writeExportRedirect(w, redirectTo)
}

func (a *App) handleUpdateBootstrapHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !a.pendingLaunchStateClear {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if a.restorePoints == nil {
		respondUnavailable(w, r, "Restore point manager unavailable. Update bootstrap cannot run.", nil)
		return
	}
	if err := a.clearPendingLaunchState(); err != nil {
		respondInternal(w, r, "Could not clear the pending launch state.", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type scratchpadOpener interface {
	Open(displayID, seed string) error
}

const initializeDataConfirmationWord = "INITIALIZE"

func renderStartupPlaceholder(a *App, w http.ResponseWriter, r *http.Request) {
	// When the request is an htmx fragment (polling job progress,
	// review counts, etc.) during the pre-mux window, return 204
	// instead of a full HTML document.  Without this guard the
	// placeholder's <body> (with hx-target="body" hx-swap="outerHTML")
	// gets innerHTML-swapped into a small target region, its body
	// triggers fire, and each response stacks another placeholder
	// body — the cascading reload bug (uibug.png / uibug2.png).
	// See blockIfFragment in fragment_guard.go for the contract;
	// this branch is just the "no redirect hint" case (pre-mux
	// has no destination to hint at).
	if blockIfFragment(w, r, "") {
		return
	}

	target := "/calendar"
	if r != nil && r.URL != nil {
		if requestPath := strings.TrimSpace(r.URL.RequestURI()); requestPath != "" && requestPath != "/" {
			target = requestPath
		}
	}
	retryTarget := startupPlaceholderRetryTarget(target)
	targetJS, err := json.Marshal(retryTarget)
	if err != nil {
		targetJS = []byte(`"/calendar?_dd_boot=1"`)
	}
	// Issue #481: stamp the resolved theme on the placeholder's
	// <html data-theme="..."> so the user's chosen palette applies
	// during the ~700ms pre-mux loading window. The pre-#481
	// behavior emitted no data-theme attribute, so High Contrast /
	// Soft users saw a flash of the gold/sepia Default-theme
	// loading card before the real /calendar page rendered with
	// the right theme. The atomic load is nil-safe — a fresh App
	// (no Startup yet, the WebView2 first-paint race) falls back
	// to ThemeDefault, matching the lifecycle.go:478-490 pattern.
	//
	// Issue #481 follow-up (theme cold-start race): the #481 fix
	// only worked once Startup() had stored the resolved theme in
	// a.theme. But the Wails WebView2 can fire its first request
	// BEFORE Startup() runs, leaving a.theme nil. The old fallback
	// hardcoded ThemeDefault even when the user persisted
	// theme="high-contrast" on disk — so High Contrast / Soft
	// users saw a flash of the Default loading card on every cold
	// launch until the 700ms JS redirect hit the warmed mux. The
	// fix: when the atomic is nil (the cold-start race window),
	// backfill from local_settings.json via resolvedBootTheme
	// (shared with /boot-theme.js). Disk read happens only during
	// this window; once Startup() stores the atomic, every later
	// request (including the placeholder's own refresh) hits the
	// in-memory path. nil dataDir (a truly fresh NewApp() with no
	// settings file) and load errors fall through to ThemeDefault.
	theme := resolvedBootTheme(a)
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("Refresh", fmt.Sprintf("1; url=%s", retryTarget))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusAccepted)
	// Issue #481 follow-up: the placeholder card colors now flow
	// through var(--theme-*) tokens (body bg uses the theme's page
	// gradient stops; card uses --theme-bg-card + --theme-accent-strong;
	// text uses --theme-text-primary / muted / accent) so the
	// loading card matches the active theme instead of hardcoding
	// the Default-theme gold/sepia hex values that previously
	// overrode the user's chosen theme during the pre-mux flash.
	fmt.Fprintf(w, `<!doctype html>
<html lang="en" data-theme="%s">
<head>
<meta charset="utf-8">
<meta http-equiv="refresh" content="1;url=%s">
<meta http-equiv="cache-control" content="no-cache, no-store, must-revalidate">
<meta http-equiv="pragma" content="no-cache">
<meta http-equiv="expires" content="0">
<title>Loading DixieData...</title>
</head>
<body class="min-h-screen" style="background: linear-gradient(180deg, var(--theme-bg-page-top) 0%%, var(--theme-bg-page-mid) 42%%, var(--theme-bg-page-bottom) 100%%);">
<div class="flex min-h-screen items-center justify-center px-6">
  <div class="rounded-3xl border border-[var(--theme-accent-strong)] bg-[var(--theme-bg-card)] px-8 py-6 shadow-[0_18px_34px_rgba(21,29,38,0.2)]">
    <p class="mb-2 text-sm uppercase tracking-[0.24em] text-[var(--theme-accent)]">Local Archive</p>
    <p class="text-2xl font-semibold text-[var(--theme-text-primary)]">Loading DixieData...</p>
    <p class="mt-2 text-sm text-[var(--theme-text-muted)]">The local archive is still starting up. This screen will refresh automatically.</p>
  </div>
</div>
<script>
window.setTimeout(function() {
  window.location.replace(%s);
}, 700);
</script>
</body>
</html>`, theme, html.EscapeString(retryTarget), string(targetJS))
}

func startupPlaceholderRetryTarget(target string) string {
	parsed, err := url.Parse(target)
	if err != nil {
		return "/calendar?_dd_boot=1"
	}
	query := parsed.Query()
	query.Set("_dd_boot", "1")
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func setupRequestAllowed(path string) bool {
	switch {
	case path == "/setup":
		return true
	case path == "/version":
		return true
	case path == "/app.js":
		return true
	case path == "/app.css":
		return true
	case path == "/htmx.min.js":
		return true
	case path == "/debug.js":
		return true
	case path == "/jobs/active":
		return true
	case path == "/layout/review-count":
		// /layout/review-count is the Review Queue badge fragment
		// polled by the layout every 30s (layout.templ). When setup
		// is required, the badge's poll would 204 + X-DixieData-Redirect
		// to /setup, and the global htmx:afterRequest listener
		// (app.js) would window.location.assign("/setup") — the user
		// is already on /setup, so this reloads the page on every
		// 30s poll and Chromium's IPC flood protection eventually
		// throttles navigation. Allowing the path through here
		// keeps the badge poll quiet (handler returns empty 200)
		// while the user completes setup. See issue: /setup mouse
		// jitter caused by layout badge reload loop.
		return true
	case strings.HasPrefix(path, "/jobs/") && strings.HasSuffix(path, "/status"):
		// /jobs/{id}/status is the polling fragment. When setup is
		// required, no jobs exist, but the layout progress slot
		// still polls every 3s. Without this allowlist, every poll
		// 303s to /setup, the browser follows the redirect, and the
		// full setup HTML document gets innerHTML-swapped into the
		// progress region — which then re-fires its own load
		// trigger and stacks the layout. See the bug report
		// `uibug.jpg` in the repo root.
		return true
	case strings.HasPrefix(path, "/wailsjs/"):
		return true
	default:
		return false
	}
}

func recoveryRequestAllowed(path string) bool {
	switch {
	case path == "/recovery":
		return true
	case path == "/version":
		return true
	case path == "/app.js":
		return true
	case path == "/app.css":
		return true
	default:
		return false
	}
}

func requestMethodOverride(r *http.Request) string {
	if r == nil || r.Method != http.MethodPost {
		return ""
	}
	if override := normalizedMethodOverride(r.Header.Get("X-HTTP-Method-Override")); override != "" {
		return override
	}
	if err := parseRequestFormForOverride(r); err == nil {
		if override := normalizedMethodOverride(r.FormValue("_method")); override != "" {
			return override
		}
	}
	return ""
}

func parseRequestFormForOverride(r *http.Request) error {
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		return r.ParseMultipartForm(64 << 20)
	}
	return r.ParseForm()
}

func normalizedMethodOverride(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case http.MethodPut:
		return http.MethodPut
	case http.MethodDelete:
		return http.MethodDelete
	case http.MethodPatch:
		return http.MethodPatch
	default:
		return ""
	}
}

func (a *App) handleShare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Issue #284: the /share landing slimmed to a
	// sub-overview. The Export & Backup, Import & Restore,
	// and Google Integration surfaces moved to
	// /share/exports, /share/imports, /share/sync
	// respectively; each subpage handler loads only its
	// own data. The landing only needs the merge-review
	// conflicts, archive counts (for the empty-state), and
	// the recent-jobs list (for the activity card).
	conflicts, err := a.backup.PendingMergeConflicts()
	if err != nil {
		respondInternal(w, r, "Could not load pending merge conflicts.", err)
		return
	}
	domainCounts, err := a.soldiers.ArchiveCounts()
	if err != nil {
		respondInternal(w, r, "Could not load archive counts.", err)
		return
	}
	// Issue #265: surface the last 3 terminal jobs in the
	// /share landing's "Recent activity" card. The registry
	// is optional in tests + the headless CLI, so guard the
	// nil case (an empty list still renders the empty-state
	// copy in the section).
	var recentJobs []viewmodel.RecentJobEntry
	if a.jobs != nil {
		recentJobs = buildRecentJobEntries(a.jobs.RecentJobs(3))
	}
	if err := presentation.ShareView(conflicts, domainCounts, recentJobs).Render(r.Context(), w); err != nil {
		// Issue #384 / Slice 1: bare Render used to silently swallow
		// the failure — the user saw a half-rendered page. Fall back
		// to respondErrorFragment so the toast fires AND the body
		// shows an EmptyStateError instead of an empty response.
		respondErrorFragment(w, r, KindInternal, "Could not render the share view.", err)
	}
}

// buildRecentJobEntries converts the jobs.Registry output
// (jobs.Job) into the viewmodel shape the /share landing
// renders. Kept as a standalone helper (not a method on
// App) because the conversion is pure data — no app state
// involved, no I/O. The function is package-private to
// app.go because the conversion only matters for the
// ShareView call site; if a future handler needs the
// same shape, lift it into viewmodel.
func buildRecentJobEntries(jobsList []jobs.Job) []viewmodel.RecentJobEntry {
	out := make([]viewmodel.RecentJobEntry, 0, len(jobsList))
	for _, job := range jobsList {
		out = append(out, viewmodel.RecentJobEntry{
			ID:            job.ID,
			Kind:          job.Kind,
			KindLabel:     job.DisplayLabel(),
			ActivityGroup: jobs.ActivityGroupFor(job.Kind),
			Status:        job.Status,
			StatusLabel:   statusLabelFor(job.Status),
			Message:       job.Message,
			ResultPath:    job.ResultPath,
			StartedAt:     job.StartedAt.UTC().Format(time.RFC3339),
			FinishedAt:    job.FinishedAt.UTC().Format(time.RFC3339),
			DetailURL:     "/jobs/" + job.ID,
		})
	}
	return out
}

// statusLabelFor returns the human label for a terminal
// job status. Matches the pill class keys in
// components.RecentJobs. Kept inline because it has one
// caller and lifts to a method only if a second caller
// shows up.
func statusLabelFor(status string) string {
	switch status {
	case jobs.StatusDone:
		return "Done"
	case jobs.StatusError:
		return "Error"
	case jobs.StatusCancelled:
		return "Cancelled"
	case jobs.StatusInterrupted:
		return "Interrupted"
	default:
		return "Unknown"
	}
}

func (a *App) handleResearchCollections(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		fromID, err := parseOptionalInt64(r.URL.Query().Get("from"), "from")
		if err != nil {
			respondValidation(w, r, "Invalid from id.", err)
			return
		}
		// Stale "?from=<id>" (soldier deleted, bookmark, merged archive) must
		// not 500 the whole hub. SoldierService.ResearchCollectionsHub returns
		// the GetByID error verbatim when the soldier is missing, so we try
		// the hub with fromID, and on a sql.ErrNoRows-style missing-row
		// failure we fall back to fromID=0 (no current context) instead of
		// returning 500 "Could not load research collections." (issue #452 follow-up).
		hub, err := a.soldiers.ResearchCollectionsHub(fromID)
		if err != nil && fromID > 0 {
			if _, lookupErr := a.soldiers.GetByID(fromID); lookupErr != nil && errors.Is(lookupErr, sql.ErrNoRows) {
				if hub, err = a.soldiers.ResearchCollectionsHub(0); err != nil {
					respondInternal(w, r, "Could not load research collections.", err)
					return
				}
			} else {
				respondInternal(w, r, "Could not load research collections.", err)
				return
			}
		} else if err != nil {
			respondInternal(w, r, "Could not load research collections.", err)
			return
		}
		// Issue #384 / Slice 1: wrap the Render call so a templ failure
		// surfaces an EmptyStateError fragment instead of an empty body.
		if err := presentation.ResearchCollectionsHubView(*hub).Render(r.Context(), w); err != nil {
			respondErrorFragment(w, r, KindInternal, "Could not render the research collections hub.", err)
		}
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			respondValidation(w, r, "Could not read the collection form.", err)
			return
		}
		if err := a.soldiers.CreateResearchCollection(r.FormValue("name"), r.FormValue("description")); err != nil {
			setToastHeaderWithType(w, "Collection could not be created.", "error")
			respondInternal(w, r, "Could not create the research collection.", err)
			return
		}
		redirectTo := "/research-collections"
		if fromID, err := parseOptionalInt64(r.FormValue("from"), "from"); err == nil && fromID > 0 {
			redirectTo = fmt.Sprintf("/research-collections?from=%d", fromID)
		}
		setToastHeader(w, "Success: research collection created.")
		w.Header().Set("X-DixieData-Redirect", redirectTo)
		fmt.Fprint(w, "Collection created.")
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) handleResearchCollectionByID(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/research-collections/"), "/")
	if path == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(path, "/")
	collectionID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		respondValidation(w, r, "Invalid collection id.", err)
		return
	}
	if len(parts) == 2 && parts[1] == "add" && r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			respondValidation(w, r, "Could not read the add-to-collection form.", err)
			return
		}
		soldierID, err := parseOptionalInt64(r.FormValue("soldier_id"), "soldier_id")
		if err != nil || soldierID < 1 {
			respondValidation(w, r, "Invalid person record id.", err)
			return
		}
		added, err := a.soldiers.AddPersonRecordToResearchCollection(collectionID, soldierID)
		if err != nil {
			setToastHeaderWithType(w, "Record could not be added to the collection.", "error")
			respondInternal(w, r, "Could not add the record to the research collection.", err)
			return
		}
		redirectTo := fmt.Sprintf("/research-collections/%d", collectionID)
		if fromID, err := parseOptionalInt64(r.FormValue("from"), "from"); err == nil && fromID > 0 {
			redirectTo = fmt.Sprintf("/research-collections/%d?from=%d", collectionID, fromID)
		}
		// Idempotent: already-in collection is a no-op, not an
		// error. Use an info-style toast to signal "nothing
		// changed" without alarming the user. Matches the
		// "Success: record added to collection" tone for the
		// fresh-add path.
		if added {
			setToastHeader(w, "Success: record added to collection.")
		} else {
			setToastHeaderWithType(w, "Already in this collection.", "info")
		}
		w.Header().Set("X-DixieData-Redirect", redirectTo)
		fmt.Fprint(w, "Record added to collection.")
		return
	}
	if len(parts) == 1 && r.Method == http.MethodGet {
		fromID, err := parseOptionalInt64(r.URL.Query().Get("from"), "from")
		if err != nil {
			respondValidation(w, r, "Invalid from id.", err)
			return
		}
		detail, err := a.soldiers.ResearchCollectionDetail(collectionID, fromID)
		if err != nil {
			respondInternal(w, r, "Could not load the research collection detail.", err)
			return
		}
		// Issue #384 / Slice 6: wrap Render.
		if err := presentation.ResearchCollectionDetailView(*detail).Render(r.Context(), w); err != nil {
			respondErrorFragment(w, r, KindInternal, "Could not render the research collection detail.", err)
		}
		return
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func (a *App) handleSoldierPDF(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the PDF export form.", err)
		return
	}

	soldier, err := a.soldiers.GetByID(id)
	if err != nil {
		respondNotFound(w, r, fmt.Sprintf("Person record %d not found.", id), err)
		return
	}
	for i := range soldier.Images {
		soldier.Images[i].ResolvedPath = filepath.Join(a.dataDir, filepath.FromSlash(soldier.Images[i].FilePath))
	}
	options := parsePDFOptionsRequest(r, "L", true)

	dupKey := fmt.Sprintf("soldier-pdf|%d|%s|%s", id, options.Orientation, soldierPDFName(*soldier, options))
	release, admitted := a.guardDialog(dupKey)
	if !admitted {
		trace.Log("handleSoldierPDF dup_reject")
		a.respondDuplicateInFlight(w, r, dupKey)
		return
	}
	defer release()

	path, err := a.SaveFileDialog(runtime.SaveDialogOptions{
		DefaultFilename: soldierPDFName(*soldier, options),
		Filters: []runtime.FileFilter{
			{DisplayName: "PDF document", Pattern: "*.pdf"},
		},
	})
	if err != nil || path == "" {
		respondError(w, r, KindValidation, "PDF export cancelled.", nil)
		return
	}
	a.enqueueExport(dupKey, "soldier_pdf", func(ctx context.Context, p *jobs.Progress) error {
		p.Set(20, "Rendering Person Record PDF")
		return a.export.ExportSoldierPDF(path, *soldier, options)
	}, path, w)
	// Issue #513: announce Source Record truncation via toast so the
	// user sees the cap regardless of whether they look at the PDF
	// or the UI. Set AFTER enqueueExport writes the redirect header
	// would be a no-op — the toast must be staged before the response.
	// (The cap is also enforced in the Typst template at
	// templates/common/record_card.typ::render-records-section, so
	// the PDF body itself never spills past PDFRecordsPerPage rows.)
	setPDFRecordsTruncationToast(w, soldier)
}

func (a *App) handleSoldierPDFNoImages(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	soldier, err := a.soldiers.GetByID(id)
	if err != nil {
		respondNotFound(w, r, fmt.Sprintf("Person record %d not found.", id), err)
		return
	}

	dupKey := fmt.Sprintf("soldier-pdf-noimg|%d|%s", id, soldierPDFNameNoImages(*soldier))
	release, admitted := a.guardDialog(dupKey)
	if !admitted {
		trace.Log("handleSoldierPDFNoImages dup_reject")
		a.respondDuplicateInFlight(w, r, dupKey)
		return
	}
	defer release()

	path, err := a.SaveFileDialog(runtime.SaveDialogOptions{
		DefaultFilename: soldierPDFNameNoImages(*soldier),
		Filters: []runtime.FileFilter{
			{DisplayName: "PDF document", Pattern: "*.pdf"},
		},
	})
	if err != nil || path == "" {
		respondError(w, r, KindValidation, "PDF export cancelled.", nil)
		return
	}
	a.enqueueExport(dupKey, "soldier_pdf_no_images", func(ctx context.Context, p *jobs.Progress) error {
		p.Set(20, "Rendering text-only PDF")
		return a.export.ExportSoldierPDFWithoutImages(path, *soldier)
	}, path, w)
	// Issue #513: same truncation toast as handleSoldierPDF.
	setPDFRecordsTruncationToast(w, soldier)
}

func (a *App) handleSoldierJPG(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the JPG export form.", err)
		return
	}

	soldier, err := a.soldiers.GetByID(id)
	if err != nil {
		respondNotFound(w, r, fmt.Sprintf("Person record %d not found.", id), err)
		return
	}
	for i := range soldier.Images {
		soldier.Images[i].ResolvedPath = filepath.Join(a.dataDir, filepath.FromSlash(soldier.Images[i].FilePath))
	}
	options := parsePDFOptionsRequest(r, "L", true)

	dupKey := fmt.Sprintf("soldier-jpg|%d|%s|%s", id, options.Orientation, soldierJPGName(*soldier, options))
	release, admitted := a.guardDialog(dupKey)
	if !admitted {
		trace.Log("handleSoldierJPG dup_reject")
		a.respondDuplicateInFlight(w, r, dupKey)
		return
	}
	defer release()

	path, err := a.SaveFileDialog(runtime.SaveDialogOptions{
		DefaultFilename: soldierJPGName(*soldier, options),
		Filters: []runtime.FileFilter{
			{DisplayName: "JPEG image", Pattern: "*.jpg"},
		},
	})
	if err != nil || path == "" {
		respondError(w, r, KindValidation, "JPG export cancelled.", nil)
		return
	}

	a.enqueueExport(dupKey, "soldier_jpg", func(ctx context.Context, p *jobs.Progress) error {
		p.Set(20, "Rendering JPG pages")
		_, err := a.export.ExportSoldierJPG(path, *soldier, options)
		return err
	}, path, w)
}

func (a *App) handleCalendarPDF(w http.ResponseWriter, r *http.Request, monthValue string) {
	trace.Log("handleCalendarPDF ENTER")
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the calendar PDF form.", err)
		return
	}

	month, err := parseBoundedInt(monthValue, "month", 1, 12)
	if err != nil {
		respondValidation(w, r, "Invalid month.", err)
		return
	}
	trace.Log("handleCalendarPDF", "month", month)
	calendar, err := a.anniversary.GetMonthCalendar(month)
	if err != nil {
		respondInternal(w, r, "Could not load the monthly calendar.", err)
		return
	}
	trace.Log("handleCalendarPDF", "days", len(calendar))
	options := parsePDFOptionsRequest(r, "P", false)
	trace.Log("handleCalendarPDF", "options", fmt.Sprintf("%+v", options))
	trace.Log("handleCalendarPDF", "ctx_nil", a.ctx == nil, "frontend", ctxHasFrontend(a.ctx))

	// Reject rapid duplicate POSTs before we hit the native dialog.
	// A double-click queues two dialog requests on the Wails main
	// window message loop; both block on the UI thread and the
	// WebView2 frontend crashes. Returning a quick 429 lets the
	// user see a toast and prevents the second click from racing
	// with the first.
	dupKey := fmt.Sprintf("cal-pdf|%d|%s|%s", month, options.Orientation, monthPDFName(month, options))
	release, admitted := a.guardDialog(dupKey)
	if !admitted {
		trace.Log("handleCalendarPDF dup_reject")
		a.respondDuplicateInFlight(w, r, dupKey)
		return
	}
	defer release()

	path, err := a.SaveFileDialog(runtime.SaveDialogOptions{
		DefaultFilename: monthPDFName(month, options),
		Filters: []runtime.FileFilter{
			{DisplayName: "PDF document", Pattern: "*.pdf"},
		},
	})
	if err != nil || path == "" {
		trace.Log("handleCalendarPDF dialog_cancelled", "err", err != nil)
		respondError(w, r, KindValidation, "Monthly PDF export cancelled.", nil)
		return
	}
	trace.Log("handleCalendarPDF dialog_returned")
	a.enqueueExport(dupKey, "monthly_pdf", func(ctx context.Context, p *jobs.Progress) error {
		p.Set(20, "Rendering monthly PDF")
		return a.export.ExportMonthlyAnniversaryPDF(path, month, calendar, options)
	}, path, w)
	trace.Log("handleCalendarPDF EXIT")
}

func (a *App) handleImageScreenshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		ImageData string `json:"imageData"`
		FileName  string `json:"fileName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		respondValidation(w, r, "Invalid screenshot payload.", err)
		return
	}

	imageData := strings.TrimSpace(payload.ImageData)
	if !strings.HasPrefix(imageData, "data:image/png;base64,") {
		respondValidation(w, r, "Screenshot must be a PNG data URL.", nil)
		return
	}

	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(imageData, "data:image/png;base64,"))
	if err != nil {
		respondValidation(w, r, "Could not decode the screenshot image data.", err)
		return
	}

	dupKey := fmt.Sprintf("screenshot|%s", imageScreenshotName(payload.FileName))
	release, admitted := a.guardDialog(dupKey)
	if !admitted {
		trace.Log("handleImageScreenshot dup_reject")
		a.respondDuplicateInFlight(w, r, dupKey)
		return
	}
	defer release()

	path, err := a.SaveFileDialog(runtime.SaveDialogOptions{
		DefaultFilename: imageScreenshotName(payload.FileName),
		Filters: []runtime.FileFilter{
			{DisplayName: "PNG image", Pattern: "*.png"},
		},
	})
	if err != nil || path == "" {
		respondError(w, r, KindValidation, "Screenshot cancelled.", nil)
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		respondInternal(w, r, "Could not write the screenshot.", err)
		return
	}
	fmt.Fprintf(w, "✓ Saved screenshot to %s", path)
}

type imageRotateRequest struct {
	ImageID   int64  `json:"imageId"`
	Direction string `json:"direction"`
}

func (a *App) handleImageRotate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req imageRotateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondValidation(w, r, "Invalid rotate request.", err)
		return
	}
	if req.ImageID < 1 {
		respondValidation(w, r, "Invalid image id.", nil)
		return
	}

	imageRecord, err := a.soldiers.GetImageByID(req.ImageID)
	if err != nil {
		respondNotFound(w, r, fmt.Sprintf("Image %d not found.", req.ImageID), err)
		return
	}
	imagePath := filepath.Join(a.dataDir, filepath.FromSlash(imageRecord.FilePath))
	switch strings.ToLower(strings.TrimSpace(req.Direction)) {
	case "cw":
		err = rotateImageFile(imagePath, true)
	case "ccw":
		err = rotateImageFile(imagePath, false)
	default:
		respondValidation(w, r, "Invalid rotate direction. Use cw or ccw.", nil)
		return
	}
	if err != nil {
		respondInternal(w, r, "Could not rotate the image.", err)
		return
	}

	fmt.Fprint(w, "Image rotated.")
}

func rotateImageFile(path string, clockwise bool) error {
	source, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open image file: %w", err)
	}
	img, format, err := image.Decode(source)
	source.Close()
	if err != nil {
		return fmt.Errorf("decode image file: %w", err)
	}

	rotated := rotateImage90(img, clockwise)
	tempPath := path + ".rotate"
	output, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("create rotated image file: %w", err)
	}

	switch strings.ToLower(format) {
	case "jpeg", "jpg":
		err = jpeg.Encode(output, rotated, &jpeg.Options{Quality: 95})
	case "png":
		err = png.Encode(output, rotated)
	case "gif":
		err = gif.Encode(output, rotated, nil)
	default:
		err = fmt.Errorf("unsupported image format for rotation: %s", format)
	}
	closeErr := output.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(tempPath)
		return err
	}

	if err := os.Remove(path); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("replace rotated image file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("replace rotated image file: %w", err)
	}
	return nil
}

func rotateImage90(src image.Image, clockwise bool) image.Image {
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, height, width))

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if clockwise {
				dst.Set(height-1-y, x, src.At(bounds.Min.X+x, bounds.Min.Y+y))
			} else {
				dst.Set(y, width-1-x, src.At(bounds.Min.X+x, bounds.Min.Y+y))
			}
		}
	}
	return dst
}

func (a *App) handleDownloadSoldierImages(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the image download form.", err)
		return
	}

	soldier, err := a.soldiers.GetByID(id)
	if err != nil {
		respondNotFound(w, r, fmt.Sprintf("Person record %d not found.", id), err)
		return
	}
	if len(soldier.Images) == 0 {
		respondError(w, r, KindValidation, "No images are attached to this record.", nil)
		return
	}

	selected, err := selectedRecordImages(*soldier, r.Form["image_ids"], a.dataDir)
	if err != nil {
		respondValidation(w, r, "Could not parse selected image ids.", err)
		return
	}
	if len(selected) == 0 {
		respondError(w, r, KindValidation, "Select at least one image to download.", nil)
		return
	}

	parentDirOpts := runtime.OpenDialogOptions{
		Title: "Choose where to copy the record images",
	}
	dupKey := guardedOpenDirectoryDialogKey("download_images", parentDirOpts)
	parentDir, admitted, ok := a.guardedOpenDirectoryDialog(dupKey, parentDirOpts)
	if !admitted {
		a.respondDuplicateInFlight(w, r, dupKey)
		return
	}
	if !ok {
		respondError(w, r, KindValidation, "Download cancelled.", nil)
		return
	}
	destinationDir := filepath.Join(parentDir, imageExportFolderName(*soldier))
	if err := a.export.ExportImages(destinationDir, selected); err != nil {
		respondInternal(w, r, "Could not copy the images to the chosen folder.", err)
		return
	}
	fmt.Fprintf(w, "✓ Copied %d image(s) to %s", len(selected), destinationDir)
}

func (a *App) handleImportSoldierImages(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	soldier, err := a.soldiers.GetByID(id)
	if err != nil {
		respondNotFound(w, r, fmt.Sprintf("Person record %d not found.", id), err)
		return
	}

	// Web-mode (issue #401): the import form posts
	// multipart/form-data with the file input's "images" field.
	// Save each uploaded file to a temp path so the existing
	// importImagePaths walker can copy it into the record's image
	// directory. Wails mode does not hit this branch (the form is
	// urlencoded); it falls through to OpenMultipleFilesDialog
	// below. The web branch is synchronous: the JS dispatcher
	// reads the response body into the gallery wrapper via the
	// form's data-results-target, so a fragment-swap on success
	// keeps the user on the soldier detail page instead of
	// bouncing through /jobs/{id} (which the async Wails path
	// uses to surface background-job progress).
	uploadedPaths := readUploadedImagePaths(w, r)
	if uploadedPaths != nil {
		imported, importErr := a.importImagePaths(*soldier, uploadedPaths)
		if importErr != nil {
			slog.Error("appshell: soldier image import (web)", "audit", "respond-error", "person_record_id", id, "imported", imported, "err", importErr.Error())
			respondInternal(w, r, "Could not import the uploaded images.", importErr)
			return
		}
		setToastHeader(w, fmt.Sprintf("Imported %d image(s).", imported))
		a.renderSoldierImagesListFragment(w, r, id)
		return
	}

	pathsOpts := runtime.OpenDialogOptions{
		Filters: []runtime.FileFilter{
			{DisplayName: "Image files", Pattern: "*.png;*.jpg;*.jpeg;*.gif;*.bmp;*.webp;*.svg"},
		},
	}
	dupKey := guardedOpenMultipleFilesDialogKey("import_images", pathsOpts)
	paths, admitted, ok := a.guardedOpenMultipleFilesDialog(dupKey, pathsOpts)
	if !admitted {
		a.respondDuplicateInFlight(w, r, dupKey)
		return
	}
	if !ok {
		respondError(w, r, KindValidation, "Image import cancelled.", nil)
		return
	}

	a.runSoldierImageImportJob(w, soldier, id, paths, r.URL.Query().Get("return"))
}

// runSoldierImageImportJob enqueues the image_import background job
// for a soldier with the given already-resolved source paths and
// writes the redirect response. Extracted from handleImportSoldierImages
// in issue #401 so the multipart upload branch and the native-dialog
// branch can share the same job-enqueue + response sequence.
func (a *App) runSoldierImageImportJob(w http.ResponseWriter, soldier *models.Soldier, id int64, paths []string, returnTarget string) {
	redirectPath := imageImportRedirectPath(id, returnTarget)
	var jobID string
	jobID = a.jobs.Start("image_import", func(ctx context.Context, p *jobs.Progress) error {
		p.Set(5, fmt.Sprintf("Importing %d image(s)", len(paths)))
		p.Shimmer(ctx, 5, 95, 60*time.Second, "Encoding images…")
		imported, importErr := a.importImagePaths(*soldier, paths)
		if importErr != nil {
			if imported > 0 {
				slog.Error("appshell: partial image import", "audit", "respond-error", "person_record_id", id, "imported", imported, "err", importErr.Error())
				return importErr
			}
			slog.Error("appshell: image import", "audit", "respond-error", "person_record_id", id, "err", importErr.Error())
			return importErr
		}
		_ = redirectPath
		p.Set(100, fmt.Sprintf("Imported %d image(s).", imported))
		return nil
	})
	setInfoToastHeader(w, fmt.Sprintf("Importing %d image(s)…", len(paths)))
	writeExportRedirect(w, "/jobs/"+jobID)
}

func imageImportRedirectPath(id int64, returnTarget string) string {
	switch strings.ToLower(strings.TrimSpace(returnTarget)) {
	case "edit":
		return fmt.Sprintf("/soldiers/%d/edit", id)
	default:
		return fmt.Sprintf("/soldiers/%d", id)
	}
}

func (a *App) handleDeleteSoldierImages(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the image delete form.", err)
		return
	}

	soldier, err := a.soldiers.GetByID(id)
	if err != nil {
		respondNotFound(w, r, fmt.Sprintf("Person record %d not found.", id), err)
		return
	}
	selected, err := selectedRecordImages(*soldier, r.Form["image_ids"], a.dataDir)
	if err != nil {
		respondValidation(w, r, "Could not parse selected image ids.", err)
		return
	}
	if len(selected) == 0 {
		respondError(w, r, KindValidation, "Select at least one image to delete.", nil)
		return
	}

	for _, image := range selected {
		if err := os.Remove(image.FilePath); err != nil && !os.IsNotExist(err) {
			respondInternal(w, r, fmt.Sprintf("Could not delete image file %s.", image.FilePath), err)
			return
		}
	}

	imageIDs := make([]int64, 0, len(selected))
	for _, image := range selected {
		imageIDs = append(imageIDs, image.ID)
	}
	if err := a.soldiers.DeleteImages(id, imageIDs); err != nil {
		respondInternal(w, r, "Could not remove the image records from the database.", err)
		return
	}

	setToastHeader(w, fmt.Sprintf("Deleted %d image(s).", len(selected)))
	a.renderSoldierImagesListFragment(w, r, id)
}

func (a *App) handleSetPrimarySoldierImage(w http.ResponseWriter, r *http.Request, id, imageID int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, err := a.soldiers.GetByID(id); err != nil {
		respondNotFound(w, r, fmt.Sprintf("Person record %d not found.", id), err)
		return
	}
	if err := a.soldiers.SetPrimaryImage(id, imageID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondNotFound(w, r, fmt.Sprintf("Image %d not found.", imageID), err)
			return
		}
		respondInternal(w, r, "Could not update the primary image.", err)
		return
	}
	setToastHeader(w, "Primary image updated.")
	a.renderSoldierImagesListFragment(w, r, id)
}

// handleSoldierImagesRoute (issue #391 Slice B.1) is the chi
// route shim for /soldiers/{id}/images. Mirrors the event-side
// handleEventImagesRoute shape (internal/appshell/events_handlers.go:1176):
//
//	GET                             -> renderSoldierImagesListFragment (fragment)
//
// B.2 promotes the per-card Delete + Set-Primary forms to swap
// targets via this fragment. Pre-B.1 the path was dispatched from
// the catch-all handleSoldierByID; B.1 extracts it to a dedicated
// chi route registered in routes.go BEFORE the /soldiers/*
// wildcard so the explicit path wins.
func (a *App) handleSoldierImagesRoute(w http.ResponseWriter, r *http.Request) {
	prefix := "/soldiers/"
	trimmed := strings.TrimPrefix(r.URL.Path, prefix)
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) < 2 || parts[1] == "" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	soldierID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	a.renderSoldierImagesListFragment(w, r, soldierID)
}

// renderSoldierImagesListFragment loads the soldier's images
// and writes the per-soldier Images panel HTML into w (the
// SoldierImagesListFragment templ helper, no Layout()).
// Shared by GET /soldiers/{id}/images (lazy-load probe +
// post-action swap target) and (post-B.2) the per-card
// Delete + Set-Primary handlers. The fragment matches the
// on-page soldier_card.templ render via the templ helper
// so the data-results-target swap is visually identical
// to the initial page render.
func (a *App) renderSoldierImagesListFragment(w http.ResponseWriter, r *http.Request, soldierID int64) {
	soldier, err := a.soldiers.GetByID(soldierID)
	if err != nil {
		respondNotFound(w, r, fmt.Sprintf("Images for person record %d not found.", soldierID), err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	vm := viewmodel.PersonRecordFromModel(*soldier)
	if err := templates.SoldierImagesListFragment(soldierID, vm.DisplayID, vm.Images).Render(r.Context(), w); err != nil {
		respondInternal(w, r, fmt.Sprintf("Could not render images for person record %d.", soldierID), err)
	}
}

func parseCalendarEventPreferencesForm(r *http.Request) (models.CalendarEventPreferences, error) {
	if err := r.ParseForm(); err != nil {
		return models.CalendarEventPreferences{}, fmt.Errorf("failed to parse form: %w", err)
	}
	preferences := models.CalendarEventPreferences{
		TitlePreset:         strings.TrimSpace(r.FormValue("title_preset")),
		StartTime:           strings.TrimSpace(r.FormValue("start_time")),
		ReminderPrimary:     strings.TrimSpace(r.FormValue("reminder_primary")),
		ReminderSecondary:   strings.TrimSpace(r.FormValue("reminder_secondary")),
		IncludeRecordID:     r.FormValue("include_record_id") == "1",
		IncludeUnit:         r.FormValue("include_unit") == "1",
		IncludeBuriedIn:     r.FormValue("include_buried_in") == "1",
		IncludeOriginalDate: r.FormValue("include_original_date") == "1",
	}
	if _, _, ok := models.CalendarTimeComponents(preferences.StartTime); !ok {
		return models.CalendarEventPreferences{}, fmt.Errorf("start time must be between 05:00 and 23:00 in 15-minute increments")
	}
	if _, ok := models.CalendarReminderMinutes(preferences.ReminderPrimary); !ok {
		return models.CalendarEventPreferences{}, fmt.Errorf("invalid primary reminder option")
	}
	if _, ok := models.CalendarReminderMinutes(preferences.ReminderSecondary); !ok {
		return models.CalendarEventPreferences{}, fmt.Errorf("invalid secondary reminder option")
	}
	if strings.TrimSpace(preferences.ReminderPrimary) != "none" && preferences.ReminderPrimary == preferences.ReminderSecondary {
		return models.CalendarEventPreferences{}, fmt.Errorf("reminder selections must be different")
	}
	if !preferences.IncludeRecordID && !preferences.IncludeUnit && !preferences.IncludeBuriedIn && !preferences.IncludeOriginalDate {
		return models.CalendarEventPreferences{}, fmt.Errorf("select at least one description field")
	}
	return preferences, nil
}

func parseSoldierForm(r *http.Request, id int64) (models.Soldier, error) {
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(64 << 20); err != nil {
			return models.Soldier{}, fmt.Errorf("failed to parse multipart form: %w", err)
		}
	} else {
		if err := r.ParseForm(); err != nil {
			return models.Soldier{}, fmt.Errorf("failed to parse form: %w", err)
		}
	}

	birthDate, err := parseOptionalCanonicalDate(r.FormValue("birth_date"), "birth_date")
	if err != nil {
		return models.Soldier{}, err
	}
	deathDate, err := parseOptionalCanonicalDate(r.FormValue("death_date"), "death_date")
	if err != nil {
		return models.Soldier{}, err
	}
	// v60 (issue #320): Event Record subtype fields. The form
	// parser keeps them on the same models.Soldier payload so
	// the existing INSERT/UPDATE statement writes all 43 columns
	// in one transaction (the v60 schema widened
	// soldierSelectColumns to 43 cols; slice 2 widened the
	// INSERT/UPDATE statements to match). For non-Event rows
	// these fields stay empty and the service layer
	// (normalizeSoldierEntry event branch) clears them
	// defensively.
	beginDate, err := parseOptionalCanonicalDate(r.FormValue("begin_date"), "begin_date")
	if err != nil {
		return models.Soldier{}, err
	}
	endDate, err := parseOptionalCanonicalDate(r.FormValue("end_date"), "end_date")
	if err != nil {
		return models.Soldier{}, err
	}
	spouseSoldierID, err := parseOptionalInt64(r.FormValue("spouse_soldier_id"), "spouse_soldier_id")
	if err != nil {
		return models.Soldier{}, err
	}

	needsReview := r.FormValue("existing_needs_review") == "1"
	reviewReason := r.FormValue("existing_review_reason")

	return models.Soldier{
		ID:                    id,
		DisplayID:             r.FormValue("display_id"),
		EntryType:             r.FormValue("entry_type"),
		SpouseSoldierID:       spouseSoldierID,
		RelationshipLabel:     r.FormValue("relationship_label"),
		MaidenName:            r.FormValue("maiden_name"),
		PensionID:             r.FormValue("pension_id"),
		ApplicationID:         r.FormValue("application_id"),
		Prefix:                r.FormValue("prefix"),
		ShowPrefixBeforeName:  r.FormValue("show_prefix_before_name") == "1",
		FirstName:             r.FormValue("first_name"),
		MiddleName:            r.FormValue("middle_name"),
		LastName:              r.FormValue("last_name"),
		Suffix:                r.FormValue("suffix"),
		Rank:                  r.FormValue("rank_out"),
		RankIn:                r.FormValue("rank_in"),
		RankOut:               r.FormValue("rank_out"),
		Unit:                  r.FormValue("unit"),
		PensionState:          r.FormValue("pension_state"),
		ConfederateHomeStatus: r.FormValue("confederate_home_status"),
		ConfederateHomeName:   r.FormValue("confederate_home_name"),
		BirthDate:             birthDate,
		DeathDate:             deathDate,
		BirthInfo:             r.FormValue("birth_info"),
		BuriedIn:              r.FormValue("buried_in"),
		Biography:             r.FormValue("biography"),
		PDFExcerptOverride:    r.FormValue("pdf_excerpt_override"),
		Notes:                 r.FormValue("notes"),
		NeedsReview:           needsReview,
		ReviewReason:          reviewReason,
		// v60 (issue #320): Event Record subtype fields. The
		// form's Kind text input + Begin / End date text inputs +
		// Description long-form textarea land here. For Person
		// Record rows these stay empty; the service layer
		// (normalizeSoldierEntry) clears them defensively for
		// non-Event rows so the row never lands in the database
		// with stale data from a previous Event edit.
		Kind:        r.FormValue("kind"),
		BeginDate:   beginDate,
		EndDate:     endDate,
		Description: r.FormValue("description"),
		Records:     parseRecordInputs(r),
	}, nil
}

// newSoldierDefaults builds the starting values for the
// /soldiers/new form. The Display ID is pre-allocated via
// (*DB).NextDXDID so the field is read-only but present on
// first render. The PensionState + ConfederateHomeStatus
// defaults use the NotApplicable constants from the
// pensionstate + confederatehomestatus packages so the
// form's selects render an explicit "Not Applicable" row
// as the default.
func (a *App) newSoldierDefaults() (models.Soldier, error) {
	displayID, err := a.database.NextDXDID()
	if err != nil {
		return models.Soldier{}, err
	}
	return models.Soldier{DisplayID: displayID, PensionState: pensionstate.NotApplicable, ConfederateHomeStatus: confederatehomestatus.NotApplicable, ShowPrefixBeforeName: false}, nil
}

func parseOptionalInt(value, field string) (int, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, fmt.Errorf("invalid %s", field)
	}
	return parsed, nil
}

func parseOptionalInt64(value, field string) (int64, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s", field)
	}
	return parsed, nil
}

func (a *App) handleCreateCalendarItem(w http.ResponseWriter, r *http.Request, month, day int) {
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the calendar item form.", err)
		return
	}
	input := records.CalendarItemInput{
		ItemType: r.FormValue("item_type"),
		Title:    r.FormValue("title"),
		Notes:    r.FormValue("notes"),
	}
	item, err := a.calendar.CreateCalendarItem(month, day, input)
	if err != nil {
		if calendarValidationError(err) {
			a.renderCalendarDayDetail(w, r, month, day, 0, input.ItemType, input.Title, input.Notes, err.Error(), "", "", http.StatusBadRequest)
			return
		}
		respondInternal(w, r, "Could not create the calendar item.", err)
		return
	}
	w.Header().Set("X-DixieData-Refresh-Calendar-Month", strconv.Itoa(month))
	a.renderCalendarDayDetail(w, r, month, day, 0, "", "", "", "", "success", fmt.Sprintf("%s saved.", calendarItemTypeLabel(item.ItemType)), http.StatusOK)
}

func (a *App) handleUpdateCalendarItem(w http.ResponseWriter, r *http.Request, month, day int, itemID int64) {
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the calendar item form.", err)
		return
	}
	input := records.CalendarItemInput{
		ItemType: r.FormValue("item_type"),
		Title:    r.FormValue("title"),
		Notes:    r.FormValue("notes"),
	}
	item, err := a.calendar.UpdateCalendarItem(itemID, input)
	if err != nil {
		switch {
		case errors.Is(err, records.ErrCalendarItemNotFound):
			respondNotFound(w, r, fmt.Sprintf("Calendar item %d not found.", itemID), err)
			return
		case calendarValidationError(err):
			a.renderCalendarDayDetail(w, r, month, day, itemID, input.ItemType, input.Title, input.Notes, err.Error(), "", "", http.StatusBadRequest)
			return
		default:
			respondInternal(w, r, "Could not update the calendar item.", err)
			return
		}
	}
	w.Header().Set("X-DixieData-Refresh-Calendar-Month", strconv.Itoa(month))
	a.renderCalendarDayDetail(w, r, month, day, 0, "", "", "", "", "success", fmt.Sprintf("%s updated.", calendarItemTypeLabel(item.ItemType)), http.StatusOK)
}

func (a *App) handleDeleteCalendarItem(w http.ResponseWriter, r *http.Request, month, day int, itemID int64) {
	if err := a.calendar.DeleteCalendarItem(itemID); err != nil {
		switch {
		case errors.Is(err, records.ErrCalendarItemNotFound):
			respondNotFound(w, r, fmt.Sprintf("Calendar item %d not found.", itemID), err)
			return
		case calendarValidationError(err):
			respondValidation(w, r, "Calendar item validation failed.", err)
			return
		default:
			respondInternal(w, r, "Could not delete the calendar item.", err)
			return
		}
	}
	w.Header().Set("X-DixieData-Refresh-Calendar-Month", strconv.Itoa(month))
	a.renderCalendarDayDetail(w, r, month, day, 0, "", "", "", "", "success", "Calendar item deleted.", http.StatusOK)
}

func (a *App) renderCalendarDayDetail(w http.ResponseWriter, r *http.Request, month, day int, editingID int64, itemType, title, notes, errorMessage, statusKind, statusMessage string, statusCode int) {
	detail, err := a.calendar.GetDay(month, day)
	if err != nil {
		if calendarValidationError(err) {
			respondValidation(w, r, "Invalid calendar date.", err)
			return
		}
		respondInternal(w, r, "Could not load the calendar day detail.", err)
		return
	}
	if editingID > 0 && strings.TrimSpace(itemType) == "" && strings.TrimSpace(title) == "" && strings.TrimSpace(notes) == "" {
		item, ok := findCalendarItem(detail.Items, editingID)
		// Issue #384 / Slice 6: replace http.Error leak with respondNotFound
		// so the raw error text doesn't reach the client.
		if !ok {
			respondNotFound(w, r, "Calendar item not found in the day's list.", records.ErrCalendarItemNotFound)
			return
		}
		itemType = item.ItemType
		title = item.Title
		notes = item.Notes
	}
	if statusCode != http.StatusOK {
		w.WriteHeader(statusCode)
	}
	// HTMX callers (the calendar page's #details-pane target) set
	// the HX-Request header and need the bare fragment so the swap
	// re-renders only the day detail inside the parent page's
	// already-styled shell. Direct navigation (saved URL, deep
	// link, curl) has no header and needs the Layout shell so
	// app.css + the top-nav + the floating dock load.
	if r.Header.Get("HX-Request") == "true" {
		// Issue #384 / Slice 6: wrap Render.
		if err := presentation.CalendarDayDetail(detail, editingID, itemType, title, notes, errorMessage, statusKind, statusMessage).Render(r.Context(), w); err != nil {
			respondErrorFragment(w, r, KindInternal, "Could not render the calendar day detail.", err)
		}
	} else {
		// Issue #384 / Slice 6: wrap Render.
		if err := presentation.CalendarDayDetailPage(detail, editingID, itemType, title, notes, errorMessage, statusKind, statusMessage).Render(r.Context(), w); err != nil {
			respondErrorFragment(w, r, KindInternal, "Could not render the calendar day detail page.", err)
		}
	}
}

func findCalendarItem(items []models.CalendarItem, itemID int64) (models.CalendarItem, bool) {
	for _, item := range items {
		if item.ID == itemID {
			return item, true
		}
	}
	return models.CalendarItem{}, false
}

func calendarValidationError(err error) bool {
	var validationErr *records.CalendarValidationError
	return errors.As(err, &validationErr)
}

func calendarItemTypeLabel(itemType string) string {
	switch itemType {
	case models.CalendarItemTypeHoliday:
		return "Holiday"
	default:
		return "Event"
	}
}

func parseOptionalBoundedInt(value, field string, min, max int) (int, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, nil
	}
	return parseBoundedInt(trimmed, field, min, max)
}

func (a *App) renderEntryForm(w http.ResponseWriter, r *http.Request, soldier models.Soldier, isEdit bool, errorMessage string, statusCode int) {
	candidates, err := a.soldiers.MarriageCandidates()
	if err != nil {
		respondInternal(w, r, "Could not load marriage candidates for the entry form.", err)
		return
	}
	suggestions, err := a.soldiers.FormSuggestions()
	if err != nil {
		respondInternal(w, r, "Could not load browse suggestions for the entry form.", err)
		return
	}
	w.WriteHeader(statusCode)
	if errorMessage != "" {
		// Issue #384 / Slice 6: wrap Render.
		if err := presentation.EntryFormWithError(soldier, candidates, suggestions, isEdit, errorMessage).Render(r.Context(), w); err != nil {
			respondErrorFragment(w, r, KindInternal, "Could not render the entry form with errors.", err)
		}
		return
	}
	// Issue #384 / Slice 6: wrap Render.
	if err := presentation.EntryForm(soldier, candidates, suggestions, isEdit).Render(r.Context(), w); err != nil {
		respondErrorFragment(w, r, KindInternal, "Could not render the entry form.", err)
	}
}

func parseOptionalCanonicalDate(value, field string) (string, error) {
	normalized, err := dates.NormalizeCanonical(value)
	if err != nil {
		return "", fmt.Errorf("invalid %s", field)
	}
	return normalized, nil
}

func parseLegacySearchComponent(value string) int {
	parsed, _ := strconv.Atoi(strings.TrimSpace(value))
	return parsed
}

func parseBoundedInt(value, field string, min, max int) (int, error) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("invalid %s", field)
	}
	if parsed < min || parsed > max {
		return 0, fmt.Errorf("invalid %s", field)
	}
	return parsed, nil
}

func selectedRecordImages(soldier models.Soldier, selectedIDs []string, dataDir string) ([]models.Image, error) {
	selectedSet := make(map[int64]struct{}, len(selectedIDs))
	for _, value := range selectedIDs {
		id, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid image selection")
		}
		selectedSet[id] = struct{}{}
	}

	var selected []models.Image
	for _, image := range soldier.Images {
		if _, ok := selectedSet[image.ID]; !ok {
			continue
		}
		image.FilePath = filepath.Join(dataDir, filepath.FromSlash(image.FilePath))
		selected = append(selected, image)
	}
	return selected, nil
}

func imageExportFolderName(soldier models.Soldier) string {
	base := strings.TrimSpace(soldier.DisplayID)
	if base == "" {
		base = fmt.Sprintf("%s-%s", soldier.FirstName, soldier.LastName)
	}
	return sanitizedFileStem(base, "soldier-images") + "_Images"
}

func imageScreenshotName(fileName string) string {
	base := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	return sanitizedFileStem(base, "archive-image") + "-screenshot.png"
}

func soldierPDFName(soldier models.Soldier, options archive.PDFOptions) string {
	base := strings.TrimSpace(soldier.DisplayID)
	if base == "" {
		base = strings.TrimSpace(soldier.FirstName + " " + soldier.LastName)
	}
	return pdfReportName(base, options, !options.IncludeImages)
}

func soldierJPGName(soldier models.Soldier, options archive.PDFOptions) string {
	base := strings.TrimSpace(soldier.DisplayID)
	if base == "" {
		base = strings.TrimSpace(soldier.FirstName + " " + soldier.LastName)
	}
	return jpgReportName(base, options, !options.IncludeImages)
}

func soldierPDFNameNoImages(soldier models.Soldier) string {
	base := strings.TrimSpace(soldier.DisplayID)
	if base == "" {
		base = strings.TrimSpace(soldier.FirstName + " " + soldier.LastName)
	}
	return pdfReportName(base, archive.PDFOptions{Orientation: "L", IncludeImages: false}, true)
}

// eventPDFName returns the per-Event PDF file name (D4: prefix
// distinguishes from per-Person PDF when both end up in the
// same download folder). Display ID is the EVT-NNNNN allocated
// by NextEventID; falls back to the kind slug when absent.
func eventPDFName(event models.Soldier) string {
	base := strings.TrimSpace(event.DisplayID)
	if base == "" {
		base = "Event"
	}
	return sanitizedFileStem("Event-"+base, "event-record") + ".pdf"
}

func monthPDFName(month int, options archive.PDFOptions) string {
	return pdfReportName(fmt.Sprintf("%s report", monthNameValue(month)), options, false)
}

func printableArchivePDFName(settings archive.PrintSettings) string {
	name := pdfReportName("dixiedata-printable-archive", archive.PDFOptions{
		Orientation:     settings.Orientation,
		PrinterFriendly: settings.PrinterFriendly,
	}, false)
	if !settings.FullBiographyPage {
		return name
	}
	return strings.TrimSuffix(name, ".pdf") + "-full-biography.pdf"
}

func pdfReportName(base string, options archive.PDFOptions, noImages bool) string {
	stem := sanitizedFileStem(base, "pdf-report")
	suffix := pdfOptionFilenameSuffix(options, noImages)
	if suffix != "" {
		stem += "-" + suffix
	}
	return stem + ".pdf"
}

func jpgReportName(base string, options archive.PDFOptions, noImages bool) string {
	stem := sanitizedFileStem(base, "jpg-report")
	suffix := pdfOptionFilenameSuffix(options, noImages)
	if suffix != "" {
		stem += "-" + suffix
	}
	return stem + ".jpg"
}

func pdfOptionFilenameSuffix(options archive.PDFOptions, noImages bool) string {
	options = options.Normalize("P", true)
	parts := make([]string, 0, 3)
	if options.PrinterFriendly {
		parts = append(parts, "printer-friendly")
	}
	if options.Orientation == "L" {
		parts = append(parts, "landscape")
	} else {
		parts = append(parts, "portrait")
	}
	if noImages {
		parts = append(parts, "no-images")
	}
	return strings.Join(parts, "-")
}

func backupArchiveName(now time.Time) string {
	return fmt.Sprintf("dixiedata-backup-%s.ddbak", now.Format("2006-01-02"))
}

func sharedArchiveName(now time.Time) string {
	return fmt.Sprintf("dixiedata-shared-%s.ddshare", now.Format("2006-01-02"))
}

func sanitizedFileStem(value, fallback string) string {
	value = strings.TrimSpace(value)
	value = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_':
			return r
		case r == ' ':
			return '-'
		default:
			return '-'
		}
	}, value)
	value = strings.Trim(value, "-")
	if value == "" {
		return fallback
	}
	return value
}

func monthNameValue(month int) string {
	months := []string{"", "January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}
	if month < 1 || month > 12 {
		return "Unknown"
	}
	return months[month]
}

func parseRecordInputs(r *http.Request) []models.Record {
	recordTypes := r.Form["record_type"]
	appIDs := r.Form["record_app_id"]
	details := r.Form["record_details"]

	count := len(recordTypes)
	if len(appIDs) > count {
		count = len(appIDs)
	}
	if len(details) > count {
		count = len(details)
	}

	records := make([]models.Record, 0, count)
	for i := 0; i < count; i++ {
		record := models.Record{}
		if i < len(recordTypes) {
			record.RecordType = recordTypes[i]
		}
		if i < len(appIDs) {
			record.AppID = appIDs[i]
		}
		if i < len(details) {
			record.Details = details[i]
		}
		records = append(records, record)
	}
	return records
}

func writeMemorialImportErrorLog(summary records.MemorialImportSummary) (string, error) {
	if len(summary.Issues) == 0 && len(summary.Skips) == 0 {
		return "", nil
	}
	file, err := os.CreateTemp("", "dixiedata-memorial-import-*.log")
	if err != nil {
		return "", err
	}
	defer debug.DeferCloseLog(file, "writeMemorialImportErrorLog.file")
	for _, skip := range summary.Skips {
		if _, err := fmt.Fprintf(file, "row=%d memorial_id=%q name=%q skipped_reason=%q\n", skip.Row, skip.MemorialID, skip.Name, skip.Reason); err != nil {
			return "", err
		}
	}
	for _, issue := range summary.Issues {
		_, err := fmt.Fprintf(file, "row=%d memorial_id=%q name=%q error=%q\n", issue.Row, issue.MemorialID, issue.Name, issue.Error)
		if err != nil {
			return "", err
		}
	}
	return file.Name(), nil
}

func exportLinkMarkup(label, path string) string {
	fileURL := "file:///" + strings.TrimPrefix(filepath.ToSlash(path), "/")
	return fmt.Sprintf(
		`<div class="rounded-2xl border border-[#d8c08d] bg-[rgba(255,248,230,0.85)] px-4 py-3 text-sm text-slate-700">%s <a href="%s" data-open-external="true" class="pill-link" target="_blank" rel="noreferrer">%s</a></div>`,
		html.EscapeString(label),
		html.EscapeString(fileURL),
		html.EscapeString(path),
	)
}

func externalLinkMarkup(label, href, text string) string {
	return fmt.Sprintf(
		`<div class="rounded-2xl border border-[#d8c08d] bg-[rgba(255,248,230,0.85)] px-4 py-3 text-sm text-slate-700">%s <a href="%s" data-open-external="true" class="pill-link" target="_blank" rel="noreferrer">%s</a></div>`,
		html.EscapeString(label),
		html.EscapeString(href),
		html.EscapeString(text),
	)
}

func (a *App) handleOpenLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the open-link form.", err)
		return
	}
	target, err := normalizeChromeOpenTarget(r.FormValue("target"))
	if err != nil {
		respondValidation(w, r, "Invalid link target.", err)
		return
	}
	if err := openLinkTarget(target); err != nil {
		respondInternal(w, r, "Could not open the link in Chrome.", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleScratchpadOpen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the scratch pad form.", err)
		return
	}
	if a.scratchpads == nil {
		respondUnavailable(w, r, "Scratch pad service is not running. Restart DixieData to recover.", nil)
		return
	}
	displayID := strings.TrimSpace(r.FormValue("display_id"))
	if displayID == "" {
		respondValidation(w, r, "A Display ID is required before opening the scratch pad.", nil)
		return
	}
	seed := r.FormValue("scratchpad_seed")
	if err := a.scratchpads.Open(displayID, seed); err != nil {
		respondInternal(w, r, "Could not open the scratch pad.", err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = fmt.Fprintf(w, "Scratch pad ready for %s.", displayID)
}

func (a *App) handleMedia(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	relative := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/media/"))
	relative = strings.TrimLeft(relative, `/\`)
	if relative == "" {
		http.NotFound(w, r)
		return
	}

	baseDir := filepath.Clean(a.dataDir)
	resolved := filepath.Join(baseDir, filepath.FromSlash(relative))
	withinBase, err := filepath.Rel(baseDir, resolved)
	if err != nil || strings.HasPrefix(withinBase, "..") {
		http.NotFound(w, r)
		return
	}

	info, err := os.Stat(resolved)
	if err == nil && !info.IsDir() {
		http.ServeFile(w, r, resolved)
		return
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		respondInternal(w, r, "Could not access the media file.", err)
		return
	}

	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	fmt.Fprintf(w, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 480 320" role="img" aria-label="Image Missing">
<rect width="480" height="320" rx="28" fill="#f6f1e4"/>
<rect x="16" y="16" width="448" height="288" rx="22" fill="#fff" stroke="#8d7440" stroke-width="4" stroke-dasharray="12 8"/>
<path d="M96 224l56-72 52 48 44-56 88 80" fill="none" stroke="#324253" stroke-width="16" stroke-linecap="round" stroke-linejoin="round"/>
<circle cx="164" cy="116" r="24" fill="#c5ab68"/>
<text x="240" y="264" text-anchor="middle" font-family="Arial, sans-serif" font-size="28" font-weight="700" fill="#22303d">Image Missing</text>
<text x="240" y="292" text-anchor="middle" font-family="Arial, sans-serif" font-size="15" fill="#324253">%s</text>
</svg>`, html.EscapeString(filepath.Base(relative)))
}

func normalizeChromeOpenTarget(target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", errors.New("missing link target")
	}
	if strings.HasPrefix(strings.ToLower(target), "http://") || strings.HasPrefix(strings.ToLower(target), "https://") || strings.HasPrefix(strings.ToLower(target), "file:///") {
		return target, nil
	}
	if filepath.IsAbs(target) {
		return "file:///" + strings.TrimPrefix(filepath.ToSlash(target), "/"), nil
	}
	parsed, err := url.Parse(target)
	if err == nil && parsed.Scheme != "" {
		return target, nil
	}
	return "", fmt.Errorf("unsupported link target: %s", target)
}

func openLinkInChrome(target string) error {
	chromePath, err := findChromeExecutable()
	if err != nil {
		return err
	}
	return exec.Command(chromePath, "--new-tab", target).Start()
}

func openLinkTarget(target string) error {
	if isFileOpenTarget(target) {
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", target).Start()
	}
	return openLinkInChrome(target)
}

func isFileOpenTarget(target string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(target)), "file:///")
}

func findChromeExecutable() (string, error) {
	candidates := []string{
		filepath.Join(os.Getenv("ProgramFiles"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("LocalAppData"), "Google", "Chrome", "Application", "chrome.exe"),
	}
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	if path, err := exec.LookPath("chrome.exe"); err == nil {
		return path, nil
	}
	if path, err := exec.LookPath("chrome"); err == nil {
		return path, nil
	}
	return "", errors.New("Google Chrome was not found")
}

func (a *App) reloadServices() error {
	soldierSvc := records.NewSoldierService(a.database)
	// Apply configured list page size from config.json (#639).
	soldierSvc.SetListPageSize(a.cfg.Limits.ListDefaultPageSize)
	// Apply configured browse page sizes.
	records.SetBrowsePageSizes(a.cfg.Limits.BrowseDefaultPageSize, a.cfg.Limits.BrowseMaxPageSize)
	// Apply configured retention caps from config.json (#639).
	update.SetMaxRetainedBackups(a.cfg.Limits.MaxRetainedBackups)
	update.SetMaxRestorePoints(a.cfg.Limits.MaxRestorePoints)
	archive.SetOrphanTrashRetentionDays(a.cfg.Limits.OrphanTrashRetentionDays)
	SetFeedbackRetentionDays(a.cfg.Limits.FeedbackRetentionDays)
	a.soldiers = soldierSvc
	// v60 (issue #320): wire the Event Service immediately after
	// the SoldierService so the constructor's "borrows
	// SoldierService.db" precondition holds. Reload after a
	// .ddbak restore replaces the same handle; the Event facade
	// is rebuilt against the fresh soldierSvc reference.
	a.events = records.NewEventService(soldierSvc)
	// Issue #343 finding #5: wire the back-reference so
	// SoldierService.ServiceTimeline can delegate the linked-
	// events-for-timeline JOIN to EventService.
	soldierSvc.SetEvents(a.events)
	a.articles = records.NewArticleService(soldierSvc, records.NewMarkdownRenderer())
	a.articles.SetPageSizes(a.cfg.Limits.ArticlePageSize, a.cfg.Limits.ArticlePageSize*4)
	viewmodel.SetBodyExcerptCap(a.cfg.Limits.ArticleExcerptChars)
	viewmodel.SetAllowedImageMIMETypes(a.cfg.Files.AllowedImageMIMETypes)
	archive.SetExportBatchSize(a.cfg.Limits.ExportBatchSize)
	// Wire the configured ThemeConfig into the markdown
	// renderer so the typst PDF output matches the browser
	// preview (issue #660 amendment #2).
	markdownRenderer := records.NewMarkdownRenderer()
	markdownRenderer.SetTheme(&a.cfg.Theme)
	a.articles.SetMarkdownRenderer(markdownRenderer)
	// Issue #671 follow-up: thread the footer template into
	// the article service so the PDF footer shows the
	// configured version + build identity instead of a
	// bare "Made with DixieData". The template key names
	// must match what article_portrait.typ reads (footer_text +
	// codename).
	short := buildinfo.GitCommit
	if len(short) > 7 {
		short = short[:7]
	}
	codename := fmt.Sprintf("v%d.%d.%d · %s",
		versioninfo.CurrentSchemaVersion,
		versioninfo.CurrentUpdateFlowVersion,
		versioninfo.AppRelease(),
		short,
	)
	a.articles.SetBranding(map[string]string{
		"footer_text": "Made with DixieData",
		"codename":    codename,
	})
	a.anniversary = records.NewAnniversaryService(a.database)
	a.calendar = records.NewCalendarService(a.database)
	a.analytics = records.NewAnalyticsService(a.database)
	a.audit = records.NewAuditService(a.database)
	a.audit.SetDuplicateSimilarityThreshold(a.cfg.Limits.DuplicateAuditThreshold)
	a.audit.SetResolvedFindingsPageSize(a.cfg.Limits.AuditPageSize)
	a.exportTemplates = records.NewExportTemplateService(a.database.Conn())
	a.shareQueuePresets = records.NewShareQueuePresetService(a.database.Conn())
	a.tags = records.NewTagService(a.database.Conn())
	a.archiveMeta = records.NewArchiveMetaService(a.database.Conn())
	a.images = archive.NewImageService(a.database)
	a.export = archive.NewExportService(a.database, soldierSvc)
	a.backup = archive.NewBackupService(a.database, soldierSvc)
	a.backup.SetMergeConflictsPageSize(a.cfg.Limits.MergeConflictsPageSize)
	// Preserve the existing jobs Registry when one is already
	// wired. reloadServices() runs in two contexts that MUST NOT
	// clobber an in-flight job's registry:
	//
	//   1. lifecycle startup: openJobsRegistry(dataDir) has
	//      already rehydrated the persistent jobs.jsonl into
	//      a.jobs. Replacing it here would silently drop every
	//      running/queued/interrupted job from the previous
	//      session on every app start.
	//
	//   2. .ddbak restore worker: handleImportBackup calls
	//      a.reopenDatabase() after the data dir is replaced.
	//      That path runs reloadServices() while the backup_import
	//      job is still in the registry. Replacing a.jobs here
	//      makes the subsequent SetResult() and the /jobs/{id}
	//      poll handler return 404 even though the import
	//      succeeded — the page shows 404s instead of the
	//      final report (issue observed in ddbak-import
	//      regression, see CHANGELOG [Unreleased] Maintenance).
	//
	// Only allocate a fresh Registry on the very first call
	// (NewApp() leaves a.jobs nil). Tests that bypass startup
	// and call reloadServices() on a fresh App still get a
	// working empty registry.
	if a.jobs == nil {
		a.jobs = jobs.NewWithConcurrency(jobsConcurrencyFromEnv(a.cfg.Limits.JobsConcurrency))
	}

	// Wire the Typst-backed Registry into the export service. Per
	// slice 7, the appshell uses Typst exclusively; if the binary
	// or templates directory is missing, ExportService falls back
	// to its fpdf Service (which is preserved as a test scaffold).
	if reg, _, err := a.buildRenderRegistry(); err == nil && reg != nil {
		a.export.SetRegistry(reg)
		// Article PDF pre-render (slice 4.2) shares the same
		// registry through a small adapter (the interface
		// lives in internal/records to avoid a pkg/render
		// import cycle).
		if a.articles != nil {
			a.articles.SetArticleRegistry(&articleRegistryAdapter{reg: reg})
		}
		// Event PDF pre-render (issue #374) shares the same
		// registry through a parallel adapter. Mirrors the
		// article wiring above; the events facade then handles
		// /events/{id}/pdf with pre-render + guarded dialog.
		if a.events != nil {
			a.events.SetEventRegistry(&eventRegistryAdapter{reg: reg})
		}
	}
	// Bulk export reads each soldier's images by absolute path.
	// Soldier.Images[i].FilePath is stored relative to the data
	// dir, and the single-record export handlers fill in
	// ResolvedPath themselves. The bulk export path (ExportFullDatabasePDF)
	// fetches its own soldiers and would otherwise leave ResolvedPath
	// empty; without it the typst image-staging step silently skips
	// the file and the template's #image("images/<name>") reference
	// fails with "file not found". SetDataDir lets the bulk path
	// resolve FilePath against the data dir on the fly.
	a.export.SetDataDir(a.dataDir)
	a.diagnostics = archive.NewDiagnosticsService(a.database, soldierSvc)
	a.google = integrations.NewGoogleService(a.dataDir)
	integrations.SetCalendarNames(a.cfg.Google.CalendarName, a.cfg.Google.TestCalendarName)
	integrations.SetHealthTimeout(time.Duration(a.cfg.Timing.GoogleHealthTimeoutS) * time.Second)
	integrations.SetOAuthWaitTimeout(time.Duration(a.cfg.Timing.GoogleOAuthWaitTimeoutS) * time.Second)
	a.updater = update.NewService(a.database, a.dataDir, func(outputPath string) error {
		_, err := a.backup.Export(outputPath, a.dataDir)
		return err
	})
	a.updater.SetHTTPTimeout(time.Duration(a.cfg.Timing.UpdateCheckTimeoutS) * time.Second)
	a.updater.SetCheckURL(a.cfg.Services.UpdateCheckURL)
	a.updateProgress = newUpdateProgressState()

	// Issue #660: wire the feedback endpoint + send timeout
	// into the package globals so handleFeedbackSubmit reads
	// the configured value instead of the built-in constant.
	formsparkConfiguredEndpoint = a.cfg.Services.FeedbackEndpoint
	feedbackSendTimeoutS = a.cfg.Timing.FeedbackSendTimeoutS
	supportuploader.SetUploadTimeout(time.Duration(a.cfg.Timing.FeedbackUploadTimeoutS) * time.Second)

	// Issue #660 amendment #1: one-shot migration of the
	// user-set update source URL from the SQLite
	// `system_config` table into `config.Services.UpdateSourceURL`.
	// The migration fires when:
	//   - cfg.Services.UpdateSourceURL is empty (default), AND
	//   - system_config has a non-empty update_source_url row.
	// The migration copies the row's value into cfg + deletes
	// the row so future reads come from cfg (which survives
	// .ddbak imports). The transaction wraps the delete + the
	// in-memory update so a crash mid-migration leaves the
	// appshell in the pre-migration state; the next launch
	// re-fires safely.
	if a.cfg.Services.UpdateSourceURL == "" {
		if existing, err := a.database.SystemConfig("update_source_url"); err == nil && strings.TrimSpace(existing) != "" {
			a.cfg.Services.UpdateSourceURL = strings.TrimSpace(existing)
			a.updater.SetSourceURL(a.cfg.Services.UpdateSourceURL)
			if err := a.database.SetSystemConfig("update_source_url", ""); err == nil {
				if err := config.Save(a.dataDir, a.cfg); err != nil {
					// Save failure is non-fatal: the in-memory
					// value is already set, so this session
					// works. The next config.Save (e.g. another
					// settings change) will persist it.
					log := debug.FromContext(context.Background())
					log.Warn("config.Save after update_source_url migration failed", "err", err.Error())
				}
			}
		}
	} else {
		// Migration already ran (cfg has the value); sync the
		// updater override so the next sourceSettings() call
		// reads it.
		a.updater.SetSourceURL(a.cfg.Services.UpdateSourceURL)
	}
	a.scratchpads = scratchpad.NewLauncher(a.dataDir, a.database)
	if a.database != nil {
		if err := a.images.EnsureShardedStorage(a.dataDir); err != nil {
			return err
		}
		if err := a.images.PurgeExpiredTrash(a.dataDir); err != nil {
			return err
		}
		required, err := a.database.IdentitySetupRequired()
		if err != nil {
			return err
		}
		a.setupRequired = required
		if !required {
			needsBackfill, err := a.database.EntryAuditIdentityBackfillNeeded()
			if err != nil {
				return err
			}
			if needsBackfill {
				if err := a.database.BackfillEntryAuditIdentity(); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// buildRenderRegistry constructs the Typst-backed Registry and
// returns it plus the templates directory. Returns (nil, "", err)
// when the Typst binary or templates directory cannot be located;
// the caller treats this as 'no Registry available' and the
// ExportService falls back to its fpdf Service for tests only.
//
// Per slice 7, the appshell does NOT include an FpdfRenderer in
// the Registry. The Registry's Resolve method falls back to the
// 'fpdf:recordType' engine when no Typst template matches, but
// because the Registry doesn't have an FpdfRenderer, that
// fallback returns an error. In practice all the production
// record types (soldier, widow, wife, linked_person) have
// matching Typst templates, so the fpdf fallback is never hit.
func (a *App) buildRenderRegistry() (*render.Registry, string, error) {
	binPath, err := a.findTypstBinary()
	if err != nil {
		return nil, "", err
	}
	templatesDir, err := a.findTemplatesDir()
	if err != nil {
		return nil, "", err
	}
	typstRenderer := render.NewTypstRenderer(binPath, filepath.Dir(templatesDir))
	// Inject theme config as theme.json into every typst workdir
	// (#637). When the config file is missing or the theme block is
	// unset (tests, first-run), the renderer's own defaultThemeJSON
	// fallback provides the hard-coded palette.
	if a.cfg.Theme.Palette != nil && len(a.cfg.Theme.Palette) > 0 {
		if themeJSON, err := json.Marshal(a.cfg.Theme); err == nil {
			typstRenderer.SetTheme(themeJSON)
		}
	}
	reg := render.NewRegistry(typstRenderer, templatesDir)
	return reg, templatesDir, nil
}

// findTypstBinary locates the bundled Typst binary. The lookup
// order is:
//   1. The directory containing the running exe (release layout
//      has <install>/bin/typst-windows.exe next to DixieData.exe).
//   2. The current working directory.
//   3. Walk up to 6 parent levels from cwd (development layout
//      where the exe runs from a subdirectory of the repo).
//
// DixieData is a Windows-only app; the primary binary is
// typst-windows.exe. The macOS and Linux names are kept as
// fallbacks so this code still locates a binary if a developer
// happens to be running it on a non-Windows host for testing,
// but the release builds bundle only typst-windows.exe.
func (a *App) findTypstBinary() (string, error) {
	candidates := []string{"typst-windows.exe", "typst-macos", "typst-linux"}
	for _, dir := range a.findTypstSearchDirs() {
		for _, name := range candidates {
			candidate := filepath.Join(dir, "bin", name)
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
		}
	}
	return "", fmt.Errorf("typst binary not found in bin/ (expected typst-windows.exe)")
}

// findTemplatesDir locates the templates/ directory. Lookup
// order matches findTypstBinary: exe's directory first, then
// cwd, then up to 6 parent levels.
func (a *App) findTemplatesDir() (string, error) {
	for _, dir := range a.findTypstSearchDirs() {
		candidate := filepath.Join(dir, "templates")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			// Verify it's the typst templates dir, not the Go
			// html/template dir at internal/templates. The sentinel
			// soldier_landscape.typ only exists in the typst tree.
			if _, err := os.Stat(filepath.Join(candidate, "soldier_landscape.typ")); err == nil {
				return candidate, nil
			}
		}
	}
	return "", fmt.Errorf("typst templates directory not found (expected templates/soldier_landscape.typ)")
}

// findTypstSearchDirs returns the directories to search for
// the Typst binary and templates, in priority order. The exe's
// directory comes first so the release layout (everything next
// to DixieData.exe) works regardless of cwd. cwd and its
// parents follow for development layouts.
func (a *App) findTypstSearchDirs() []string {
	seen := map[string]bool{}
	var dirs []string

	add := func(dir string) {
		if dir == "" || seen[dir] {
			return
		}
		seen[dir] = true
		dirs = append(dirs, dir)
	}

	// 1. Exe's directory.
	if exePath, err := os.Executable(); err == nil {
		add(filepath.Dir(exePath))
	}

	// 2. cwd and up to 6 parent levels.
	cwd, err := os.Getwd()
	if err == nil {
		dir := cwd
		for i := 0; i < 6; i++ {
			add(dir)
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	return dirs
}

func (a *App) activatePendingRecovery(restorePointID string, cause error) error {
	if a.database != nil {
		a.database.Close()
		a.database = nil
	}
	record, err := a.restorePoints.Get(restorePointID)
	if err != nil {
		return fmt.Errorf("load restore point %q: %w", restorePointID, err)
	}
	a.pendingRecovery = &record
	if cause != nil {
		a.recoveryFailure = cause.Error()
	}
	return nil
}

func (a *App) initializeLocalData() error {
	if filepath.Base(a.dataDir) != ".dixiedata" {
		return fmt.Errorf("refusing to initialize unexpected data directory: %s", a.dataDir)
	}
	parent := filepath.Dir(a.dataDir)
	backupDir, err := os.MkdirTemp(parent, ".dixiedata-backup-*")
	if err != nil {
		return err
	}
	if a.database != nil {
		a.database.Close()
		a.database = nil
	}
	// MkdirTemp guarantees a unique name; no collision with
	// stale backups from prior failed inits.
	if _, err := os.Stat(a.dataDir); err == nil {
		// Issue #216: retry the rename with exponential backoff
		// so transient Windows handle conflicts (WAL/SHM files,
		// antivirus) have time to release. 5 attempts, 4 sleep
		// periods of 200/400/800/1600ms = 3s total wait.
		if err := renameWithRetry(a.dataDir, backupDir, 5, 200*time.Millisecond); err != nil {
			// os.Rename on Windows may fail persistently when file
			// handles in the data dir are still held. Fall back to
			// os.RemoveAll (old destructive behavior) — the user
			// already confirmed this is a destructive operation.
			slog.Warn("appshell: initialize: rename failed, falling back to RemoveAll", "err", err)
			_ = os.RemoveAll(backupDir)
			if err := os.RemoveAll(a.dataDir); err != nil {
				return err
			}
			// Skip the rollback: no backup dir exists to restore from.
			return a.reopenDatabase()
		}
	}
	if err := a.reopenDatabase(); err != nil {
		// Restore the old data dir from backup so the user
		// doesn't lose data. setupRequired=true sends every
		// blocked branch in App.ServeHTTP to /setup.
		_ = os.Rename(backupDir, a.dataDir)
		a.setupRequired = true
		return fmt.Errorf("reopen after rename: %w", err)
	}
	_ = os.RemoveAll(backupDir)
	return nil
}

// recoveryLink returns the destination for error-page recovery
// navigation. /setup when the DB is gone or setup is required,
// /calendar otherwise.
func (a *App) recoveryLink() string {
	if a.setupRequired || a.database == nil {
		return "/setup"
	}
	return "/calendar"
}

// renameWithRetry calls os.Rename with exponential backoff so
// transient Windows handle conflicts (SQLite WAL/SHM, antivirus,
// OneDrive) have time to release. Pattern from #216 (replaceDataDir).
func renameWithRetry(src, dst string, attempts int, baseDelay time.Duration) error {
	var lastErr error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			time.Sleep(baseDelay * time.Duration(1<<(i-1)))
		}
		// os.Rename on Windows may fail if any file in src is
		// still held by another process (WAL/SHM handles left
		// behind by sql.DB.Close). Each subsequent attempt waits
		// longer for those handles to be released.
		if err := os.Rename(src, dst); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	return lastErr
}

func (a *App) reopenDatabase() error {
	database, err := db.Open(a.dataDir)
	if err != nil {
		return err
	}
	a.database = database
	return a.reloadServices()
}

func loadQuotes(data []byte) ([]models.Quote, error) {
	var payload struct {
		Quotes []models.Quote `json:"civil_war_quotes"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	if payload.Quotes == nil {
		payload.Quotes = []models.Quote{}
	}
	return payload.Quotes, nil
}

func (a *App) listAllSoldiers() ([]models.Soldier, error) {
	var soldiers []models.Soldier
	page := 1
	for {
		batch, _, err := a.soldiers.List(page, 500)
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		soldiers = append(soldiers, batch...)
		if len(batch) < 500 {
			break
		}
		page++
	}
	return soldiers, nil
}

func selectQuoteForArchive(quotes []models.Quote, totalSoldiers int) models.Quote {
	if len(quotes) == 0 {
		return models.Quote{}
	}
	if totalSoldiers < 0 {
		totalSoldiers = 0
	}
	index := (totalSoldiers / 3) % len(quotes)
	return quotes[index]
}

func parseInitialSetupForm(r *http.Request) (models.InitialSetupForm, int, error) {
	if err := r.ParseForm(); err != nil {
		return models.InitialSetupForm{}, 0, fmt.Errorf("failed to parse setup form")
	}
	form := models.InitialSetupForm{
		FirstName:  strings.TrimSpace(r.FormValue("first_name")),
		MiddleName: strings.TrimSpace(r.FormValue("middle_name")),
		LastName:   strings.TrimSpace(r.FormValue("last_name")),
		BirthYear:  strings.TrimSpace(r.FormValue("birth_year")),
	}
	birthYear, err := parseBoundedInt(form.BirthYear, "birth_year", 1000, 9999)
	if err != nil {
		return form, 0, err
	}
	prefix, err := db.BuildUserNodePrefix(form.FirstName, form.MiddleName, form.LastName, birthYear)
	if err != nil {
		return form, 0, err
	}
	form.PrefixPreview = prefix
	return form, birthYear, nil
}

func parsePage(value string) int {
	page, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || page < 1 {
		return 1
	}
	return page
}

func parseCSVInt64s(value string) ([]int64, error) {
	parts := strings.Split(strings.TrimSpace(value), ",")
	results := make([]int64, 0, len(parts))
	seen := map[int64]struct{}{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil || id < 1 {
			return nil, fmt.Errorf("invalid ids")
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		results = append(results, id)
	}
	return results, nil
}

func parseSelectedSoldierIDs(values []string) ([]int64, error) {
	selected := make([]int64, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		id, err := strconv.ParseInt(trimmed, 10, 64)
		if err != nil || id < 1 {
			return nil, fmt.Errorf("invalid review queue selection")
		}
		selected = append(selected, id)
	}
	return selected, nil
}

func compareIDsFromRequest(r *http.Request) (int64, int64, error) {
	id1, err1 := parseOptionalInt64(strings.TrimSpace(r.URL.Query().Get("id1")), "id1")
	id2, err2 := parseOptionalInt64(strings.TrimSpace(r.URL.Query().Get("id2")), "id2")
	if err1 == nil && err2 == nil && id1 > 0 && id2 > 0 {
		if id1 == id2 {
			return 0, 0, fmt.Errorf("choose two different records to compare")
		}
		return id1, id2, nil
	}
	values := r.URL.Query()["compare_ids"]
	if len(values) != 2 {
		return 0, 0, fmt.Errorf("choose exactly two records to compare")
	}
	selected, err := parseSelectedSoldierIDs(values)
	if err != nil || len(selected) != 2 {
		return 0, 0, fmt.Errorf("choose exactly two records to compare")
	}
	if selected[0] == selected[1] {
		return 0, 0, fmt.Errorf("choose two different records to compare")
	}
	return selected[0], selected[1], nil
}

func (a *App) attachDetailBackLink(soldier *models.Soldier, fromValue string) error {
	if soldier == nil || fromValue == "" {
		return nil
	}
	fromID, err := strconv.ParseInt(fromValue, 10, 64)
	if err != nil || fromID < 1 || fromID == soldier.ID {
		return nil
	}
	source, err := a.soldiers.GetByID(fromID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	soldier.BackLinkURL = fmt.Sprintf("/soldiers/%d", fromID)
	soldier.BackLinkLabel = "Back to " + linkedRecordLabel(*source)
	return nil
}

func linkedRecordLabel(s models.Soldier) string {
	switch strings.TrimSpace(strings.ToLower(s.EntryType)) {
	case "wife":
		return "Wife Record"
	case "widow":
		return "Widow Record"
	default:
		return "Soldier Record"
	}
}

func (a *App) saveUploadedImages(r *http.Request, soldier models.Soldier) error {
	if r.MultipartForm == nil || len(r.MultipartForm.File["images"]) == 0 {
		return nil
	}

	recordDir, relativeDir := appdata.RecordImageDir(a.dataDir, soldier.DisplayID)
	if err := os.MkdirAll(recordDir, 0o755); err != nil {
		return fmt.Errorf("create image directory: %w", err)
	}
	namePrefix := filepath.Base(relativeDir)
	nextSequence, err := nextStoredImageSequence(recordDir, namePrefix)
	if err != nil {
		return fmt.Errorf("prepare image filenames: %w", err)
	}

	var issues []string
	for _, fileHeader := range r.MultipartForm.File["images"] {
		if fileHeader == nil || fileHeader.Filename == "" {
			continue
		}
		if !isAllowedImageFile(fileHeader.Filename) {
			issues = append(issues, fmt.Sprintf("unsupported image file: %s", fileHeader.Filename))
			continue
		}

		storedName := standardizedImageFileName(namePrefix, nextSequence, fileHeader.Filename)
		absolutePath := filepath.Join(recordDir, storedName)
		relativePath := filepath.Join(relativeDir, storedName)

		if err := saveUploadedFile(fileHeader, absolutePath); err != nil {
			issues = append(issues, err.Error())
			continue
		}
		if err := a.soldiers.AddImage(soldier.ID, storedName, relativePath, ""); err != nil {
			_ = os.Remove(absolutePath)
			issues = append(issues, err.Error())
			continue
		}
		nextSequence++
	}

	if len(issues) > 0 {
		return errors.New(strings.Join(issues, "; "))
	}
	return nil
}

func (a *App) importImagePaths(soldier models.Soldier, paths []string) (int, error) {
	recordDir, relativeDir := appdata.RecordImageDir(a.dataDir, soldier.DisplayID)
	if err := os.MkdirAll(recordDir, 0o755); err != nil {
		return 0, fmt.Errorf("create image directory: %w", err)
	}
	namePrefix := filepath.Base(relativeDir)
	nextSequence, err := nextStoredImageSequence(recordDir, namePrefix)
	if err != nil {
		return 0, fmt.Errorf("prepare image filenames: %w", err)
	}

	imported := 0
	var issues []string
	for _, sourcePath := range paths {
		sourcePath = strings.TrimSpace(sourcePath)
		if sourcePath == "" {
			continue
		}
		fileName := filepath.Base(sourcePath)
		if !isAllowedImageFile(fileName) {
			issues = append(issues, fmt.Sprintf("unsupported image file: %s", fileName))
			continue
		}
		info, err := os.Stat(sourcePath)
		if err != nil {
			issues = append(issues, fmt.Sprintf("read image file %s: %v", fileName, err))
			continue
		}
		if info.IsDir() || info.Size() == 0 {
			issues = append(issues, fmt.Sprintf("image file %s is empty", fileName))
			continue
		}

		storedName := standardizedImageFileName(namePrefix, nextSequence, fileName)
		absolutePath := filepath.Join(recordDir, storedName)
		relativePath := filepath.Join(relativeDir, storedName)

		if err := copyImageFile(sourcePath, absolutePath); err != nil {
			issues = append(issues, err.Error())
			continue
		}
		if err := a.soldiers.AddImage(soldier.ID, storedName, relativePath, ""); err != nil {
			_ = os.Remove(absolutePath)
			issues = append(issues, err.Error())
			continue
		}
		imported++
		nextSequence++
	}

	if len(issues) > 0 {
		return imported, errors.New(strings.Join(issues, "; "))
	}
	return imported, nil
}

func saveUploadedFile(fileHeader *multipart.FileHeader, destination string) error {
	src, err := fileHeader.Open()
	if err != nil {
		return fmt.Errorf("open upload %s: %w", fileHeader.Filename, err)
	}
	defer debug.DeferCloseLog(src, "saveUploadedFile.src")

	dst, err := os.Create(destination)
	if err != nil {
		return fmt.Errorf("create image file %s: %w", destination, err)
	}
	defer debug.DeferCloseLog(dst, "saveUploadedFile.dst")

	written, err := io.Copy(dst, src)
	if err != nil {
		return fmt.Errorf("write image file %s: %w", destination, err)
	}
	if written == 0 {
		dst.Close()
		_ = os.Remove(destination)
		return fmt.Errorf("image file %s is empty", fileHeader.Filename)
	}
	return nil
}

func copyImageFile(sourcePath, destination string) error {
	src, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open image file %s: %w", filepath.Base(sourcePath), err)
	}
	defer debug.DeferCloseLog(src, "copyImageFile.src")

	dst, err := os.Create(destination)
	if err != nil {
		return fmt.Errorf("create image file %s: %w", destination, err)
	}
	defer debug.DeferCloseLog(dst, "copyImageFile.dst")

	written, err := io.Copy(dst, src)
	if err != nil {
		return fmt.Errorf("write image file %s: %w", destination, err)
	}
	if written == 0 {
		dst.Close()
		_ = os.Remove(destination)
		return fmt.Errorf("image file %s is empty", filepath.Base(sourcePath))
	}
	return nil
}

func standardizedImageFileName(prefix string, sequence int, originalName string) string {
	return fmt.Sprintf("%s-img-%03d%s", strings.TrimSpace(prefix), sequence, normalizedImageExtension(originalName))
}

func nextStoredImageSequence(recordDir, prefix string) (int, error) {
	entries, err := os.ReadDir(recordDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 1, nil
		}
		return 0, err
	}

	maxSequence := 0
	patternPrefix := prefix + "-img-"
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, patternPrefix) {
			continue
		}
		base := strings.TrimSuffix(name, filepath.Ext(name))
		sequenceText := strings.TrimPrefix(base, patternPrefix)
		sequence, err := strconv.Atoi(sequenceText)
		if err != nil {
			continue
		}
		if sequence > maxSequence {
			maxSequence = sequence
		}
	}
	return maxSequence + 1, nil
}

func normalizedImageExtension(name string) string {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(name))) {
	case ".jpg":
		return ".jpg"
	case ".jpeg":
		return ".jpeg"
	case ".png":
		return ".png"
	case ".gif":
		return ".gif"
	case ".webp":
		return ".webp"
	case ".bmp":
		return ".bmp"
	case ".svg":
		return ".svg"
	default:
		return ".img"
	}
}

func isAllowedImageFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg":
		return true
	default:
		return false
	}
}

// jobsConcurrencyFromEnv reads the optional DIXIEDATA_JOBS_CONCURRENCY
// environment variable and falls back to jobs.DefaultConcurrency when it
// is unset, empty, or not a positive integer. Clamps to a sane upper
// bound (16) so a typo or runaway script cannot exhaust the host.
func jobsConcurrencyFromEnv(cfgDefault int) int {
	const envKey = "DIXIEDATA_JOBS_CONCURRENCY"
	const upperBound = 16
	raw := strings.TrimSpace(os.Getenv(envKey))
	if raw == "" {
		if cfgDefault > 0 {
			return cfgDefault
		}
		return jobs.DefaultConcurrency
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		if cfgDefault > 0 {
			return cfgDefault
		}
		return jobs.DefaultConcurrency
	}
	if n > upperBound {
		n = upperBound
	}
	return n
}

// articleRegistryAdapter adapts the *render.Registry to the
// internal/records.ArticleRegistry interface so the article
// service can call into the typst-backed render path without
// depending on pkg/render (which would create a cycle:
// pkg/render -> internal/records).
type articleRegistryAdapter struct {
	reg *render.Registry
}

func (a *articleRegistryAdapter) RenderArticle(ctx context.Context, recordType, orientation string, data map[string]any, w io.Writer) error {
	if a == nil || a.reg == nil {
		return errors.New("articleRegistryAdapter: nil registry")
	}
	settings := render.PrintSettings{
		Orientation: orientation,
		SingleRecordTemplate: recordType + "_" + func() string {
			if orientation == "P" {
				return "portrait"
			}
			return "landscape"
		}(),
	}.Normalize()
	return a.reg.Render(ctx, settings, recordType, data, w)
}

// eventRegistryAdapter adapts the *render.Registry to the
// EventRegistry interface declared in internal/records
// (event_service.go). Mirrors articleRegistryAdapter above so
// the issue #374 Event orientation picker has the same
// pre-render seam the article picker (issue #321 slice-4.5)
// already uses.
type eventRegistryAdapter struct {
	reg *render.Registry
}

func (a *eventRegistryAdapter) RenderEvent(ctx context.Context, recordType, orientation string, data map[string]any, w io.Writer) error {
	if a == nil || a.reg == nil {
		return errors.New("eventRegistryAdapter: nil registry")
	}
	settings := render.PrintSettings{
		Orientation: orientation,
		SingleRecordTemplate: recordType + "_" + func() string {
			if orientation == "P" {
				return "portrait"
			}
			return "landscape"
		}(),
	}.Normalize()
	return a.reg.Render(ctx, settings, recordType, data, w)
}

// readUploadedImagePaths inspects r for a multipart/form-data body
// with one or more file parts under the "images" field name and
// streams each one to a temporary file on disk. It returns:
//
//   - non-nil slice when at least one uploaded file was written; the
//     slice contains the on-disk temp paths that the caller can feed
//     to importImagePaths.
//   - nil when the request is not multipart (i.e. the Wails
//     native-dialog path will be used instead). Caller should fall
//     through to its dialog branch.
//
// Respond-write side effects on the error paths: the helper calls
// respondValidation / respondInternal so the HTTP response is fully
// formed even when the caller bails out after a nil return.
//
// Issue #401: web-mode has no native OpenMultipleFilesDialog override,
// so the import form posts multipart instead. This helper bridges the
// browser upload to the existing filesystem-path-based import walker.
// Temp files are intentionally not cleaned up here; the import job
// copies the bytes into the record's image directory and the OS
// reclaims the temp on next reboot.
func readUploadedImagePaths(w http.ResponseWriter, r *http.Request) []string {
	contentType := r.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "multipart/form-data") {
		return nil
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		respondValidation(w, r, "Could not parse uploaded images.", err)
		return nil
	}
	files := r.MultipartForm.File["images"]
	if len(files) == 0 {
		return nil
	}
	paths := make([]string, 0, len(files))
	for _, fh := range files {
		if fh == nil {
			continue
		}
		src, err := fh.Open()
		if err != nil {
			respondValidation(w, r, "Could not open uploaded file.", err)
			return nil
		}
		ext := filepath.Ext(fh.Filename)
		tmp, err := os.CreateTemp("", "dixiedata-upload-*"+ext)
		if err != nil {
			src.Close()
			respondInternal(w, r, "Could not save uploaded file.", err)
			return nil
		}
		if _, err := io.Copy(tmp, src); err != nil {
			src.Close()
			tmp.Close()
			os.Remove(tmp.Name())
			respondInternal(w, r, "Could not read uploaded file.", err)
			return nil
		}
		src.Close()
		tmp.Close()
		paths = append(paths, tmp.Name())
	}
	return paths
}
