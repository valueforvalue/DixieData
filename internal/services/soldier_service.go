package services

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/valueforvalue/DixieData/internal/dates"
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
)

const (
	soldierSelectColumns     = `id, display_id, sync_id, entry_type, spouse_soldier_id, maiden_name, is_generated, pension_id, application_id, prefix, first_name, middle_name, last_name, suffix, rank, rank_in, rank_out, unit, pension_state, confederate_home_status, confederate_home_name, death_year, death_month, death_day, birth_date, death_date, birth_info, buried_in, notes, needs_review, review_reason, added_by, last_edited_by, last_edited_fields, last_edited_at, created_at, updated_at`
	soldierListSelectColumns = soldierSelectColumns + `, COALESCE((SELECT display_id FROM soldiers linked WHERE linked.id = soldiers.spouse_soldier_id), ''), (SELECT COUNT(*) FROM records WHERE records.soldier_id = soldiers.id), (SELECT COUNT(*) FROM images WHERE images.soldier_id = soldiers.id)`
	recordSelectColumns      = `id, sync_id, soldier_id, soldier_sync_id, record_type, app_id, details`
	imageSelectColumns       = `id, sync_id, soldier_id, soldier_sync_id, file_name, file_path, caption, is_primary`
)

type SoldierService struct {
	db                *db.DB
	formSuggestionsMu sync.RWMutex
	formSuggestions   *models.SoldierFormSuggestions
}

func NewSoldierService(database *db.DB) *SoldierService {
	return &SoldierService{db: database}
}

func (s *SoldierService) Create(soldier models.Soldier) (*models.Soldier, error) {
	conn := s.db.Conn()
	tx, err := conn.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	generatedDisplayID := strings.TrimSpace(soldier.DisplayID) == ""
	if soldier.DisplayID == "" {
		id, err := s.db.NextDXDID()
		if err != nil {
			return nil, err
		}
		soldier.DisplayID = id
	}
	nodePrefix, err := s.db.NodePrefix()
	if err != nil {
		return nil, err
	}
	soldier.DisplayID = normalizeDisplayID(soldier.DisplayID, nodePrefix)
	if strings.TrimSpace(soldier.SyncID) == "" {
		soldier.SyncID, err = db.NewSyncID()
		if err != nil {
			return nil, err
		}
	}
	if err := normalizeSoldierEntry(tx, &soldier); err != nil {
		return nil, err
	}
	normalizeConfederateHomeFields(&soldier)
	if err := normalizeSoldierDates(&soldier); err != nil {
		return nil, err
	}
	soldier.IsGenerated = generatedDisplayID || isGeneratedDisplayID(soldier.DisplayID)
	soldier.Rank = canonicalRank(soldier)
	if strings.TrimSpace(soldier.CreatedAt) == "" {
		soldier.CreatedAt = currentSQLiteTimestamp()
	}
	if strings.TrimSpace(soldier.UpdatedAt) == "" {
		soldier.UpdatedAt = soldier.CreatedAt
	}
	stampCreateAuditFields(s.currentAuditActor(), &soldier)

	res, err := tx.Exec(`INSERT INTO soldiers (display_id, sync_id, entry_type, spouse_soldier_id, maiden_name, is_generated, pension_id, application_id, prefix, first_name, middle_name, last_name, suffix, rank, rank_in, rank_out, unit, pension_state, confederate_home_status, confederate_home_name, death_year, death_month, death_day, birth_date, death_date, birth_info, buried_in, notes, needs_review, review_reason, added_by, last_edited_by, last_edited_fields, last_edited_at, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		soldier.DisplayID, soldier.SyncID, soldier.EntryType, nullableInt64(soldier.SpouseSoldierID), soldier.MaidenName, soldier.IsGenerated, soldier.PensionID, soldier.ApplicationID, soldier.Prefix, soldier.FirstName, soldier.MiddleName, soldier.LastName, soldier.Suffix,
		soldier.Rank, soldier.RankIn, soldier.RankOut, soldier.Unit, soldier.PensionState, soldier.ConfederateHomeStatus, soldier.ConfederateHomeName, soldier.DeathYear, soldier.DeathMonth,
		soldier.DeathDay, soldier.BirthDate, soldier.DeathDate, soldier.BirthInfo, soldier.BuriedIn, soldier.Notes, soldier.NeedsReview, soldier.ReviewReason, soldier.AddedBy, soldier.LastEditedBy, soldier.LastEditedFields, soldier.LastEditedAt, soldier.CreatedAt, soldier.UpdatedAt)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	soldier.ID = id

	if err := replaceRecords(tx, soldier.ID, soldier.SyncID, soldier.Records); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	s.invalidateFormSuggestions()
	return &soldier, nil
}

func isGeneratedDisplayID(displayID string) bool {
	_, _, ok := db.CanonicalDisplayID(db.SanitizeID(displayID, ""))
	return ok
}

func isFiveDigitGeneratedSuffix(value string) bool {
	if len(value) != 5 {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (s *SoldierService) GetByID(id int64) (*models.Soldier, error) {
	conn := s.db.Conn()
	row := conn.QueryRow(`SELECT `+soldierSelectColumns+` FROM soldiers WHERE id = ?`, id)
	soldier, err := scanSoldier(row)
	if err != nil {
		return nil, err
	}

	rows, err := conn.Query(`SELECT `+recordSelectColumns+` FROM records WHERE soldier_id = ? ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var r models.Record
		if err := rows.Scan(&r.ID, &r.SyncID, &r.SoldierID, &r.SoldierSyncID, &r.RecordType, &r.AppID, &r.Details); err != nil {
			return nil, err
		}
		soldier.Records = append(soldier.Records, r)
	}

	imgRows, err := conn.Query(`SELECT `+imageSelectColumns+` FROM images WHERE soldier_id = ? ORDER BY is_primary DESC, id`, id)
	if err != nil {
		return nil, err
	}
	defer imgRows.Close()
	for imgRows.Next() {
		var img models.Image
		if err := imgRows.Scan(&img.ID, &img.SyncID, &img.SoldierID, &img.SoldierSyncID, &img.FileName, &img.FilePath, &img.Caption, &img.IsPrimary); err != nil {
			return nil, err
		}
		soldier.Images = append(soldier.Images, img)
	}
	soldier.SpouseName = spouseReference(conn, soldier.SpouseSoldierID)
	soldier.SpouseDisplayID = spouseDisplayID(conn, soldier.SpouseSoldierID)

	return soldier, nil
}

func (s *SoldierService) Update(soldier models.Soldier) error {
	conn := s.db.Conn()
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	before, err := loadSoldierAuditSnapshot(tx, soldier.ID)
	if err != nil {
		return err
	}

	soldier.Rank = canonicalRank(soldier)
	nodePrefix, err := s.db.NodePrefix()
	if err != nil {
		return err
	}
	soldier.DisplayID = normalizeDisplayID(soldier.DisplayID, nodePrefix)
	if err := hydrateSoldierIdentity(tx, &soldier); err != nil {
		return err
	}
	if err := normalizeSoldierEntry(tx, &soldier); err != nil {
		return err
	}
	normalizeConfederateHomeFields(&soldier)
	if err := normalizeSoldierDates(&soldier); err != nil {
		return err
	}
	if strings.TrimSpace(soldier.UpdatedAt) == "" {
		soldier.UpdatedAt = currentSQLiteTimestamp()
	}
	stampUpdateAuditFields(s.currentAuditActor(), before, &soldier)

	_, err = tx.Exec(`UPDATE soldiers SET display_id=?, sync_id=?, entry_type=?, spouse_soldier_id=?, maiden_name=?, pension_id=?, application_id=?, prefix=?, first_name=?, middle_name=?, last_name=?, suffix=?, rank=?, rank_in=?, rank_out=?, unit=?, pension_state=?, confederate_home_status=?, confederate_home_name=?, death_year=?, death_month=?, death_day=?, birth_date=?, death_date=?, birth_info=?, buried_in=?, notes=?, needs_review=?, review_reason=?, added_by=?, last_edited_by=?, last_edited_fields=?, last_edited_at=?, updated_at=? WHERE id=?`,
		soldier.DisplayID, soldier.SyncID, soldier.EntryType, nullableInt64(soldier.SpouseSoldierID), soldier.MaidenName, soldier.PensionID, soldier.ApplicationID, soldier.Prefix, soldier.FirstName, soldier.MiddleName, soldier.LastName, soldier.Suffix, soldier.Rank, soldier.RankIn, soldier.RankOut, soldier.Unit, soldier.PensionState, soldier.ConfederateHomeStatus, soldier.ConfederateHomeName,
		soldier.DeathYear, soldier.DeathMonth, soldier.DeathDay, soldier.BirthDate, soldier.DeathDate, soldier.BirthInfo, soldier.BuriedIn, soldier.Notes, soldier.NeedsReview, soldier.ReviewReason, soldier.AddedBy, soldier.LastEditedBy, soldier.LastEditedFields, soldier.LastEditedAt, soldier.UpdatedAt, soldier.ID)
	if err != nil {
		return err
	}

	if err := replaceRecords(tx, soldier.ID, soldier.SyncID, soldier.Records); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	s.invalidateFormSuggestions()
	return nil
}

func (s *SoldierService) Delete(id int64) error {
	if _, err := s.db.Conn().Exec(`DELETE FROM soldiers WHERE id = ?`, id); err != nil {
		return err
	}
	s.invalidateFormSuggestions()
	return nil
}

func (s *SoldierService) AddImage(soldierID int64, fileName, filePath, caption string) error {
	if caption == "" {
		caption = fileName
	}
	soldierSyncID, err := s.soldierSyncIDByID(soldierID)
	if err != nil {
		return err
	}
	imageSyncID, err := db.NewSyncID()
	if err != nil {
		return err
	}
	isPrimary, err := s.shouldAssignPrimaryImage(soldierID)
	if err != nil {
		return err
	}
	_, err = s.db.Conn().Exec(
		`INSERT INTO images (sync_id, soldier_id, soldier_sync_id, file_name, file_path, caption, is_primary) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		imageSyncID,
		soldierID,
		soldierSyncID,
		fileName,
		filePath,
		caption,
		isPrimary,
	)
	if err != nil {
		return err
	}
	return s.touchAuditFields(soldierID, "images")
}

func (s *SoldierService) DeleteImages(soldierID int64, imageIDs []int64) error {
	if len(imageIDs) == 0 {
		return nil
	}

	placeholders := make([]string, len(imageIDs))
	args := make([]interface{}, 0, len(imageIDs)+1)
	args = append(args, soldierID)
	for index, imageID := range imageIDs {
		placeholders[index] = "?"
		args = append(args, imageID)
	}

	_, err := s.db.Conn().Exec(
		fmt.Sprintf(`DELETE FROM images WHERE soldier_id = ? AND id IN (%s)`, strings.Join(placeholders, ",")),
		args...,
	)
	if err != nil {
		return err
	}
	if err := s.ensurePrimaryImage(soldierID); err != nil {
		return err
	}
	return s.touchAuditFields(soldierID, "images")
}

func (s *SoldierService) SetPrimaryImage(soldierID, imageID int64) error {
	var count int
	if err := s.db.Conn().QueryRow(`SELECT COUNT(1) FROM images WHERE soldier_id = ? AND id = ?`, soldierID, imageID).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		return sql.ErrNoRows
	}
	if _, err := s.db.Conn().Exec(`UPDATE images SET is_primary = CASE WHEN id = ? THEN 1 ELSE 0 END WHERE soldier_id = ?`, imageID, soldierID); err != nil {
		return err
	}
	return s.touchAuditFields(soldierID, "primary_image")
}

func (s *SoldierService) GetImageByID(imageID int64) (*models.Image, error) {
	row := s.db.Conn().QueryRow(`SELECT `+imageSelectColumns+` FROM images WHERE id = ?`, imageID)
	var image models.Image
	if err := row.Scan(&image.ID, &image.SyncID, &image.SoldierID, &image.SoldierSyncID, &image.FileName, &image.FilePath, &image.Caption, &image.IsPrimary); err != nil {
		return nil, err
	}
	return &image, nil
}

func (s *SoldierService) ArchiveCounts() (models.ArchiveCounts, error) {
	row := s.db.Conn().QueryRow(`
		SELECT
			COALESCE(SUM(CASE
				WHEN entry_type IS NULL OR TRIM(entry_type) = '' OR LOWER(TRIM(entry_type)) = 'soldier' THEN 1
				ELSE 0
			END), 0),
			COALESCE(SUM(CASE
				WHEN LOWER(TRIM(entry_type)) IN ('wife', 'widow') THEN 1
				ELSE 0
			END), 0)
		FROM soldiers`)
	var counts models.ArchiveCounts
	if err := row.Scan(&counts.TotalSoldiers, &counts.TotalWivesWidows); err != nil {
		return models.ArchiveCounts{}, err
	}
	return counts, nil
}

func (s *SoldierService) ReviewQueue(page, pageSize int) ([]models.Soldier, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	conn := s.db.Conn()
	var total int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM soldiers WHERE needs_review = 1`).Scan(&total); err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	rows, err := conn.Query(`SELECT `+soldierSelectColumns+` FROM soldiers WHERE needs_review = 1 ORDER BY updated_at DESC, last_name, first_name LIMIT ? OFFSET ?`, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	soldiers, err := scanSoldiers(rows)
	return soldiers, total, err
}

func (s *SoldierService) MarkReviewResolved(soldierID int64) error {
	if _, err := s.db.Conn().Exec(`UPDATE soldiers SET needs_review = 0, review_reason = '' WHERE id = ?`, soldierID); err != nil {
		return err
	}
	return s.touchAuditFields(soldierID, "review_status")
}

func (s *SoldierService) SetReviewStatus(soldierID int64, needsReview bool, reason string) error {
	reason = strings.TrimSpace(reason)
	if !needsReview {
		reason = ""
	}
	if _, err := s.db.Conn().Exec(`UPDATE soldiers SET needs_review = ?, review_reason = ? WHERE id = ?`, needsReview, reason, soldierID); err != nil {
		return err
	}
	if needsReview {
		return s.touchAuditFields(soldierID, "needs_review")
	}
	return s.touchAuditFields(soldierID, "review_status")
}

func (s *SoldierService) SearchPage(query string, page, pageSize int) ([]models.Soldier, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}

	query = strings.TrimSpace(query)
	if query == "" {
		return s.List(page, pageSize)
	}

	offset := (page - 1) * pageSize
	soldiers, total, err := s.searchWithFTS(query, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	if total > 0 {
		return soldiers, total, nil
	}
	return s.searchWithLike(query, pageSize, offset)
}

func (s *SoldierService) searchWithFTS(query string, pageSize, offset int) ([]models.Soldier, int, error) {
	if err := s.db.SyncScratchpadSearchIndex(); err != nil {
		return nil, 0, err
	}
	conn := s.db.Conn()
	matchQuery := ftsSearchExpression(query)
	if matchQuery == "" {
		return []models.Soldier{}, 0, nil
	}
	recordArgs := recordSearchLikeArgs(query)

	var total int
	err := conn.QueryRow(`
		SELECT COUNT(*) FROM (
			SELECT soldier_id AS id FROM soldiers_fts WHERE soldiers_fts MATCH ?
			UNION
			SELECT soldier_id AS id FROM records WHERE `+recordSearchLikeClause()+`
		) matches
	`, append([]interface{}{matchQuery}, recordArgs...)...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rowArgs := append(append([]interface{}{matchQuery}, recordArgs...), pageSize, offset)
	rows, err := conn.Query(`
		WITH matches AS (
			SELECT soldier_id,
				COALESCE(snippet(soldiers_fts, 18, '', '', '...', 12), '') AS notes_snippet,
				COALESCE(snippet(soldiers_fts, 19, '', '', '...', 12), '') AS scratch_snippet,
				bm25(soldiers_fts) AS score
			FROM soldiers_fts
			WHERE soldiers_fts MATCH ?
			UNION
			SELECT soldier_id, '', '', 1000.0
			FROM records
			WHERE `+recordSearchLikeClause()+`
		)
		SELECT `+soldierListSelectColumns+`, COALESCE(MAX(notes_snippet), ''), COALESCE(MAX(scratch_snippet), '')
		FROM soldiers
		JOIN matches ON matches.soldier_id = soldiers.id
		GROUP BY soldiers.id
		ORDER BY MIN(score), last_name, first_name
		LIMIT ? OFFSET ?
	`, rowArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	soldiers := []models.Soldier{}
	for rows.Next() {
		var (
			soldier        models.Soldier
			notesSnippet   string
			scratchSnippet string
		)
		if err := rows.Scan(append(soldierListScanDest(&soldier), &notesSnippet, &scratchSnippet)...); err != nil {
			return nil, 0, err
		}
		hydrateLegacyDeathParts(&soldier)
		normalizeConfederateHomeFields(&soldier)
		if snippetContainsQuery(notesSnippet, query) {
			soldier.SearchMatchField = "Notes"
			soldier.SearchMatchSnippet = strings.TrimSpace(notesSnippet)
		} else if snippetContainsQuery(scratchSnippet, query) {
			soldier.SearchMatchField = "Scratch Pad"
			soldier.SearchMatchSnippet = strings.TrimSpace(scratchSnippet)
		} else {
			soldier.SearchMatchField, soldier.SearchMatchSnippet = quickSearchMatch(soldier, quickSearchTerms(query))
		}
		soldiers = append(soldiers, soldier)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return soldiers, total, nil
}

func quickSearchLikeClause() string {
	return `display_id LIKE ? OR pension_id LIKE ? OR application_id LIKE ? OR prefix LIKE ? OR first_name LIKE ? OR middle_name LIKE ? OR last_name LIKE ? OR suffix LIKE ? OR unit LIKE ? OR rank LIKE ? OR rank_in LIKE ? OR rank_out LIKE ? OR pension_state LIKE ? OR confederate_home_status LIKE ? OR confederate_home_name LIKE ? OR buried_in LIKE ? OR maiden_name LIKE ? OR notes LIKE ? OR EXISTS (
		SELECT 1 FROM records
		WHERE records.soldier_id = soldiers.id
			AND (record_type LIKE ? OR app_id LIKE ? OR details LIKE ?)
	)`
}

func quickSearchLikeArgs(query string) []interface{} {
	like := "%" + query + "%"
	return []interface{}{
		like, like, like,
		like, like, like, like, like,
		like, like, like, like,
		like, like, like,
		like, like, like,
		like, like, like,
	}
}

func (s *SoldierService) searchWithLike(query string, pageSize, offset int) ([]models.Soldier, int, error) {
	conn := s.db.Conn()
	args := quickSearchLikeArgs(query)

	var total int
	err := conn.QueryRow(`
		SELECT COUNT(*)
		FROM soldiers
		WHERE `+quickSearchLikeClause()+`
	`, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := conn.Query(`
		SELECT `+soldierListSelectColumns+`
		FROM soldiers
		WHERE `+quickSearchLikeClause()+`
		ORDER BY last_name, first_name
		LIMIT ? OFFSET ?
	`, append(args, pageSize, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	soldiers, err := scanListSoldiers(rows)
	return annotateQuickSearchMatches(soldiers, query), total, err
}

func recordSearchLikeClause() string {
	return `record_type LIKE ? OR app_id LIKE ? OR details LIKE ?`
}

func recordSearchLikeArgs(query string) []interface{} {
	like := "%" + query + "%"
	return []interface{}{like, like, like}
}

func ftsSearchExpression(query string) string {
	terms := quickSearchTerms(query)
	if len(terms) == 0 {
		return ""
	}
	parts := make([]string, 0, len(terms))
	for _, term := range terms {
		trimmed := strings.TrimSpace(term)
		if trimmed == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf(`"%s"*`, strings.ReplaceAll(trimmed, `"`, `""`)))
	}
	return strings.Join(parts, " AND ")
}

func snippetContainsQuery(snippet, query string) bool {
	if strings.TrimSpace(snippet) == "" {
		return false
	}
	lowerSnippet := strings.ToLower(snippet)
	for _, term := range quickSearchTerms(query) {
		if term != "" && strings.Contains(lowerSnippet, term) {
			return true
		}
	}
	return false
}

func (s *SoldierService) AdvancedSearch(search models.SoldierSearch, page, pageSize int) ([]models.Soldier, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}

	search.DisplayID = strings.TrimSpace(search.DisplayID)
	search.EntryType = strings.TrimSpace(strings.ToLower(search.EntryType))
	search.FirstName = strings.TrimSpace(search.FirstName)
	search.MiddleName = strings.TrimSpace(search.MiddleName)
	search.LastName = strings.TrimSpace(search.LastName)
	search.MaidenName = strings.TrimSpace(search.MaidenName)
	search.Rank = strings.TrimSpace(search.Rank)
	search.RankIn = strings.TrimSpace(search.RankIn)
	search.RankOut = strings.TrimSpace(search.RankOut)
	search.Unit = strings.TrimSpace(search.Unit)
	search.RecordType = strings.TrimSpace(search.RecordType)
	search.PensionState = strings.TrimSpace(search.PensionState)
	search.ConfederateHomeStatus = strings.TrimSpace(search.ConfederateHomeStatus)
	search.ConfederateHomeName = strings.TrimSpace(search.ConfederateHomeName)
	search.BuriedIn = strings.TrimSpace(search.BuriedIn)
	search.ReviewStatus = strings.TrimSpace(search.ReviewStatus)
	search.BirthDate = strings.TrimSpace(search.BirthDate)
	search.BirthYear = strings.TrimSpace(search.BirthYear)
	search.BirthYearTo = strings.TrimSpace(search.BirthYearTo)
	search.DeathDate = strings.TrimSpace(search.DeathDate)
	search.DeathYear = strings.TrimSpace(search.DeathYear)
	search.DeathYearTo = strings.TrimSpace(search.DeathYearTo)
	search.DeathMonth = strings.TrimSpace(search.DeathMonth)
	search.DeathDay = strings.TrimSpace(search.DeathDay)

	whereParts := []string{}
	args := []interface{}{}
	appendContainsFilter := func(column, value string) {
		whereParts = append(whereParts, column+" LIKE ?")
		args = append(args, "%"+value+"%")
	}
	appendYearFilter := func(expression, startValue, endValue, field string) error {
		if startValue == "" && endValue == "" {
			return nil
		}
		if startValue != "" && endValue == "" {
			parsed, err := strconv.Atoi(startValue)
			if err != nil {
				return fmt.Errorf("invalid %s", field)
			}
			whereParts = append(whereParts, expression+" = ?")
			args = append(args, parsed)
			return nil
		}

		var (
			start int
			end   int
			err   error
		)
		if startValue != "" {
			start, err = strconv.Atoi(startValue)
			if err != nil {
				return fmt.Errorf("invalid %s", field)
			}
		}
		if endValue != "" {
			end, err = strconv.Atoi(endValue)
			if err != nil {
				return fmt.Errorf("invalid %s_to", field)
			}
		}
		switch {
		case startValue == "":
			whereParts = append(whereParts, expression+" <= ?")
			args = append(args, end)
		case endValue == "":
			whereParts = append(whereParts, expression+" >= ?")
			args = append(args, start)
		default:
			if end < start {
				start, end = end, start
			}
			whereParts = append(whereParts, expression+" BETWEEN ? AND ?")
			args = append(args, start, end)
		}
		return nil
	}

	if search.DisplayID != "" {
		appendContainsFilter("display_id", search.DisplayID)
	}
	switch search.EntryType {
	case "soldier":
		whereParts = append(whereParts, "(entry_type IS NULL OR TRIM(entry_type) = '' OR LOWER(TRIM(entry_type)) = 'soldier')")
	case "wife", "widow":
		whereParts = append(whereParts, "LOWER(TRIM(entry_type)) = ?")
		args = append(args, search.EntryType)
	}
	if search.FirstName != "" {
		appendContainsFilter("first_name", search.FirstName)
	}
	if search.MiddleName != "" {
		appendContainsFilter("middle_name", search.MiddleName)
	}
	if search.LastName != "" {
		appendContainsFilter("last_name", search.LastName)
	}
	if search.MaidenName != "" {
		appendContainsFilter("maiden_name", search.MaidenName)
	}
	if search.Rank != "" {
		whereParts = append(whereParts, "(rank LIKE ? OR rank_in LIKE ? OR rank_out LIKE ?)")
		args = append(args, "%"+search.Rank+"%", "%"+search.Rank+"%", "%"+search.Rank+"%")
	}
	if search.RankIn != "" {
		appendContainsFilter("rank_in", search.RankIn)
	}
	if search.RankOut != "" {
		appendContainsFilter("rank_out", search.RankOut)
	}
	if search.Unit != "" {
		appendContainsFilter("unit", search.Unit)
	}
	if search.RecordType != "" {
		whereParts = append(whereParts, "EXISTS (SELECT 1 FROM records WHERE records.soldier_id = soldiers.id AND records.record_type LIKE ?)")
		args = append(args, "%"+search.RecordType+"%")
	}
	if search.PensionState != "" {
		whereParts = append(whereParts, "pension_state = ?")
		args = append(args, search.PensionState)
	}
	if search.ConfederateHomeStatus != "" {
		whereParts = append(whereParts, "confederate_home_status = ?")
		args = append(args, search.ConfederateHomeStatus)
	}
	if search.ConfederateHomeName != "" {
		appendContainsFilter("confederate_home_name", search.ConfederateHomeName)
	}
	if search.BuriedIn != "" {
		appendContainsFilter("buried_in", search.BuriedIn)
	}
	switch strings.ToLower(search.ReviewStatus) {
	case "clean":
		whereParts = append(whereParts, "needs_review = 0")
	case "review":
		whereParts = append(whereParts, "needs_review = 1")
	}
	if search.BirthDate != "" {
		normalized, err := dates.NormalizeCanonical(search.BirthDate)
		if err != nil {
			return nil, 0, fmt.Errorf("invalid birth_date")
		}
		whereParts = append(whereParts, "birth_date = ?")
		args = append(args, normalized)
	}
	if err := appendYearFilter(`NULLIF(CAST(substr(trim(coalesce(birth_date, '')), -4) AS INTEGER), 0)`, search.BirthYear, search.BirthYearTo, "birth_year"); err != nil {
		return nil, 0, err
	}
	if search.DeathDate != "" {
		normalized, err := dates.NormalizeCanonical(search.DeathDate)
		if err != nil {
			return nil, 0, fmt.Errorf("invalid death_date")
		}
		whereParts = append(whereParts, "death_date = ?")
		args = append(args, normalized)
	}
	if err := appendYearFilter("NULLIF(death_year, 0)", search.DeathYear, search.DeathYearTo, "death_year"); err != nil {
		return nil, 0, err
	}

	exactFilters := []struct {
		value  string
		field  string
		column string
	}{
		{search.DeathMonth, "death_month", "death_month"},
		{search.DeathDay, "death_day", "death_day"},
	}
	for _, filter := range exactFilters {
		if filter.value == "" {
			continue
		}
		parsed, err := strconv.Atoi(filter.value)
		if err != nil {
			return nil, 0, fmt.Errorf("invalid %s", filter.field)
		}
		whereParts = append(whereParts, filter.column+" = ?")
		args = append(args, parsed)
	}

	if len(whereParts) == 0 {
		return s.List(page, pageSize)
	}

	whereClause := strings.Join(whereParts, " AND ")
	conn := s.db.Conn()

	var total int
	if err := conn.QueryRow("SELECT COUNT(*) FROM soldiers WHERE "+whereClause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	rows, err := conn.Query(
		"SELECT "+soldierListSelectColumns+" FROM soldiers WHERE "+whereClause+" ORDER BY last_name, first_name LIMIT ? OFFSET ?",
		append(args, pageSize, offset)...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	soldiers, err := scanListSoldiers(rows)
	return soldiers, total, err
}

func (s *SoldierService) List(page, pageSize int) ([]models.Soldier, int, error) {
	conn := s.db.Conn()
	var total int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM soldiers`).Scan(&total); err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	rows, err := conn.Query(`SELECT `+soldierListSelectColumns+` FROM soldiers ORDER BY last_name, first_name LIMIT ? OFFSET ?`, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	soldiers, err := scanListSoldiers(rows)
	return soldiers, total, err
}

func (s *SoldierService) ListByEntryTypes(entryTypes []string, page, pageSize int) ([]models.Soldier, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	normalized := make([]string, 0, len(entryTypes))
	for _, entryType := range entryTypes {
		trimmed := strings.TrimSpace(strings.ToLower(entryType))
		if trimmed != "" {
			normalized = append(normalized, trimmed)
		}
	}
	if len(normalized) == 0 {
		return []models.Soldier{}, 0, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(normalized)), ",")
	args := make([]interface{}, 0, len(normalized))
	for _, entryType := range normalized {
		args = append(args, entryType)
	}
	whereClause := fmt.Sprintf("LOWER(TRIM(entry_type)) IN (%s)", placeholders)
	conn := s.db.Conn()
	var total int
	if err := conn.QueryRow("SELECT COUNT(*) FROM soldiers WHERE "+whereClause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	rows, err := conn.Query(
		"SELECT "+soldierListSelectColumns+" FROM soldiers WHERE "+whereClause+" ORDER BY last_name, first_name LIMIT ? OFFSET ?",
		append(args, pageSize, offset)...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	soldiers, err := scanListSoldiers(rows)
	return soldiers, total, err
}

func (s *SoldierService) RecentByIDs(ids []int64, limit int) ([]models.Soldier, error) {
	if limit < 1 {
		limit = 10
	}
	if len(ids) == 0 {
		return []models.Soldier{}, nil
	}
	if len(ids) > limit {
		ids = ids[:limit]
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.db.Conn().Query(
		"SELECT "+soldierListSelectColumns+" FROM soldiers WHERE id IN ("+placeholders+")",
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	soldiers, err := scanListSoldiers(rows)
	if err != nil {
		return nil, err
	}
	index := make(map[int64]models.Soldier, len(soldiers))
	for _, soldier := range soldiers {
		index[soldier.ID] = soldier
	}
	ordered := make([]models.Soldier, 0, len(ids))
	for _, id := range ids {
		if soldier, ok := index[id]; ok {
			ordered = append(ordered, soldier)
		}
	}
	return ordered, nil
}

func scanSoldier(row *sql.Row) (*models.Soldier, error) {
	var s models.Soldier
	err := row.Scan(soldierScanDest(&s)...)
	if err != nil {
		return nil, err
	}
	hydrateLegacyDeathParts(&s)
	normalizeConfederateHomeFields(&s)
	return &s, nil
}

func scanSoldiers(rows *sql.Rows) ([]models.Soldier, error) {
	var soldiers []models.Soldier
	for rows.Next() {
		var s models.Soldier
		if err := rows.Scan(soldierScanDest(&s)...); err != nil {
			return nil, err
		}
		hydrateLegacyDeathParts(&s)
		normalizeConfederateHomeFields(&s)
		soldiers = append(soldiers, s)
	}
	if soldiers == nil {
		soldiers = []models.Soldier{}
	}
	return soldiers, rows.Err()
}

func scanListSoldiers(rows *sql.Rows) ([]models.Soldier, error) {
	var soldiers []models.Soldier
	for rows.Next() {
		var s models.Soldier
		if err := rows.Scan(soldierListScanDest(&s)...); err != nil {
			return nil, err
		}
		hydrateLegacyDeathParts(&s)
		normalizeConfederateHomeFields(&s)
		soldiers = append(soldiers, s)
	}
	if soldiers == nil {
		soldiers = []models.Soldier{}
	}
	return soldiers, rows.Err()
}

func annotateQuickSearchMatches(soldiers []models.Soldier, query string) []models.Soldier {
	terms := quickSearchTerms(query)
	if len(terms) == 0 {
		return soldiers
	}
	for index := range soldiers {
		soldiers[index].SearchMatchField, soldiers[index].SearchMatchSnippet = quickSearchMatch(soldiers[index], terms)
	}
	return soldiers
}

func quickSearchTerms(query string) []string {
	return strings.FieldsFunc(strings.ToLower(strings.TrimSpace(query)), func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
}

func quickSearchMatch(soldier models.Soldier, terms []string) (string, string) {
	for _, candidate := range []struct {
		label string
		value string
	}{
		{label: "Record ID", value: strings.TrimSpace(soldier.DisplayID)},
		{label: "Pension ID", value: strings.TrimSpace(soldier.PensionID)},
		{label: "Application ID", value: strings.TrimSpace(soldier.ApplicationID)},
		{label: "Name", value: strings.TrimSpace(soldier.GetFullName())},
		{label: "Rank", value: soldierSearchRank(soldier)},
		{label: "Unit", value: strings.TrimSpace(soldier.Unit)},
		{label: "Pension State", value: strings.TrimSpace(soldier.PensionState)},
		{label: "Buried In", value: strings.TrimSpace(soldier.BuriedIn)},
		{label: "Maiden Name", value: strings.TrimSpace(soldier.MaidenName)},
	} {
		if candidate.value == "" {
			continue
		}
		lowerValue := strings.ToLower(candidate.value)
		for _, term := range terms {
			if term != "" && strings.Contains(lowerValue, term) {
				return candidate.label, candidate.value
			}
		}
	}
	return "", ""
}

func soldierSearchRank(soldier models.Soldier) string {
	for _, value := range []string{soldier.RankOut, soldier.Rank, soldier.RankIn} {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func (s *SoldierService) ManualComparison(leftID, rightID int64) (*DuplicateAuditComparison, error) {
	if leftID < 1 || rightID < 1 || leftID == rightID {
		return nil, fmt.Errorf("choose two different records to compare")
	}
	leftSoldier, err := s.GetByID(leftID)
	if err != nil {
		return nil, err
	}
	rightSoldier, err := s.GetByID(rightID)
	if err != nil {
		return nil, err
	}
	fields := buildDuplicateAuditComparisonFields(*leftSoldier, *rightSoldier, map[string]struct{}{})
	for index := range fields {
		fields[index].Highlighted = fields[index].LeftValue != fields[index].RightValue
	}
	return &DuplicateAuditComparison{
		PageTitle:    "Record Comparison",
		BackHref:     "/soldiers",
		BackLabel:    "Back",
		Reason:       "Manual side-by-side comparison of two selected records.",
		Status:       "manual",
		LeftSoldier:  *leftSoldier,
		RightSoldier: *rightSoldier,
		Fields:       fields,
	}, nil
}

func replaceRecords(tx *sql.Tx, soldierID int64, soldierSyncID string, records []models.Record) error {
	if _, err := tx.Exec(`DELETE FROM records WHERE soldier_id = ?`, soldierID); err != nil {
		return err
	}
	for _, record := range normalizeRecords(records) {
		if strings.TrimSpace(record.SyncID) == "" {
			syncID, err := db.NewSyncID()
			if err != nil {
				return err
			}
			record.SyncID = syncID
		}
		record.SoldierSyncID = soldierSyncID
		if _, err := tx.Exec(
			`INSERT INTO records (sync_id, soldier_id, soldier_sync_id, record_type, app_id, details) VALUES (?, ?, ?, ?, ?, ?)`,
			record.SyncID,
			soldierID,
			record.SoldierSyncID,
			record.RecordType,
			record.AppID,
			record.Details,
		); err != nil {
			return err
		}
	}
	return nil
}

func normalizeRecords(records []models.Record) []models.Record {
	normalized := make([]models.Record, 0, len(records))
	for _, record := range records {
		record.RecordType = strings.TrimSpace(record.RecordType)
		record.AppID = strings.TrimSpace(record.AppID)
		record.Details = strings.TrimSpace(record.Details)
		if record.RecordType == "" && record.AppID == "" && record.Details == "" {
			continue
		}
		normalized = append(normalized, record)
	}
	return normalized
}

func soldierScanDest(s *models.Soldier) []interface{} {
	var (
		displayID             sql.NullString
		syncID                sql.NullString
		entryType             sql.NullString
		maidenName            sql.NullString
		spouseSoldierID       sql.NullInt64
		pensionID             sql.NullString
		applicationID         sql.NullString
		prefix                sql.NullString
		firstName             sql.NullString
		middleName            sql.NullString
		lastName              sql.NullString
		suffix                sql.NullString
		rank                  sql.NullString
		rankIn                sql.NullString
		rankOut               sql.NullString
		unit                  sql.NullString
		pensionState          sql.NullString
		confederateHomeStatus sql.NullString
		confederateHomeName   sql.NullString
		birthInfo             sql.NullString
		buriedIn              sql.NullString
		notes                 sql.NullString
		reviewReason          sql.NullString
		addedBy               sql.NullString
		lastEditedBy          sql.NullString
		lastEditedFields      sql.NullString
		lastEditedAt          sql.NullString
		createdAt             sql.NullString
		deathYear             sql.NullInt64
		deathMonth            sql.NullInt64
		deathDay              sql.NullInt64
		birthDate             sql.NullString
		deathDate             sql.NullString
		updatedAt             sql.NullString
	)

	return []interface{}{
		&s.ID,
		nullStringDest(&s.DisplayID, &displayID),
		nullStringDest(&s.SyncID, &syncID),
		nullStringDest(&s.EntryType, &entryType),
		nullInt64Dest(&s.SpouseSoldierID, &spouseSoldierID),
		nullStringDest(&s.MaidenName, &maidenName),
		&s.IsGenerated,
		nullStringDest(&s.PensionID, &pensionID),
		nullStringDest(&s.ApplicationID, &applicationID),
		nullStringDest(&s.Prefix, &prefix),
		nullStringDest(&s.FirstName, &firstName),
		nullStringDest(&s.MiddleName, &middleName),
		nullStringDest(&s.LastName, &lastName),
		nullStringDest(&s.Suffix, &suffix),
		nullStringDest(&s.Rank, &rank),
		nullStringDest(&s.RankIn, &rankIn),
		nullStringDest(&s.RankOut, &rankOut),
		nullStringDest(&s.Unit, &unit),
		nullStringDest(&s.PensionState, &pensionState),
		nullStringDest(&s.ConfederateHomeStatus, &confederateHomeStatus),
		nullStringDest(&s.ConfederateHomeName, &confederateHomeName),
		nullIntDest(&s.DeathYear, &deathYear),
		nullIntDest(&s.DeathMonth, &deathMonth),
		nullIntDest(&s.DeathDay, &deathDay),
		nullStringDest(&s.BirthDate, &birthDate),
		nullStringDest(&s.DeathDate, &deathDate),
		nullStringDest(&s.BirthInfo, &birthInfo),
		nullStringDest(&s.BuriedIn, &buriedIn),
		nullStringDest(&s.Notes, &notes),
		&s.NeedsReview,
		nullStringDest(&s.ReviewReason, &reviewReason),
		nullStringDest(&s.AddedBy, &addedBy),
		nullStringDest(&s.LastEditedBy, &lastEditedBy),
		nullStringDest(&s.LastEditedFields, &lastEditedFields),
		nullStringDest(&s.LastEditedAt, &lastEditedAt),
		nullStringDest(&s.CreatedAt, &createdAt),
		nullStringDest(&s.UpdatedAt, &updatedAt),
	}
}

func soldierListScanDest(s *models.Soldier) []interface{} {
	dest := soldierScanDest(s)
	dest = append(dest, &s.SpouseDisplayID, &s.RecordCount, &s.ImageCount)
	return dest
}

func normalizeSoldierEntry(tx *sql.Tx, soldier *models.Soldier) error {
	soldier.EntryType = normalizeEntryType(soldier.EntryType)
	soldier.Prefix = strings.TrimSpace(soldier.Prefix)
	soldier.FirstName = strings.TrimSpace(soldier.FirstName)
	soldier.MiddleName = strings.TrimSpace(soldier.MiddleName)
	soldier.LastName = strings.TrimSpace(soldier.LastName)
	soldier.Suffix = strings.TrimSpace(soldier.Suffix)
	soldier.MaidenName = strings.TrimSpace(soldier.MaidenName)
	if soldier.EntryType == "soldier" {
		soldier.SpouseSoldierID = 0
		return nil
	}
	if soldier.SpouseSoldierID < 1 {
		return fmt.Errorf("spouse_soldier_id required")
	}
	var spouseType string
	if err := tx.QueryRow(`SELECT entry_type FROM soldiers WHERE id = ?`, soldier.SpouseSoldierID).Scan(&spouseType); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("selected spouse record not found")
		}
		return err
	}
	if normalizeEntryType(spouseType) != "soldier" {
		return fmt.Errorf("selected spouse must be a soldier record")
	}
	return nil
}

func normalizeEntryType(entryType string) string {
	switch strings.ToLower(strings.TrimSpace(entryType)) {
	case "wife":
		return "wife"
	case "widow":
		return "widow"
	default:
		return "soldier"
	}
}

func normalizeConfederateHomeFields(soldier *models.Soldier) {
	soldier.ConfederateHomeStatus = normalizeConfederateHomeStatus(soldier.ConfederateHomeStatus)
	soldier.ConfederateHomeName = strings.TrimSpace(soldier.ConfederateHomeName)
	if soldier.ConfederateHomeStatus == "None" {
		soldier.ConfederateHomeName = ""
	}
}

func normalizeConfederateHomeStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "inmate":
		return "Inmate"
	case "staffer":
		return "Staffer"
	case "trustee":
		return "Trustee"
	default:
		return "None"
	}
}

func (s *SoldierService) loadFormSuggestions() (models.SoldierFormSuggestions, error) {
	rankIn, err := distinctTextValues(s.db.Conn(), `SELECT DISTINCT TRIM(rank_in) FROM soldiers WHERE rank_in IS NOT NULL AND TRIM(rank_in) <> '' ORDER BY TRIM(rank_in)`)
	if err != nil {
		return models.SoldierFormSuggestions{}, err
	}
	rankOut, err := distinctTextValues(s.db.Conn(), `SELECT value FROM (
		SELECT DISTINCT TRIM(rank_out) AS value FROM soldiers WHERE rank_out IS NOT NULL AND TRIM(rank_out) <> ''
		UNION
		SELECT DISTINCT TRIM(rank) AS value FROM soldiers WHERE rank IS NOT NULL AND TRIM(rank) <> ''
	) ORDER BY value`)
	if err != nil {
		return models.SoldierFormSuggestions{}, err
	}
	unit, err := distinctTextValues(s.db.Conn(), `SELECT DISTINCT TRIM(unit) FROM soldiers WHERE unit IS NOT NULL AND TRIM(unit) <> '' ORDER BY TRIM(unit)`)
	if err != nil {
		return models.SoldierFormSuggestions{}, err
	}
	prefix, err := distinctTextValues(s.db.Conn(), `SELECT DISTINCT TRIM(prefix) FROM soldiers WHERE prefix IS NOT NULL AND TRIM(prefix) <> '' ORDER BY TRIM(prefix)`)
	if err != nil {
		return models.SoldierFormSuggestions{}, err
	}
	suffix, err := distinctTextValues(s.db.Conn(), `SELECT DISTINCT TRIM(suffix) FROM soldiers WHERE suffix IS NOT NULL AND TRIM(suffix) <> '' ORDER BY TRIM(suffix)`)
	if err != nil {
		return models.SoldierFormSuggestions{}, err
	}
	pensionState, err := distinctTextValues(s.db.Conn(), `SELECT DISTINCT TRIM(pension_state) FROM soldiers WHERE pension_state IS NOT NULL AND TRIM(pension_state) <> '' ORDER BY TRIM(pension_state)`)
	if err != nil {
		return models.SoldierFormSuggestions{}, err
	}
	buriedIn, err := distinctTextValues(s.db.Conn(), `SELECT DISTINCT TRIM(buried_in) FROM soldiers WHERE buried_in IS NOT NULL AND TRIM(buried_in) <> '' ORDER BY TRIM(buried_in)`)
	if err != nil {
		return models.SoldierFormSuggestions{}, err
	}
	confederateHomeName, err := distinctTextValues(s.db.Conn(), `SELECT DISTINCT TRIM(confederate_home_name) FROM soldiers WHERE confederate_home_name IS NOT NULL AND TRIM(confederate_home_name) <> '' ORDER BY TRIM(confederate_home_name)`)
	if err != nil {
		return models.SoldierFormSuggestions{}, err
	}
	recordType, err := distinctTextValues(s.db.Conn(), `SELECT DISTINCT TRIM(record_type) FROM records WHERE record_type IS NOT NULL AND TRIM(record_type) <> '' ORDER BY TRIM(record_type)`)
	if err != nil {
		return models.SoldierFormSuggestions{}, err
	}
	return models.SoldierFormSuggestions{
		RankIn:              rankIn,
		RankOut:             rankOut,
		Unit:                unit,
		Prefix:              prefix,
		Suffix:              suffix,
		PensionState:        pensionState,
		BuriedIn:            buriedIn,
		ConfederateHomeName: confederateHomeName,
		RecordType:          recordType,
	}, nil
}

func distinctTextValues(conn *sql.DB, query string) ([]string, error) {
	rows, err := conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	values := []string{}
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		values = append(values, strings.TrimSpace(value))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

func (s *SoldierService) invalidateFormSuggestions() {
	s.formSuggestionsMu.Lock()
	defer s.formSuggestionsMu.Unlock()
	s.formSuggestions = nil
}

func nullableInt64(value int64) interface{} {
	if value < 1 {
		return nil
	}
	return value
}

func nullInt64Dest(target *int64, holder *sql.NullInt64) interface{ Scan(any) error } {
	return scannerFunc(func(value any) error {
		if err := holder.Scan(value); err != nil {
			return err
		}
		if holder.Valid {
			*target = holder.Int64
		} else {
			*target = 0
		}
		return nil
	})
}

func (s *SoldierService) MarriageCandidates() ([]models.Soldier, error) {
	rows, err := s.db.Conn().Query(`SELECT ` + soldierSelectColumns + ` FROM soldiers WHERE entry_type = 'soldier' ORDER BY last_name, first_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSoldiers(rows)
}

func (s *SoldierService) FormSuggestions() (models.SoldierFormSuggestions, error) {
	s.formSuggestionsMu.RLock()
	if s.formSuggestions != nil {
		cached := *s.formSuggestions
		s.formSuggestionsMu.RUnlock()
		return cached, nil
	}
	s.formSuggestionsMu.RUnlock()

	suggestions, err := s.loadFormSuggestions()
	if err != nil {
		return models.SoldierFormSuggestions{}, err
	}

	s.formSuggestionsMu.Lock()
	s.formSuggestions = &suggestions
	s.formSuggestionsMu.Unlock()
	return suggestions, nil
}

func spouseReference(conn *sql.DB, spouseSoldierID int64) string {
	if spouseSoldierID < 1 {
		return ""
	}
	var spouse models.Soldier
	if err := conn.QueryRow(`SELECT prefix, first_name, middle_name, last_name, suffix FROM soldiers WHERE id = ?`, spouseSoldierID).Scan(&spouse.Prefix, &spouse.FirstName, &spouse.MiddleName, &spouse.LastName, &spouse.Suffix); err == nil {
		if fullName := strings.TrimSpace(spouse.GetFullName()); fullName != "" {
			return fullName
		}
	}
	var displayID string
	if err := conn.QueryRow(`SELECT display_id FROM soldiers WHERE id = ?`, spouseSoldierID).Scan(&displayID); err == nil {
		return strings.TrimSpace(displayID)
	}
	return ""
}

func spouseDisplayID(conn *sql.DB, spouseSoldierID int64) string {
	if spouseSoldierID < 1 {
		return ""
	}
	var displayID string
	if err := conn.QueryRow(`SELECT display_id FROM soldiers WHERE id = ?`, spouseSoldierID).Scan(&displayID); err != nil {
		return ""
	}
	return strings.TrimSpace(displayID)
}

func normalizeDisplayID(displayID, nodePrefix string) string {
	return db.SanitizeID(displayID, nodePrefix)
}

func normalizeSoldierDates(soldier *models.Soldier) error {
	birthDate := strings.TrimSpace(soldier.BirthDate)
	if birthDate == "" {
		birthDate = dates.ParseBirthInfo(strings.TrimSpace(soldier.BirthInfo))
	}
	normalizedBirth, err := dates.NormalizeCanonical(birthDate)
	if err != nil {
		return fmt.Errorf("invalid birth_date")
	}
	soldier.BirthDate = normalizedBirth

	deathDate := strings.TrimSpace(soldier.DeathDate)
	if deathDate == "" {
		deathDate = dates.MustFormat(soldier.DeathMonth, soldier.DeathDay, soldier.DeathYear)
	}
	normalizedDeath, err := dates.NormalizeCanonical(deathDate)
	if err != nil {
		return fmt.Errorf("invalid death_date")
	}
	soldier.DeathDate = normalizedDeath
	hydrateLegacyDeathParts(soldier)
	return nil
}

func currentSQLiteTimestamp() string {
	return time.Now().UTC().Format("2006-01-02 15:04:05")
}

func hydrateLegacyDeathParts(soldier *models.Soldier) {
	partial, err := dates.ParseCanonical(strings.TrimSpace(soldier.DeathDate))
	if err != nil {
		return
	}
	soldier.DeathMonth = partial.Month
	soldier.DeathDay = partial.Day
	soldier.DeathYear = partial.Year
}

func hydrateSoldierIdentity(tx *sql.Tx, soldier *models.Soldier) error {
	row := tx.QueryRow(`SELECT sync_id, added_by, created_at, needs_review, review_reason FROM soldiers WHERE id = ?`, soldier.ID)
	var currentSyncID sql.NullString
	var addedBy sql.NullString
	var createdAt sql.NullString
	var reviewReason sql.NullString
	var needsReview bool
	if err := row.Scan(&currentSyncID, &addedBy, &createdAt, &needsReview, &reviewReason); err != nil {
		return err
	}
	if strings.TrimSpace(soldier.SyncID) == "" {
		soldier.SyncID = currentSyncID.String
	}
	if strings.TrimSpace(soldier.AddedBy) == "" {
		soldier.AddedBy = addedBy.String
	}
	if strings.TrimSpace(soldier.CreatedAt) == "" {
		soldier.CreatedAt = createdAt.String
	}
	if !soldier.NeedsReview && strings.TrimSpace(soldier.ReviewReason) == "" {
		soldier.NeedsReview = needsReview
		soldier.ReviewReason = reviewReason.String
	}
	return nil
}

func (s *SoldierService) currentAuditActor() string {
	if identity, err := s.db.UserIdentity(); err == nil {
		if branding := strings.TrimSpace(identity.BrandingName()); branding != "" {
			return branding
		}
		fullName := strings.TrimSpace(strings.Join([]string{identity.FirstName, identity.MiddleName, identity.LastName}, " "))
		if fullName != "" {
			return fullName
		}
	}
	if nodePrefix, err := s.db.NodePrefix(); err == nil && strings.TrimSpace(nodePrefix) != "" {
		return strings.TrimSpace(nodePrefix)
	}
	return "Unknown"
}

func stampCreateAuditFields(actor string, soldier *models.Soldier) {
	if strings.TrimSpace(soldier.AddedBy) == "" {
		soldier.AddedBy = actor
	}
	if strings.TrimSpace(soldier.LastEditedBy) == "" {
		soldier.LastEditedBy = actor
	}
	if strings.TrimSpace(soldier.LastEditedFields) == "" {
		soldier.LastEditedFields = "created"
	}
	if strings.TrimSpace(soldier.LastEditedAt) == "" {
		soldier.LastEditedAt = soldier.UpdatedAt
	}
}

func stampUpdateAuditFields(actor string, before *models.Soldier, soldier *models.Soldier) {
	if strings.TrimSpace(soldier.AddedBy) == "" && before != nil {
		soldier.AddedBy = before.AddedBy
	}
	changed := diffSoldierFields(before, soldier)
	if len(changed) == 0 {
		changed = []string{"Metadata updated."}
	}
	soldier.LastEditedBy = actor
	soldier.LastEditedFields = strings.Join(changed, "\n")
	soldier.LastEditedAt = soldier.UpdatedAt
}

func diffSoldierFields(before *models.Soldier, after *models.Soldier) []string {
	if before == nil || after == nil {
		return []string{"Metadata updated."}
	}
	type comparedField struct {
		label  string
		before string
		after  string
	}
	fields := []comparedField{
		{"Record ID", auditDisplayID(strings.TrimSpace(before.DisplayID)), auditDisplayID(strings.TrimSpace(after.DisplayID))},
		{"Record Type", auditEntryType(strings.TrimSpace(before.EntryType)), auditEntryType(strings.TrimSpace(after.EntryType))},
		{"Linked Spouse Record", auditSpouseID(before.SpouseSoldierID), auditSpouseID(after.SpouseSoldierID)},
		{"Maiden Name", auditTextValue(before.MaidenName), auditTextValue(after.MaidenName)},
		{"Pension ID", auditTextValue(before.PensionID), auditTextValue(after.PensionID)},
		{"Application ID", auditTextValue(before.ApplicationID), auditTextValue(after.ApplicationID)},
		{"Prefix", auditTextValue(before.Prefix), auditTextValue(after.Prefix)},
		{"First Name", auditTextValue(before.FirstName), auditTextValue(after.FirstName)},
		{"Middle Name", auditTextValue(before.MiddleName), auditTextValue(after.MiddleName)},
		{"Last Name", auditTextValue(before.LastName), auditTextValue(after.LastName)},
		{"Suffix", auditTextValue(before.Suffix), auditTextValue(after.Suffix)},
		{"Rank In", auditTextValue(before.RankIn), auditTextValue(after.RankIn)},
		{"Rank Out", auditTextValue(before.RankOut), auditTextValue(after.RankOut)},
		{"Unit", auditTextValue(before.Unit), auditTextValue(after.Unit)},
		{"Pension State", auditTextValue(before.PensionState), auditTextValue(after.PensionState)},
		{"Confederate Home Status", auditTextValue(before.ConfederateHomeStatus), auditTextValue(after.ConfederateHomeStatus)},
		{"Confederate Home Name", auditTextValue(before.ConfederateHomeName), auditTextValue(after.ConfederateHomeName)},
		{"Birth Date", auditDateValue(before.BirthDate), auditDateValue(after.BirthDate)},
		{"Death Date", auditDateValue(before.DeathDate), auditDateValue(after.DeathDate)},
		{"Birth Info", auditLongTextValue(before.BirthInfo), auditLongTextValue(after.BirthInfo)},
		{"Buried In", auditTextValue(before.BuriedIn), auditTextValue(after.BuriedIn)},
		{"Notes", auditLongTextValue(before.Notes), auditLongTextValue(after.Notes)},
		{"Needs Review", auditBoolValue(before.NeedsReview), auditBoolValue(after.NeedsReview)},
		{"Review Reason", auditLongTextValue(before.ReviewReason), auditLongTextValue(after.ReviewReason)},
	}
	changed := make([]string, 0, len(fields)+1)
	for _, field := range fields {
		if field.before != field.after {
			changed = append(changed, fmt.Sprintf("%s changed from %s to %s.", field.label, field.before, field.after))
		}
	}
	if !recordsEqual(before.Records, after.Records) {
		changed = append(changed, "Records updated.")
	}
	return changed
}

func auditDisplayID(value string) string {
	if strings.TrimSpace(value) == "" {
		return "\"Not recorded\""
	}
	return fmt.Sprintf("%q", strings.TrimSpace(value))
}

func auditEntryType(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "wife":
		return "\"Wife\""
	case "widow":
		return "\"Widow\""
	default:
		return "\"Soldier\""
	}
}

func auditSpouseID(value int64) string {
	if value <= 0 {
		return "\"Not recorded\""
	}
	return fmt.Sprintf("%q", fmt.Sprintf("DB ID %d", value))
}

func auditTextValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "\"Not recorded\""
	}
	return fmt.Sprintf("%q", trimmed)
}

func auditLongTextValue(value string) string {
	normalized := strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if normalized == "" {
		return "\"Not recorded\""
	}
	if len(normalized) > 72 {
		normalized = normalized[:69] + "..."
	}
	return fmt.Sprintf("%q", normalized)
}

func auditDateValue(value string) string {
	display := strings.TrimSpace(dates.Display(strings.TrimSpace(value)))
	if display == "" || display == "Not recorded" {
		return "\"Not recorded\""
	}
	return fmt.Sprintf("%q", display)
}

func auditBoolValue(value bool) string {
	if value {
		return `"Yes"`
	}
	return `"No"`
}

func recordsEqual(left, right []models.Record) bool {
	left = normalizeRecords(left)
	right = normalizeRecords(right)
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if strings.TrimSpace(left[index].RecordType) != strings.TrimSpace(right[index].RecordType) ||
			strings.TrimSpace(left[index].AppID) != strings.TrimSpace(right[index].AppID) ||
			strings.TrimSpace(left[index].Details) != strings.TrimSpace(right[index].Details) {
			return false
		}
	}
	return true
}

func loadSoldierAuditSnapshot(tx *sql.Tx, soldierID int64) (*models.Soldier, error) {
	row := tx.QueryRow(`SELECT `+soldierSelectColumns+` FROM soldiers WHERE id = ?`, soldierID)
	soldier, err := scanSoldier(row)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(`SELECT `+recordSelectColumns+` FROM records WHERE soldier_id = ? ORDER BY id`, soldierID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var record models.Record
		if err := rows.Scan(&record.ID, &record.SyncID, &record.SoldierID, &record.SoldierSyncID, &record.RecordType, &record.AppID, &record.Details); err != nil {
			return nil, err
		}
		soldier.Records = append(soldier.Records, record)
	}
	return soldier, rows.Err()
}

func (s *SoldierService) touchAuditFields(soldierID int64, fields ...string) error {
	actor := s.currentAuditActor()
	updatedAt := currentSQLiteTimestamp()
	changedFields := strings.Join(auditTouchDescriptions(fields), "\n")
	_, err := s.db.Conn().Exec(`UPDATE soldiers SET last_edited_by = ?, last_edited_fields = ?, last_edited_at = ?, updated_at = ? WHERE id = ?`,
		actor, changedFields, updatedAt, updatedAt, soldierID)
	return err
}

func auditTouchDescriptions(fields []string) []string {
	if len(fields) == 0 {
		return []string{"Metadata updated."}
	}
	descriptions := make([]string, 0, len(fields))
	for _, field := range fields {
		switch strings.TrimSpace(strings.ToLower(field)) {
		case "images":
			descriptions = append(descriptions, "Images updated.")
		case "primary_image":
			descriptions = append(descriptions, "Primary image updated.")
		case "records":
			descriptions = append(descriptions, "Records updated.")
		case "needs_review":
			descriptions = append(descriptions, "Review status updated.")
		case "review_status":
			descriptions = append(descriptions, "Review queue cleared.")
		default:
			label := strings.ReplaceAll(strings.TrimSpace(field), "_", " ")
			label = strings.TrimSpace(strings.Title(label))
			if label == "" {
				label = "Metadata"
			}
			descriptions = append(descriptions, label+" updated.")
		}
	}
	return descriptions
}

func (s *SoldierService) soldierSyncIDByID(soldierID int64) (string, error) {
	var syncID string
	if err := s.db.Conn().QueryRow(`SELECT sync_id FROM soldiers WHERE id = ?`, soldierID).Scan(&syncID); err != nil {
		return "", err
	}
	return syncID, nil
}

func (s *SoldierService) shouldAssignPrimaryImage(soldierID int64) (bool, error) {
	var count int
	if err := s.db.Conn().QueryRow(`SELECT COUNT(1) FROM images WHERE soldier_id = ?`, soldierID).Scan(&count); err != nil {
		return false, err
	}
	return count == 0, nil
}

func (s *SoldierService) ensurePrimaryImage(soldierID int64) error {
	var count int
	if err := s.db.Conn().QueryRow(`SELECT COUNT(1) FROM images WHERE soldier_id = ?`, soldierID).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		return nil
	}
	var primaryCount int
	if err := s.db.Conn().QueryRow(`SELECT COUNT(1) FROM images WHERE soldier_id = ? AND is_primary = 1`, soldierID).Scan(&primaryCount); err != nil {
		return err
	}
	if primaryCount > 0 {
		return nil
	}
	_, err := s.db.Conn().Exec(`UPDATE images SET is_primary = CASE WHEN id = (
		SELECT id FROM images WHERE soldier_id = ? ORDER BY id LIMIT 1
	) THEN 1 ELSE 0 END WHERE soldier_id = ?`, soldierID, soldierID)
	return err
}

func nullStringDest(target *string, holder *sql.NullString) interface{ Scan(any) error } {
	return scannerFunc(func(value any) error {
		if err := holder.Scan(value); err != nil {
			return err
		}
		if holder.Valid {
			*target = holder.String
		} else {
			*target = ""
		}
		return nil
	})
}

func nullIntDest(target *int, holder *sql.NullInt64) interface{ Scan(any) error } {
	return scannerFunc(func(value any) error {
		if err := holder.Scan(value); err != nil {
			return err
		}
		if holder.Valid {
			*target = int(holder.Int64)
		} else {
			*target = 0
		}
		return nil
	})
}

type scannerFunc func(any) error

func (f scannerFunc) Scan(value any) error {
	return f(value)
}

func canonicalRank(soldier models.Soldier) string {
	if strings.TrimSpace(soldier.RankOut) != "" {
		return strings.TrimSpace(soldier.RankOut)
	}
	if strings.TrimSpace(soldier.Rank) != "" {
		return strings.TrimSpace(soldier.Rank)
	}
	return strings.TrimSpace(soldier.RankIn)
}

func searchableFirstName(soldier models.Soldier) string {
	return strings.TrimSpace(strings.TrimSpace(soldier.FirstName) + " " + strings.TrimSpace(soldier.MiddleName))
}

func searchableLastName(soldier models.Soldier) string {
	return strings.TrimSpace(soldier.LastName)
}

func searchableUnit(soldier models.Soldier) string {
	return strings.TrimSpace(strings.TrimSpace(soldier.Unit) + " " + strings.TrimSpace(soldier.PensionState))
}

func searchableRank(soldier models.Soldier) string {
	return strings.TrimSpace(strings.TrimSpace(soldier.RankIn) + " " + strings.TrimSpace(soldier.RankOut))
}
