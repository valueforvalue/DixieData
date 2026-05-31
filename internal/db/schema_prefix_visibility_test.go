package db

import (
	"database/sql"
	"testing"
)

func TestOpenMigratesShowPrefixBeforeNameToHiddenByDefault(t *testing.T) {
	dataDir := t.TempDir()
	legacyConn, err := sql.Open("sqlite", Path(dataDir))
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { legacyConn.Close() })

	if _, err := legacyConn.Exec(`
CREATE TABLE schema_version (
	version    INTEGER PRIMARY KEY,
	applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE soldiers (
	id                    INTEGER PRIMARY KEY AUTOINCREMENT,
	display_id            TEXT UNIQUE NOT NULL,
	sync_id               TEXT,
	entry_type            TEXT NOT NULL DEFAULT 'soldier',
	spouse_soldier_id     INTEGER REFERENCES soldiers(id) ON DELETE SET NULL,
	relationship_label    TEXT,
	maiden_name           TEXT,
	is_generated          BOOLEAN DEFAULT 0,
	pension_id            TEXT,
	application_id        TEXT,
	prefix                TEXT,
	first_name            TEXT,
	middle_name           TEXT,
	last_name             TEXT,
	suffix                TEXT,
	rank                  TEXT,
	rank_in               TEXT,
	rank_out              TEXT,
	unit                  TEXT,
	pension_state         TEXT,
	confederate_home_status TEXT DEFAULT 'None',
	confederate_home_name TEXT,
	death_year            INTEGER,
	death_month           INTEGER,
	death_day             INTEGER,
	birth_date            TEXT,
	death_date            TEXT,
	birth_info            TEXT,
	buried_in             TEXT,
	notes                 TEXT,
	needs_review          BOOLEAN DEFAULT 0,
	review_reason         TEXT,
	added_by              TEXT,
	last_edited_by        TEXT,
	last_edited_fields    TEXT,
	last_edited_at        DATETIME,
	created_at            DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at            DATETIME
);

CREATE TABLE records (
	id              INTEGER PRIMARY KEY AUTOINCREMENT,
	sync_id         TEXT,
	soldier_id      INTEGER REFERENCES soldiers(id) ON DELETE CASCADE,
	soldier_sync_id TEXT,
	record_type     TEXT,
	app_id          TEXT,
	details         TEXT
);

CREATE TABLE images (
	id              INTEGER PRIMARY KEY AUTOINCREMENT,
	sync_id         TEXT,
	soldier_id      INTEGER REFERENCES soldiers(id) ON DELETE CASCADE,
	soldier_sync_id TEXT,
	file_name       TEXT,
	file_path       TEXT,
	caption         TEXT,
	is_primary      BOOLEAN DEFAULT 0
);
`); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}
	if _, err := legacyConn.Exec(`INSERT INTO soldiers (display_id, is_generated, first_name, last_name) VALUES ('DXD-00001', 1, 'John', 'Taylor')`); err != nil {
		t.Fatalf("insert legacy soldier: %v", err)
	}
	if _, err := legacyConn.Exec(`INSERT INTO soldiers (display_id, first_name, last_name, pension_state) VALUES ('DXD-00002', 'Andrew', 'Morris', 'None')`); err != nil {
		t.Fatalf("insert legacy pension-state soldier: %v", err)
	}
	if _, err := legacyConn.Exec(`PRAGMA user_version = 23`); err != nil {
		t.Fatalf("set legacy user_version: %v", err)
	}
	if err := legacyConn.Close(); err != nil {
		t.Fatalf("Close legacy connection: %v", err)
	}

	database, err := Open(dataDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer database.Close()

	var showPrefix bool
	if err := database.Conn().QueryRow(`SELECT show_prefix_before_name FROM soldiers WHERE display_id = 'DXD-00001'`).Scan(&showPrefix); err != nil {
		t.Fatalf("read show_prefix_before_name: %v", err)
	}
	if showPrefix {
		t.Fatal("expected migrated soldiers to hide prefix by default")
	}

	var pensionState string
	if err := database.Conn().QueryRow(`SELECT pension_state FROM soldiers WHERE display_id = 'DXD-00002'`).Scan(&pensionState); err != nil {
		t.Fatalf("read pension_state: %v", err)
	}
	if pensionState != "NA" {
		t.Fatalf("expected migrated pension state to be NA, got %q", pensionState)
	}
}
