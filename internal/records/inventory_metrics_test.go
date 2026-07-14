// inventory_metrics_test.go -- issue #580 slice 1.
// Pins the aggregation contract for ActivityMetrics so the
// /inventory page's Metrics section reflects the same shape
// the smoke probe asserts against.
//
// Aggregation contract (locked decisions):
//   - Primary entries only: Soldiers + Spouse Records + Linked
//     Persons + Event Records + live Articles.
//   - Article Snapshots (is_snapshot = 1) are excluded so the
//     activity count matches the existing Articles inventory
//     headline number.
//   - Stored created_at is the date source. SQLite's
//     `date(created_at)` normalises the timestamp to a
//     YYYY-MM-DD string so day buckets are tz-neutral.
//   - First / latest dates are extreme keys across the map.
//   - ActiveDayCount equals the number of distinct keys in
//     the day map.
package records

import (
	"sort"
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
)

func TestActivityMetricsEmptyDatabase(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)
	got, err := svc.ActivityMetrics(t.Context())
	if err != nil {
		t.Fatalf("ActivityMetrics: %v", err)
	}
	if len(got.EntriesPerDay) != 0 {
		t.Errorf("EntriesPerDay = %v; want empty map", got.EntriesPerDay)
	}
	if got.ActiveDayCount != 0 {
		t.Errorf("ActiveDayCount = %d; want 0", got.ActiveDayCount)
	}
	if got.FirstEntryDate != "" || got.LatestEntryDate != "" {
		t.Errorf("FirstEntryDate / LatestEntryDate = %q / %q; want both empty for an empty archive", got.FirstEntryDate, got.LatestEntryDate)
	}
	var total int
	total += got.TotalsByType.Soldiers
	total += got.TotalsByType.SpouseRecords
	total += got.TotalsByType.LinkedPersons
	total += got.TotalsByType.EventRecords
	total += got.TotalsByType.Articles
	if total != 0 {
		t.Errorf("TotalsByType sum = %d; want 0 for empty archive", total)
	}
}

// TestActivityMetricsAggregatesPrimaryEntries pins the
// locked-decision entry-type aggregation: each primary type
// contributes to the per-day bucket + the totals; Article
// Snapshots do NOT contribute.
func TestActivityMetricsAggregatesPrimaryEntries(t *testing.T) {
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
		EntryType: "wife",
		SpouseSoldierID: 1,
		RelationshipLabel: "wife",
	})
	insertSoldier(t, svc, models.Soldier{
		DisplayID: "P-003",
		FirstName: "Sibling",
		LastName:  "Doe",
		EntryType: "linked_person",
		SpouseSoldierID: 1,
		RelationshipLabel: "sibling",
	})
	insertSoldier(t, svc, models.Soldier{
		DisplayID: "EVT-001",
		Kind:      "Battle",
		EntryType: "event",
		BeginDate: "07/01/1863",
	})
	// Live article -- included.
	insertArticle(t, d, "ART-001", "First article", false)
	// Snapshot -- excluded.
	insertArticle(t, d, "ART-001-S1", "First snapshot", true)

	got, err := svc.ActivityMetrics(t.Context())
	if err != nil {
		t.Fatalf("ActivityMetrics: %v", err)
	}
	if got.TotalsByType.Soldiers != 1 {
		t.Errorf("TotalsByType.Soldiers = %d; want 1", got.TotalsByType.Soldiers)
	}
	if got.TotalsByType.SpouseRecords != 1 {
		t.Errorf("TotalsByType.SpouseRecords = %d; want 1", got.TotalsByType.SpouseRecords)
	}
	if got.TotalsByType.LinkedPersons != 1 {
		t.Errorf("TotalsByType.LinkedPersons = %d; want 1", got.TotalsByType.LinkedPersons)
	}
	if got.TotalsByType.EventRecords != 1 {
		t.Errorf("TotalsByType.EventRecords = %d; want 1", got.TotalsByType.EventRecords)
	}
	if got.TotalsByType.Articles != 1 {
		t.Errorf("TotalsByType.Articles = %d; want 1 (snapshot excluded)", got.TotalsByType.Articles)
	}
	if got.ActiveDayCount == 0 {
		t.Errorf("ActiveDayCount = 0; want at least 1 (rows were just inserted)")
	}
	if got.FirstEntryDate == "" || got.LatestEntryDate == "" {
		t.Errorf("FirstEntryDate / LatestEntryDate = %q / %q; want both set", got.FirstEntryDate, got.LatestEntryDate)
	}
	if got.FirstEntryDate > got.LatestEntryDate {
		t.Errorf("FirstEntryDate %q > LatestEntryDate %q; first must precede latest", got.FirstEntryDate, got.LatestEntryDate)
	}
}

// TestActivityMetricsKeysAreSortable pins the contract that
// the per-day bucket keys are YYYY-MM-DD strings (lexicographic
// sort = chronological sort). Used by the templ partial to
// render in chronological order without a separate Date parse.
func TestActivityMetricsKeysAreSortable(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)
	insertSoldier(t, svc, models.Soldier{
		DisplayID: "P-001",
		EntryType: "soldier",
	})

	got, err := svc.ActivityMetrics(t.Context())
	if err != nil {
		t.Fatalf("ActivityMetrics: %v", err)
	}
	if len(got.EntriesPerDay) == 0 {
		t.Fatal("got no day buckets; insert test failed")
	}
	keys := make([]string, 0, len(got.EntriesPerDay))
	for k := range got.EntriesPerDay {
		keys = append(keys, k)
	}
	sorted := make([]string, len(keys))
	copy(sorted, keys)
	sort.Strings(sorted)
	for i := range keys {
		if keys[i] != sorted[i] {
			t.Errorf("day keys are not lexicographically sorted; got %v want %v", keys, sorted)
			break
		}
	}
}

// TestActivityMetricsHeadlinesMatch ensures the activity totals
// equal the headline counts the ArchiveCounts() helper returns.
// This is the cross-check the smoke probe depends on (issue #580
// acceptance criterion: "Totals by type match the existing
// Archive Inventory summary").
func TestActivityMetricsHeadlinesMatch(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)
	insertSoldier(t, svc, models.Soldier{DisplayID: "P-001", EntryType: "soldier"})
	insertSoldier(t, svc, models.Soldier{DisplayID: "P-002", EntryType: "wife", SpouseSoldierID: 1, RelationshipLabel: "wife"})
	insertSoldier(t, svc, models.Soldier{DisplayID: "EVT-001", EntryType: "event", Kind: "Battle"})

	metrics, err := svc.ActivityMetrics(t.Context())
	if err != nil {
		t.Fatalf("ActivityMetrics: %v", err)
	}
	counts, err := svc.ArchiveCounts()
	if err != nil {
		t.Fatalf("ArchiveCounts: %v", err)
	}
	if metrics.TotalsByType.EventRecords != counts.EventRecords {
		t.Errorf("Event totals = %d; headline events = %d",
			metrics.TotalsByType.EventRecords, counts.EventRecords)
	}
}

// helpers -- issue #580.

func insertSoldier(t *testing.T, svc *SoldierService, s models.Soldier) {
	t.Helper()
	if _, err := svc.Create(s); err != nil {
		t.Fatalf("create soldier %s: %v", s.DisplayID, err)
	}
}

// insertArticle writes a single row into articles so the
// activity query has a sample to read. Schema mirrors the
// articles table in internal/db/migrations.go.
func insertArticle(t *testing.T, d *db.DB, displayID, title string, isSnapshot bool) {
	t.Helper()
	snapshot := 0
	if isSnapshot {
		snapshot = 1
	}
	conn := d.Conn()
	if _, err := conn.Exec(`INSERT INTO articles (sync_id, display_id, title, subtitle, body_md, body_html, created_at, updated_at, is_snapshot) VALUES (?, ?, ?, '', '', '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ?)`,
		"sync-"+displayID, displayID, title, snapshot); err != nil {
		t.Fatalf("insert article %s: %v", displayID, err)
	}
}
