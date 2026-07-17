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
// right_record_id. Returns *sql.Rows the caller must Close.
func (r *AuditRecordRepo) FindingsForRecordIDs(ctx context.Context, q repo.Querier, recordIDs []int64) (*sql.Rows, error) {
	if len(recordIDs) == 0 {
		recordIDs = []int64{0}
	}
	args := make([]interface{}, 0, len(recordIDs)*2)
	orClauses := make([]string, 0, len(recordIDs)*2)
	for _, id := range recordIDs {
		orClauses = append(orClauses, "left_record_id = ?", "right_record_id = ?")
		args = append(args, id, id)
	}
	query := `SELECT ` + AuditRecordSelectColumns + ` FROM duplicate_audit_findings WHERE ` + strings.Join(orClauses, " OR ") + ` ORDER BY id`
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