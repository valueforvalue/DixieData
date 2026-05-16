package db

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/dates"
)

const schema = `
CREATE TABLE IF NOT EXISTS soldiers (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    display_id   TEXT UNIQUE NOT NULL,
    sync_id      TEXT,
    entry_type   TEXT NOT NULL DEFAULT 'soldier',
    spouse_soldier_id INTEGER REFERENCES soldiers(id) ON DELETE SET NULL,
    maiden_name  TEXT,
    is_generated BOOLEAN DEFAULT 0,
    pension_id   TEXT,
    application_id TEXT,
    prefix       TEXT,
    first_name   TEXT,
    middle_name  TEXT,
    last_name    TEXT,
    suffix       TEXT,
    rank         TEXT,
    rank_in      TEXT,
    rank_out     TEXT,
    unit         TEXT,
    pension_state TEXT,
    confederate_home_status TEXT DEFAULT 'None',
    confederate_home_name TEXT,
    death_year   INTEGER,
    death_month  INTEGER,
    death_day    INTEGER,
    birth_date   TEXT,
    death_date   TEXT,
    birth_info   TEXT,
    buried_in    TEXT,
    notes        TEXT,
    needs_review BOOLEAN DEFAULT 0,
    review_reason TEXT,
    added_by     TEXT,
    last_edited_by TEXT,
    last_edited_fields TEXT,
    last_edited_at DATETIME,
    created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME
);

CREATE TABLE IF NOT EXISTS records (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    sync_id      TEXT,
    soldier_id   INTEGER REFERENCES soldiers(id) ON DELETE CASCADE,
    soldier_sync_id TEXT,
    record_type  TEXT,
    app_id       TEXT,
    details      TEXT
);

CREATE TABLE IF NOT EXISTS images (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    sync_id      TEXT,
    soldier_id   INTEGER REFERENCES soldiers(id) ON DELETE CASCADE,
    soldier_sync_id TEXT,
    file_name    TEXT,
    file_path    TEXT,
    caption      TEXT,
    is_primary   BOOLEAN DEFAULT 0
);

CREATE TABLE IF NOT EXISTS merge_review_sessions (
    id           TEXT PRIMARY KEY,
    archive_path TEXT NOT NULL,
    source_root  TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'open',
    created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME
);

CREATE TABLE IF NOT EXISTS merge_review_conflicts (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id       TEXT NOT NULL REFERENCES merge_review_sessions(id) ON DELETE CASCADE,
    conflict_type    TEXT NOT NULL,
    reason           TEXT NOT NULL,
    soldier_sync_id  TEXT NOT NULL,
    local_soldier_id INTEGER,
    local_display_id TEXT,
    source_display_id TEXT NOT NULL,
    local_data       TEXT,
    source_data      TEXT NOT NULL,
    resolution       TEXT,
    created_at       DATETIME DEFAULT CURRENT_TIMESTAMP,
    resolved_at      DATETIME
);

CREATE TABLE IF NOT EXISTS duplicate_audit_findings (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    pair_key         TEXT UNIQUE NOT NULL,
    left_soldier_id  INTEGER NOT NULL REFERENCES soldiers(id) ON DELETE CASCADE,
    right_soldier_id INTEGER NOT NULL REFERENCES soldiers(id) ON DELETE CASCADE,
    finding_type     TEXT NOT NULL,
    reason           TEXT NOT NULL,
    highlight_fields TEXT NOT NULL,
    status           TEXT NOT NULL DEFAULT 'open',
    created_at       DATETIME DEFAULT CURRENT_TIMESTAMP,
    last_detected_at DATETIME,
    resolved_at      DATETIME
);

CREATE INDEX IF NOT EXISTS idx_soldiers_death ON soldiers(death_month, death_day);
CREATE INDEX IF NOT EXISTS idx_merge_review_conflicts_session ON merge_review_conflicts(session_id);
CREATE INDEX IF NOT EXISTS idx_merge_review_conflicts_resolution ON merge_review_conflicts(resolution);
CREATE INDEX IF NOT EXISTS idx_duplicate_audit_findings_status ON duplicate_audit_findings(status);
CREATE INDEX IF NOT EXISTS idx_duplicate_audit_findings_left ON duplicate_audit_findings(left_soldier_id);
CREATE INDEX IF NOT EXISTS idx_duplicate_audit_findings_right ON duplicate_audit_findings(right_soldier_id);

CREATE VIRTUAL TABLE IF NOT EXISTS soldiers_fts USING fts5(
    first_name, last_name, unit, soldier_rank,
    content=soldiers, content_rowid=id
);
`

const phase1DistributedMergeMigration = `
CREATE TABLE IF NOT EXISTS system_config (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

UPDATE soldiers
SET birth_date = '00/00/0000'
WHERE birth_date IS NULL OR TRIM(birth_date) = '';

UPDATE soldiers
SET death_date = printf('%02d/%02d/%04d', COALESCE(death_month, 0), COALESCE(death_day, 0), COALESCE(death_year, 0))
WHERE death_date IS NULL OR TRIM(death_date) = '';

UPDATE soldiers
SET updated_at = COALESCE(NULLIF(created_at, ''), CURRENT_TIMESTAMP)
WHERE updated_at IS NULL OR TRIM(updated_at) = '';

UPDATE soldiers
SET sync_id = ` + syncIDSQL + `
WHERE sync_id IS NULL OR TRIM(sync_id) = '';

INSERT INTO system_config(key, value)
SELECT 'node_prefix', 'DXD'
WHERE NOT EXISTS (SELECT 1 FROM system_config WHERE key = 'node_prefix');

INSERT INTO system_config(key, value)
SELECT
    'node_id',
    ` + syncIDSQL + `
WHERE NOT EXISTS (SELECT 1 FROM system_config WHERE key = 'node_id');

CREATE UNIQUE INDEX IF NOT EXISTS idx_soldiers_sync_id ON soldiers(sync_id);

UPDATE records
SET soldier_sync_id = (
    SELECT soldiers.sync_id
    FROM soldiers
    WHERE soldiers.id = records.soldier_id
)
WHERE soldier_sync_id IS NULL OR TRIM(soldier_sync_id) = '';

UPDATE records
SET sync_id = ` + syncIDSQL + `
WHERE sync_id IS NULL OR TRIM(sync_id) = '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_records_sync_id ON records(sync_id);
CREATE INDEX IF NOT EXISTS idx_records_soldier_sync_id ON records(soldier_sync_id);

UPDATE images
SET soldier_sync_id = (
    SELECT soldiers.sync_id
    FROM soldiers
    WHERE soldiers.id = images.soldier_id
)
WHERE soldier_sync_id IS NULL OR TRIM(soldier_sync_id) = '';

UPDATE images
SET sync_id = ` + syncIDSQL + `
WHERE sync_id IS NULL OR TRIM(sync_id) = '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_images_sync_id ON images(sync_id);
CREATE INDEX IF NOT EXISTS idx_images_soldier_sync_id ON images(soldier_sync_id);
`

const phase2CanonicalDatesMigration = `
UPDATE soldiers
SET entry_type = 'soldier'
WHERE entry_type IS NULL OR TRIM(entry_type) = '';

UPDATE soldiers
SET death_date = CASE
    WHEN COALESCE(death_year, 0) = 0 AND COALESCE(death_month, 0) = 0 AND COALESCE(death_day, 0) = 0 THEN ''
    ELSE printf('%02d/%02d/%04d', COALESCE(death_month, 0), COALESCE(death_day, 0), COALESCE(death_year, 0))
END
WHERE death_date IS NULL OR TRIM(death_date) = '';

UPDATE soldiers
SET birth_date = ''
WHERE birth_date = '00/00/0000';
`

func applySchema(db *DB) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(schema); err != nil {
		return err
	}
	for _, migration := range []struct {
		table  string
		column string
		sql    string
	}{
		{table: "soldiers", column: "buried_in", sql: `ALTER TABLE soldiers ADD COLUMN buried_in TEXT`},
		{table: "soldiers", column: "pension_id", sql: `ALTER TABLE soldiers ADD COLUMN pension_id TEXT`},
		{table: "soldiers", column: "application_id", sql: `ALTER TABLE soldiers ADD COLUMN application_id TEXT`},
		{table: "soldiers", column: "prefix", sql: `ALTER TABLE soldiers ADD COLUMN prefix TEXT`},
		{table: "soldiers", column: "middle_name", sql: `ALTER TABLE soldiers ADD COLUMN middle_name TEXT`},
		{table: "soldiers", column: "suffix", sql: `ALTER TABLE soldiers ADD COLUMN suffix TEXT`},
		{table: "soldiers", column: "rank_in", sql: `ALTER TABLE soldiers ADD COLUMN rank_in TEXT`},
		{table: "soldiers", column: "rank_out", sql: `ALTER TABLE soldiers ADD COLUMN rank_out TEXT`},
		{table: "soldiers", column: "pension_state", sql: `ALTER TABLE soldiers ADD COLUMN pension_state TEXT`},
		{table: "soldiers", column: "confederate_home_status", sql: `ALTER TABLE soldiers ADD COLUMN confederate_home_status TEXT DEFAULT 'None'`},
		{table: "soldiers", column: "confederate_home_name", sql: `ALTER TABLE soldiers ADD COLUMN confederate_home_name TEXT`},
		{table: "soldiers", column: "sync_id", sql: `ALTER TABLE soldiers ADD COLUMN sync_id TEXT`},
		{table: "soldiers", column: "entry_type", sql: `ALTER TABLE soldiers ADD COLUMN entry_type TEXT NOT NULL DEFAULT 'soldier'`},
		{table: "soldiers", column: "spouse_soldier_id", sql: `ALTER TABLE soldiers ADD COLUMN spouse_soldier_id INTEGER REFERENCES soldiers(id) ON DELETE SET NULL`},
		{table: "soldiers", column: "maiden_name", sql: `ALTER TABLE soldiers ADD COLUMN maiden_name TEXT`},
		{table: "soldiers", column: "birth_date", sql: `ALTER TABLE soldiers ADD COLUMN birth_date TEXT`},
		{table: "soldiers", column: "death_date", sql: `ALTER TABLE soldiers ADD COLUMN death_date TEXT`},
		{table: "soldiers", column: "needs_review", sql: `ALTER TABLE soldiers ADD COLUMN needs_review BOOLEAN DEFAULT 0`},
		{table: "soldiers", column: "review_reason", sql: `ALTER TABLE soldiers ADD COLUMN review_reason TEXT`},
		{table: "soldiers", column: "added_by", sql: `ALTER TABLE soldiers ADD COLUMN added_by TEXT`},
		{table: "soldiers", column: "last_edited_by", sql: `ALTER TABLE soldiers ADD COLUMN last_edited_by TEXT`},
		{table: "soldiers", column: "last_edited_fields", sql: `ALTER TABLE soldiers ADD COLUMN last_edited_fields TEXT`},
		{table: "soldiers", column: "last_edited_at", sql: `ALTER TABLE soldiers ADD COLUMN last_edited_at DATETIME`},
		{table: "soldiers", column: "updated_at", sql: `ALTER TABLE soldiers ADD COLUMN updated_at DATETIME`},
		{table: "records", column: "sync_id", sql: `ALTER TABLE records ADD COLUMN sync_id TEXT`},
		{table: "records", column: "soldier_sync_id", sql: `ALTER TABLE records ADD COLUMN soldier_sync_id TEXT`},
		{table: "images", column: "sync_id", sql: `ALTER TABLE images ADD COLUMN sync_id TEXT`},
		{table: "images", column: "soldier_sync_id", sql: `ALTER TABLE images ADD COLUMN soldier_sync_id TEXT`},
		{table: "images", column: "is_primary", sql: `ALTER TABLE images ADD COLUMN is_primary BOOLEAN DEFAULT 0`},
	} {
		exists, err := columnExists(tx, migration.table, migration.column)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := tx.Exec(migration.sql); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE soldiers SET is_generated = 1 WHERE is_generated = 0 AND display_id GLOB 'DXD-[0-9][0-9][0-9][0-9][0-9]'`); err != nil {
		return err
	}
	if _, err := tx.Exec(phase1DistributedMergeMigration); err != nil {
		return err
	}
	if _, err := tx.Exec(phase2CanonicalDatesMigration); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE soldiers SET confederate_home_status = 'None' WHERE confederate_home_status IS NULL OR TRIM(confederate_home_status) = ''`); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE soldiers SET needs_review = 0 WHERE needs_review IS NULL`); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE soldiers SET review_reason = '' WHERE review_reason IS NULL`); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE soldiers SET confederate_home_name = '' WHERE confederate_home_name IS NULL`); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE soldiers SET confederate_home_name = '' WHERE confederate_home_status = 'None'`); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE soldiers SET last_edited_at = COALESCE(NULLIF(updated_at, ''), NULLIF(created_at, ''), CURRENT_TIMESTAMP) WHERE last_edited_at IS NULL OR TRIM(last_edited_at) = ''`); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE images SET is_primary = 0 WHERE is_primary IS NULL`); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE images SET is_primary = 1 WHERE id IN (
		SELECT MIN(id)
		FROM images
		GROUP BY soldier_id
		HAVING MAX(CASE WHEN is_primary = 1 THEN 1 ELSE 0 END) = 0
	)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_soldiers_spouse ON soldiers(spouse_soldier_id)`); err != nil {
		return err
	}
	if err := migrateNodePrefixConfiguration(tx); err != nil {
		return err
	}
	if err := migrateSanitizedDisplayIDs(tx); err != nil {
		return err
	}
	if err := migrateCanonicalDateData(tx); err != nil {
		return err
	}
	if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, buildinfo.SchemaVersion)); err != nil {
		return err
	}

	return tx.Commit()
}

func columnExists(tx *sql.Tx, table, column string) (bool, error) {
	var query string
	switch table {
	case "soldiers":
		query = `PRAGMA table_info(soldiers)`
	case "records":
		query = `PRAGMA table_info(records)`
	case "images":
		query = `PRAGMA table_info(images)`
	case "system_config":
		query = `PRAGMA table_info(system_config)`
	case "merge_review_sessions":
		query = `PRAGMA table_info(merge_review_sessions)`
	case "merge_review_conflicts":
		query = `PRAGMA table_info(merge_review_conflicts)`
	case "duplicate_audit_findings":
		query = `PRAGMA table_info(duplicate_audit_findings)`
	default:
		return false, fmt.Errorf("unsupported table for schema introspection: %s", table)
	}
	rows, err := tx.Query(query)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid        int
			name       string
			dataType   string
			notNull    int
			defaultVal interface{}
			pk         int
		)
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultVal, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

func migrateNodePrefixConfiguration(tx *sql.Tx) error {
	var (
		complete   sql.NullString
		firstName  sql.NullString
		middleName sql.NullString
		lastName   sql.NullString
		birthYear  sql.NullString
	)
	if err := tx.QueryRow(`SELECT value FROM system_config WHERE key = 'user_identity_complete'`).Scan(&complete); err != nil && err != sql.ErrNoRows {
		return err
	}
	if strings.TrimSpace(complete.String) != "1" {
		return nil
	}
	if err := tx.QueryRow(`SELECT value FROM system_config WHERE key = 'user_first_name'`).Scan(&firstName); err != nil && err != sql.ErrNoRows {
		return err
	}
	if err := tx.QueryRow(`SELECT value FROM system_config WHERE key = 'user_middle_name'`).Scan(&middleName); err != nil && err != sql.ErrNoRows {
		return err
	}
	if err := tx.QueryRow(`SELECT value FROM system_config WHERE key = 'user_last_name'`).Scan(&lastName); err != nil && err != sql.ErrNoRows {
		return err
	}
	if err := tx.QueryRow(`SELECT value FROM system_config WHERE key = 'user_birth_year'`).Scan(&birthYear); err != nil && err != sql.ErrNoRows {
		return err
	}
	parsedBirthYear, err := strconv.Atoi(strings.TrimSpace(birthYear.String))
	if err != nil {
		return nil
	}
	nodePrefix, err := BuildUserNodePrefix(firstName.String, middleName.String, lastName.String, parsedBirthYear)
	if err != nil {
		return nil
	}
	_, err = tx.Exec(`
		INSERT INTO system_config(key, value)
		VALUES ('node_prefix', ?)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = CURRENT_TIMESTAMP
	`, nodePrefix)
	return err
}

func migrateSanitizedDisplayIDs(tx *sql.Tx) error {
	type displayRecord struct {
		id        int64
		displayID string
	}

	nodePrefix := NormalizeNodePrefix("")
	var storedPrefix sql.NullString
	if err := tx.QueryRow(`SELECT value FROM system_config WHERE key = 'node_prefix'`).Scan(&storedPrefix); err != nil && err != sql.ErrNoRows {
		return err
	}
	if strings.TrimSpace(storedPrefix.String) != "" {
		nodePrefix = NormalizeNodePrefix(storedPrefix.String)
	}

	rows, err := tx.Query(`SELECT id, display_id FROM soldiers ORDER BY id`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var (
		records []displayRecord
		maxSeq  int
		taken   = map[string]int64{}
	)
	for rows.Next() {
		var record displayRecord
		if err := rows.Scan(&record.id, &record.displayID); err != nil {
			return err
		}
		records = append(records, record)
		taken[record.displayID] = record.id
		if _, sequence, ok := CanonicalDisplayID(record.displayID); ok && sequence > maxSeq {
			maxSeq = sequence
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, record := range records {
		sanitized := SanitizeID(record.displayID, nodePrefix)
		if sanitized == record.displayID {
			continue
		}
		if existingID, exists := taken[sanitized]; exists && existingID != record.id {
			maxSeq++
			sanitized = NextGeneratedDisplayID(nodePrefix, maxSeq)
		}
		if _, err := tx.Exec(`UPDATE soldiers SET display_id = ? WHERE id = ?`, sanitized, record.id); err != nil {
			return err
		}
		delete(taken, record.displayID)
		taken[sanitized] = record.id
	}
	return nil
}

func migrateCanonicalDateData(tx *sql.Tx) error {
	rows, err := tx.Query(`SELECT id, birth_date, birth_info, death_date, death_year, death_month, death_day FROM soldiers`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type updateRow struct {
		id        int64
		birthDate string
		deathDate string
		year      int
		month     int
		day       int
	}
	updates := []updateRow{}
	for rows.Next() {
		var (
			id         int64
			birthDate  sql.NullString
			birthInfo  sql.NullString
			deathDate  sql.NullString
			deathYear  sql.NullInt64
			deathMonth sql.NullInt64
			deathDay   sql.NullInt64
		)
		if err := rows.Scan(&id, &birthDate, &birthInfo, &deathDate, &deathYear, &deathMonth, &deathDay); err != nil {
			return err
		}

		normalizedBirth := strings.TrimSpace(birthDate.String)
		if normalizedBirth == "00/00/0000" || normalizedBirth == "" {
			normalizedBirth = dates.ParseBirthInfo(strings.TrimSpace(birthInfo.String))
		}
		normalizedBirth, err = dates.NormalizeCanonical(normalizedBirth)
		if err != nil {
			normalizedBirth = ""
		}

		normalizedDeath := strings.TrimSpace(deathDate.String)
		if normalizedDeath == "" {
			normalizedDeath = dates.MustFormat(int(deathMonth.Int64), int(deathDay.Int64), int(deathYear.Int64))
		}
		normalizedDeath, err = dates.NormalizeCanonical(normalizedDeath)
		if err != nil {
			normalizedDeath = ""
		}
		partialDeath, err := dates.ParseCanonical(normalizedDeath)
		if err != nil {
			partialDeath = dates.PartialDate{}
		}

		updates = append(updates, updateRow{
			id:        id,
			birthDate: normalizedBirth,
			deathDate: normalizedDeath,
			year:      partialDeath.Year,
			month:     partialDeath.Month,
			day:       partialDeath.Day,
		})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, update := range updates {
		if _, err := tx.Exec(`UPDATE soldiers SET birth_date = ?, death_date = ?, death_year = ?, death_month = ?, death_day = ? WHERE id = ?`,
			update.birthDate, update.deathDate, update.year, update.month, update.day, update.id); err != nil {
			return err
		}
	}
	return nil
}
