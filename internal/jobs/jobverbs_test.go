package jobs

import "testing"

// TestFailedVerb pins the contract for FailedVerb, the helper that
// picks the right user-facing past-tense verb for an errored or
// cancelled job. The helper is the single source of truth for the
// error/cancel label across the /jobs/{id} status page (the only
// surface that renders the verb). New kinds must be added to the
// switch in jobverbs.go AND the helper tests below; otherwise the
// kind falls through to "Operation failed." which is the correct
// fallback for unrecognised kinds.
//
// Each case pins both states (cancelled=false → "failed.",
// cancelled=true → "cancelled."). 17 known kinds + 1 unknown kind
// × 2 states = 36 cases.
func TestFailedVerb(t *testing.T) {
	tests := []struct {
		kind      string
		cancelled bool
		want      string
	}{
		// Imports (4 kinds).
		{"backup_import", false, "Import failed."},
		{"backup_import", true, "Import cancelled."},
		{"shared_import", false, "Import failed."},
		{"shared_import", true, "Import cancelled."},
		{"memorial_import", false, "Import failed."},
		{"memorial_import", true, "Import cancelled."},
		{"image_import", false, "Import failed."},
		{"image_import", true, "Import cancelled."},

		// Exports (13 kinds).
		{"static_archive", false, "Export failed."},
		{"static_archive", true, "Export cancelled."},
		{"database_pdf", false, "Export failed."},
		{"database_pdf", true, "Export cancelled."},
		{"soldier_pdf", false, "Export failed."},
		{"soldier_pdf", true, "Export cancelled."},
		{"soldier_pdf_no_images", false, "Export failed."},
		{"soldier_pdf_no_images", true, "Export cancelled."},
		{"soldier_jpg", false, "Export failed."},
		{"soldier_jpg", true, "Export cancelled."},
		{"monthly_pdf", false, "Export failed."},
		{"monthly_pdf", true, "Export cancelled."},
		{"backup_archive", false, "Export failed."},
		{"backup_archive", true, "Export cancelled."},
		{"shared_archive", false, "Export failed."},
		{"shared_archive", true, "Export cancelled."},
		{"shared_archive_subset", false, "Export failed."},
		{"shared_archive_subset", true, "Export cancelled."},
		{"json_export", false, "Export failed."},
		{"json_export", true, "Export cancelled."},
		{"excel_export", false, "Export failed."},
		{"excel_export", true, "Export cancelled."},
		{"icalendar_export", false, "Export failed."},
		{"icalendar_export", true, "Export cancelled."},
		{"insights_pdf", false, "Export failed."},
		{"insights_pdf", true, "Export cancelled."},
		{"bug_report", false, "Export failed."},
		{"bug_report", true, "Export cancelled."},
		{"article_pdf", false, "Export failed."},
		{"article_pdf", true, "Export cancelled."},
		{"feedback_log", false, "Export failed."},
		{"feedback_log", true, "Export cancelled."},
		{"google_sheets_export", false, "Export failed."},
		{"google_sheets_export", true, "Export cancelled."},

		// Audits (issue #556 slice 4 — previously silent).
		{"image_orphan_cleanup", false, "Cleanup failed."},
		{"image_orphan_cleanup", true, "Cleanup cancelled."},
		{"duplicate_audit", false, "Audit failed."},
		{"duplicate_audit", true, "Audit cancelled."},

		// Reviews (issue #556 slice 4 — previously silent).
		{"review_bulk_resolve", false, "Resolve failed."},
		{"review_bulk_resolve", true, "Resolve cancelled."},
		{"review_bulk_delete", false, "Delete failed."},
		{"review_bulk_delete", true, "Delete cancelled."},

		// Integrations (issue #556 slice 4 — previously silent).
		{"google_drive_backup", false, "Backup failed."},
		{"google_drive_backup", true, "Backup cancelled."},

		// Unknown kind — fallback to "Operation".
		{"future_kind", false, "Operation failed."},
		{"future_kind", true, "Operation cancelled."},
		{"", false, "Operation failed."},
		{"", true, "Operation cancelled."},
	}

	for _, tt := range tests {
		name := tt.kind
		if name == "" {
			name = "empty"
		}
		state := "failed"
		if tt.cancelled {
			state = "cancelled"
		}
		t.Run(name+"/"+state, func(t *testing.T) {
			got := FailedVerb(tt.kind, tt.cancelled)
			if got != tt.want {
				t.Fatalf("FailedVerb(%q, %v) = %q, want %q", tt.kind, tt.cancelled, got, tt.want)
			}
		})
	}
}