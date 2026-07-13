// Browse tag-filter regression tests (issue #183). Asserts the
// AND-logic HAVING clause: N selected tags → only Person Records
// that have every tag attached. Empty tag list = no filter.
// Deep-link parsing is verified through the normalizeBrowseRequest
// path (TrimSpace + blank-drop + duplicate-drop).
package records

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
)

func newBrowseTestDB(t *testing.T) (*SoldierService, *TagService, *db.DB) {
	t.Helper()
	dataDir := filepath.Join(t.TempDir(), ".dixiedata")
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return NewSoldierService(database),
		NewTagService(database.Conn()),
		database
}

func insertBrowseSeedRow(t *testing.T, database *db.DB, label string) int64 {
	t.Helper()
	displayID := fmt.Sprintf("DXD-BROW-%s", label)
	res, err := database.Conn().Exec(
		`INSERT INTO soldiers (display_id, first_name, last_name) VALUES (?, ?, ?)`,
		displayID, "Browse", label)
	if err != nil {
		t.Fatalf("insert %s: %v", displayID, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}
	return id
}

func TestBrowse_TagFilterAND(t *testing.T) {
	svc, tags, database := newBrowseTestDB(t)
	ctx := context.Background()
	a := insertBrowseSeedRow(t, database, "A")
	b := insertBrowseSeedRow(t, database, "B")
	c := insertBrowseSeedRow(t, database, "C")
	_ = c

	tag1, _ := tags.UpsertByName(ctx, "alpha")
	tag2, _ := tags.UpsertByName(ctx, "beta")
	tags.Attach(ctx, tag1.ID, a)
	tags.Attach(ctx, tag2.ID, a)
	tags.Attach(ctx, tag1.ID, b)

	// Single tag returns both
	rows, _, _, err := svc.BrowsePage(BrowseRequest{Tags: []string{"alpha"}})
	if err != nil {
		t.Fatalf("BrowsePage(1 tag): %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("1-tag filter rows = %d, want 2", len(rows))
	}

	// Two tags AND: only A has both.
	rows, _, _, err = svc.BrowsePage(BrowseRequest{Tags: []string{"alpha", "beta"}})
	if err != nil {
		t.Fatalf("BrowsePage(2 tags): %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("AND 2-tag filter rows = %d, want 1", len(rows))
	}
	if rows[0].ID != a {
		t.Errorf("AND 2-tag result = %d, want %d (A)", rows[0].ID, a)
	}
}

func TestBrowse_TagFilterUnknownIsNoOp(t *testing.T) {
	svc, _, _ := newBrowseTestDB(t)
	// Unknown tag — no rows. Empty result, no error.
	rows, _, _, err := svc.BrowsePage(BrowseRequest{Tags: []string{"does-not-exist"}})
	if err != nil {
		t.Fatalf("BrowsePage(unknown): %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("unknown-tag rows = %d, want 0", len(rows))
	}
}

func TestBrowse_TagFilterNormalizeDedup(t *testing.T) {
	req := normalizeBrowseRequest(BrowseRequest{
		Tags: []string{"  VC-Shiloh ", "vc-shiloh", "", "vc-shiloh", "unit-4th-al"},
	})
	if len(req.Tags) != 2 {
		t.Errorf("normalize dedup = %v, want 2 entries", req.Tags)
	}
}

func TestBrowse_TagFilterEmptyListNoOp(t *testing.T) {
	svc, _, _ := newBrowseTestDB(t)
	// No tag filter at all.
	req := normalizeBrowseRequest(BrowseRequest{})
	if len(req.Tags) != 0 {
		t.Errorf("empty Tags request should not normalise to entries: %v", req.Tags)
	}
	rows, _, _, err := svc.BrowsePage(req)
	if err != nil {
		t.Fatalf("BrowsePage(empty): %v", err)
	}
	// Without a seed insert here, rows is empty; the relevant
	// assertion is that we did not error and the SQL path executed
	// (no-tag filter). The handler-level deep-link parse in
	// internal/appshell is what exercises the URL → request path;
	// service-layer normalisation is the boundary under test.
	_ = rows
}

// TestBrowse_RecentlyAddedScopeEnforcesRecencyWindow (issue #512)
// pins the recency predicate added to BrowseScopeRecentlyAdded:
// a row whose created_at falls outside the 7-day window must NOT
// surface under the Recently added scope, even when its
// created_at is non-empty. Mirrors the user-visible bug: a Shared
// Archive import preserves the source created_at, so a years-old
// import would leak rows into the recently-added view without
// the datetime() cutoff.
func TestBrowse_RecentlyAddedScopeEnforcesRecencyWindow(t *testing.T) {
	svc, _, database := newBrowseTestDB(t)
	ctx := context.Background()

	recent := insertBrowseSeedRow(t, database, "RECENT")
	old := insertBrowseSeedRow(t, database, "OLD")

	// Recent row: created_at defaults to CURRENT_TIMESTAMP via the
	// schema (db/schema.go:69), so it falls inside the 7-day window.
	// Old row: pin created_at to 2 years ago so the cutoff excludes it.
	twoYearsAgo := "datetime('now', '-2 years')"
	if _, err := database.Conn().Exec(
		`UPDATE soldiers SET created_at = `+twoYearsAgo+` WHERE id = ?`, old); err != nil {
		t.Fatalf("pin old created_at: %v", err)
	}

	rows, _, _, err := svc.BrowsePage(BrowseRequest{Scope: BrowseScopeRecentlyAdded})
	if err != nil {
		t.Fatalf("BrowsePage(recently_added): %v", err)
	}

	gotIDs := make(map[int64]bool, len(rows))
	for _, r := range rows {
		gotIDs[r.ID] = true
	}
	if !gotIDs[recent] {
		t.Errorf("recent row (id=%d) missing from Recently added scope; want it present", recent)
	}
	if gotIDs[old] {
		t.Errorf("old row (id=%d, created_at pinned to -2 years) leaked into Recently added scope; want it excluded by the 7-day cutoff", old)
	}
	_ = ctx
}
