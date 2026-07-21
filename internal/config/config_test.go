package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaults_AllFieldsNonZero(t *testing.T) {
	cfg := Defaults()

	// Window
	if cfg.Window.Width != 1280 {
		t.Errorf("Window.Width = %d, want 1280", cfg.Window.Width)
	}
	if cfg.Window.Height != 800 {
		t.Errorf("Window.Height = %d, want 800", cfg.Window.Height)
	}

	// UI
	if cfg.UI.ToastDurationMs != 4000 {
		t.Errorf("UI.ToastDurationMs = %d, want 4000", cfg.UI.ToastDurationMs)
	}
	if cfg.UI.LandingPage != "/calendar" {
		t.Errorf("UI.LandingPage = %q, want /calendar", cfg.UI.LandingPage)
	}

	// Limits — spot-check key values
	if cfg.Limits.BrowseDefaultPageSize != 100 {
		t.Errorf("Limits.BrowseDefaultPageSize = %d, want 100", cfg.Limits.BrowseDefaultPageSize)
	}
	if cfg.Limits.MaxRetainedBackups != 5 {
		t.Errorf("Limits.MaxRetainedBackups = %d, want 5", cfg.Limits.MaxRetainedBackups)
	}
	if cfg.Limits.JobsConcurrency != 2 {
		t.Errorf("Limits.JobsConcurrency = %d, want 2", cfg.Limits.JobsConcurrency)
	}

	// Timing
	if cfg.Timing.JobsPollMs != 3000 {
		t.Errorf("Timing.JobsPollMs = %d, want 3000", cfg.Timing.JobsPollMs)
	}
	if cfg.Timing.ShutdownTimeoutS != 5 {
		t.Errorf("Timing.ShutdownTimeoutS = %d, want 5", cfg.Timing.ShutdownTimeoutS)
	}

	// PDF
	if cfg.PDF.Paper != "us-letter" {
		t.Errorf("PDF.Paper = %q, want us-letter", cfg.PDF.Paper)
	}

	// Google
	if cfg.Google.CalendarName != "DixieData" {
		t.Errorf("Google.CalendarName = %q, want DixieData", cfg.Google.CalendarName)
	}

	// Theme — palette
	if cfg.Theme.Palette["accent"] != "#8d7440" {
		t.Errorf("Theme.Palette[accent] = %q", cfg.Theme.Palette["accent"])
	}
}

func TestLoad_MissingFileReturnsDefaults(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load missing file: %v", err)
	}
	if cfg.Window.Width != 1280 {
		t.Errorf("missing file: Width = %d, want 1280", cfg.Window.Width)
	}
}

func TestLoad_PartialFileMergesDefaults(t *testing.T) {
	dir := t.TempDir()
	// Write a partial config — only window size overridden.
	partial := `{"window": {"width": 1920, "height": 1080}}`
	path := Path(dir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(partial), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load partial: %v", err)
	}
	if cfg.Window.Width != 1920 {
		t.Errorf("partial: Width = %d, want 1920", cfg.Window.Width)
	}
	if cfg.Window.Height != 1080 {
		t.Errorf("partial: Height = %d, want 1080", cfg.Window.Height)
	}
	// Unset keys should keep defaults.
	if cfg.UI.LandingPage != "/calendar" {
		t.Errorf("partial: LandingPage = %q, want /calendar", cfg.UI.LandingPage)
	}
	if cfg.Limits.BrowseDefaultPageSize != 100 {
		t.Errorf("partial: BrowseDefaultPageSize = %d, want 100", cfg.Limits.BrowseDefaultPageSize)
	}
}

func TestSaveAndLoad_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	cfg := Defaults()
	cfg.Window.Width = 1024
	cfg.Window.Height = 768
	cfg.UI.ToastDurationMs = 2000
	cfg.Limits.RecentRecordsCap = 20

	if err := Save(dir, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Window.Width != 1024 {
		t.Errorf("roundtrip Width: %d", loaded.Window.Width)
	}
	if loaded.UI.ToastDurationMs != 2000 {
		t.Errorf("roundtrip ToastDurationMs: %d", loaded.UI.ToastDurationMs)
	}
	if loaded.Limits.RecentRecordsCap != 20 {
		t.Errorf("roundtrip RecentRecordsCap: %d", loaded.Limits.RecentRecordsCap)
	}
	// Unchanged keys should still have defaults.
	if loaded.UI.LandingPage != "/calendar" {
		t.Errorf("roundtrip LandingPage: %q", loaded.UI.LandingPage)
	}
}

func TestForClient_ExcludesServerOnly(t *testing.T) {
	cfg := Defaults()
	client := cfg.ForClient()

	// Client-visible values should be present.
	if client.ToastDurationMs != 4000 {
		t.Errorf("client ToastDurationMs: %d", client.ToastDurationMs)
	}
	if client.LandingPage != "/calendar" {
		t.Errorf("client LandingPage: %q", client.LandingPage)
	}
	if client.RecentRecordsCap != 10 {
		t.Errorf("client RecentRecordsCap: %d", client.RecentRecordsCap)
	}
	if client.JobsPollMs != 3000 {
		t.Errorf("client JobsPollMs: %d", client.JobsPollMs)
	}
	if client.PDFPaper != "us-letter" {
		t.Errorf("client PDFPaper: %q", client.PDFPaper)
	}

	// Server-only values should NOT be present on ClientConfig.
	// Verify by marshaling to JSON and checking absence of keys.
	data, err := json.Marshal(client)
	if err != nil {
		t.Fatal(err)
	}
	jsonStr := string(data)
	for _, forbidden := range []string{"window", "updateCheckTimeoutS", "shutdownTimeoutS"} {
		if contains(jsonStr, forbidden) {
			t.Errorf("ClientConfig JSON contains server-only key %q", forbidden)
		}
	}
}

func TestMergeConfig_ThemeNonDestructive(t *testing.T) {
	dst := Defaults()
	src := Config{
		Theme: ThemeConfig{
			Palette: map[string]string{
				"accent": "#ff0000",
			},
		},
	}
	mergeConfig(&dst, &src)
	// Overridden key.
	if dst.Theme.Palette["accent"] != "#ff0000" {
		t.Errorf("accent not merged: %q", dst.Theme.Palette["accent"])
	}
	// Non-overridden keys should survive.
	if dst.Theme.Palette["text_primary"] != "#22303d" {
		t.Errorf("text_primary overwritten: %q", dst.Theme.Palette["text_primary"])
	}
}

func TestPath_AtStateRoot(t *testing.T) {
	got := Path("/home/user/.dixiedata")
	want := filepath.Join("/home/user/.dixiedata-state", "config.json")
	if got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
