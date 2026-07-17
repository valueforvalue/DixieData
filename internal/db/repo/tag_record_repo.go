// Package repo declares the repository interfaces that decouple
// DixieData's domain services (internal/records/, internal/articles/,
// etc.) from the concrete database driver.
//
// Slice 5 of issue #613 adds the TagRecordRepo seam.
//
// Tags live in two tables:
//   - `tags` — canonical tag list (id, name, normalized_name)
//   - `person_record_tags` — per-Person-Record junction
//
// Slice 5 covers the 4 highest-traffic methods:
// UpsertByName, Attach, Detach, List. Rename / MergeInto /
// Delete / Get are out of scope (slice 6+ if needed).
//
// Design contract (same as the other repos): repos own SQL,
// context-aware, concrete impls under sqlite/.
package repo

import (
	"context"
)

// TagRecordRepo is the data-access seam for Tags.
type TagRecordRepo interface {
	// UpsertByName inserts a new tag (or returns the existing
	// id when normalized_name already exists). Returns the
	// tag id. The caller supplies an Execer (typically
	// *sql.Tx) so the upsert composes atomically with the
	// Attach/Delist that follows.
	UpsertByName(ctx context.Context, ex Execer, insertName string) (int64, error)

	// Attach inserts a person_record_tags row for the
	// (personID, tagID) pair. Idempotent (INSERT OR IGNORE
	// via the unique index). The caller supplies an Execer.
	Attach(ctx context.Context, ex Execer, tagID, personID int64) error

	// Detach removes the person_record_tags row. Returns the
	// rows-affected count (0 if no row matched).
	Detach(ctx context.Context, ex Execer, tagID, personID int64) (int64, error)
}