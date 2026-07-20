// memorial_import_repo.go — issue #625 slice 9b.
//
// SQLite-backed implementation of repo.MemorialImportRepo.
// The SQL emitted here is verbatim-copied from the legacy
// inline helpers in internal/records/memorial_import.go.
// Same parameter bindings, same TRIM wrappers, same
// INSERT OR IGNORE / UPDATE patterns.

package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/db/repo"
)

// Compile-time check: MemorialImportRepo satisfies
// repo.MemorialImportRepo.
var _ repo.MemorialImportRepo = (*MemorialImportRepo)(nil)

// MemorialImportRepo is the SQLite-backed implementation of
// repo.MemorialImportRepo.
type MemorialImportRepo struct {
	db *db.DB
}

// NewMemorialImportRepo constructs a SQLite-backed
// MemorialImportRepo.
func NewMemorialImportRepo(d *db.DB) *MemorialImportRepo {
	return &MemorialImportRepo{db: d}
}

// MemorialIDExists mirrors the legacy memorialIDExists helper
// verbatim. The memorialRecordType constant lives in the
// memorial_import.go service; the repo only owns the SQL.
// The service validates + trims memorialID before calling;
// the repo expects a non-empty, pre-trimmed value.
func (r *MemorialImportRepo) MemorialIDExists(ctx context.Context, q repo.Querier, memorialID string) (bool, error) {
	const memorialRecordType = "Find a Grave"
	var count int
	if err := db.WithBusyRetry(3, func() error {
		return q.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM records WHERE LOWER(TRIM(record_type)) = LOWER(TRIM(?)) AND TRIM(app_id) = ?`,
			memorialRecordType, memorialID,
		).Scan(&count)
	}); err != nil {
		return false, err
	}
	return count > 0, nil
}

// EnsureImportBatch mirrors the legacy ensureImportBatchRecord
// helper verbatim: INSERT OR IGNORE INTO import_batches.
func (r *MemorialImportRepo) EnsureImportBatch(ctx context.Context, ex repo.Execer, batchID, archivePath string) error {
	return db.WithBusyRetry(3, func() error {
		// Validate that Execer is usable
		if ex == nil {
			return fmt.Errorf("memorial import repo: Execer is nil")
		}
		_, err := ex.ExecContext(ctx,
			`INSERT OR IGNORE INTO import_batches (id, archive_path) VALUES (?, ?)`,
			strings.TrimSpace(batchID), filepath.Clean(strings.TrimSpace(archivePath)),
		)
		return err
	})
}

// SetSoldierImportBatch mirrors the legacy setSoldierImportBatch
// helper verbatim: UPDATE soldiers SET import_batch_id = ?.
func (r *MemorialImportRepo) SetSoldierImportBatch(ctx context.Context, ex repo.Execer, soldierID int64, batchID string) error {
	if soldierID < 1 {
		return fmt.Errorf("memorial import repo: invalid soldier id %d", soldierID)
	}
	return db.WithBusyRetry(3, func() error {
		if ex == nil {
			return fmt.Errorf("memorial import repo: Execer is nil")
		}
		_, err := ex.ExecContext(ctx,
			`UPDATE soldiers SET import_batch_id = ? WHERE id = ?`,
			strings.TrimSpace(batchID), soldierID,
		)
		return err
	})
}

// Ensure *sql.DB can serve as repo.Querier + repo.Execer.
var (
	_ repo.Querier = (*sql.DB)(nil)
	_ repo.Execer  = (*sql.DB)(nil)
)
