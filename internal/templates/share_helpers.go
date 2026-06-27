// Helper functions for share.templ. Pure Go, no templ syntax; lives
// here so the .templ file stays focused on markup. Functions here
// are referenced from share.templ; keep them in this file rather
// than duplicating across the split when PR #F3 lands.
//
// When extracting helpers from share.templ, follow this rule: if
// the function body contains only Go (no templ syntax, no references
// to other templ symbols), move it here. Functions that reference
// templ Components or other templ symbols must stay in the .templ
// file.
package templates

import (
	"fmt"
	"sort"
	"strings"

	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

// exportFilterUnknownValue is the sentinel value used to represent
// records with an unknown value for a given filter dimension.
const exportFilterUnknownValue = "__unknown__"

func exportRecordOptionLabel(record viewmodel.ExportRecordOption) string {
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

func exportUniqueFilterValues(records []viewmodel.ExportRecordOption, valueFor func(viewmodel.ExportRecordOption) string) []string {
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

func exportHasUnknownValue(records []viewmodel.ExportRecordOption, valueFor func(viewmodel.ExportRecordOption) string) bool {
	for _, record := range records {
		if isExportMissingValue(valueFor(record)) {
			return true
		}
	}
	return false
}

func mergeReviewConfirmMessage(action string, conflict viewmodel.MergeReviewConflict) string {
	localID := conflict.LocalDisplayID
	if strings.TrimSpace(localID) == "" {
		if conflict.LocalRecord != nil {
			localID = conflict.LocalRecord.DisplayID
		} else {
			localID = "(empty)"
		}
	}
	incomingID := strings.TrimSpace(conflict.IncomingDisplayID)
	if incomingID == "" {
		incomingID = strings.TrimSpace(conflict.IncomingRecord.DisplayID)
	}
	if incomingID == "" {
		incomingID = "(empty)"
	}
	switch action {
	case "Keep Local":
		return fmt.Sprintf("Keep Local: keep '%s' and drop '%s'? The incoming Person Record will not be added.", localID, incomingID)
	case "Keep Incoming":
		return fmt.Sprintf("Keep Incoming: drop '%s' and replace with '%s'? The local Person Record will be overwritten (Source Records, images, and scratch pad remain).", localID, incomingID)
	case "Keep Both":
		return fmt.Sprintf("Keep Both: keep '%s' and import '%s' under a new unique Display ID? Both Person Records will be in your Local Archive after this.", localID, incomingID)
	}
	return fmt.Sprintf("Confirm %s for conflict between '%s' and '%s'?", action, localID, incomingID)
}