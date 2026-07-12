// inventory_counts_test.go — regression net for the new
// Count / KindRollup service methods that power the
// /inventory page (issue #491) + the extended ArchiveCounts
// SQL that powers the same page.
package records

import (
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

func TestEventService_Count(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)
	events := NewEventService(svc)
	svc.SetEvents(events)
	for i := 0; i < 5; i++ {
		displayID := "EVT-COUNT-" + string(rune('A'+i))
		_, err := svc.Create(models.Soldier{
			EntryType: "event",
			Kind:      "Battle",
			DisplayID: displayID,
			BeginDate: "07/01/1863",
		})
		if err != nil {
			t.Fatalf("Create event %d: %v", i, err)
		}
	}
	n, err := svc.events.Count()
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 5 {
		t.Errorf("Count = %d; want 5", n)
	}
}

func TestEventService_KindRollup(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)
	events := NewEventService(svc)
	svc.SetEvents(events)
	// 3 Battles, 2 Campaigns, 1 Death, 1 with blank kind.
	kinds := []string{"Battle", "Battle", "Battle", "Campaign", "Campaign", "Death", ""}
	for i, kind := range kinds {
		displayID := "EVT-KR-" + string(rune('A'+i))
		_, err := svc.Create(models.Soldier{
			EntryType: "event",
			Kind:      kind,
			DisplayID: displayID,
			BeginDate: "07/01/1863",
		})
		if err != nil {
			t.Fatalf("Create event (kind=%q): %v", kind, err)
		}
	}
	rollup, err := svc.events.KindRollup()
	if err != nil {
		t.Fatalf("KindRollup: %v", err)
	}
	// Map kind -> count.
	got := make(map[string]int)
	for _, item := range rollup {
		got[item.Kind] = item.Count
	}
	expected := map[string]int{
		"Battle":         3,
		"Campaign":       2,
		"Death":          1,
		"(unspecified)": 1, // blank kind bucketed
	}
	if len(got) != len(expected) {
		t.Errorf("KindRollup returned %d kinds; want %d (got=%v)", len(got), len(expected), got)
	}
	for k, v := range expected {
		if got[k] != v {
			t.Errorf("KindRollup[%q] = %d; want %d", k, got[k], v)
		}
	}
	// Order: count desc, kind asc. So "Battle" first.
	if len(rollup) == 0 || rollup[0].Kind != "Battle" {
		t.Errorf("KindRollup first item should be 'Battle' (highest count); got %+v", rollup)
	}
}

func TestTagService_Count(t *testing.T) {
	d := newTestDB(t)
	svc := NewTagService(d.Conn())
	// Empty archive = 0.
	if n, err := svc.Count(t.Context()); err != nil {
		t.Fatalf("Count (empty): %v", err)
	} else if n != 0 {
		t.Errorf("Count (empty) = %d; want 0", n)
	}
	for i := 0; i < 4; i++ {
		_, err := svc.UpsertByName(t.Context(), "tag-"+string(rune('A'+i)))
		if err != nil {
			t.Fatalf("UpsertByName: %v", err)
		}
	}
	n, err := svc.Count(t.Context())
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 4 {
		t.Errorf("Count = %d; want 4", n)
	}
}

func TestArticleService_Count(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	if _, err := d.ConfigureUserIdentity("Test", "Author", "Person", 1900); err != nil {
		t.Fatalf("ConfigureUserIdentity: %v", err)
	}
	artSvc := NewArticleService(soldierSvc)
	// 2 live + 1 snapshot (snapshots excluded from Count).
	live1, err := artSvc.Create(models.Article{Title: "Live 1", BodyMD: "# Hi"})
	if err != nil {
		t.Fatalf("Create live 1: %v", err)
	}
	live2, err := artSvc.Create(models.Article{Title: "Live 2", BodyMD: "# Hi"})
	if err != nil {
		t.Fatalf("Create live 2: %v", err)
	}
	if _, err := artSvc.Snapshot(live1.ID); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	n, err := artSvc.Count()
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 2 {
		t.Errorf("Count = %d; want 2 (snapshots excluded)", n)
	}
	_ = live2
}
