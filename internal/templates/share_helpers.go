// Helper functions for share.templ. Pure Go, no templ syntax; lives
// here so the .templ file stays focused on markup.
//
// The export-related helpers (exportFilterUnknownValue,
// exportRecordOptionLabel, exportUniqueFilterValues, etc.) moved to
// internal/templates/partials/print_config_helpers.go in issue #176
// so the extracted print-config modal partial can use them without
// a reverse package import. The helpers used by share.templ itself
// for the merge-review section (mergeReviewConfirmMessage and the
// exportHasUnknownValue helper) remain here.
package templates

import (
	"fmt"
	"strings"

	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

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