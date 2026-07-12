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

// TestLocalSettings_ResolvedTheme pins the empty-string default
// resolution so callers don't have to repeat the fallback.
func TestLocalSettings_ResolvedTheme(t *testing.T) {
	cases := []struct {
		in   LocalSettings
		want string
	}{
		{LocalSettings{Theme: ""}, ThemeDefault},
		{LocalSettings{Theme: ThemeDefault}, ThemeDefault},
		{LocalSettings{Theme: ThemeHighContrast}, ThemeHighContrast},
		{LocalSettings{Theme: ThemeSoft}, ThemeSoft},
	}
	for _, c := range cases {
		if got := c.in.ResolvedTheme(); got != c.want {
			t.Errorf("ResolvedTheme(%q) = %q, want %q", c.in.Theme, got, c.want)
		}
	}
}