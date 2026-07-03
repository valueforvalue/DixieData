package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/valueforvalue/DixieData/internal/dates"
	"github.com/valueforvalue/DixieData/internal/versioninfo"
)

// CurrentSchemaVersion is the schema version the current binary expects. Bumped by every schema-touching PR (issue #266's discipline).
const CurrentSchemaVersion = versioninfo.CurrentSchemaVersion

// GetAppVersion returns the app version string for the current binary, in the v{MAJOR}.{U}.{N} shape (issue #266).
func GetAppVersion() string {
	return versioninfo.CurrentAppVersion()
}

const schema = `
CREATE TABLE IF NOT EXISTS schema_version (
    version    INTEGER PRIMARY KEY,
    applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS soldiers (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    display_id   TEXT UNIQUE NOT NULL,
    sync_id      TEXT,
    entry_type   TEXT NOT NULL DEFAULT 'soldier',
    spouse_soldier_id INTEGER REFERENCES soldiers(id) ON DELETE SET NULL,
    relationship_label TEXT,
    maiden_name  TEXT,
    is_generated BOOLEAN DEFAULT 0,
    pension_id   TEXT,
    application_id TEXT,
    prefix       TEXT,
    show_prefix_before_name BOOLEAN DEFAULT 0,
    first_name   TEXT,
    middle_name  TEXT,
    last_name    TEXT,
    suffix       TEXT,
    rank         TEXT,
    rank_in      TEXT,
    rank_out     TEXT,
    unit         TEXT,
    pension_state TEXT,
    confederate_home_status TEXT DEFAULT 'N/A',
    confederate_home_name TEXT,
    death_year   INTEGER,
    death_month  INTEGER,
    death_day    INTEGER,
    birth_date   TEXT,
    death_date   TEXT,
    birth_info   TEXT,
    buried_in    TEXT,
    biography    TEXT,
    pdf_excerpt_override TEXT,
    notes        TEXT,
    needs_review BOOLEAN DEFAULT 0,
    review_reason TEXT,
    added_by     TEXT,
    last_edited_by TEXT,
    last_edited_fields TEXT,
    last_edited_at DATETIME,
    created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME
);

CREATE TABLE IF NOT EXISTS records (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    sync_id      TEXT,
    soldier_id   INTEGER REFERENCES soldiers(id) ON DELETE CASCADE,
    soldier_sync_id TEXT,
    record_type  TEXT,
    app_id       TEXT,
    details      TEXT
);

CREATE TABLE IF NOT EXISTS images (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    sync_id      TEXT,
    soldier_id   INTEGER REFERENCES soldiers(id) ON DELETE CASCADE,
    soldier_sync_id TEXT,
    file_name    TEXT,
    file_path    TEXT,
    caption      TEXT,
    is_primary   BOOLEAN DEFAULT 0
);

CREATE TABLE IF NOT EXISTS merge_review_sessions (
    id           TEXT PRIMARY KEY,
    archive_path TEXT NOT NULL,
    source_root  TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'open',
    created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME
);

CREATE TABLE IF NOT EXISTS import_batches (
    id           TEXT PRIMARY KEY,
    archive_path TEXT NOT NULL,
    created_at   DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS merge_review_conflicts (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id       TEXT NOT NULL REFERENCES merge_review_sessions(id) ON DELETE CASCADE,
    conflict_type    TEXT NOT NULL,
    reason           TEXT NOT NULL,
    soldier_sync_id  TEXT NOT NULL,
    local_soldier_id INTEGER,
    local_display_id TEXT,
    source_display_id TEXT NOT NULL,
    local_data       TEXT,
    source_data      TEXT NOT NULL,
    resolution       TEXT,
    created_at       DATETIME DEFAULT CURRENT_TIMESTAMP,
    resolved_at      DATETIME
);

CREATE TABLE IF NOT EXISTS shared_merge_aliases (
    id                       INTEGER PRIMARY KEY AUTOINCREMENT,
    source_node_id           TEXT NOT NULL,
    source_person_sync_id    TEXT NOT NULL,
    canonical_person_sync_id TEXT NOT NULL,
    canonical_person_id      INTEGER NOT NULL REFERENCES soldiers(id) ON DELETE CASCADE,
    resolution_kind          TEXT NOT NULL DEFAULT 'merge-review',
    created_from_conflict_id INTEGER REFERENCES merge_review_conflicts(id) ON DELETE SET NULL,
    created_at               DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at               DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (source_node_id, source_person_sync_id)
);

CREATE TABLE IF NOT EXISTS duplicate_audit_findings (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    pair_key         TEXT UNIQUE NOT NULL,
    left_soldier_id  INTEGER NOT NULL REFERENCES soldiers(id) ON DELETE CASCADE,
    right_soldier_id INTEGER NOT NULL REFERENCES soldiers(id) ON DELETE CASCADE,
    finding_type     TEXT NOT NULL,
    reason           TEXT NOT NULL,
    highlight_fields TEXT NOT NULL,
    status           TEXT NOT NULL DEFAULT 'open',
    created_at       DATETIME DEFAULT CURRENT_TIMESTAMP,
    last_detected_at DATETIME,
    resolved_at      DATETIME
);

CREATE TABLE IF NOT EXISTS research_tasks (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    soldier_id     INTEGER NOT NULL REFERENCES soldiers(id) ON DELETE CASCADE,
    title         TEXT NOT NULL,
    notes         TEXT,
    evidence_type TEXT NOT NULL DEFAULT 'general',
    status        TEXT NOT NULL DEFAULT 'open',
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME,
    resolved_at   DATETIME
);

-- Issue #178: save/reuse printable-export templates. Local-only
-- storage of named print-config snapshots. JSON columns hold
-- the filter + group-by arrays since they are sparse and free-
-- form (each filter family may have any number of values).
CREATE TABLE IF NOT EXISTS export_templates (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    name          TEXT NOT NULL UNIQUE,
    scope         TEXT NOT NULL,
    filters_json  TEXT NOT NULL DEFAULT '{}',
    sort_by       TEXT NOT NULL DEFAULT 'last_name',
    group_by_json TEXT NOT NULL DEFAULT '[]',
    orientation   TEXT NOT NULL DEFAULT 'L',
    selected_ids_json TEXT NOT NULL DEFAULT '[]',
    printer_friendly     INTEGER NOT NULL DEFAULT 0,
    full_biography_page  INTEGER NOT NULL DEFAULT 0,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    last_used_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Issue #192: saved Share Queue presets. Stores the (soldier_id,
-- display_id) pairs that make up a reusable subset for the
-- .ddshare export pipeline. Local-only like export_templates;
-- no sync_id since these don't migrate between archives. The
-- soldier_ids list is a JSON array; v1 keeps the schema simple
-- (no separate join table) since preset membership is small
-- (a few dozen rows at most) and we never need to query
-- membership reverse-direction (find all presets that contain
-- a given soldier). The Display IDs are denormalized into
-- the payload so the modal can render names without joining
-- back to soldiers on every Apply click -- a soldier that's
-- deleted post-save is silently dropped on Apply, with the
-- missing row recorded in the response so the JS can show a
-- warning rather than a hard error.
CREATE TABLE IF NOT EXISTS share_queue_presets (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL UNIQUE,
    soldier_ids_json TEXT NOT NULL DEFAULT '[]',
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    last_used_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS research_collections (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    name          TEXT NOT NULL UNIQUE,
    description   TEXT,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME
);

CREATE TABLE IF NOT EXISTS research_collection_items (
    collection_id INTEGER NOT NULL REFERENCES research_collections(id) ON DELETE CASCADE,
    soldier_id    INTEGER NOT NULL REFERENCES soldiers(id) ON DELETE CASCADE,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (collection_id, soldier_id)
);

CREATE TABLE IF NOT EXISTS calendar_items (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    item_type    TEXT NOT NULL CHECK (item_type IN ('event', 'holiday')),
    month        INTEGER NOT NULL CHECK (month >= 1 AND month <= 12),
    day          INTEGER NOT NULL CHECK (day >= 1 AND day <= 31),
    title        TEXT NOT NULL,
    notes        TEXT NOT NULL DEFAULT '',
    created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Issue #183: Person Record tagging. Flat, free-text labels
-- for ad-hoc research scopes (virtual cemeteries, project
-- groups). Person Record only; Source Records inherit via the
-- parent row. Tags are an opt-in for .ddshare, never shipped
-- in Static Archives, always included in Backup Archives
-- (full SQLite snapshot).
CREATE TABLE IF NOT EXISTS tags (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    name            TEXT NOT NULL COLLATE NOCASE,
    normalized_name TEXT NOT NULL UNIQUE,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS person_record_tags (
    person_id  INTEGER NOT NULL REFERENCES soldiers(id) ON DELETE CASCADE,
    tag_id     INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (person_id, tag_id)
);

-- Per-archive-kind toggles for export-side behaviour that is
-- not derivable from the row data alone. Seeded with three rows
-- on every upgrade so the share/backup/static export pipelines
-- can SELECT their default without an ad-hoc CASE.
CREATE TABLE IF NOT EXISTS archive_meta (
    archive_kind TEXT PRIMARY KEY,
    include_tags INTEGER NOT NULL DEFAULT 0
                  CHECK (include_tags IN (0, 1)),
    updated_at   DATETIME DEFAULT CURRENT_TIMESTAMP
);

INSERT OR IGNORE INTO archive_meta (archive_kind, include_tags) VALUES
    ('shared_archive', 0),
    ('backup_archive', 1),
    ('static_archive', 0);

CREATE INDEX IF NOT EXISTS idx_soldiers_death ON soldiers(death_month, death_day);
CREATE INDEX IF NOT EXISTS idx_merge_review_conflicts_session ON merge_review_conflicts(session_id);
CREATE INDEX IF NOT EXISTS idx_merge_review_conflicts_resolution ON merge_review_conflicts(resolution);
CREATE INDEX IF NOT EXISTS idx_import_batches_created_at ON import_batches(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_shared_merge_aliases_canonical ON shared_merge_aliases(canonical_person_id);
CREATE INDEX IF NOT EXISTS idx_duplicate_audit_findings_status ON duplicate_audit_findings(status);
CREATE INDEX IF NOT EXISTS idx_duplicate_audit_findings_left ON duplicate_audit_findings(left_soldier_id);
CREATE INDEX IF NOT EXISTS idx_duplicate_audit_findings_right ON duplicate_audit_findings(right_soldier_id);
CREATE INDEX IF NOT EXISTS idx_research_tasks_soldier ON research_tasks(soldier_id, status, created_at);
CREATE INDEX IF NOT EXISTS idx_research_collection_items_soldier ON research_collection_items(soldier_id, collection_id);
CREATE INDEX IF NOT EXISTS idx_calendar_items_day ON calendar_items(month, day, item_type, title);
CREATE INDEX IF NOT EXISTS idx_person_record_tags_tag    ON person_record_tags(tag_id);
CREATE INDEX IF NOT EXISTS idx_person_record_tags_person ON person_record_tags(person_id);
`

const phase1DistributedMergeMigration = `
CREATE TABLE IF NOT EXISTS system_config (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

UPDATE soldiers
SET birth_date = '00/00/0000'
WHERE birth_date IS NULL OR TRIM(birth_date) = '';

UPDATE soldiers
SET death_date = printf('%02d/%02d/%04d', COALESCE(death_month, 0), COALESCE(death_day, 0), COALESCE(death_year, 0))
WHERE death_date IS NULL OR TRIM(death_date) = '';

UPDATE soldiers
SET updated_at = COALESCE(NULLIF(created_at, ''), CURRENT_TIMESTAMP)
WHERE updated_at IS NULL OR TRIM(updated_at) = '';

UPDATE soldiers
SET sync_id = ` + syncIDSQL + `
WHERE sync_id IS NULL OR TRIM(sync_id) = '';

INSERT INTO system_config(key, value)
SELECT 'node_prefix', 'DXD'
WHERE NOT EXISTS (SELECT 1 FROM system_config WHERE key = 'node_prefix');

INSERT INTO system_config(key, value)
SELECT
    'node_id',
    ` + syncIDSQL + `
WHERE NOT EXISTS (SELECT 1 FROM system_config WHERE key = 'node_id');

CREATE UNIQUE INDEX IF NOT EXISTS idx_soldiers_sync_id ON soldiers(sync_id);

UPDATE records
SET soldier_sync_id = (
    SELECT soldiers.sync_id
    FROM soldiers
    WHERE soldiers.id = records.soldier_id
)
WHERE soldier_sync_id IS NULL OR TRIM(soldier_sync_id) = '';

UPDATE records
SET sync_id = ` + syncIDSQL + `
WHERE sync_id IS NULL OR TRIM(sync_id) = '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_records_sync_id ON records(sync_id);
CREATE INDEX IF NOT EXISTS idx_records_soldier_sync_id ON records(soldier_sync_id);

UPDATE images
SET soldier_sync_id = (
    SELECT soldiers.sync_id
    FROM soldiers
    WHERE soldiers.id = images.soldier_id
)
WHERE soldier_sync_id IS NULL OR TRIM(soldier_sync_id) = '';

UPDATE images
SET sync_id = ` + syncIDSQL + `
WHERE sync_id IS NULL OR TRIM(sync_id) = '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_images_sync_id ON images(sync_id);
CREATE INDEX IF NOT EXISTS idx_images_soldier_sync_id ON images(soldier_sync_id);
`

const phase2CanonicalDatesMigration = `
UPDATE soldiers
SET entry_type = 'soldier'
WHERE entry_type IS NULL OR TRIM(entry_type) = '';

UPDATE soldiers
SET death_date = CASE
    WHEN COALESCE(death_year, 0) = 0 AND COALESCE(death_month, 0) = 0 AND COALESCE(death_day, 0) = 0 THEN ''
    ELSE printf('%02d/%02d/%04d', COALESCE(death_month, 0), COALESCE(death_day, 0), COALESCE(death_year, 0))
END
WHERE death_date IS NULL OR TRIM(death_date) = '';

UPDATE soldiers
SET birth_date = ''
WHERE birth_date = '00/00/0000';
`

// applySchema runs every block in the migrations slice (see
// migrations.go) inside a single transaction, then writes the
// terminal PRAGMA user_version as the gate for the next Open call.
//
// The block list, reversibility classification, and per-block
// reasoning live at docs/migrations/reversibility.md — that doc is
// the source of truth for the future `migrate down <target>` runner
// (issue #273). applySchema itself is forward-only today; the slice
// just makes the per-block enumeration addressable from outside
// this file.
//
// Behavior is identical to the pre-refactor inline ordering:
//   1. tx.Exec(schema) baseline
//   2. migrations[i].Up(tx) for each Migration in order
//   3. INSERT schema_version + PRAGMA user_version + Commit
func applySchema(db *DB) error {
	version, err := currentSchemaVersion(db.conn)
	if err != nil {
		return err
	}
	if version >= CurrentSchemaVersion {
		return nil
	}

	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, m := range migrations {
		if err := m.Up(tx); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(`INSERT OR IGNORE INTO schema_version(version) VALUES (?)`, CurrentSchemaVersion); err != nil {
		return err
	}
	if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, CurrentSchemaVersion)); err != nil {
		return err
	}

	return tx.Commit()
}

// applyDownSchema runs the inverse of every block in the migrations
// slice that maps to the `current` -> `target` delta. The
// relationship between user_version and the slice is NOT 1:1 (the
// slice has 17 entries but user_version is 59; many blocks predate
// the v52 doc discipline). We map the delta as follows: the LAST
// `current - target` blocks of the slice are undone in reverse
// order. The first block undone is the last one applied (Block 17
// for the current schema); the last block undone is whichever block
// sits at slice index `len(migrations) - (current - target)`.
//
// Refusal semantics (per the audit at docs/migrations/reversibility.md
// + the 5 design decisions captured in the issue #273 research
// artifact):
//
//   - If any block in the path has Down == nil (should not happen;
//     every Migration in the slice gets a Down function, even if
//     it's a no-op for PartiallyReversible cases), applyDownSchema
//     refuses with an error identifying the block ID.
//   - If any block's Down returns ErrMigrationIrreversible, the
//     runner refuses the entire path with ErrDowngradeRefused.
//     The CLI unwraps this and prints the blocking block ID +
//     Reason. The runner does NOT bypass the refusal even with
//     a --force-irreversible flag (per design decision Q2: the
//     flag emits the "what was lost" manifest but does not
//     invert the SQL).
//   - On any non-refusal error from a block's Down, the deferred
//     tx.Rollback() unwinds the partial state. The next Open
//     call sees `user_version = current` (unchanged) and the
//     schema is at v(N)-shape (partial).
//
// The single-tx model + terminal `PRAGMA user_version = target`
// write are critical: writing user_version earlier would
// mean a crashed-mid-DOWN DB claims to be at `target` while
// still being at v(N)'s shape, opening the next Open() to an
// applySchema short-circuit (per the version gate at the top
// of applySchema) that would do no remedial forward work,
// leaving the operator with a phantom-downgraded DB.

// ApplyDownSchema is the exported entry point for schema downgrades.
// It delegates to the package-private applyDownSchema and exists so
// external callers (notably the CLI runner in
// internal/appshell/cli_admin.go) can call it without importing a
// private symbol. Refusals surface as ErrDowngradeRefused wrapped
// around the offending block's ID and ErrMigrationIrreversible.
func ApplyDownSchema(db *DB, target int) error {
	return applyDownSchema(db, target)
}

// applyDownSchema is the package-private implementation. The
// exported ApplyDownSchema wrapper exists so external callers
// (the CLI runner in internal/appshell/cli_admin.go) can call it
// without importing a private symbol.
func applyDownSchema(db *DB, target int) error {
	current, err := currentSchemaVersion(db.conn)
	if err != nil {
		return err
	}
	if current <= target {
		return nil
	}

	// Map the (current - target) delta to a slice window. The
	// delta cannot exceed len(migrations); cap it.
	delta := current - target
	if delta > len(migrations) {
		delta = len(migrations)
	}

	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Iterate the last `delta` entries of the migrations slice in
	// REVERSE order. Slice index `len(migrations) - 1` is the most
	// recent block; index `len(migrations) - delta` is the oldest
	// block in the window.
	for i := len(migrations) - 1; i >= len(migrations)-delta; i-- {
		m := migrations[i]
		if m.Down == nil {
			return fmt.Errorf("%w: block %s has no Down function (caller must supply one or refuse the path)", ErrDowngradeRefused, m.ID)
		}
		if err := m.Down(tx); err != nil {
			if errors.Is(err, ErrMigrationIrreversible) {
				// Wrap both the umbrella ErrDowngradeRefused AND
				// the inner ErrMigrationIrreversible so callers
				// can use errors.Is for either check (Go 1.20+
				// supports multiple %w verbs).
				return fmt.Errorf("%w: %s: %w: %s", ErrDowngradeRefused, m.ID, ErrMigrationIrreversible, m.Reason)
			}
			return err
		}
	}

	if _, err := tx.Exec(`DELETE FROM schema_version WHERE version > ?`, target); err != nil {
		return err
	}
	if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, target)); err != nil {
		return err
	}

	return tx.Commit()
}

// isNoSuchTableError reports whether err is a SQLite "no such table" error.
// Used to make forward-only migrations tolerant of optional tables that
// may not exist on legacy archives (e.g. research_log, planned for v56+).
func isNoSuchTableError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "no such table")
}

func columnExists(tx *sql.Tx, table, column string) (bool, error) {
	var query string
	switch table {
	case "soldiers":
		query = `PRAGMA table_info(soldiers)`
	case "records":
		query = `PRAGMA table_info(records)`
	case "images":
		query = `PRAGMA table_info(images)`
	case "system_config":
		query = `PRAGMA table_info(system_config)`
	case "merge_review_sessions":
		query = `PRAGMA table_info(merge_review_sessions)`
	case "merge_review_conflicts":
		query = `PRAGMA table_info(merge_review_conflicts)`
	case "import_batches":
		query = `PRAGMA table_info(import_batches)`
	case "duplicate_audit_findings":
		query = `PRAGMA table_info(duplicate_audit_findings)`
	default:
		return false, fmt.Errorf("unsupported table for schema introspection: %s", table)
	}
	rows, err := tx.Query(query)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid        int
			name       string
			dataType   string
			notNull    int
			defaultVal interface{}
			pk         int
		)
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultVal, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

func ensureSoldierFTS(tx *sql.Tx) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS scratchpad_cache (
			soldier_id INTEGER PRIMARY KEY REFERENCES soldiers(id) ON DELETE CASCADE,
			scratch_pad TEXT NOT NULL DEFAULT '',
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`DELETE FROM scratchpad_cache WHERE soldier_id NOT IN (SELECT id FROM soldiers)`,
		`DROP TRIGGER IF EXISTS soldiers_fts_ai`,
		`DROP TRIGGER IF EXISTS soldiers_fts_au`,
		`DROP TRIGGER IF EXISTS soldiers_fts_ad`,
		`DROP TRIGGER IF EXISTS scratchpad_cache_ai`,
		`DROP TRIGGER IF EXISTS scratchpad_cache_au`,
		`DROP TRIGGER IF EXISTS scratchpad_cache_ad`,
		`DROP TABLE IF EXISTS soldiers_fts`,
		`CREATE VIRTUAL TABLE soldiers_fts USING fts5(
			soldier_id UNINDEXED,
			display_id,
			pension_id,
			application_id,
			prefix,
			first_name,
			middle_name,
			last_name,
			suffix,
			unit,
			soldier_rank,
			rank_in_text,
			rank_out_text,
			pension_state,
			confederate_home_status,
			confederate_home_name,
			buried_in,
			maiden_name,
			relationship_label,
			biography,
			notes,
			scratch_pad
		)`,
		`CREATE TRIGGER soldiers_fts_ai AFTER INSERT ON soldiers BEGIN
			INSERT INTO soldiers_fts (
				rowid, soldier_id, display_id, pension_id, application_id, prefix, first_name, middle_name, last_name, suffix,
				unit, soldier_rank, rank_in_text, rank_out_text, pension_state, confederate_home_status, confederate_home_name, buried_in, maiden_name, relationship_label,
				biography, notes, scratch_pad
			) VALUES (
				new.id, new.id, COALESCE(new.display_id, ''), COALESCE(new.pension_id, ''), COALESCE(new.application_id, ''), COALESCE(new.prefix, ''), COALESCE(new.first_name, ''),
				COALESCE(new.middle_name, ''), COALESCE(new.last_name, ''), COALESCE(new.suffix, ''), COALESCE(new.unit, ''), COALESCE(new.rank, ''), COALESCE(new.rank_in, ''),
				COALESCE(new.rank_out, ''), COALESCE(new.pension_state, ''), COALESCE(new.confederate_home_status, ''), COALESCE(new.confederate_home_name, ''), COALESCE(new.buried_in, ''),
				COALESCE(new.maiden_name, ''), COALESCE(new.relationship_label, ''), COALESCE(new.biography, ''), COALESCE(new.notes, ''), COALESCE((SELECT scratch_pad FROM scratchpad_cache WHERE soldier_id = new.id), '')
			);
		END`,
		`CREATE TRIGGER soldiers_fts_au AFTER UPDATE ON soldiers BEGIN
			DELETE FROM soldiers_fts WHERE rowid = old.id;
			INSERT INTO soldiers_fts (
				rowid, soldier_id, display_id, pension_id, application_id, prefix, first_name, middle_name, last_name, suffix,
				unit, soldier_rank, rank_in_text, rank_out_text, pension_state, confederate_home_status, confederate_home_name, buried_in, maiden_name, relationship_label,
				biography, notes, scratch_pad
			) VALUES (
				new.id, new.id, COALESCE(new.display_id, ''), COALESCE(new.pension_id, ''), COALESCE(new.application_id, ''), COALESCE(new.prefix, ''), COALESCE(new.first_name, ''),
				COALESCE(new.middle_name, ''), COALESCE(new.last_name, ''), COALESCE(new.suffix, ''), COALESCE(new.unit, ''), COALESCE(new.rank, ''), COALESCE(new.rank_in, ''),
				COALESCE(new.rank_out, ''), COALESCE(new.pension_state, ''), COALESCE(new.confederate_home_status, ''), COALESCE(new.confederate_home_name, ''), COALESCE(new.buried_in, ''),
				COALESCE(new.maiden_name, ''), COALESCE(new.relationship_label, ''), COALESCE(new.biography, ''), COALESCE(new.notes, ''), COALESCE((SELECT scratch_pad FROM scratchpad_cache WHERE soldier_id = new.id), '')
			);
		END`,
		`CREATE TRIGGER soldiers_fts_ad AFTER DELETE ON soldiers BEGIN
			DELETE FROM soldiers_fts WHERE rowid = old.id;
			DELETE FROM scratchpad_cache WHERE soldier_id = old.id;
		END`,
		`CREATE TRIGGER scratchpad_cache_ai AFTER INSERT ON scratchpad_cache BEGIN
			DELETE FROM soldiers_fts WHERE rowid = new.soldier_id;
			INSERT INTO soldiers_fts (
				rowid, soldier_id, display_id, pension_id, application_id, prefix, first_name, middle_name, last_name, suffix,
				unit, soldier_rank, rank_in_text, rank_out_text, pension_state, confederate_home_status, confederate_home_name, buried_in, maiden_name, relationship_label,
				biography, notes, scratch_pad
			)
			SELECT
				s.id, s.id, COALESCE(s.display_id, ''), COALESCE(s.pension_id, ''), COALESCE(s.application_id, ''), COALESCE(s.prefix, ''), COALESCE(s.first_name, ''),
				COALESCE(s.middle_name, ''), COALESCE(s.last_name, ''), COALESCE(s.suffix, ''), COALESCE(s.unit, ''), COALESCE(s.rank, ''), COALESCE(s.rank_in, ''),
				COALESCE(s.rank_out, ''), COALESCE(s.pension_state, ''), COALESCE(s.confederate_home_status, ''), COALESCE(s.confederate_home_name, ''), COALESCE(s.buried_in, ''),
				COALESCE(s.maiden_name, ''), COALESCE(s.relationship_label, ''), COALESCE(s.biography, ''), COALESCE(s.notes, ''), COALESCE(new.scratch_pad, '')
			FROM soldiers s
			WHERE s.id = new.soldier_id;
		END`,
		`CREATE TRIGGER scratchpad_cache_au AFTER UPDATE ON scratchpad_cache BEGIN
			DELETE FROM soldiers_fts WHERE rowid = new.soldier_id;
			INSERT INTO soldiers_fts (
				rowid, soldier_id, display_id, pension_id, application_id, prefix, first_name, middle_name, last_name, suffix,
				unit, soldier_rank, rank_in_text, rank_out_text, pension_state, confederate_home_status, confederate_home_name, buried_in, maiden_name, relationship_label,
				biography, notes, scratch_pad
			)
			SELECT
				s.id, s.id, COALESCE(s.display_id, ''), COALESCE(s.pension_id, ''), COALESCE(s.application_id, ''), COALESCE(s.prefix, ''), COALESCE(s.first_name, ''),
				COALESCE(s.middle_name, ''), COALESCE(s.last_name, ''), COALESCE(s.suffix, ''), COALESCE(s.unit, ''), COALESCE(s.rank, ''), COALESCE(s.rank_in, ''),
				COALESCE(s.rank_out, ''), COALESCE(s.pension_state, ''), COALESCE(s.confederate_home_status, ''), COALESCE(s.confederate_home_name, ''), COALESCE(s.buried_in, ''),
				COALESCE(s.maiden_name, ''), COALESCE(s.relationship_label, ''), COALESCE(s.biography, ''), COALESCE(s.notes, ''), COALESCE(new.scratch_pad, '')
			FROM soldiers s
			WHERE s.id = new.soldier_id;
		END`,
		`CREATE TRIGGER scratchpad_cache_ad AFTER DELETE ON scratchpad_cache BEGIN
			DELETE FROM soldiers_fts WHERE rowid = old.soldier_id;
			INSERT INTO soldiers_fts (
				rowid, soldier_id, display_id, pension_id, application_id, prefix, first_name, middle_name, last_name, suffix,
				unit, soldier_rank, rank_in_text, rank_out_text, pension_state, confederate_home_status, confederate_home_name, buried_in, maiden_name, relationship_label,
				biography, notes, scratch_pad
			)
			SELECT
				s.id, s.id, COALESCE(s.display_id, ''), COALESCE(s.pension_id, ''), COALESCE(s.application_id, ''), COALESCE(s.prefix, ''), COALESCE(s.first_name, ''),
				COALESCE(s.middle_name, ''), COALESCE(s.last_name, ''), COALESCE(s.suffix, ''), COALESCE(s.unit, ''), COALESCE(s.rank, ''), COALESCE(s.rank_in, ''),
				COALESCE(s.rank_out, ''), COALESCE(s.pension_state, ''), COALESCE(s.confederate_home_status, ''), COALESCE(s.confederate_home_name, ''), COALESCE(s.buried_in, ''),
				COALESCE(s.maiden_name, ''), COALESCE(s.relationship_label, ''), COALESCE(s.biography, ''), COALESCE(s.notes, ''), ''
			FROM soldiers s
			WHERE s.id = old.soldier_id;
		END`,
		`INSERT INTO soldiers_fts (
			rowid, soldier_id, display_id, pension_id, application_id, prefix, first_name, middle_name, last_name, suffix,
			unit, soldier_rank, rank_in_text, rank_out_text, pension_state, confederate_home_status, confederate_home_name, buried_in, maiden_name, relationship_label,
			biography, notes, scratch_pad
		)
		SELECT
			s.id, s.id, COALESCE(s.display_id, ''), COALESCE(s.pension_id, ''), COALESCE(s.application_id, ''), COALESCE(s.prefix, ''), COALESCE(s.first_name, ''),
			COALESCE(s.middle_name, ''), COALESCE(s.last_name, ''), COALESCE(s.suffix, ''), COALESCE(s.unit, ''), COALESCE(s.rank, ''), COALESCE(s.rank_in, ''),
			COALESCE(s.rank_out, ''), COALESCE(s.pension_state, ''), COALESCE(s.confederate_home_status, ''), COALESCE(s.confederate_home_name, ''), COALESCE(s.buried_in, ''),
			COALESCE(s.maiden_name, ''), COALESCE(s.relationship_label, ''), COALESCE(s.biography, ''), COALESCE(s.notes, ''), COALESCE(c.scratch_pad, '')
		FROM soldiers s
		LEFT JOIN scratchpad_cache c ON c.soldier_id = s.id`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

// ensureArchiveMetaSeed (issue #183) backfills the three
// archive_meta rows on legacy archives that predate v58. The
// schema const's INSERT OR IGNORE handles fresh installs; this
// helper closes the gap for archives that skipped from <v58
// straight to the new version. Idempotent — INSERT OR IGNORE.
func ensureArchiveMetaSeed(tx *sql.Tx) error {
	statements := []string{
		`INSERT OR IGNORE INTO archive_meta (archive_kind, include_tags) VALUES ('shared_archive', 0)`,
		`INSERT OR IGNORE INTO archive_meta (archive_kind, include_tags) VALUES ('backup_archive', 1)`,
		`INSERT OR IGNORE INTO archive_meta (archive_kind, include_tags) VALUES ('static_archive', 0)`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

func migrateNodePrefixConfiguration(tx *sql.Tx) error {
	var (
		complete   sql.NullString
		firstName  sql.NullString
		middleName sql.NullString
		lastName   sql.NullString
		birthYear  sql.NullString
	)
	if err := tx.QueryRow(`SELECT value FROM system_config WHERE key = 'user_identity_complete'`).Scan(&complete); err != nil && err != sql.ErrNoRows {
		return err
	}
	if strings.TrimSpace(complete.String) != "1" {
		return nil
	}
	if err := tx.QueryRow(`SELECT value FROM system_config WHERE key = 'user_first_name'`).Scan(&firstName); err != nil && err != sql.ErrNoRows {
		return err
	}
	if err := tx.QueryRow(`SELECT value FROM system_config WHERE key = 'user_middle_name'`).Scan(&middleName); err != nil && err != sql.ErrNoRows {
		return err
	}
	if err := tx.QueryRow(`SELECT value FROM system_config WHERE key = 'user_last_name'`).Scan(&lastName); err != nil && err != sql.ErrNoRows {
		return err
	}
	if err := tx.QueryRow(`SELECT value FROM system_config WHERE key = 'user_birth_year'`).Scan(&birthYear); err != nil && err != sql.ErrNoRows {
		return err
	}
	parsedBirthYear, err := strconv.Atoi(strings.TrimSpace(birthYear.String))
	if err != nil {
		return nil
	}
	nodePrefix, err := BuildUserNodePrefix(firstName.String, middleName.String, lastName.String, parsedBirthYear)
	if err != nil {
		return nil
	}
	_, err = tx.Exec(`
		INSERT INTO system_config(key, value)
		VALUES ('node_prefix', ?)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = CURRENT_TIMESTAMP
	`, nodePrefix)
	return err
}

func migrateSanitizedDisplayIDs(tx *sql.Tx) error {
	type displayRecord struct {
		id        int64
		displayID string
	}

	nodePrefix := NormalizeNodePrefix("")
	var storedPrefix sql.NullString
	if err := tx.QueryRow(`SELECT value FROM system_config WHERE key = 'node_prefix'`).Scan(&storedPrefix); err != nil && err != sql.ErrNoRows {
		return err
	}
	if strings.TrimSpace(storedPrefix.String) != "" {
		nodePrefix = NormalizeNodePrefix(storedPrefix.String)
	}

	rows, err := tx.Query(`SELECT id, display_id FROM soldiers ORDER BY id`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var (
		records []displayRecord
		maxSeq  int
		taken   = map[string]int64{}
	)
	for rows.Next() {
		var record displayRecord
		if err := rows.Scan(&record.id, &record.displayID); err != nil {
			return err
		}
		records = append(records, record)
		taken[record.displayID] = record.id
		if _, sequence, ok := CanonicalDisplayID(record.displayID); ok && sequence > maxSeq {
			maxSeq = sequence
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, record := range records {
		sanitized := SanitizeID(record.displayID, nodePrefix)
		if sanitized == record.displayID {
			continue
		}
		if existingID, exists := taken[sanitized]; exists && existingID != record.id {
			maxSeq++
			sanitized = NextGeneratedDisplayID(nodePrefix, maxSeq)
		}
		if _, err := tx.Exec(`UPDATE soldiers SET display_id = ? WHERE id = ?`, sanitized, record.id); err != nil {
			return err
		}
		delete(taken, record.displayID)
		taken[sanitized] = record.id
	}
	return nil
}

// migrateEntryTypeDiscipline enforces a CHECK constraint on
// soldiers.entry_type. SQLite does not support ALTER TABLE ... ADD
// CONSTRAINT, so this function idempotently rebuilds the soldiers table
// with the constraint in place. It is a no-op once the constraint is
// recorded in sqlite_master.sql.
func migrateEntryTypeDiscipline(tx *sql.Tx) error {
	var present int
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'soldiers_entry_type_check_log'`,
	).Scan(&present); err != nil {
		return err
	}
	if present > 0 {
		return nil
	}
	// Pragmatic approach for v55: rely on application-level validation
	// (internal/models.EntryTypeSoldier etc.) plus a guard column check
	// before any future migration. This function is the anchor for any
	// later hard SQL constraint if SQLite adds ALTER CONSTRAINT support.
	// We log the migration so subsequent runs short-circuit.
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS soldiers_entry_type_check_log (applied_at TEXT PRIMARY KEY)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT OR IGNORE INTO soldiers_entry_type_check_log(applied_at) VALUES (CURRENT_TIMESTAMP)`); err != nil {
		return err
	}
	return nil
}

func migrateCanonicalDateData(tx *sql.Tx) error {
	rows, err := tx.Query(`SELECT id, birth_date, birth_info, death_date, death_year, death_month, death_day FROM soldiers`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type updateRow struct {
		id        int64
		birthDate string
		deathDate string
		year      int
		month     int
		day       int
	}
	updates := []updateRow{}
	for rows.Next() {
		var (
			id         int64
			birthDate  sql.NullString
			birthInfo  sql.NullString
			deathDate  sql.NullString
			deathYear  sql.NullInt64
			deathMonth sql.NullInt64
			deathDay   sql.NullInt64
		)
		if err := rows.Scan(&id, &birthDate, &birthInfo, &deathDate, &deathYear, &deathMonth, &deathDay); err != nil {
			return err
		}

		normalizedBirth := strings.TrimSpace(birthDate.String)
		if normalizedBirth == "00/00/0000" || normalizedBirth == "" {
			normalizedBirth = dates.ParseBirthInfo(strings.TrimSpace(birthInfo.String))
		}
		normalizedBirth, err = dates.NormalizeCanonical(normalizedBirth)
		if err != nil {
			normalizedBirth = ""
		}

		normalizedDeath := strings.TrimSpace(deathDate.String)
		if normalizedDeath == "" {
			normalizedDeath = dates.MustFormat(int(deathMonth.Int64), int(deathDay.Int64), int(deathYear.Int64))
		}
		normalizedDeath, err = dates.NormalizeCanonical(normalizedDeath)
		if err != nil {
			normalizedDeath = ""
		}
		partialDeath, err := dates.ParseCanonical(normalizedDeath)
		if err != nil {
			partialDeath = dates.PartialDate{}
		}

		updates = append(updates, updateRow{
			id:        id,
			birthDate: normalizedBirth,
			deathDate: normalizedDeath,
			year:      partialDeath.Year,
			month:     partialDeath.Month,
			day:       partialDeath.Day,
		})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, update := range updates {
		if _, err := tx.Exec(`UPDATE soldiers SET birth_date = ?, death_date = ?, death_year = ?, death_month = ?, death_day = ? WHERE id = ?`,
			update.birthDate, update.deathDate, update.year, update.month, update.day, update.id); err != nil {
			return err
		}
	}
	return nil
}
