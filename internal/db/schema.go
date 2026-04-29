package db

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
    first_name, last_name, unit, rank,
    content=soldiers, content_rowid=id
);
`

func applySchema(db *DB) error {
	_, err := db.conn.Exec(schema)
	return err
}
