package db

import (
	"database/sql"
	"errors"
	"testing"
)

// TestMigrationsCatalogueIsOrdered locks the per-block execution order
// in the migrations slice. The future DOWN runner (issue #273) walks
// this slice in reverse; a reorder here would silently change the
// runner's behaviour. The block IDs are stable identifiers documented
// at docs/migrations/reversibility.md.
func TestMigrationsCatalogueIsOrdered(t *testing.T) {
	migs := Migrations()
	if len(migs) == 0 {
		t.Fatal("Migrations() returned empty slice; catalogue regression")
	}

	want := []string{
		"block-1-schema-baseline",
		"block-2-add-column-loop",
		"block-3-is-generated-flip",
		"block-4-phase1-distributed-merge",
		"block-5-phase2-canonical-dates",
		"block-6-soldiers-normalization",
		"block-7-last-edited-at-backfill",
		"block-8-images-is-primary",
		"block-9-idx-soldiers-spouse",
		"block-10-idx-soldiers-import-batch",
		"block-11-node-prefix-configuration",
		"block-12-sanitized-display-ids",
		"block-13-canonical-date-data",
		"block-14-ensure-soldier-fts",
		"block-15-archive-meta-seed",
		"block-16-entry-type-discipline",
		"block-17-research-log-evidence-rename",
	}
	for i, wantID := range want {
		if migs[i].ID != wantID {
			t.Errorf("migrations[%d].ID = %q, want %q (order is part of the DOWN runner's contract; see docs/migrations/reversibility.md)",
				i, migs[i].ID, wantID)
		}
	}
}

// TestMigrationsCatalogueHasUp locks that every migration carries a
// non-nil Up function. A nil Up would cause applySchema to panic at
// tx time, which the defer tx.Rollback() cannot recover from cleanly.
func TestMigrationsCatalogueHasUp(t *testing.T) {
	for i, m := range Migrations() {
		if m.Up == nil {
			t.Errorf("migrations[%d].ID=%q has nil Up function", i, m.ID)
		}
	}
}

// TestMigrationsCatalogueHasDown locks that every migration carries
// a non-nil Down function. applyDownSchema refuses the path with
// ErrDowngradeRefused when it encounters a nil Down (per the
// applyDownSchema comment), so a silent nil here would surface as
// an opaque "block N has no Down function" error at runner time.
// PartiallyReversible blocks can have a no-op Down (Block 11's
// "best-effort no-op") but the function must still be present.
func TestMigrationsCatalogueHasDown(t *testing.T) {
	for i, m := range Migrations() {
		if m.Down == nil {
			t.Errorf("migrations[%d].ID=%q has nil Down function", i, m.ID)
		}
	}
}

// TestMigrationsIrreversibleDownRefuses locks the runner contract
// for Irreversible blocks: their Down function must return
// ErrMigrationIrreversible. applyDownSchema unwraps this to refuse
// the path with ErrDowngradeRefused.
func TestMigrationsIrreversibleDownRefuses(t *testing.T) {
	// Stand up a real *sql.Tx so we can call the Down closure.
	// In-memory SQLite + a single shared connection gives us
	// the minimal environment the Down function needs.
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer conn.Close()
	tx, err := conn.Begin()
	if err != nil {
		t.Fatalf("conn.Begin: %v", err)
	}
	defer tx.Rollback()

	for _, m := range Migrations() {
		if m.Reversibility != Irreversible {
			continue
		}
		err := m.Down(tx)
		if !errors.Is(err, ErrMigrationIrreversible) {
			t.Errorf("%s (Irreversible) Down returned %v, want ErrMigrationIrreversible", m.ID, err)
		}
	}
}

// TestMigrationsReversibilityMapping locks the per-block reversibility
// classification. Issue #273's DOWN runner will refuse paths that
// cross Irreversible blocks unless --force-irreversible is supplied;
// a silent flip from PartiallyReversible to Reversible (or vice
// versa) would change the runner's gate behaviour. The expected
// mapping mirrors docs/migrations/reversibility.md exactly.
func TestMigrationsReversibilityMapping(t *testing.T) {
	want := map[string]Reversibility{
		"block-1-schema-baseline":              Reversible,
		"block-2-add-column-loop":              Reversible,
		"block-3-is-generated-flip":            PartiallyReversible,
		"block-4-phase1-distributed-merge":     Irreversible,
		"block-5-phase2-canonical-dates":       Irreversible,
		"block-6-soldiers-normalization":       PartiallyReversible,
		"block-7-last-edited-at-backfill":      PartiallyReversible,
		"block-8-images-is-primary":            PartiallyReversible,
		"block-9-idx-soldiers-spouse":          Reversible,
		"block-10-idx-soldiers-import-batch":   Reversible,
		"block-11-node-prefix-configuration":   PartiallyReversible,
		"block-12-sanitized-display-ids":       Irreversible,
		"block-13-canonical-date-data":         Irreversible,
		"block-14-ensure-soldier-fts":          PartiallyReversible,
		"block-15-archive-meta-seed":           Reversible,
		"block-16-entry-type-discipline":       Reversible,
		"block-17-research-log-evidence-rename": Irreversible,
		"block-60-event-records-event-person-links-fk-rename": PartiallyReversible,
		"block-61-event-sources":                 Reversible,
	}

	for _, m := range Migrations() {
		expected, ok := want[m.ID]
		if !ok {
			t.Errorf("migrations slice contains unknown block ID %q (add it to the want map and docs/migrations/reversibility.md)", m.ID)
			continue
		}
		if m.Reversibility != expected {
			t.Errorf("%s reversibility = %s, want %s (per docs/migrations/reversibility.md)",
				m.ID, m.Reversibility, expected)
		}
		if m.Reason == "" {
			t.Errorf("%s has empty Reason; every block must cite the catalogue rationale", m.ID)
		}
	}

	if len(Migrations()) != len(want) {
		t.Errorf("Migrations() length = %d, want %d (catalogue drift; update the want map)",
			len(Migrations()), len(want))
	}
}

// TestReversibilityString locks the string labels used by the future
// "what was lost" manifest (issue #273 Q2). The labels are part of
// the CLI surface.
func TestReversibilityString(t *testing.T) {
	cases := []struct {
		r    Reversibility
		want string
	}{
		{Reversible, "reversible"},
		{PartiallyReversible, "partially_reversible"},
		{Irreversible, "irreversible"},
		{Reversibility(99), "unknown"},
	}
	for _, c := range cases {
		if got := c.r.String(); got != c.want {
			t.Errorf("Reversibility(%d).String() = %q, want %q", c.r, got, c.want)
		}
	}
}

// TestReversibilityIrreducibleCount locks the count of Irreversible
// blocks at 5. The audit (docs/migrations/reversibility.md) classifies
// Blocks 4, 5, 12, 13, 17 as Irreversible. A change here means the
// DOWN runner's gate semantics changed too.
func TestReversibilityIrreducibleCount(t *testing.T) {
	irr := 0
	for _, m := range Migrations() {
		if m.Reversibility == Irreversible {
			irr++
		}
	}
	if irr != 5 {
		t.Errorf("Irreversible migration count = %d, want 5 (Blocks 4, 5, 12, 13, 17 per docs/migrations/reversibility.md)", irr)
	}
}