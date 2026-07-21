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
	Window  WindowConfig  `json:"window"`
	UI      UIConfig      `json:"ui"`
	Limits  LimitsConfig  `json:"limits"`
	Timing  TimingConfig  `json:"timing"`
	PDF     PDFConfig     `json:"pdf"`
	Google  GoogleConfig  `json:"google"`
	Theme   ThemeConfig   `json:"theme"`
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

// LimitsConfig controls caps and retention policies.
type LimitsConfig struct {
	BrowseDefaultPageSize    int `json:"browse_default_page_size"`
	BrowseMaxPageSize        int `json:"browse_max_page_size"`
	ListDefaultPageSize      int `json:"list_default_page_size"`
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
}

// TimingConfig controls polling intervals and debounce delays.
type TimingConfig struct {
	JobsPollMs           int `json:"jobs_poll_ms"`
	ReviewBadgePollMs    int `json:"review_badge_poll_ms"`
	JobStatusPollMs      int `json:"job_status_poll_ms"`
	UndoRedoPollMs       int `json:"undo_redo_poll_ms"`
	BrowseFilterDebounceMs int `json:"browse_filter_debounce_ms"`
	PrintPreviewDebounceMs int `json:"print_preview_debounce_ms"`
	UpdateCheckTimeoutS    int `json:"update_check_timeout_s"`
	ShutdownTimeoutS       int `json:"shutdown_timeout_s"`
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
type ThemeConfig struct {
	Palette   map[string]string     `json:"palette"`
	TypeScale map[string]TypeSize   `json:"type_scale"`
	Fonts     FontConfig            `json:"fonts"`
	Branding  BrandingConfig        `json:"branding"`
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
		Limits: LimitsConfig{
			BrowseDefaultPageSize:    100,
			BrowseMaxPageSize:        250,
			ListDefaultPageSize:      50,
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
		},
		Timing: TimingConfig{
			JobsPollMs:            3000,
			ReviewBadgePollMs:     30000,
			JobStatusPollMs:       2000,
			UndoRedoPollMs:        500,
			BrowseFilterDebounceMs: 200,
			PrintPreviewDebounceMs: 150,
			UpdateCheckTimeoutS:    45,
			ShutdownTimeoutS:       5,
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
	if src.Limits.BrowseDefaultPageSize != 0 {
		dst.Limits.BrowseDefaultPageSize = src.Limits.BrowseDefaultPageSize
	}
	if src.Limits.BrowseMaxPageSize != 0 {
		dst.Limits.BrowseMaxPageSize = src.Limits.BrowseMaxPageSize
	}
	if src.Limits.ListDefaultPageSize != 0 {
		dst.Limits.ListDefaultPageSize = src.Limits.ListDefaultPageSize
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
}

// ClientConfig is the subset of Config exposed to the frontend
// via window.__dixieConfig. It excludes server-only values
// (window size, update timeout, shutdown timeout) that the
// frontend has no use for.
type ClientConfig struct {
	ToastDurationMs         int    `json:"toastDurationMs"`
	LandingPage             string `json:"landingPage"`
	RecentRecordsCap        int    `json:"recentRecordsCap"`
	ResearchRecentsCap      int    `json:"researchRecentsCap"`
	BackStackDepth          int    `json:"backStackDepth"`
	NotesPreviewChars       int    `json:"notesPreviewChars"`
	JobsPollMs              int    `json:"jobsPollMs"`
	ReviewBadgePollMs       int    `json:"reviewBadgePollMs"`
	JobStatusPollMs         int    `json:"jobStatusPollMs"`
	UndoRedoPollMs          int    `json:"undoRedoPollMs"`
	BrowseFilterDebounceMs  int    `json:"browseFilterDebounceMs"`
	PrintPreviewDebounceMs  int    `json:"printPreviewDebounceMs"`
	PDFPaper                string `json:"pdfPaper"`
	GoogleCalendarName      string `json:"googleCalendarName"`
	GoogleTestCalendarName  string `json:"googleTestCalendarName"`
}

// ForClient returns the subset of Config meant for frontend
// consumption via window.__dixieConfig.
func (c Config) ForClient() ClientConfig {
	return ClientConfig{
		ToastDurationMs:        c.UI.ToastDurationMs,
		LandingPage:            c.UI.LandingPage,
		RecentRecordsCap:       c.Limits.RecentRecordsCap,
		ResearchRecentsCap:     c.Limits.ResearchRecentsCap,
		BackStackDepth:         c.Limits.BackStackDepth,
		NotesPreviewChars:      c.Limits.NotesPreviewChars,
		JobsPollMs:             c.Timing.JobsPollMs,
		ReviewBadgePollMs:      c.Timing.ReviewBadgePollMs,
		JobStatusPollMs:        c.Timing.JobStatusPollMs,
		UndoRedoPollMs:         c.Timing.UndoRedoPollMs,
		BrowseFilterDebounceMs: c.Timing.BrowseFilterDebounceMs,
		PrintPreviewDebounceMs: c.Timing.PrintPreviewDebounceMs,
		PDFPaper:               c.PDF.Paper,
		GoogleCalendarName:     c.Google.CalendarName,
		GoogleTestCalendarName: c.Google.TestCalendarName,
	}
}
