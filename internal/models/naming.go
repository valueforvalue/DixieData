package models

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

func (u UserIdentity) BrandingName() string {
	firstInitial := firstInitial(u.FirstName)
	lastName := strings.TrimSpace(u.LastName)
	switch {
	case firstInitial != "" && lastName != "":
		return firstInitial + ". " + lastName
	case lastName != "":
		return lastName
	default:
		return ""
	}
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

func firstInitial(value string) string {
	for _, r := range strings.ToUpper(strings.TrimSpace(value)) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return string(r)
		}
	}
	return ""
}
