package records

import (
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

// TestRunDataQualityScan_EventRowsDoNotFireIdentityMissing (issue #530)
// is the regression net for the false positive where every event row
// (entry_type = 'event', no first_name/last_name by design) was
// flagged with Identity & Naming / identity-missing.
//
// The test seeds three rows in one archive:
//   - a fully-named soldier (control: must NOT fire identity-missing)
//   - a widow with no name (must STILL fire identity-missing because
//     widow IS a person-bearing entry type — the gate must not over-fire)
//   - an event record (must NOT fire identity-missing — the bug shape)
//
// Then runs the high-confidence scan and asserts on the per-row issue
// codes. A regression that drops the gate re-introduces the
// false-positive for the event row; a regression that over-fires
// (e.g. blanks the gate entirely) breaks the widow assertion.
func TestRunDataQualityScan_EventRowsDoNotFireIdentityMissing(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)
	events := NewEventService(svc)

	// Control: a fully-named soldier row.
	if _, err := svc.Create(models.Soldier{
		FirstName: "James",
		LastName:  "Carter",
	}); err != nil {
		t.Fatalf("Create soldier: %v", err)
	}

	// Person-bearing but unnamed: widow with no first_name/last_name.
	// Must STILL fire identity-missing — the gate must only block
	// non-person-bearing types.
	husband, err := svc.Create(models.Soldier{
		FirstName: "Thomas",
		LastName:  "Walker",
	})
	if err != nil {
		t.Fatalf("Create husband: %v", err)
	}
	if _, err := svc.Create(models.Soldier{
		EntryType:       models.EntryTypeWidow,
		SpouseSoldierID: husband.ID,
		// intentionally blank first_name + last_name
	}); err != nil {
		t.Fatalf("Create widow: %v", err)
	}

	// Non-person-bearing: event record with kind + begin_date set.
	if _, err := events.CreateEvent(models.Soldier{
		EntryType: models.EntryTypeEvent,
		Kind:      "Battle",
		BeginDate: "07/01/1862",
		EndDate:   "07/03/1862",
	}); err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}

	result, err := svc.RunDataQualityScan(string(DataQualityModeHighConfidence))
	if err != nil {
		t.Fatalf("RunDataQualityScan: %v", err)
	}

	// Group issues by (entry_type, code) for assertions.
	hasIssue := func(entryType, code string) bool {
		for _, issue := range result.Issues {
			if issue.EntryType == entryType && issue.Code == code {
				return true
			}
		}
		return false
	}

	// Bug-shape assertion: event rows must NOT fire identity-missing.
	if hasIssue(models.EntryTypeEvent, "identity-missing") {
		t.Errorf("event rows fired identity-missing; the #530 gate regressed")
		for _, issue := range result.Issues {
			if issue.EntryType == models.EntryTypeEvent && issue.Code == "identity-missing" {
				t.Logf("  false positive: %s (%s)", issue.DisplayID, issue.Name)
			}
		}
	}

	// Over-fire guard: widow with no name must STILL fire
	// identity-missing (widow IS a person-bearing entry type).
	if !hasIssue(models.EntryTypeWidow, "identity-missing") {
		t.Errorf("widow with no name did not fire identity-missing; the gate over-fires")
	}

	// Sanity: a fully-named soldier does not fire identity-missing.
	if hasIssue(models.EntryTypeSoldier, "identity-missing") {
		t.Errorf("named soldier fired identity-missing; the scan is over-eager")
	}
}

// TestIsPersonBearingEntryType (issue #530) pins the helper semantics
// in internal/models. The data-quality scan's Identity & Naming gate
// is the primary consumer; a change to the allowed set ripples to
// every scan run, so the contract is captured here as a regression net.
func TestIsPersonBearingEntryType(t *testing.T) {
	cases := []struct {
		entryType string
		want      bool
	}{
		{models.EntryTypeSoldier, true},
		{models.EntryTypeWife, true},
		{models.EntryTypeWidow, true},
		{models.EntryTypeLinkedPerson, true},
		{models.EntryTypeEvent, false},
		{"", false},
		{"unknown", false},
	}
	for _, c := range cases {
		if got := models.IsPersonBearingEntryType(c.entryType); got != c.want {
			t.Errorf("IsPersonBearingEntryType(%q) = %v, want %v", c.entryType, got, c.want)
		}
	}
}
