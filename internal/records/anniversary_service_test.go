package records

import (
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

func TestAnniversaryService_GetByMonthDay(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	annSvc := NewAnniversaryService(d)

	// Create soldiers with specific death dates
	_, err := soldierSvc.Create(models.Soldier{
		FirstName: "A", LastName: "One",
		DeathMonth: 4, DeathDay: 12, DeathYear: 1865,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err = soldierSvc.Create(models.Soldier{
		FirstName: "B", LastName: "Two",
		DeathMonth: 4, DeathDay: 12, DeathYear: 1862,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err = soldierSvc.Create(models.Soldier{
		FirstName: "C", LastName: "Three",
		DeathMonth: 7, DeathDay: 3, DeathYear: 1863,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	soldiers, err := annSvc.GetByMonthDay(4, 12)
	if err != nil {
		t.Fatalf("GetByMonthDay: %v", err)
	}
	if len(soldiers) != 2 {
		t.Errorf("got %d soldiers, want 2", len(soldiers))
	}
}

func TestAnniversaryService_GetByMonthDay_DayZero(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	annSvc := NewAnniversaryService(d)

	// Month-wide anniversary (day=0 means unknown day)
	_, err := soldierSvc.Create(models.Soldier{
		FirstName: "Unknown", LastName: "Day",
		DeathMonth: 6, DeathDay: 0, DeathYear: 1864,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Specific day in same month — should not appear in day=0 query
	_, err = soldierSvc.Create(models.Soldier{
		FirstName: "Known", LastName: "Day",
		DeathMonth: 6, DeathDay: 15, DeathYear: 1864,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Query for month-wide (day=0)
	monthWide, err := annSvc.GetByMonthDay(6, 0)
	if err != nil {
		t.Fatalf("GetByMonthDay day=0: %v", err)
	}
	if len(monthWide) != 1 {
		t.Errorf("got %d month-wide, want 1", len(monthWide))
	}

	// Query for specific day
	daySpecific, err := annSvc.GetByMonthDay(6, 15)
	if err != nil {
		t.Fatalf("GetByMonthDay day=15: %v", err)
	}
	if len(daySpecific) != 1 {
		t.Errorf("got %d day-specific, want 1", len(daySpecific))
	}
}

func TestAnniversaryService_GetMonthCalendar(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	annSvc := NewAnniversaryService(d)

	_, _ = soldierSvc.Create(models.Soldier{FirstName: "A", LastName: "B", DeathMonth: 3, DeathDay: 1})
	_, _ = soldierSvc.Create(models.Soldier{FirstName: "C", LastName: "D", DeathMonth: 3, DeathDay: 1})
	_, _ = soldierSvc.Create(models.Soldier{FirstName: "E", LastName: "F", DeathMonth: 3, DeathDay: 15})
	_, _ = soldierSvc.Create(models.Soldier{FirstName: "G", LastName: "H", DeathMonth: 3, DeathDay: 0}) // month-wide
	_, _ = soldierSvc.Create(models.Soldier{FirstName: "I", LastName: "J", DeathMonth: 4, DeathDay: 1}) // different month

	cal, err := annSvc.GetMonthCalendar(3)
	if err != nil {
		t.Fatalf("GetMonthCalendar: %v", err)
	}

	if len(cal[1]) != 2 {
		t.Errorf("day 1 has %d soldiers, want 2", len(cal[1]))
	}
	if len(cal[15]) != 1 {
		t.Errorf("day 15 has %d soldiers, want 1", len(cal[15]))
	}
	if len(cal[0]) != 1 {
		t.Errorf("month-wide (day 0) has %d soldiers, want 1", len(cal[0]))
	}
	// April soldier should not appear in March calendar
	if _, ok := cal[4]; ok {
		t.Errorf("calendar has key for wrong month")
	}
}
