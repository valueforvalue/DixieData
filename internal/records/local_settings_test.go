package records

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadLocalSettings_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	want := LocalSettings{DebugMode: true, Theme: ThemeHighContrast}
	if err := SaveLocalSettings(dir, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := LoadLocalSettings(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.DebugMode != want.DebugMode {
		t.Errorf("DebugMode = %v, want %v", got.DebugMode, want.DebugMode)
	}
	if got.Theme != want.Theme {
		t.Errorf("Theme = %q, want %q", got.Theme, want.Theme)
	}
}

func TestLoadLocalSettings_NotExist(t *testing.T) {
	dir := t.TempDir()
	s, err := LoadLocalSettings(dir)
	if err != nil {
		t.Fatalf("Load on missing file: %v", err)
	}
	if s.DebugMode {
		t.Error("zero value should have DebugMode=false")
	}
	if s.Theme != "" {
		t.Errorf("zero value Theme = %q, want empty", s.Theme)
	}
}

// TestLocalSettingsPath_ResolvesUnderStateRoot pins that
// local_settings.json lives at the sibling-of-archive state root
// (NOT inside the archive dir). The .ddbak restore code path
// renames the archive dir; if the file lived inside, every
// restore would wipe the user's theme + debug-mode choice.
func TestLocalSettingsPath_ResolvesUnderStateRoot(t *testing.T) {
	dir := t.TempDir()
	got := LocalSettingsPath(dir)
	// state root is <parent-of-dir>/.dixiedata-state/
	wantParent := filepath.Dir(dir)
	want := filepath.Join(wantParent, ".dixiedata-state", "local_settings.json")
	if got != want {
		t.Errorf("LocalSettingsPath = %q, want %q", got, want)
	}
}

// TestLoadLocalSettings_MigratesFromLegacyPath pins the one-time
// migration that copies the file from the old location
// (<dataDir>/local_settings.json) to the new state-root location
// on first read. Without this, every existing user would lose
// their debug_mode setting across the path move.
func TestLoadLocalSettings_MigratesFromLegacyPath(t *testing.T) {
	dir := t.TempDir()
	legacyPath := filepath.Join(dir, "local_settings.json")
	legacy := LocalSettings{DebugMode: true, Theme: ThemeSoft}
	legacyData, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("Marshal legacy: %v", err)
	}
	if err := os.WriteFile(legacyPath, legacyData, 0o644); err != nil {
		t.Fatalf("seed legacy file: %v", err)
	}

	got, err := LoadLocalSettings(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !got.DebugMode {
		t.Error("migrated DebugMode = false, want true")
	}
	if got.Theme != ThemeSoft {
		t.Errorf("migrated Theme = %q, want %q", got.Theme, ThemeSoft)
	}

	// Confirm the new file was actually written to the state root
	// (not just read from the legacy path).
	newPath := LocalSettingsPath(dir)
	if _, err := os.Stat(newPath); err != nil {
		t.Errorf("expected migrated file at %q, got %v", newPath, err)
	}
	// Legacy file is left in place as a tombstone.
	if _, err := os.Stat(legacyPath); err != nil {
		t.Errorf("legacy tombstone missing at %q, got %v", legacyPath, err)
	}
}

// TestLocalSettings_ResolvedTheme pins the empty-string fallback
// resolution so callers don't have to repeat the fallback.
// Issue #494: empty-string now resolves to ThemeSoft (the new
// default for fresh installs). The persisted "default" value
// continues to resolve to ThemeClassic (the renamed Default).
func TestLocalSettings_ResolvedTheme(t *testing.T) {
	cases := []struct {
		in   LocalSettings
		want string
	}{
		{LocalSettings{Theme: ""}, ThemeSoft},
		{LocalSettings{Theme: "default"}, ThemeClassic},
		{LocalSettings{Theme: ThemeClassic}, ThemeClassic},
		{LocalSettings{Theme: ThemeHighContrast}, ThemeHighContrast},
		{LocalSettings{Theme: ThemeSoft}, ThemeSoft},
	}
	for _, c := range cases {
		if got := c.in.ResolvedTheme(); got != c.want {
			t.Errorf("ResolvedTheme(%q) = %q, want %q", c.in.Theme, got, c.want)
		}
	}
}

// TestLoadLocalSettings_ExportSurfaceRoundTrip pins issue #534:
// the per-user export.surface preference (jobs-page | toast-only)
// must survive a Save -> Load round-trip so a user who picks
// "Toast only" once keeps the preference across restarts. The
// dispatcher consults this on every export request.
func TestLoadLocalSettings_ExportSurfaceRoundTrip(t *testing.T) {
	dataDir := t.TempDir()
	in := LocalSettings{
		DebugMode:     true,
		Theme:         ThemeSoft,
		ExportSurface: "toast-only",
	}
	if err := SaveLocalSettings(dataDir, in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := LoadLocalSettings(dataDir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.ExportSurface != "toast-only" {
		t.Errorf("ExportSurface round-trip = %q; want %q", got.ExportSurface, "toast-only")
	}
}

// TestLoadLocalSettings_ExportSurfaceDefaultEmpty pins the
// zero-value contract: a fresh install (no local_settings.json)
// returns ExportSurface="", which the dispatcher MUST interpret
// as "jobs-page" (today's behavior). The default-on contract
// is what keeps the #533 parity fix stable for users who
// haven't picked the new preference yet.
func TestLoadLocalSettings_ExportSurfaceDefaultEmpty(t *testing.T) {
	got, err := LoadLocalSettings(t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.ExportSurface != "" {
		t.Errorf("zero-value ExportSurface = %q; want \"\" (jobs-page fallback)", got.ExportSurface)
	}
}

// TestResolvedExportSurface pins the fallback contract: empty
// value -> "jobs-page", "toast-only" -> "toast-only", unknown ->
// "jobs-page" (safe default). Mirrors ResolvedTheme's shape.
func TestResolvedExportSurface(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "jobs-page"},
		{"jobs-page", "jobs-page"},
		{"toast-only", "toast-only"},
		{"unknown-future-value", "jobs-page"},
	}
	for _, c := range cases {
		if got := ResolvedExportSurface(c.in); got != c.want {
			t.Errorf("ResolvedExportSurface(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestLoadLocalSettings_SupportEndpointRoundTrip pins issue
// #544 slice 2: the per-user support endpoint URL (the
// destination for "Send to support" uploads) survives a
// Save -> Load round-trip so a user who configures the
// endpoint once keeps it across restarts. The field is
// omitempty so old local_settings.json files load cleanly
// without the key.
func TestLoadLocalSettings_SupportEndpointRoundTrip(t *testing.T) {
	dataDir := t.TempDir()
	in := LocalSettings{
		DebugMode:       false,
		Theme:           ThemeSoft,
		ExportSurface:   "toast-only",
		SupportEndpoint: "https://support.example.invalid/upload",
	}
	if err := SaveLocalSettings(dataDir, in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := LoadLocalSettings(dataDir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.SupportEndpoint != "https://support.example.invalid/upload" {
		t.Errorf("SupportEndpoint round-trip = %q; want %q", got.SupportEndpoint, "https://support.example.invalid/upload")
	}
}

// TestLoadLocalSettings_SupportEndpointDefaultEmpty pins the
// zero-value contract: a fresh install returns
// SupportEndpoint="", which the UI MUST interpret as "feature
// off" -- the "Send to support" buttons render but a click
// surfaces a toast like 'Configure the support endpoint in
// Settings first' (per issue #544's locked design call).
// This is the same opt-in pattern as ExportSurface's default
// + ResolvedExportSurface's "" -> "jobs-page" fallback.
func TestLoadLocalSettings_SupportEndpointDefaultEmpty(t *testing.T) {
	got, err := LoadLocalSettings(t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.SupportEndpoint != "" {
		t.Errorf("zero-value SupportEndpoint = %q; want \"\" (feature off)", got.SupportEndpoint)
	}
}