// Helpers shared between share.templ and partials/print_config_modal.templ.
// Lives in the partials package so the partial can use them without a
// reverse import of the parent templates package (would create a cycle).
// Share-time helpers that aren't referenced by the partial stay in
// internal/templates/share_helpers.go.
package partials

import (
	"fmt"
	"sort"
	"strings"

	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

// exportFilterUnknownValue is the sentinel value used to represent
// records with an unknown value for a given filter dimension.
// ExportFilterUnknownValue is the sentinel value the print-config
// modal uses to represent records with an unknown value for a
// given filter dimension. Exported so appshell handlers can
// cross-check stored template filter values against current
// archive values (issue #181) without reaching into private
// partials-package state.
const ExportFilterUnknownValue = "__unknown__"

func exportRecordOptionLabel(record viewmodel.ExportRecordOption) string {
	return ExportRecordOptionLabel(record)
}

// ExportRecordOptionLabel is the exported form of exportRecordOptionLabel.
// Used by partials/print_records_fragment.templ which is generated as
// a separate Go file in the same package but needs the public symbol
// (tlower-cased function names are package-private even across files).
func ExportRecordOptionLabel(record viewmodel.ExportRecordOption) string {
	name := strings.TrimSpace(record.DisplayName)
	if name == "" {
		name = "Unnamed Record"
	}
	entryType := exportRecordOptionEntryTypeLabel(record.EntryType)
	if entryType == "" {
		return fmt.Sprintf("%s - %s", strings.TrimSpace(record.DisplayID), name)
	}
	return fmt.Sprintf("%s - %s (%s)", strings.TrimSpace(record.DisplayID), name, entryType)
}

func exportRecordOptionEntryTypeLabel(value string) string {
	return ExportRecordOptionEntryTypeLabel(value)
}

// ExportRecordOptionEntryTypeLabel is the exported form of
// exportRecordOptionEntryTypeLabel. Mirrors ExportRecordOptionLabel.
func ExportRecordOptionEntryTypeLabel(value string) string {
	words := strings.Fields(strings.ReplaceAll(strings.TrimSpace(value), "_", " "))
	for i := range words {
		if words[i] == "" {
			continue
		}
		words[i] = strings.ToUpper(words[i][:1]) + strings.ToLower(words[i][1:])
	}
	return strings.Join(words, " ")
}

func exportRecordOptionSearchText(record viewmodel.ExportRecordOption) string {
	return ExportRecordOptionSearchText(record)
}

// ExportRecordOptionSearchText is the exported form of
// exportRecordOptionSearchText. Mirrors ExportRecordOptionLabel.
func ExportRecordOptionSearchText(record viewmodel.ExportRecordOption) string {
	return strings.ToLower(strings.TrimSpace(strings.Join([]string{
		record.DisplayID,
		record.DisplayName,
		strings.ReplaceAll(record.EntryType, "_", " "),
		record.Unit,
		record.PensionState,
		record.ConfederateHomeStatus,
		record.BuriedIn,
	}, " ")))
}

func isExportMissingValue(value string) bool {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "", "N/A", "NA", "NOT RECORDED", "NONE":
		return true
	default:
		return false
	}
}

// ExportUniqueFilterValues is the exported form of
// exportUniqueFilterValues — needed by appshell handlers
// (issue #181) that compute stale-template warnings.
func ExportUniqueFilterValues(records []viewmodel.ExportRecordOption, valueFor func(viewmodel.ExportRecordOption) string) []string {
	seen := map[string]bool{}
	values := make([]string, 0, len(records))
	for _, record := range records {
		value := strings.TrimSpace(valueFor(record))
		if isExportMissingValue(value) {
			continue
		}
		key := strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool {
		return strings.ToLower(values[i]) < strings.ToLower(values[j])
	})
	return values
}