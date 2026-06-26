package records

import (
	"path/filepath"
	"testing"
)

func TestLoadLocalSettings_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	want := LocalSettings{DebugMode: true}
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
}

func TestLoadLocalSettings_DefaultPath(t *testing.T) {
	got := LocalSettingsPath("X:/some/dir")
	want := filepath.Join("X:/some/dir", "local_settings.json")
	if got != want {
		t.Errorf("LocalSettingsPath = %q, want %q", got, want)
	}
}