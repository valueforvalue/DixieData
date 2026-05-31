package viewmodel

import (
	"github.com/valueforvalue/DixieData/internal/persondisplay"
)

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
