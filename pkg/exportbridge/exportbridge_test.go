package exportbridge

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

// TestPrintSettingsFromFormDefaults pins the appshell's "no
// selections" form values as a valid PrintSettings. This is the
// baseline that dixiedata-tune's print-defaults output must
// reproduce byte-for-byte.
func TestPrintSettingsFromFormDefaults(t *testing.T) {
	values := url.Values{}
	settings, err := PrintSettingsFromForm(values)
	if err != nil {
		t.Fatalf("PrintSettingsFromForm: %v", err)
	}
	if settings.SortBy != "last_name" {
		t.Fatalf("SortBy = %q, want last_name (Normalize default)", settings.SortBy)
	}
	if settings.Scope != "all" {
		t.Fatalf("Scope = %q, want all (Normalize default for empty scope)", settings.Scope)
	}
	if !settings.ExportAll {
		t.Fatalf("ExportAll = false, want true (Normalize default for scope=all)")
	}
}

// TestPrintSettingsFromFormSelectedWithoutIDs documents the
// actual contract: Normalize silently rewrites scope=selected
// with no selected_ids to scope=all. The appshell's check at
// parsePrintSettingsRequest was dead code because Normalize had
// already collapsed the case. We mirror that contract here.
func TestPrintSettingsFromFormSelectedWithoutIDs(t *testing.T) {
	values := url.Values{}
	values.Set("scope", "selected")
	settings, err := PrintSettingsFromForm(values)
	if err != nil {
		t.Fatalf("PrintSettingsFromForm: %v", err)
	}
	if settings.Scope != "all" {
		t.Fatalf("Scope = %q, want all (Normalize collapses empty selected)", settings.Scope)
	}
	if !settings.ExportAll {
		t.Fatalf("ExportAll = false, want true after Normalize collapse")
	}
}

// TestPrintSettingsFromFormSelectedWithIDs exercises the success
// path: scope=selected with valid IDs builds a PrintSettings
// carrying them through.
func TestPrintSettingsFromFormSelectedWithIDs(t *testing.T) {
	values := url.Values{}
	values.Set("scope", "selected")
	values.Add("selected_ids", "54")
	values.Add("selected_ids", "55")
	settings, err := PrintSettingsFromForm(values)
	if err != nil {
		t.Fatalf("PrintSettingsFromForm: %v", err)
	}
	if settings.Scope != "selected" {
		t.Fatalf("Scope = %q, want selected", settings.Scope)
	}
	if len(settings.SelectedIDs) != 2 {
		t.Fatalf("len(SelectedIDs) = %d, want 2", len(settings.SelectedIDs))
	}
	if settings.SelectedIDs[0] != 54 || settings.SelectedIDs[1] != 55 {
		t.Fatalf("SelectedIDs = %v, want [54 55]", settings.SelectedIDs)
	}
}

// TestPrintSettingsFromFormGroupByFlags verifies the four group
// checkboxes map to the corresponding PrintSettings booleans.
func TestPrintSettingsFromFormGroupByFlags(t *testing.T) {
	values := url.Values{}
	values.Set("group_by_unit", "1")
	values.Set("group_by_pension_state", "1")
	// Confed and BuriedIn left absent.
	settings, err := PrintSettingsFromForm(values)
	if err != nil {
		t.Fatalf("PrintSettingsFromForm: %v", err)
	}
	if !settings.GroupByUnit || !settings.GroupByPensionState {
		t.Fatalf("GroupByUnit=%v GroupByPensionState=%v, want both true",
			settings.GroupByUnit, settings.GroupByPensionState)
	}
	if settings.GroupByConfederateHomeStatus || settings.GroupByBuriedIn {
		t.Fatalf("GroupByConfed=%v GroupByBuriedIn=%v, want both false",
			settings.GroupByConfederateHomeStatus, settings.GroupByBuriedIn)
	}
}

// TestPDFOptionsFromFormDefaults exercises the per-record PDF
// options parsing path that the appshell uses for the single-
// record soldier export.
func TestPDFOptionsFromFormDefaults(t *testing.T) {
	values := url.Values{}
	opts := PDFOptionsFromForm(values, "L", true)
	if opts.Orientation != "L" {
		t.Fatalf("Orientation = %q, want L (Normalize default)", opts.Orientation)
	}
	if !opts.IncludeImages {
		t.Fatalf("IncludeImages = false, want true (default)")
	}
}

// TestPDFOptionsFromFormIncludesImages verifies the bool form
// value path. Empty value defaults; explicit "1" sets true;
// explicit "0" sets false.
func TestPDFOptionsFromFormIncludesImages(t *testing.T) {
	cases := []struct {
		raw      string
		fallback bool
		want     bool
	}{
		{"", true, true},
		{"", false, false},
		{"1", false, true},
		{"0", true, false},
		{"true", false, true},
		{"false", true, false},
	}
	for _, c := range cases {
		values := url.Values{}
		if c.raw != "" {
			values.Set("include_images", c.raw)
		}
		got := PDFOptionsFromForm(values, "L", c.fallback).IncludeImages
		if got != c.want {
			t.Errorf("raw=%q fallback=%v: IncludeImages = %v, want %v",
				c.raw, c.fallback, got, c.want)
		}
	}
}

// TestNewBulkRendererCreatesMissingDB documents that
// NewBulkRenderer creates a fresh SQLite file at the given path
// (the SQLite driver auto-creates parent directories). This is the
// expected appshell behaviour too; the appshell's lifecycle.go
// calls db.Open against a path that may not exist yet during
// first-run setup.
func TestNewBulkRendererCreatesMissingDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "subdir", "fresh.db")
	dataDir := t.TempDir()
	r, err := NewBulkRenderer(dbPath, dataDir)
	if err != nil {
		t.Fatalf("NewBulkRenderer: %v", err)
	}
	defer r.Close()
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("expected SQLite file at %q: %v", dbPath, err)
	}
}