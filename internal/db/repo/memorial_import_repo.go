// memorial_import_repo.go — issue #625 slice 9b.
//
// MemorialImportRepo is the data-access seam for the memorial
// (.ddjson) import path. Extracts the 3 inline-SQL helper
// functions from internal/records/memorial_import.go so the
// service no longer owns raw SQL for import-batch tracking
// and duplicate detection.

package repo

import "context"

// MemorialImportRepo is the data-access seam for memorial
// (.ddjson) import operations.
//
// Design contract (mirrors QualityScanRepo and the rest of
// the repo package):
//
//   - Methods take Querier/Execer interfaces so the caller
//     can pass a *sql.DB or *sql.Tx.
//
//   - The repo is pure data access — domain logic
//     (trimming, validation) stays in the service.
type MemorialImportRepo interface {
	// MemorialIDExists reports whether a record with the given
	// memorial_id already exists in the archive (matched by
	// record_type = 'memorial' AND app_id = memorialID).
	MemorialIDExists(ctx context.Context, q Querier, memorialID string) (bool, error)

	// EnsureImportBatch inserts an import_batches row if one
	// does not already exist for the given batchID.
	EnsureImportBatch(ctx context.Context, ex Execer, batchID, archivePath string) error

	// SetSoldierImportBatch records the import_batch_id on a
	// soldier row.
	SetSoldierImportBatch(ctx context.Context, ex Execer, soldierID int64, batchID string) error
}
