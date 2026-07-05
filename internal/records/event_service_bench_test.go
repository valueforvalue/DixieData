package records

import (
	"fmt"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

// Micro-benchmarks for EventService hot paths (issue #320 child
// #338, slot 16/16 \u2014 the validation step that gates #320 close).
// Each benchmark seeds its own DB via newTestDB so the fixture
// stays self-contained; benchmarks do NOT share state across each
// other.
//
// Baselines established in docs/benchmarks/events.md are the
// reference numbers a future schema/registry change is compared
// against.
//
// Run with: go test -bench BenchmarkEventService -benchmem \
//                       ./internal/records/

// eventBenchSeed seeds `eventCount` Event Records + `personCount`
// Person Records (entry_type=soldier) and returns the env the
// benchmarks need. Returns nil + non-nil error if seeding fails.
// The two pools are otherwise unlinked; per-benchmark setup
// attaches what it needs.
func eventBenchSeed(b *testing.B, eventCount, personCount int) (svc *SoldierService, ev *EventService) {
	b.Helper()
	d := newTestDB(&testing.T{})
	svc = NewSoldierService(d)
	ev = NewEventService(svc)

	for i := 0; i < personCount; i++ {
		if _, err := svc.Create(models.Soldier{
			FirstName: "Bench",
			LastName:  fmt.Sprintf("Person-%05d", i),
		}); err != nil {
			b.Fatalf("seed person %d: %v", i, err)
		}
	}
	for i := 0; i < eventCount; i++ {
		if _, err := ev.CreateEvent(models.Soldier{
			Kind:      fmt.Sprintf("BenchKind-%05d", i),
			BeginDate: "07/01/1863",
			EndDate:   "07/03/1863",
		}); err != nil {
			b.Fatalf("seed event %d: %v", i, err)
		}
	}
	return svc, ev
}

// firstSoldierID returns the lowest soldiers.id whose
// entry_type=person kind, used by the attach/detach round-trip
// benchmark to look up the seeded Person Record without
// depending on DisplayID ordering.
func firstSoldierID(b *testing.B, svc *SoldierService, entryType string) int64 {
	b.Helper()
	var id int64
	if err := svc.db.Conn().QueryRow(
		`SELECT id FROM soldiers WHERE entry_type = ? ORDER BY id LIMIT 1`,
		entryType,
	).Scan(&id); err != nil {
		b.Fatalf("firstSoldierID(%q): %v", entryType, err)
	}
	return id
}

// BenchmarkListEventsPagination drives ListEvents at a fixed
// page size over a 5,000-row pool. The micro-benchmark anchors
// the v60 pagination path so future schema/scan changes show
// up in `go test -bench` regression. Compare baseline in
// docs/benchmarks/events.md.
func BenchmarkListEventsPagination(b *testing.B) {
	const events = 5000
	const pageSize = 50
	_, ev := eventBenchSeed(b, events, 0)
	const maxPage = events / pageSize
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		page := (i % maxPage) + 1
		got, err := ev.ListEvents(page, pageSize)
		if err != nil {
			b.Fatalf("ListEvents page=%d: %v", page, err)
		}
		if len(got) == 0 {
			b.Fatalf("ListEvents page=%d returned 0 rows; expected %d", page, pageSize)
		}
	}
}

// BenchmarkListEventsPageSizeDrift documents the per-page-size
// cost across {25, 50, 100, 200} at a 5,000-row pool size. The
// acceptance criterion for issue #338 slot #320.15 item 3 is
// "monotonic time per page" \u2014 i.e. the 200-row page must not be
// 8x slower than the 25-row page. The benchmark prints the
// ratio in the report line so reviewers see drift immediately.
//
// Run with: go test -bench BenchmarkListEventsPageSizeDrift -benchmem
func BenchmarkListEventsPageSizeDrift(b *testing.B) {
	const events = 5000
	_, ev := eventBenchSeed(b, events, 0)
	for _, pageSize := range []int{25, 50, 100, 200} {
		pageSize := pageSize
		b.Run(fmt.Sprintf("PageSize=%d", pageSize), func(b *testing.B) {
			maxPageLocal := events / pageSize
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				page := (i % maxPageLocal) + 1
				if _, err := ev.ListEvents(page, pageSize); err != nil {
					b.Fatalf("ListEvents size=%d page=%d: %v", pageSize, page, err)
				}
			}
		})
	}
}

// BenchmarkAttachDetachRoundTrip seeds one event + one person,
// then loops attach + detach under b.N. This catches the N+1
// regression class in the junction CRUD path: per-iteration cost
// stays constant only if Attach and Detach are single-shot.
//
// Run with: go test -bench BenchmarkAttachDetachRoundTrip -benchmem
func BenchmarkAttachDetachRoundTrip(b *testing.B) {
	svc, ev := eventBenchSeed(b, 1, 1)
	events, err := ev.ListEvents(1, 1)
	if err != nil {
		b.Fatalf("seed ListEvents: %v", err)
	}
	if len(events) != 1 {
		b.Fatalf("seed: expected 1 event, got %d", len(events))
	}
	eventID := events[0].ID
	personID := firstSoldierID(b, svc, models.EntryTypeSoldier)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ev.AttachEventToPerson(eventID, personID); err != nil {
			b.Fatalf("Attach %d: %v", i, err)
		}
		if err := ev.DetachEventFromPerson(eventID, personID); err != nil {
			b.Fatalf("Detach %d: %v", i, err)
		}
	}
}

// BenchmarkListForPerson validates that ListForPerson stays a
// single SQL roundtrip even when the linked-event count grows.
// Uses an event-pool of 100 to match issue #338 item 4's
// "junction with 100+ rows" assertion.
//
// Run with: go test -bench BenchmarkListForPerson -benchmem
func BenchmarkListForPerson(b *testing.B) {
	svc, ev := eventBenchSeed(b, 100, 1)
	personID := firstSoldierID(b, svc, models.EntryTypeSoldier)
	eventIDs := []int64{}
	for page := 1; ; page++ {
		chunk, err := ev.ListEvents(page, 50)
		if err != nil {
			b.Fatalf("seed ListEvents page=%d: %v", page, err)
		}
		for _, e := range chunk {
			eventIDs = append(eventIDs, e.ID)
		}
		if len(chunk) < 50 {
			break
		}
	}
	for _, eid := range eventIDs {
		if _, err := ev.AttachEventToPerson(eid, personID); err != nil {
			b.Fatalf("attach %d: %v", eid, err)
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		got, err := ev.ListForPerson(personID)
		if err != nil {
			b.Fatalf("ListForPerson: %v", err)
		}
		if len(got) != len(eventIDs) {
			b.Fatalf("ListForPerson = %d events, want %d", len(got), len(eventIDs))
		}
	}
}
