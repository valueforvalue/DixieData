package viewmodel

import (
	"github.com/valueforvalue/DixieData/internal/persondisplay"
)

// GetFullName returns the canonical display name for the Person
// Record, formatted per the rules in internal/persondisplay.FullName
// (prefix-before-name toggle, suffix handling, etc.). Used by the
// browse list, the soldier header, and every place the UI shows
// the soldier's name in long form.
func (s PersonRecord) GetFullName() string {
	return persondisplay.FullName(persondisplay.NameParts{
		Prefix:               s.Prefix,
		ShowPrefixBeforeName: s.ShowPrefixBeforeName,
		FirstName:            s.FirstName,
		MiddleName:           s.MiddleName,
		LastName:             s.LastName,
		Suffix:               s.Suffix,
	})
}
