package records

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/valueforvalue/DixieData/internal/dates"
	"github.com/valueforvalue/DixieData/internal/models"
)

type DataQualityMode string

const (
	DataQualityModeHighConfidence DataQualityMode = "high-confidence"
	DataQualityModeAdvanced       DataQualityMode = "advanced"

	qualityReviewReasonMarker = "Heuristic scan flagged data-quality issues."
)

type DataQualityIssue struct {
	SoldierID int64
	DisplayID string
	Name      string
	EntryType string
	Group     string
	Code      string
	Severity  string
	Summary   string
	Detail    string
}

// DataQualityScanResult is a records-layer type used by the matching service.
type DataQualityScanResult struct {
	Mode           DataQualityMode
	ScannedRecords int
	Issues         []DataQualityIssue
}

// DataQualityApplyResult is a records-layer type used by the matching service.
type DataQualityApplyResult struct {
	Selected       int
	Flagged        int
	AlreadyInQueue int
	NotFound       int
}

type qualityScanCandidate struct {
	ID              int64
	DisplayID       string
	EntryType       string
	SpouseSoldierID int64
	FirstName       string
	MiddleName      string
	LastName        string
	BirthDate       string
	DeathDate       string
	BirthInfo       string
	BuriedIn        string
}

func normalizeDataQualityMode(raw string) DataQualityMode {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(DataQualityModeAdvanced):
		return DataQualityModeAdvanced
	default:
		return DataQualityModeHighConfidence
	}
}

// RunDataQualityScan runs the data-quality scan (per DataQualityMode) over every Soldier; returns the per-kind issue-group rollup.
func (s *SoldierService) RunDataQualityScan(modeRaw string) (DataQualityScanResult, error) {
	mode := normalizeDataQualityMode(modeRaw)
	candidates, err := s.loadQualityScanCandidates()
	if err != nil {
		return DataQualityScanResult{}, err
	}

	issues := make([]DataQualityIssue, 0)
	spouseTypes, err := s.loadEntryTypesByID()
	if err != nil {
		return DataQualityScanResult{}, err
	}

	for _, candidate := range candidates {
		issues = append(issues, evaluateQualityIssues(candidate, spouseTypes, mode)...)
	}

	// v60 (issue #320): Event Records with zero links to any
	// Person Record are review-queue candidates. The Event exists
	// but is orphaned (the researcher created it but hasn't
	// attached it to anyone yet, or the attached Person Records
	// were all deleted). This check fires for every Event Record
	// regardless of scan mode (the zero-link condition is a
	// structural integrity issue, not a content-quality issue).
	eventLinkIssues, err := s.loadEventZeroLinkIssues()
	if err != nil {
		return DataQualityScanResult{}, err
	}
	issues = append(issues, eventLinkIssues...)

	if mode == DataQualityModeAdvanced {
		advancedIssues, err := s.loadAdvancedSourceRecordIssues()
		if err != nil {
			return DataQualityScanResult{}, err
		}
		issues = append(issues, advancedIssues...)
	}

	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Group != issues[j].Group {
			return issues[i].Group < issues[j].Group
		}
		if issues[i].DisplayID != issues[j].DisplayID {
			return issues[i].DisplayID < issues[j].DisplayID
		}
		return issues[i].Code < issues[j].Code
	})

	return DataQualityScanResult{
		Mode:           mode,
		ScannedRecords: len(candidates),
		Issues:         issues,
	}, nil
}

// ApplyDataQualityFindingsToReviewQueue turns one DataQualityIssueGroup into a Review Queue entry for the user to resolve manually.
func (s *SoldierService) ApplyDataQualityFindingsToReviewQueue(ids []int64) (DataQualityApplyResult, error) {
	result := DataQualityApplyResult{}
	uniqueIDs := dedupePositiveIDs(ids)
	result.Selected = len(uniqueIDs)
	for _, id := range uniqueIDs {
		var (
			needsReview bool
			reason      string
		)
		err := s.db.Conn().QueryRow(`SELECT needs_review, COALESCE(review_reason, '') FROM soldiers WHERE id = ?`, id).Scan(&needsReview, &reason)
		if err != nil {
			if err == sql.ErrNoRows {
				result.NotFound++
				continue
			}
			return DataQualityApplyResult{}, err
		}

		nextReason := mergeQualityReviewReason(reason)
		if needsReview {
			if strings.TrimSpace(nextReason) != strings.TrimSpace(reason) {
				if _, err := s.db.Conn().Exec(`UPDATE soldiers SET review_reason = ? WHERE id = ?`, nextReason, id); err != nil {
					return DataQualityApplyResult{}, err
				}
				if err := s.touchAuditFields(id, "review_status"); err != nil {
					return DataQualityApplyResult{}, err
				}
			}
			result.AlreadyInQueue++
			continue
		}

		if err := s.SetReviewStatus(id, true, nextReason); err != nil {
			return DataQualityApplyResult{}, err
		}
		result.Flagged++
	}
	return result, nil
}

func mergeQualityReviewReason(existing string) string {
	trimmed := strings.TrimSpace(existing)
	if trimmed == "" {
		return qualityReviewReasonMarker
	}
	if strings.Contains(strings.ToLower(trimmed), strings.ToLower(qualityReviewReasonMarker)) {
		return trimmed
	}
	return trimmed + " | " + qualityReviewReasonMarker
}

func dedupePositiveIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	unique := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id < 1 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	return unique
}

func (s *SoldierService) loadQualityScanCandidates() ([]qualityScanCandidate, error) {
	rows, err := s.db.Conn().Query(`
		SELECT id, display_id, entry_type, COALESCE(spouse_soldier_id, 0),
		       COALESCE(first_name, ''), COALESCE(middle_name, ''), COALESCE(last_name, ''),
		       COALESCE(birth_date, ''), COALESCE(death_date, ''),
		       COALESCE(birth_info, ''), COALESCE(buried_in, '')
		FROM soldiers`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	candidates := make([]qualityScanCandidate, 0)
	for rows.Next() {
		var candidate qualityScanCandidate
		if err := rows.Scan(
			&candidate.ID, &candidate.DisplayID, &candidate.EntryType, &candidate.SpouseSoldierID,
			&candidate.FirstName, &candidate.MiddleName, &candidate.LastName,
			&candidate.BirthDate, &candidate.DeathDate,
			&candidate.BirthInfo, &candidate.BuriedIn,
		); err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	return candidates, rows.Err()
}

func (s *SoldierService) loadEntryTypesByID() (map[int64]string, error) {
	rows, err := s.db.Conn().Query(`SELECT id, COALESCE(entry_type, 'soldier') FROM soldiers`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := map[int64]string{}
	for rows.Next() {
		var id int64
		var entryType string
		if err := rows.Scan(&id, &entryType); err != nil {
			return nil, err
		}
		results[id] = normalizeEntryType(entryType)
	}
	return results, rows.Err()
}

func (s *SoldierService) loadAdvancedSourceRecordIssues() ([]DataQualityIssue, error) {
	rows, err := s.db.Conn().Query(`
		SELECT s.id, COALESCE(s.display_id, ''), COALESCE(s.first_name, ''), COALESCE(s.middle_name, ''), COALESCE(s.last_name, ''), COUNT(r.id)
		FROM soldiers s
		JOIN records r ON r.person_record_id = s.id
		WHERE TRIM(COALESCE(r.record_type, '')) = ''
		  AND TRIM(COALESCE(r.app_id, '')) = ''
		  AND TRIM(COALESCE(r.details, '')) = ''
		GROUP BY s.id, s.display_id, s.first_name, s.middle_name, s.last_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	issues := make([]DataQualityIssue, 0)
	for rows.Next() {
		var (
			id               int64
			displayID        string
			firstName        string
			middleName       string
			lastName         string
			incompleteRecord int
		)
		if err := rows.Scan(&id, &displayID, &firstName, &middleName, &lastName, &incompleteRecord); err != nil {
			return nil, err
		}
		issues = append(issues, DataQualityIssue{
			SoldierID: id,
			DisplayID: strings.TrimSpace(displayID),
			Name:      buildIssueName(firstName, middleName, lastName),
			Group:     "Source Records",
			Code:      "source-record-empty",
			Severity:  "medium",
			Summary:   "One or more source records are effectively blank.",
			Detail:    fmt.Sprintf("%d source record row(s) have empty type, app ID, and details.", incompleteRecord),
		})
	}
	return issues, rows.Err()
}

func evaluateQualityIssues(candidate qualityScanCandidate, spouseTypes map[int64]string, mode DataQualityMode) []DataQualityIssue {
	issues := make([]DataQualityIssue, 0)
	entryType := normalizeEntryType(candidate.EntryType)
	displayID := strings.TrimSpace(candidate.DisplayID)
	firstName := strings.TrimSpace(candidate.FirstName)
	middleName := strings.TrimSpace(candidate.MiddleName)
	lastName := strings.TrimSpace(candidate.LastName)
	name := buildIssueName(firstName, middleName, lastName)

	if displayID == "" || (firstName == "" && lastName == "") {
		issues = append(issues, DataQualityIssue{
			SoldierID: candidate.ID,
			DisplayID: displayID,
			Name:      name,
			EntryType: entryType,
			Group:     "Identity & Naming",
			Code:      "identity-missing",
			Severity:  "high",
			Summary:   "Core identity data is missing.",
			Detail:    "Record is missing display ID or both first/last name values.",
		})
	}

	birth, birthErr := dates.ParseCanonical(candidate.BirthDate)
	death, deathErr := dates.ParseCanonical(candidate.DeathDate)
	if strings.TrimSpace(candidate.BirthDate) != "" && birthErr != nil {
		issues = append(issues, DataQualityIssue{
			SoldierID: candidate.ID,
			DisplayID: displayID,
			Name:      name,
			EntryType: entryType,
			Group:     "Dates & Chronology",
			Code:      "birth-date-invalid",
			Severity:  "high",
			Summary:   "Birth date is not in canonical format.",
			Detail:    fmt.Sprintf("Birth date %q could not be parsed as MM/DD/YYYY with 00 placeholders.", strings.TrimSpace(candidate.BirthDate)),
		})
	}
	if strings.TrimSpace(candidate.DeathDate) != "" && deathErr != nil {
		issues = append(issues, DataQualityIssue{
			SoldierID: candidate.ID,
			DisplayID: displayID,
			Name:      name,
			EntryType: entryType,
			Group:     "Dates & Chronology",
			Code:      "death-date-invalid",
			Severity:  "high",
			Summary:   "Death date is not in canonical format.",
			Detail:    fmt.Sprintf("Death date %q could not be parsed as MM/DD/YYYY with 00 placeholders.", strings.TrimSpace(candidate.DeathDate)),
		})
	}
	if birthErr == nil && deathErr == nil && chronologyClearlyInvalid(birth, death) {
		issues = append(issues, DataQualityIssue{
			SoldierID: candidate.ID,
			DisplayID: displayID,
			Name:      name,
			EntryType: entryType,
			Group:     "Dates & Chronology",
			Code:      "chronology-death-before-birth",
			Severity:  "high",
			Summary:   "Chronology is contradictory.",
			Detail:    "Death date is earlier than birth date.",
		})
	}

	if entryType != "soldier" {
		switch {
		case candidate.SpouseSoldierID < 1:
			issues = append(issues, DataQualityIssue{
				SoldierID: candidate.ID,
				DisplayID: displayID,
				Name:      name,
				EntryType: entryType,
				Group:     "Relationship Integrity",
				Code:      "spouse-link-missing",
				Severity:  "high",
				Summary:   "Spouse-linked entry is missing its linked soldier.",
				Detail:    fmt.Sprintf("Entry type %q requires a spouse_soldier_id.", entryType),
			})
		default:
			spouseType, ok := spouseTypes[candidate.SpouseSoldierID]
			if !ok {
				issues = append(issues, DataQualityIssue{
					SoldierID: candidate.ID,
					DisplayID: displayID,
					Name:      name,
					EntryType: entryType,
					Group:     "Relationship Integrity",
					Code:      "spouse-link-target-missing",
					Severity:  "high",
					Summary:   "Linked spouse target no longer exists.",
					Detail:    fmt.Sprintf("spouse_soldier_id %d does not match an existing soldier row.", candidate.SpouseSoldierID),
				})
			} else if spouseType != "soldier" {
				issues = append(issues, DataQualityIssue{
					SoldierID: candidate.ID,
					DisplayID: displayID,
					Name:      name,
					EntryType: entryType,
					Group:     "Relationship Integrity",
					Code:      "spouse-link-target-invalid",
					Severity:  "high",
					Summary:   "Linked spouse target is not a soldier record.",
					Detail:    fmt.Sprintf("spouse_soldier_id %d points to entry type %q.", candidate.SpouseSoldierID, spouseType),
				})
			}
		}
	}

	if hasObviousPlaceholderNoise(firstName, lastName, candidate.BirthInfo, candidate.BuriedIn) {
		issues = append(issues, DataQualityIssue{
			SoldierID: candidate.ID,
			DisplayID: displayID,
			Name:      name,
			EntryType: entryType,
			Group:     "Placeholder Content",
			Code:      "placeholder-noise",
			Severity:  "medium",
			Summary:   "Core fields contain obvious placeholder noise.",
			Detail:    "Detected obvious placeholder markers (for example lorem/todo/placeholder/asdf/???) in key identity or location fields.",
		})
	}

	if mode == DataQualityModeAdvanced && len(lastName) == 1 {
		issues = append(issues, DataQualityIssue{
			SoldierID: candidate.ID,
			DisplayID: displayID,
			Name:      name,
			EntryType: entryType,
			Group:     "Identity & Naming",
			Code:      "surname-too-short",
			Severity:  "medium",
			Summary:   "Last name looks unusually short.",
			Detail:    "Last name is a single character; verify this is intentional.",
		})
	}

	return issues
}

func chronologyClearlyInvalid(birth, death dates.PartialDate) bool {
	if birth.Year == 0 || death.Year == 0 {
		return false
	}
	if death.Year < birth.Year {
		return true
	}
	if death.Year > birth.Year {
		return false
	}
	if birth.Month > 0 && death.Month > 0 {
		if death.Month < birth.Month {
			return true
		}
		if death.Month > birth.Month {
			return false
		}
		if birth.Day > 0 && death.Day > 0 && death.Day < birth.Day {
			return true
		}
	}
	return false
}

func hasObviousPlaceholderNoise(values ...string) bool {
	for _, value := range values {
		lower := strings.ToLower(strings.TrimSpace(value))
		if lower == "" {
			continue
		}
		for _, marker := range []string{"lorem ipsum", "placeholder", "todo", "asdf", "???"} {
			if strings.Contains(lower, marker) {
				return true
			}
		}
	}
	return false
}

func buildIssueName(first, middle, last string) string {
	name := strings.TrimSpace(strings.Join([]string{
		strings.TrimSpace(first),
		strings.TrimSpace(middle),
		strings.TrimSpace(last),
	}, " "))
	if name == "" {
		return "Unnamed Record"
	}
	return strings.Join(strings.Fields(name), " ")
}

// loadEventZeroLinkIssues (issue #320) returns one
// DataQualityIssue per Event Record that has zero
// event_person_links rows. The Event exists but is orphaned
// (no Person Record references it). The Event is still
// reachable via the EVT-NNNNN Display ID, but the researcher
// probably forgot to attach it; the review queue surfaces the
// Event so they can either attach Person Records or delete it.
//
// The Event's user-facing name on a review-queue card is
// composed from kind + begin_date + end_date, not first/last
// name (Events have no Person Record name parts). The Display
// ID still uses the EVT-NNNNN namespace.
func (s *SoldierService) loadEventZeroLinkIssues() ([]DataQualityIssue, error) {
	rows, err := s.db.Conn().Query(
		`SELECT s.id, s.display_id, s.kind, s.begin_date, s.end_date
		 FROM soldiers s
		 LEFT JOIN event_person_links epl ON epl.event_id = s.id
		 WHERE s.entry_type = ? AND epl.id IS NULL
		 ORDER BY s.updated_at DESC, s.id DESC`,
		models.EntryTypeEvent,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var issues []DataQualityIssue
	for rows.Next() {
		var (
			id          int64
			displayID   string
			kind        string
			beginDate   string
			endDate     string
		)
		if err := rows.Scan(&id, &displayID, &kind, &beginDate, &endDate); err != nil {
			return nil, err
		}
		name := buildEventIssueName(kind, beginDate, endDate)
		issues = append(issues, DataQualityIssue{
			SoldierID: id,
			DisplayID: displayID,
			Name:      name,
			EntryType: models.EntryTypeEvent,
			Group:     "Event Integrity",
			Code:      "event-zero-links",
			Severity:  "medium",
			Summary:   "Event Record is not linked to any Person Record.",
			Detail:    "Event exists but has zero event_person_links rows. Attach at least one Person Record, or delete the Event if it was created by accident.",
		})
	}
	return issues, rows.Err()
}

// buildEventIssueName composes a user-facing label for an Event
// Record on a review-queue card. Pattern: "{kind} ({begin} - {end})"
// with the missing date fields stripped. Falls back to
// "Unnamed Event" when nothing is set.
func buildEventIssueName(kind, beginDate, endDate string) string {
	kind = strings.TrimSpace(kind)
	beginDate = strings.TrimSpace(beginDate)
	endDate = strings.TrimSpace(endDate)
	switch {
	case kind != "" && beginDate != "" && endDate != "":
		return fmt.Sprintf("%s (%s - %s)", kind, beginDate, endDate)
	case kind != "" && beginDate != "":
		return fmt.Sprintf("%s (%s)", kind, beginDate)
	case kind != "" && endDate != "":
		return fmt.Sprintf("%s (- %s)", kind, endDate)
	case kind != "":
		return kind
	case beginDate != "" || endDate != "":
		return fmt.Sprintf("%s - %s", beginDate, endDate)
	default:
		return "Unnamed Event"
	}
}
