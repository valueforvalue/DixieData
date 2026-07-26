// Package config loads and saves the DixieData application
// configuration file (config.json). Config lives at the state
// root alongside local_settings.json — it is per-machine state,
// not archive data, so a .ddbak restore does not overwrite it.
//
// Config values are the user-tunable knobs that were previously
// hard-coded constants across the codebase: window size, calendar
// timezone, UI timing, retention limits, and pagination caps.
// Every value has a sensible default; a missing config.json
// is equivalent to all defaults.
//
// Pragmatic Programmer Tip #55: "Parameterize Your App Using
// External Configuration." Tip #25: "Keep Knowledge in Plain
// Text." The config file is plain JSON, human-readable, and
// lives in the application data directory.
//
// Refs issues #636, #637, #638, #639.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/valueforvalue/DixieData/internal/appdata"
)

// Config is the full application configuration. Every field has a
// JSON tag and a default value applied by Defaults() when the
// config file is missing. Zero-value Config is NOT valid — always
// use Defaults() or Load().
type Config struct {
	Window   WindowConfig   `json:"window"`
	UI       UIConfig       `json:"ui"`
	Calendar CalendarConfig `json:"calendar"`
	Services ServicesConfig `json:"services"`
	Files    FilesConfig    `json:"files"`
	Limits   LimitsConfig   `json:"limits"`
	Timing   TimingConfig   `json:"timing"`
	PDF      PDFConfig      `json:"pdf"`
	Google   GoogleConfig   `json:"google"`
	Theme    ThemeConfig    `json:"theme"`
}

// ServicesConfig holds external-service endpoints + the
// user-set update source URL (the latter was relocated from the
// SQLite `system_config` table in issue #660 amendment #1 so it
// survives .ddbak imports).
type ServicesConfig struct {
	UpdateCheckURL  string `json:"update_check_url"`
	UpdateSourceURL string `json:"update_source_url"`
	RepositoryURL   string `json:"repository_url"`
	FeedbackEndpoint string `json:"feedback_endpoint"`
}

// FilesConfig holds file-type allowlists. The image MIME list
// is the single source of truth for both the HTML form
// `accept` attribute and the backend validation.
type FilesConfig struct {
	AllowedImageMIMETypes []string `json:"allowed_image_mime_types"`
}

// WindowConfig controls the OS window at launch.
type WindowConfig struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// UIConfig controls user-visible display preferences.
type UIConfig struct {
	ToastDurationMs int    `json:"toast_duration_ms"`
	LandingPage     string `json:"landing_page"`
}

// CalendarConfig controls calendar display preferences.
type CalendarConfig struct {
	Timezone string `json:"timezone"`
}

// LimitsConfig controls caps and retention policies.
type LimitsConfig struct {
	BrowseDefaultPageSize    int `json:"browse_default_page_size"`
	BrowseMaxPageSize        int `json:"browse_max_page_size"`
	ListDefaultPageSize      int `json:"list_default_page_size"`
	ArticlePageSize          int `json:"article_page_size"`
	AuditPageSize            int `json:"audit_page_size"`
	MergeConflictsPageSize   int `json:"merge_conflicts_page_size"`
	RecentRecordsCap         int `json:"recent_records_cap"`
	ResearchRecentsCap       int `json:"research_recents_cap"`
	BackStackDepth           int `json:"back_stack_depth"`
	MaxRetainedBackups       int `json:"max_retained_backups"`
	MaxRestorePoints         int `json:"max_restore_points"`
	OrphanTrashRetentionDays int `json:"orphan_trash_retention_days"`
	FeedbackRetentionDays    int `json:"feedback_retention_days"`
	DuplicateAuditThreshold  int `json:"duplicate_audit_threshold"`
	JobsConcurrency          int `json:"jobs_concurrency"`
	NotesPreviewChars        int `json:"notes_preview_chars"`
	ArticleExcerptChars      int `json:"article_excerpt_chars"`
	DebugLogRingSize         int `json:"debug_log_ring_size"`
	ExportBatchSize          int `json:"export_batch_size"`
}

// TimingConfig controls polling intervals and debounce delays.
type TimingConfig struct {
	JobsPollMs                int `json:"jobs_poll_ms"`
	ReviewBadgePollMs         int `json:"review_badge_poll_ms"`
	JobStatusPollMs           int `json:"job_status_poll_ms"`
	UndoRedoPollMs            int `json:"undo_redo_poll_ms"`
	BrowseFilterDebounceMs    int `json:"browse_filter_debounce_ms"`
	PrintPreviewDebounceMs    int `json:"print_preview_debounce_ms"`
	UpdateCheckTimeoutS       int `json:"update_check_timeout_s"`
	ShutdownTimeoutS          int `json:"shutdown_timeout_s"`
	ClientLogFlushMs          int `json:"client_log_flush_ms"`
	ClientLogFlushThreshold   int `json:"client_log_flush_threshold"`
	ClientLogMaxBuffer        int `json:"client_log_max_buffer"`
	FeedbackSendTimeoutS      int `json:"feedback_send_timeout_s"`
	FeedbackUploadTimeoutS    int `json:"feedback_upload_timeout_s"`
	GoogleHealthTimeoutS      int `json:"google_health_timeout_s"`
	GoogleOAuthWaitTimeoutS   int `json:"google_oauth_wait_timeout_s"`
}

// PDFConfig controls PDF export defaults.
type PDFConfig struct {
	Paper   string       `json:"paper"`
	Margins MarginConfig `json:"margins"`
}

// MarginConfig is the page margin geometry for PDF exports.
type MarginConfig struct {
	Top    string `json:"top"`
	Bottom string `json:"bottom"`
	Left   string `json:"left"`
	Right  string `json:"right"`
}

// GoogleConfig controls Google Calendar integration labels.
type GoogleConfig struct {
	CalendarName     string `json:"calendar_name"`
	TestCalendarName string `json:"test_calendar_name"`
}

// ThemeConfig controls the visual theme tokens shared between
// CSS and Typst PDF rendering. This is the single source of
// truth for colors, type scale, and geometry.
//
// Issue #660 amendment #2 extended this with browser-palette
// + heading_color + blockquote_border + code_bg + heading_h{1..4}
// + body_prose + code_size so the markdown → typst converter
// can consume the same theme tokens the CSS uses.
type ThemeConfig struct {
	Palette        map[string]string     `json:"palette"`
	TypeScale      map[string]TypeSize   `json:"type_scale"`
	Fonts          FontConfig            `json:"fonts"`
	Branding       BrandingConfig        `json:"branding"`
	PaletteBrowser map[string]string     `json:"palette_browser"`
	FontsBrowser   FontConfigBrowser     `json:"fonts_browser"`
	TypeScaleBrowser map[string]TypeSize `json:"type_scale_browser"`
	PaletteActivity map[string]string    `json:"palette_activity"`
	PaletteCalendar map[string]string    `json:"palette_calendar"`
	HeadingColor   string                `json:"heading_color"`
	BlockquoteBorder string              `json:"blockquote_border"`
	CodeBackground string                `json:"code_background"`
}

// FontConfigBrowser controls the browser-side font stacks
// (issue #660). Distinct from the existing FontConfig because
// the browser uses platform-native stacks ("Helvetica Neue",
// Georgia, monospace) while the Typst PDF renderer uses
// deterministic installed-font names (Arial, Times New Roman,
// DejaVu Sans Mono).
type FontConfigBrowser struct {
	BodySans  string   `json:"body_sans"`
	BodySerif []string `json:"body_serif"`
	Mono      string   `json:"mono"`
}

// TypeSize is a font size + optional line height.
type TypeSize struct {
	SizePt float64 `json:"size_pt"`
	LinePt float64 `json:"line_pt,omitempty"`
}

// FontConfig controls the font stacks for PDF rendering.
type FontConfig struct {
	BodySans  string   `json:"body_sans"`
	BodySerif []string `json:"body_serif"`
	Mono      string   `json:"mono"`
}

// BrandingConfig controls PDF header/footer chrome text.
type BrandingConfig struct {
	HeaderSuffix   string `json:"header_suffix"`
	FooterTemplate string `json:"footer_template"`
}

// Defaults returns a Config with all values set to their
// production defaults (the current hard-coded values).
// This is the fallback when config.json is missing.
func Defaults() Config {
	return Config{
		Window: WindowConfig{
			Width:  1280,
			Height: 800,
		},
		UI: UIConfig{
			ToastDurationMs: 4000,
			LandingPage:     "/calendar",
		},
		Calendar: CalendarConfig{
			Timezone: "America/Chicago",
		},
		Services: ServicesConfig{
			UpdateCheckURL:   "https://api.github.com/repos/valueforvalue/DixieData/releases/latest",
			UpdateSourceURL:  "",
			RepositoryURL:    "https://github.com/valueforvalue/DixieData",
			FeedbackEndpoint: "https://submit-form.com/vJSONT1nB",
		},
		Files: FilesConfig{
			AllowedImageMIMETypes: []string{
				"image/png",
				"image/jpeg",
				"image/gif",
				"image/bmp",
				"image/webp",
				"image/svg+xml",
			},
		},
		Limits: LimitsConfig{
			BrowseDefaultPageSize:    100,
			BrowseMaxPageSize:        250,
			ListDefaultPageSize:      50,
			ArticlePageSize:          25,
			AuditPageSize:            25,
			MergeConflictsPageSize:   50,
			RecentRecordsCap:         10,
			ResearchRecentsCap:       10,
			BackStackDepth:           8,
			MaxRetainedBackups:       5,
			MaxRestorePoints:         2,
			OrphanTrashRetentionDays: 30,
			FeedbackRetentionDays:    365,
			DuplicateAuditThreshold:  2,
			JobsConcurrency:          2,
			NotesPreviewChars:        260,
			ArticleExcerptChars:      280,
			DebugLogRingSize:         500,
			ExportBatchSize:          500,
		},
		Timing: TimingConfig{
			JobsPollMs:               3000,
			ReviewBadgePollMs:        30000,
			JobStatusPollMs:          2000,
			UndoRedoPollMs:           500,
			BrowseFilterDebounceMs:   200,
			PrintPreviewDebounceMs:   150,
			UpdateCheckTimeoutS:      45,
			ShutdownTimeoutS:         5,
			ClientLogFlushMs:         2000,
			ClientLogFlushThreshold:  50,
			ClientLogMaxBuffer:       500,
			FeedbackSendTimeoutS:     35,
			FeedbackUploadTimeoutS:   30,
			GoogleHealthTimeoutS:     5,
			GoogleOAuthWaitTimeoutS:  120,
		},
		PDF: PDFConfig{
			Paper: "us-letter",
			Margins: MarginConfig{
				Top:    "0.4in",
				Bottom: "0.4in",
				Left:   "0.63in",
				Right:  "0.63in",
			},
		},
		Google: GoogleConfig{
			CalendarName:     "DixieData",
			TestCalendarName: "DixieData Test",
		},
		Theme: ThemeConfig{
			Palette: map[string]string{
				"accent":         "#8d7440",
				"accent_strong":  "#a88a46",
				"text_primary":   "#22303d",
				"text_secondary": "#445260",
				"text_muted":     "#71808e",
				"link":           "#4A90E2",
				"danger":         "#54211d",
				"divider":        "#8d7440",
				"panel_fill":     "#fff8e7",
			},
			TypeScale: map[string]TypeSize{
				"section_title": {SizePt: 9, LinePt: 6},
				"field_label":   {SizePt: 8, LinePt: 4.5},
				"field_value":   {SizePt: 9, LinePt: 4.5},
				"body":          {SizePt: 9, LinePt: 5},
				"biography":     {SizePt: 11, LinePt: 6},
				"header":        {SizePt: 10},
				"footer":        {SizePt: 8},
			},
			Fonts: FontConfig{
				BodySans:  "Arial",
				BodySerif: []string{"Times New Roman", "Liberation Serif", "DejaVu Serif"},
				Mono:      "DejaVu Sans Mono",
			},
			Branding: BrandingConfig{
				HeaderSuffix:   "'s Civil War Research Archive",
				FooterTemplate: "Made with DixieData | Version: {app_version} | Build: {build_identity}",
			},
			HeadingColor:     "#22303d",
			BlockquoteBorder: "#8d7440",
			CodeBackground:   "rgb(36 48 61 / 0.06)",
			PaletteBrowser: map[string]string{
				"ink":          "#22303d",
				"sepia":        "#8d7440",
				"parchment":    "#fff8e7",
				"panel":        "#ffffff",
				"border":       "rgb(141 116 64 / 0.35)",
				"text_muted":   "#71808e",
				"text_strong":  "#22303d",
				"link":         "#4A90E2",
				"danger":       "#54211d",
				"event":        "#7cb3e2",
				"holiday":      "#d98989",
				"today":        "#1f5b3b",
				"quote_text":   "#7d4f2d",
			},
			FontsBrowser: FontConfigBrowser{
				BodySans:  `"Helvetica Neue", Arial, sans-serif`,
				BodySerif: []string{`Georgia, "Times New Roman", serif`},
				Mono:      `"ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace"`,
			},
			TypeScaleBrowser: map[string]TypeSize{
				"h1":       {SizePt: 1.85, LinePt: 1.3},
				"h2":       {SizePt: 1.5,  LinePt: 1.25},
				"h3":       {SizePt: 1.25, LinePt: 1.2},
				"h4":       {SizePt: 1.1,  LinePt: 1.15},
				"body_prose": {SizePt: 1.0, LinePt: 1.6},
				"code_size":  {SizePt: 0.92, LinePt: 1.5},
			},
			PaletteActivity: map[string]string{
				"create":   "#a14747",
				"update":   "#7d4f2d",
				"merge":    "#4f7d6b",
				"share":    "#b6854f",
				"comment":  "#3f5d8a",
				"neutral":  "#71808e",
			},
			PaletteCalendar: map[string]string{
				"event":      "#7cb3e2",
				"holiday":    "#d98989",
				"today":      "#1f5b3b",
				"quote_text": "#7d4f2d",
				"border":     "rgb(141 116 64 / 0.3)",
				"fill":       "#fff8e7",
			},
		},
	}
}

// Path returns the absolute path to config.json at the state
// root (sibling of the data dir, alongside local_settings.json).
func Path(dataDir string) string {
	return filepath.Join(appdata.StateRoot(dataDir), "config.json")
}

// Load reads config.json and merges it with defaults. Missing
// file returns Defaults(). Malformed file returns the parse
// error. Partial files are merged: any key missing from the
// on-disk JSON keeps its default value.
func Load(dataDir string) (Config, error) {
	cfg := Defaults()
	path := Path(dataDir)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	// Unmarshal into a copy so partial files merge with defaults.
	var disk Config
	if err := json.Unmarshal(data, &disk); err != nil {
		return cfg, err
	}
	// Merge: non-zero disk values overwrite defaults.
	mergeConfig(&cfg, &disk)
	return cfg, nil
}

// Save writes config.json atomically (write temp + rename).
func Save(dataDir string, cfg Config) error {
	path := Path(dataDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// mergeConfig overlays non-zero values from src onto dst.
// Zero values in src are skipped so a partial config file
// keeps defaults for unset keys.
func mergeConfig(dst *Config, src *Config) {
	if src.Window.Width != 0 {
		dst.Window.Width = src.Window.Width
	}
	if src.Window.Height != 0 {
		dst.Window.Height = src.Window.Height
	}
	if src.UI.ToastDurationMs != 0 {
		dst.UI.ToastDurationMs = src.UI.ToastDurationMs
	}
	if src.UI.LandingPage != "" {
		dst.UI.LandingPage = src.UI.LandingPage
	}
	if src.Calendar.Timezone != "" {
		dst.Calendar.Timezone = src.Calendar.Timezone
	}
	if src.Services.UpdateCheckURL != "" {
		dst.Services.UpdateCheckURL = src.Services.UpdateCheckURL
	}
	// UpdateSourceURL is allowed to be empty (the default
	// is empty), so we merge only when src has it set
	// AND dst does not. The one-shot migration (see
	// appshell.reloadServices) seeds the dst value from
	// the legacy system_config row before this merge runs
	// for installs that pre-date the slice.
	if src.Services.UpdateSourceURL != "" {
		dst.Services.UpdateSourceURL = src.Services.UpdateSourceURL
	}
	if src.Services.RepositoryURL != "" {
		dst.Services.RepositoryURL = src.Services.RepositoryURL
	}
	if src.Services.FeedbackEndpoint != "" {
		dst.Services.FeedbackEndpoint = src.Services.FeedbackEndpoint
	}
	if len(src.Files.AllowedImageMIMETypes) > 0 {
		dst.Files.AllowedImageMIMETypes = src.Files.AllowedImageMIMETypes
	}
	if src.Limits.BrowseDefaultPageSize != 0 {
		dst.Limits.BrowseDefaultPageSize = src.Limits.BrowseDefaultPageSize
	}
	if src.Limits.BrowseMaxPageSize != 0 {
		dst.Limits.BrowseMaxPageSize = src.Limits.BrowseMaxPageSize
	}
	if src.Limits.ListDefaultPageSize != 0 {
		dst.Limits.ListDefaultPageSize = src.Limits.ListDefaultPageSize
	}
	if src.Limits.ArticlePageSize != 0 {
		dst.Limits.ArticlePageSize = src.Limits.ArticlePageSize
	}
	if src.Limits.AuditPageSize != 0 {
		dst.Limits.AuditPageSize = src.Limits.AuditPageSize
	}
	if src.Limits.MergeConflictsPageSize != 0 {
		dst.Limits.MergeConflictsPageSize = src.Limits.MergeConflictsPageSize
	}
	if src.Limits.RecentRecordsCap != 0 {
		dst.Limits.RecentRecordsCap = src.Limits.RecentRecordsCap
	}
	if src.Limits.ResearchRecentsCap != 0 {
		dst.Limits.ResearchRecentsCap = src.Limits.ResearchRecentsCap
	}
	if src.Limits.BackStackDepth != 0 {
		dst.Limits.BackStackDepth = src.Limits.BackStackDepth
	}
	if src.Limits.MaxRetainedBackups != 0 {
		dst.Limits.MaxRetainedBackups = src.Limits.MaxRetainedBackups
	}
	if src.Limits.MaxRestorePoints != 0 {
		dst.Limits.MaxRestorePoints = src.Limits.MaxRestorePoints
	}
	if src.Limits.OrphanTrashRetentionDays != 0 {
		dst.Limits.OrphanTrashRetentionDays = src.Limits.OrphanTrashRetentionDays
	}
	if src.Limits.FeedbackRetentionDays != 0 {
		dst.Limits.FeedbackRetentionDays = src.Limits.FeedbackRetentionDays
	}
	if src.Limits.DuplicateAuditThreshold != 0 {
		dst.Limits.DuplicateAuditThreshold = src.Limits.DuplicateAuditThreshold
	}
	if src.Limits.JobsConcurrency != 0 {
		dst.Limits.JobsConcurrency = src.Limits.JobsConcurrency
	}
	if src.Limits.NotesPreviewChars != 0 {
		dst.Limits.NotesPreviewChars = src.Limits.NotesPreviewChars
	}
	if src.Limits.ArticleExcerptChars != 0 {
		dst.Limits.ArticleExcerptChars = src.Limits.ArticleExcerptChars
	}
	if src.Limits.DebugLogRingSize != 0 {
		dst.Limits.DebugLogRingSize = src.Limits.DebugLogRingSize
	}
	if src.Limits.ExportBatchSize != 0 {
		dst.Limits.ExportBatchSize = src.Limits.ExportBatchSize
	}
	if src.Timing.JobsPollMs != 0 {
		dst.Timing.JobsPollMs = src.Timing.JobsPollMs
	}
	if src.Timing.ReviewBadgePollMs != 0 {
		dst.Timing.ReviewBadgePollMs = src.Timing.ReviewBadgePollMs
	}
	if src.Timing.JobStatusPollMs != 0 {
		dst.Timing.JobStatusPollMs = src.Timing.JobStatusPollMs
	}
	if src.Timing.UndoRedoPollMs != 0 {
		dst.Timing.UndoRedoPollMs = src.Timing.UndoRedoPollMs
	}
	if src.Timing.BrowseFilterDebounceMs != 0 {
		dst.Timing.BrowseFilterDebounceMs = src.Timing.BrowseFilterDebounceMs
	}
	if src.Timing.PrintPreviewDebounceMs != 0 {
		dst.Timing.PrintPreviewDebounceMs = src.Timing.PrintPreviewDebounceMs
	}
	if src.Timing.UpdateCheckTimeoutS != 0 {
		dst.Timing.UpdateCheckTimeoutS = src.Timing.UpdateCheckTimeoutS
	}
	if src.Timing.ShutdownTimeoutS != 0 {
		dst.Timing.ShutdownTimeoutS = src.Timing.ShutdownTimeoutS
	}
	if src.Timing.ClientLogFlushMs != 0 {
		dst.Timing.ClientLogFlushMs = src.Timing.ClientLogFlushMs
	}
	if src.Timing.ClientLogFlushThreshold != 0 {
		dst.Timing.ClientLogFlushThreshold = src.Timing.ClientLogFlushThreshold
	}
	if src.Timing.ClientLogMaxBuffer != 0 {
		dst.Timing.ClientLogMaxBuffer = src.Timing.ClientLogMaxBuffer
	}
	if src.Timing.FeedbackSendTimeoutS != 0 {
		dst.Timing.FeedbackSendTimeoutS = src.Timing.FeedbackSendTimeoutS
	}
	if src.Timing.FeedbackUploadTimeoutS != 0 {
		dst.Timing.FeedbackUploadTimeoutS = src.Timing.FeedbackUploadTimeoutS
	}
	if src.Timing.GoogleHealthTimeoutS != 0 {
		dst.Timing.GoogleHealthTimeoutS = src.Timing.GoogleHealthTimeoutS
	}
	if src.Timing.GoogleOAuthWaitTimeoutS != 0 {
		dst.Timing.GoogleOAuthWaitTimeoutS = src.Timing.GoogleOAuthWaitTimeoutS
	}
	if src.PDF.Paper != "" {
		dst.PDF.Paper = src.PDF.Paper
	}
	if src.PDF.Margins.Top != "" {
		dst.PDF.Margins = src.PDF.Margins
	}
	if src.Google.CalendarName != "" {
		dst.Google.CalendarName = src.Google.CalendarName
	}
	if src.Google.TestCalendarName != "" {
		dst.Google.TestCalendarName = src.Google.TestCalendarName
	}
	// Theme: merge palette, type-scale, fonts, branding
	// non-destructively — missing keys keep defaults.
	if src.Theme.Palette != nil {
		if dst.Theme.Palette == nil {
			dst.Theme.Palette = make(map[string]string)
		}
		for k, v := range src.Theme.Palette {
			dst.Theme.Palette[k] = v
		}
	}
	if src.Theme.TypeScale != nil {
		if dst.Theme.TypeScale == nil {
			dst.Theme.TypeScale = make(map[string]TypeSize)
		}
		for k, v := range src.Theme.TypeScale {
			dst.Theme.TypeScale[k] = v
		}
	}
	if src.Theme.Fonts.BodySans != "" {
		dst.Theme.Fonts.BodySans = src.Theme.Fonts.BodySans
	}
	if len(src.Theme.Fonts.BodySerif) > 0 {
		dst.Theme.Fonts.BodySerif = src.Theme.Fonts.BodySerif
	}
	if src.Theme.Fonts.Mono != "" {
		dst.Theme.Fonts.Mono = src.Theme.Fonts.Mono
	}
	if src.Theme.Branding.HeaderSuffix != "" {
		dst.Theme.Branding.HeaderSuffix = src.Theme.Branding.HeaderSuffix
	}
	if src.Theme.Branding.FooterTemplate != "" {
		dst.Theme.Branding.FooterTemplate = src.Theme.Branding.FooterTemplate
	}
	if src.Theme.HeadingColor != "" {
		dst.Theme.HeadingColor = src.Theme.HeadingColor
	}
	if src.Theme.BlockquoteBorder != "" {
		dst.Theme.BlockquoteBorder = src.Theme.BlockquoteBorder
	}
	if src.Theme.CodeBackground != "" {
		dst.Theme.CodeBackground = src.Theme.CodeBackground
	}
	if src.Theme.PaletteBrowser != nil {
		if dst.Theme.PaletteBrowser == nil {
			dst.Theme.PaletteBrowser = make(map[string]string)
		}
		for k, v := range src.Theme.PaletteBrowser {
			dst.Theme.PaletteBrowser[k] = v
		}
	}
	if src.Theme.FontsBrowser.BodySans != "" {
		dst.Theme.FontsBrowser.BodySans = src.Theme.FontsBrowser.BodySans
	}
	if len(src.Theme.FontsBrowser.BodySerif) > 0 {
		dst.Theme.FontsBrowser.BodySerif = src.Theme.FontsBrowser.BodySerif
	}
	if src.Theme.FontsBrowser.Mono != "" {
		dst.Theme.FontsBrowser.Mono = src.Theme.FontsBrowser.Mono
	}
	if src.Theme.TypeScaleBrowser != nil {
		if dst.Theme.TypeScaleBrowser == nil {
			dst.Theme.TypeScaleBrowser = make(map[string]TypeSize)
		}
		for k, v := range src.Theme.TypeScaleBrowser {
			dst.Theme.TypeScaleBrowser[k] = v
		}
	}
	if src.Theme.PaletteActivity != nil {
		if dst.Theme.PaletteActivity == nil {
			dst.Theme.PaletteActivity = make(map[string]string)
		}
		for k, v := range src.Theme.PaletteActivity {
			dst.Theme.PaletteActivity[k] = v
		}
	}
	if src.Theme.PaletteCalendar != nil {
		if dst.Theme.PaletteCalendar == nil {
			dst.Theme.PaletteCalendar = make(map[string]string)
		}
		for k, v := range src.Theme.PaletteCalendar {
			dst.Theme.PaletteCalendar[k] = v
		}
	}
}

// ClientConfig is the subset of Config exposed to the frontend
// via window.__dixieConfig. It excludes server-only values
// (window size, update timeout, shutdown timeout) that the
// frontend has no use for.
type ClientConfig struct {
	ToastDurationMs          int      `json:"toastDurationMs"`
	LandingPage              string   `json:"landingPage"`
	CalendarTimezone         string   `json:"calendarTimezone"`
	UpdateSourceURL          string   `json:"updateSourceUrl"`
	RepositoryURL            string   `json:"repositoryUrl"`
	FeedbackEndpoint         string   `json:"feedbackEndpoint"`
	AllowedImageMIMETypes    []string `json:"allowedImageMimeTypes"`
	RecentRecordsCap         int      `json:"recentRecordsCap"`
	ResearchRecentsCap       int      `json:"researchRecentsCap"`
	BackStackDepth           int      `json:"backStackDepth"`
	NotesPreviewChars        int      `json:"notesPreviewChars"`
	ArticleExcerptChars      int      `json:"articleExcerptChars"`
	DebugLogRingSize         int      `json:"debugLogRingSize"`
	ExportBatchSize          int      `json:"exportBatchSize"`
	JobsPollMs               int      `json:"jobsPollMs"`
	ReviewBadgePollMs        int      `json:"reviewBadgePollMs"`
	JobStatusPollMs          int      `json:"jobStatusPollMs"`
	UndoRedoPollMs           int      `json:"undoRedoPollMs"`
	BrowseFilterDebounceMs   int      `json:"browseFilterDebounceMs"`
	PrintPreviewDebounceMs   int      `json:"printPreviewDebounceMs"`
	ClientLogFlushMs         int      `json:"clientLogFlushMs"`
	ClientLogFlushThreshold  int      `json:"clientLogFlushThreshold"`
	ClientLogMaxBuffer       int      `json:"clientLogMaxBuffer"`
	FeedbackSendTimeoutS     int      `json:"feedbackSendTimeoutS"`
	FeedbackUploadTimeoutS   int      `json:"feedbackUploadTimeoutS"`
	PDFPaper                 string   `json:"pdfPaper"`
	GoogleCalendarName       string   `json:"googleCalendarName"`
	GoogleTestCalendarName   string   `json:"googleTestCalendarName"`
	ThemeBrowserPalette      map[string]string     `json:"themeBrowserPalette"`
	ThemeBrowserFonts        FontConfigBrowser     `json:"themeBrowserFonts"`
	ThemeBrowserTypeScale    map[string]TypeSize   `json:"themeBrowserTypeScale"`
	ThemeActivityPalette     map[string]string     `json:"themeActivityPalette"`
	ThemeCalendarPalette     map[string]string     `json:"themeCalendarPalette"`
	ThemeHeadingColor        string                 `json:"themeHeadingColor"`
	ThemeBlockquoteBorder    string                 `json:"themeBlockquoteBorder"`
	ThemeCodeBackground      string                 `json:"themeCodeBackground"`
}

// ForClient returns the subset of Config meant for frontend
// consumption via window.__dixieConfig.
func (c Config) ForClient() ClientConfig {
	return ClientConfig{
		ToastDurationMs:        c.UI.ToastDurationMs,
		LandingPage:            c.UI.LandingPage,
		CalendarTimezone:       c.Calendar.Timezone,
		UpdateSourceURL:        c.Services.UpdateSourceURL,
		RepositoryURL:          c.Services.RepositoryURL,
		FeedbackEndpoint:       c.Services.FeedbackEndpoint,
		AllowedImageMIMETypes:  c.Files.AllowedImageMIMETypes,
		RecentRecordsCap:       c.Limits.RecentRecordsCap,
		ResearchRecentsCap:     c.Limits.ResearchRecentsCap,
		BackStackDepth:         c.Limits.BackStackDepth,
		NotesPreviewChars:      c.Limits.NotesPreviewChars,
		ArticleExcerptChars:    c.Limits.ArticleExcerptChars,
		DebugLogRingSize:       c.Limits.DebugLogRingSize,
		ExportBatchSize:        c.Limits.ExportBatchSize,
		JobsPollMs:             c.Timing.JobsPollMs,
		ReviewBadgePollMs:      c.Timing.ReviewBadgePollMs,
		JobStatusPollMs:        c.Timing.JobStatusPollMs,
		UndoRedoPollMs:         c.Timing.UndoRedoPollMs,
		BrowseFilterDebounceMs: c.Timing.BrowseFilterDebounceMs,
		PrintPreviewDebounceMs: c.Timing.PrintPreviewDebounceMs,
		ClientLogFlushMs:       c.Timing.ClientLogFlushMs,
		ClientLogFlushThreshold: c.Timing.ClientLogFlushThreshold,
		ClientLogMaxBuffer:     c.Timing.ClientLogMaxBuffer,
		FeedbackSendTimeoutS:   c.Timing.FeedbackSendTimeoutS,
		FeedbackUploadTimeoutS: c.Timing.FeedbackUploadTimeoutS,
		PDFPaper:               c.PDF.Paper,
		GoogleCalendarName:     c.Google.CalendarName,
		GoogleTestCalendarName: c.Google.TestCalendarName,
		ThemeBrowserPalette:    c.Theme.PaletteBrowser,
		ThemeBrowserFonts:      c.Theme.FontsBrowser,
		ThemeBrowserTypeScale:  c.Theme.TypeScaleBrowser,
		ThemeActivityPalette:   c.Theme.PaletteActivity,
		ThemeCalendarPalette:   c.Theme.PaletteCalendar,
		ThemeHeadingColor:      c.Theme.HeadingColor,
		ThemeBlockquoteBorder:  c.Theme.BlockquoteBorder,
		ThemeCodeBackground:    c.Theme.CodeBackground,
	}
}
