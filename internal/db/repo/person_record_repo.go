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

	"github.com/valueforvalue/DixieData/internal/models"
)

// Execer is the minimal interface the write methods need
// from the caller's transaction context. Both *sql.DB and
// *sql.Tx satisfy it, so callers can either pass a tx (when
// composing multiple writes atomically) or a *sql.DB (when
// the write stands alone, like the slice-2 Delete path
// which is not part of any surrounding transaction).
type Execer interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

// Querier is the minimal interface the read methods need
// from the caller's transaction context. Both *sql.DB and
// *sql.Tx satisfy it. Slice 3's junction reads pass the
// caller's tx (or *sql.DB for standalone reads) so the
// read participates in the surrounding transaction.
type Querier interface {
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
}

// DBExecQuerier combines Execer + Querier. Both *sql.DB and
// *sql.Tx satisfy it; the slice-3 methods that compose reads
// + writes in one call (rare in practice, but the LinksForEvent
// path could grow one) use this combined interface.
type DBExecQuerier interface {
	Execer
	Querier
}

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

	// Create inserts a new Person Record and returns the
	// generated primary-key id via LastInsertId. The caller
	// supplies an Execer (typically *sql.Tx for atomicity
	// with replaceRecords, but *sql.DB works too when the
	// insert stands alone).
	Create(ctx context.Context, ex Execer, s models.Soldier) (int64, error)

	// Update modifies an existing Person Record identified
	// by s.ID and returns the rows-affected count. A return
	// of 0 with no error means no row matched; the service
	// layer is responsible for translating that to
	// ErrSoldierNotFound.
	Update(ctx context.Context, ex Execer, s models.Soldier) (int64, error)

	// Delete removes the Person Record identified by id
	// and returns the rows-affected count. A return of 0
	// with no error means no row matched.
	Delete(ctx context.Context, ex Execer, id int64) (int64, error)
}