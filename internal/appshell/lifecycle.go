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
	if err := configureStressLogging(); err != nil {
		a.startupErr = fmt.Errorf("failed to configure stress logging: %w", err)
		a.setupRoutes()
		return
	}
	a.ctx = ctx
	a.dataDir = appdata.DefaultDir()
	a.restorePoints = update.NewRestorePointManager(a.dataDir)
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
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		buffered.FlushTo(w)
		return
	}
	a.mux.ServeHTTP(w, r)
}
