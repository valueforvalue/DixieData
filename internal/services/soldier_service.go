package services

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/valueforvalue/DixieData/internal/dates"
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
)

const (
	soldierSelectColumns = `id, display_id, sync_id, entry_type, spouse_soldier_id, maiden_name, is_generated, pension_id, application_id, first_name, middle_name, last_name, rank, rank_in, rank_out, unit, pension_state, death_year, death_month, death_day, birth_date, death_date, birth_info, buried_in, notes, created_at, updated_at`
	recordSelectColumns  = `id, sync_id, soldier_id, soldier_sync_id, record_type, app_id, details`
	imageSelectColumns   = `id, sync_id, soldier_id, soldier_sync_id, file_name, file_path, caption`
)

type SoldierService struct {
	db *db.DB
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

	res, err := tx.Exec(`INSERT INTO soldiers (display_id, sync_id, entry_type, spouse_soldier_id, maiden_name, is_generated, pension_id, application_id, first_name, middle_name, last_name, rank, rank_in, rank_out, unit, pension_state, death_year, death_month, death_day, birth_date, death_date, birth_info, buried_in, notes, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		soldier.DisplayID, soldier.SyncID, soldier.EntryType, nullableInt64(soldier.SpouseSoldierID), soldier.MaidenName, soldier.IsGenerated, soldier.PensionID, soldier.ApplicationID, soldier.FirstName, soldier.MiddleName, soldier.LastName,
		soldier.Rank, soldier.RankIn, soldier.RankOut, soldier.Unit, soldier.PensionState, soldier.DeathYear, soldier.DeathMonth,
		soldier.DeathDay, soldier.BirthDate, soldier.DeathDate, soldier.BirthInfo, soldier.BuriedIn, soldier.Notes, soldier.CreatedAt, soldier.UpdatedAt)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	soldier.ID = id

	_, err = tx.Exec(`INSERT INTO soldiers_fts(rowid, first_name, last_name, unit, soldier_rank) VALUES (?,?,?,?,?)`,
		id, searchableFirstName(soldier), searchableLastName(soldier), searchableUnit(soldier), searchableRank(soldier))
	if err != nil {
		return nil, err
	}

	if err := replaceRecords(tx, soldier.ID, soldier.SyncID, soldier.Records); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &soldier, nil
}

func isGeneratedDisplayID(displayID string) bool {
	trimmed := strings.TrimSpace(displayID)
	parts := strings.Split(trimmed, "-")
	switch len(parts) {
	case 2:
		if parts[0] == "" || !prefixSegmentContainsDigit(parts[0]) {
			return false
		}
		return isFiveDigitGeneratedSuffix(parts[1])
	case 3:
		if strings.ToUpper(parts[1]) != "DXD" || parts[0] == "" {
			return false
		}
		return isFiveDigitGeneratedSuffix(parts[2])
	default:
		return false
	}
}

func prefixSegmentContainsDigit(value string) bool {
	for _, r := range value {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
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

	imgRows, err := conn.Query(`SELECT `+imageSelectColumns+` FROM images WHERE soldier_id = ? ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	defer imgRows.Close()
	for imgRows.Next() {
		var img models.Image
		if err := imgRows.Scan(&img.ID, &img.SyncID, &img.SoldierID, &img.SoldierSyncID, &img.FileName, &img.FilePath, &img.Caption); err != nil {
			return nil, err
		}
		soldier.Images = append(soldier.Images, img)
	}
	soldier.SpouseName = spouseReference(conn, soldier.SpouseSoldierID)

	return soldier, nil
}

func (s *SoldierService) Update(soldier models.Soldier) error {
	conn := s.db.Conn()
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

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
	if err := normalizeSoldierDates(&soldier); err != nil {
		return err
	}
	if strings.TrimSpace(soldier.UpdatedAt) == "" {
		soldier.UpdatedAt = currentSQLiteTimestamp()
	}

	_, err = tx.Exec(`UPDATE soldiers SET display_id=?, sync_id=?, entry_type=?, spouse_soldier_id=?, maiden_name=?, pension_id=?, application_id=?, first_name=?, middle_name=?, last_name=?, rank=?, rank_in=?, rank_out=?, unit=?, pension_state=?, death_year=?, death_month=?, death_day=?, birth_date=?, death_date=?, birth_info=?, buried_in=?, notes=?, updated_at=? WHERE id=?`,
		soldier.DisplayID, soldier.SyncID, soldier.EntryType, nullableInt64(soldier.SpouseSoldierID), soldier.MaidenName, soldier.PensionID, soldier.ApplicationID, soldier.FirstName, soldier.MiddleName, soldier.LastName, soldier.Rank, soldier.RankIn, soldier.RankOut, soldier.Unit, soldier.PensionState,
		soldier.DeathYear, soldier.DeathMonth, soldier.DeathDay, soldier.BirthDate, soldier.DeathDate, soldier.BirthInfo, soldier.BuriedIn, soldier.Notes, soldier.UpdatedAt, soldier.ID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`INSERT INTO soldiers_fts(soldiers_fts, rowid) VALUES('delete', ?)`,
		soldier.ID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO soldiers_fts(rowid, first_name, last_name, unit, soldier_rank) VALUES (?,?,?,?,?)`,
		soldier.ID, searchableFirstName(soldier), searchableLastName(soldier), searchableUnit(soldier), searchableRank(soldier))
	if err != nil {
		return err
	}

	if err := replaceRecords(tx, soldier.ID, soldier.SyncID, soldier.Records); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *SoldierService) Delete(id int64) error {
	_, err := s.db.Conn().Exec(`DELETE FROM soldiers WHERE id = ?`, id)
	return err
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
	_, err = s.db.Conn().Exec(
		`INSERT INTO images (sync_id, soldier_id, soldier_sync_id, file_name, file_path, caption) VALUES (?, ?, ?, ?, ?, ?)`,
		imageSyncID,
		soldierID,
		soldierSyncID,
		fileName,
		filePath,
		caption,
	)
	return err
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
	return err
}

func (s *SoldierService) GetImageByID(imageID int64) (*models.Image, error) {
	row := s.db.Conn().QueryRow(`SELECT `+imageSelectColumns+` FROM images WHERE id = ?`, imageID)
	var image models.Image
	if err := row.Scan(&image.ID, &image.SyncID, &image.SoldierID, &image.SoldierSyncID, &image.FileName, &image.FilePath, &image.Caption); err != nil {
		return nil, err
	}
	return &image, nil
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
	if err == nil {
		return soldiers, total, nil
	}
	return s.searchWithLike(query, pageSize, offset)
}

func (s *SoldierService) searchWithFTS(query string, pageSize, offset int) ([]models.Soldier, int, error) {
	conn := s.db.Conn()
	like := "%" + query + "%"

	var total int
	err := conn.QueryRow(`
		SELECT COUNT(*) FROM (
			SELECT id FROM soldiers WHERE display_id LIKE ? OR pension_id LIKE ? OR application_id LIKE ? OR buried_in LIKE ?
			UNION
			SELECT rowid AS id FROM soldiers_fts WHERE soldiers_fts MATCH ?
		) matches
	`, like, like, like, like, query).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := conn.Query(`
		SELECT `+soldierSelectColumns+`
		FROM soldiers
		WHERE id IN (
			SELECT id FROM soldiers WHERE display_id LIKE ? OR pension_id LIKE ? OR application_id LIKE ? OR buried_in LIKE ?
			UNION
			SELECT rowid AS id FROM soldiers_fts WHERE soldiers_fts MATCH ?
		)
		ORDER BY last_name, first_name
		LIMIT ? OFFSET ?
	`, like, like, like, like, query, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	soldiers, err := scanSoldiers(rows)
	return soldiers, total, err
}

func (s *SoldierService) searchWithLike(query string, pageSize, offset int) ([]models.Soldier, int, error) {
	conn := s.db.Conn()
	like := "%" + query + "%"
	args := []interface{}{like, like, like, like, like, like, like, like, like, like, like, like}
	args = append(args, like)

	var total int
	err := conn.QueryRow(`
		SELECT COUNT(*)
		FROM soldiers
		WHERE display_id LIKE ? OR pension_id LIKE ? OR application_id LIKE ? OR first_name LIKE ? OR middle_name LIKE ? OR last_name LIKE ? OR unit LIKE ? OR rank LIKE ? OR rank_in LIKE ? OR rank_out LIKE ? OR pension_state LIKE ? OR buried_in LIKE ? OR maiden_name LIKE ?
	`, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := conn.Query(`
		SELECT `+soldierSelectColumns+`
		FROM soldiers
		WHERE display_id LIKE ? OR pension_id LIKE ? OR application_id LIKE ? OR first_name LIKE ? OR middle_name LIKE ? OR last_name LIKE ? OR unit LIKE ? OR rank LIKE ? OR rank_in LIKE ? OR rank_out LIKE ? OR pension_state LIKE ? OR buried_in LIKE ? OR maiden_name LIKE ?
		ORDER BY last_name, first_name
		LIMIT ? OFFSET ?
	`, append(args, pageSize, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	soldiers, err := scanSoldiers(rows)
	return soldiers, total, err
}

func (s *SoldierService) AdvancedSearch(search models.SoldierSearch, page, pageSize int) ([]models.Soldier, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}

	search.DisplayID = strings.TrimSpace(search.DisplayID)
	search.FirstName = strings.TrimSpace(search.FirstName)
	search.MiddleName = strings.TrimSpace(search.MiddleName)
	search.LastName = strings.TrimSpace(search.LastName)
	search.Rank = strings.TrimSpace(search.Rank)
	search.Unit = strings.TrimSpace(search.Unit)
	search.PensionState = strings.TrimSpace(search.PensionState)
	search.BuriedIn = strings.TrimSpace(search.BuriedIn)
	search.BirthDate = strings.TrimSpace(search.BirthDate)
	search.DeathDate = strings.TrimSpace(search.DeathDate)
	search.DeathYear = strings.TrimSpace(search.DeathYear)
	search.DeathMonth = strings.TrimSpace(search.DeathMonth)
	search.DeathDay = strings.TrimSpace(search.DeathDay)

	whereParts := []string{}
	args := []interface{}{}

	if search.DisplayID != "" {
		whereParts = append(whereParts, "display_id LIKE ?")
		args = append(args, "%"+search.DisplayID+"%")
	}
	if search.FirstName != "" {
		whereParts = append(whereParts, "first_name LIKE ?")
		args = append(args, "%"+search.FirstName+"%")
	}
	if search.MiddleName != "" {
		whereParts = append(whereParts, "middle_name LIKE ?")
		args = append(args, "%"+search.MiddleName+"%")
	}
	if search.LastName != "" {
		whereParts = append(whereParts, "last_name LIKE ?")
		args = append(args, "%"+search.LastName+"%")
	}
	if search.Rank != "" {
		whereParts = append(whereParts, "(rank LIKE ? OR rank_in LIKE ? OR rank_out LIKE ?)")
		args = append(args, "%"+search.Rank+"%", "%"+search.Rank+"%", "%"+search.Rank+"%")
	}
	if search.Unit != "" {
		whereParts = append(whereParts, "unit LIKE ?")
		args = append(args, "%"+search.Unit+"%")
	}
	if search.PensionState != "" {
		whereParts = append(whereParts, "pension_state = ?")
		args = append(args, search.PensionState)
	}
	if search.BuriedIn != "" {
		whereParts = append(whereParts, "buried_in LIKE ?")
		args = append(args, "%"+search.BuriedIn+"%")
	}
	if search.BirthDate != "" {
		normalized, err := dates.NormalizeCanonical(search.BirthDate)
		if err != nil {
			return nil, 0, fmt.Errorf("invalid birth_date")
		}
		whereParts = append(whereParts, "birth_date = ?")
		args = append(args, normalized)
	}
	if search.DeathDate != "" {
		normalized, err := dates.NormalizeCanonical(search.DeathDate)
		if err != nil {
			return nil, 0, fmt.Errorf("invalid death_date")
		}
		whereParts = append(whereParts, "death_date = ?")
		args = append(args, normalized)
	}

	exactFilters := []struct {
		value  string
		field  string
		column string
	}{
		{search.DeathYear, "death_year", "death_year"},
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
		"SELECT "+soldierSelectColumns+" FROM soldiers WHERE "+whereClause+" ORDER BY last_name, first_name LIMIT ? OFFSET ?",
		append(args, pageSize, offset)...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	soldiers, err := scanSoldiers(rows)
	return soldiers, total, err
}

func (s *SoldierService) List(page, pageSize int) ([]models.Soldier, int, error) {
	conn := s.db.Conn()
	var total int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM soldiers`).Scan(&total); err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	rows, err := conn.Query(`SELECT `+soldierSelectColumns+` FROM soldiers ORDER BY last_name, first_name LIMIT ? OFFSET ?`, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	soldiers, err := scanSoldiers(rows)
	return soldiers, total, err
}

func scanSoldier(row *sql.Row) (*models.Soldier, error) {
	var s models.Soldier
	err := row.Scan(soldierScanDest(&s)...)
	if err != nil {
		return nil, err
	}
	hydrateLegacyDeathParts(&s)
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
		soldiers = append(soldiers, s)
	}
	if soldiers == nil {
		soldiers = []models.Soldier{}
	}
	return soldiers, rows.Err()
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
		displayID       sql.NullString
		syncID          sql.NullString
		entryType       sql.NullString
		maidenName      sql.NullString
		spouseSoldierID sql.NullInt64
		pensionID       sql.NullString
		applicationID   sql.NullString
		firstName       sql.NullString
		middleName      sql.NullString
		lastName        sql.NullString
		rank            sql.NullString
		rankIn          sql.NullString
		rankOut         sql.NullString
		unit            sql.NullString
		pensionState    sql.NullString
		birthInfo       sql.NullString
		buriedIn        sql.NullString
		notes           sql.NullString
		createdAt       sql.NullString
		deathYear       sql.NullInt64
		deathMonth      sql.NullInt64
		deathDay        sql.NullInt64
		birthDate       sql.NullString
		deathDate       sql.NullString
		updatedAt       sql.NullString
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
		nullStringDest(&s.FirstName, &firstName),
		nullStringDest(&s.MiddleName, &middleName),
		nullStringDest(&s.LastName, &lastName),
		nullStringDest(&s.Rank, &rank),
		nullStringDest(&s.RankIn, &rankIn),
		nullStringDest(&s.RankOut, &rankOut),
		nullStringDest(&s.Unit, &unit),
		nullStringDest(&s.PensionState, &pensionState),
		nullIntDest(&s.DeathYear, &deathYear),
		nullIntDest(&s.DeathMonth, &deathMonth),
		nullIntDest(&s.DeathDay, &deathDay),
		nullStringDest(&s.BirthDate, &birthDate),
		nullStringDest(&s.DeathDate, &deathDate),
		nullStringDest(&s.BirthInfo, &birthInfo),
		nullStringDest(&s.BuriedIn, &buriedIn),
		nullStringDest(&s.Notes, &notes),
		nullStringDest(&s.CreatedAt, &createdAt),
		nullStringDest(&s.UpdatedAt, &updatedAt),
	}
}

func normalizeSoldierEntry(tx *sql.Tx, soldier *models.Soldier) error {
	soldier.EntryType = normalizeEntryType(soldier.EntryType)
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

func spouseReference(conn *sql.DB, spouseSoldierID int64) string {
	if spouseSoldierID < 1 {
		return ""
	}
	var fullName string
	if err := conn.QueryRow(`SELECT trim(coalesce(first_name, '') || ' ' || coalesce(last_name, '')) FROM soldiers WHERE id = ?`, spouseSoldierID).Scan(&fullName); err == nil && strings.TrimSpace(fullName) != "" {
		return fullName
	}
	var displayID string
	if err := conn.QueryRow(`SELECT display_id FROM soldiers WHERE id = ?`, spouseSoldierID).Scan(&displayID); err == nil {
		return strings.TrimSpace(displayID)
	}
	return ""
}

func normalizeDisplayID(displayID, nodePrefix string) string {
	trimmed := strings.TrimSpace(displayID)
	if trimmed == "" {
		return ""
	}
	prefix := db.NormalizeNodePrefix(nodePrefix)
	if strings.HasPrefix(trimmed, prefix+"-") {
		return trimmed
	}
	return prefix + "-" + trimmed
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
	row := tx.QueryRow(`SELECT sync_id, created_at FROM soldiers WHERE id = ?`, soldier.ID)
	var currentSyncID sql.NullString
	var createdAt sql.NullString
	if err := row.Scan(&currentSyncID, &createdAt); err != nil {
		return err
	}
	if strings.TrimSpace(soldier.SyncID) == "" {
		soldier.SyncID = currentSyncID.String
	}
	if strings.TrimSpace(soldier.CreatedAt) == "" {
		soldier.CreatedAt = createdAt.String
	}
	return nil
}

func (s *SoldierService) soldierSyncIDByID(soldierID int64) (string, error) {
	var syncID string
	if err := s.db.Conn().QueryRow(`SELECT sync_id FROM soldiers WHERE id = ?`, soldierID).Scan(&syncID); err != nil {
		return "", err
	}
	return syncID, nil
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
