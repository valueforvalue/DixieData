// Package repo declares the repository interfaces that decouple
// DixieData's domain services from the concrete database driver.
//
// Slice 7 of issue #613 adds the AuditRecordRepo seam.
//
// Audit data lives in `duplicate_audit_findings` (the
// per-Person-Record duplicate-candidate list) +
// `system_config` (for the similarity threshold). Slice 7
// covers 4 methods — the simpler CRUD surface.
// RunDuplicateAudit (the heavy cursor-loop write path that
// scans the whole soldiers table + writes per-finding
// inserts) stays in AuditService for slice 8+ if at all.
package repo

import (
	"context"
	"database/sql"
)

// AuditRecordRepo is the data-access seam for the duplicate
// audit findings table. Slice 7 covers the 2 read methods;
// the resolve methods (which compose cross-table sync via
// syncSoldierDuplicateReviewStateTx) stay in AuditService
// for slice 8+ if at all.
type AuditRecordRepo interface {
	// FindingsForRecordIDs returns the findings where the
	// given record id appears as either left_record_id or
	// right_record_id. Returns *sql.Rows the caller must
	// Close.
	FindingsForRecordIDs(ctx context.Context, q Querier, recordIDs []int64) (*sql.Rows, error)

	// ListResolvedFindings returns paginated resolved findings
	// ordered by resolved_at DESC, id DESC, plus the total
	// resolved-finding count.
	ListResolvedFindings(ctx context.Context, q Querier, page, pageSize int) (*sql.Rows, int, error)
}