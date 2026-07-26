// Command dixiedata-web boots the DixieData App on a plain HTTP listener so
// the UI can be driven by headless browsers (e.g. Playwright) without the
// Wails WebView2 wrapper. Routes, handlers, templ templates, app.js, and
// app.css are all real — this is the production app served over HTTP.
//
// Intended use:
//   - UI/UX audits (Playwright + axe-core)
//   - Manual browser smoke tests against a seed-loaded data dir
//   - Anything that needs a real network-attached HTML surface
//
// Not intended for production. The web-mode app uses context.Background()
// for the appshell ctx, so any handler that calls Wails dialog APIs will
// panic. Read-only browsing routes do not call them.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/valueforvalue/DixieData/internal/appdata"
	"github.com/valueforvalue/DixieData/internal/appshell"
	"github.com/valueforvalue/DixieData/internal/config"
	"github.com/valueforvalue/DixieData/internal/debug"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8000", "HTTP listen address")
	scratchDir := flag.String("scratch-dir", defaultScratchDir(), "Directory for the ephemeral data store (database, images, logs). Override DIXIEDATA_DATA_DIR to change.")
	flag.Parse()

	// Force the App to use a sandboxed data dir even if the developer has a
	// real .dixiedata next to their checkout. Audit runs must not touch it.
	dataDir, err := filepath.Abs(*scratchDir)
	if err != nil {
		log.Fatalf("resolve scratch dir: %v", err)
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Fatalf("create scratch dir %s: %v", dataDir, err)
	}
	if err := os.Setenv("DIXIEDATA_DATA_DIR", dataDir); err != nil {
		log.Fatalf("set DIXIEDATA_DATA_DIR: %v", err)
	}
	// Re-resolve after env var is set so the app picks up the override.
	dataDir = appdata.DefaultDir()
	log.Printf("dixiedata-web: data dir = %s", dataDir)

	app := appshell.NewApp()
	// Intentionally NOT calling WithFrontendAssets — appshell falls back to
	// reading app.js/app.css from ./frontend/ on disk, which keeps edits
	// in app.js live during audit iterations without a rebuild.

	// Optional: DIXIE_OPEN_FILE_DIALOG_PATH=/path/to/file.ddbak wires
	// the open-file-dialog override so headless browser probes can
	// drive the .ddbak restore flow end-to-end. Used by
	// audit/probe-full-restore.mjs to verify the layout-split fix.
	if dialogPath := os.Getenv("DIXIE_OPEN_FILE_DIALOG_PATH"); strings.TrimSpace(dialogPath) != "" {
		// Capture by value so the closure stays stable if the env
		// var changes later.
		captured := dialogPath
		app.SetOpenFileDialogOverride(func(_ any) (string, error) {
			return captured, nil
		})
		log.Printf("dixiedata-web: OpenFileDialog override wired to %s", captured)
	}

	// Export destinations: web-mode has no native SaveFileDialog, so the
	// audit smoke harness and manual browser smoke tests would otherwise
	// hit the errWailsFrontendUnavailable path and be redirected back to
	// /share instead of /jobs/{id}. Wire a synthetic picker that
	// auto-routes every export into <DIXIE_SAVE_FILE_DIR> (defaulting to
	// <dataDir>/exports/). Each override call computes the destination
	// from the SaveDialogOptions the handler passed — DefaultFilename
	// drives the filename, while a per-call counter avoids clobbering
	// when the harness fires several exports in a row.
	saveDir := strings.TrimSpace(os.Getenv("DIXIE_SAVE_FILE_DIR"))
	if saveDir == "" {
		saveDir = filepath.Join(dataDir, "exports")
	}
	if err := os.MkdirAll(saveDir, 0o755); err != nil {
		log.Fatalf("create save-file dir %s: %v", saveDir, err)
	}
	var exportSeq atomic.Uint64
	saveDir = saveDir // capture for closure
	app.SetSaveFileDialogOverride(func(opts any) (string, error) {
		defaultName := "dixiedata-export.bin"
		if wailsOpts, ok := opts.(wailsruntime.SaveDialogOptions); ok && strings.TrimSpace(wailsOpts.DefaultFilename) != "" {
			defaultName = wailsOpts.DefaultFilename
		}
		// Append a per-call sequence so successive exports don't overwrite
		// each other in the audit harness.
		seq := exportSeq.Add(1)
		ext := filepath.Ext(defaultName)
		base := strings.TrimSuffix(defaultName, ext)
		return filepath.Join(saveDir, fmt.Sprintf("%s-%d%s", base, seq, ext)), nil
	})
	log.Printf("dixiedata-web: SaveFileDialog override wired to %s", saveDir)

	// Optional: DIXIE_OPEN_DIRECTORY_DIALOG_PATH=/some/dir wires the
	// directory-picker override so headless browser probes can drive
	// the "Download images to folder" + "Choose where to copy record
	// images" flows end-to-end. Without this, those handlers fall
	// back to errWailsFrontendUnavailable and the user sees an
	// uninformative toast. Used by audit/smoke.mjs Phase 2.
	if dirPath := strings.TrimSpace(os.Getenv("DIXIE_OPEN_DIRECTORY_DIALOG_PATH")); dirPath != "" {
		captured := dirPath
		app.SetOpenDirectoryDialogOverride(func(_ any) (string, error) {
			return captured, nil
		})
		log.Printf("dixiedata-web: OpenDirectoryDialog override wired to %s", captured)
	}

	// Optional: DIXIE_BROWSER_OPEN_URL_LOG=/path/to/log.txt makes
	// BrowserOpenURL record the requested URL into the file (one
	// URL per line) instead of failing in web-mode. Used by
	// audit/smoke.mjs to verify the "Open result" + "Open log
	// folder" flows land a file:// URL into the override file. The
	// audit harness then reads the file and asserts the URL is the
	// one the handler intended to open. Without this hook the user
	// sees an info-toast "Open in OS file manager" fallback and
	// the audit harness has no way to assert correctness.
	if logPath := strings.TrimSpace(os.Getenv("DIXIE_BROWSER_OPEN_URL_LOG")); logPath != "" {
		captured := logPath
		app.SetBrowserOpenURLOverride(func(rawURL string) error {
			f, err := os.OpenFile(captured, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
			if err != nil {
				return err
			}
			defer debug.DeferCloseLog(f, "main.BrowserOpenURLOverride.url-log")
			if _, err := f.WriteString(rawURL + "\n"); err != nil {
				return err
			}
			return nil
		})
		log.Printf("dixiedata-web: BrowserOpenURL override logging to %s", captured)
	}

	app.Startup(context.Background())

	mux := http.NewServeMux()
	mux.Handle("/", app)

	server := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("dixiedata-web: serving on http://%s (Ctrl+C to stop)", *addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server: %v", err)
		}
	}()

	<-stop
	log.Printf("dixiedata-web: shutting down")

	// Issue #660: read the configured shutdown timeout
	// (cfg.Timing.ShutdownTimeoutS, default 5s). The CLI
	// doesn't load config.json directly so we use
	// config.Defaults() — the same default the appshell
	// would see with no config.json present.
	shutdownTimeout := time.Duration(config.Defaults().Timing.ShutdownTimeoutS) * time.Second
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	app.Shutdown(shutdownCtx)

	fmt.Println("dixiedata-web: bye")
}

// defaultScratchDir returns the canonical scratch-dir path
// dixiedata-web uses when -scratch-dir is not provided. The
// env override + the .scratch/webmode anchor-in-repo-root
// default are documented in the package-level comment so
// audit playbooks can rely on a stable path.
//
// The path is ALWAYS anchored in the repo root (via
// appdata.ProjectRoot()) so a process started from
// ~/Downloads still lands the scratch dir under the repo,
// not somewhere the caller's cwd leads. This eliminates
// the "scratch landed in the wrong place" class of bug
// (notably the 2026-07 confusion where the live archive at
// ~/.dixiedata was being mutated instead of the audit
// harness's ./scratch/webmode).
//
// .scratch/ is .gitignored. Never point this at the Wails
// binary's canonical .dixiedata/ archive; the audit
// harness is required to start with `-scratch-dir` set to
// a non-live path so the seeded fixture doesn't overwrite
// the user's real data. The Playwright probe
// `audit/_probe-scratch-vs-live.mjs` (added together with
// this) catches the inverse mistake (running against the
// live archive with mutation-side-effect scripts).
func defaultScratchDir() string {
	if v := strings.TrimSpace(os.Getenv("DIXIEDATA_WEB_SCRATCH_DIR")); v != "" {
		return v
	}
	root, err := appdata.ProjectRoot()
	if err != nil {
		// Fall back to the cwd-relative default; the
		// error path is well-tested by the existing
		// smoke probes and the failure mode is loud
		// (the user sees the wrong dir, immediately
		// notices, sets DIXIEDATA_WEB_SCRATCH_DIR).
		return filepath.Join(".scratch", "webmode")
	}
	return filepath.Join(root, ".scratch", "webmode")
}