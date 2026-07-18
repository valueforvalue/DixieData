// Package sqlite provides the SQLite-backed implementations of
// the repository interfaces declared in internal/db/repo.
//
// This file ships the SQLite impl of AuditRecordRepo (issue
// #613 slice 7). The canonical `duplicate_audit_findings`
// schema uses left_record_id / right_record_id (not
// soldier_id / candidate_soldier_id) + status enum
// (open / resolved). Slice 7 covers the 2 read methods;
// resolve methods stay in AuditService.
package sqlite

import (
	"context"
	"database/sql"
	"strings"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/db/repo"
)

// AuditRecordSelectColumns is the SELECT column list used by
// AuditRecordRepo (8 columns matching the canonical schema).
const AuditRecordSelectColumns = `id, pair_key, left_record_id, right_record_id, finding_type, reason, highlight_fields, status, created_at, last_detected_at, resolved_at`

// AuditRecordRepo is the SQLite-backed implementation of
// repo.AuditRecordRepo.
type AuditRecordRepo struct {
	db *db.DB
}

var _ repo.AuditRecordRepo = (*AuditRecordRepo)(nil)

// NewAuditRecordRepo constructs a SQLite-backed
// AuditRecordRepo.
func NewAuditRecordRepo(d *db.DB) *AuditRecordRepo {
	return &AuditRecordRepo{db: d}
}

// FindingsForRecordIDs returns the findings where the given
// record id appears as either left_record_id or
// right_record_id. The statusFilter argument scopes the
// result set to a single status value ("" = no filter).
// Returns *sql.Rows the caller must Close.
func (r *AuditRecordRepo) FindingsForRecordIDs(ctx context.Context, q repo.Querier, recordIDs []int64, statusFilter string) (*sql.Rows, error) {
	if len(recordIDs) == 0 {
		recordIDs = []int64{0}
	}
	args := make([]interface{}, 0, len(recordIDs)*2)
	orClauses := make([]string, 0, len(recordIDs)*2)
	for _, id := range recordIDs {
		orClauses = append(orClauses, "left_record_id = ?", "right_record_id = ?")
		args = append(args, id, id)
	}
	// Wrap the OR chain in parens so the status filter (added
	// after, as `AND status = ?`) binds to the entire OR
	// group, not just the last branch. Without the parens,
	// SQLite's operator precedence evaluates `AND` before
	// `OR`, so the status filter would only apply to the
	// last branch — the earlier branches would match
	// regardless of status.
	whereClause := "(" + strings.Join(orClauses, " OR ") + ")"
	if statusFilter != "" {
		whereClause += " AND status = ?"
		args = append(args, statusFilter)
	}
	query := `SELECT ` + AuditRecordSelectColumns + ` FROM duplicate_audit_findings WHERE ` + whereClause + ` ORDER BY id`
	var rows *sql.Rows
	if err := db.WithBusyRetry(3, func() error {
		qr, qErr := q.QueryContext(ctx, query, args...)
		if qErr != nil {
			return qErr
		}
		rows = qr
		return nil
	}); err != nil {
		return nil, err
	}
	return rows, nil
}

// ListResolvedFindings returns paginated resolved findings
// (status = 'resolved') ordered by resolved_at DESC, id
// DESC, plus the total resolved-finding count.
func (r *AuditRecordRepo) ListResolvedFindings(ctx context.Context, q repo.Querier, page, pageSize int) (*sql.Rows, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 25
	}
	var total int
	if err := db.WithBusyRetry(3, func() error {
		return q.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM duplicate_audit_findings WHERE status = 'resolved'`,
		).Scan(&total)
	}); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	var rows *sql.Rows
	if err := db.WithBusyRetry(3, func() error {
		qr, qErr := q.QueryContext(ctx,
			`SELECT `+AuditRecordSelectColumns+` FROM duplicate_audit_findings WHERE status = 'resolved' ORDER BY resolved_at DESC, id DESC LIMIT ? OFFSET ?`,
			pageSize, offset,
		)
		if qErr != nil {
			return qErr
		}
		rows = qr
		return nil
	}); err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// ListResolvedFindingsEnriched returns paginated resolved
// findings with the LEFT JOINed display_id columns from
// the soldiers table. The 8-col row shape matches the
// legacy audit_service.go::ListResolvedFindings scan dest
// so the service refactor is a 1:1 swap.
func (r *AuditRecordRepo) ListResolvedFindingsEnriched(ctx context.Context, q repo.Querier, page, pageSize int) (*sql.Rows, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 25
	}
	var total int
	if err := db.WithBusyRetry(3, func() error {
		return q.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM duplicate_audit_findings WHERE status = 'resolved'`,
		).Scan(&total)
	}); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	var rows *sql.Rows
	if err := db.WithBusyRetry(3, func() error {
		qr, qErr := q.QueryContext(ctx,
			`SELECT
				d.id,
				d.left_record_id,
				d.right_record_id,
				COALESCE(l.display_id, ''),
				COALESCE(r.display_id, ''),
				d.finding_type,
				d.reason,
				COALESCE(d.resolved_at, '')
			FROM duplicate_audit_findings d
			LEFT JOIN soldiers l ON l.id = d.left_record_id
			LEFT JOIN soldiers r ON r.id = d.right_record_id
			WHERE d.status = 'resolved'
			ORDER BY COALESCE(d.resolved_at, d.created_at) DESC, d.id DESC
			LIMIT ? OFFSET ?`,
			pageSize, offset,
		)
		if qErr != nil {
			return qErr
		}
		rows = qr
		return nil
	}); err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}