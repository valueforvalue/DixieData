package confederatehomestatus

import "strings"

const (
	NotApplicable = "N/A"
	Inmate        = "Inmate"
	Staffer       = "Staffer"
	Trustee       = "Trustee"
)

// Normalize collapses the known legacy "not applicable" variants
// to the canonical N/A bucket and trims surrounding whitespace.
// Unknown values (e.g. "Resident", "Applicant", "Volunteer")
// pass through (trimmed) so that browse/filter on a stored
// unknown value still matches the stored unknown value.
//
// The previous default-branch behavior of rewriting any unknown
// value to N/A silently destroyed stored data: a user who
// imported a backup with confederate_home_status="Resident"
// would see their records silently re-bucketed as N/A on the
// next browse/filter, and a user filtering by "Resident" would
// get 0 results because the filter got normalized to "N/A".
// See TestNormalizeUnknownValuePreserves for the regression net.
//
// Mirrors the pattern in internal/pensionstate/pensionstate.go,
// which has the same shape and which was already correct.
func Normalize(value string) string {
	trimmed := strings.TrimSpace(value)
	switch strings.ToLower(trimmed) {
	case "inmate":
		return Inmate
	case "staffer":
		return Staffer
	case "trustee":
		return Trustee
	case "", "none", "na", "n/a", "not recorded":
		return NotApplicable
	default:
		return trimmed
	}
}
