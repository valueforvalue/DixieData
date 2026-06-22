// Package render owns the Typst-backed PDF export pipeline. Slice 0 of
// the Typst migration plan extracted the fpdf code from internal/archive
// into this package; subsequent slices replaced the fpdf renderer with
// the TypstRenderer. After slice 7, fpdf is no longer part of the
// export path.
package render

import (
	"sort"
	"strings"

	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/records"
)

// SoldierLister is the slice of *records.SoldierService the bulk
// export helpers need. Defined as an interface so the render package
// does not import internal/records transitively.
type SoldierLister interface {
	List(page, pageSize int) ([]models.Soldier, int, error)
	GetByID(id int64) (*models.Soldier, error)
}

// AnalyticsSnapshot is re-aliased for callers that don't want to import
// internal/records.
type AnalyticsSnapshot = records.AnalyticsSnapshot

// AnalyticsCount is re-aliased for the same reason.
type AnalyticsCount = records.AnalyticsCount

// PrintSettings captures the user's export options for a single
// render. It was originally part of the fpdf Service's API; the
// Typst path consumes the same struct so existing call sites
// (the appshell HTTP handler, the test suite) keep working.
//
// Issue #68 splits the legacy single Template field into
// SingleRecordTemplate (used by per-record renders) and
// BulkTemplate (used by the bulk export). The bulk path must
// use BulkTemplate, never SingleRecordTemplate, because the
// bulk payload's data shape (data["soldiers"] array) is
// incompatible with per-record templates that read
// data["soldier"].
type PrintSettings struct {
	Scope                         string
	Orientation                   string
	SingleRecordTemplate          string
	BulkTemplate                  string
	PrinterFriendly               bool
	FullBiographyPage             bool
	SortBy                        string
	GroupByUnit                   bool
	GroupByPensionState           bool
	GroupByConfederateHomeStatus  bool
	GroupByBuriedIn               bool
	FilterBuriedIn                []string
	FilterEntryTypes              []string
	FilterUnits                   []string
	FilterPensionStates           []string
	FilterConfederateHomeStatuses []string
	ExportAll                     bool
	SelectedIDs                   []int64
	IncludeImages                 bool
	PrintableArchive              bool
}

// HasFilters returns true if any of the filter slices is non-empty.
func (s PrintSettings) HasFilters() bool {
	return len(s.FilterBuriedIn) > 0 ||
		len(s.FilterEntryTypes) > 0 ||
		len(s.FilterUnits) > 0 ||
		len(s.FilterPensionStates) > 0 ||
		len(s.FilterConfederateHomeStatuses) > 0
}

// PrintFilterUnknownValue is the sentinel string used by the typst
// renderers to indicate "no filter value set". Mirrored from
// internal/archive.PrintFilterUnknownValue for callers that import
// the render package directly.
const PrintFilterUnknownValue = "__unknown__"

// PDFOptions captures the per-record PDF render options.
type PDFOptions struct {
	Orientation      string `json:"orientation"`
	PrinterFriendly  bool   `json:"printerFriendly"`
	IncludeImages    bool   `json:"includeImages"`
	PrintableArchive bool   `json:"printableArchive"`
	Template         string `json:"template"`
}

// Normalize fills in the default values for the fields the caller
// left empty. The default orientation is the first argument; the
// default includeImages is the second.
func (o PDFOptions) Normalize(defaultOrientation string, defaultIncludeImages bool) PDFOptions {
	// upper-cased comparison matches the original fpdf path's
	// behaviour so existing callers (which pass mixed-case
	// orientation strings) keep working.
	if o.Orientation == "" {
		o.Orientation = defaultOrientation
	}
	return o
}

// Sort constants for PrintSettings.SortBy.
const (
	PrintSortLastName  = "last_name"
	PrintSortBirthYear = "birth_year"
	PrintSortDeathYear = "death_year"
)

// Scope constants for PrintSettings.Scope.
const (
	PrintScopeAll      = "all"
	PrintScopeFiltered = "filtered"
	PrintScopeSelected = "selected"
)

// Group constants for PrintSettings.GroupBy*.
const (
	PrintGroupUnit                  = "unit"
	PrintGroupPensionState          = "pension_state"
	PrintGroupConfederateHomeStatus = "confederate_home_status"
	PrintGroupBuriedIn              = "buried_in"
)

// Normalize fills in defaults for PrintSettings. Mirrors the
// helper that lived in the fpdf pdf_helpers.go file:
//   - scope "all" implies ExportAll = true
//   - scope "selected" with no selected IDs falls back to all
//   - scope "filtered" with no filter family set falls back to all
//   - SortBy defaults to PrintSortLastName
//   - filter slices have the PrintFilterUnknownValue sentinel
//     removed (the typst templates treat empty filter values as
//     "unknown" implicitly; the sentinel is a no-op in the new
//     path).
func (s PrintSettings) Normalize() PrintSettings {
	if s.SortBy == "" {
		s.SortBy = PrintSortLastName
	}
	s.FilterBuriedIn = stripFilterUnknown(s.FilterBuriedIn)
	s.FilterEntryTypes = stripFilterUnknown(s.FilterEntryTypes)
	s.FilterUnits = stripFilterUnknown(s.FilterUnits)
	s.FilterPensionStates = stripFilterUnknown(s.FilterPensionStates)
	s.FilterConfederateHomeStatuses = stripFilterUnknown(s.FilterConfederateHomeStatuses)
	switch s.Scope {
	case PrintScopeAll:
		s.ExportAll = true
	case PrintScopeSelected:
		if len(s.SelectedIDs) == 0 {
			s.Scope = PrintScopeAll
			s.ExportAll = true
		}
	case PrintScopeFiltered:
		if !s.HasFilters() {
			s.Scope = PrintScopeAll
			s.ExportAll = true
		}
	default:
		s.Scope = PrintScopeAll
		s.ExportAll = true
	}
	return s
}

// stripFilterUnknown removes the PrintFilterUnknownValue sentinel
// from a filter slice. The fpdf path used this sentinel as a
// placeholder; the typst path treats missing values as "unknown"
// implicitly and does not need the sentinel.
func stripFilterUnknown(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v == PrintFilterUnknownValue {
			continue
		}
		out = append(out, v)
	}
	return out
}

// FilterPrintableSoldiers returns the subset of soldiers that
// match the filter settings, when the scope is filtered. The
// helper is a no-op for the all/selected scopes. The bulk-export
// path uses it to honor the printable archive's structured
// filter inputs.
func FilterPrintableSoldiers(soldiers []models.Soldier, settings PrintSettings) []models.Soldier {
	if settings.Scope != PrintScopeFiltered || !settings.HasFilters() {
		return soldiers
	}
	filtered := make([]models.Soldier, 0, len(soldiers))
	for _, soldier := range soldiers {
		if matchesPrintableFilters(soldier, settings) {
			filtered = append(filtered, soldier)
		}
	}
	return filtered
}

func matchesPrintableFilters(soldier models.Soldier, settings PrintSettings) bool {
	buried := strings.TrimSpace(soldier.BuriedIn)
	if buried == "" {
		buried = PrintFilterUnknownValue
	}
	entryType := strings.ToLower(strings.TrimSpace(soldier.EntryType))
	if entryType == "" {
		entryType = PrintFilterUnknownValue
	}
	unit := strings.TrimSpace(soldier.Unit)
	if unit == "" {
		unit = PrintFilterUnknownValue
	}
	return matchesPrintableFilterFamily(settings.FilterBuriedIn, buried) &&
		matchesPrintableFilterFamily(settings.FilterEntryTypes, entryType) &&
		matchesPrintableFilterFamily(settings.FilterUnits, unit)
}

func matchesPrintableFilterFamily(selected []string, actual string) bool {
	if len(selected) == 0 {
		return true
	}
	for _, candidate := range selected {
		if strings.EqualFold(strings.TrimSpace(candidate), strings.TrimSpace(actual)) {
			return true
		}
	}
	return false
}

// SortPrintableSoldiers orders the soldiers in place according
// to the settings.SortBy value. Stable for last_name; falls
// back to last_name for any other value.
func SortPrintableSoldiers(soldiers []models.Soldier, settings PrintSettings) {
	sort.Slice(soldiers, func(i, j int) bool {
		switch settings.SortBy {
		case PrintSortBirthYear:
			left, _ := printYear(soldiers[i].BirthDate)
			right, _ := printYear(soldiers[j].BirthDate)
			if left != right {
				return left < right
			}
		case PrintSortDeathYear:
			left, _ := printYear(soldiers[i].DeathDate)
			right, _ := printYear(soldiers[j].DeathDate)
			if left != right {
				return left < right
			}
		}
		left := strings.ToLower(strings.TrimSpace(soldiers[i].LastName))
		right := strings.ToLower(strings.TrimSpace(soldiers[j].LastName))
		if left != right {
			return left < right
		}
		leftFirst := strings.ToLower(strings.TrimSpace(soldiers[i].FirstName))
		rightFirst := strings.ToLower(strings.TrimSpace(soldiers[j].FirstName))
		return leftFirst < rightFirst
	})
}

// printYear extracts a 4-digit year from a date string. Returns
// 0 if no year can be parsed. Handles partial dates like
// "00/00/1830" and full ISO dates.
func printYear(value string) (int, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	// Find the first 4-digit run of digits in the string.
	for i := 0; i+4 <= len(value); i++ {
		if value[i] >= '0' && value[i] <= '9' &&
			value[i+1] >= '0' && value[i+1] <= '9' &&
			value[i+2] >= '0' && value[i+2] <= '9' &&
			value[i+3] >= '0' && value[i+3] <= '9' {
			year := 0
			for j := 0; j < 4; j++ {
				year = year*10 + int(value[i+j]-'0')
			}
			if year >= 1000 && year <= 9999 {
				return year, true
			}
		}
	}
	return 0, false
}

// EmptyPDFValue returns the display value for a field. Empty
// strings become "Unknown". The fpdf path used this to keep the
// record card consistent; the typst templates now handle
// "Unknown" themselves, so this helper is preserved for
// callers that build printable archive text outside the
// templates.
func EmptyPDFValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "Unknown"
	}
	return value
}

// GroupKey describes one group in the printable archive's
// divider-page sequence. The bulk template (templates/bulk_soldier.typ)
// loops over Group and renders the divider page before the group's
// soldiers. The struct is serialised to JSON inside the typst
// data.json payload; field names below are the JSON keys the
// template reads.
type GroupKey struct {
	// Axis is the printable archive grouping axis the group was
	// built from: "unit", "pension_state",
	// "confederate_home_status", or "buried_in".
	Axis string `json:"axis"`
	// Label is the human-readable axis name the divider page
	// prints ("Unit", "Pension State", ...).
	Label string `json:"label"`
	// Value is the group's key (e.g. "Co. C, 19th AL
	// Infantry"). Empty when the soldier has no value on the
	// axis.
	Value string `json:"value"`
	// Level is the divider depth: 1 for the first (outermost)
	// axis, deeper levels reserved for nested grouping in a
	// future slice.
	Level int `json:"level"`
	// Soldiers is the sorted list of soldiers in this group.
	// The bulk template iterates these and renders a record
	// card per soldier.
	Soldiers []models.Soldier `json:"soldiers"`
}

// ActiveGroupAxis returns the grouping axis the user picked via
// PrintSettings.GroupBy*, with precedence Unit > PensionState >
// ConfederateHomeStatus > BuriedIn (matching the fpdf path's
// behaviour). Returns the empty string when no grouping is
// requested.
func ActiveGroupAxis(settings PrintSettings) string {
	switch {
	case settings.GroupByUnit:
		return PrintGroupUnit
	case settings.GroupByPensionState:
		return PrintGroupPensionState
	case settings.GroupByConfederateHomeStatus:
		return PrintGroupConfederateHomeStatus
	case settings.GroupByBuriedIn:
		return PrintGroupBuriedIn
	}
	return ""
}

// GroupPrintableSoldiers partitions an already-sorted soldiers
// slice into groups along the active axis. When no axis is
// active, returns a single group with no Label/Value so the
// template renders the records without a divider. The order of
// groups follows the order of first appearance in the input
// slice, which is the input's sort order.
func GroupPrintableSoldiers(soldiers []models.Soldier, settings PrintSettings) []GroupKey {
	axis := ActiveGroupAxis(settings)
	if axis == "" {
		return []GroupKey{{Axis: "", Soldiers: soldiers}}
	}
	indexByKey := map[string]int{}
	groups := []GroupKey{}
	for _, s := range soldiers {
		key := groupAxisValue(s, axis)
		idx, ok := indexByKey[key]
		if !ok {
			idx = len(groups)
			indexByKey[key] = idx
			groups = append(groups, GroupKey{
				Axis:     axis,
				Label:    groupAxisLabel(axis),
				Value:    groupDisplayValue(axis, key),
				Level:    1,
				Soldiers: []models.Soldier{},
			})
		}
		groups[idx].Soldiers = append(groups[idx].Soldiers, s)
	}
	return groups
}

// groupAxisValue returns the raw value of the given axis on a
// soldier. Empty values map to the same group so the divider
// page reads "(unknown)" once instead of emitting one divider per
// soldier with no value.
func groupAxisValue(s models.Soldier, axis string) string {
	switch axis {
	case PrintGroupUnit:
		return strings.TrimSpace(s.Unit)
	case PrintGroupPensionState:
		return strings.TrimSpace(s.PensionState)
	case PrintGroupConfederateHomeStatus:
		return strings.TrimSpace(s.ConfederateHomeStatus)
	case PrintGroupBuriedIn:
		return strings.TrimSpace(s.BuriedIn)
	}
	return ""
}

// groupAxisLabel returns the human-readable axis name for the
// divider page header.
func groupAxisLabel(axis string) string {
	switch axis {
	case PrintGroupUnit:
		return "Unit"
	case PrintGroupPensionState:
		return "Pension State"
	case PrintGroupConfederateHomeStatus:
		return "Confederate Home Status"
	case PrintGroupBuriedIn:
		return "Burial Location"
	}
	return ""
}

// groupDisplayValue renders the axis value for the divider page.
// Empty values become "(unknown)" so the divider page still
// names a coherent group.
func groupDisplayValue(axis, value string) string {
	if strings.TrimSpace(value) == "" {
		return "(unknown)"
	}
	return value
}
