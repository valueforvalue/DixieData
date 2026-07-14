package jobs

import "strings"

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

	// Summarizer produces the JobSummary for this kind. nil falls
	// through to defaultSummarizer (which renders the kind's
	// label + sub-second duration — the safe shape for unknown
	// kinds). The future slice-2 migration wires this into
	// Summary().
	Summarizer func(Job) JobSummary
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
	},
	"soldier_pdf_no_images": {
		DisplayLabel:  "Soldier PDF (no images)",
		ActivityGroup: "exports",
		DismissTarget: "/soldiers",
		PastTense:     "Export",
	},
	"soldier_jpg": {
		DisplayLabel:  "Soldier JPG bundle",
		ActivityGroup: "exports",
		DismissTarget: "/soldiers",
		PastTense:     "Export",
	},
	"monthly_pdf": {
		DisplayLabel:  "Monthly calendar PDF",
		ActivityGroup: "exports",
		DismissTarget: "/calendar",
		PastTense:     "Export",
	},
	"database_pdf": {
		DisplayLabel:  "Printable archive PDF",
		ActivityGroup: "exports",
		DismissTarget: "/soldiers",
		PastTense:     "Export",
	},
	"static_archive": {
		DisplayLabel:  "Static web archive",
		ActivityGroup: "exports",
		DismissTarget: "/soldiers",
		PastTense:     "Export",
	},
	"backup_archive": {
		DisplayLabel:  "Local Archive backup",
		ActivityGroup: "exports",
		DismissTarget: "/settings",
		PastTense:     "Export",
	},
	"shared_archive": {
		DisplayLabel:  "Shared Archive",
		ActivityGroup: "exports",
		DismissTarget: "/share",
		PastTense:     "Export",
	},
	"shared_archive_subset": {
		DisplayLabel:  "Shared Archive subset",
		ActivityGroup: "exports",
		DismissTarget: "/share",
		PastTense:     "Export",
	},
	"json_export": {
		DisplayLabel:  "JSON export",
		ActivityGroup: "exports",
		DismissTarget: "/export",
		PastTense:     "Export",
	},
	"excel_export": {
		DisplayLabel:  "Excel export",
		ActivityGroup: "exports",
		DismissTarget: "/export",
		PastTense:     "Export",
	},
	"icalendar_export": {
		DisplayLabel:  "iCalendar export",
		ActivityGroup: "exports",
		DismissTarget: "/calendar",
		PastTense:     "Export",
	},
	"insights_pdf": {
		DisplayLabel:  "Insights PDF",
		ActivityGroup: "exports",
		DismissTarget: "/insights",
		PastTense:     "Export",
	},
	"bug_report": {
		DisplayLabel:  "Bug-report bundle",
		ActivityGroup: "reports",
		DismissTarget: "/jobs",
		PastTense:     "Export",
	},
	"article_pdf": {
		DisplayLabel:  "Article PDF",
		ActivityGroup: "exports",
		DismissTarget: "/articles",
		PastTense:     "Export",
	},
	"feedback_log": {
		DisplayLabel:  "Feedback log",
		ActivityGroup: "reports",
		DismissTarget: "/settings",
		PastTense:     "Export",
	},

	// --- Imports ---
	"image_import": {
		DisplayLabel:  "Image import",
		ActivityGroup: "imports",
		DismissTarget: "/browse",
		PastTense:     "Import",
	},
	"backup_import": {
		DisplayLabel:  "Backup restore",
		ActivityGroup: "imports",
		DismissTarget: "/settings",
		PastTense:     "Import",
	},
	"shared_import": {
		DisplayLabel:  "Shared Archive import",
		ActivityGroup: "imports",
		DismissTarget: "/share",
		PastTense:     "Import",
	},
	"memorial_import": {
		DisplayLabel:  "Memorial JSON import",
		ActivityGroup: "imports",
		DismissTarget: "/share",
		PastTense:     "Import",
	},

	// --- Audits ---
	"image_orphan_cleanup": {
		DisplayLabel:  "Image orphan cleanup",
		ActivityGroup: "audits",
		DismissTarget: "/settings#images",
		PastTense:     "Cleanup",
	},
	"duplicate_audit": {
		DisplayLabel:  "Duplicate audit",
		ActivityGroup: "audits",
		DismissTarget: "/insights",
		PastTense:     "Audit",
	},

	// --- Reviews ---
	"review_bulk_resolve": {
		DisplayLabel:  "Bulk review resolution",
		ActivityGroup: "reviews",
		DismissTarget: "/reviews",
		PastTense:     "Resolve",
	},
	"review_bulk_delete": {
		DisplayLabel:  "Bulk review delete",
		ActivityGroup: "reviews",
		DismissTarget: "/reviews",
		PastTense:     "Delete",
	},

	// --- Integrations ---
	"google_drive_backup": {
		DisplayLabel:  "Google Drive backup",
		ActivityGroup: "integrations",
		DismissTarget: "/integrations",
		PastTense:     "Backup",
	},
	"google_sheets_export": {
		DisplayLabel:  "Google Sheets export",
		ActivityGroup: "integrations",
		DismissTarget: "/integrations",
		PastTense:     "Export",
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