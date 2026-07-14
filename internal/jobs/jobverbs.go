package jobs

// FailedVerb returns the user-facing past-tense verb that pairs
// with job.Kind for an errored (cancelled=false) or cancelled
// (cancelled=true) job. See also Job.DisplayLabel for the kind →
// display noun mapping.
//
// Issue #556 slice 4: reads from KindRegistry[kind].PastTense. The
// 17-kind switch (4 imports + 13 exports) is replaced by the
// registry; the 8 kinds that weren't in the old switch (article_pdf,
// bug_report, feedback_log, google_drive_backup, google_sheets_export,
// shared_archive_subset, image_orphan_cleanup, review_bulk_resolve,
// review_bulk_delete, duplicate_audit) no longer silently render
// "Operation failed." for the user — they now render the kind's
// accurate verb ("Export failed.", "Cleanup failed.", "Resolve
// failed.", etc.). Unknown kinds still fall through to "Operation".
//
//	FailedVerb("backup_import", false) → "Import failed."
//	FailedVerb("static_archive", false) → "Export failed."
//	FailedVerb("review_bulk_resolve", false) → "Resolve failed."
//	FailedVerb("future_kind",    false) → "Operation failed."
//	FailedVerb("backup_import", true)  → "Import cancelled."
func FailedVerb(kind string, cancelled bool) string {
	verb := "Operation"
	if meta, ok := KindRegistry[kind]; ok && meta.PastTense != "" {
		verb = meta.PastTense
	}
	if cancelled {
		return verb + " cancelled."
	}
	return verb + " failed."
}