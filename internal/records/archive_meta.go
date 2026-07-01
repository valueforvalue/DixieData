// Per-archive-kind export toggles (issue #183). The first toggle
// is `include_tags` — opt-in for Shared Archive (.ddshare),
// always-on for Backup Archive (full SQLite snapshot),
// always-off for Static Archive (HTML export excludes working
// notes). The table is keyed by archive_kind so future toggles
// land as additional columns without schema churn.
package records

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// Archive kinds — mirrored from the archive_meta seed row keys.
const (
	ArchiveKindShared = "shared_archive"
	ArchiveKindBackup = "backup_archive"
	ArchiveKindStatic = "static_archive"
)

// ErrArchiveMetaNotFound is returned when no row exists for the
// requested archive_kind. Should not happen on live archives
// (seeded at schema time), but exposed so callers can recover
// cleanly if a manual SQL delete strips the row.
var ErrArchiveMetaNotFound = errors.New("archive meta not found")

// ArchiveMetaService provides read/write access to the
// archive_meta table.
type ArchiveMetaService struct {
	db *sql.DB
}

func NewArchiveMetaService(db *sql.DB) *ArchiveMetaService {
	return &ArchiveMetaService{db: db}
}

// ArchiveMeta is the row shape returned by Get. Future toggles
// are added as new fields; existing rows pick up the schema
// default via the table's CHECK + DEFAULT clauses.
type ArchiveMeta struct {
	Kind        string `json:"archive_kind"`
	IncludeTags bool   `json:"include_tags"`
	UpdatedAt   string `json:"updated_at"`
}

// Get returns the meta row for an archive kind.
// ErrArchiveMetaNotFound on miss.
func (s *ArchiveMetaService) Get(ctx context.Context, kind string) (ArchiveMeta, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT archive_kind, include_tags, updated_at
		 FROM archive_meta WHERE archive_kind = ?`, strings.TrimSpace(kind))
	var m ArchiveMeta
	var include int
	if err := row.Scan(&m.Kind, &include, &m.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ArchiveMeta{}, ErrArchiveMetaNotFound
		}
		return ArchiveMeta{}, err
	}
	m.IncludeTags = include != 0
	return m, nil
}

// SetIncludeTags updates (or inserts) the include_tags toggle for
// a given archive_kind. The CHECK constraint enforces 0/1; any
// other value returns an SQL error.
func (s *ArchiveMetaService) SetIncludeTags(ctx context.Context, kind string, include bool) (ArchiveMeta, error) {
	cleaned := strings.TrimSpace(kind)
	if cleaned == "" {
		return ArchiveMeta{}, errors.New("archive kind is required")
	}
	v := 0
	if include {
		v = 1
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO archive_meta (archive_kind, include_tags, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(archive_kind) DO UPDATE SET
			include_tags = excluded.include_tags,
			updated_at   = CURRENT_TIMESTAMP`,
		cleaned, v); err != nil {
		return ArchiveMeta{}, err
	}
	return s.Get(ctx, cleaned)
}

// IncludeTags is the convenience read used by export handlers.
// Returns false on any error (graceful degradation — the export
// continues without tags and emits a warning to the share-page UI).
func (s *ArchiveMetaService) IncludeTags(ctx context.Context, kind string) bool {
	m, err := s.Get(ctx, kind)
	if err != nil {
		return false
	}
	return m.IncludeTags
}
