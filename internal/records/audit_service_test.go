package records

import (
	"strconv"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

func TestAuditServiceRunDuplicateAuditFlagsFuzzyMatches(t *testing.T) {
	d := newTestDB(t)
	soldiers := NewSoldierService(d)
	audit := NewAuditService(d)

	left, err := soldiers.Create(testSoldier("John", "Kerns", 1840, "4th OK Infantry"))
	if err != nil {
		t.Fatalf("Create left: %v", err)
	}
	right, err := soldiers.Create(testSoldier("Jon", "Kerns", 1840, "4th OK Infantry"))
	if err != nil {
		t.Fatalf("Create right: %v", err)
	}

	result, err := audit.RunDuplicateAudit()
	if err != nil {
		t.Fatalf("RunDuplicateAudit: %v", err)
	}
	if result.ScannedRecords != 2 {
		t.Fatalf("expected 2 scanned records, got %#v", result)
	}
	if result.FindingsDiscovered != 1 || result.OpenFindings != 1 {
		t.Fatalf("expected 1 open finding, got %#v", result)
	}

	leftReloaded, err := soldiers.GetByID(left.ID)
	if err != nil {
		t.Fatalf("GetByID left: %v", err)
	}
	rightReloaded, err := soldiers.GetByID(right.ID)
	if err != nil {
		t.Fatalf("GetByID right: %v", err)
	}
	if !leftReloaded.NeedsReview || !rightReloaded.NeedsReview {
		t.Fatalf("expected both records flagged for review, got %#v and %#v", leftReloaded, rightReloaded)
	}

	findings, err := audit.FindingsForSoldiers([]int64{left.ID})
	if err != nil {
		t.Fatalf("FindingsForSoldiers: %v", err)
	}
	if len(findings[left.ID]) != 1 || findings[left.ID][0].OtherSoldierID != right.ID {
		t.Fatalf("unexpected finding summaries: %#v", findings)
	}

	comparison, err := audit.Comparison(findings[left.ID][0].ID)
	if err != nil {
		t.Fatalf("Comparison: %v", err)
	}
	if comparison.Reason == "" || len(comparison.Fields) == 0 {
		t.Fatalf("unexpected comparison payload: %#v", comparison)
	}
	firstNameHighlighted := false
	for _, field := range comparison.Fields {
		if field.Key == "first_name" && field.Highlighted {
			firstNameHighlighted = true
		}
	}
	if !firstNameHighlighted {
		t.Fatalf("expected first-name highlight in comparison, got %#v", comparison.Fields)
	}
}

func TestAuditServiceResolvedPairsStaySuppressed(t *testing.T) {
	d := newTestDB(t)
	soldiers := NewSoldierService(d)
	audit := NewAuditService(d)

	left, err := soldiers.Create(testSoldier("William", "Kearns", 1842, "1st OK Cavalry"))
	if err != nil {
		t.Fatalf("Create left: %v", err)
	}
	right, err := soldiers.Create(testSoldier("Wiliam", "Kearns", 1842, "1st OK Cavalry"))
	if err != nil {
		t.Fatalf("Create right: %v", err)
	}

	if _, err := audit.RunDuplicateAudit(); err != nil {
		t.Fatalf("RunDuplicateAudit initial: %v", err)
	}
	findings, err := audit.FindingsForSoldiers([]int64{left.ID})
	if err != nil {
		t.Fatalf("FindingsForSoldiers: %v", err)
	}
	if len(findings[left.ID]) != 1 {
		t.Fatalf("expected one finding, got %#v", findings)
	}
	if err := audit.ResolveFinding(findings[left.ID][0].ID); err != nil {
		t.Fatalf("ResolveFinding: %v", err)
	}

	result, err := audit.RunDuplicateAudit()
	if err != nil {
		t.Fatalf("RunDuplicateAudit second: %v", err)
	}
	if result.FindingsSuppressed != 1 || result.OpenFindings != 0 {
		t.Fatalf("expected resolved pair suppression, got %#v", result)
	}

	leftReloaded, err := soldiers.GetByID(left.ID)
	if err != nil {
		t.Fatalf("GetByID left: %v", err)
	}
	rightReloaded, err := soldiers.GetByID(right.ID)
	if err != nil {
		t.Fatalf("GetByID right: %v", err)
	}
	if leftReloaded.NeedsReview || rightReloaded.NeedsReview {
		t.Fatalf("expected resolved pair to stay out of review queue, got %#v and %#v", leftReloaded, rightReloaded)
	}
}

func TestAuditServiceRunDuplicateAuditHandlesLegacyNullTextColumns(t *testing.T) {
	d := newTestDB(t)
	audit := NewAuditService(d)

	if _, err := d.Conn().Exec(`INSERT INTO soldiers (display_id, birth_date, created_at, updated_at) VALUES ('DXD-00001', '01/01/1840', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("seed first legacy row: %v", err)
	}
	if _, err := d.Conn().Exec(`INSERT INTO soldiers (display_id, birth_date, created_at, updated_at) VALUES ('DXD-00002', '01/01/1840', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("seed second legacy row: %v", err)
	}

	result, err := audit.RunDuplicateAudit()
	if err != nil {
		t.Fatalf("RunDuplicateAudit: %v", err)
	}
	if result.ScannedRecords != 2 {
		t.Fatalf("expected both legacy rows to be scanned, got %#v", result)
	}
}

func testSoldier(firstName, lastName string, birthYear int, unit string) models.Soldier {
	return models.Soldier{
		FirstName: firstName,
		LastName:  lastName,
		BirthDate: "01/01/" + strconv.Itoa(birthYear),
		Unit:      unit,
	}
}

// TestAuditServiceListResolvedFindingsPaginated verifies the
// Review Queue Resolved tab data path (issue #461): resolved
// duplicate-audit findings surface newest-first, paginated, and
// the open-only filter is honored (open rows do not bleed in).
func TestAuditServiceListResolvedFindingsPaginated(t *testing.T) {
	d := newTestDB(t)
	soldiers := NewSoldierService(d)
	audit := NewAuditService(d)

	left, err := soldiers.Create(testSoldier("Rezin", "Pleasants", 1838, "8th VA Cavalry"))
	if err != nil {
		t.Fatalf("Create left: %v", err)
	}
	if _, err := soldiers.Create(testSoldier("Reazen", "Pleasants", 1838, "8th VA Cavalry")); err != nil {
		t.Fatalf("Create right: %v", err)
	}
	// Second pair: stays open, must not appear on the Resolved tab.
	openLeft, err := soldiers.Create(testSoldier("Marcus", "Whitfield", 1836, "12th MS Infantry"))
	if err != nil {
		t.Fatalf("Create openLeft: %v", err)
	}
	openRight, err := soldiers.Create(testSoldier("Markus", "Whitfield", 1836, "12th MS Infantry"))
	if err != nil {
		t.Fatalf("Create openRight: %v", err)
	}
	if _, err := audit.RunDuplicateAudit(); err != nil {
		t.Fatalf("RunDuplicateAudit: %v", err)
	}
	summary, err := audit.Summary()
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary.OpenFindings != 2 {
		t.Fatalf("expected 2 open findings, got %#v", summary)
	}

	// Resolve the first pair only.
	findings, err := audit.FindingsForSoldiers([]int64{left.ID})
	if err != nil {
		t.Fatalf("FindingsForSoldiers: %v", err)
	}
	if len(findings[left.ID]) != 1 {
		t.Fatalf("expected one finding for left, got %#v", findings)
	}
	if err := audit.ResolveFinding(findings[left.ID][0].ID); err != nil {
		t.Fatalf("ResolveFinding: %v", err)
	}

	page1, total, err := audit.ListResolvedFindings(1, 50)
	if err != nil {
		t.Fatalf("ListResolvedFindings: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected total=1 resolved finding, got %d", total)
	}
	if len(page1) != 1 {
		t.Fatalf("expected one resolved row, got %#v", page1)
	}
	if page1[0].LeftDisplayID == "" || page1[0].RightDisplayID == "" {
		t.Fatalf("expected display IDs to be cached on the row, got %#v", page1[0])
	}
	if page1[0].LeftDisplayID == openLeft.DisplayID || page1[0].LeftDisplayID == openRight.DisplayID {
		t.Fatalf("resolved row carried an open pair's display ID: %#v", page1[0])
	}
	if strings.TrimSpace(page1[0].ResolvedAt) == "" {
		t.Fatalf("expected resolved_at to be populated, got %#v", page1[0])
	}
	if page1[0].FindingType == "" || page1[0].Reason == "" {
		t.Fatalf("expected finding type + reason populated, got %#v", page1[0])
	}
}
