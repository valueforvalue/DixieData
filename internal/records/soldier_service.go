package records

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/valueforvalue/DixieData/internal/dates"
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
)

const (
	soldierSelectColumns     = `id, display_id, sync_id, entry_type, spouse_soldier_id, relationship_label, maiden_name, is_generated, pension_id, application_id, prefix, first_name, middle_name, last_name, suffix, rank, rank_in, rank_out, unit, pension_state, confederate_home_status, confederate_home_name, death_year, death_month, death_day, birth_date, death_date, birth_info, buried_in, notes, needs_review, review_reason, added_by, last_edited_by, last_edited_fields, last_edited_at, created_at, updated_at`
	soldierListSelectColumns = soldierSelectColumns + `, COALESCE((SELECT display_id FROM soldiers linked WHERE linked.id = soldiers.spouse_soldier_id), ''), (SELECT COUNT(*) FROM records WHERE records.soldier_id = soldiers.id), (SELECT COUNT(*) FROM images WHERE images.soldier_id = soldiers.id)`
	recordSelectColumns      = `id, sync_id, soldier_id, soldier_sync_id, record_type, app_id, details`
	imageSelectColumns       = `id, sync_id, soldier_id, soldier_sync_id, file_name, file_path, caption, is_primary`
)

type SoldierService struct {
	db                *db.DB
	formSuggestionsMu sync.RWMutex
	formSuggestions   *models.SoldierFormSuggestions
}

type UnitCamaraderieGraph struct {
	Central            models.Soldier
	UnitLabel          string
	RegimentLabel      string
	CompanyLabel       string
	SameUnit           []UnitCamaraderieConnection
	SameCompanyVariant []UnitCamaraderieConnection
	SameRegiment       []UnitCamaraderieConnection
}

type UnitCamaraderieConnection struct {
	Soldier      models.Soldier
	Relation     string
	Strength     int
	StrengthText string
}

type ServiceTimeline struct {
	Central            models.Soldier
	Events             []ServiceTimelineEvent
	UndatedRecords     []models.Record
	StartLabel         string
	EndLabel           string
	ExactEventCount    int
	InferredEventCount int
}

type ServiceTimelineEvent struct {
	Title           string
	DateLabel       string
	Description     string
	SourceLabel     string
	Category        string
	ConfidenceLabel string
	Approximate     bool
	sortDate        dates.PartialDate
	sortOrder       int
}

type ResearchTask struct {
	ID           int64
	SoldierID    int64
	Title        string
	Notes        string
	EvidenceType string
	Status       string
	CreatedAt    string
	UpdatedAt    string
	ResolvedAt   string
}

type ResearchTaskSuggestion struct {
	Title        string
	Notes        string
	EvidenceType string
}

type ResearchLog struct {
	Central       models.Soldier
	Tasks         []ResearchTask
	Suggestions   []ResearchTaskSuggestion
	OpenCount     int
	ResolvedCount int
}

type ResearchPack struct {
	Central         models.Soldier
	Scope           string
	PlaceLabel      string
	Description     string
	Related         []models.Soldier
	TopUnits        []AnalyticsCount
	TopCemeteries   []AnalyticsCount
	OpenReviewCount int
}

type ResearchCollection struct {
	ID              int64
	Name            string
	Description     string
	CreatedAt       string
	UpdatedAt       string
	ItemCount       int
	ContainsCurrent bool
}

type ResearchCollectionHub struct {
	Current     *models.Soldier
	Collections []ResearchCollection
}

type ResearchCollectionDetail struct {
	Collection ResearchCollection
	Current    *models.Soldier
	Members    []models.Soldier
}

func NewSoldierService(database *db.DB) *SoldierService {
	return &SoldierService{db: database}
}

func (s *SoldierService) Create(soldier models.Soldier) (*models.Soldier, error) {
	conn := s.db.Conn()
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

	tx, err := conn.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

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

	res, err := tx.Exec(`INSERT INTO soldiers (display_id, sync_id, entry_type, spouse_soldier_id, relationship_label, maiden_name, is_generated, pension_id, application_id, prefix, first_name, middle_name, last_name, suffix, rank, rank_in, rank_out, unit, pension_state, confederate_home_status, confederate_home_name, death_year, death_month, death_day, birth_date, death_date, birth_info, buried_in, notes, needs_review, review_reason, added_by, last_edited_by, last_edited_fields, last_edited_at, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		soldier.DisplayID, soldier.SyncID, soldier.EntryType, nullableInt64(soldier.SpouseSoldierID), soldier.RelationshipLabel, soldier.MaidenName, soldier.IsGenerated, soldier.PensionID, soldier.ApplicationID, soldier.Prefix, soldier.FirstName, soldier.MiddleName, soldier.LastName, soldier.Suffix,
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

func (s *SoldierService) GetByDisplayID(displayID string) (*models.Soldier, error) {
	trimmed := strings.TrimSpace(displayID)
	if trimmed == "" {
		return nil, os.ErrNotExist
	}

	conn := s.db.Conn()
	row := conn.QueryRow(`SELECT `+soldierSelectColumns+` FROM soldiers WHERE upper(display_id) = upper(?)`, trimmed)
	soldier, err := scanSoldier(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}

	rows, err := conn.Query(`SELECT `+recordSelectColumns+` FROM records WHERE soldier_id = ? ORDER BY id`, soldier.ID)
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

	imgRows, err := conn.Query(`SELECT `+imageSelectColumns+` FROM images WHERE soldier_id = ? ORDER BY is_primary DESC, id`, soldier.ID)
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
	nodePrefix, err := s.db.NodePrefix()
	if err != nil {
		return err
	}
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

	_, err = tx.Exec(`UPDATE soldiers SET display_id=?, sync_id=?, entry_type=?, spouse_soldier_id=?, relationship_label=?, maiden_name=?, pension_id=?, application_id=?, prefix=?, first_name=?, middle_name=?, last_name=?, suffix=?, rank=?, rank_in=?, rank_out=?, unit=?, pension_state=?, confederate_home_status=?, confederate_home_name=?, death_year=?, death_month=?, death_day=?, birth_date=?, death_date=?, birth_info=?, buried_in=?, notes=?, needs_review=?, review_reason=?, added_by=?, last_edited_by=?, last_edited_fields=?, last_edited_at=?, updated_at=? WHERE id=?`,
		soldier.DisplayID, soldier.SyncID, soldier.EntryType, nullableInt64(soldier.SpouseSoldierID), soldier.RelationshipLabel, soldier.MaidenName, soldier.PensionID, soldier.ApplicationID, soldier.Prefix, soldier.FirstName, soldier.MiddleName, soldier.LastName, soldier.Suffix, soldier.Rank, soldier.RankIn, soldier.RankOut, soldier.Unit, soldier.PensionState, soldier.ConfederateHomeStatus, soldier.ConfederateHomeName,
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
			END), 0),
			COALESCE(SUM(CASE
				WHEN LOWER(TRIM(entry_type)) = 'linked_person' THEN 1
				ELSE 0
			END), 0)
		FROM soldiers`)
	var counts models.ArchiveCounts
	if err := row.Scan(&counts.TotalSoldiers, &counts.TotalWivesWidows, &counts.TotalLinkedPeople); err != nil {
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
				COALESCE(snippet(soldiers_fts, 19, '', '', '...', 12), '') AS notes_snippet,
				COALESCE(snippet(soldiers_fts, 20, '', '', '...', 12), '') AS scratch_snippet,
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
	return `display_id LIKE ? OR pension_id LIKE ? OR application_id LIKE ? OR prefix LIKE ? OR first_name LIKE ? OR middle_name LIKE ? OR last_name LIKE ? OR suffix LIKE ? OR unit LIKE ? OR rank LIKE ? OR rank_in LIKE ? OR rank_out LIKE ? OR pension_state LIKE ? OR confederate_home_status LIKE ? OR confederate_home_name LIKE ? OR buried_in LIKE ? OR maiden_name LIKE ? OR relationship_label LIKE ? OR notes LIKE ? OR EXISTS (
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
		like, like, like, like,
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
	case "wife", "widow", "linked_person":
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
	if search.RelationshipLabel != "" {
		appendContainsFilter("relationship_label", search.RelationshipLabel)
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

func (s *SoldierService) UnitCamaraderieGraph(soldierID int64) (*UnitCamaraderieGraph, error) {
	central, err := s.GetByID(soldierID)
	if err != nil {
		return nil, err
	}
	if normalizeEntryType(central.EntryType) != "soldier" {
		return nil, fmt.Errorf("unit camaraderie is available for soldier records only")
	}
	keys := deriveUnitGraphKeys(central.Unit)
	if keys.normalizedUnit == "" {
		return nil, fmt.Errorf("this record does not have enough unit information for camaraderie analysis")
	}

	rows, err := s.db.Conn().Query(`
		SELECT `+soldierListSelectColumns+`
		FROM soldiers
		WHERE id <> ?
		  AND TRIM(COALESCE(unit, '')) <> ''
		  AND (entry_type IS NULL OR TRIM(entry_type) = '' OR LOWER(TRIM(entry_type)) = 'soldier')
		ORDER BY last_name, first_name
	`, soldierID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	peers, err := scanListSoldiers(rows)
	if err != nil {
		return nil, err
	}

	graph := &UnitCamaraderieGraph{
		Central:       *central,
		UnitLabel:     strings.TrimSpace(central.Unit),
		RegimentLabel: keys.regimentLabel,
		CompanyLabel:  keys.companyLabel,
	}

	for _, peer := range peers {
		peerKeys := deriveUnitGraphKeys(peer.Unit)
		switch {
		case peerKeys.normalizedUnit == "":
			continue
		case peerKeys.normalizedUnit == keys.normalizedUnit:
			graph.SameUnit = append(graph.SameUnit, newUnitCamaraderieConnection(peer, "Same recorded unit", 3))
		case keys.regimentKey != "" && peerKeys.regimentKey == keys.regimentKey && keys.companyKey != "" && peerKeys.companyKey == keys.companyKey:
			graph.SameCompanyVariant = append(graph.SameCompanyVariant, newUnitCamaraderieConnection(peer, "Same company letter in regiment variant", 2))
		case keys.regimentKey != "" && peerKeys.regimentKey == keys.regimentKey:
			graph.SameRegiment = append(graph.SameRegiment, newUnitCamaraderieConnection(peer, "Same regiment", 1))
		}
	}

	sort.SliceStable(graph.SameUnit, func(i, j int) bool { return unitConnectionLess(graph.SameUnit[i], graph.SameUnit[j]) })
	sort.SliceStable(graph.SameCompanyVariant, func(i, j int) bool {
		return unitConnectionLess(graph.SameCompanyVariant[i], graph.SameCompanyVariant[j])
	})
	sort.SliceStable(graph.SameRegiment, func(i, j int) bool { return unitConnectionLess(graph.SameRegiment[i], graph.SameRegiment[j]) })

	graph.SameUnit = limitUnitConnections(graph.SameUnit, 12)
	graph.SameCompanyVariant = limitUnitConnections(graph.SameCompanyVariant, 12)
	graph.SameRegiment = limitUnitConnections(graph.SameRegiment, 18)
	return graph, nil
}

func (s *SoldierService) ServiceTimeline(soldierID int64) (*ServiceTimeline, error) {
	central, err := s.GetByID(soldierID)
	if err != nil {
		return nil, err
	}
	if normalizeEntryType(central.EntryType) != "soldier" {
		return nil, fmt.Errorf("service timeline is available for soldier records only")
	}

	timeline := &ServiceTimeline{Central: *central}
	if partial, approximate := soldierBirthTimelineDate(*central); partial.HasAny() {
		timeline.Events = append(timeline.Events, newServiceTimelineEvent(
			"Birth",
			partial,
			"Profile",
			strings.TrimSpace(central.BirthInfo),
			"life",
			approximate,
			0,
		))
	}
	for _, record := range central.Records {
		event, ok := serviceTimelineEventFromRecord(record)
		if ok {
			timeline.Events = append(timeline.Events, event)
			continue
		}
		if timelineRecordHasContent(record) {
			timeline.UndatedRecords = append(timeline.UndatedRecords, record)
		}
	}
	if partial, approximate := soldierDeathTimelineDate(*central); partial.HasAny() {
		timeline.Events = append(timeline.Events, newServiceTimelineEvent(
			"Death",
			partial,
			"Profile",
			"",
			"death",
			approximate,
			900,
		))
		if strings.TrimSpace(central.BuriedIn) != "" {
			timeline.Events = append(timeline.Events, newServiceTimelineEvent(
				"Burial recorded",
				partial,
				"Profile",
				central.BuriedIn,
				"burial",
				true,
				901,
			))
		}
	}

	sort.SliceStable(timeline.Events, func(i, j int) bool {
		return serviceTimelineEventLess(timeline.Events[i], timeline.Events[j])
	})
	if len(timeline.Events) > 0 {
		timeline.StartLabel = timeline.Events[0].DateLabel
		timeline.EndLabel = timeline.Events[len(timeline.Events)-1].DateLabel
	}
	for _, event := range timeline.Events {
		if event.Approximate {
			timeline.InferredEventCount++
		} else {
			timeline.ExactEventCount++
		}
	}
	return timeline, nil
}

func (s *SoldierService) ResearchLog(soldierID int64) (*ResearchLog, error) {
	central, err := s.GetByID(soldierID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Conn().Query(`
		SELECT id, soldier_id, title, notes, evidence_type, status, created_at, COALESCE(updated_at, ''), COALESCE(resolved_at, '')
		FROM research_tasks
		WHERE soldier_id = ?
		ORDER BY CASE status WHEN 'open' THEN 0 ELSE 1 END, created_at DESC, id DESC
	`, soldierID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	log := &ResearchLog{
		Central:     *central,
		Suggestions: suggestedResearchTasks(*central),
	}
	for rows.Next() {
		var task ResearchTask
		if err := rows.Scan(&task.ID, &task.SoldierID, &task.Title, &task.Notes, &task.EvidenceType, &task.Status, &task.CreatedAt, &task.UpdatedAt, &task.ResolvedAt); err != nil {
			return nil, err
		}
		log.Tasks = append(log.Tasks, task)
		if strings.EqualFold(strings.TrimSpace(task.Status), "resolved") {
			log.ResolvedCount++
		} else {
			log.OpenCount++
		}
	}
	return log, rows.Err()
}

func (s *SoldierService) AddResearchTask(soldierID int64, title, notes, evidenceType string) error {
	if _, err := s.GetByID(soldierID); err != nil {
		return err
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return fmt.Errorf("research task title is required")
	}
	evidenceType = normalizeResearchEvidenceType(evidenceType)
	notes = strings.TrimSpace(notes)
	_, err := s.db.Conn().Exec(`
		INSERT INTO research_tasks (soldier_id, title, notes, evidence_type, status, updated_at)
		VALUES (?, ?, ?, ?, 'open', ?)
	`, soldierID, title, notes, evidenceType, currentSQLiteTimestamp())
	return err
}

func (s *SoldierService) ResolveResearchTask(soldierID, taskID int64) error {
	result, err := s.db.Conn().Exec(`
		UPDATE research_tasks
		SET status = 'resolved', updated_at = ?, resolved_at = ?
		WHERE id = ? AND soldier_id = ? AND status <> 'resolved'
	`, currentSQLiteTimestamp(), currentSQLiteTimestamp(), taskID, soldierID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("research task not found")
	}
	return nil
}

func (s *SoldierService) ResearchPackForSoldier(soldierID int64, scope string) (*ResearchPack, error) {
	central, err := s.GetByID(soldierID)
	if err != nil {
		return nil, err
	}
	scope = strings.ToLower(strings.TrimSpace(scope))
	label := ""
	whereClause := ""
	args := []interface{}{}
	switch scope {
	case "state":
		label = researchPackStateLabel(*central)
		if label == "" {
			return nil, fmt.Errorf("this record does not have a state research pack yet")
		}
		whereClause = `(LOWER(TRIM(COALESCE(pension_state, ''))) = LOWER(?) OR LOWER(COALESCE(birth_info, '')) LIKE LOWER(?))`
		args = []interface{}{label, "%" + label + "%"}
	case "county":
		label, _ = parseBirthCountyState(central.BirthInfo)
		if label == "" {
			return nil, fmt.Errorf("this record does not have a county research pack yet")
		}
		whereClause = `LOWER(COALESCE(birth_info, '')) LIKE LOWER(?)`
		args = []interface{}{"%" + label + "%"}
	default:
		return nil, fmt.Errorf("unknown research pack scope")
	}

	relatedRows, err := s.db.Conn().Query(
		"SELECT "+soldierListSelectColumns+" FROM soldiers WHERE id <> ? AND "+whereClause+" ORDER BY last_name, first_name LIMIT 40",
		append([]interface{}{soldierID}, args...)...,
	)
	if err != nil {
		return nil, err
	}
	defer relatedRows.Close()
	related, err := scanListSoldiers(relatedRows)
	if err != nil {
		return nil, err
	}

	pack := &ResearchPack{
		Central:     *central,
		Scope:       scope,
		PlaceLabel:  label,
		Related:     related,
		Description: researchPackDescription(scope, label),
	}
	if pack.TopUnits, err = s.researchPackCounts(whereClause, args, "unit", 5); err != nil {
		return nil, err
	}
	if pack.TopCemeteries, err = s.researchPackCounts(whereClause, args, "buried_in", 5); err != nil {
		return nil, err
	}
	if err := s.db.Conn().QueryRow("SELECT COUNT(*) FROM soldiers WHERE "+whereClause+" AND needs_review = 1", args...).Scan(&pack.OpenReviewCount); err != nil {
		return nil, err
	}
	return pack, nil
}

func (s *SoldierService) ResearchPackForPersonRecord(personRecordID int64, scope string) (*ResearchPack, error) {
	return s.ResearchPackForSoldier(personRecordID, scope)
}

func (s *SoldierService) ResearchCollectionsHub(currentSoldierID int64) (*ResearchCollectionHub, error) {
	hub := &ResearchCollectionHub{}
	if currentSoldierID > 0 {
		current, err := s.GetByID(currentSoldierID)
		if err != nil {
			return nil, err
		}
		hub.Current = current
	}
	rows, err := s.db.Conn().Query(`
		SELECT c.id, c.name, COALESCE(c.description, ''), c.created_at, COALESCE(c.updated_at, ''), COUNT(i.soldier_id),
		       CASE WHEN ? > 0 AND EXISTS (SELECT 1 FROM research_collection_items existing WHERE existing.collection_id = c.id AND existing.soldier_id = ?) THEN 1 ELSE 0 END
		FROM research_collections c
		LEFT JOIN research_collection_items i ON i.collection_id = c.id
		GROUP BY c.id, c.name, c.description, c.created_at, c.updated_at
		ORDER BY LOWER(c.name) ASC
	`, currentSoldierID, currentSoldierID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			collection ResearchCollection
			contains   int
		)
		if err := rows.Scan(&collection.ID, &collection.Name, &collection.Description, &collection.CreatedAt, &collection.UpdatedAt, &collection.ItemCount, &contains); err != nil {
			return nil, err
		}
		collection.ContainsCurrent = contains == 1
		hub.Collections = append(hub.Collections, collection)
	}
	return hub, rows.Err()
}

func (s *SoldierService) CreateResearchCollection(name, description string) error {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if name == "" {
		return fmt.Errorf("collection name is required")
	}
	_, err := s.db.Conn().Exec(`
		INSERT INTO research_collections (name, description, updated_at)
		VALUES (?, ?, ?)
	`, name, description, currentSQLiteTimestamp())
	return err
}

func (s *SoldierService) AddSoldierToResearchCollection(collectionID, soldierID int64) error {
	if _, err := s.GetByID(soldierID); err != nil {
		return err
	}
	result, err := s.db.Conn().Exec(`
		INSERT OR IGNORE INTO research_collection_items (collection_id, soldier_id, created_at)
		VALUES (?, ?, ?)
	`, collectionID, soldierID, currentSQLiteTimestamp())
	if err != nil {
		return err
	}
	if _, err := s.db.Conn().Exec(`UPDATE research_collections SET updated_at = ? WHERE id = ?`, currentSQLiteTimestamp(), collectionID); err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("record is already in that collection")
	}
	return nil
}

func (s *SoldierService) AddPersonRecordToResearchCollection(collectionID, personRecordID int64) error {
	return s.AddSoldierToResearchCollection(collectionID, personRecordID)
}

func (s *SoldierService) ResearchCollectionDetail(collectionID int64, currentSoldierID int64) (*ResearchCollectionDetail, error) {
	detail := &ResearchCollectionDetail{}
	if currentSoldierID > 0 {
		current, err := s.GetByID(currentSoldierID)
		if err != nil {
			return nil, err
		}
		detail.Current = current
	}
	if err := s.db.Conn().QueryRow(`
		SELECT c.id, c.name, COALESCE(c.description, ''), c.created_at, COALESCE(c.updated_at, ''), COUNT(i.soldier_id)
		FROM research_collections c
		LEFT JOIN research_collection_items i ON i.collection_id = c.id
		WHERE c.id = ?
		GROUP BY c.id, c.name, c.description, c.created_at, c.updated_at
	`, collectionID).Scan(&detail.Collection.ID, &detail.Collection.Name, &detail.Collection.Description, &detail.Collection.CreatedAt, &detail.Collection.UpdatedAt, &detail.Collection.ItemCount); err != nil {
		return nil, err
	}
	rows, err := s.db.Conn().Query(`
		SELECT `+soldierListSelectColumns+`
		FROM soldiers
		WHERE id IN (SELECT soldier_id FROM research_collection_items WHERE collection_id = ?)
		ORDER BY last_name, first_name
	`, collectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	members, err := scanListSoldiers(rows)
	if err != nil {
		return nil, err
	}
	detail.Members = members
	if currentSoldierID > 0 {
		for _, member := range members {
			if member.ID == currentSoldierID {
				detail.Collection.ContainsCurrent = true
				break
			}
		}
	}
	return detail, nil
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
		{label: "Display ID", value: strings.TrimSpace(soldier.DisplayID)},
		{label: "Pension ID", value: strings.TrimSpace(soldier.PensionID)},
		{label: "Application ID", value: strings.TrimSpace(soldier.ApplicationID)},
		{label: "Name", value: strings.TrimSpace(soldier.GetFullName())},
		{label: "Rank", value: soldierSearchRank(soldier)},
		{label: "Unit", value: strings.TrimSpace(soldier.Unit)},
		{label: "Pension State", value: strings.TrimSpace(soldier.PensionState)},
		{label: "Buried In", value: strings.TrimSpace(soldier.BuriedIn)},
		{label: "Maiden Name", value: strings.TrimSpace(soldier.MaidenName)},
		{label: "Relationship", value: strings.TrimSpace(soldier.RelationshipLabel)},
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

type unitGraphKeys struct {
	normalizedUnit string
	regimentKey    string
	regimentLabel  string
	companyKey     string
	companyLabel   string
}

var (
	companyLetterPattern     = regexp.MustCompile(`(?i)\b(?:company|co)\.?\s*([a-z])\b`)
	companyPrefixPattern     = regexp.MustCompile(`(?i)^\s*(?:company|co)\.?\s*[a-z]\s*[, -]*`)
	nonAlphaNumPattern       = regexp.MustCompile(`[^a-z0-9]+`)
	slashTimelineDatePattern = regexp.MustCompile(`\b(\d{1,2})/(\d{1,2})/(\d{4})\b`)
	birthCountyStatePattern  = regexp.MustCompile(`(?i)\b(?:in\s+)?([A-Za-z .'-]+ County),\s*([A-Za-z .'-]+)\b`)
	birthStateTailPattern    = regexp.MustCompile(`(?i),\s*([A-Za-z .'-]+)\.?\s*$`)
)

func deriveUnitGraphKeys(unit string) unitGraphKeys {
	trimmed := strings.TrimSpace(unit)
	if trimmed == "" {
		return unitGraphKeys{}
	}
	keys := unitGraphKeys{
		normalizedUnit: normalizeUnitGraphText(trimmed),
		regimentLabel:  trimmed,
	}
	if key, label := extractUnitCompany(trimmed); key != "" {
		keys.companyKey = key
		keys.companyLabel = label
	}
	if idx := strings.Index(trimmed, ","); idx >= 0 {
		keys.regimentLabel = strings.TrimSpace(trimmed[idx+1:])
	} else if stripped := strings.TrimSpace(companyPrefixPattern.ReplaceAllString(trimmed, "")); stripped != "" {
		keys.regimentLabel = stripped
	}
	if keys.regimentLabel == "" {
		keys.regimentLabel = trimmed
	}
	keys.regimentKey = normalizeUnitGraphText(keys.regimentLabel)
	return keys
}

func extractUnitCompany(unit string) (string, string) {
	if match := companyLetterPattern.FindStringSubmatch(unit); len(match) > 1 {
		key := strings.ToUpper(strings.TrimSpace(match[1]))
		if key != "" {
			return key, "Company " + key
		}
	}
	fields := strings.Fields(normalizeUnitGraphText(unit))
	if len(fields) >= 2 && (fields[0] == "co" || fields[0] == "company") && len(fields[1]) == 1 {
		key := strings.ToUpper(fields[1])
		return key, "Company " + key
	}
	return "", ""
}

func normalizeUnitGraphText(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = nonAlphaNumPattern.ReplaceAllString(normalized, " ")
	return strings.Join(strings.Fields(normalized), " ")
}

func newUnitCamaraderieConnection(soldier models.Soldier, relation string, strength int) UnitCamaraderieConnection {
	return UnitCamaraderieConnection{
		Soldier:      soldier,
		Relation:     relation,
		Strength:     strength,
		StrengthText: unitConnectionStrengthText(strength),
	}
}

func unitConnectionStrengthText(strength int) string {
	switch strength {
	case 3:
		return "Strong"
	case 2:
		return "Medium"
	default:
		return "Broad"
	}
}

func unitConnectionLess(left, right UnitCamaraderieConnection) bool {
	if left.Strength != right.Strength {
		return left.Strength > right.Strength
	}
	if strings.ToLower(strings.TrimSpace(left.Soldier.LastName)) != strings.ToLower(strings.TrimSpace(right.Soldier.LastName)) {
		return strings.ToLower(strings.TrimSpace(left.Soldier.LastName)) < strings.ToLower(strings.TrimSpace(right.Soldier.LastName))
	}
	return strings.ToLower(strings.TrimSpace(left.Soldier.FirstName)) < strings.ToLower(strings.TrimSpace(right.Soldier.FirstName))
}

func limitUnitConnections(connections []UnitCamaraderieConnection, limit int) []UnitCamaraderieConnection {
	if limit <= 0 || len(connections) <= limit {
		return connections
	}
	return connections[:limit]
}

func soldierBirthTimelineDate(soldier models.Soldier) (dates.PartialDate, bool) {
	if partial, err := dates.ParseCanonical(strings.TrimSpace(soldier.BirthDate)); err == nil && partial.HasAny() {
		return partial, false
	}
	if parsed := strings.TrimSpace(dates.ParseBirthInfo(soldier.BirthInfo)); parsed != "" {
		if partial, err := dates.ParseCanonical(parsed); err == nil && partial.HasAny() {
			return partial, true
		}
	}
	return dates.PartialDate{}, false
}

func soldierDeathTimelineDate(soldier models.Soldier) (dates.PartialDate, bool) {
	if partial, err := dates.ParseCanonical(strings.TrimSpace(soldier.DeathDate)); err == nil && partial.HasAny() {
		return partial, false
	}
	partial := dates.PartialDate{
		Month: soldier.DeathMonth,
		Day:   soldier.DeathDay,
		Year:  soldier.DeathYear,
	}
	return partial, partial.HasAny()
}

func serviceTimelineEventFromRecord(record models.Record) (ServiceTimelineEvent, bool) {
	partial, approximate, ok := inferTimelineDateFromText(record.Details)
	if !ok {
		return ServiceTimelineEvent{}, false
	}
	description := strings.TrimSpace(record.Details)
	title := strings.TrimSpace(record.RecordType)
	if title == "" {
		title = "Archive record"
	}
	return newServiceTimelineEvent(
		title,
		partial,
		recordTimelineSourceLabel(record),
		description,
		recordTimelineCategory(record),
		approximate,
		100+recordTimelineOrder(record),
	), true
}

func inferTimelineDateFromText(value string) (dates.PartialDate, bool, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return dates.PartialDate{}, false, false
	}
	if match := slashTimelineDatePattern.FindStringSubmatch(trimmed); len(match) == 4 {
		month, monthErr := strconv.Atoi(match[1])
		day, dayErr := strconv.Atoi(match[2])
		year, yearErr := strconv.Atoi(match[3])
		if monthErr == nil && dayErr == nil && yearErr == nil {
			partial := dates.PartialDate{Month: month, Day: day, Year: year}
			if partial.HasAny() {
				return partial, false, true
			}
		}
	}
	if parsed := strings.TrimSpace(dates.ParseBirthInfo(trimmed)); parsed != "" {
		if partial, err := dates.ParseCanonical(parsed); err == nil && partial.HasAny() {
			return partial, true, true
		}
	}
	return dates.PartialDate{}, false, false
}

func newServiceTimelineEvent(title string, partial dates.PartialDate, sourceLabel, description, category string, approximate bool, order int) ServiceTimelineEvent {
	confidence := "Exact"
	if approximate {
		confidence = "Inferred"
	}
	return ServiceTimelineEvent{
		Title:           title,
		DateLabel:       dates.Display(partial.Format()),
		Description:     strings.TrimSpace(description),
		SourceLabel:     strings.TrimSpace(sourceLabel),
		Category:        strings.TrimSpace(category),
		ConfidenceLabel: confidence,
		Approximate:     approximate,
		sortDate:        partial,
		sortOrder:       order,
	}
}

func serviceTimelineEventLess(left, right ServiceTimelineEvent) bool {
	if left.sortDate.Year != right.sortDate.Year {
		return left.sortDate.Year < right.sortDate.Year
	}
	if left.sortDate.Month != right.sortDate.Month {
		return left.sortDate.Month < right.sortDate.Month
	}
	if left.sortDate.Day != right.sortDate.Day {
		return left.sortDate.Day < right.sortDate.Day
	}
	if left.Approximate != right.Approximate {
		return !left.Approximate
	}
	if left.sortOrder != right.sortOrder {
		return left.sortOrder < right.sortOrder
	}
	return strings.ToLower(left.Title) < strings.ToLower(right.Title)
}

func recordTimelineSourceLabel(record models.Record) string {
	label := strings.TrimSpace(record.RecordType)
	if label == "" {
		label = "Archive record"
	}
	if strings.TrimSpace(record.AppID) == "" {
		return label
	}
	return label + " · " + strings.TrimSpace(record.AppID)
}

func recordTimelineCategory(record models.Record) string {
	label := strings.ToLower(strings.TrimSpace(record.RecordType))
	switch {
	case strings.Contains(label, "pension"), strings.Contains(label, "application"):
		return "pension"
	case strings.Contains(label, "parole"), strings.Contains(label, "muster"), strings.Contains(label, "service"), strings.Contains(label, "roster"):
		return "service"
	case strings.Contains(label, "grave"), strings.Contains(label, "burial"), strings.Contains(label, "cemetery"):
		return "burial"
	default:
		return "archive"
	}
}

func recordTimelineOrder(record models.Record) int {
	switch recordTimelineCategory(record) {
	case "service":
		return 10
	case "pension":
		return 20
	case "burial":
		return 30
	default:
		return 40
	}
}

func timelineRecordHasContent(record models.Record) bool {
	return strings.TrimSpace(record.RecordType) != "" || strings.TrimSpace(record.AppID) != "" || strings.TrimSpace(record.Details) != ""
}

func suggestedResearchTasks(soldier models.Soldier) []ResearchTaskSuggestion {
	suggestions := make([]ResearchTaskSuggestion, 0, 8)
	if isSoldierEntryType(soldier.EntryType) && strings.TrimSpace(soldier.Unit) == "" {
		suggestions = append(suggestions, ResearchTaskSuggestion{
			Title:        "Confirm unit assignment",
			Notes:        "No unit is recorded yet. Verify regiment, company, or service branch from attached evidence.",
			EvidenceType: "service",
		})
	}
	if strings.TrimSpace(soldier.PensionID) == "" && strings.TrimSpace(soldier.ApplicationID) == "" {
		suggestions = append(suggestions, ResearchTaskSuggestion{
			Title:        "Locate pension or application file",
			Notes:        "No pension ID or application ID is attached yet. Check pension indexes or state archive holdings.",
			EvidenceType: "pension",
		})
	}
	if strings.TrimSpace(soldier.BuriedIn) == "" {
		suggestions = append(suggestions, ResearchTaskSuggestion{
			Title:        "Confirm burial location",
			Notes:        "Burial place is still missing. Search cemetery registers, memorial sites, or obituary sources.",
			EvidenceType: "burial",
		})
	}
	if strings.TrimSpace(soldier.BirthDate) == "" && strings.TrimSpace(soldier.BirthInfo) == "" {
		suggestions = append(suggestions, ResearchTaskSuggestion{
			Title:        "Confirm birth details",
			Notes:        "Birth date and place context are missing. Look for census, family bible, or probate evidence.",
			EvidenceType: "vital",
		})
	}
	if strings.TrimSpace(soldier.DeathDate) == "" && soldier.DeathYear == 0 {
		suggestions = append(suggestions, ResearchTaskSuggestion{
			Title:        "Confirm death details",
			Notes:        "Death timing is incomplete. Search pension closure files, death certificates, and memorial records.",
			EvidenceType: "vital",
		})
	}
	if len(soldier.Records) == 0 {
		suggestions = append(suggestions, ResearchTaskSuggestion{
			Title:        "Attach primary source records",
			Notes:        "No archive records are attached yet. Add muster, parole, pension, memorial, or correspondence sources.",
			EvidenceType: "archive",
		})
	}
	if !isSoldierEntryType(soldier.EntryType) && soldier.SpouseSoldierID == 0 {
		title := "Link spouse soldier record"
		notes := "Family relationship is not connected yet. Identify and link the matching soldier profile."
		if strings.TrimSpace(soldier.EntryType) == "linked_person" {
			title = "Link related soldier record"
			notes = "This person record still needs its anchor soldier record. Identify and link the matching soldier profile."
		}
		suggestions = append(suggestions, ResearchTaskSuggestion{Title: title, Notes: notes, EvidenceType: "family"})
	}
	if isPersonRecordEntryType(soldier.EntryType) && strings.TrimSpace(soldier.RelationshipLabel) == "" {
		suggestions = append(suggestions, ResearchTaskSuggestion{
			Title:        "Confirm relationship to soldier",
			Notes:        "Relationship to the linked soldier is still blank. Add the exact relationship label used in the source material.",
			EvidenceType: "family",
		})
	}
	if !isSoldierEntryType(soldier.EntryType) && !isPersonRecordEntryType(soldier.EntryType) && strings.TrimSpace(soldier.MaidenName) == "" {
		suggestions = append(suggestions, ResearchTaskSuggestion{
			Title:        "Confirm maiden name",
			Notes:        "Maiden name is still blank. Search marriage, pension, census, or obituary records for supporting evidence.",
			EvidenceType: "family",
		})
	}
	return suggestions
}

func normalizeResearchEvidenceType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "service", "pension", "burial", "vital", "family", "archive":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "general"
	}
}

func isSoldierEntryType(value string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	return trimmed == "" || trimmed == "soldier"
}

func isPersonRecordEntryType(value string) bool {
	return strings.ToLower(strings.TrimSpace(value)) == "linked_person"
}

func researchPackStateLabel(soldier models.Soldier) string {
	if trimmed := strings.TrimSpace(soldier.PensionState); trimmed != "" {
		return trimmed
	}
	_, state := parseBirthCountyState(soldier.BirthInfo)
	return state
}

func parseBirthCountyState(value string) (string, string) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", ""
	}
	if match := birthCountyStatePattern.FindStringSubmatch(trimmed); len(match) == 3 {
		county := strings.TrimSpace(match[1])
		if len(county) > 3 && strings.EqualFold(county[:3], "in ") {
			county = strings.TrimSpace(county[3:])
		}
		return county, strings.Trim(strings.TrimSpace(match[2]), ". ")
	}
	if match := birthStateTailPattern.FindStringSubmatch(trimmed); len(match) == 2 {
		return "", strings.Trim(strings.TrimSpace(match[1]), ". ")
	}
	return "", ""
}

func researchPackDescription(scope, label string) string {
	switch scope {
	case "county":
		return fmt.Sprintf("Records tied to %s through birth-place context and related local evidence.", label)
	default:
		return fmt.Sprintf("Records tied to %s through pension filing or birth-place context.", label)
	}
}

func (s *SoldierService) researchPackCounts(whereClause string, args []interface{}, field string, limit int) ([]AnalyticsCount, error) {
	query := fmt.Sprintf(`
		SELECT TRIM(%s) AS label, COUNT(*)
		FROM soldiers
		WHERE %s AND TRIM(COALESCE(%s, '')) <> ''
		GROUP BY TRIM(%s)
		ORDER BY COUNT(*) DESC, LOWER(TRIM(%s)) ASC
		LIMIT ?
	`, field, whereClause, field, field, field)
	rows, err := s.db.Conn().Query(query, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := []AnalyticsCount{}
	for rows.Next() {
		var count AnalyticsCount
		if err := rows.Scan(&count.Label, &count.Count); err != nil {
			return nil, err
		}
		counts = append(counts, count)
	}
	return counts, rows.Err()
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
		PageTitle:    "Person Record Comparison",
		BackHref:     "/soldiers",
		BackLabel:    "Back",
		Reason:       "Manual side-by-side comparison of two selected person records.",
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
		relationshipLabel     sql.NullString
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
		nullStringDest(&s.RelationshipLabel, &relationshipLabel),
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
	soldier.RelationshipLabel = strings.TrimSpace(soldier.RelationshipLabel)
	if soldier.EntryType == "soldier" {
		soldier.SpouseSoldierID = 0
		soldier.RelationshipLabel = ""
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
	if soldier.EntryType == "linked_person" {
		if soldier.RelationshipLabel == "" {
			return fmt.Errorf("relationship_label required")
		}
		return nil
	}
	soldier.RelationshipLabel = ""
	return nil
}

func normalizeEntryType(entryType string) string {
	switch strings.ToLower(strings.TrimSpace(entryType)) {
	case "wife":
		return "wife"
	case "widow":
		return "widow"
	case "linked_person":
		return "linked_person"
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
	relationshipLabel, err := distinctTextValues(s.db.Conn(), `SELECT DISTINCT TRIM(relationship_label) FROM soldiers WHERE LOWER(TRIM(entry_type)) = 'linked_person' AND relationship_label IS NOT NULL AND TRIM(relationship_label) <> '' ORDER BY TRIM(relationship_label)`)
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
		RelationshipLabel:   relationshipLabel,
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
		{"Display ID", auditDisplayID(strings.TrimSpace(before.DisplayID)), auditDisplayID(strings.TrimSpace(after.DisplayID))},
		{"Person Record Type", auditEntryType(strings.TrimSpace(before.EntryType)), auditEntryType(strings.TrimSpace(after.EntryType))},
		{"Linked Spouse Record", auditSpouseID(before.SpouseSoldierID), auditSpouseID(after.SpouseSoldierID)},
		{"Relationship to Soldier", auditTextValue(before.RelationshipLabel), auditTextValue(after.RelationshipLabel)},
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
	case "linked_person":
		return "\"Person Record\""
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
