package confederatehomestatus

import "strings"

const (
	NotApplicable = "NA"
	Inmate        = "Inmate"
	Staffer       = "Staffer"
	Trustee       = "Trustee"
)

func Normalize(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "inmate":
		return Inmate
	case "staffer":
		return Staffer
	case "trustee":
		return Trustee
	case "", "none", "na", "n/a":
		return NotApplicable
	default:
		return NotApplicable
	}
}
