package jobs

import (
	"fmt"
	"os"
	"strings"
)

// KindMeta centralises the per-kind UI metadata that used to be
// scattered across DisplayLabel (jobs.go), Summary (jobs.go),
// DismissTargetPath (jobs.go), FailedVerb (jobverbs.go), and the
// Recent Activity panel heading (templates/components/recent_jobs.templ).
//
// The registry is the single source of truth. Adding a new kind is
// a one-entry change here; the consumers read from this map and
// fall back to a safe default for unknown kinds (forward-compat for
// pre-refactor JSONL log entries that reference kinds the registry
// doesn't know about yet).
//
// Slice 1 (issue #556) populates only DisplayLabel and the
// forward-looking JobResult.TrashRoot field. Slices 2-5 migrate the
// other consumers onto the same registry.
type KindMeta struct {
	// DisplayLabel is the friendly title rendered in page headings,
	// activity rows, and summary cards. Title-case; no underscores.
	// Empty falls through to humanize(kind).
	DisplayLabel string

	// ActivityGroup names the bucket the Recent Activity panel uses
	// to sub-section this kind. One of: "exports", "imports",
	// "reviews", "audits", "settings", "integrations", "reports".
	// Empty falls through to "exports" (the historical default).
	ActivityGroup string

	// DismissTarget is the route the dismiss button navigates to
	// when the user closes the /jobs/{id} status page. Must be a
	// valid routebuilder constant or a hash-fragment extension of
	// one. Empty falls through to /jobs (the safe "back to the list"
	// fallback, NOT /share — the pre-refactor fallback routed
	// review_bulk_resolve etc. to /share, which is wrong).
	DismissTarget string

	// PastTense is the verb stem used by FailedVerb() for this
	// kind ("Export" / "Import" / "Resolve" / "Cleanup" / ...).
	// Empty falls through to "Operation" (the legacy default).
	// The future slice-4 migration wires this into FailedVerb.
	PastTense string

	// Base is an optional pointer to a parent KindMeta whose
	// zero-value fields this entry inherits. Used for sibling
	// kinds like soldier_pdf / soldier_pdf_no_images that share
	// most of their metadata. nil means "no inheritance".
	Base *KindMeta

	// Summarizer produces the JobSummary for this kind. The label
// parameter is the resolved DisplayLabel (registry hit or
// humanizeKind fallback) so Summarizer funcs don't have to call
// j.DisplayLabel() themselves — that's the init-cycle break:
// DisplayLabel reads KindRegistry, and Summarizers are stored in
// KindRegistry, so having Summarizers call DisplayLabel transitively
// references KindRegistry from its own initializer (compile-time
// cycle). Passing the label as a parameter breaks the cycle while
// keeping the per-kind Summarizers simple.
//
// nil falls through to defaultSummarizer (which renders the
// kind's label + sub-second duration — the safe shape for unknown
// kinds). Slice 2 wires this into Summary().
	Summarizer func(j Job, label string) JobSummary
}

// defaultSummarizer is the safe fallback for kinds that either
// aren't in the registry OR are in the registry but don't define
// a per-kind Summarizer. Renders the kind's friendly label +
// duration + size (when an artifact exists). Critically, for
// kinds with no ResultPath (the zero-state shape from issue #543
// — image_orphan_cleanup, review_bulk_resolve, etc.), the headline
// falls back to j.Message so the user sees what the job actually
// did, and the Size: line is suppressed (issue #543's exact fix).
//
// Unknown kinds (legacy JSONL log entries predating the registry)
// get humanizeKind() labels here too — no raw snake_case leaks to
// the summary card.
func defaultSummarizer(j Job, label string) JobSummary {
	s := JobSummary{
		Kind:       j.Kind,
		Label:      label,
		ResultPath: j.ResultPath,
	}
	if j.Status != StatusDone || j.StartedAt.IsZero() || j.FinishedAt.IsZero() {
		return s
	}
	s.Duration = j.FinishedAt.Sub(j.StartedAt)
	if j.ResultPath != "" {
		if info, err := os.Stat(j.ResultPath); err == nil {
			s.SizeBytes = info.Size()
		}
	}
	if s.SizeBytes > 0 {
		// Artifact-producing kind — surface the size like the
		// pre-#556 default arm did. Defensive: a future kind that
		// writes a ResultPath but forgets to register a Summarizer
		// lands here and still produces a sensible card.
		s.Headline = fmt.Sprintf("%s complete — %s.", label, formatBytes(s.SizeBytes))
		s.DetailLines = []string{
			fmt.Sprintf("Size: %s", formatBytes(s.SizeBytes)),
			fmt.Sprintf("Duration: %s", formatDuration(s.Duration)),
		}
	} else if j.Message != "" {
		// Zero-state kind shape (issue #543): anchor on the worker's
		// progress message instead of producing a misleading
		// "Size: 0 B" headline.
		s.Headline = j.Message
		s.DetailLines = []string{fmt.Sprintf("Duration: %s", formatDuration(s.Duration))}
	} else {
		// Last-resort fallback: a kind with no ResultPath AND no
		// Message (a future kind whose worker forgot both) still
		// renders a non-empty card.
		s.Headline = fmt.Sprintf("%s complete.", label)
		s.DetailLines = []string{fmt.Sprintf("Duration: %s", formatDuration(s.Duration))}
	}
	return s
}

// Per-kind Summarizer funcs. Each preserves the exact copy from
// the pre-#556 switch so existing test assertions continue to
// pass. Factored out of jobs.go's Summary() so the registry can
// hold them by reference. All take `(j Job, label string)` — the
// label is the resolved DisplayLabel passed by Summary() to break
// the init cycle (see Summarizer field docstring).
//
// The summarize* funcs use augmentSummarizer: it calls
// defaultSummarizer FIRST so the running-job early-return (no
// headline, no detail lines until the job is StatusDone with
// timestamps) propagates correctly. When the job is still
// running, the augment closure is never invoked — the empty
// summary is returned as-is. When the job is done, the closure
// augments the default summary with kind-specific copy.

// augmentSummarizer runs defaultSummarizer and only invokes the
// `augment` closure if the job is in a terminal-done state (not
// still running). The closure may freely overwrite Headline +
// DetailLines knowing the job has StatusDone + timestamps.
func augmentSummarizer(j Job, label string, augment func(s *JobSummary)) JobSummary {
	s := defaultSummarizer(j, label)
	if j.Status != StatusDone || j.StartedAt.IsZero() || j.FinishedAt.IsZero() {
		return s
	}
	augment(&s)
	return s
}

// summarizeSoldierPDF covers soldier_pdf + soldier_pdf_no_images.
// Identical shape; the kind distinction is purely label-level
// (DisplayLabel handles it).
func summarizeSoldierPDF(j Job, label string) JobSummary {
	return augmentSummarizer(j, label, func(s *JobSummary) {
		s.Headline = fmt.Sprintf("%s complete — %s.", label, formatBytes(s.SizeBytes))
		s.DetailLines = []string{
			fmt.Sprintf("Size: %s", formatBytes(s.SizeBytes)),
			fmt.Sprintf("Duration: %s", formatDuration(s.Duration)),
		}
	})
}

// summarizeSoldierJPG preserves the hard-coded "Soldier JPG
// export" label (pre-dates the friendly DisplayLabel work).
func summarizeSoldierJPG(j Job, label string) JobSummary {
	return augmentSummarizer(j, label, func(s *JobSummary) {
		s.Headline = fmt.Sprintf("Soldier JPG export complete — %s.", formatBytes(s.SizeBytes))
		s.DetailLines = []string{
			fmt.Sprintf("Size: %s", formatBytes(s.SizeBytes)),
			fmt.Sprintf("Duration: %s", formatDuration(s.Duration)),
		}
	})
}

// summarizeMonthlyPDF preserves the hard-coded "Monthly calendar
// PDF" headline.
func summarizeMonthlyPDF(j Job, label string) JobSummary {
	return augmentSummarizer(j, label, func(s *JobSummary) {
		s.Headline = fmt.Sprintf("Monthly calendar PDF complete — %s.", formatBytes(s.SizeBytes))
		s.DetailLines = []string{
			fmt.Sprintf("Size: %s", formatBytes(s.SizeBytes)),
			fmt.Sprintf("Duration: %s", formatDuration(s.Duration)),
		}
	})
}

// summarizeBackupArchive adds the "Use 'Load Backup'..." hint line.
func summarizeBackupArchive(j Job, label string) JobSummary {
	return augmentSummarizer(j, label, func(s *JobSummary) {
		s.Headline = fmt.Sprintf("Backup archive complete — %s.", formatBytes(s.SizeBytes))
		s.DetailLines = []string{
			fmt.Sprintf("Size: %s", formatBytes(s.SizeBytes)),
			fmt.Sprintf("Duration: %s", formatDuration(s.Duration)),
			"Use 'Load Backup' on the Share page to restore this archive.",
		}
		s.DetailLines = appendExportStats(s.DetailLines, j.Result)
	})
}

// summarizeSharedArchive + summarizeSharedArchiveSubset cover the
// .ddshare exports.
func summarizeSharedArchive(j Job, label string) JobSummary {
	return augmentSummarizer(j, label, func(s *JobSummary) {
		s.Headline = fmt.Sprintf("Shared archive complete — %s.", formatBytes(s.SizeBytes))
		s.DetailLines = []string{
			fmt.Sprintf("Size: %s", formatBytes(s.SizeBytes)),
			fmt.Sprintf("Duration: %s", formatDuration(s.Duration)),
			"Send this .ddshare file to another DixieData user; they can preview it on the Share page.",
		}
		s.DetailLines = appendExportStats(s.DetailLines, j.Result)
	})
}

func summarizeSharedArchiveSubset(j Job, label string) JobSummary {
	return augmentSummarizer(j, label, func(s *JobSummary) {
		s.Headline = fmt.Sprintf("Subset shared archive complete — %s.", formatBytes(s.SizeBytes))
		s.DetailLines = []string{
			fmt.Sprintf("Size: %s", formatBytes(s.SizeBytes)),
			fmt.Sprintf("Duration: %s", formatDuration(s.Duration)),
			"Subset of Person Records staged from the Share Queue; send to another DixieData user.",
		}
		s.DetailLines = appendExportStats(s.DetailLines, j.Result)
	})
}

// summarizeGenericExport covers json_export, excel_export, and
// icalendar_export. All three have identical shape; the label
// difference is rendered through the label parameter.
func summarizeGenericExport(j Job, label string) JobSummary {
	return augmentSummarizer(j, label, func(s *JobSummary) {
		s.Headline = fmt.Sprintf("%s complete — %s.", label, formatBytes(s.SizeBytes))
		s.DetailLines = []string{
			fmt.Sprintf("Size: %s", formatBytes(s.SizeBytes)),
			fmt.Sprintf("Duration: %s", formatDuration(s.Duration)),
		}
		s.DetailLines = appendExportStats(s.DetailLines, j.Result)
	})
}

// summarizeDatabasePDF preserves the "Printable archive PDF" copy
// + the contents description.
func summarizeDatabasePDF(j Job, label string) JobSummary {
	return augmentSummarizer(j, label, func(s *JobSummary) {
		s.Headline = fmt.Sprintf("Printable archive PDF complete — %s.", formatBytes(s.SizeBytes))
		s.DetailLines = []string{
			fmt.Sprintf("Size: %s", formatBytes(s.SizeBytes)),
			fmt.Sprintf("Duration: %s", formatDuration(s.Duration)),
			"The PDF contains every record grouped and sorted per your export settings.",
		}
		s.DetailLines = appendExportStats(s.DetailLines, j.Result)
	})
}

// summarizeStaticArchive covers the static-archive export with
// per-kind content counts (issue #492).
func summarizeStaticArchive(j Job, label string) JobSummary {
	return augmentSummarizer(j, label, func(s *JobSummary) {
		s.Headline = fmt.Sprintf("Static archive complete — %s.", formatBytes(s.SizeBytes))
		s.DetailLines = []string{
			fmt.Sprintf("Size: %s", formatBytes(s.SizeBytes)),
			fmt.Sprintf("Duration: %s", formatDuration(s.Duration)),
			"Open the .zip and host it on any static-file web server to browse the archive without DixieData.",
		}
		if j.Result.StaticArchive != nil {
			s.DetailLines = appendStaticArchiveStats(s.DetailLines, *j.Result.StaticArchive)
		} else {
			s.DetailLines = append(s.DetailLines, "Contents unavailable for this archive — exported before counts were tracked.")
		}
	})
}

// summarizeInsightsOrBugReport covers insights_pdf + bug_report.
func summarizeInsightsOrBugReport(j Job, label string) JobSummary {
	return augmentSummarizer(j, label, func(s *JobSummary) {
		s.Headline = fmt.Sprintf("%s complete — %s.", label, formatBytes(s.SizeBytes))
		s.DetailLines = []string{
			fmt.Sprintf("Size: %s", formatBytes(s.SizeBytes)),
			fmt.Sprintf("Duration: %s", formatDuration(s.Duration)),
		}
	})
}

// summarizeImageImport, summarizeBackupImport, summarizeSharedImport,
// summarizeMemorialImport cover the four import kinds. Each shows
// j.Message as the first detail line when set (the worker uses
// p.Set() to communicate progress) + the kind-specific stats
// helper.
func summarizeImageImport(j Job, label string) JobSummary {
	return augmentSummarizer(j, label, func(s *JobSummary) {
		s.Headline = fmt.Sprintf("%s complete.", label)
		if j.Message != "" {
			s.DetailLines = []string{j.Message, fmt.Sprintf("Duration: %s", formatDuration(s.Duration))}
		} else {
			s.DetailLines = []string{fmt.Sprintf("Duration: %s", formatDuration(s.Duration))}
		}
	})
}

func summarizeBackupImport(j Job, label string) JobSummary {
	return augmentSummarizer(j, label, func(s *JobSummary) {
		s.Headline = fmt.Sprintf("%s complete.", label)
		if j.Message != "" {
			s.DetailLines = []string{j.Message, fmt.Sprintf("Duration: %s", formatDuration(s.Duration))}
		} else {
			s.DetailLines = []string{fmt.Sprintf("Duration: %s", formatDuration(s.Duration))}
		}
		s.DetailLines = appendBackupRestoreStats(s.DetailLines, j.Result)
	})
}

func summarizeSharedImport(j Job, label string) JobSummary {
	return augmentSummarizer(j, label, func(s *JobSummary) {
		s.Headline = fmt.Sprintf("%s complete.", label)
		if j.Message != "" {
			s.DetailLines = []string{j.Message, fmt.Sprintf("Duration: %s", formatDuration(s.Duration))}
		} else {
			s.DetailLines = []string{fmt.Sprintf("Duration: %s", formatDuration(s.Duration))}
		}
		s.DetailLines = appendSharedImportStats(s.DetailLines, j.Result)
	})
}

func summarizeMemorialImport(j Job, label string) JobSummary {
	return augmentSummarizer(j, label, func(s *JobSummary) {
		s.Headline = fmt.Sprintf("%s complete.", label)
		s.DetailLines = []string{fmt.Sprintf("Duration: %s", formatDuration(s.Duration))}
		s.DetailLines = appendMemorialImportStats(s.DetailLines, j.Result)
	})
}

// summarizeZeroState covers the six zero-state kinds (issue #543):
// image_orphan_cleanup, duplicate_audit, review_bulk_resolve,
// review_bulk_delete, google_drive_backup, google_sheets_export.
// All six populate j.Message via p.Set(100, "..."); the summary
// card surfaces that as the headline + sub-second Duration.
// google_drive_backup + google_sheets_export also surface the
// RemoteURL link (issue #552).
func summarizeZeroState(j Job, label string) JobSummary {
	return augmentSummarizer(j, label, func(s *JobSummary) {
		if j.Message != "" {
			s.Headline = j.Message
		} else {
			s.Headline = fmt.Sprintf("%s complete.", label)
		}
		s.DetailLines = []string{fmt.Sprintf("Duration: %s", formatDuration(s.Duration))}
		if j.Result.RemoteURL != "" {
			linkLabel := "Open in Drive"
			switch j.Result.RemoteKind {
			case "sheets":
				linkLabel = "Open in Sheets"
			}
			s.RemoteURL = j.Result.RemoteURL
			s.RemoteLabel = linkLabel
		}
	})
}

// KindRegistry is the authoritative kind → metadata table.
// Slices 2-5 migrate the consumers that currently switch on j.Kind
// to read from this map.
var KindRegistry = map[string]KindMeta{
	// --- Exports (PDF + JSON + iCalendar + bulk archive) ---
	"soldier_pdf": {
		DisplayLabel:  "Soldier PDF",
		ActivityGroup: "exports",
		DismissTarget: "/soldiers",
		PastTense:     "Export",
		Summarizer:    summarizeSoldierPDF,
	},
	"soldier_pdf_no_images": {
		DisplayLabel:  "Soldier PDF (no images)",
		ActivityGroup: "exports",
		DismissTarget: "/soldiers",
		PastTense:     "Export",
		Summarizer:    summarizeSoldierPDF,
	},
	"soldier_jpg": {
		DisplayLabel:  "Soldier JPG bundle",
		ActivityGroup: "exports",
		DismissTarget: "/soldiers",
		PastTense:     "Export",
		Summarizer:    summarizeSoldierJPG,
	},
	"monthly_pdf": {
		DisplayLabel:  "Monthly calendar PDF",
		ActivityGroup: "exports",
		DismissTarget: "/calendar",
		PastTense:     "Export",
		Summarizer:    summarizeMonthlyPDF,
	},
	"database_pdf": {
		DisplayLabel:  "Printable archive PDF",
		ActivityGroup: "exports",
		DismissTarget: "/soldiers",
		PastTense:     "Export",
		Summarizer:    summarizeDatabasePDF,
	},
	"static_archive": {
		DisplayLabel:  "Static web archive",
		ActivityGroup: "exports",
		DismissTarget: "/soldiers",
		PastTense:     "Export",
		Summarizer:    summarizeStaticArchive,
	},
	"backup_archive": {
		DisplayLabel:  "Local Archive backup",
		ActivityGroup: "exports",
		DismissTarget: "/settings",
		PastTense:     "Export",
		Summarizer:    summarizeBackupArchive,
	},
	"shared_archive": {
		DisplayLabel:  "Shared Archive",
		ActivityGroup: "exports",
		DismissTarget: "/share",
		PastTense:     "Export",
		Summarizer:    summarizeSharedArchive,
	},
	"shared_archive_subset": {
		DisplayLabel:  "Shared Archive subset",
		ActivityGroup: "exports",
		DismissTarget: "/share",
		PastTense:     "Export",
		Summarizer:    summarizeSharedArchiveSubset,
	},
	"json_export": {
		DisplayLabel:  "JSON export",
		ActivityGroup: "exports",
		DismissTarget: "/export",
		PastTense:     "Export",
		Summarizer:    summarizeGenericExport,
	},
	"excel_export": {
		DisplayLabel:  "Excel export",
		ActivityGroup: "exports",
		DismissTarget: "/export",
		PastTense:     "Export",
		Summarizer:    summarizeGenericExport,
	},
	"icalendar_export": {
		DisplayLabel:  "iCalendar export",
		ActivityGroup: "exports",
		DismissTarget: "/calendar",
		PastTense:     "Export",
		Summarizer:    summarizeGenericExport,
	},
	"insights_pdf": {
		DisplayLabel:  "Insights PDF",
		ActivityGroup: "exports",
		DismissTarget: "/insights",
		PastTense:     "Export",
		Summarizer:    summarizeInsightsOrBugReport,
	},
	"bug_report": {
		DisplayLabel:  "Bug-report bundle",
		ActivityGroup: "reports",
		DismissTarget: "/jobs",
		PastTense:     "Export",
		Summarizer:    summarizeInsightsOrBugReport,
	},
	"article_pdf": {
		DisplayLabel:  "Article PDF",
		ActivityGroup: "exports",
		DismissTarget: "/articles",
		PastTense:     "Export",
		Summarizer:    summarizeGenericExport,
	},
	"feedback_log": {
		DisplayLabel:  "Feedback log",
		ActivityGroup: "reports",
		DismissTarget: "/settings",
		PastTense:     "Export",
		Summarizer:    summarizeGenericExport,
	},

	// --- Imports ---
	"image_import": {
		DisplayLabel:  "Image import",
		ActivityGroup: "imports",
		DismissTarget: "/browse",
		PastTense:     "Import",
		Summarizer:    summarizeImageImport,
	},
	"backup_import": {
		DisplayLabel:  "Backup restore",
		ActivityGroup: "imports",
		DismissTarget: "/settings",
		PastTense:     "Import",
		Summarizer:    summarizeBackupImport,
	},
	"shared_import": {
		DisplayLabel:  "Shared Archive import",
		ActivityGroup: "imports",
		DismissTarget: "/share",
		PastTense:     "Import",
		Summarizer:    summarizeSharedImport,
	},
	"memorial_import": {
		DisplayLabel:  "Memorial JSON import",
		ActivityGroup: "imports",
		DismissTarget: "/share",
		PastTense:     "Import",
		Summarizer:    summarizeMemorialImport,
	},

	// --- Audits ---
	"image_orphan_cleanup": {
		DisplayLabel:  "Image orphan cleanup",
		ActivityGroup: "audits",
		DismissTarget: "/settings#images",
		PastTense:     "Cleanup",
		Summarizer:    summarizeZeroState,
	},
	"duplicate_audit": {
		DisplayLabel:  "Duplicate audit",
		ActivityGroup: "audits",
		DismissTarget: "/insights",
		PastTense:     "Audit",
		Summarizer:    summarizeZeroState,
	},

	// --- Reviews ---
	"review_bulk_resolve": {
		DisplayLabel:  "Bulk review resolution",
		ActivityGroup: "reviews",
		DismissTarget: "/reviews",
		PastTense:     "Resolve",
		Summarizer:    summarizeZeroState,
	},
	"review_bulk_delete": {
		DisplayLabel:  "Bulk review delete",
		ActivityGroup: "reviews",
		DismissTarget: "/reviews",
		PastTense:     "Delete",
		Summarizer:    summarizeZeroState,
	},

	// --- Integrations ---
	"google_drive_backup": {
		DisplayLabel:  "Google Drive backup",
		ActivityGroup: "integrations",
		DismissTarget: "/integrations",
		PastTense:     "Backup",
		Summarizer:    summarizeZeroState,
	},
	"google_sheets_export": {
		DisplayLabel:  "Google Sheets export",
		ActivityGroup: "integrations",
		DismissTarget: "/integrations",
		PastTense:     "Export",
		Summarizer:    summarizeZeroState,
	},
}

// knownActivityGroups is the closed set the recent-activities view
// (slice 5) renders sub-sections for. Adding a new group requires
// both a new entry here AND a new sub-section in
// templates/components/recent_jobs.templ — by construction.
var knownActivityGroups = map[string]struct{}{
	"exports":      {},
	"imports":      {},
	"reviews":      {},
	"audits":       {},
	"settings":     {},
	"integrations": {},
	"reports":      {},
}

// kindMetaFor returns the effective KindMeta for the given kind
// string, resolving any Base inheritance. Returns the zero-value
// KindMeta (empty DisplayLabel, empty ActivityGroup, etc.) for
// unknown kinds; callers must apply safe fallbacks.
func kindMetaFor(kind string) KindMeta {
	meta, ok := KindRegistry[kind]
	if !ok {
		return KindMeta{}
	}
	if meta.Base != nil {
		// Field-by-field inheritance: a zero-value field in the
		// child falls through to the base. Pointer indirection
		// preserves the "set vs unset" distinction per field.
		base := *meta.Base
		if meta.DisplayLabel == "" {
			meta.DisplayLabel = base.DisplayLabel
		}
		if meta.ActivityGroup == "" {
			meta.ActivityGroup = base.ActivityGroup
		}
		if meta.DismissTarget == "" {
			meta.DismissTarget = base.DismissTarget
		}
		if meta.PastTense == "" {
			meta.PastTense = base.PastTense
		}
		if meta.Summarizer == nil {
			meta.Summarizer = base.Summarizer
		}
	}
	return meta
}

// humanizeKind converts a snake_case kind into a title-case label
// for unknown-kind fallback rendering. Examples:
//   "review_bulk_resolve" → "Review Bulk Resolve"
//   "soldier_pdf"         → "Soldier Pdf"
//   "google_drive_backup" → "Google Drive Backup"
//
// Pure-string transform — no I/O, no allocations beyond the output
// strings.Builder. Used by DisplayLabel, DismissTargetPath (slice 3),
// and the default Summarizer (slice 2) for unknown kinds.
func humanizeKind(kind string) string {
	if kind == "" {
		return ""
	}
	parts := strings.Split(kind, "_")
	var b strings.Builder
	for i, p := range parts {
		if i > 0 {
			b.WriteByte(' ')
		}
		if p == "" {
			continue
		}
		// ASCII title-case for the first letter; non-letter
		// characters pass through unchanged. Sufficient for the
		// current kind vocabulary (all ASCII snake_case).
		if p[0] >= 'a' && p[0] <= 'z' {
			b.WriteByte(p[0] - 32)
			b.WriteString(p[1:])
		} else {
			b.WriteString(p)
		}
	}
	return b.String()
}