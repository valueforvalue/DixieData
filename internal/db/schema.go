package db

import (
	"database/sql"
	"fmt"

	"github.com/valueforvalue/DixieData/internal/buildinfo"
)

const schema = `
CREATE TABLE IF NOT EXISTS soldiers (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    display_id   TEXT UNIQUE NOT NULL,
    sync_id      TEXT,
    is_generated BOOLEAN DEFAULT 0,
    pension_id   TEXT,
    application_id TEXT,
    first_name   TEXT,
    middle_name  TEXT,
    last_name    TEXT,
    rank         TEXT,
    rank_in      TEXT,
    rank_out     TEXT,
    unit         TEXT,
    pension_state TEXT,
    death_year   INTEGER,
    death_month  INTEGER,
    death_day    INTEGER,
    birth_date   TEXT,
    death_date   TEXT,
    birth_info   TEXT,
    buried_in    TEXT,
    notes        TEXT,
    created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME
);

CREATE TABLE IF NOT EXISTS records (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    soldier_id   INTEGER REFERENCES soldiers(id) ON DELETE CASCADE,
    record_type  TEXT,
    app_id       TEXT,
    details      TEXT
);

CREATE TABLE IF NOT EXISTS images (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    soldier_id   INTEGER REFERENCES soldiers(id) ON DELETE CASCADE,
    file_name    TEXT,
    file_path    TEXT,
    caption      TEXT
);

CREATE INDEX IF NOT EXISTS idx_soldiers_death ON soldiers(death_month, death_day);

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
SET display_id = 'TDM65-' || display_id
WHERE display_id IS NOT NULL
  AND TRIM(display_id) <> ''
  AND display_id NOT LIKE 'TDM65-%';

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
SET sync_id = lower(
    hex(randomblob(4)) || '-' ||
    hex(randomblob(2)) || '-' ||
    '7' || substr(hex(randomblob(2)), 2) || '-' ||
    substr('89ab', (abs(random()) % 4) + 1, 1) || substr(hex(randomblob(2)), 2) || '-' ||
    hex(randomblob(6))
)
WHERE sync_id IS NULL OR TRIM(sync_id) = '';

INSERT INTO system_config(key, value)
VALUES ('node_prefix', 'TDM65')
ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP;

INSERT INTO system_config(key, value)
SELECT
    'node_id',
    lower(
        hex(randomblob(4)) || '-' ||
        hex(randomblob(2)) || '-' ||
        '7' || substr(hex(randomblob(2)), 2) || '-' ||
        substr('89ab', (abs(random()) % 4) + 1, 1) || substr(hex(randomblob(2)), 2) || '-' ||
        hex(randomblob(6))
    )
WHERE NOT EXISTS (SELECT 1 FROM system_config WHERE key = 'node_id');

CREATE UNIQUE INDEX IF NOT EXISTS idx_soldiers_sync_id ON soldiers(sync_id);
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
		{table: "soldiers", column: "middle_name", sql: `ALTER TABLE soldiers ADD COLUMN middle_name TEXT`},
		{table: "soldiers", column: "rank_in", sql: `ALTER TABLE soldiers ADD COLUMN rank_in TEXT`},
		{table: "soldiers", column: "rank_out", sql: `ALTER TABLE soldiers ADD COLUMN rank_out TEXT`},
		{table: "soldiers", column: "pension_state", sql: `ALTER TABLE soldiers ADD COLUMN pension_state TEXT`},
		{table: "soldiers", column: "sync_id", sql: `ALTER TABLE soldiers ADD COLUMN sync_id TEXT`},
		{table: "soldiers", column: "birth_date", sql: `ALTER TABLE soldiers ADD COLUMN birth_date TEXT`},
		{table: "soldiers", column: "death_date", sql: `ALTER TABLE soldiers ADD COLUMN death_date TEXT`},
		{table: "soldiers", column: "updated_at", sql: `ALTER TABLE soldiers ADD COLUMN updated_at DATETIME`},
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
