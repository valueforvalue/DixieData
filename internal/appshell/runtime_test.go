package appshell

import (
	"context"
	"errors"
	"testing"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func TestWailsGuardsRejectMissingFrontend(t *testing.T) {
	app := NewApp()
	// a.ctx is nil (zero value). All guards must refuse without
	// touching the real wails runtime, which would call os.Exit.

	if _, err := app.SaveFileDialog(wailsruntime.SaveDialogOptions{}); !errors.Is(err, errWailsFrontendUnavailable) {
		t.Fatalf("SaveFileDialog: got %v want errWailsFrontendUnavailable", err)
	}
	if _, err := app.OpenFileDialog(wailsruntime.OpenDialogOptions{}); !errors.Is(err, errWailsFrontendUnavailable) {
		t.Fatalf("OpenFileDialog: got %v want errWailsFrontendUnavailable", err)
	}
	if _, err := app.OpenDirectoryDialog(wailsruntime.OpenDialogOptions{}); !errors.Is(err, errWailsFrontendUnavailable) {
		t.Fatalf("OpenDirectoryDialog: got %v want errWailsFrontendUnavailable", err)
	}
	if _, err := app.OpenMultipleFilesDialog(wailsruntime.OpenDialogOptions{}); !errors.Is(err, errWailsFrontendUnavailable) {
		t.Fatalf("OpenMultipleFilesDialog: got %v want errWailsFrontendUnavailable", err)
	}
	if err := app.BrowserOpenURL("file:///tmp/example.pdf"); !errors.Is(err, errWailsFrontendUnavailable) {
		t.Fatalf("BrowserOpenURL: got %v want errWailsFrontendUnavailable", err)
	}
	if err := app.Quit(); !errors.Is(err, errWailsFrontendUnavailable) {
		t.Fatalf("Quit: got %v want errWailsFrontendUnavailable", err)
	}
}

func TestWailsGuardsRejectContextBackground(t *testing.T) {
	app := NewApp()
	app.ctx = context.Background()

	if _, err := app.SaveFileDialog(wailsruntime.SaveDialogOptions{}); !errors.Is(err, errWailsFrontendUnavailable) {
		t.Fatalf("SaveFileDialog on Background: got %v want errWailsFrontendUnavailable", err)
	}
}

func TestBrowserOpenURLMalformedReturnsParseErrorNotFrontendSentinel(t *testing.T) {
	// WJ-4: BrowserOpenURL's web-mode fallback must distinguish
	// "no frontend" (expected) from "malformed URL" (real bug).
	// Previously both returned errWailsFrontendUnavailable.
	app := NewApp() // nil ctx → no frontend

	cases := []string{
		"://no-scheme",
		"http://[::1",  // unclosed bracket
		"%zz",          // invalid percent encoding
	}
	for _, raw := range cases {
		err := app.BrowserOpenURL(raw)
		if errors.Is(err, errWailsFrontendUnavailable) {
			t.Errorf("BrowserOpenURL(%q) returned errWailsFrontendUnavailable instead of parse error", raw)
		}
		if err == nil {
			t.Errorf("BrowserOpenURL(%q) returned nil; expected parse error", raw)
		}
	}
}

func TestOpenMultipleFilesDialogOverrideTakesPrecedenceOverGuard(t *testing.T) {
	// Phase-0 prerequisite for the image-import migration: the new
	// test hook must intercept the multi-file dialog BEFORE the
	// frontend guard fires so httptest can inject paths without a
	// Wails runtime.
	app := NewApp()
	want := []string{"/tmp/a.png", "/tmp/b.jpg"}
	app.SetOpenMultipleFilesDialogOverride(func(opts any) ([]string, error) {
		return want, nil
	})
	got, err := app.OpenMultipleFilesDialog(wailsruntime.OpenDialogOptions{})
	if err != nil {
		t.Fatalf("OpenMultipleFilesDialog via override: got %v want nil", err)
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("OpenMultipleFilesDialog via override: got %v want %v", got, want)
	}
}

func TestOpenMultipleFilesDialogWithoutOverrideStillReturnsFrontendSentinel(t *testing.T) {
	// Regression guard: without an override installed, the
	// multi-file dialog must still reject ctx-less calls so the
	// web-mode binary never panics through wailsruntime.
	app := NewApp()
	if _, err := app.OpenMultipleFilesDialog(wailsruntime.OpenDialogOptions{}); !errors.Is(err, errWailsFrontendUnavailable) {
		t.Fatalf("OpenMultipleFilesDialog without override: got %v want errWailsFrontendUnavailable", err)
	}
}

func TestOpenDirectoryDialogOverrideTakesPrecedenceOverGuard(t *testing.T) {
	// Phase-2: web-mode binary needs a hook so the
	// "Download images to folder" + "Choose where to copy record
	// images" flows can run end-to-end without a real OS
	// directory picker. The override must intercept BEFORE the
	// frontend guard fires (otherwise httptest can't inject a
	// destination path).
	app := NewApp()
	want := "/tmp/dixiedata-images-dest"
	app.SetOpenDirectoryDialogOverride(func(opts any) (string, error) {
		return want, nil
	})
	got, err := app.OpenDirectoryDialog(wailsruntime.OpenDialogOptions{})
	if err != nil {
		t.Fatalf("OpenDirectoryDialog via override: got %v want nil", err)
	}
	if got != want {
		t.Fatalf("OpenDirectoryDialog via override: got %q want %q", got, want)
	}
}

func TestOpenDirectoryDialogWithoutOverrideStillReturnsFrontendSentinel(t *testing.T) {
	// Regression guard: without an override installed, the
	// directory dialog must still reject ctx-less calls so the
	// web-mode binary never panics through wailsruntime.
	app := NewApp()
	if _, err := app.OpenDirectoryDialog(wailsruntime.OpenDialogOptions{}); !errors.Is(err, errWailsFrontendUnavailable) {
		t.Fatalf("OpenDirectoryDialog without override: got %v want errWailsFrontendUnavailable", err)
	}
}

func TestBrowserOpenURLOverrideTakesPrecedenceOverGuard(t *testing.T) {
	// Phase-2: web-mode binary needs a hook so the
	// "Open result" + "Open log folder" flows can capture the
	// file:// URL the handler wanted to open, instead of falling
	// back to the "no frontend" sentinel that the user can't
	// see. The override receives the raw URL and returns nil on
	// success.
	app := NewApp()
	var captured string
	app.SetBrowserOpenURLOverride(func(rawURL string) error {
		captured = rawURL
		return nil
	})
	if err := app.BrowserOpenURL("file:///tmp/example.pdf"); err != nil {
		t.Fatalf("BrowserOpenURL via override: got %v want nil", err)
	}
	if captured != "file:///tmp/example.pdf" {
		t.Fatalf("BrowserOpenURL via override: captured %q want %q", captured, "file:///tmp/example.pdf")
	}
}

func TestBrowserOpenURLOverrideCanSurfaceError(t *testing.T) {
	// The override is also the seam for surfacing "URL not
	// openable" errors to the user. Verify the override's
	// returned error propagates rather than being swallowed.
	app := NewApp()
	wantErr := errors.New("URL not openable in this environment")
	app.SetBrowserOpenURLOverride(func(rawURL string) error {
		return wantErr
	})
	if err := app.BrowserOpenURL("file:///tmp/example.pdf"); !errors.Is(err, wantErr) {
		t.Fatalf("BrowserOpenURL via override: got %v want %v", err, wantErr)
	}
}

