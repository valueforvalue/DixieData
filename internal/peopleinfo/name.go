// Package peopleinfo provides shared name-derivation helpers used by both
// the PDF render path (internal/render) and the non-PDF export path
// (internal/archive) for JSON, iCal, and Excel exports. The functions
// here are small and only operate on the public models.Soldier type, so
// they are not coupled to either calling package.
package peopleinfo

import (
	"strings"

	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/persondisplay"
)

// SoldierFullName returns the assembled name string for the soldier.
// Returns the Display ID if the assembled name is empty.
func SoldierFullName(soldier models.Soldier) string {
	name := persondisplay.FullName(persondisplay.NameParts{
		Prefix:               soldier.Prefix,
		ShowPrefixBeforeName: soldier.ShowPrefixBeforeName,
		FirstName:            soldier.FirstName,
		MiddleName:           soldier.MiddleName,
		LastName:             soldier.LastName,
		Suffix:               soldier.Suffix,
	})
	if strings.TrimSpace(name) == "" {
		return strings.TrimSpace(soldier.DisplayID)
	}
	return name
}

// SoldierDisplayName returns the name used in lists. Prefers the assembled
// name; falls back to the entry type label when the name is empty.
func SoldierDisplayName(soldier models.Soldier) string {
	if name := SoldierFullName(soldier); name != "" {
		return name
	}
	return DisplayEntryType(soldier)
}

// DisplayEntryType returns the user-facing label for a soldier's entry type.
func DisplayEntryType(soldier models.Soldier) string {
	switch soldier.EntryType {
	case "wife":
		return "Wife"
	case "widow":
		return "Widow"
	case "linked_person":
		return "Person Record"
	default:
		return "Soldier"
	}
}
