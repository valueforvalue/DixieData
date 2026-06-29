package appshell

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// errWailsFrontendUnavailable is returned by the guarded runtime
// wrappers when the appshell is running outside a real Wails
// frontend (web-mode audits, headless harnesses, tests, or any
// state where the lifecycle context never received the
// "frontend"/"logger"/"events" values Wails injects at startup).
//
// Without this guard the underlying wails runtime helpers call
// log.Fatalf, which translates to os.Exit(1) and kills the host
// process with no recoverable error. That's how the calendar PDF
// crash the user reported manifested: Export Month →
// SaveFileDialog(a.ctx, ...) → ctx had no "frontend" value →
// os.Exit.
var errWailsFrontendUnavailable = errors.New("wails frontend unavailable in this runtime (web-mode or pre-startup)")

// wailsHasFrontend reports whether ctx carries the "frontend" value
// Wails installs at startup. A nil ctx, a context.Background(), or
// any ctx never handed to OnStartup will report false. Used to
// short-circuit the runtime.* helpers before they reach their
// log.Fatalf bail-out path.
func wailsHasFrontend(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	return ctx.Value("frontend") != nil
}

// ctxHasFrontend is a package-internal alias used by handler
// logging. Exported to runtime_test.go so tests can probe the
// state of a.ctx at known points.
var ctxHasFrontend = wailsHasFrontend

// SaveFileDialog wraps wailsruntime.SaveFileDialog and returns a
// clear error instead of os.Exit when the frontend is unavailable.
// Cancel-by-user is still signalled via ("", errUserCancelled)
// below.
func (a *App) SaveFileDialog(opts wailsruntime.SaveDialogOptions) (string, error) {
	if a.saveFileDialogOverride != nil {
		return a.saveFileDialogOverride(opts)
	}
	if !wailsHasFrontend(a.ctx) {
		return "", errWailsFrontendUnavailable
	}
	return wailsruntime.SaveFileDialog(a.ctx, opts)
}

// OpenDirectoryDialog wraps wailsruntime.OpenDirectoryDialog.
func (a *App) OpenDirectoryDialog(opts wailsruntime.OpenDialogOptions) (string, error) {
	if !wailsHasFrontend(a.ctx) {
		return "", errWailsFrontendUnavailable
	}
	return wailsruntime.OpenDirectoryDialog(a.ctx, opts)
}

// OpenFileDialog wraps wailsruntime.OpenFileDialog.
func (a *App) OpenFileDialog(opts wailsruntime.OpenDialogOptions) (string, error) {
	if a.openFileDialogOverride != nil {
		return a.openFileDialogOverride(opts)
	}
	if !wailsHasFrontend(a.ctx) {
		return "", errWailsFrontendUnavailable
	}
	return wailsruntime.OpenFileDialog(a.ctx, opts)
}

// SetOpenFileDialogOverride installs a hook that replaces the
// wailsruntime.OpenFileDialog call. Returns the supplied path
// unchanged, simulating the user picking that file. Used by the
// web-mode binary to drive the .ddbak restore flow end-to-end
// without a real OS file picker; used by httptest to inject a
// known path without panicking through wailsruntime.
func (a *App) SetOpenFileDialogOverride(fn func(opts any) (string, error)) {
	a.openFileDialogOverride = fn
}

// SetSaveFileDialogOverride installs a hook that replaces the
// wailsruntime.SaveFileDialog call. Same role as
// SetOpenFileDialogOverride but for the export side. Used by
// the web-mode binary (cmd/dixiedata-web) to drive the export
// flow end-to-end without a real OS file picker, so the audit
// smoke harness and manual browser smoke tests can verify the
// full handler → enqueueExport → /jobs/{id} redirect chain.
//
// The closure receives the SaveDialogOptions so callers can
// inspect DefaultFilename / Filters and compute a destination
// path. Returning ("", nil) signals "user cancelled" — handlers
// translate that to a toast on the current page.
func (a *App) SetSaveFileDialogOverride(fn func(opts any) (string, error)) {
	a.saveFileDialogOverride = fn
}

// OpenMultipleFilesDialog wraps wailsruntime.OpenMultipleFilesDialog.
func (a *App) OpenMultipleFilesDialog(opts wailsruntime.OpenDialogOptions) ([]string, error) {
	if a.openMultipleFilesDialogOverride != nil {
		return a.openMultipleFilesDialogOverride(opts)
	}
	if !wailsHasFrontend(a.ctx) {
		return nil, errWailsFrontendUnavailable
	}
	return wailsruntime.OpenMultipleFilesDialog(a.ctx, opts)
}

// SetOpenMultipleFilesDialogOverride installs a hook that replaces
// the wailsruntime.OpenMultipleFilesDialog call. Mirrors
// SetOpenFileDialogOverride but for multi-select. Used by httptest
// to inject a known slice of paths without panicking through
// wailsruntime, and by the web-mode binary to drive image-import
// flows end-to-end without a real OS file picker.
func (a *App) SetOpenMultipleFilesDialogOverride(fn func(opts any) ([]string, error)) {
	a.openMultipleFilesDialogOverride = fn
}

// BrowserOpenURL wraps wailsruntime.BrowserOpenURL. The Wails
// runtime's BrowserOpenURL is a void function that calls
// log.Fatalf on a missing frontend, so without this guard the
// caller has no way to recover and the process exits.
func (a *App) BrowserOpenURL(rawURL string) error {
	if !wailsHasFrontend(a.ctx) {
		// Best-effort fallback: validate the URL first so callers can
		// distinguish "running in web-mode" (expected, ignore) from
		// "malformed URL" (real bug, surface to user). We deliberately
		// do NOT call os/exec to open the URL here — the appshell in
		// web-mode can't speak to a browser anyway, and the caller
		// will surface the error to the user.
		if _, err := url.Parse(rawURL); err != nil {
			return fmt.Errorf("debug: parse URL %q: %w", rawURL, err)
		}
		return errWailsFrontendUnavailable
	}
	wailsruntime.BrowserOpenURL(a.ctx, rawURL)
	return nil
}

// Quit wraps wailsruntime.Quit. Quit triggers an os.Exit so we
// guard it explicitly: never quit when the frontend is gone (the
// caller is mid-recovery and would otherwise terminate the host
// from under itself).
func (a *App) Quit() error {
	if !wailsHasFrontend(a.ctx) {
		return errWailsFrontendUnavailable
	}
	wailsruntime.Quit(a.ctx)
	return nil
}