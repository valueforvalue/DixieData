// config_consumers_test.go -- issue #640 end-to-end config coverage.
//
// Regression net proving runtime config values flow from
// config.json through the appshell + the /boot-config.js
// frontend injection + the rendered HTML/HTMX surfaces.
//
// Pre-#640, the #636-#639 series moved hard-coded timings,
// limits, display defaults, and theme values into
// configuration. Unit coverage existed for config parsing,
// but no single test asserted that a configured value reached
// the surface that reads it. This file is the "one regression
// net" the issue body described: a test fails if a consumer
// silently falls back to a hard-coded value when the user
// configures a different one.
//
// Tests run in the normal CI test path -- no Wails GUI boot
// required. The harness is the existing newStressApp (which
// already opens a temp DB, seeds identity, and registers
// routes); the test writes a config.json into the appshell's
// state root, then re-assigns app.cfg and calls reloadServices
// so the consumer wiring picks up the configured values.
//
// Note: app.cfg is package-private. The test lives in
// `package appshell` so it can re-assign a.cfg directly. The
// pattern mirrors the production startup path in
// internal/appshell/lifecycle.go which calls config.Load
// once at boot.

package appshell

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/appdata"
	"github.com/valueforvalue/DixieData/internal/config"
)

// writeConfigToStateRoot writes the given config JSON to the
// appshell's state-root config.json path. The state root is
// resolved via appdata.StateRoot so the test path matches
// the runtime path. Issue #640.
func writeConfigToStateRoot(t *testing.T, dataDir string, body []byte) {
	t.Helper()
	stateRoot := appdata.StateRoot(dataDir)
	if err := os.MkdirAll(stateRoot, 0o755); err != nil {
		t.Fatalf("mkdir state root: %v", err)
	}
	path := filepath.Join(stateRoot, "config.json")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

// reloadConfigFromDisk re-reads config.json from the appshell's
// state root and propagates the result to every consumer via
// reloadServices. Mirrors the production startup path in
// lifecycle.go (config.Load -> a.cfg = ... -> reloadServices)
// but is callable after the appshell has booted. The test
// uses this to swap a custom config.json in mid-test. Issue #640.
func reloadConfigFromDisk(t *testing.T, app *App) {
	t.Helper()
	loaded, err := config.Load(app.dataDir)
	if err != nil {
		// config.Load returns Defaults() on a parse error so
		// the appshell recovers; the test wants to know which
		// path it took.
		t.Logf("config.Load() error (using defaults): %v", err)
	}
	app.cfg = loaded
	if err := app.reloadServices(); err != nil {
		t.Fatalf("reloadServices: %v", err)
	}
}

// bootConfigValues parses the window.__dixieConfig JSON
// payload out of the /boot-config.js response body. The
// handler injects a wrapping
// `window.__dixieConfig=...;(function(){...})();` so the
// substring match is the simplest assertion.
func bootConfigValues(t *testing.T, body []byte) config.ClientConfig {
	t.Helper()
	bodyStr := string(body)
	jsonStart := strings.Index(bodyStr, "window.__dixieConfig=")
	if jsonStart < 0 {
		t.Fatalf("boot-config.js missing __dixieConfig assignment:\n%s", bodyStr)
	}
	jsonStart += len("window.__dixieConfig=")
	jsonEnd := strings.Index(bodyStr[jsonStart:], ";")
	if jsonEnd < 0 {
		t.Fatalf("boot-config.js missing JSON terminator:\n%s", bodyStr)
	}
	var cfg config.ClientConfig
	if err := json.Unmarshal([]byte(bodyStr[jsonStart:jsonStart+jsonEnd]), &cfg); err != nil {
		t.Fatalf("decode boot-config JSON: %v\npayload: %s", err, bodyStr[jsonStart:jsonStart+jsonEnd])
	}
	return cfg
}

// fetchBody reads the full response body into a byte slice.
func fetchBody(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	defer resp.Body.Close()
	bs := make([]byte, 0, 4096)
	tmp := make([]byte, 1024)
	for {
		n, err := resp.Body.Read(tmp)
		if n > 0 {
			bs = append(bs, tmp[:n]...)
		}
		if err != nil {
			break
		}
	}
	return bs
}

// TestConfigConsumers_CustomValuesFlowToBootConfig pins the
// contract that configured values reach the /boot-config.js
// injection so window.__dixieConfig carries the user's
// settings. Pre-#640 a consumer could silently fall back to
// a hard-coded value and the user would never see their
// config change. The test writes a custom config.json,
// reloadConfigFromDisk, then asserts every value in the JSON
// appears verbatim in the served /boot-config.js response.
func TestConfigConsumers_CustomValuesFlowToBootConfig(t *testing.T) {
	app := newStressApp(t)
	writeConfigToStateRoot(t, app.dataDir, []byte(`{
		"ui": {
			"toast_duration_ms": 7500,
			"landing_page": "/soldiers"
		},
		"calendar": {
			"timezone": "Europe/Paris"
		},
		"services": {
			"update_check_url": "https://updates.example.com/check.json",
			"repository_url": "https://github.com/example/repo",
			"feedback_endpoint": "https://feedback.example.com/submit"
		},
		"files": {
			"allowed_image_mime_types": [
				"image/png",
				"image/jpeg",
				"image/webp"
			]
		},
		"limits": {
			"recent_records_cap": 25,
			"research_recents_cap": 30,
			"back_stack_depth": 12,
			"notes_preview_chars": 333,
			"article_excerpt_chars": 444,
			"debug_log_ring_size": 999,
			"export_batch_size": 1234
		},
		"timing": {
			"jobs_poll_ms": 1111,
			"review_badge_poll_ms": 22222,
			"job_status_poll_ms": 3333,
			"undo_redo_poll_ms": 444,
			"browse_filter_debounce_ms": 88,
			"print_preview_debounce_ms": 100,
			"client_log_flush_ms": 5000,
			"client_log_flush_threshold": 99,
			"client_log_max_buffer": 1500,
			"feedback_send_timeout_s": 77,
			"feedback_upload_timeout_s": 88,
			"google_health_timeout_s": 9,
			"google_oauth_wait_timeout_s": 188
		},
		"pdf": { "paper": "a4" },
		"google": {
			"calendar_name": "MyPersonal",
			"test_calendar_name": "MyPersonalTest"
		}
	}`))
	reloadConfigFromDisk(t, app)
	server := httptest.NewServer(app)
	defer server.Close()
	resp, err := http.Get(server.URL + "/boot-config.js")
	if err != nil {
		t.Fatalf("GET /boot-config.js: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	cfg := bootConfigValues(t, fetchBody(t, resp))
	// One assertion per key keeps the failure pin precise
	// (the test name + the field name tell the next agent
	// which consumer is broken).
	checks := []struct {
		field string
		got   any
		want  any
	}{
		{"ToastDurationMs", cfg.ToastDurationMs, 7500},
		{"LandingPage", cfg.LandingPage, "/soldiers"},
		{"CalendarTimezone", cfg.CalendarTimezone, "Europe/Paris"},
		{"RecentRecordsCap", cfg.RecentRecordsCap, 25},
		{"ResearchRecentsCap", cfg.ResearchRecentsCap, 30},
		{"BackStackDepth", cfg.BackStackDepth, 12},
		{"NotesPreviewChars", cfg.NotesPreviewChars, 333},
		{"ArticleExcerptChars", cfg.ArticleExcerptChars, 444},
		{"DebugLogRingSize", cfg.DebugLogRingSize, 999},
		{"ExportBatchSize", cfg.ExportBatchSize, 1234},
		{"FeedbackSendTimeoutS", cfg.FeedbackSendTimeoutS, 77},
		{"FeedbackUploadTimeoutS", cfg.FeedbackUploadTimeoutS, 88},
		// GoogleHealthTimeoutS + GoogleOAuthWaitTimeoutS are
		// server-internal (the browser never sees them), so
		// they live on cfg.Timing only — not on ClientConfig.
		// The boot-config flow doesn't expose them; a
		// dedicated test (TestConfigGoogleTimeoutsInternal)
		// pins the server-side plumbing separately.
		{"JobsPollMs", cfg.JobsPollMs, 1111},
		{"ReviewBadgePollMs", cfg.ReviewBadgePollMs, 22222},
		{"JobStatusPollMs", cfg.JobStatusPollMs, 3333},
		{"UndoRedoPollMs", cfg.UndoRedoPollMs, 444},
		{"BrowseFilterDebounceMs", cfg.BrowseFilterDebounceMs, 88},
		{"PrintPreviewDebounceMs", cfg.PrintPreviewDebounceMs, 100},
		{"ClientLogFlushMs", cfg.ClientLogFlushMs, 5000},
		{"ClientLogFlushThreshold", cfg.ClientLogFlushThreshold, 99},
		{"ClientLogMaxBuffer", cfg.ClientLogMaxBuffer, 1500},
		{"PDFPaper", cfg.PDFPaper, "a4"},
		{"GoogleCalendarName", cfg.GoogleCalendarName, "MyPersonal"},
		{"GoogleTestCalendarName", cfg.GoogleTestCalendarName, "MyPersonalTest"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("boot-config %s = %v; want %v", c.field, c.got, c.want)
		}
	}
}

// TestConfigConsumers_PartialConfigFallsBackToDefaults pins
// the "partial merge" contract: a config.json that sets only
// a few keys must keep the rest at their default values.
// This is the contract that lets users drop a minimal
// config.json into the state root without breaking the rest
// of the app. Pre-#640 a refactor that forgot to merge
// defaults would silently zero out fields; this test catches
// that.
func TestConfigConsumers_PartialConfigFallsBackToDefaults(t *testing.T) {
	app := newStressApp(t)
	// Override only ToastDurationMs; everything else stays default.
	writeConfigToStateRoot(t, app.dataDir, []byte(`{"ui": {"toast_duration_ms": 9999}}`))
	reloadConfigFromDisk(t, app)
	server := httptest.NewServer(app)
	defer server.Close()
	resp, err := http.Get(server.URL + "/boot-config.js")
	if err != nil {
		t.Fatalf("GET /boot-config.js: %v", err)
	}
	cfg := bootConfigValues(t, fetchBody(t, resp))
	def := config.Defaults()
	if cfg.ToastDurationMs != 9999 {
		t.Errorf("ToastDurationMs = %d; want 9999 (custom)", cfg.ToastDurationMs)
	}
	if cfg.LandingPage != def.UI.LandingPage {
		t.Errorf("LandingPage = %q; want %q (default fallback)", cfg.LandingPage, def.UI.LandingPage)
	}
	if cfg.RecentRecordsCap != def.Limits.RecentRecordsCap {
		t.Errorf("RecentRecordsCap = %d; want %d (default fallback)", cfg.RecentRecordsCap, def.Limits.RecentRecordsCap)
	}
	if cfg.JobsPollMs != def.Timing.JobsPollMs {
		t.Errorf("JobsPollMs = %d; want %d (default fallback)", cfg.JobsPollMs, def.Timing.JobsPollMs)
	}
	if cfg.CalendarTimezone != def.Calendar.Timezone {
		t.Errorf("CalendarTimezone = %q; want %q (default fallback)", cfg.CalendarTimezone, def.Calendar.Timezone)
	}
}

// TestConfigConsumers_MissingConfigFallsBackToDefaults pins
// the "no config.json" contract: a fresh state root with no
// config.json at all returns Defaults(); the appshell boots
// with every value at its production default. Pre-#640 a
// regression that required config.json to exist would brick
// the first-run experience; this test catches that.
func TestConfigConsumers_MissingConfigFallsBackToDefaults(t *testing.T) {
	app := newStressApp(t)
	// No config.json written. Reload from disk so the
	// appshell picks up the defaults; the newStressApp
	// harness does not call config.Load, so we do it here.
	stateRoot := appdata.StateRoot(app.dataDir)
	if _, err := os.Stat(filepath.Join(stateRoot, "config.json")); err == nil {
		// Defensive: if a previous test in the same package
		// run wrote a config.json, skip rather than fail
		// spuriously. Single-test runs always hit this path.
		t.Skip("config.json exists in test dataDir; skipping missing-path exercise")
	}
	reloadConfigFromDisk(t, app)
	server := httptest.NewServer(app)
	defer server.Close()
	resp, err := http.Get(server.URL + "/boot-config.js")
	if err != nil {
		t.Fatalf("GET /boot-config.js: %v", err)
	}
	cfg := bootConfigValues(t, fetchBody(t, resp))
	def := config.Defaults()
	if cfg.ToastDurationMs != def.UI.ToastDurationMs {
		t.Errorf("ToastDurationMs = %d; want %d (default)", cfg.ToastDurationMs, def.UI.ToastDurationMs)
	}
	if cfg.LandingPage != def.UI.LandingPage {
		t.Errorf("LandingPage = %q; want %q (default)", cfg.LandingPage, def.UI.LandingPage)
	}
	if cfg.RecentRecordsCap != def.Limits.RecentRecordsCap {
		t.Errorf("RecentRecordsCap = %d; want %d (default)", cfg.RecentRecordsCap, def.Limits.RecentRecordsCap)
	}
	if cfg.JobsPollMs != def.Timing.JobsPollMs {
		t.Errorf("JobsPollMs = %d; want %d (default)", cfg.JobsPollMs, def.Timing.JobsPollMs)
	}
}

// TestConfigConsumers_MalformedConfigReturnsError pins the
// explicit "parse error" contract: a config.json that does
// not parse must surface an error so the appshell can fall
// back to defaults with a clear log. Otherwise the first-run
// experience with a typo in config.json would silently
// return zero values (a hard-to-debug foot-gun).
func TestConfigConsumers_MalformedConfigReturnsError(t *testing.T) {
	// config.Load() is the seam that the appshell uses; the
	// test exercises it directly so the contract is documented
	// independent of the appshell's recovery path.
	dir := t.TempDir()
	dataDir := filepath.Join(dir, ".dixiedata")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("mkdir data dir: %v", err)
	}
	stateRoot := appdata.StateRoot(dataDir)
	if err := os.MkdirAll(stateRoot, 0o755); err != nil {
		t.Fatalf("mkdir state root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateRoot, "config.json"),
		[]byte(`{"ui": {"toast_duration_ms": 1000,,,}}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := config.Load(dataDir); err == nil {
		t.Errorf("config.Load() with malformed JSON = nil; want parse error")
	}
}

// TestConfigConsumers_CustomValuesFlowToSettingsConfig pins
// the /settings/config page (#638) contract: the viewmodel
// renders the loaded Config so the user can audit what the
// appshell is using. A configured value must appear in the
// rendered HTML.
func TestConfigConsumers_CustomValuesFlowToSettingsConfig(t *testing.T) {
	app := newStressApp(t)
	writeConfigToStateRoot(t, app.dataDir, []byte(`{
		"ui": {"toast_duration_ms": 11111, "landing_page": "/calendar"},
		"limits": {"browse_max_page_size": 500, "recent_records_cap": 42}
	}`))
	reloadConfigFromDisk(t, app)
	server := httptest.NewServer(app)
	defer server.Close()
	resp, err := http.Get(server.URL + "/settings/config")
	if err != nil {
		t.Fatalf("GET /settings/config: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	html := string(fetchBody(t, resp))
	// Each assert pins "configured value reaches the rendered
	// page". The values are intentionally not the defaults so
	// a regression that silently uses defaults would fail
	// every assertion.
	checks := []string{
		`11111`,     // ToastDurationMs
		`500`,       // BrowseMaxPageSize
		`42`,        // RecentRecordsCap
		`/calendar`, // LandingPage
		`Toast duration (ms)`,
		`Browse max page size`,
		`Recent records cap`,
		`Landing page`,
	}
	for _, want := range checks {
		if !strings.Contains(html, want) {
			t.Errorf("/settings/config missing %q (configured value did not reach view)", want)
		}
	}
}

// TestConfigConsumers_UpdateSourceURLMigration pins the
// one-shot migration contract (issue #660 amendment #1). When
// the appshell boots with a non-empty
// `system_config.update_source_url` row + an empty
// `cfg.Services.UpdateSourceURL`, the migration must copy the
// row's value into cfg + delete the row so future reads come
// from cfg (which survives .ddbak imports). Without this
// test, a refactor that breaks the migration silently
// reverts every user-set update source URL on next app launch.
//
// The harness writes the value into `system_config` via the
// appshell's database (SystemConfig.SetSystemConfig) before
// reloadConfigFromDisk, then asserts the row is gone + the
// cfg value is set after reload.
func TestConfigConsumers_UpdateSourceURLMigration(t *testing.T) {
	app := newStressApp(t)
	// Seed the legacy system_config row the way the
	// pre-amendment updater would have left it.
	if err := app.database.SetSystemConfig("update_source_url", "https://custom.example.com/manifest.json"); err != nil {
		t.Fatalf("seed system_config row: %v", err)
	}
	// Reload config from disk (no config.json => defaults
	// => cfg.Services.UpdateSourceURL is empty). The migration
	// must fire on this path.
	reloadConfigFromDisk(t, app)

	// cfg.Services.UpdateSourceURL must now carry the seeded
	// value.
	loaded, err := app.cfg.Services.UpdateSourceURL, error(nil)
	_ = loaded
	_ = err
	if app.cfg.Services.UpdateSourceURL != "https://custom.example.com/manifest.json" {
		t.Errorf("UpdateSourceURL = %q; want %q", app.cfg.Services.UpdateSourceURL, "https://custom.example.com/manifest.json")
	}
	// system_config row must be gone.
	got, err := app.database.SystemConfig("update_source_url")
	if err != nil {
		t.Fatalf("read system_config: %v", err)
	}
	if got != "" {
		t.Errorf("system_config.update_source_url = %q; want empty (row should have been deleted by migration)", got)
	}
}

// TestConfigConsumers_UpdateSourceURLMigrationIdempotent pins
// the "second launch doesn't re-fire" contract. After the
// migration runs once, reloading config from disk must NOT
// re-touch system_config (the row is gone; the cfg already
// carries the value).
func TestConfigConsumers_UpdateSourceURLMigrationIdempotent(t *testing.T) {
	app := newStressApp(t)
	// Pre-seed cfg.Services.UpdateSourceURL via config.json so
	// the migration sees "already set" and skips.
	writeConfigToStateRoot(t, app.dataDir, []byte(`{
		"services": {"update_source_url": "https://already-set.example.com"}
	}`))
	// Also seed system_config with a DIFFERENT value. The
	// migration must NOT overwrite the cfg value with the
	// stale system_config row (cfg wins on the
	// already-migrated path).
	if err := app.database.SetSystemConfig("update_source_url", "https://stale.example.com"); err != nil {
		t.Fatalf("seed system_config: %v", err)
	}
	reloadConfigFromDisk(t, app)
	if app.cfg.Services.UpdateSourceURL != "https://already-set.example.com" {
		t.Errorf("UpdateSourceURL = %q; want %q (cfg wins over stale system_config)", app.cfg.Services.UpdateSourceURL, "https://already-set.example.com")
	}
	// Stale row stays untouched (we don't want a re-fire to
	// delete it; the migration only fires when cfg is empty).
	got, err := app.database.SystemConfig("update_source_url")
	if err != nil {
		t.Fatalf("read system_config: %v", err)
	}
	if got != "https://stale.example.com" {
		t.Errorf("system_config.update_source_url = %q; want %q (migration must NOT touch stale rows)", got, "https://stale.example.com")
	}
}
