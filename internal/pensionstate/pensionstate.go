package pensionstate

import "strings"

const NotApplicable = "N/A"

func Normalize(value string) string {
	trimmed := strings.TrimSpace(value)
	switch strings.ToLower(trimmed) {
	case "", "none", "na", "n/a", "not recorded":
		return NotApplicable
	default:
		return trimmed
	}
}
