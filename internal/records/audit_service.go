package records

import (
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/agnivade/levenshtein"
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
)

const (
	defaultDuplicateAuditSimilarityThreshold = 2
	duplicateAuditReasonPrefix               = "Duplicate Audit: "
)

type AuditService struct {
	db *db.DB
}

type DuplicateAuditSummary struct {
	OpenFindings        int
	ResolvedFindings    int
	LastRunAt           string
	SimilarityThreshold int
}

type DuplicateAuditRunResult struct {
	ScannedRecords     int
	FindingsDiscovered int
	FindingsCreated    int
	FindingsSuppressed int
	OpenFindings       int
}

type DuplicateAuditFindingSummary struct {
	ID             int64
	OtherSoldierID int64
	OtherDisplayID string
	OtherName      string
	Reason         string
}

type ReviewQueueEntry struct {
	Soldier           models.Soldier
	DuplicateFindings []DuplicateAuditFindingSummary
}

type DuplicateAuditComparisonField struct {
	Key         string
	Label       string
	LeftValue   string
	RightValue  string
	Highlighted bool
}

type DuplicateAuditComparison struct {
	FindingID    int64
	FindingType  string
	PageTitle    string
	BackHref     string
	BackLabel    string
	Reason       string
	Status       string
	LeftSoldier  models.Soldier
	RightSoldier models.Soldier
	Fields       []DuplicateAuditComparisonField
}

type auditCandidate struct {
	ID         int64
	DisplayID  string
	Prefix     string
	FirstName  string
	MiddleName string
	LastName   string
	Suffix     string
	BirthDate  string
	Unit       string
	BuriedIn   string
	MaidenName string
}

type duplicateAuditFindingCandidate struct {
	PairKey         string
	LeftSoldierID   int64
	RightSoldierID  int64
	FindingType     string
	Reason          string
	HighlightFields string
}

func NewAuditService(database *db.DB) *AuditService {
	return &AuditService{db: database}
}

func (s *AuditService) SimilarityThreshold() (int, error) {
	threshold := defaultDuplicateAuditSimilarityThreshold
	var raw string
	err := s.db.Conn().QueryRow(`SELECT value FROM system_config WHERE key = 'duplicate_audit_similarity_threshold'`).Scan(&raw)
	if err != nil {
		if err == sql.ErrNoRows {
			return threshold, nil
		}
		return 0, err
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || parsed < 1 || parsed > 4 {
		return threshold, nil
	}
	return parsed, nil
}

func (s *AuditService) Summary() (DuplicateAuditSummary, error) {
	threshold, err := s.SimilarityThreshold()
	if err != nil {
		return DuplicateAuditSummary{}, err
	}
	summary := DuplicateAuditSummary{SimilarityThreshold: threshold}
	if err := s.db.Conn().QueryRow(`
		SELECT
			COALESCE(SUM(CASE WHEN status = 'open' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'resolved' THEN 1 ELSE 0 END), 0),
			COALESCE(MAX(COALESCE(last_detected_at, created_at)), '')
		FROM duplicate_audit_findings`).Scan(&summary.OpenFindings, &summary.ResolvedFindings, &summary.LastRunAt); err != nil {
		return DuplicateAuditSummary{}, err
	}
	return summary, nil
}

func (s *AuditService) RunDuplicateAudit() (DuplicateAuditRunResult, error) {
	threshold, err := s.SimilarityThreshold()
	if err != nil {
		return DuplicateAuditRunResult{}, err
	}
	candidates, err := s.loadCandidates()
	if err != nil {
		return DuplicateAuditRunResult{}, err
	}
	found := discoverDuplicateAuditFindings(candidates, threshold)
	result := DuplicateAuditRunResult{
		ScannedRecords:     len(candidates),
		FindingsDiscovered: len(found),
	}

	tx, err := s.db.Conn().Begin()
	if err != nil {
		return DuplicateAuditRunResult{}, err
	}
	defer tx.Rollback()

	existing, err := loadExistingDuplicateAuditFindings(tx)
	if err != nil {
		return DuplicateAuditRunResult{}, err
	}
	now := currentSQLiteTimestamp()
	affected := map[int64]struct{}{}
	foundKeys := make(map[string]struct{}, len(found))
	for key, finding := range found {
		foundKeys[key] = struct{}{}
		if state, ok := existing[key]; ok {
			if state.Status == "resolved" {
				result.FindingsSuppressed++
				continue
			}
			if _, err := tx.Exec(`UPDATE duplicate_audit_findings
				SET finding_type = ?, reason = ?, highlight_fields = ?, last_detected_at = ?, status = 'open'
				WHERE id = ?`,
				finding.FindingType, finding.Reason, finding.HighlightFields, now, state.ID); err != nil {
				return DuplicateAuditRunResult{}, err
			}
			affected[finding.LeftSoldierID] = struct{}{}
			affected[finding.RightSoldierID] = struct{}{}
			continue
		}
		if _, err := tx.Exec(`INSERT INTO duplicate_audit_findings (pair_key, left_soldier_id, right_soldier_id, finding_type, reason, highlight_fields, status, created_at, last_detected_at)
			VALUES (?, ?, ?, ?, ?, ?, 'open', ?, ?)`,
			finding.PairKey, finding.LeftSoldierID, finding.RightSoldierID, finding.FindingType, finding.Reason, finding.HighlightFields, now, now); err != nil {
			return DuplicateAuditRunResult{}, err
		}
		result.FindingsCreated++
		affected[finding.LeftSoldierID] = struct{}{}
		affected[finding.RightSoldierID] = struct{}{}
	}

	for key, state := range existing {
		if state.Status != "open" {
			continue
		}
		if _, ok := foundKeys[key]; ok {
			continue
		}
		if _, err := tx.Exec(`DELETE FROM duplicate_audit_findings WHERE id = ?`, state.ID); err != nil {
			return DuplicateAuditRunResult{}, err
		}
		affected[state.LeftSoldierID] = struct{}{}
		affected[state.RightSoldierID] = struct{}{}
	}

	for soldierID := range affected {
		if err := s.syncSoldierDuplicateReviewStateTx(tx, soldierID); err != nil {
			return DuplicateAuditRunResult{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return DuplicateAuditRunResult{}, err
	}

	summary, err := s.Summary()
	if err != nil {
		return DuplicateAuditRunResult{}, err
	}
	result.OpenFindings = summary.OpenFindings
	return result, nil
}

func (s *AuditService) ResolveFinding(findingID int64) error {
	tx, err := s.db.Conn().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var leftID, rightID int64
	err = tx.QueryRow(`SELECT left_soldier_id, right_soldier_id FROM duplicate_audit_findings WHERE id = ?`, findingID).Scan(&leftID, &rightID)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE duplicate_audit_findings SET status = 'resolved', resolved_at = ? WHERE id = ?`, currentSQLiteTimestamp(), findingID); err != nil {
		return err
	}
	if err := s.syncSoldierDuplicateReviewStateTx(tx, leftID); err != nil {
		return err
	}
	if err := s.syncSoldierDuplicateReviewStateTx(tx, rightID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *AuditService) ResolveFindingsForSoldier(soldierID int64) error {
	tx, err := s.db.Conn().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.Query(`SELECT id, left_soldier_id, right_soldier_id FROM duplicate_audit_findings WHERE status = 'open' AND (left_soldier_id = ? OR right_soldier_id = ?)`, soldierID, soldierID)
	if err != nil {
		return err
	}
	defer rows.Close()

	affected := map[int64]struct{}{soldierID: {}}
	var findingIDs []int64
	for rows.Next() {
		var findingID, leftID, rightID int64
		if err := rows.Scan(&findingID, &leftID, &rightID); err != nil {
			return err
		}
		findingIDs = append(findingIDs, findingID)
		affected[leftID] = struct{}{}
		affected[rightID] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(findingIDs) > 0 {
		placeholders := make([]string, len(findingIDs))
		args := make([]any, 0, len(findingIDs)+1)
		args = append(args, currentSQLiteTimestamp())
		for index, findingID := range findingIDs {
			placeholders[index] = "?"
			args = append(args, findingID)
		}
		if _, err := tx.Exec(`UPDATE duplicate_audit_findings SET status = 'resolved', resolved_at = ? WHERE id IN (`+strings.Join(placeholders, ",")+`)`, args...); err != nil {
			return err
		}
	}
	for id := range affected {
		if err := s.syncSoldierDuplicateReviewStateTx(tx, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *AuditService) FindingsForSoldiers(soldierIDs []int64) (map[int64][]DuplicateAuditFindingSummary, error) {
	if len(soldierIDs) == 0 {
		return map[int64][]DuplicateAuditFindingSummary{}, nil
	}
	placeholders := make([]string, len(soldierIDs))
	args := make([]any, 0, len(soldierIDs)*2)
	for index, soldierID := range soldierIDs {
		placeholders[index] = "?"
		args = append(args, soldierID)
	}
	args = append(args, args[:len(soldierIDs)]...)
	rows, err := s.db.Conn().Query(`
		SELECT id, left_soldier_id, right_soldier_id, reason
		FROM duplicate_audit_findings
		WHERE status = 'open' AND (left_soldier_id IN (`+strings.Join(placeholders, ",")+`) OR right_soldier_id IN (`+strings.Join(placeholders, ",")+`))
		ORDER BY id ASC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type findingRow struct {
		ID      int64
		LeftID  int64
		RightID int64
		Reason  string
	}
	var findings []findingRow
	lookupIDs := map[int64]struct{}{}
	for rows.Next() {
		var row findingRow
		if err := rows.Scan(&row.ID, &row.LeftID, &row.RightID, &row.Reason); err != nil {
			return nil, err
		}
		findings = append(findings, row)
		lookupIDs[row.LeftID] = struct{}{}
		lookupIDs[row.RightID] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	candidates, err := s.lookupCandidateMap(lookupIDs)
	if err != nil {
		return nil, err
	}
	results := make(map[int64][]DuplicateAuditFindingSummary, len(soldierIDs))
	for _, row := range findings {
		left := candidates[row.LeftID]
		right := candidates[row.RightID]
		leftSummary := DuplicateAuditFindingSummary{
			ID:             row.ID,
			OtherSoldierID: row.RightID,
			OtherDisplayID: strings.TrimSpace(right.DisplayID),
			OtherName:      strings.TrimSpace(right.GetFullName()),
			Reason:         row.Reason,
		}
		rightSummary := DuplicateAuditFindingSummary{
			ID:             row.ID,
			OtherSoldierID: row.LeftID,
			OtherDisplayID: strings.TrimSpace(left.DisplayID),
			OtherName:      strings.TrimSpace(left.GetFullName()),
			Reason:         row.Reason,
		}
		results[row.LeftID] = append(results[row.LeftID], leftSummary)
		results[row.RightID] = append(results[row.RightID], rightSummary)
	}
	return results, nil
}

func (s *AuditService) Comparison(findingID int64) (*DuplicateAuditComparison, error) {
	var (
		leftID          int64
		rightID         int64
		findingType     string
		reason          string
		highlightFields string
		status          string
	)
	err := s.db.Conn().QueryRow(`SELECT left_soldier_id, right_soldier_id, finding_type, reason, highlight_fields, status FROM duplicate_audit_findings WHERE id = ?`, findingID).
		Scan(&leftID, &rightID, &findingType, &reason, &highlightFields, &status)
	if err != nil {
		return nil, err
	}
	leftSoldier, err := NewSoldierService(s.db).GetByID(leftID)
	if err != nil {
		return nil, err
	}
	rightSoldier, err := NewSoldierService(s.db).GetByID(rightID)
	if err != nil {
		return nil, err
	}
	highlightSet := parseHighlightFields(highlightFields)
	return &DuplicateAuditComparison{
		FindingID:    findingID,
		FindingType:  findingType,
		PageTitle:    "Duplicate Comparison",
		BackHref:     "/review-queue",
		BackLabel:    "Back to Review Queue",
		Reason:       reason,
		Status:       status,
		LeftSoldier:  *leftSoldier,
		RightSoldier: *rightSoldier,
		Fields:       buildDuplicateAuditComparisonFields(*leftSoldier, *rightSoldier, highlightSet),
	}, nil
}

func (s *AuditService) loadCandidates() ([]auditCandidate, error) {
	rows, err := s.db.Conn().Query(`
		SELECT
			id,
			COALESCE(display_id, ''),
			COALESCE(prefix, ''),
			COALESCE(first_name, ''),
			COALESCE(middle_name, ''),
			COALESCE(last_name, ''),
			COALESCE(suffix, ''),
			COALESCE(birth_date, ''),
			COALESCE(unit, ''),
			COALESCE(buried_in, ''),
			COALESCE(maiden_name, '')
		FROM soldiers
		ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	candidates := []auditCandidate{}
	for rows.Next() {
		var candidate auditCandidate
		if err := rows.Scan(&candidate.ID, &candidate.DisplayID, &candidate.Prefix, &candidate.FirstName, &candidate.MiddleName, &candidate.LastName, &candidate.Suffix, &candidate.BirthDate, &candidate.Unit, &candidate.BuriedIn, &candidate.MaidenName); err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	return candidates, rows.Err()
}

type duplicateAuditFindingState struct {
	ID             int64
	Status         string
	LeftSoldierID  int64
	RightSoldierID int64
}

func loadExistingDuplicateAuditFindings(tx *sql.Tx) (map[string]duplicateAuditFindingState, error) {
	rows, err := tx.Query(`SELECT id, pair_key, status, left_soldier_id, right_soldier_id FROM duplicate_audit_findings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	states := map[string]duplicateAuditFindingState{}
	for rows.Next() {
		var (
			state   duplicateAuditFindingState
			pairKey string
		)
		if err := rows.Scan(&state.ID, &pairKey, &state.Status, &state.LeftSoldierID, &state.RightSoldierID); err != nil {
			return nil, err
		}
		states[pairKey] = state
	}
	return states, rows.Err()
}

func discoverDuplicateAuditFindings(candidates []auditCandidate, threshold int) map[string]duplicateAuditFindingCandidate {
	found := map[string]duplicateAuditFindingCandidate{}
	addFinding := func(left, right auditCandidate, findingType, reason string, highlightFields ...string) {
		if left.ID == right.ID {
			return
		}
		leftID, rightID := orderedIDs(left.ID, right.ID)
		pairKey := fmt.Sprintf("%d:%d", leftID, rightID)
		if _, exists := found[pairKey]; exists {
			return
		}
		found[pairKey] = duplicateAuditFindingCandidate{
			PairKey:         pairKey,
			LeftSoldierID:   leftID,
			RightSoldierID:  rightID,
			FindingType:     findingType,
			Reason:          duplicateAuditReasonPrefix + reason,
			HighlightFields: strings.Join(uniqueStringsPreserveOrder(highlightFields), ","),
		}
	}

	pass1Groups := map[string][]auditCandidate{}
	for _, candidate := range candidates {
		birthYear, ok := auditBirthYear(candidate.BirthDate)
		if !ok {
			continue
		}
		firstName := normalizeAuditName(candidate.FirstName)
		lastName := normalizeAuditName(candidate.LastName)
		unit := normalizeAuditText(candidate.Unit)
		if firstName == "" || lastName == "" || unit == "" {
			continue
		}
		key := strings.Join([]string{firstName, lastName, strconv.Itoa(birthYear), unit}, "|")
		pass1Groups[key] = append(pass1Groups[key], candidate)
	}
	for _, group := range pass1Groups {
		addAuditPairs(group, func(left, right auditCandidate) {
			addFinding(left, right, "exact-human-match",
				fmt.Sprintf("Exact human match: %s and %s share the same first name, last name, birth year, and unit.", auditCandidateLabel(left), auditCandidateLabel(right)),
				"first_name", "last_name", "birth_year", "unit")
		})
	}

	pass2Groups := map[string][]auditCandidate{}
	for _, candidate := range candidates {
		birthYear, ok := auditBirthYear(candidate.BirthDate)
		if !ok {
			continue
		}
		lastName := normalizeAuditName(candidate.LastName)
		firstName := normalizeAuditName(candidate.FirstName)
		if lastName == "" || firstName == "" {
			continue
		}
		key := strings.Join([]string{lastName, strconv.Itoa(birthYear)}, "|")
		pass2Groups[key] = append(pass2Groups[key], candidate)
	}
	for _, group := range pass2Groups {
		if len(group) < 2 || len(group) > 40 {
			continue
		}
		for leftIndex := 0; leftIndex < len(group)-1; leftIndex++ {
			for rightIndex := leftIndex + 1; rightIndex < len(group); rightIndex++ {
				left := group[leftIndex]
				right := group[rightIndex]
				leftName := normalizeAuditName(left.FirstName)
				rightName := normalizeAuditName(right.FirstName)
				if leftName == "" || rightName == "" || leftName == rightName {
					continue
				}
				if levenshtein.ComputeDistance(leftName, rightName) > threshold {
					continue
				}
				addFinding(left, right, "fuzzy-first-name",
					fmt.Sprintf("Fuzzy match: %q and %q share the same last name and birth year.", strings.TrimSpace(left.FirstName), strings.TrimSpace(right.FirstName)),
					"first_name", "last_name", "birth_year")
			}
		}
	}

	locationGroups := map[string][]auditCandidate{}
	maidenGroups := map[string][]auditCandidate{}
	for _, candidate := range candidates {
		fullName := normalizeAuditName(auditCandidateFullName(candidate))
		if fullName != "" {
			buriedIn := normalizeAuditText(candidate.BuriedIn)
			if buriedIn != "" {
				locationGroups[fullName+"|"+buriedIn] = append(locationGroups[fullName+"|"+buriedIn], candidate)
			}
		}
		lastName := normalizeAuditName(candidate.LastName)
		maiden := normalizeAuditName(candidate.MaidenName)
		unit := normalizeAuditText(candidate.Unit)
		if lastName != "" && maiden != "" && unit != "" {
			maidenGroups[lastName+"|"+maiden+"|"+unit] = append(maidenGroups[lastName+"|"+maiden+"|"+unit], candidate)
		}
	}
	for _, group := range locationGroups {
		addAuditPairs(group, func(left, right auditCandidate) {
			addFinding(left, right, "burial-location-match",
				fmt.Sprintf("Location match: %s and %s share the same full name and burial location.", auditCandidateLabel(left), auditCandidateLabel(right)),
				"prefix", "first_name", "middle_name", "last_name", "suffix", "buried_in")
		})
	}
	for _, group := range maidenGroups {
		addAuditPairs(group, func(left, right auditCandidate) {
			addFinding(left, right, "maiden-name-match",
				fmt.Sprintf("Family match: %s and %s share the same last name, maiden name, and unit.", auditCandidateLabel(left), auditCandidateLabel(right)),
				"last_name", "maiden_name", "unit")
		})
	}

	return found
}

func addAuditPairs(group []auditCandidate, pair func(left, right auditCandidate)) {
	if len(group) < 2 {
		return
	}
	for leftIndex := 0; leftIndex < len(group)-1; leftIndex++ {
		for rightIndex := leftIndex + 1; rightIndex < len(group); rightIndex++ {
			pair(group[leftIndex], group[rightIndex])
		}
	}
}

func orderedIDs(left, right int64) (int64, int64) {
	if left < right {
		return left, right
	}
	return right, left
}

func normalizeAuditText(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
}

func normalizeAuditName(value string) string {
	replacer := strings.NewReplacer(".", "", ",", "", "'", "", "\"", "", "-", "", "_", "", "(", "", ")", "")
	return normalizeAuditText(replacer.Replace(value))
}

func auditBirthYear(canonicalDate string) (int, bool) {
	trimmed := strings.TrimSpace(canonicalDate)
	if len(trimmed) < 4 {
		return 0, false
	}
	year, err := strconv.Atoi(trimmed[len(trimmed)-4:])
	if err != nil || year < 1000 {
		return 0, false
	}
	return year, true
}

func auditCandidateFullName(candidate auditCandidate) string {
	parts := []string{
		strings.TrimSpace(candidate.Prefix),
		strings.TrimSpace(candidate.FirstName),
		strings.TrimSpace(candidate.MiddleName),
		strings.TrimSpace(candidate.LastName),
		strings.TrimSpace(candidate.Suffix),
	}
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			values = append(values, part)
		}
	}
	return strings.Join(values, " ")
}

func auditCandidateLabel(candidate auditCandidate) string {
	if displayID := strings.TrimSpace(candidate.DisplayID); displayID != "" {
		return displayID
	}
	if fullName := strings.TrimSpace(auditCandidateFullName(candidate)); fullName != "" {
		return fullName
	}
	return fmt.Sprintf("record %d", candidate.ID)
}

func uniqueStringsPreserveOrder(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func parseHighlightFields(raw string) map[string]struct{} {
	set := map[string]struct{}{}
	for _, part := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			set[trimmed] = struct{}{}
		}
	}
	return set
}

func buildDuplicateAuditComparisonFields(left, right models.Soldier, highlights map[string]struct{}) []DuplicateAuditComparisonField {
	birthYearLeft, _ := auditBirthYear(left.BirthDate)
	birthYearRight, _ := auditBirthYear(right.BirthDate)
	fields := []DuplicateAuditComparisonField{
		{Key: "display_id", Label: "Display ID", LeftValue: auditComparisonValue(left.DisplayID), RightValue: auditComparisonValue(right.DisplayID)},
		{Key: "prefix", Label: "Prefix", LeftValue: auditComparisonValue(left.Prefix), RightValue: auditComparisonValue(right.Prefix)},
		{Key: "first_name", Label: "First Name", LeftValue: auditComparisonValue(left.FirstName), RightValue: auditComparisonValue(right.FirstName)},
		{Key: "middle_name", Label: "Middle Name", LeftValue: auditComparisonValue(left.MiddleName), RightValue: auditComparisonValue(right.MiddleName)},
		{Key: "last_name", Label: "Last Name", LeftValue: auditComparisonValue(left.LastName), RightValue: auditComparisonValue(right.LastName)},
		{Key: "suffix", Label: "Suffix", LeftValue: auditComparisonValue(left.Suffix), RightValue: auditComparisonValue(right.Suffix)},
		{Key: "birth_year", Label: "Birth Year", LeftValue: auditComparisonIntValue(birthYearLeft), RightValue: auditComparisonIntValue(birthYearRight)},
		{Key: "birth_date", Label: "Birth Date", LeftValue: auditComparisonValue(left.BirthDate), RightValue: auditComparisonValue(right.BirthDate)},
		{Key: "unit", Label: "Unit", LeftValue: auditComparisonValue(left.Unit), RightValue: auditComparisonValue(right.Unit)},
		{Key: "maiden_name", Label: "Maiden Name", LeftValue: auditComparisonValue(left.MaidenName), RightValue: auditComparisonValue(right.MaidenName)},
		{Key: "buried_in", Label: "Buried In", LeftValue: auditComparisonValue(left.BuriedIn), RightValue: auditComparisonValue(right.BuriedIn)},
		{Key: "entry_type", Label: "Person Record Type", LeftValue: auditComparisonValue(left.EntryType), RightValue: auditComparisonValue(right.EntryType)},
		{Key: "added_by", Label: "Created By", LeftValue: auditComparisonValue(left.AddedBy), RightValue: auditComparisonValue(right.AddedBy)},
	}
	for index := range fields {
		_, fields[index].Highlighted = highlights[fields[index].Key]
	}
	return fields
}

func auditComparisonValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "Not recorded"
	}
	return strings.TrimSpace(value)
}

func auditComparisonIntValue(value int) string {
	if value < 1000 {
		return "Not recorded"
	}
	return strconv.Itoa(value)
}

func (s *AuditService) lookupCandidateMap(ids map[int64]struct{}) (map[int64]models.Soldier, error) {
	if len(ids) == 0 {
		return map[int64]models.Soldier{}, nil
	}
	ordered := make([]int64, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Slice(ordered, func(left, right int) bool {
		return ordered[left] < ordered[right]
	})
	placeholders := make([]string, len(ordered))
	args := make([]any, 0, len(ordered))
	for index, id := range ordered {
		placeholders[index] = "?"
		args = append(args, id)
	}
	rows, err := s.db.Conn().Query(`SELECT `+soldierSelectColumns+` FROM soldiers WHERE id IN (`+strings.Join(placeholders, ",")+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	soldiers, err := scanSoldiers(rows)
	if err != nil {
		return nil, err
	}
	results := make(map[int64]models.Soldier, len(soldiers))
	for _, soldier := range soldiers {
		results[soldier.ID] = soldier
	}
	return results, nil
}

func (s *AuditService) syncSoldierDuplicateReviewStateTx(tx *sql.Tx, soldierID int64) error {
	var (
		currentNeedsReview bool
		currentReason      string
	)
	err := tx.QueryRow(`SELECT needs_review, COALESCE(review_reason, '') FROM soldiers WHERE id = ?`, soldierID).Scan(&currentNeedsReview, &currentReason)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}

	var openReason string
	openCount := 0
	rows, err := tx.Query(`
		SELECT reason
		FROM duplicate_audit_findings
		WHERE status = 'open' AND (left_soldier_id = ? OR right_soldier_id = ?)
		ORDER BY id ASC`, soldierID, soldierID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var reason string
		if err := rows.Scan(&reason); err != nil {
			rows.Close()
			return err
		}
		openCount++
		if openReason == "" {
			openReason = strings.TrimSpace(reason)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	desiredNeedsReview := currentNeedsReview
	desiredReason := currentReason
	trimmedReason := strings.TrimSpace(currentReason)
	switch {
	case openCount > 0:
		if strings.HasPrefix(trimmedReason, duplicateAuditReasonPrefix) || trimmedReason == "" {
			desiredReason = openReason
		}
		desiredNeedsReview = true
	case strings.HasPrefix(trimmedReason, duplicateAuditReasonPrefix):
		desiredNeedsReview = false
		desiredReason = ""
	default:
		return nil
	}
	if currentNeedsReview == desiredNeedsReview && strings.TrimSpace(currentReason) == strings.TrimSpace(desiredReason) {
		return nil
	}

	actor := NewSoldierService(s.db).currentAuditActor()
	updatedAt := currentSQLiteTimestamp()
	fieldKey := "review_status"
	if desiredNeedsReview {
		fieldKey = "needs_review"
	}
	_, err = tx.Exec(`UPDATE soldiers SET needs_review = ?, review_reason = ?, last_edited_by = ?, last_edited_fields = ?, last_edited_at = ?, updated_at = ? WHERE id = ?`,
		desiredNeedsReview,
		desiredReason,
		actor,
		strings.Join(auditTouchDescriptions([]string{fieldKey}), "\n"),
		updatedAt,
		updatedAt,
		soldierID,
	)
	return err
}
