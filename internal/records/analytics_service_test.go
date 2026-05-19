package records

import (
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

func TestAnalyticsService_Snapshot(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)
	analytics := NewAnalyticsService(d)

	soldier, err := svc.Create(models.Soldier{
		FirstName:             "John",
		LastName:              "Taylor",
		BirthDate:             "00/00/1834",
		DeathDate:             "00/00/1864",
		BuriedIn:              "Oak Hill Cemetery",
		ConfederateHomeStatus: "Inmate",
		ConfederateHomeName:   "Texas Confederate Home",
		PensionState:          "Texas",
		Unit:                  "1st Texas Infantry",
	})
	if err != nil {
		t.Fatalf("Create soldier: %v", err)
	}
	if _, err := svc.Create(models.Soldier{
		FirstName:             "Samuel",
		LastName:              "Carter",
		BirthDate:             "00/00/1841",
		DeathDate:             "00/00/1902",
		BuriedIn:              "Oak Hill Cemetery",
		ConfederateHomeStatus: "Trustee",
		ConfederateHomeName:   "Texas Confederate Home",
		PensionState:          "Texas",
		Unit:                  "1st Texas Infantry",
	}); err != nil {
		t.Fatalf("Create second soldier: %v", err)
	}
	if _, err := svc.Create(models.Soldier{
		EntryType:       "widow",
		SpouseSoldierID: soldier.ID,
		FirstName:       "Martha",
		LastName:        "Taylor",
		BirthDate:       "00/00/1840",
		DeathDate:       "00/00/1911",
		BuriedIn:        "Rose Hill Cemetery",
		PensionState:    "Arkansas",
	}); err != nil {
		t.Fatalf("Create widow: %v", err)
	}

	snapshot, err := analytics.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if snapshot.RecordTypes.TotalSoldiers != 2 || snapshot.RecordTypes.TotalWivesWidows != 1 {
		t.Fatalf("unexpected record type counts: %#v", snapshot.RecordTypes)
	}
	if len(snapshot.CemeteryDensity) == 0 || snapshot.CemeteryDensity[0].Label != "Oak Hill Cemetery" || snapshot.CemeteryDensity[0].Count != 2 {
		t.Fatalf("unexpected cemetery density: %#v", snapshot.CemeteryDensity)
	}
	if len(snapshot.ConfederateHomeStatus) == 0 || snapshot.ConfederateHomeStatus[0].Count < 1 {
		t.Fatalf("unexpected Confederate Home status counts: %#v", snapshot.ConfederateHomeStatus)
	}
	if len(snapshot.ConfederateHomeNames) == 0 || snapshot.ConfederateHomeNames[0].Label != "Texas Confederate Home" {
		t.Fatalf("unexpected Confederate Home names: %#v", snapshot.ConfederateHomeNames)
	}
	if len(snapshot.PensionDistribution) == 0 || snapshot.PensionDistribution[0].Label != "Texas" {
		t.Fatalf("unexpected pension distribution: %#v", snapshot.PensionDistribution)
	}
	if len(snapshot.UnitRepresentation) == 0 || snapshot.UnitRepresentation[0].Label != "1st Texas Infantry" {
		t.Fatalf("unexpected unit representation: %#v", snapshot.UnitRepresentation)
	}
	if len(snapshot.BirthDecadeDistribution) == 0 || snapshot.BirthDecadeDistribution[0].Label != "1830s" {
		t.Fatalf("unexpected birth decades: %#v", snapshot.BirthDecadeDistribution)
	}
	if len(snapshot.DeathDecadeDistribution) == 0 || snapshot.DeathDecadeDistribution[0].Label != "1860s" {
		t.Fatalf("unexpected death decades: %#v", snapshot.DeathDecadeDistribution)
	}
}
