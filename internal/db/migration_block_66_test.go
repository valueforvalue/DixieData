// Regression net for block-66 (research_collection_items
// soldier_id → person_record_id rename).
//
// Background: the v54→v60 consolidated rename migration
// (block-60, internal/db/migrations.go:310) renamed
// soldier_id→person_record_id on 5 FK tables (records, images,
// scratchpad_cache, research_tasks, etc.) but missed
// research_collection_items. The table was added in commit
// 4645ae5 (v1.1 research workflow, May 2026) with the old
// soldier_id column name; the inline schema constant in
// internal/db/schema.go was later updated to use
// person_record_id for fresh installs, but no migration was
// ever added to rename the column on existing DBs.
//
// Symptom: a GET /research-collections request ran the
// ResearchCollectionsHub query (internal/records/soldier_service.go
// :1471) which references i.person_record_id. The DB returned
// `SQL logic error: no such column: i.person_record_id` and
// the handler 500s. The user saw the dark-slate "Internal
// server error" page. The Wails debug console log (reproduced
// 2026-07-09) showed:
//   ERROR appshell: request failed http method=GET err=SQL
//   logic error: no such column: i.person_record_id (1)
//   component=http audit=respond-error kind=internal
//   path=/research-collections
//
// These two tests pin the fix:
//
//   1. TestBlock66RenamesResearchCollectionItemsColumn — build
//      a v65-style DB (research_collection_items.soldier_id
//      present, no person_record_id), run applySchema, assert
//      the column is renamed. Verified FAIL on prior commit
//      (no rename), PASS now.
//
//   2. TestBlock66PreservesExistingRows — same setup, insert a
//      row keyed by soldier_id, run applySchema, assert the
//      row survives the rename and is now keyed by
//      person_record_id. Pins the "no data loss" property of
//      ALTER TABLE ... RENAME COLUMN.
package db

import (
	"context"
	"database/sql"
	"testing"

	"github.com/valueforvalue/DixieData/internal/testtemp"
)

// buildPreBlock66DB builds a v65-style DB in a fresh data dir.
// It runs block-1 (schema baseline) plus blocks 2..66 (which
// lands at v66 + the rename), then backs out the rename
// manually and sets user_version=65 so a subsequent Open() re-
// runs block-66 as the only forward delta.
//
// The "open, back-date, close" pattern matters: SQLite's WAL
// mode holds shared locks for the connection's lifetime, and
// the applySchema path inside Open() expects an exclusive lock
// for its PRAGMA + DDL sequence. Two concurrent Open() calls on
// the same DB file would deadlock on the WAL, so we sequence
// them.
//
// buildPreBlock66DB builds a v65-style DB in a fresh data dir
// using the raw modernc sqlite driver (no WAL pragma, no
// applySchema). The setup is: open with sql.Open, run the
// inline schema constant (CREATE TABLE IF NOT EXISTS creates
// the tables on a fresh DB), back-date the rename, set
// user_version=65, close.
//
// Why not Open(dataDir)? Open() enables WAL mode, which holds
// per-connection shared locks for the connection's lifetime.
// Calling Open() twice on the same DB file (once to back-date,
// once to run the migration) deadlocks on the WAL between the
// two calls. The migration_backup_test.go pattern uses
// sql.Open("sqlite", ...) for the legacy setup to avoid this.
// Adopting the same pattern here.
func buildPreBlock66DB(t *testing.T) (dataDir string, preHasSoldierID, preHasPersonRecordID bool) {
	t.Helper()
	dataDir = testtemp.New(t).Path()
	conn, err := sql.Open("sqlite", Path(dataDir))
	if err != nil {
		t.Fatalf("sql.Open pre-block-66 DB: %v", err)
	}
	// Apply the inline schema so the tables exist on the fresh
	// DB. CREATE TABLE IF NOT EXISTS is idempotent.
	if _, err := conn.Exec(schema); err != nil {
		conn.Close()
		t.Fatalf("apply schema: %v", err)
	}
	// Back-date the rename: person_record_id -> soldier_id on
	// research_collection_items, matching the v65 on-disk shape
	// the user is on.
	if _, err := conn.Exec(`ALTER TABLE research_collection_items RENAME COLUMN person_record_id TO soldier_id`); err != nil {
		conn.Close()
		t.Fatalf("back-date column rename: %v", err)
	}
	if _, err := conn.Exec(`PRAGMA user_version = 65`); err != nil {
		conn.Close()
		t.Fatalf("set user_version=65: %v", err)
	}
	preHasSoldierID = columnExistsOnConn(conn, "research_collection_items", "soldier_id")
	preHasPersonRecordID = columnExistsOnConn(conn, "research_collection_items", "person_record_id")
	if err := conn.Close(); err != nil {
		t.Fatalf("Close pre: %v", err)
	}
	return dataDir, preHasSoldierID, preHasPersonRecordID
}

func TestBlock66RenamesResearchCollectionItemsColumn(t *testing.T) {
	dataDir, preHasSoldierID, preHasPersonRecordID := buildPreBlock66DB(t)
	if !preHasSoldierID {
		t.Fatalf("pre-migration: expected soldier_id column on research_collection_items")
	}
	if preHasPersonRecordID {
		t.Fatalf("pre-migration: did NOT expect person_record_id column on research_collection_items")
	}

	// Run the migration via Open (which calls applySchema).
	post, err := Open(dataDir)
	if err != nil {
		t.Fatalf("Open post: %v", err)
	}
	defer post.Close()

	if columnExistsOnConn(post.Conn(), "research_collection_items", "soldier_id") {
		t.Errorf("post-migration: soldier_id column should be gone")
	}
	if !columnExistsOnConn(post.Conn(), "research_collection_items", "person_record_id") {
		t.Errorf("post-migration: person_record_id column should exist")
	}

	// user_version should now match CurrentSchemaVersion.
	var v int
	if err := post.Conn().QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if v != CurrentSchemaVersion {
		t.Errorf("user_version = %d, want %d", v, CurrentSchemaVersion)
	}
}

func TestBlock66PreservesExistingRows(t *testing.T) {
	dataDir, _, _ := buildPreBlock66DB(t)

	// Re-open the DB (the pre-build closed its connection) to
	// seed a row keyed by soldier_id in the pre-migration shape.
	// We use the raw modernc driver (not Open's WAL wrapper) so
	// the connection releases cleanly before the migration Open.
	pre, err := sql.Open("sqlite", Path(dataDir))
	if err != nil {
		t.Fatalf("sql.Open pre: %v", err)
	}
	if _, err := pre.Exec(`INSERT INTO soldiers (sync_id, display_id, first_name, last_name) VALUES ('sync-1', 'DXD-00001', 'Test', 'Soldier')`); err != nil {
		pre.Close()
		t.Fatalf("insert soldiers row: %v", err)
	}
	if _, err := pre.Exec(`INSERT INTO research_collections (name) VALUES ('Test Collection')`); err != nil {
		pre.Close()
		t.Fatalf("insert research_collections row: %v", err)
	}
	if _, err := pre.Exec(`INSERT INTO research_collection_items (collection_id, soldier_id) VALUES (1, 1)`); err != nil {
		pre.Close()
		t.Fatalf("insert research_collection_items row: %v", err)
	}
	if err := pre.Close(); err != nil {
		t.Fatalf("Close pre: %v", err)
	}

	// Run the migration.
	post, err := Open(dataDir)
	if err != nil {
		t.Fatalf("Open post: %v", err)
	}
	defer post.Close()

	// Row must survive the rename, now keyed by person_record_id.
	var count int
	if err := post.Conn().QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM research_collection_items WHERE person_record_id = 1`,
	).Scan(&count); err != nil {
		t.Fatalf("count via person_record_id: %v", err)
	}
	if count != 1 {
		t.Errorf("row should survive RENAME COLUMN: got count=%d, want 1", count)
	}
}

// columnExistsOnConn is a helper that mirrors columnExists but
// operates on a *sql.DB instead of a *sql.Tx. columnExists in
// migrations.go is private and tx-scoped.
func columnExistsOnConn(db *sql.DB, table, column string) bool {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid        int
			name       string
			ctype      string
			notnull    int
			dfltValue  sql.NullString
			pk         int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			return false
		}
		if name == column {
			return true
		}
	}
	return false
}
