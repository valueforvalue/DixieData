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
		"block-60-v54-to-v60-jump",
		"block-2-event-sources",
		"block-3-articles",
		"block-63-source-sort-order",
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
		"block-1-schema-baseline":    Reversible,
		"block-60-v54-to-v60-jump":   Irreversible,
		"block-2-event-sources":      Reversible,
		"block-3-articles":           Reversible,
		"block-63-source-sort-order": Reversible,
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
// blocks. The consolidated v54→v60 jump block is the only
// Irreversible block in the current 3-block slice; the
// pre-#320 v1-v53 chain that previously contributed 5
// Irreversible blocks (4, 5, 12, 13, 17) was collapsed into
// the inline block-1 schema for fresh installs + the
// consolidated jump for v54 upgrades. A change here means
// the DOWN runner's gate semantics changed too.
func TestReversibilityIrreducibleCount(t *testing.T) {
	irr := 0
	for _, m := range Migrations() {
		if m.Reversibility == Irreversible {
			irr++
		}
	}
	if irr != 1 {
		t.Errorf("Irreversible migration count = %d, want 1 (block-60-v54-to-v60-jump)", irr)
	}
}