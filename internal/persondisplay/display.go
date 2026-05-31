package persondisplay

import "strings"

type NameParts struct {
	Prefix               string
	ShowPrefixBeforeName bool
	FirstName            string
	MiddleName           string
	LastName             string
	Suffix               string
}

func FullName(parts NameParts) string {
	nameParts := compactNameParts(prefixNamePart(parts.Prefix, parts.ShowPrefixBeforeName), parts.FirstName, parts.MiddleName, parts.LastName)
	name := strings.Join(nameParts, " ")
	suffix := strings.TrimSpace(parts.Suffix)
	if suffix == "" {
		return name
	}
	if name == "" {
		return suffix
	}
	return name + ", " + suffix
}

func SoldierServiceLine(rankOut, rank, rankIn, unit string) string {
	primaryRank := strings.TrimSpace(rankOut)
	if primaryRank == "" {
		primaryRank = strings.TrimSpace(rank)
	}
	if primaryRank == "" {
		primaryRank = strings.TrimSpace(rankIn)
	}
	trimmedUnit := strings.TrimSpace(unit)
	switch {
	case primaryRank != "" && trimmedUnit != "":
		return primaryRank + " - " + trimmedUnit
	case primaryRank != "":
		return primaryRank
	default:
		return trimmedUnit
	}
}

func prefixNamePart(prefix string, showPrefix bool) string {
	if !showPrefix {
		return ""
	}
	return prefix
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
