// Package repo declares the repository interfaces that decouple
// DixieData's domain services (internal/records/, internal/articles/,
// etc.) from the concrete database driver.
//
// Slice 4 of issue #613 adds the ArticleRecordRepo seam.
//
// Articles live in their own `articles` table (created by
// migration block-3-articles). Slice 4 covers the 5
// highest-coupling inline SQL paths in
// internal/records/article_service.go: the read paths
// (GetByID, List) and the write paths (Create, Update,
// Delete). Cross-table methods (refs, snapshots, images)
// stay in ArticleService for slice 5+ if at all.
//
// Design contract (same as PersonRecordRepo +
// EventRecordRepo):
//
//   - Repos own SQL. Return *sql.Row / *sql.Rows for
//     collections; return int64 for write-result counts.
//   - Context-aware: every method takes context.Context.
//   - Concrete impls live under subpackages
//     (internal/db/repo/sqlite/ for SQLite-backed).
package repo

import (
	"context"
	"database/sql"

	"github.com/valueforvalue/DixieData/internal/models"
)

// ArticleRecordRepo is the data-access seam for the
// `articles` table.
//
// Slice 4 covers the 5 highest-coupling paths:
// Create / GetByID / List / Update / Delete. Future slices
// add cross-table methods (refs, snapshots, images) as
// needed.
type ArticleRecordRepo interface {
	// Create inserts a new Article and returns the generated
	// primary-key id. The caller supplies an Execer
	// (typically *sql.Tx for atomicity with refs / snapshots).
	Create(ctx context.Context, ex Execer, a models.Article) (int64, error)

	// GetByID returns the Article row with the given
	// primary-key ID. Returns sql.ErrNoRows if no row matches.
	GetByID(ctx context.Context, id int64) (*sql.Row, error)

	// List returns a paginated slice of Articles ordered by
	// updated_at DESC, id DESC, plus the total count. The
	// caller must Close the returned *sql.Rows.
	List(ctx context.Context, page, pageSize int) (*sql.Rows, int, error)

	// Update modifies an existing Article identified by
	// a.ID and returns the rows-affected count.
	Update(ctx context.Context, ex Execer, a models.Article) (int64, error)

	// Delete removes the Article identified by id and returns
	// the rows-affected count. The legacy Delete filters on
	// `is_snapshot = 0` so snapshots are preserved by the
	// schema's ON DELETE CASCADE rules.
	Delete(ctx context.Context, ex Execer, id int64) (int64, error)
}