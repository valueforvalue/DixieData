# v61 Bug Repro — Issue #340

This is the scratch reproduction test used to verify the v60
data-loss bug on 2026-07-04. Run against the v60 code (pre-
v61) to confirm the bug exists; run against v61+ to confirm
the fix.

## The test (failing on v60, passing on v61+)

```go
package records

import (
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

// Confirms that updating an Event preserves attached Source
// Records. FAILING on v60 (slot #329 reused the `records`
// table, which `replaceRecords` wipes on every Update).
// PASSING on v61+ (slot #340 fix: dedicated `event_sources`
// table, no `replaceRecords` interaction).
func TestEventSourceBugRepro(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	eventSvc := NewEventService(soldierSvc)

	ev, err := eventSvc.CreateEvent(models.Soldier{Kind: "Battle"})
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}

	_, err = eventSvc.AttachSourceToEvent(ev.ID, models.Record{
		RecordType: "Pension Application",
		AppID:      "APP-1880-7701",
		Details:    "Filed 1880",
	})
	if err != nil {
		t.Fatalf("AttachSourceToEvent: %v", err)
	}

	ev.Kind = "Engagement"
	ev.Description = "Updated."
	if err := eventSvc.UpdateEvent(*ev); err != nil {
		t.Fatalf("UpdateEvent: %v", err)
	}

	after, err := eventSvc.ListSourcesForEvent(ev.ID)
	if err != nil {
		t.Fatalf("ListSourcesForEvent: %v", err)
	}
	t.Logf("sources after update: %d", len(after))
	if len(after) != 1 {
		t.Errorf("sources wiped on update: want 1, got %d", len(after))
	}
}
```

## Actual output on v60 (2026-07-04)

```
=== RUN   TestEventSourceBugRepro
    event_source_bug_test.go:38: sources after update: 0
    event_source_bug_test.go:40: sources wiped: want 1, got 0
--- FAIL: TestEventSourceBugRepro (0.08s)
FAIL
```

## Root cause

`internal/records/soldier_service.go:2086` `replaceRecords`
runs on every `SoldierService.Update`:

```go
if _, err := tx.Exec(`DELETE FROM records WHERE person_record_id = ?`, soldierID); err != nil {
    return err
}
// then re-insert from soldier.Records
```

This is **REPLACE-only semantics** — pre-existing for Person
Records where the edit form carries the full records list.

`internal/appshell/events_handlers.go:395` `parseEventForm`
does **not** populate `Records`. The Event edit form has no
source-records input field (sources are managed via the
separate panel). So `UpdateEvent` calls
`SoldierService.Update(event)` with `Records: nil`, and
`replaceRecords` deletes every row where `person_record_id =
event.ID` and inserts zero.

## Status

This scratch file lived at `internal/records/event_source_bug_test.go`
during the verification pass and was deleted once the failure
was confirmed. The real regression test
`TestEventService_UpdateEventPreservesAttachedSources` lands
in slice 3 of the v61 decomposition
(`docs/agents/notes/v61-event-sources-decomposition.md`).