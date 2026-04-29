package db

import "strings"

const schema = `
CREATE TABLE IF NOT EXISTS soldiers (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    display_id   TEXT UNIQUE NOT NULL,
    is_generated BOOLEAN DEFAULT 0,
    first_name   TEXT,
    last_name    TEXT,
    rank         TEXT,
    unit         TEXT,
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
	return nil
}
