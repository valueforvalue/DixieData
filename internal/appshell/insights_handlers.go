// insights_handlers.go holds the insights HTTP handlers: the top-level
// /insights page, the per-soldier drilldown, and the duplicate-audit
// trigger. Extracted from app.go as step 8 of the God-class reduction
// tracked in issue #42. Handlers stay on *App; routes registered in
// routes.go. The handleExportInsightsPDF (registered as /insights/report/pdf)
// is the export-side counterpart and lives in exports_handlers.go.
package appshell

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/valueforvalue/DixieData/internal/jobs"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/presentation"
)

func (a *App) handleInsights(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	snapshot, err := a.analytics.Snapshot()
	if err != nil {
		respondInternal(w, r, "Could not build the insights snapshot.", err)
		return
	}
	counts := models.ArchiveCounts{
		TotalSoldiers:     snapshot.RecordTypes.TotalSoldiers,
		TotalWivesWidows:  snapshot.RecordTypes.TotalWivesWidows,
		TotalLinkedPeople: snapshot.RecordTypes.TotalLinkedPeople,
	}
	presentation.InsightsView(snapshot, counts).Render(r.Context(), w)
}

func (a *App) handleInsightsDrilldown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	scope := strings.TrimSpace(r.URL.Query().Get("scope"))
	value := strings.TrimSpace(r.URL.Query().Get("value"))
	page := parsePage(r.URL.Query().Get("page"))

	title, description, search, useGroupedSpouseQuery, err := insightDrilldownConfig(scope, value)
	if err != nil {
		respondValidation(w, r, "Invalid insights drilldown scope.", err)
		return
	}

	var (
		soldiers []models.Soldier
		total    int
	)
	if useGroupedSpouseQuery {
		soldiers, total, err = a.soldiers.ListByEntryTypes([]string{"wife", "widow"}, page, 50)
	} else {
		soldiers, total, err = a.soldiers.AdvancedSearch(search, page, 50)
	}
	if err != nil {
		respondInternal(w, r, "Could not run the insights drilldown search.", err)
		return
	}
	presentation.InsightsDrilldownView(title, description, soldiers, search, page, total, 50, scope, value).Render(r.Context(), w)
}

func insightDrilldownConfig(scope, value string) (string, string, models.SoldierSearch, bool, error) {
	search := models.SoldierSearch{Mode: "advanced"}
	switch scope {
	case "entry_type":
		switch strings.ToLower(value) {
		case "soldier":
			search.EntryType = "soldier"
			return "Soldier Records", "Records included in the Insights soldier total.", search, false, nil
		case "linked_person":
			search.EntryType = "linked_person"
			return "Person Records", "Records included in the Insights person-record total.", search, false, nil
		case "spouse":
			return "Spouse Records", "Wife and widow records included in the Insights spouse total.", search, true, nil
		}
	case "buried_in":
		search.BuriedIn = value
		return "Burial Drilldown", fmt.Sprintf("Records buried in %s.", value), search, false, nil
	case "confederate_home_status":
		search.ConfederateHomeStatus = value
		return "Confederate Home Status", fmt.Sprintf("Records with Confederate Home status set to %s.", value), search, false, nil
	case "confederate_home_name":
		search.ConfederateHomeName = value
		return "Confederate Home Name", fmt.Sprintf("Records tied to %s.", value), search, false, nil
	case "pension_state":
		search.PensionState = value
		return "Pension State", fmt.Sprintf("Records with pension state %s.", value), search, false, nil
	case "unit":
		search.Unit = value
		return "Unit Drilldown", fmt.Sprintf("Records tied to %s.", value), search, false, nil
	case "birth_decade":
		decade, err := insightDecadeValue(value)
		if err != nil {
			return "", "", models.SoldierSearch{}, false, err
		}
		search.BirthYear = fmt.Sprintf("%d", decade)
		search.BirthYearTo = fmt.Sprintf("%d", decade+9)
		return "Birth Decade Drilldown", fmt.Sprintf("Records with birth years in the %ds.", decade), search, false, nil
	case "death_decade":
		decade, err := insightDecadeValue(value)
		if err != nil {
			return "", "", models.SoldierSearch{}, false, err
		}
		search.DeathYear = fmt.Sprintf("%d", decade)
		search.DeathYearTo = fmt.Sprintf("%d", decade+9)
		return "Death Decade Drilldown", fmt.Sprintf("Records with death years in the %ds.", decade), search, false, nil
	case "review_status":
		search.ReviewStatus = value
		return "Review Queue Drilldown", "Records currently flagged for review from the duplicate audit and research queue.", search, false, nil
	}
	return "", "", models.SoldierSearch{}, false, fmt.Errorf("unknown insight drilldown")
}

func insightDecadeValue(value string) (int, error) {
	trimmed := strings.TrimSpace(strings.TrimSuffix(strings.ToLower(value), "s"))
	if len(trimmed) != 4 {
		return 0, fmt.Errorf("invalid decade")
	}
	decade, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, fmt.Errorf("invalid decade")
	}
	return decade, nil
}

func (a *App) handleRunDuplicateAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Defer the audit to a background job so the user sees real
	// progress. The audit can scan hundreds of records; the prior
	// inline path blocked the request goroutine for the duration.
	var jobID string
	jobID = a.jobs.Start("duplicate_audit", func(ctx context.Context, p *jobs.Progress) error {
		p.Set(10, "Scanning archive for duplicates")
		result, err := a.audit.RunDuplicateAudit()
		if err != nil {
			return err
		}
		p.Set(100, fmt.Sprintf("Scanned %d records, %d candidate pairs (%d suppressed).", result.ScannedRecords, result.FindingsDiscovered, result.FindingsSuppressed))
		_ = jobID
		return nil
	})
	setToastHeader(w, "Duplicate audit started\u2026")
	w.Header().Set("Location", "/jobs/"+jobID)
	w.WriteHeader(http.StatusSeeOther)
}
