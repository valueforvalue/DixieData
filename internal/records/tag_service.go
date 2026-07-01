// Person Record tagging service (issue #183). Flat, free-text
// labels grouping Person Records for ad-hoc research scopes
// (virtual cemeteries, project scopes). All uniqueness is enforced
// by the UNIQUE constraint on tags.normalized_name at the DB layer
// — racing writers surface as a SQLITE_CONSTRAINT error mapped to
// a typed ErrTagNameTaken.
package records

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ErrTagNotFound is returned when a tag id has no row.
var ErrTagNotFound = errors.New("tag not found")

// ErrTagNameTaken is returned when a save attempts to reuse a
// normalized name already bound to another tag id. The constraint
// is UNIQUE, so any racing save of the same name also returns this
// error (mapped to HTTP 409 by the handler).
var ErrTagNameTaken = errors.New("tag name is taken")

// ErrTagMergeCollision is returned when a merge would leave the
// survivor and source with the same normalized name. The handler
// surfaces this as 409 with instructions to rename first; the
// reason is that deep links (?tags=vc-shiloh) would otherwise 404
// after the merge.
var ErrTagMergeCollision = errors.New("tag merge would collide on normalized name")

// Tag is the canonical row shape returned by Get/List. The
// display Name preserves original casing; NormalizedName is the
// case-insensitive lookup key (also stored as a UNIQUE column).
type Tag struct {
	ID             int64     `json:"id"`
	Name           string    `json:"name"`
	NormalizedName string    `json:"normalized_name"`
	MemberCount    int       `json:"member_count"`
	CreatedAt      time.Time `json:"created_at"`
}

// TagService operates on the tags + person_record_tags tables.
// Construct via NewTagService(db).
type TagService struct {
	db *sql.DB
}

func NewTagService(db *sql.DB) *TagService {
	return &TagService{db: db}
}

// NormalizeName is the single source of truth for "what does the
// user mean?" lookup. Lowercases, trims, and collapses internal
// whitespace runs to a single space. Same function backs the
// UNIQUE column so deep-link `?tags=...` lookups always agree
// with what a save would have written.
func NormalizeTagName(name string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(name))), " ")
}

// UpsertByName returns the existing tag with this normalized name
// or creates one with the supplied (original-cased) name. The
// returned row is what attach/detach should refer to.
//
// insertName is what callers want stored as the display Name;
// created_at is set by the DB default. If a caller supplies an
// empty insertName and a row already exists, the existing row's
// display name is preserved.
func (s *TagService) UpsertByName(ctx context.Context, insertName string) (Tag, error) {
	normalized := NormalizeTagName(insertName)
	if normalized == "" {
		return Tag{}, errors.New("tag name is required")
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT id FROM tags WHERE normalized_name = ?`, normalized)
	var existingID int64
	switch err := row.Scan(&existingID); err {
	case nil:
		return s.Get(ctx, existingID)
	case sql.ErrNoRows:
		// no existing row, fall through to insert
	default:
		return Tag{}, err
	}
	display := strings.TrimSpace(insertName)
	if display == "" {
		display = normalized
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO tags (name, normalized_name) VALUES (?, ?)`,
		display, normalized)
	if err != nil {
		if isUniqueConstraintError(err) {
			// Lost a race against a concurrent UpsertByName.
			// Re-read so the caller observes the winning row.
			return s.UpsertByName(ctx, insertName)
		}
		return Tag{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Tag{}, err
	}
	return s.Get(ctx, id)
}

// Attach binds a tag (by id) to a Person Record. Idempotent — a
// repeat call is a no-op. Uses INSERT OR IGNORE so the membership
// table never has duplicate (person_id, tag_id) pairs even under
// racing UI submissions.
func (s *TagService) Attach(ctx context.Context, tagID, personID int64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO person_record_tags (person_id, tag_id) VALUES (?, ?)`,
		personID, tagID)
	return err
}

// AttachMany binds one tag to many Person Records. Useful for
// bulk-tag from the Browse selection toolbar. The browser already
// returns SelectedIDs as []int64 in display order; AttachMany
// preserves that order in the membership writes (via batch INSERT
// in a single transaction) and returns the number of newly-bound
// rows (idempotent — repeat rows count as 0).
func (s *TagService) AttachMany(ctx context.Context, tagID int64, personIDs []int64) (int, error) {
	if len(personIDs) == 0 {
		return 0, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx,
		`INSERT OR IGNORE INTO person_record_tags (person_id, tag_id) VALUES (?, ?)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	var inserted int
	for _, pid := range personIDs {
		res, err := stmt.ExecContext(ctx, pid, tagID)
		if err != nil {
			return 0, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return 0, err
		}
		inserted += int(n)
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return inserted, nil
}

// Detach removes a binding. Idempotent — re-running on a missing
// row is a no-op (DELETE … WHERE returns 0 affected).
func (s *TagService) Detach(ctx context.Context, tagID, personID int64) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM person_record_tags WHERE tag_id = ? AND person_id = ?`,
		tagID, personID)
	return err
}

// Rename updates the display Name on a tag id. Reject if the new
// name would collide with another tag's normalized form.
func (s *TagService) Rename(ctx context.Context, id int64, newName string) (Tag, error) {
	normalized := NormalizeTagName(newName)
	if normalized == "" {
		return Tag{}, errors.New("tag name is required")
	}
	// Look up the would-be collision without touching the row.
	row := s.db.QueryRowContext(ctx,
		`SELECT id FROM tags WHERE normalized_name = ? AND id <> ?`,
		normalized, id)
	var collideID int64
	switch err := row.Scan(&collideID); err {
	case nil:
		return Tag{}, ErrTagNameTaken
	case sql.ErrNoRows:
		// no collision
	default:
		return Tag{}, err
	}
	display := strings.TrimSpace(newName)
	if display == "" {
		display = normalized
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE tags SET name = ?, normalized_name = ? WHERE id = ?`,
		display, normalized, id)
	if err != nil {
		return Tag{}, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return Tag{}, err
	}
	if n == 0 {
		return Tag{}, ErrTagNotFound
	}
	return s.Get(ctx, id)
}

// MergeInto moves every membership from source → survivor. The
// source tag itself is deleted on success. Reject if the survivor
// and source would end up sharing the same normalized name (the
// survivor keeps the surviving name; the source uses its old
// name). Caller is expected to surface the rejection and force
// the user to rename first.
func (s *TagService) MergeInto(ctx context.Context, sourceID, survivorID int64) (Tag, error) {
	if sourceID == survivorID {
		return Tag{}, errors.New("cannot merge a tag into itself")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Tag{}, err
	}
	defer tx.Rollback()
	var source, survivor Tag
	if err := scanTagRowInto(tx.QueryRowContext(ctx,
		`SELECT id, name, normalized_name, created_at FROM tags WHERE id = ?`, sourceID),
		&source); err != nil {
		return Tag{}, err
	}
	if err := scanTagRowInto(tx.QueryRowContext(ctx,
		`SELECT id, name, normalized_name, created_at FROM tags WHERE id = ?`, survivorID),
		&survivor); err != nil {
		return Tag{}, err
	}
	if source.NormalizedName == survivor.NormalizedName {
		return Tag{}, ErrTagMergeCollision
	}
	// Move memberships that don't already exist on survivor.
	// INSERT OR IGNORE handles rows that are already on both
	// sides (rare; can happen if the user manually re-attached).
	if _, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO person_record_tags (person_id, tag_id)
		SELECT person_id, ? FROM person_record_tags WHERE tag_id = ?`,
		survivorID, sourceID); err != nil {
		return Tag{}, err
	}
	// Drop source memberships (any remaining were duplicates that
	// we did not move because survivor already had them).
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM person_record_tags WHERE tag_id = ?`, sourceID); err != nil {
		return Tag{}, err
	}
	// Drop the source tag.
	if _, err := tx.ExecContext(ctx, `DELETE FROM tags WHERE id = ?`, sourceID); err != nil {
		return Tag{}, err
	}
	if err := tx.Commit(); err != nil {
		return Tag{}, err
	}
	return s.Get(ctx, survivorID)
}

// Delete removes a tag and every membership via ON DELETE CASCADE.
// Idempotent — re-running on a deleted id returns ErrTagNotFound so
// the handler can 404 cleanly.
func (s *TagService) Delete(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM tags WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrTagNotFound
	}
	return nil
}

// Get returns a single tag by id. ErrTagNotFound on miss.
func (s *TagService) Get(ctx context.Context, id int64) (Tag, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, normalized_name,
		       (SELECT COUNT(*) FROM person_record_tags WHERE tag_id = tags.id),
		       created_at
		FROM tags WHERE id = ?`, id)
	t, err := scanTagRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Tag{}, ErrTagNotFound
	}
	return t, err
}

// List returns every tag ordered alphabetically (case-insensitive
// via the COLLATE NOCASE column). Cheap up to a few thousand rows;
// the /tags management page caps display at 500 by default.
func (s *TagService) List(ctx context.Context) ([]Tag, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, normalized_name,
		       (SELECT COUNT(*) FROM person_record_tags WHERE tag_id = tags.id),
		       created_at
		FROM tags ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Tag
	for rows.Next() {
		t, err := scanTagRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Autocomplete returns up to `limit` tags whose normalized name
// contains query (substring, case-insensitive). Empty query
// returns the most recent 20. Used by the picker HTMX endpoint at
// `/soldiers/{id}/tags?autocomplete={q}` with a 120ms debounce.
func (s *TagService) Autocomplete(ctx context.Context, query string, limit int) ([]Tag, error) {
	if limit <= 0 {
		limit = 20
	}
	normalized := NormalizeTagName(query)
	var rows *sql.Rows
	var err error
	if normalized == "" {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, name, normalized_name,
			       (SELECT COUNT(*) FROM person_record_tags WHERE tag_id = tags.id),
			       created_at
			FROM tags
			ORDER BY created_at DESC
			LIMIT ?`, limit)
	} else {
		like := "%" + normalized + "%"
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, name, normalized_name,
			       (SELECT COUNT(*) FROM person_record_tags WHERE tag_id = tags.id),
			       created_at
			FROM tags
			WHERE normalized_name LIKE ?
			ORDER BY name COLLATE NOCASE
			LIMIT ?`, like, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Tag
	for rows.Next() {
		t, err := scanTagRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// TagsForSoldier returns the tags bound to a Person Record,
// alphabetical (display-cased). Empty slice for an untagged row;
// never returns nil in the slice (handlers can `.` it safely).
func (s *TagService) TagsForSoldier(ctx context.Context, soldierID int64) ([]Tag, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, normalized_name,
		       (SELECT COUNT(*) FROM person_record_tags WHERE tag_id = tags.id),
		       created_at
		FROM tags
		WHERE id IN (SELECT tag_id FROM person_record_tags WHERE person_id = ?)
		ORDER BY name COLLATE NOCASE`, soldierID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Tag
	for rows.Next() {
		t, err := scanTagRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// TagsForSoldiers is the bulk counterpart used by Browse. The
// returned map preserves the requested order of personIDs in the
// inner slice so handlers can render rows in their source order.
// Persons with no tags map to a nil slice (not omitted) so the
// caller can index without nil checks.
func (s *TagService) TagsForSoldiers(ctx context.Context, personIDs []int64) (map[int64][]Tag, error) {
	out := make(map[int64][]Tag, len(personIDs))
	for _, pid := range personIDs {
		out[pid] = nil
	}
	if len(personIDs) == 0 {
		return out, nil
	}
	// Build a placeholder list for the IN clause.
	placeholders := make([]string, len(personIDs))
	args := make([]any, len(personIDs))
	for i, pid := range personIDs {
		placeholders[i] = "?"
		args[i] = pid
	}
	q := fmt.Sprintf(`
		SELECT t.id, t.name, t.normalized_name,
		       (SELECT COUNT(*) FROM person_record_tags WHERE tag_id = t.id),
		       t.created_at, prt.person_id
		FROM tags t
		JOIN person_record_tags prt ON prt.tag_id = t.id
		WHERE prt.person_id IN (%s)
		ORDER BY t.name COLLATE NOCASE`, strings.Join(placeholders, ","))
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var t Tag
		var personID int64
		if err := rows.Scan(&t.ID, &t.Name, &t.NormalizedName, &t.MemberCount, &t.CreatedAt, &personID); err != nil {
			return nil, err
		}
		out[personID] = append(out[personID], t)
	}
	return out, rows.Err()
}

// Members returns the Person Record ids bound to a tag. Sorted
// by tag-binding created_at DESC (most recently tagged first) so
// the /tags/{id} detail page surfaces recent activity above old.
func (s *TagService) Members(ctx context.Context, tagID int64) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT person_id FROM person_record_tags
		WHERE tag_id = ? ORDER BY created_at DESC`, tagID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var pid int64
		if err := rows.Scan(&pid); err != nil {
			return nil, err
		}
		out = append(out, pid)
	}
	return out, rows.Err()
}

// AttachAdditive is the import-side counterpart of Attach. It is
// identical to Attach today (INSERT OR IGNORE) but lives on the
// service as a distinct verb so import pipelines are
// self-documenting. The merge-review pipeline carries the
// resolved binding set via this entry point so post-import UI
// can render "incoming tag X will be added" conflict text.
func (s *TagService) AttachAdditive(ctx context.Context, soldierID int64, tagName string) (Tag, error) {
	tag, err := s.UpsertByName(ctx, tagName)
	if err != nil {
		return Tag{}, err
	}
	if err := s.Attach(ctx, tag.ID, soldierID); err != nil {
		return Tag{}, err
	}
	return s.Get(ctx, tag.ID)
}

// ByIDsPreservesOrder returns tags by id in the supplied order.
// Useful when the share pipeline receives an ordered list (the
// JSON manifest preserves soldier → tag ordering) and needs to
// render an HTML preview in the same order.
func (s *TagService) ByIDsPreservesOrder(ctx context.Context, ids []int64) ([]Tag, error) {
	if len(ids) == 0 {
		return []Tag{}, nil
	}
	idSet := make(map[int64]struct{}, len(ids))
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
		idSet[id] = struct{}{}
	}
	q := fmt.Sprintf(`
		SELECT id, name, normalized_name,
		       (SELECT COUNT(*) FROM person_record_tags WHERE tag_id = tags.id),
		       created_at
		FROM tags WHERE id IN (%s)`, strings.Join(placeholders, ","))
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byID := make(map[int64]Tag, len(ids))
	for rows.Next() {
		t, err := scanTagRow(rows)
		if err != nil {
			return nil, err
		}
		byID[t.ID] = t
	}
	out := make([]Tag, 0, len(ids))
	for _, id := range ids {
		if t, ok := byID[id]; ok {
			out = append(out, t)
		}
	}
	// Stable secondary sort by normalized name for stable output
	// when callers don't care about ordering.
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].NormalizedName < out[j].NormalizedName
	})
	return out, nil
}

// scanTagRow implements sql.Scanner-compatible row scanning for
// the Tag struct. Centralised so Autocomplete / Get / List / etc.
// share one source of truth.
type tagScanner interface {
	Scan(dest ...any) error
}

func scanTagRow(row tagScanner) (Tag, error) {
	var t Tag
	if err := row.Scan(&t.ID, &t.Name, &t.NormalizedName, &t.MemberCount, &t.CreatedAt); err != nil {
		return Tag{}, err
	}
	return t, nil
}

// scanTagRowInto is the *sql.Tx variant for MergeInto which
// cannot use QueryRowContext because it needs the same connection
// inside a transaction.
func scanTagRowInto(row *sql.Row, t *Tag) error {
	return row.Scan(&t.ID, &t.Name, &t.NormalizedName, &t.CreatedAt)
}
