// Package models holds domain types for the DixieData Local Archive.
//
// The constants in this file are the canonical names for entry_type and
// evidence_type values. Use these instead of string literals in templates,
// viewmodels, and SQL bindings so a typo or rename does not silently
// introduce an invalid value.
package models

// Entry type values for the soldiers.entry_type column. These are the
// only legal values; v55 added an application-level discipline (see
// internal/db/schema.go migrateEntryTypeDiscipline) and the Tier 2
// rename in issue #97 will keep the names stable across the URL +
// table column changes.
const (
	EntryTypeSoldier      = "soldier"
	EntryTypeWife         = "wife"
	EntryTypeWidow        = "widow"
	EntryTypeLinkedPerson = "linked_person"
)

// AllEntryTypes returns the canonical entry-type values in display order.
// Use this for form selects so the order is consistent across the app.
func AllEntryTypes() []string {
	return []string{
		EntryTypeSoldier,
		EntryTypeWife,
		EntryTypeWidow,
		EntryTypeLinkedPerson,
	}
}

// IsValidEntryType reports whether s is one of the canonical entry types.
func IsValidEntryType(s string) bool {
	switch s {
	case EntryTypeSoldier, EntryTypeWife, EntryTypeWidow, EntryTypeLinkedPerson:
		return true
	}
	return false
}

// Evidence type values for the research_log.evidence_type column. The
// values mirror the glossary terms in CONTEXT.md so the UI label and
// the stored value agree.
const (
	EvidenceTypeLocalArchive   = "local_archive"
	EvidenceTypeSharedArchive  = "shared_archive"
	EvidenceTypeBackupArchive  = "backup_archive"
	EvidenceTypeStaticArchive  = "static_archive"
	EvidenceTypeRestorePoint   = "restore_point"
	EvidenceTypeMemorialJSON   = "memorial_json"
	EvidenceTypeFindAGrave     = "find_a_grave"
	EvidenceTypePensionRecord  = "pension_record"
	EvidenceTypeApplicationRec = "application_record"
	EvidenceTypeOther          = "other"
)

// AllEvidenceTypes returns the canonical evidence-type values in display order.
func AllEvidenceTypes() []string {
	return []string{
		EvidenceTypeLocalArchive,
		EvidenceTypeSharedArchive,
		EvidenceTypeBackupArchive,
		EvidenceTypeStaticArchive,
		EvidenceTypeRestorePoint,
		EvidenceTypeMemorialJSON,
		EvidenceTypeFindAGrave,
		EvidenceTypePensionRecord,
		EvidenceTypeApplicationRec,
		EvidenceTypeOther,
	}
}

// IsValidEvidenceType reports whether s is one of the canonical evidence types.
func IsValidEvidenceType(s string) bool {
	for _, v := range AllEvidenceTypes() {
		if v == s {
			return true
		}
	}
	return false
}
