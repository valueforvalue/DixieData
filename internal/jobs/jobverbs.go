package jobs

// FailedVerb returns the user-facing past-tense verb that pairs
// with job.Kind for an errored (cancelled=false) or cancelled
// (cancelled=true) job. See also Job.DisplayLabel for the kind →
// display noun mapping. New kinds must be added to BOTH helpers,
// otherwise they fall through to "Operation failed." which is
// the correct fallback for unrecognised kinds.
//
//	FailedVerb("backup_import", false) → "Import failed."
//	FailedVerb("static_archive", false) → "Export failed."
//	FailedVerb("future_kind",    false) → "Operation failed."
//	FailedVerb("backup_import", true)  → "Import cancelled."
func FailedVerb(kind string, cancelled bool) string {
	verb := "Operation"
	switch kind {
	// Imports (4 kinds).
	case "backup_import", "shared_import", "memorial_import", "image_import":
		verb = "Import"
	// Exports (13 kinds).
	case "static_archive", "database_pdf", "soldier_pdf", "soldier_pdf_no_images",
		"soldier_jpg", "monthly_pdf", "backup_archive", "shared_archive",
		"json_export", "excel_export", "icalendar_export", "insights_pdf",
		"bug_report":
		verb = "Export"
	}
	if cancelled {
		return verb + " cancelled."
	}
	return verb + " failed."
}