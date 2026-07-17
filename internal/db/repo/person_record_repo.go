// Package repo declares the repository interfaces that decouple
// DixieData's domain services (internal/records/, internal/articles/,
// etc.) from the concrete database driver.
//
// Slice 1 of issue #613 introduces the seam for the Person Record
// table (the `soldiers` table in the canonical schema). Future
// slices add EventRepo, ArticleRepo, TagRepo, AuditRepo, etc.
// behind the same pattern.
//
// Design contract:
//
//   - Repos own SQL. They return *sql.Row / *sql.Rows + a count
//     where applicable. Callers (services) own scan + domain
//     normalization.
//
//   - Repos return *sql.Row / *sql.Rows (not *models.Soldier)
//     so the scan-helper + domain-normalization step stays
//     inside the consuming service. This keeps domain packages
//     (pensionstate, confederatehomestatus, dates) out of the
//     repo package — the repo is pure data access.
//
//   - Column lists are exported from the repo package so scan
//     helpers in the consuming service stay in sync with what
//     the repo's SELECT emits.
//
//   - Concrete impls live under subpackages
//     (internal/db/repo/sqlite/ for the SQLite-backed impl).
//     Postgres / in-memory / fake impls land in their own
//     subpackages in future slices.
//
//   - Context-aware: every method takes context.Context. The
//     SQLite impl ignores it today (the underlying driver does
//     not honor ctx), but the signature future-proofs the
//     seam for drivers that do.
package repo

import (
	"context"
	"database/sql"
)

// PersonRecordRepo is the data-access seam for the Person Record
// table (canonical table name: `soldiers`). Slice 1 covers the
// two read methods the browse + soldier-detail surfaces need;
// future slices add the write methods + the cross-table joins
// (Records, Images, Spouse lookup).
//
// The interface returns *sql.Row / *sql.Rows + counts so the
// calling service can do its own scan + domain normalization.
// See package doc for rationale.
type PersonRecordRepo interface {
	// GetByID returns the Person Record row with the given
	// primary-key ID. Returns sql.ErrNoRows if no row matches.
	// The returned *sql.Row must be scanned with the
	// PersonRecordSelectColumns scan dest list (exported as
	// a constant on the impl package for type-safety).
	GetByID(ctx context.Context, id int64) (*sql.Row, error)

	// List returns a paginated slice of Person Records ordered
	// by last_name, first_name, plus the total count of rows
	// in the table (across all pages). The caller passes
	// page (1-indexed) and pageSize. Returns *sql.Rows that
	// the caller must Close.
	List(ctx context.Context, page, pageSize int) (*sql.Rows, int, error)
}