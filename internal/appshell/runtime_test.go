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

