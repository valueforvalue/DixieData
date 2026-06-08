package records

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/valueforvalue/DixieData/internal/dates"
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

type DataQualityScanResult struct {
	Mode           DataQualityMode
	ScannedRecords int
	Issues         []DataQualityIssue
}

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
		JOIN records r ON r.soldier_id = s.id
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
