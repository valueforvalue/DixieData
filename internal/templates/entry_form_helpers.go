// Helper functions for entry_form.templ. Pure Go, no templ syntax;
// lives next to entry_form.templ so the .templ file stays focused on
// markup. Functions here are referenced from entry_form.templ,
// initial_setup.templ, and settings.templ — keeping them in a
// single file makes the dependency obvious.
//
// When splitting entry_form.templ into smaller files (PR #4 follow-up),
// keep these helpers in this file rather than duplicating across the
// new files.
package templates

import (
	"fmt"
	"strings"

	"github.com/valueforvalue/DixieData/internal/confederatehomestatus"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/pensionstate"
	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

func formTitle(s viewmodel.PersonRecord, isEdit bool) string {
	label := entryTypeLabel(normalizedEntryType(s))
	if isEdit {
		return "Edit " + label + " Person Record"
	}
	return "New " + label + " Person Record"
}

func displayIDInputClass(isEdit bool) string {
	if isEdit {
		return "field-input"
	}
	return "field-input bg-slate-100 text-slate-500"
}

func draftKey(s viewmodel.PersonRecord, isEdit bool) string {
	if isEdit {
		return fmt.Sprintf("edit-soldier-%d", s.ID)
	}
	return "new-soldier"
}

func draftMode(isEdit bool) string {
	if isEdit {
		return "edit"
	}
	return "new"
}

func draftRecordVersion(s viewmodel.PersonRecord, isEdit bool) string {
	if !isEdit {
		return ""
	}
	parts := []string{}
	for _, candidate := range []string{strings.TrimSpace(s.UpdatedAt), strings.TrimSpace(s.LastEditedAt), strings.TrimSpace(s.CreatedAt)} {
		if candidate != "" {
			parts = append(parts, candidate)
		}
	}
	parts = append(parts, fmt.Sprintf("%d", s.ID))
	return strings.Join(parts, "|")
}

func draftResetPath(s viewmodel.PersonRecord, isEdit bool) string {
	if isEdit {
		return fmt.Sprintf("/soldiers/%d/edit", s.ID)
	}
	return "/soldiers/new"
}

func recordPersistenceClass(isEdit bool) string {
	if isEdit {
		return "rounded-2xl border border-emerald-700/40 bg-emerald-50/80 px-4 py-3 text-sm text-emerald-900"
	}
	return "rounded-2xl border border-amber-700/40 bg-amber-50/80 px-4 py-3 text-sm text-amber-900"
}

func recordPersistenceHeading(isEdit bool) string {
	if isEdit {
		return "Committed to database."
	}
	return "Local draft only."
}

func recordPersistenceMessage(isEdit bool) string {
	if isEdit {
		return "This person record currently matches the primary database until you make new local edits."
	}
	return "This new person record is cached in localStorage until you create it in the database."
}

func pensionStates() []string {
	return []string{
		pensionstate.NotApplicable,
		"Alabama", "Alaska", "Arizona", "Arkansas", "California", "Colorado", "Connecticut",
		"Delaware", "Florida", "Georgia", "Hawaii", "Idaho", "Illinois", "Indiana", "Iowa",
		"Kansas", "Kentucky", "Louisiana", "Maine", "Maryland", "Massachusetts", "Michigan",
		"Minnesota", "Mississippi", "Missouri", "Montana", "Nebraska", "Nevada",
		"New Hampshire", "New Jersey", "New Mexico", "New York", "North Carolina",
		"North Dakota", "Ohio", "Oklahoma", "Oregon", "Pennsylvania", "Rhode Island",
		"South Carolina", "South Dakota", "Tennessee", "Texas", "Utah", "Vermont",
		"Virginia", "Washington", "West Virginia", "Wisconsin", "Wyoming",
	}
}

func confederateHomeStatuses() []string {
	return []string{confederatehomestatus.NotApplicable, confederatehomestatus.Inmate, confederatehomestatus.Staffer, confederatehomestatus.Trustee}
}

func selectedPensionState(s viewmodel.PersonRecord) string {
	if strings.TrimSpace(s.PensionState) == "" {
		return pensionstate.NotApplicable
	}
	return pensionstate.Normalize(s.PensionState)
}

func selectedConfederateHomeStatus(s viewmodel.PersonRecord) string {
	switch confederatehomestatus.Normalize(s.ConfederateHomeStatus) {
	case confederatehomestatus.Inmate, confederatehomestatus.Staffer, confederatehomestatus.Trustee:
		return confederatehomestatus.Normalize(s.ConfederateHomeStatus)
	default:
		return confederatehomestatus.NotApplicable
	}
}

func rankOutValue(s viewmodel.PersonRecord) string {
	if s.RankOut != "" {
		return s.RankOut
	}
	return s.Rank
}

func pdfExcerptBudget() int {
	return 1200
}

type entryTypeOption struct {
	Value string
	Label string
}

func entryTypes() []entryTypeOption {
	return []entryTypeOption{
		{Value: models.EntryTypeSoldier, Label: "Soldier"},
		{Value: models.EntryTypeLinkedPerson, Label: "Person Record"},
		{Value: models.EntryTypeWife, Label: "Wife"},
		{Value: models.EntryTypeWidow, Label: "Widow"},
	}
}

func normalizedEntryType(s viewmodel.PersonRecord) string {
	switch s.EntryType {
	case models.EntryTypeWife, models.EntryTypeWidow, models.EntryTypeLinkedPerson:
		return s.EntryType
	default:
		return models.EntryTypeSoldier
	}
}

func entryTypeLabel(entryType string) string {
	for _, option := range entryTypes() {
		if option.Value == entryType {
			return option.Label
		}
	}
	return "Soldier"
}

func spouseCandidateLabel(s viewmodel.PersonRecord) string {
	label := strings.TrimSpace(s.GetFullName())
	if label == "" {
		label = strings.TrimSpace(s.DisplayID)
	}
	if strings.TrimSpace(s.DisplayID) == "" {
		return label
	}
	return label + " (" + s.DisplayID + ")"
}

func mergeReviewDisplayName(soldier viewmodel.PersonRecord) string {
	name := strings.TrimSpace(soldier.GetFullName())
	if name != "" {
		return name
	}
	return strings.TrimSpace(soldier.DisplayID)
}

func boolFlag(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func formRecords(records []viewmodel.SourceRecord) []viewmodel.SourceRecord {
	if len(records) == 0 {
		return []viewmodel.SourceRecord{{}}
	}
	return records
}