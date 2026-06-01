package records

import (
	"slices"
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
)

func insertLegacyFilterSoldier(t *testing.T, d *db.DB, displayID, pensionState, confederateHomeStatus string) {
	t.Helper()
	if _, err := d.Conn().Exec(
		`INSERT INTO soldiers (display_id, is_generated, first_name, last_name, rank, unit, pension_state, confederate_home_status) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		displayID, false, "Legacy", displayID, "Private", "Test Unit", pensionState, confederateHomeStatus,
	); err != nil {
		t.Fatalf("insert legacy soldier %s: %v", displayID, err)
	}
}

func TestNormalizeBrowseRequestLeavesBlankPensionStateEmpty(t *testing.T) {
	normalized := normalizeBrowseRequest(BrowseRequest{})
	if normalized.PensionState != "" {
		t.Fatalf("blank pension filter normalized to %q", normalized.PensionState)
	}
}

func TestSoldierService_BrowsePageDoesNotDefaultBlankPensionStateToNA(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	if _, err := svc.Create(models.Soldier{FirstName: "Albert", LastName: "One", PensionState: "N/A"}); err != nil {
		t.Fatalf("create first: %v", err)
	}
	if _, err := svc.Create(models.Soldier{FirstName: "Benjamin", LastName: "Two", PensionState: "Texas"}); err != nil {
		t.Fatalf("create second: %v", err)
	}

	soldiers, total, normalized, err := svc.BrowsePage(BrowseRequest{})
	if err != nil {
		t.Fatalf("BrowsePage: %v", err)
	}
	if normalized.PensionState != "" {
		t.Fatalf("normalized pension filter = %q", normalized.PensionState)
	}
	if total != 2 || len(soldiers) != 2 {
		t.Fatalf("expected all records with blank pension filter, total=%d len=%d", total, len(soldiers))
	}
}

func TestSoldierService_BrowsePageMatchesLegacyNormalizedStatusValues(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	insertLegacyFilterSoldier(t, d, "LEGACY-00001", "None", "none")
	insertLegacyFilterSoldier(t, d, "LEGACY-00002", "NA", "")
	insertLegacyFilterSoldier(t, d, "LEGACY-00003", "N/A", "N/A")
	insertLegacyFilterSoldier(t, d, "LEGACY-00004", "Texas", "Resident")

	pensionMatches, pensionTotal, _, err := svc.BrowsePage(BrowseRequest{PensionState: "N/A"})
	if err != nil {
		t.Fatalf("BrowsePage pension_state=N/A: %v", err)
	}
	if pensionTotal != 3 || len(pensionMatches) != 3 {
		t.Fatalf("expected 3 normalized pension matches, total=%d len=%d", pensionTotal, len(pensionMatches))
	}

	confederateMatches, confederateTotal, _, err := svc.BrowsePage(BrowseRequest{ConfederateHomeStatus: "N/A"})
	if err != nil {
		t.Fatalf("BrowsePage confederate_home_status=N/A: %v", err)
	}
	if confederateTotal != 3 || len(confederateMatches) != 3 {
		t.Fatalf("expected 3 normalized confederate-home matches, total=%d len=%d", confederateTotal, len(confederateMatches))
	}
}

func TestSoldierService_BrowsePageFiltersByBuriedIn(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	if _, err := svc.Create(models.Soldier{FirstName: "Albert", LastName: "One", BuriedIn: "Oak Hill Cemetery"}); err != nil {
		t.Fatalf("create first: %v", err)
	}
	if _, err := svc.Create(models.Soldier{FirstName: "Benjamin", LastName: "Two", BuriedIn: "Maple Grove Cemetery"}); err != nil {
		t.Fatalf("create second: %v", err)
	}
	if _, err := svc.Create(models.Soldier{FirstName: "Charles", LastName: "Three", BuriedIn: "Oak Hill Cemetery"}); err != nil {
		t.Fatalf("create third: %v", err)
	}

	soldiers, total, normalized, err := svc.BrowsePage(BrowseRequest{BuriedIn: "Oak Hill Cemetery"})
	if err != nil {
		t.Fatalf("BrowsePage buried_in=Oak Hill Cemetery: %v", err)
	}
	if normalized.BuriedIn != "Oak Hill Cemetery" {
		t.Fatalf("normalized buried_in = %q", normalized.BuriedIn)
	}
	if total != 2 || len(soldiers) != 2 {
		t.Fatalf("expected 2 buried-in matches, total=%d len=%d", total, len(soldiers))
	}
}

func TestSoldierService_AdvancedSearchMatchesLegacyNormalizedStatusValues(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	insertLegacyFilterSoldier(t, d, "ADV-00001", "None", "none")
	insertLegacyFilterSoldier(t, d, "ADV-00002", "N/A", "N/A")
	insertLegacyFilterSoldier(t, d, "ADV-00003", "Texas", "Resident")

	pensionMatches, pensionTotal, err := svc.AdvancedSearch(models.SoldierSearch{PensionState: "N/A"}, 1, 50)
	if err != nil {
		t.Fatalf("AdvancedSearch pension_state=N/A: %v", err)
	}
	if pensionTotal != 2 || len(pensionMatches) != 2 {
		t.Fatalf("expected 2 advanced-search pension matches, total=%d len=%d", pensionTotal, len(pensionMatches))
	}

	confederateMatches, confederateTotal, err := svc.AdvancedSearch(models.SoldierSearch{ConfederateHomeStatus: "N/A"}, 1, 50)
	if err != nil {
		t.Fatalf("AdvancedSearch confederate_home_status=N/A: %v", err)
	}
	if confederateTotal != 2 || len(confederateMatches) != 2 {
		t.Fatalf("expected 2 advanced-search confederate-home matches, total=%d len=%d", confederateTotal, len(confederateMatches))
	}
}

func TestSoldierService_FormSuggestionsNormalizeLegacyPensionStates(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	insertLegacyFilterSoldier(t, d, "SUG-00001", "None", "")
	insertLegacyFilterSoldier(t, d, "SUG-00002", "NA", "")
	insertLegacyFilterSoldier(t, d, "SUG-00003", "Texas", "")

	suggestions, err := svc.FormSuggestions()
	if err != nil {
		t.Fatalf("FormSuggestions: %v", err)
	}
	if !slices.Equal(suggestions.PensionState, []string{"N/A", "Texas"}) {
		t.Fatalf("pension state suggestions = %#v", suggestions.PensionState)
	}
}
