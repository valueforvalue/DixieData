// inventory_metrics_kind_test.go -- issue #583 slice 1.
//
// Pins the per-kind daily breakdown contract for ActivityMetrics.
// The Activity metrics line graph (issue #583) renders one line
// per kind (soldier / spouse / linked / event / article); each line
// is the per-day count for that kind. The storage layer's CTE
// already groups by (day, kind) -- the change is to expose that
// grouping instead of summing it away.
//
// Contract:
//   - EntriesPerDayByKind has 5 buckets: "soldier", "spouse",
//     "linked", "event", "article". The slice order in the
//     templ/JS layer is locked to that exact order.
//   - Each inner map is keyed by YYYY-MM-DD with the day's
//     per-kind count. Missing days are absent (not zero).
//   - Existing EntriesPerDay (the aggregate) and TotalsByType
//     (the per-kind totals) are unchanged. Backward compat.
//   - Empty archive: EntriesPerDayByKind has 5 buckets, each
//     an empty (non-nil) map. This avoids a nil-deref in the
//     templ partial.

package records

import (
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

// TestActivityMetricsEntriesPerDayByKindShapes pins the 5-kind
// bucket contract. Even on an empty archive the outer map has
// every key with a non-nil empty inner map.
func TestActivityMetricsEntriesPerDayByKindShapes(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	got, err := svc.ActivityMetrics(t.Context())
	if err != nil {
		t.Fatalf("ActivityMetrics: %v", err)
	}
	wantKeys := []string{"soldier", "spouse", "linked", "event", "article"}
	if len(got.EntriesPerDayByKind) != len(wantKeys) {
		t.Fatalf("EntriesPerDayByKind has %d buckets; want %d (%v)",
			len(got.EntriesPerDayByKind), len(wantKeys), wantKeys)
	}
	for _, k := range wantKeys {
		inner, ok := got.EntriesPerDayByKind[k]
		if !ok {
			t.Errorf("missing kind bucket %q in EntriesPerDayByKind", k)
			continue
		}
		if inner == nil {
			t.Errorf("kind bucket %q is nil; want non-nil empty map", k)
		}
	}
}

// TestActivityMetricsEntriesPerDayByKindAggregates pins the
// per-kind daily rollup. Inserting two soldiers on day X and one
// event on day Y must produce EntriesPerDayByKind["soldier"][X]=2
// and EntriesPerDayByKind["event"][Y]=1 with every other bucket
// empty.
func TestActivityMetricsEntriesPerDayByKindAggregates(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)
	events := NewEventService(svc)
	svc.SetEvents(events)

	insertSoldier(t, svc, models.Soldier{
		DisplayID: "P-001",
		FirstName: "John",
		LastName:  "Doe",
		EntryType: "soldier",
	})
	insertSoldier(t, svc, models.Soldier{
		DisplayID: "P-002",
		FirstName: "Jane",
		LastName:  "Doe",
		EntryType: "soldier",
	})
	insertSoldier(t, svc, models.Soldier{
		DisplayID: "EVT-001",
		Kind:      "Battle",
		EntryType: "event",
		BeginDate: "07/01/1863",
	})

	got, err := svc.ActivityMetrics(t.Context())
	if err != nil {
		t.Fatalf("ActivityMetrics: %v", err)
	}
	// Find the soldier day + the event day; SQLite's date()
	// normalises CURRENT_TIMESTAMP so all three rows land on
	// the same UTC day for the soldier bucket.
	soldierDay, ok := findSingleDay(t, got.EntriesPerDayByKind["soldier"])
	if !ok {
		t.Fatalf("soldier bucket has no days; got %v", got.EntriesPerDayByKind["soldier"])
	}
	if got.EntriesPerDayByKind["soldier"][soldierDay] != 2 {
		t.Errorf("soldier bucket on %s = %d; want 2",
			soldierDay, got.EntriesPerDayByKind["soldier"][soldierDay])
	}
	eventDay, ok := findSingleDay(t, got.EntriesPerDayByKind["event"])
	if !ok {
		t.Fatalf("event bucket has no days; got %v", got.EntriesPerDayByKind["event"])
	}
	if got.EntriesPerDayByKind["event"][eventDay] != 1 {
		t.Errorf("event bucket on %s = %d; want 1",
			eventDay, got.EntriesPerDayByKind["event"][eventDay])
	}
	for _, k := range []string{"spouse", "linked", "article"} {
		if len(got.EntriesPerDayByKind[k]) != 0 {
			t.Errorf("%s bucket has %d days; want 0 (no inserts of that kind)",
				k, len(got.EntriesPerDayByKind[k]))
		}
	}
}

// TestActivityMetricsEntriesPerDayByKindMatchesTotals pins
// the invariant: the per-kind daily totals (summed across
// days) must equal the existing TotalsByType. The line graph
// and the per-kind rollup card cannot disagree.
func TestActivityMetricsEntriesPerDayByKindMatchesTotals(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)
	events := NewEventService(svc)
	svc.SetEvents(events)

	insertSoldier(t, svc, models.Soldier{DisplayID: "P-001", EntryType: "soldier"})
	insertSoldier(t, svc, models.Soldier{DisplayID: "P-002", EntryType: "wife", SpouseSoldierID: 1, RelationshipLabel: "wife"})
	insertSoldier(t, svc, models.Soldier{DisplayID: "P-003", EntryType: "linked_person", SpouseSoldierID: 1, RelationshipLabel: "sibling"})
	insertSoldier(t, svc, models.Soldier{DisplayID: "EVT-001", EntryType: "event", Kind: "Battle"})
	insertArticle(t, d, "ART-001", "Live article", false)
	insertArticle(t, d, "ART-001-S1", "Snapshot", true)

	got, err := svc.ActivityMetrics(t.Context())
	if err != nil {
		t.Fatalf("ActivityMetrics: %v", err)
	}
	pairs := map[string]struct {
		byKind map[string]int
		total  int
	}{
		"soldier": {got.EntriesPerDayByKind["soldier"], got.TotalsByType.Soldiers},
		"spouse":  {got.EntriesPerDayByKind["spouse"], got.TotalsByType.SpouseRecords},
		"linked":  {got.EntriesPerDayByKind["linked"], got.TotalsByType.LinkedPersons},
		"event":   {got.EntriesPerDayByKind["event"], got.TotalsByType.EventRecords},
		"article": {got.EntriesPerDayByKind["article"], got.TotalsByType.Articles},
	}
	for kind, p := range pairs {
		sum := 0
		for _, n := range p.byKind {
			sum += n
		}
		if sum != p.total {
			t.Errorf("%s: sum-of-daily=%d != TotalsByType=%d",
				kind, sum, p.total)
		}
	}
}

// findSingleDay returns the single key in a one-element map.
// The test helpers rely on SQLite's date() landing each kind's
// inserts on one day; if the test ever crosses a UTC midnight
// boundary the map will have >1 entries and the assertion will
// tell us clearly.
func findSingleDay(t *testing.T, m map[string]int) (string, bool) {
	t.Helper()
	if len(m) == 0 {
		return "", false
	}
	if len(m) > 1 {
		t.Logf("warning: bucket has %d days, expected 1: %v", len(m), m)
	}
	for k := range m {
		return k, true
	}
	return "", false
}