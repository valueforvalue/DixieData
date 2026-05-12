package db

import (
	"fmt"
	"strings"

	"github.com/valueforvalue/DixieData/internal/buildinfo"
)

const schema = `
CREATE TABLE IF NOT EXISTS soldiers (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    display_id   TEXT UNIQUE NOT NULL,
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
    birth_info   TEXT,
    buried_in    TEXT,
    notes        TEXT,
    created_at   DATETIME DEFAULT CURRENT_TIMESTAMP
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

func applySchema(db *DB) error {
	if _, err := db.conn.Exec(schema); err != nil {
		return err
	}
	if _, err := db.conn.Exec(`ALTER TABLE soldiers ADD COLUMN buried_in TEXT`); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
		return err
	}
	for _, statement := range []string{
		`ALTER TABLE soldiers ADD COLUMN pension_id TEXT`,
		`ALTER TABLE soldiers ADD COLUMN application_id TEXT`,
		`ALTER TABLE soldiers ADD COLUMN middle_name TEXT`,
		`ALTER TABLE soldiers ADD COLUMN rank_in TEXT`,
		`ALTER TABLE soldiers ADD COLUMN rank_out TEXT`,
		`ALTER TABLE soldiers ADD COLUMN pension_state TEXT`,
	} {
		if _, err := db.conn.Exec(statement); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
			return err
		}
	}
	if _, err := db.conn.Exec(`UPDATE soldiers SET is_generated = 1 WHERE is_generated = 0 AND display_id GLOB 'DXD-[0-9][0-9][0-9][0-9][0-9]'`); err != nil {
		return err
	}
	if _, err := db.conn.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, buildinfo.SchemaVersion)); err != nil {
		return err
	}
	return nil
}
