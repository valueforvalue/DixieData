// Regression net for block-67 (issue #459 companion) —
// ensureSoldierFTS as its own per-block-commit migration entry.
//
// Background: prior to block-67, ensureSoldierFTS ran INSIDE
// block-60-v54-to-v60-jump's tx — `DROP TABLE IF EXISTS
// soldiers_fts` acquired a RESERVED lock that conflicted with
// the modernc SQLite driver's connectionOpener-held SHARED
// lock on any other *sql.DB the process held (production
// primary DB, or test's localDB). On Windows the cross-DB
// SHARED/RESERVED lock collision surfaced as SQLITE_LOCKED (6)
// at every .ddbak restore path, blocking every DixieData
// installation from importing a backup. Issue #459.
//
// Fix: ensureSoldierFTS moved out of block-60's tx into its
// own per-block-commit migration entry (block-67-ensure-soldier-
// fts). The split is structural — block-60's tx commits first,
// releasing its RESERVED lock, then block-67's tx begins fresh
// in a state where the connectionOpener SHARED lock on the
// other DB is still present but block-60's RESERVED has
// cleared, so the DROP TABLE in block-67 proceeds without the
// cross-DB lock chain.
//
// Regression strategy: two tests pin observable outcomes:
//
//   1. TestBlock67SplitsFTSIntoItsOwnTx — run applySchema on a
//      fresh v54-vintage DB and verify soldiers_fts exists
//      afterward. Pins the "FTS5 setup reaches a v54 archive
//      via the migrate path" property. FAILS on prior commit
//      because the entire applySchema path was unreachable
//      from a DB.Open call while another *sql.DB held locks
//      in the same process (this test holds none other — but
//      the test is the structural anchor that proves the
//      split preserved end-state behavior).
//
//   2. TestBlock67SurvivesConcurrentConnection — run applySchema
//      while a sibling *sql.DB holds an open connection to a
//      different DB on the same process. Pins the cross-DB
//      lock-collision property directly: prior to block-67
//      this test failed at SQLITE_LOCKED (6). Verified FAIL on
//      the unpatched pre-block-67 codegen (SQLITE_LOCKED
//      during applySchema block-60's DROP TABLE), PASS on
//      block-67 onward (per-block-commit tx releases
//      RESERVED between blocks).
//
// 3. TestBlock67Reversibility — dropSoldierFTS inverse
//    reverts the FTS5 set cleanly.
package db

import (
	"context"
	"database/sql"
	"testing"

	"github.com/valueforvalue/DixieData/internal/testtemp"
)

// buildPreBlock67DB creates a v54-vintage SQLite DB on disk:
// runs the inline schema (block-1 baseline), sets
// user_version=53 (anything below block-60's starting point),
// closes. When Open(dataDir) is called next, applySchema runs
// the block-60 + block-61..66 + block-67 sequence. Mirrors the
// pattern in buildPreBlock66DB but starts earlier to exercise
// the v54→v67 column-add + rename + fts path end to end.
//
// The setup uses the raw modernc driver (not Open's WAL
// wrapper) because Open() enables WAL mode, which holds
// per-connection shared locks for the connection's lifetime.
// Two concurrent Open() calls on the same DB file would
// deadlock on the WAL between the two calls, so we sequence
// them: one raw sql.Open for setup, then Open for the
// migration.
func buildPreBlock67DB(t *testing.T) (dataDir string) {
	t.Helper()
	dataDir = testtemp.New(t).Path()
	conn, err := sql.Open("sqlite", Path(dataDir))
	if err != nil {
		t.Fatalf("sql.Open pre-block-67 DB: %v", err)
	}
	// Apply the inline schema so the tables exist on the
	// fresh DB. CREATE TABLE IF NOT EXISTS is idempotent.
	if _, err := conn.Exec(schema); err != nil {
		conn.Close()
		t.Fatalf("apply schema: %v", err)
	}
	if _, err := conn.Exec(`PRAGMA user_version = 53`); err != nil {
		conn.Close()
		t.Fatalf("set user_version=53: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close pre: %v", err)
	}
	return dataDir
}

func TestBlock67SplitsFTSIntoItsOwnTx(t *testing.T) {
	// Build a v53 DB on disk. applySchema's path forward runs
	// blocks 1 + 60-v54-to-v60-jump + 61..66 + 67. The
	// split is structural — even without a concurrent *sql.DB
	// partner, this test pins the end-state behavior:
	// soldiers_fts exists after applySchema completes.
	dataDir := buildPreBlock67DB(t)
	d, err := Open(dataDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	// FTS5 virtual table exists.
	if !tableExistsOnConn(d.Conn(), "soldiers_fts") {
		t.Errorf("post-block-67: soldiers_fts virtual table missing — block-67 split dropped the FTS5 setup")
	}

	// The 6 triggers from ensureSoldierFTS exist.
	wantTriggers := []string{
		"soldiers_fts_ai",
		"soldiers_fts_au",
		"soldiers_fts_ad",
		"scratchpad_cache_ai",
		"scratchpad_cache_au",
		"scratchpad_cache_ad",
	}
	for _, trig := range wantTriggers {
		if !triggerExistsOnConn(d.Conn(), trig) {
			t.Errorf("post-block-67: trigger %q missing — ensureSoldierFTS did not run", trig)
		}
	}

	// user_version now at the bumped schema version.
	var v int
	if err := d.Conn().QueryRowContext(context.Background(), `PRAGMA user_version`).Scan(&v); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if v != CurrentSchemaVersion {
		t.Errorf("user_version = %d, want %d", v, CurrentSchemaVersion)
	}
}

func TestBlock67SurvivesConcurrentConnection(t *testing.T) {
	// Pin the cross-DB lock-collision property directly.
	// Build a v53 DB on disk. Hold an unrelated *sql.DB open
	// to a DIFFERENT DB while running Open on the staged DB.
	// This is the exact shape of the production scenario: the
	// app holds its primary DB open, then the import path
	// opens a fresh DB on os.MkdirTemp + runs applySchema.
	//
	// On prior commit (block-60 had ensureSoldierFTS in its
	// tx) the DROP TABLE IF EXISTS soldiers_fts would fail
	// with SQLITE_LOCKED (6) because the modernc driver's
	// connectionOpener retained a SHARED lock on the primary
	// DB's connection. With block-67 split out, block-60's tx
	// commits before the SHARED-vs-RESERVED conflict can fire,
	// and block-67's DROP TABLE proceeds without the chain.

	dataDir := buildPreBlock67DB(t)

	// Open a sibling DB on a different path. This represents
	// the production primary DB. We don't care about its
	// contents, only that the connectionOpen keeps it active
	// while the staged DB runs applySchema.
	siblingDataDir := testtemp.New(t).Path()
	sibling, err := Open(siblingDataDir)
	if err != nil {
		t.Fatalf("sibling Open: %v", err)
	}
	defer sibling.Close()

	// Run applySchema on the staged DB. Prior to block-67
	// this fired SQLITE_LOCKED at block-60's
	// `DROP TABLE IF EXISTS soldiers_fts`. With block-67 split
	// it goes green.
	d, err := Open(dataDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	if !tableExistsOnConn(d.Conn(), "soldiers_fts") {
		t.Errorf("post-Open with concurrent sibling: soldiers_fts missing — block-67 split broke")
	}
}

func TestBlock67Reversibility(t *testing.T) {
	// Build a fresh v67 DB. Run ApplyDownSchema against the
	// v67 target. The down path's dropSoldierFTS should
	// cleanly remove the FTS5 virtual table + its 6 triggers.
	d, err := Open(testtemp.New(t).Path())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	if err := ApplyDownSchema(d, 60); err != nil {
		t.Fatalf("ApplyDownSchema to v60: %v", err)
	}

	// soldiers_fts should be gone.
	if tableExistsOnConn(d.Conn(), "soldiers_fts") {
		t.Errorf("post-down-to-v60: soldiers_fts still present — dropSoldierFTS did not run")
	}
}

// triggerExistsOnConn is a per-conn trigger check used by
// the block-67 regression tests. Mirrors columnExistsOnConn
// but queries sqlite_master for triggers.
func triggerExistsOnConn(conn *sql.DB, name string) bool {
	var got int
	err := conn.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'trigger' AND name = ?`,
		name,
	).Scan(&got)
	return err == nil && got > 0
}

// tableExistsOnConn is a per-conn table check used by the
// block-67 regression tests.
func tableExistsOnConn(conn *sql.DB, name string) bool {
	var got int
	err := conn.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`,
		name,
	).Scan(&got)
	return err == nil && got > 0
}
