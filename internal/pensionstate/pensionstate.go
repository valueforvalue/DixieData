package pensionstate

import "strings"

const NotApplicable = "NA"

func Normalize(value string) string {
	trimmed := strings.TrimSpace(value)
	switch strings.ToLower(trimmed) {
	case "none", "na", "n/a":
		return NotApplicable
	default:
		return trimmed
	}
}
