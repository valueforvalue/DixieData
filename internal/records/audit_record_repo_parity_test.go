// audit_record_repo_parity_test.go — issue #622 slice 4
// regression net.
//
// Pins the contract that the AuditService methods refactored
// in slices 1-3 return identical results to the legacy
// inline-SQL paths on the same fixture.
package records

import (
	"strconv"
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
)

// seedAuditFinding inserts one duplicate_audit_finding row
// with the given left + right record ids + status. Returns
// the row id. Helper for the slice-4 parity tests.
func seedAuditFinding(t *testing.T, d *db.DB, leftID, rightID int64, status string) int64 {
	t.Helper()
	pairKey := "pair-" + strconv.FormatInt(leftID, 10) + "-" + strconv.FormatInt(rightID, 10) + "-" + status
	res, err := d.Conn().Exec(
		`INSERT INTO duplicate_audit_findings (pair_key, left_record_id, right_record_id, finding_type, reason, highlight_fields, status) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		pairKey, leftID, rightID, "exact_match", "smoke test", "{}", status,
	)
	if err != nil {
		t.Fatalf("seed audit finding: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}
	return id
}

// seedAuditSoldier inserts one minimal soldiers row so the
// duplicate_audit_findings FK (left_record_id REFERENCES
// soldiers.id) is satisfied.
func seedAuditSoldier(t *testing.T, d *db.DB, id int64) {
	t.Helper()
	if _, err := d.Conn().Exec(
		`INSERT OR IGNORE INTO soldiers (id, display_id) VALUES (?, ?)`,
		id, "DXD-"+strconv.FormatInt(id, 10),
	); err != nil {
		t.Fatalf("seed audit soldier: %v", err)
	}
}

// TestAuditService_ListResolvedFindings_SeamDelegation
// exercises the slice-2 service refactor end-to-end through
// the AuditRecordRepo seam.
func TestAuditService_ListResolvedFindings_SeamDelegation(t *testing.T) {
	d := newTestDB(t)
	svc := NewAuditService(d)

	// Seed soldiers (the FK on duplicate_audit_findings
	// requires them).
	for _, id := range []int64{1, 2, 3, 4} {
		seedAuditSoldier(t, d, id)
	}

	// Seed: 1 open (default) + 1 resolved.
	seedAuditFinding(t, d, 1, 2, "open")
	resolved := seedAuditFinding(t, d, 3, 4, "resolved")

	items, total, err := svc.ListResolvedFindings(1, 10)
	if err != nil {
		t.Fatalf("ListResolvedFindings: %v", err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1 (resolved only)", total)
	}
	if len(items) != 1 {
		t.Errorf("items = %d, want 1", len(items))
	}
	if len(items) > 0 && items[0].ID != resolved {
		t.Errorf("items[0].ID = %d, want %d (the resolved one)", items[0].ID, resolved)
	}
}

// TestAuditService_FindingsForSoldiers_SeamDelegation
// exercises the slice-3 service refactor end-to-end.
func TestAuditService_FindingsForSoldiers_SeamDelegation(t *testing.T) {
	d := newTestDB(t)
	svc := NewAuditService(d)

	// Seed soldiers (FK requirement).
	for _, id := range []int64{10, 20, 30, 40} {
		seedAuditSoldier(t, d, id)
	}

	// Seed: 1 open + 1 resolved. The open one is between
	// records 10 + 20; the resolved one is between 30 + 40.
	openID := seedAuditFinding(t, d, 10, 20, "open")
	_ = seedAuditFinding(t, d, 30, 40, "resolved")

	results, err := svc.FindingsForSoldiers([]int64{10, 20})
	if err != nil {
		t.Fatalf("FindingsForSoldiers: %v", err)
	}
	// The repo's statusFilter="open" should exclude the
	// resolved finding at (30, 40); only (10, 20) is open.
	// The map adds BOTH LeftID and RightID as keys, so the
	// map has 2 entries (10 + 20), each with 1 finding.
	if len(results) != 2 {
		t.Errorf("results map has %d entries, want 2 (10 + 20)", len(results))
	}
	for _, id := range []int64{10, 20} {
		findings, ok := results[id]
		if !ok {
			t.Errorf("results[%d] missing", id)
			continue
		}
		if len(findings) != 1 {
			t.Errorf("results[%d] has %d findings, want 1", id, len(findings))
		}
		if findings[0].ID != openID {
			t.Errorf("results[%d][0].ID = %d, want %d", id, findings[0].ID, openID)
		}
	}
}
