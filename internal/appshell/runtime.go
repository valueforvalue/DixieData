package appshell

import (
	"context"
	"errors"
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
	if !wailsHasFrontend(a.ctx) {
		return "", errWailsFrontendUnavailable
	}
	return wailsruntime.OpenFileDialog(a.ctx, opts)
}

// OpenMultipleFilesDialog wraps wailsruntime.OpenMultipleFilesDialog.
func (a *App) OpenMultipleFilesDialog(opts wailsruntime.OpenDialogOptions) ([]string, error) {
	if !wailsHasFrontend(a.ctx) {
		return nil, errWailsFrontendUnavailable
	}
	return wailsruntime.OpenMultipleFilesDialog(a.ctx, opts)
}

// BrowserOpenURL wraps wailsruntime.BrowserOpenURL. The Wails
// runtime's BrowserOpenURL is a void function that calls
// log.Fatalf on a missing frontend, so without this guard the
// caller has no way to recover and the process exits.
func (a *App) BrowserOpenURL(rawURL string) error {
	if !wailsHasFrontend(a.ctx) {
		// Best-effort fallback: validate the URL and let the caller
		// decide what to do. We deliberately do NOT call os/exec to
		// open the URL here — the appshell in web-mode can't speak
		// to a browser anyway, and the caller will surface the error
		// to the user.
		if _, err := url.Parse(rawURL); err != nil {
			return errWailsFrontendUnavailable
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