// reviews_handlers.go holds the review-queue and merge-review HTTP
// handlers. Extracted from app.go as step 11 of the God-class reduction
// tracked in issue #42. Handlers stay on *App; routes registered in
// routes.go. This is the final PR3 step.
package appshell

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/valueforvalue/DixieData/internal/presentation"
)

func (a *App) handleReviewQueue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	page := parsePage(r.URL.Query().Get("page"))
	soldiers, total, err := a.soldiers.ReviewQueue(page, 50)
	if err != nil {
		respondInternal(w, r, "Could not load the review queue.", err)
		return
	}
	soldierIDs := make([]int64, 0, len(soldiers))
	for _, soldier := range soldiers {
		soldierIDs = append(soldierIDs, soldier.ID)
	}
	findings, err := a.audit.FindingsForPersonRecords(soldierIDs)
	if err != nil {
		respondInternal(w, r, "Could not load review findings.", err)
		return
	}
	presentation.ReviewQueueView(soldiers, findings, page, total, 50).Render(r.Context(), w)
}

func (a *App) handleReviewQueueBulk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the bulk action form.", err)
		return
	}
	selected, err := parseSelectedSoldierIDs(r.Form["selected_ids"])
	if err != nil {
		respondValidation(w, r, "Could not parse selected record ids.", err)
		return
	}
	if len(selected) == 0 {
		setToastHeaderWithType(w, "Select at least one review record first.", "warning")
		fmt.Fprint(w, "Select at least one review record first.")
		return
	}
	action := strings.ToLower(strings.TrimSpace(r.FormValue("bulk_action")))
	switch action {
	case "ignore":
		for _, soldierID := range selected {
			if err := a.soldiers.MarkReviewResolved(soldierID); err != nil {
				respondInternal(w, r, fmt.Sprintf("Could not resolve review status for record %d.", soldierID), err)
				return
			}
			if err := a.audit.ResolveFindingsForPersonRecord(soldierID); err != nil {
				respondInternal(w, r, fmt.Sprintf("Could not resolve findings for record %d.", soldierID), err)
				return
			}
		}
		setToastHeader(w, fmt.Sprintf("Resolved %d review queue item(s).", len(selected)))
	case "delete":
		for _, soldierID := range selected {
			if err := a.audit.ResolveFindingsForPersonRecord(soldierID); err != nil {
				respondInternal(w, r, fmt.Sprintf("Could not resolve findings for record %d.", soldierID), err)
				return
			}
			if err := a.soldiers.Delete(soldierID); err != nil {
				respondInternal(w, r, fmt.Sprintf("Could not delete record %d.", soldierID), err)
				return
			}
		}
		setToastHeaderWithType(w, fmt.Sprintf("Deleted %d review queue record(s).", len(selected)), "success")
	default:
		respondValidation(w, r, "Unknown bulk action. Use ignore or delete.", nil)
		return
	}
	w.Header().Set("X-DixieData-Redirect", "/review-queue")
	fmt.Fprint(w, "Review queue updated.")
}

func (a *App) handleMergeReviewConflict(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/merge-review/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	conflictID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || conflictID < 1 {
		respondValidation(w, r, "Invalid merge review id.", err)
		return
	}
	var decision string
	switch parts[1] {
	case "keep-local":
		decision = "keep-local"
	case "keep-both":
		decision = "keep-both"
	case "keep-shared":
		decision = "keep-shared"
	case "use-shared":
		decision = "use-shared"
	default:
		http.NotFound(w, r)
		return
	}
	if err := a.backup.ResolveMergeConflict(conflictID, decision, a.dataDir); err != nil {
		respondInternal(w, r, "Merge review decision could not be applied.", err)
		return
	}
	w.Header().Set("X-DixieData-Redirect", "/export")
	setToastHeader(w, "Success: merge review updated.")
	fmt.Fprint(w, "Merge review updated.")
}

func (a *App) handleResolveReviewStatus(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := a.audit.ResolveFindingsForPersonRecord(id); err != nil {
		respondInternal(w, r, fmt.Sprintf("Could not resolve findings for record %d.", id), err)
		return
	}
	if err := a.soldiers.MarkReviewResolved(id); err != nil {
		respondInternal(w, r, fmt.Sprintf("Could not mark record %d as resolved.", id), err)
		return
	}
	setToastHeader(w, "Success: review item resolved.")
	if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("context")), "queue") {
		fmt.Fprint(w, "")
		return
	}
	w.Header().Set("X-DixieData-Redirect", fmt.Sprintf("/soldiers/%d", id))
	fmt.Fprint(w, "Review status cleared.")
}

func (a *App) handleFlagReviewStatus(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the review flag form.", err)
		return
	}
	reason := strings.TrimSpace(r.FormValue("review_reason"))
	if reason == "" {
		setToastHeaderWithType(w, "Add a review note before sending this record to the queue.", "warning")
		fmt.Fprint(w, "Add a review note before sending this record to the queue.")
		return
	}
	if err := a.soldiers.SetReviewStatus(id, true, reason); err != nil {
		setToastHeaderWithType(w, "Review queue update failed.", "error")
		respondInternal(w, r, fmt.Sprintf("Could not flag record %d for review.", id), err)
		return
	}
	setToastHeader(w, "Success: record added to the review queue.")
	w.Header().Set("X-DixieData-Redirect", fmt.Sprintf("/soldiers/%d", id))
	fmt.Fprint(w, "Review status updated.")
}

func (a *App) handleReviewQueueCompare(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/review-queue/compare/")
	path = strings.Trim(path, "/")
	if path == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(path, "/")
	findingID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		http.Error(w, "invalid comparison id", http.StatusBadRequest)
		return
	}
	if len(parts) > 1 && parts[1] == "resolve" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := a.audit.ResolveFinding(findingID); err != nil {
			respondInternal(w, r, fmt.Sprintf("Could not resolve duplicate audit finding %d.", findingID), err)
			return
		}
		setToastHeader(w, "Success: duplicate pair resolved.")
		w.Header().Set("X-DixieData-Redirect", "/review-queue")
		fmt.Fprint(w, "Duplicate pair resolved.")
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	comparison, err := a.audit.Comparison(findingID)
	if err != nil {
		respondNotFound(w, r, fmt.Sprintf("Duplicate audit comparison %d not found.", findingID), err)
		return
	}
	presentation.ReviewQueueCompareView(*comparison).Render(r.Context(), w)
}

func (a *App) handleCompare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id1, id2, err := compareIDsFromRequest(r)
	if err != nil {
		respondValidation(w, r, "Could not parse comparison record ids.", err)
		return
	}
	comparison, err := a.soldiers.ManualComparison(id1, id2)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondNotFound(w, r, "One of the selected records no longer exists.", err)
			return
		}
		respondInternal(w, r, "Could not build the comparison view.", err)
		return
	}
	if fromID, err := parseOptionalInt64(r.URL.Query().Get("from"), "from"); err == nil && fromID > 0 {
		comparison.BackHref = fmt.Sprintf("/soldiers/%d", fromID)
		comparison.BackLabel = "Back to Person Record"
	}
	presentation.ReviewQueueCompareView(*comparison).Render(r.Context(), w)
}
