// Package sqlite provides the SQLite-backed implementations of
// the repository interfaces declared in internal/db/repo.
//
// This file ships the SQLite impl of TagRecordRepo (issue #613
// slice 5). Parity contract: every method's SQL matches the
// legacy inline SQL in internal/records/tag_service.go
// verbatim.
package sqlite

import (
	"context"
	"strings"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/db/repo"
)

// TagRecordRepo is the SQLite-backed implementation of
// repo.TagRecordRepo.
type TagRecordRepo struct {
	db *db.DB
}

// Compile-time check: TagRecordRepo satisfies
// repo.TagRecordRepo.
var _ repo.TagRecordRepo = (*TagRecordRepo)(nil)

// NewTagRecordRepo constructs a SQLite-backed TagRecordRepo.
func NewTagRecordRepo(d *db.DB) *TagRecordRepo {
	return &TagRecordRepo{db: d}
}

// UpsertByName inserts a new tag (or returns the existing id
// when normalized_name already exists). Returns the tag id.
//
// Mirrors the legacy inline SQL in
// internal/records/tag_service.go::UpsertByName: the
// normalized form (trim + lowercase) is what the UNIQUE
// index guards on; the original name preserves its casing.
func (r *TagRecordRepo) UpsertByName(ctx context.Context, ex repo.Execer, insertName string) (int64, error) {
	normalized := strings.ToLower(strings.TrimSpace(insertName))
	res, err := ex.ExecContext(ctx,
		`INSERT INTO tags (name, normalized_name) VALUES (?, ?)
		 ON CONFLICT(normalized_name) DO NOTHING`,
		insertName, normalized,
	)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if id == 0 {
		// ON CONFLICT triggered; look up the existing id.
		// The caller passes an Execer (tx or *sql.DB), but
		// we need a Querier for the SELECT. Both satisfy
		// Querier, so we re-use the same arg as a Querier.
		if q, ok := ex.(repo.Querier); ok {
			if err := q.QueryRowContext(ctx,
				`SELECT id FROM tags WHERE normalized_name = ?`,
				normalized,
			).Scan(&id); err != nil {
				return 0, err
			}
		} else {
			return 0, err
		}
	}
	return id, nil
}

// Attach inserts a person_record_tags row. Idempotent
// (INSERT OR IGNORE).
func (r *TagRecordRepo) Attach(ctx context.Context, ex repo.Execer, tagID, personID int64) error {
	_, err := ex.ExecContext(ctx,
		`INSERT OR IGNORE INTO person_record_tags (person_id, tag_id) VALUES (?, ?)`,
		personID, tagID,
	)
	return err
}

// Detach removes the person_record_tags row. Returns the
// rows-affected count.
func (r *TagRecordRepo) Detach(ctx context.Context, ex repo.Execer, tagID, personID int64) (int64, error) {
	res, err := ex.ExecContext(ctx,
		`DELETE FROM person_record_tags WHERE person_id = ? AND tag_id = ?`,
		personID, tagID,
	)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return n, nil
}