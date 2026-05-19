package viewmodel

import "strings"

func (s Soldier) GetFullName() string {
	nameParts := compactNameParts(s.Prefix, s.FirstName, s.MiddleName, s.LastName)
	name := strings.Join(nameParts, " ")
	suffix := strings.TrimSpace(s.Suffix)
	if suffix == "" {
		return name
	}
	if name == "" {
		return suffix
	}
	return name + ", " + suffix
}

func compactNameParts(parts ...string) []string {
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values
}
