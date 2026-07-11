// Characterization tests for the ensureSoldierFTS split
// (issue #343 finding #6). Prior to the split, ensureSoldierFTS
// was a single 250-LoC function in schema.go doing three
// distinct concerns: scratchpad_cache CREATE+cleanup, FTS5
// virtual table build + bulk INSERT from soldiers, and 6
// CREATE TRIGGER statements. Down was a single dropSoldierFTS
// mirroring the same scope.
//
// After the split, each concern lives in its own helper + has
// its own inverse, and the umbrella ensureSoldierFTS /
// dropSoldierFTS compose them. Each helper is independently
// runnable on a fresh v54-vintage DB and independently
// reversible, so a future caller can re-run any subset
// without touching the others.
//
// v54+ is the supported floor (per repo migration discipline
// documented in migrations.go block-60 preamble), so the
// helpers do not target pre-v54 archives. The block-18 Down
// no-op complaint from the original issue is moot under that
// floor and is intentionally not addressed here.
//
// End-to-end behavior (block-67 Up/Down composing all three)
// is pinned by the existing block-67 regression tests. These
// three tests pin each sub-helper in isolation so the
// sub-helpers themselves can be evolved without losing the
// "you can run them standalone" property the split was meant
// to deliver.
package db

import (
	"database/sql"
	"testing"

	"github.com/valueforvalue/DixieData/internal/testtemp"
)

// ftsTriggers lists the 6 triggers ensureSoldierFTS installs
// (3 on soldiers, 3 on scratchpad_cache). The split tests
// assert this set is the union of the two sub-helpers' work.
var ftsTriggers = []string{
	"soldiers_fts_ai",
	"soldiers_fts_au",
	"soldiers_fts_ad",
	"scratchpad_cache_ai",
	"scratchpad_cache_au",
	"scratchpad_cache_ad",
}

// TestEnsureScratchpadCache pins the scratchpad_cache
// sub-helper in isolation. On a fresh v54 DB, after the
// helper runs the table exists with the documented columns
// and the DELETE cleanup has fired (no-op on an empty DB).
// dropScratchpadCache removes the table; running it twice
// is safe (idempotent DROP).
func TestEnsureScratchpadCache(t *testing.T) {
	d, err := Open(testtemp.New(t).Path())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	if err := runOnConnTx(d.Conn(), ensureScratchpadCache); err != nil {
		t.Fatalf("ensureScratchpadCache: %v", err)
	}

	if !tableExistsOnConn(d.Conn(), "scratchpad_cache") {
		t.Errorf("post-ensureScratchpadCache: scratchpad_cache missing")
	}

	// Documented columns exist. Pins the schema shape so a
	// future edit to the helper can't silently drop a column
	// the FTS5 bulk INSERT depends on.
	for _, want := range []string{"person_record_id", "scratch_pad", "updated_at"} {
		if !columnExistsOnConn(d.Conn(), "scratchpad_cache", want) {
			t.Errorf("post-ensureScratchpadCache: column %q missing", want)
		}
	}

	if err := runOnConnTx(d.Conn(), dropScratchpadCache); err != nil {
		t.Fatalf("dropScratchpadCache: %v", err)
	}
	if tableExistsOnConn(d.Conn(), "scratchpad_cache") {
		t.Errorf("post-dropScratchpadCache: scratchpad_cache still present")
	}

	// Idempotent re-drop is safe.
	if err := runOnConnTx(d.Conn(), dropScratchpadCache); err != nil {
		t.Errorf("dropScratchpadCache (re-run on absent table): %v", err)
	}
}

// TestEnsureSoldierFTSVirtualTable pins the FTS5 virtual
// table sub-helper in isolation. Requires scratchpad_cache
// to exist first (the helper's bulk INSERT LEFT JOINs it),
// so the test installs scratchpad_cache before the helper.
// The helper drops + recreates the virtual table + runs the
// bulk INSERT from soldiers + scratchpad_cache.
//
// Drop the 6 triggers before the helper runs so the
// post-condition assertion (VirtualTable helper does NOT
// install triggers) can observe a clean delta. The triggers
// are present from applySchema block-67's run on Open().
func TestEnsureSoldierFTSVirtualTable(t *testing.T) {
	d, err := Open(testtemp.New(t).Path())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	// Pre-condition: scratchpad_cache must exist (the bulk
	// INSERT LEFT JOINs it). applySchema already created it
	// via block-67, but call the helper directly so the test
	// pins the seam even if applySchema changes shape.
	if err := runOnConnTx(d.Conn(), dropSoldierFTStriggers); err != nil {
		t.Fatalf("precondition dropSoldierFTStriggers: %v", err)
	}

	// Insert one soldier so the bulk INSERT has a row to
	// project. Use the documented NOT-NULL columns.
	if _, err := d.Conn().Exec(`INSERT INTO soldiers (display_id, sync_id, first_name, last_name) VALUES (?, ?, ?, ?)`,
		"DXD-00001", "sync-1", "Thomas", "Carter"); err != nil {
		t.Fatalf("insert soldier: %v", err)
	}

	if err := runOnConnTx(d.Conn(), ensureSoldierFTSVirtualTable); err != nil {
		t.Fatalf("ensureSoldierFTSVirtualTable: %v", err)
	}

	if !tableExistsOnConn(d.Conn(), "soldiers_fts") {
		t.Errorf("post-ensureSoldierFTSVirtualTable: soldiers_fts missing")
	}

	// Bulk INSERT projected the soldier into soldiers_fts.
	var count int
	if err := d.Conn().QueryRow(`SELECT COUNT(*) FROM soldiers_fts WHERE display_id = ?`, "DXD-00001").Scan(&count); err != nil {
		t.Fatalf("count soldiers_fts: %v", err)
	}
	if count != 1 {
		t.Errorf("post-ensureSoldierFTSVirtualTable: soldiers_fts row for DXD-00001 = %d, want 1", count)
	}

	// Triggers must NOT be installed by this helper — that
	// is ensureSoldierFTStriggers' job. Pins the seam: each
	// helper does exactly one thing. We dropped them in the
	// pre-condition above; if VirtualTable had re-installed
	// any, this assertion catches it.
	for _, trig := range ftsTriggers {
		if triggerExistsOnConn(d.Conn(), trig) {
			t.Errorf("post-ensureSoldierFTSVirtualTable: trigger %q present (belongs to ensureSoldierFTStriggers)", trig)
		}
	}

	if err := runOnConnTx(d.Conn(), dropSoldierFTSVirtualTable); err != nil {
		t.Fatalf("dropSoldierFTSVirtualTable: %v", err)
	}
	if tableExistsOnConn(d.Conn(), "soldiers_fts") {
		t.Errorf("post-dropSoldierFTSVirtualTable: soldiers_fts still present")
	}
}

// TestEnsureSoldierFTStriggers pins the 6-trigger sub-helper
// in isolation. Requires scratchpad_cache + soldiers_fts to
// exist (the triggers reference both). The helper drops the
// 6 triggers if present + creates them fresh. Inverse drops
// the same 6, leaving the tables intact (the triggers are
// what the helper owns; the tables belong to the other
// helpers).
func TestEnsureSoldierFTStriggers(t *testing.T) {
	d, err := Open(testtemp.New(t).Path())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	// Pre-conditions: both tables must exist.
	if err := runOnConnTx(d.Conn(), ensureScratchpadCache); err != nil {
		t.Fatalf("precondition ensureScratchpadCache: %v", err)
	}
	if err := runOnConnTx(d.Conn(), ensureSoldierFTSVirtualTable); err != nil {
		t.Fatalf("precondition ensureSoldierFTSVirtualTable: %v", err)
	}

	if err := runOnConnTx(d.Conn(), ensureSoldierFTStriggers); err != nil {
		t.Fatalf("ensureSoldierFTStriggers: %v", err)
	}

	for _, trig := range ftsTriggers {
		if !triggerExistsOnConn(d.Conn(), trig) {
			t.Errorf("post-ensureSoldierFTStriggers: trigger %q missing", trig)
		}
	}

	if err := runOnConnTx(d.Conn(), dropSoldierFTStriggers); err != nil {
		t.Fatalf("dropSoldierFTStriggers: %v", err)
	}
	for _, trig := range ftsTriggers {
		if triggerExistsOnConn(d.Conn(), trig) {
			t.Errorf("post-dropSoldierFTStriggers: trigger %q still present", trig)
		}
	}

	// Inverse leaves the tables intact — they belong to the
	// other helpers.
	if !tableExistsOnConn(d.Conn(), "scratchpad_cache") {
		t.Errorf("post-dropSoldierFTStriggers: scratchpad_cache unexpectedly dropped")
	}
	if !tableExistsOnConn(d.Conn(), "soldiers_fts") {
		t.Errorf("post-dropSoldierFTStriggers: soldiers_fts unexpectedly dropped")
	}

	// Idempotent re-drop is safe.
	if err := runOnConnTx(d.Conn(), dropSoldierFTStriggers); err != nil {
		t.Errorf("dropSoldierFTStriggers (re-run with absent triggers): %v", err)
	}
}

// TestEnsureSoldierFTSComposition pins the end-to-end
// composer. Running ensureSoldierFTS on a fresh DB must
// produce the same observable state as running the three
// sub-helpers in sequence — same 2 tables, same 6 triggers,
// same projected soldier row. dropSoldierFTS mirrors.
//
// This is the regression net for "the umbrella stays
// equivalent to the parts." If a future refactor splits
// further or merges sub-helpers, this test catches drift
// in the composer's contract.
func TestEnsureSoldierFTSComposition(t *testing.T) {
	d, err := Open(testtemp.New(t).Path())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	if _, err := d.Conn().Exec(`INSERT INTO soldiers (display_id, sync_id, first_name, last_name) VALUES (?, ?, ?, ?)`,
		"DXD-00001", "sync-1", "Thomas", "Carter"); err != nil {
		t.Fatalf("insert soldier: %v", err)
	}

	if err := runOnConnTx(d.Conn(), ensureSoldierFTS); err != nil {
		t.Fatalf("ensureSoldierFTS: %v", err)
	}

	if !tableExistsOnConn(d.Conn(), "scratchpad_cache") {
		t.Errorf("post-ensureSoldierFTS: scratchpad_cache missing")
	}
	if !tableExistsOnConn(d.Conn(), "soldiers_fts") {
		t.Errorf("post-ensureSoldierFTS: soldiers_fts missing")
	}
	for _, trig := range ftsTriggers {
		if !triggerExistsOnConn(d.Conn(), trig) {
			t.Errorf("post-ensureSoldierFTS: trigger %q missing", trig)
		}
	}

	var count int
	if err := d.Conn().QueryRow(`SELECT COUNT(*) FROM soldiers_fts WHERE display_id = ?`, "DXD-00001").Scan(&count); err != nil {
		t.Fatalf("count soldiers_fts: %v", err)
	}
	if count != 1 {
		t.Errorf("post-ensureSoldierFTS: soldiers_fts row for DXD-00001 = %d, want 1", count)
	}

	if err := runOnConnTx(d.Conn(), dropSoldierFTS); err != nil {
		t.Fatalf("dropSoldierFTS: %v", err)
	}
	if tableExistsOnConn(d.Conn(), "soldiers_fts") {
		t.Errorf("post-dropSoldierFTS: soldiers_fts still present")
	}
	for _, trig := range ftsTriggers {
		if triggerExistsOnConn(d.Conn(), trig) {
			t.Errorf("post-dropSoldierFTS: trigger %q still present", trig)
		}
	}

	// Composer leaves scratchpad_cache alone — block-67's
	// downgrade path targets v60, where scratchpad_cache
	// existed pre-block-67 too. dropScratchpadCache is a
	// separate helper for callers that need it (none today).
	if !tableExistsOnConn(d.Conn(), "scratchpad_cache") {
		t.Errorf("post-dropSoldierFTS: scratchpad_cache unexpectedly dropped (composer owns only FTS5 artifacts)")
	}
}

// columnExistsOnConn is provided by migration_block_66_test.go
// (the block-66 split landed first; its helper has the same
// signature). Reused here to keep the split characterization
// tests aligned with the existing per-conn column check.

// runOnConnTx opens a tx on conn, runs fn, and commits (or
// rolls back on error). Mirrors the Begin/Commit pattern
// used by the production callers in identity.go and
// scratchpad.go so the sub-helpers are exercised under the
// same shape they will see in block-67's tx.
func runOnConnTx(conn *sql.DB, fn func(*sql.Tx) error) error {
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
