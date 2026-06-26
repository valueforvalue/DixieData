// lifecycle.go holds the App HTTP server lifecycle: the buffered response
// writer, the NewApp constructor + WithFrontendAssets builder, the Startup/
// Shutdown entry points, the ServeHTTP handler, and the frontend-asset
// helpers that ServeHTTP relies on. Extracted from app.go as step 2 of
// the God-class reduction tracked in issue #42. Domain handler methods
// stay in their respective *_handlers.go files; this file is the "framework".
package appshell

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"

	"github.com/valueforvalue/DixieData/internal/appdata"
	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/debug"
	"github.com/valueforvalue/DixieData/internal/records"
	"github.com/valueforvalue/DixieData/internal/update"
)


// --- bufferedResponseWriter + methods ---
type bufferedResponseWriter struct {
	header     http.Header
	statusCode int
	body       bytes.Buffer
}

func newBufferedResponseWriter() *bufferedResponseWriter {
	return &bufferedResponseWriter{header: make(http.Header)}
}

func (b *bufferedResponseWriter) Header() http.Header {
	return b.header
}

func (b *bufferedResponseWriter) WriteHeader(statusCode int) {
	if b.statusCode != 0 {
		return
	}
	b.statusCode = statusCode
}

func (b *bufferedResponseWriter) Write(data []byte) (int, error) {
	if b.statusCode == 0 {
		b.statusCode = http.StatusOK
	}
	return b.body.Write(data)
}

func (b *bufferedResponseWriter) FlushTo(target http.ResponseWriter) {
	for key, values := range b.header {
		for _, value := range values {
			target.Header().Add(key, value)
		}
	}
	statusCode := b.statusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	target.WriteHeader(statusCode)
	if b.body.Len() > 0 {
		_, _ = target.Write(b.body.Bytes())
	}
}

// --- NewApp, WithFrontendAssets, Startup, Shutdown, startup, shutdown ---
func NewApp() *App {
	return &App{}
}

func (a *App) WithFrontendAssets(frontendAssets fs.FS) *App {
	a.frontendAssets = frontendAssets
	return a
}

func (a *App) Startup(ctx context.Context) {
	a.startup(ctx)
}

func (a *App) Shutdown(ctx context.Context) {
	a.shutdown(ctx)
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.dataDir = appdata.DefaultDir()

	// One-time migration: move logs out of the data directory into a
	// sibling .dixiedata-logs/ folder. Before the split, app logs
	// lived at <dataDir>/logs/app.log.jsonl, which forced the
	// .ddbak restore code path to release its Windows file handle
	// on the log file before os.Rename(.dixiedata, ...) could
	// succeed. The migration runs BEFORE debug.Configure so the new
	// log file is opened in the new location, not the old one.
	if moved, err := migrateLogsToSiblingDir(a.dataDir); err != nil {
		fmt.Printf("warning: log migration failed (continuing with new layout): %v\n", err)
	} else if moved > 0 {
		fmt.Printf("info: migrated %d log file(s) out of .dixiedata/ into .dixiedata-logs/\n", moved)
	}

	// Configure structured logging AFTER dataDir is resolved so the log
	// file lands in the correct location.
	logPath := appdata.AppLogPath(a.dataDir)
	if err := debug.Configure(debug.Config{
		LogPath:       logPath,
		RingSize:      500,
		AppName:       buildinfo.AppName,
		AppVersion:    buildinfo.AppVersion,
		BuildIdentity: buildinfo.BuildIdentity(),
	}); err != nil {
		fmt.Printf("warning: failed to configure debug logging: %v\n", err)
	}

	if err := configureStressLogging(); err != nil {
		a.startupErr = fmt.Errorf("failed to configure stress logging: %w", err)
		a.setupRoutes()
		return
	}
	a.restorePoints = update.NewRestorePointManager(a.dataDir)
	// Load local settings + apply DebugMode. Settings takes effect on
	// the next slog call (debug.SetDebugMode raises/lowers the level
	// floor).
	if settings, err := records.LoadLocalSettings(a.dataDir); err == nil {
		a.debugMode.Store(settings.DebugMode)
		debug.SetDebugMode(settings.DebugMode)
	} else {
		fmt.Printf("warning: could not load local settings: %v\n", err)
	}
	// Replace the placeholder Registry from NewApp() with one wired
	// to the on-disk JSONL log so background jobs survive webview
	// reloads and app restarts.
	a.jobs = openJobsRegistry(a.dataDir)
	pruneFeedbackLogOnStartup(a.dataDir)
	var err error
	a.quotes, err = loadQuotes(embeddedQuotes)
	if err != nil {
		a.startupErr = fmt.Errorf("failed to load quotes: %w", err)
		fmt.Println(a.startupErr)
		a.setupRoutes()
		return
	}

	if err := a.restorePoints.Housekeeping(); err != nil {
		a.startupErr = fmt.Errorf("failed to prepare restore point storage: %w", err)
		fmt.Println(a.startupErr)
		a.setupRoutes()
		return
	}

	postUpdateLaunchState, err := a.restorePoints.LoadLaunchState()
	if err != nil {
		a.startupErr = fmt.Errorf("failed to read restore point state: %w", err)
		fmt.Println(a.startupErr)
		a.setupRoutes()
		return
	}
	if postUpdateLaunchState != nil {
		switch {
		case !postUpdateLaunchState.MatchesCurrentBuild(buildinfo.AppVersion, buildinfo.BuildIdentity()):
			if err := a.restorePoints.ClearLaunchState(); err != nil {
				a.startupErr = fmt.Errorf("failed to clear stale restore point state: %w", err)
				fmt.Println(a.startupErr)
				a.setupRoutes()
				return
			}
			postUpdateLaunchState = nil
		case postUpdateLaunchState.Status == update.RestorePointLaunchStarting:
			if err := a.activatePendingRecovery(postUpdateLaunchState.RestorePointID, errors.New("DixieData did not reach a healthy first launch after the update.")); err != nil {
				a.startupErr = err
				fmt.Println(a.startupErr)
			}
			a.setupRoutes()
			return
		case postUpdateLaunchState.Status == update.RestorePointLaunchPrepared:
			postUpdateLaunchState, err = a.restorePoints.MarkLaunchStarting()
			if err != nil {
				a.startupErr = fmt.Errorf("failed to mark restore point launch state: %w", err)
				fmt.Println(a.startupErr)
				a.setupRoutes()
				return
			}
		}
	}

	a.database, err = db.Open(a.dataDir)
	if err != nil {
		if postUpdateLaunchState != nil {
			if recoveryErr := a.activatePendingRecovery(postUpdateLaunchState.RestorePointID, fmt.Errorf("failed to open database: %w", err)); recoveryErr != nil {
				a.startupErr = recoveryErr
				fmt.Println(a.startupErr)
			}
			a.setupRoutes()
			return
		}
		a.startupErr = fmt.Errorf("failed to open database: %w", err)
		fmt.Println(a.startupErr)
		a.setupRoutes()
		return
	}

	if err := a.reloadServices(); err != nil {
		if a.database != nil {
			a.database.Close()
			a.database = nil
		}
		if postUpdateLaunchState != nil {
			if recoveryErr := a.activatePendingRecovery(postUpdateLaunchState.RestorePointID, err); recoveryErr != nil {
				a.startupErr = recoveryErr
				fmt.Println(a.startupErr)
			}
			a.setupRoutes()
			return
		}
		a.startupErr = err
		fmt.Println(a.startupErr)
		a.setupRoutes()
		return
	}

	a.pendingLaunchStateClear = postUpdateLaunchState != nil

	a.setupRoutes()
}

func (a *App) shutdown(ctx context.Context) {
	_ = debug.Flush()
	_ = debug.Close()
	if a.database != nil {
		a.database.Close()
	}
}

// --- handleFrontendAsset ---
func (a *App) handleFrontendAsset(name, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path != "/"+name {
			http.NotFound(w, r)
			return
		}

		data, err := a.readFrontendAsset(name)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "no-store")
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		_, _ = w.Write(data)
	}
}

// --- readFrontendAsset ---
func (a *App) readFrontendAsset(name string) ([]byte, error) {
	if a.frontendAssets != nil {
		return fs.ReadFile(a.frontendAssets, name)
	}
	candidates := []string{filepath.Join("frontend", filepath.FromSlash(name))}
	if root, err := appdata.ProjectRoot(); err == nil {
		candidates = append([]string{filepath.Join(root, "frontend", filepath.FromSlash(name))}, candidates...)
	}
	var lastErr error
	for _, candidate := range candidates {
		data, err := os.ReadFile(candidate)
		if err == nil {
			return data, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// --- ServeHTTP ---
func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	debug.FromContext(r.Context()).Debug("request received",
		"component", "http",
		"method", r.Method,
		"path", r.URL.Path,
		"referer", r.Header.Get("Referer"),
	)
	defer func() {
		if pv := recover(); pv != nil {
			path := LogCrash(r, pv)
			// http.Error sets headers + writes a plain error body.
			// If the inner handler already wrote headers this is a
			// best-effort 500 response.
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "Internal server error. See %s for details.\n", path)
		}
	}()
	if a.mux == nil {
		// During startup, serve bootstrap frontend assets directly so the initial
		// index page can keep executing client bootstrap logic while routes warm up.
		switch r.URL.Path {
		case "/app.js":
			a.handleFrontendAsset("app.js", "text/javascript; charset=utf-8").ServeHTTP(w, r)
			return
		case "/app.css":
			a.handleFrontendAsset("app.css", "text/css; charset=utf-8").ServeHTTP(w, r)
			return
		case "/debug.js":
			a.handleFrontendAsset("debug.js", "text/javascript; charset=utf-8").ServeHTTP(w, r)
			return
		case "/htmx.min.js":
			a.handleFrontendAsset("htmx.min.js", "text/javascript; charset=utf-8").ServeHTTP(w, r)
			return
		}
		renderStartupPlaceholder(w, r)
		return
	}
	if a.pendingRecovery != nil && !recoveryRequestAllowed(r.URL.Path) {
		http.Redirect(w, r, "/recovery", http.StatusSeeOther)
		return
	}
	if a.startupErr != nil {
		http.Error(w, a.startupErr.Error(), http.StatusInternalServerError)
		return
	}
	if a.setupRequired && !setupRequestAllowed(r.URL.Path) {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	if override := requestMethodOverride(r); override != "" {
		r = r.Clone(r.Context())
		r.Method = override
	}
	if a.pendingLaunchStateClear && shouldAttemptPostUpdateHealthClear(r) {
		buffered := newBufferedResponseWriter()
		a.mux.ServeHTTP(buffered, r)
		if buffered.statusCode >= http.StatusOK && buffered.statusCode < http.StatusMultipleChoices {
			if err := a.clearPendingLaunchState(); err != nil {
				respondInternal(w, r, "Could not clear the pending launch state after update.", err)
				return
			}
		}
		buffered.FlushTo(w)
		return
	}
	// Inject debug-mode flag into the request context so templates can
	// render the Debug Console button without needing the App struct.
	ctx := debug.WithDebugMode(r.Context(), a.debugMode.Load())
	a.mux.ServeHTTP(w, r.WithContext(ctx))
}

// migrateLogsToSiblingDir is the one-time startup migration that moves
// the app log files out of <dataDir>/logs/ into the sibling
// <parent-of-dataDir>/.dixiedata-logs/ directory.
//
// Before the layout split, app logs lived inside .dixiedata/, which
// meant the .ddbak restore code path (replaceDataDir → os.Rename) had
// to release the open log file handle on Windows before the rename
// could succeed. Every restore attempt failed with "Access is denied"
// while the log file was still open. Splitting app state from
// archive state makes restore atomic and removes the handle-release
// requirement entirely.
//
// Returns the number of files moved (0 if there was nothing to
// migrate, or if the migration had already run on a previous start).
// Errors are non-fatal — the caller logs them and continues with the
// new layout, so a half-migrated state does not block app startup.
func migrateLogsToSiblingDir(dataDir string) (int, error) {
	oldLogsDir := filepath.Join(dataDir, "logs")
	newLogsDir := appdata.LogsRoot(dataDir)

	// Nothing to do if the old location never existed (fresh install).
	if _, err := os.Stat(oldLogsDir); err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("stat old logs dir: %w", err)
	}

	// If the new location already exists, the migration already ran
	// (or the user manually moved logs). Move any stragglers from
	// old → new without overwriting; both sides converge to the same
	// content so this is safe to re-run.
	if err := os.MkdirAll(newLogsDir, 0o755); err != nil {
		return 0, fmt.Errorf("create new logs dir: %w", err)
	}

	entries, err := os.ReadDir(oldLogsDir)
	if err != nil {
		return 0, fmt.Errorf("read old logs dir: %w", err)
	}

	moved := 0
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		oldPath := filepath.Join(oldLogsDir, entry.Name())
		newPath := filepath.Join(newLogsDir, entry.Name())
		// Skip if the destination already has the file — never
		// overwrite. The user may have started writing the new log
		// before we got to the migration.
		if _, err := os.Stat(newPath); err == nil {
			continue
		}
		// os.Rename on Windows fails if the destination exists or if
		// the source is held open by another process. Both are
		// acceptable: skip and continue with the next file. A future
		// restart can retry stragglers.
		if err := os.Rename(oldPath, newPath); err != nil {
			continue
		}
		moved++
	}

	// If the old logs dir is now empty, remove it so the data
	// directory contains no stale log artifacts. Failure here is
	// non-fatal — the dir will just sit empty inside the data
	// folder until the next restore wipes it via replaceDataDir.
	if remaining, _ := os.ReadDir(oldLogsDir); len(remaining) == 0 {
		_ = os.Remove(oldLogsDir)
	}

	return moved, nil
}
