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
	// right_record_id. The statusFilter argument scopes
	// the result set to a single status value (e.g.
	// "open", "resolved", or "" for no filter). Returns
	// *sql.Rows the caller must Close.
	FindingsForRecordIDs(ctx context.Context, q Querier, recordIDs []int64, statusFilter string) (*sql.Rows, error)

	// ListResolvedFindings returns paginated resolved findings
	// ordered by resolved_at DESC, id DESC, plus the total
	// resolved-finding count.
	ListResolvedFindings(ctx context.Context, q Querier, page, pageSize int) (*sql.Rows, int, error)

	// ListResolvedFindingsEnriched returns the same shape as
	// ListResolvedFindings but with the LEFT JOINed
	// display_id columns from the soldiers table for both
	// the left and right record ids. The 8-col row shape
	// (id, left_record_id, right_record_id, left_display_id,
	// right_display_id, finding_type, reason, resolved_at)
	// matches the legacy audit_service.go::ListResolvedFindings
	// scan dest so the service refactor is a 1:1 swap.
	// The JOIN is part of the slice-8 contract because the
	// legacy service never read the table-pure subset;
	// surfacing the JOIN at the repo keeps the service
	// table-agnostic.
	ListResolvedFindingsEnriched(ctx context.Context, q Querier, page, pageSize int) (*sql.Rows, int, error)
}