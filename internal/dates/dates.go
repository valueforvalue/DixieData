// Package dates parses and formats partial Civil-War-era dates from canonical and freeform strings.
package dates

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// PartialDate is the canonical 3-field date shape used throughout the Local Archive. Year is 0 when unknown (the Civil War era rarely records exact birth years); Month + Day are the only required fields.
type PartialDate struct {
	Month int
	Day   int
	Year  int
}

var birthInfoDatePatterns = []struct {
	re          *regexp.Regexp
	monthIndex  int
	dayIndex    int
	yearIndex   int
	monthIsName bool
}{
	{re: regexp.MustCompile(`(?i)\b([A-Za-z]+)\.?\s+(\d{1,2}),?\s+(\d{4})\b`), monthIndex: 1, dayIndex: 2, yearIndex: 3, monthIsName: true},
	{re: regexp.MustCompile(`(?i)\b(\d{1,2})\s+([A-Za-z]+)\.?,?\s+(\d{4})\b`), monthIndex: 2, dayIndex: 1, yearIndex: 3, monthIsName: true},
	{re: regexp.MustCompile(`(?i)\b([A-Za-z]+)\.?\s+(\d{4})\b`), monthIndex: 1, yearIndex: 2, monthIsName: true},
	{re: regexp.MustCompile(`\b(\d{4})\b`), yearIndex: 1},
}

// ParseCanonical parses a partial date string into a PartialDate. Accepts year-only (\"1862\"), year+month (\"1862-03\"), full ISO (\"1862-03-15\"), and bare month-day (\"03-15\" — year defaults to 0).
func ParseCanonical(value string) (PartialDate, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return PartialDate{}, nil
	}

	parts := strings.Split(trimmed, "/")
	if len(parts) != 3 {
		return PartialDate{}, fmt.Errorf("invalid date")
	}

	month, err := parseComponent(parts[0], 0, 12)
	if err != nil {
		return PartialDate{}, fmt.Errorf("invalid date")
	}
	day, err := parseComponent(parts[1], 0, 31)
	if err != nil {
		return PartialDate{}, fmt.Errorf("invalid date")
	}
	year, err := parseComponent(parts[2], 0, 9999)
	if err != nil {
		return PartialDate{}, fmt.Errorf("invalid date")
	}
	if year > 0 && year < 1000 {
		return PartialDate{}, fmt.Errorf("invalid date")
	}
	return PartialDate{Month: month, Day: day, Year: year}, nil
}

// MustFormat formats the given year/month/day as a canonical 3-field string; panics on invalid input. Use Format for error-returning variants.
func MustFormat(month, day, year int) string {
	if month == 0 && day == 0 && year == 0 {
		return ""
	}
	return fmt.Sprintf("%02d/%02d/%04d", month, day, year)
}

// NormalizeCanonical canonicalizes a partial-date string. Strips whitespace, normalizes separators to '-', and applies the same parse rules as ParseCanonical.
func NormalizeCanonical(value string) (string, error) {
	partial, err := ParseCanonical(value)
	if err != nil {
		return "", err
	}
	return partial.Format(), nil
}

// Format formats a PartialDate as the canonical 3-field string. Returns 'unknown' when the date has no fields set.
func (p PartialDate) Format() string {
	return MustFormat(p.Month, p.Day, p.Year)
}

// HasAny reports whether the PartialDate has any field set (year, month, or day).
func (p PartialDate) HasAny() bool {
	return p.Month > 0 || p.Day > 0 || p.Year > 0
}

// HasYear reports whether the PartialDate has a non-zero year.
func (p PartialDate) HasYear() bool {
	return p.Year > 0
}

// HasMonthDay reports whether the PartialDate has a non-zero month + day.
func (p PartialDate) HasMonthDay() bool {
	return p.Month > 0 && p.Day > 0
}

// Display renders a PartialDate for UI use: \"15 March 1862\" or \"15 March\" when year is unknown.
func Display(value string) string {
	partial, err := ParseCanonical(value)
	if err != nil {
		return strings.TrimSpace(value)
	}
	if !partial.HasAny() {
		return "N/A"
	}
	if partial.Year == 0 {
		if partial.Month == 0 {
			return "N/A"
		}
		if partial.Day == 0 {
			return monthLabel(partial.Month)
		}
		return fmt.Sprintf("%s %d", monthLabel(partial.Month), partial.Day)
	}
	if partial.Month == 0 {
		return fmt.Sprintf("%d", partial.Year)
	}
	if partial.Day == 0 {
		return fmt.Sprintf("%s %d", monthLabel(partial.Month), partial.Year)
	}
	return fmt.Sprintf("%s %d, %d", monthLabel(partial.Month), partial.Day, partial.Year)
}

// DisplayUnknown renders an unknown-date placeholder ('—' by default).
func DisplayUnknown(value string) string {
	display := Display(value)
	if display == "N/A" {
		return "Unknown"
	}
	return display
}

// ParseBirthInfo extracts the per-source-record birth-date fragment from a freeform Source Record string (e.g. a census line). Heuristic; best-effort.
func ParseBirthInfo(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	for _, pattern := range birthInfoDatePatterns {
		matches := pattern.re.FindStringSubmatch(trimmed)
		if len(matches) == 0 {
			continue
		}
		partial := PartialDate{}
		if pattern.yearIndex > 0 {
			year, err := strconv.Atoi(matches[pattern.yearIndex])
			if err != nil || year < 1000 {
				continue
			}
			partial.Year = year
		}
		if pattern.monthIndex > 0 {
			monthRaw := matches[pattern.monthIndex]
			if pattern.monthIsName {
				month := parseMonthName(monthRaw)
				if month == 0 {
					continue
				}
				partial.Month = month
			}
		}
		if pattern.dayIndex > 0 {
			day, err := strconv.Atoi(matches[pattern.dayIndex])
			if err != nil || day < 1 || day > 31 {
				continue
			}
			partial.Day = day
		}
		return partial.Format()
	}
	return ""
}

func parseComponent(value string, min, max int) (int, error) {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) == 0 {
		return 0, nil
	}
	parsed, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, err
	}
	if parsed < min || parsed > max {
		return 0, fmt.Errorf("out of range")
	}
	return parsed, nil
}

func parseMonthName(value string) int {
	switch strings.Trim(strings.ToLower(strings.TrimSpace(value)), ".") {
	case "jan", "january":
		return 1
	case "feb", "february":
		return 2
	case "mar", "march":
		return 3
	case "apr", "april":
		return 4
	case "may":
		return 5
	case "jun", "june":
		return 6
	case "jul", "july":
		return 7
	case "aug", "august":
		return 8
	case "sep", "sept", "september":
		return 9
	case "oct", "october":
		return 10
	case "nov", "november":
		return 11
	case "dec", "december":
		return 12
	default:
		return 0
	}
}

func monthLabel(month int) string {
	labels := []string{"", "January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}
	if month < 1 || month >= len(labels) {
		return ""
	}
	return labels[month]
}
