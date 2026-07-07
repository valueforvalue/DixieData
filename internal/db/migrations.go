package db

import (
	"database/sql"
	"errors"
	"fmt"
)

// Reversibility classifies a single migration block by whether its
// effect can be cleanly reversed. Issue #273 ("feat(cli): add 'migrate
// down <version>'") and docs/migrations/reversibility.md enumerate
// every block; the values here are the runtime contract.
//
//   - Reversible: pure additive (CREATE TABLE IF NOT EXISTS,
//     ADD COLUMN with default, CREATE INDEX). Apply the inverse on
//     DOWN.
//   - PartiallyReversible: requires data transformation; pre-state
//     is recoverable from columns still on the row. Apply the inverse
//     with best-effort; user verifies post-down.
//   - Irreversible: pre-state discarded (random UUID, lossy printf,
//     sanitization, canonicalization). Refuse DOWN past this block
//     unless a restore-point ID is supplied.
//
// The per-block catalogue is at docs/migrations/reversibility.md.
type Reversibility int

const (
	// Reversible marks blocks whose effect can be inverted by a
	// single SQL statement (typically DROP TABLE IF EXISTS,
	// DROP COLUMN, DROP INDEX).
	Reversible Reversibility = iota
	// PartiallyReversible marks blocks whose inverse is best-effort:
	// pre-state is recoverable from columns still on the row, but
	// applying the inverse may over-correct or skip rows whose
	// pre-state marker is absent.
	PartiallyReversible
	// Irreversible marks blocks that discard pre-state without
	// preservation. DOWN runners must refuse past these without
	// explicit acknowledgement (--force-irreversible) or a
	// restore-point ID.
	Irreversible
)

// String returns the lowercase snake_case label for the reversibility
// class. Used by the audit harness + CLI "what was lost" manifest.
func (r Reversibility) String() string {
	switch r {
	case Reversible:
		return "reversible"
	case PartiallyReversible:
		return "partially_reversible"
	case Irreversible:
		return "irreversible"
	default:
		return "unknown"
	}
}

// Migration is a single forward schema op paired with an inverse
// (where one exists) and a reversibility class. The Up function
// runs inside the applySchema transaction in slice order; the
// Down function runs inside applyDownSchema in REVERSE slice
// order from `current` to `target+1`.
//
// The fields:
//   - ID: stable string identifier. Used by the DOWN runner to skip
//     blocks by name and by the audit harness to print a per-block
//     table. Convention: "block-N-<slug>" where N is the execution
//     order from the catalogue in docs/migrations/reversibility.md.
//   - Up: the forward SQL/HELPER step. Returns nil to indicate
//     success; non-nil to abort the transaction (applySchema's
//     defer tx.Rollback() handles the unwind).
//   - Down: the inverse step, where one exists. For Irreversible
//     blocks, Down returns ErrMigrationIrreversible to refuse the
//     DOWN path at runner time unless --force-irreversible is
//     supplied (in which case the runner still gets a refusal — see
//     the CLI's runAdminMigrateDown for the full contract). For
//     PartiallyReversible blocks, Down performs best-effort
//     inversion with the understanding that some rows may be
//     over-corrected or skipped. For Reversible blocks, Down is
//     the precise inverse.
//   - Reversibility: classification per the Reversibility enum.
//   - Reason: one-line human-readable explanation cited by the
//     audit catalogue. Used by the "what was lost" manifest.
type Migration struct {
	ID            string
	Up            func(*sql.Tx) error
	Down          func(*sql.Tx) error
	Reversibility Reversibility
	Reason        string
}

// ErrMigrationIrreversible is returned by Migration.Down when the
// block's effect cannot be cleanly inverted. The CLI runner surfaces
// this as a refusal unless --force-irreversible is supplied (which
// still prints the "what was lost" manifest but does NOT bypass the
// refusal — see the design decisions captured in the issue #273
// audit at docs/migrations/reversibility.md).
var ErrMigrationIrreversible = errors.New("migration is irreversible")

// ErrDowngradeRefused is the umbrella error returned by
// applyDownSchema when the path crosses an Irreversible block.
// The CLI unwraps this to find the blocking Migration ID and prints
// it to the operator along with the per-block Reason.
var ErrDowngradeRefused = errors.New("schema downgrade refused")

// migrations enumerates every block that runs inside applySchema in
// execution order. The catalogue is the source of truth for the
// future DOWN runner (issue #273); docs/migrations/reversibility.md
// mirrors this slice's per-block reasoning.
//
// Block numbering follows docs/migrations/reversibility.md:
//
// Pre-#320 v1-v53 blocks (1-17) have been collapsed into the
// block-1 inline schema for fresh installs + a consolidated
// v54→v60 jump block (block-1.5). The catalogue below reflects
// the new shape. The v1-v53 chain is no longer a separate slice
// entry; the inline `schema` constant in block-1 covers every
// table the old chain would have built incrementally.
//
//   Block 1   - Inline `tx.Exec(schema)` constant (CREATE TABLE +
//               CREATE INDEX + archive_meta seed) for the full
//               v60+ surface. Covers v1-v53 + v54-v59 + v60
//               (everything fresh installs need).
//   Block 1.5 - Consolidated v54→v60 jump (issue #320, replaces
//               the v60 work + all the v1-v53 block chain).
//               Adds scratchpad_cache table + 4 soldiers columns
//               + 12 RENAME COLUMN (soldier_id/soldier_sync_id →
//               person_record_id/person_sync_id) + creates
//               event_person_links junction. Idempotent on
//               fresh installs (every sub-block is guarded).
//   Block 2   - event_sources table (v61, issue #340). The
//               per-Event Source Records table that fixes the
//               v60 slot #329 data-loss bug.
//
// The terminal `PRAGMA user_version` write is NOT in the slice —
// it's bookkeeping applied by applySchema after the slice
// iteration completes, mirroring the UP path's terminal write.
var migrations = []Migration{
	// Block 1 - Inline CREATE TABLE / CREATE INDEX / archive_meta seed.
	// Reversible: DROP TABLE IF EXISTS for each, DROP INDEX IF EXISTS
	// for each, DELETE FROM archive_meta WHERE archive_kind IN (...).
	{
		ID:            "block-1-schema-baseline",
		Reversibility: Reversible,
		Reason: "Pure additive: 19 CREATE TABLE IF NOT EXISTS + 11+ CREATE INDEX IF NOT EXISTS + archive_meta seed. Inverse: DROP TABLE/INDEX + DELETE seed rows.",
		Up: func(tx *sql.Tx) error {
			_, err := tx.Exec(schema)
			return err
		},
		Down: func(tx *sql.Tx) error {
			return reverseSchemaBaseline(tx)
		},
	},
	// Block 1.5 (block-60) — Consolidated v54→v60 jump (issue #320).
	// Replaces the incremental v1-v17 block chain (block-2 ADD
	// COLUMN loop, block-3 is_generated flip, block-4 phase1
	// distributed-merge, block-5 canonical dates, block-6
	// soldiers normalization, block-7 last_edited_at backfill,
	// block-8 images is_primary, block-9/10 indexes,
	// block-11 node prefix, block-12 sanitized display ids,
	// block-13 canonical date data, block-14 ensureSoldierFTS,
	// block-15 archive_meta seed, block-16 entry_type
	// discipline, block-17 research_log evidence_type rename) +
	// the v60-specific work (column renames, scratchpad_cache
	// table, event_person_links junction, soldiers Event Record
	// columns). Every sub-block is guarded by columnExists /
	// CREATE TABLE IF NOT EXISTS so the block is idempotent on
	// fresh installs (where the v60 column names + new tables
	// are already inline in the block-1 schema constant) AND on
	// the legacy v54 production archives.
	//
	// v54 archives carry the old column names (soldier_id,
	// soldier_sync_id, local_soldier_id, etc.) + lack the
	// scratchpad_cache table + the v60 soldiers columns. The
	// v54→v60 path is the only upgrade path the migration has
	// to support in practice; v1-v53 archives pre-date the
	// v52 doc discipline and are not expected in the wild.
	// The DOWN runner can still reflow column names + drop the
	// v60 tables for a v60→v54 reverse, but the v1-v53 chain is
	// not reversible from this position.
	//
	// Sub-blocks (run in this order within one tx):
	//   A0.  CREATE TABLE scratchpad_cache + index. New in v60.
	//   A0.5. 4 ADD COLUMN on soldiers (kind, begin_date, end_date,
	//        description) for the Event Record subtype. Guarded
	//        by columnExists so fresh installs (where block-1
	//        inline-created them) are no-ops.
	//   A1. CREATE TABLE event_person_links + 3 indexes.
	//   B.  12 RENAME COLUMN statements (soldier_id→person_record_id
	//        + soldier_sync_id→person_sync_id across 5 FK tables;
	//        local_soldier_id→local_record_id, left/right_soldier_id
	//        → left/right_record_id in the 2 conflict tables).
	//        Each guarded by columnExists.
	//   D.  UPDATE records SET person_sync_id backfill (skipped if
	//        soldiers.sync_id doesn't exist — pre-v52 archives
	//        handled that via block-4 in the old chain; the
	//        block-4 phase1 migration's records UPDATE handles
	//        it for fresh v54 installs that somehow lack the
	//        sync_id column).
	//
	// The FTS5 trigger DROP+RECREATE that was in the v60 sub-block
	// C of the original v60 migration is now handled by the
	// standalone ensureSoldierFTS call site that runs after this
	// block — the trigger text references columns block-2 added
	// to the inline schema on fresh installs (biography,
	// maiden_name, etc.), so the rebuild has to happen AFTER
	// every column is in place.
	{
		ID:            "block-60-v54-to-v60-jump",
		Reversibility: Irreversible,
		Reason: "Consolidated v54→v60 migration: 4 soldiers columns (kind, begin_date, end_date, description) + scratchpad_cache table + 12 RENAME COLUMN (soldier_id→person_record_id + soldier_sync_id→person_sync_id across 5 FK tables) + event_person_links table + 3 indexes + sync_id backfill. RENAME COLUMN is technically reversible, but the v60 Event Records (the user-added rows in event_person_links keyed by the new FK column names) would be lost on a v60→v54 reverse, and the soldiers-kind/begin_date/end_date columns would be re-dropped even if the user has data in them. Classified Irreversible per the conservative rule: any user-added data the path would discard is reason enough.",
		Up: func(tx *sql.Tx) error {
			// Pre-#320 v1-v53 archives pre-date the inline schema's
			// column coverage. Run the ADD COLUMN loop first so
			// every column the rest of this block + the inline
			// schema assumes is present on the v54-or-earlier row.
			// The loop is columnExists-guarded so fresh installs
			// (where block-1 inline-created every column) are
			// no-ops.
			if err := applyAddColumnLoop(tx); err != nil {
				return err
			}

			// v1-v54 archives carry pre-normalization values
			// (pension_state='None', confederate_home_status='None',
			// needs_review NULL, etc.). The 7-UPDATE chain in
			// applySoldiersNormalization brings legacy rows into
			// compliance. Runs against the OLD column name set
			// (soldiers.* haven't been renamed yet at this point)
			// so no columnExists guard needed.
			if err := applySoldiersNormalization(tx); err != nil {
				return err
			}

			// v54-or-earlier images may have is_primary=NULL or no
			// primary image elected. applyImagesIsPrimary
			// NULL-coalesces and elects MIN(id) as primary for each
			// (person_record_id | soldier_id) — the helper picks
			// the column that exists. Runs against the OLD name set
			// so the GROUP BY hits soldier_id.
			if err := applyImagesIsPrimary(tx); err != nil {
				return err
			}

			// Sub-block A0: scratchpad_cache table (new in v60). The
			// inline CREATE TABLE in block-1 only creates it on fresh
			// installs; v54→v60 upgrades need this explicit create
			// before the column-rename sub-block can touch it.
			if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS scratchpad_cache (
				person_record_id INTEGER PRIMARY KEY REFERENCES soldiers(id) ON DELETE CASCADE,
				scratch_pad TEXT NOT NULL DEFAULT '',
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
			)`); err != nil {
				return err
			}
			if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_scratchpad_cache_updated_at ON scratchpad_cache(updated_at)`); err != nil {
				return err
			}

			// Sub-block A0.5: v60-specific soldiers columns (kind /
			// begin_date / end_date / description) for the Event
			// Record subtype. Inline CREATE has them on fresh
			// installs; v54→v60 upgrades need the ADD COLUMN.
			for _, col := range []struct{ name, def string }{
				{"kind", "TEXT"},
				{"begin_date", "TEXT"},
				{"end_date", "TEXT"},
				{"description", "TEXT"},
			} {
				exists, err := columnExists(tx, "soldiers", col.name)
				if err != nil {
					return err
				}
				if exists {
					continue
				}
				if _, err := tx.Exec(fmt.Sprintf(`ALTER TABLE soldiers ADD COLUMN %s %s`, col.name, col.def)); err != nil {
					return err
				}
			}

			// Sub-block A1: CREATE TABLE event_person_links + indexes.
			if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS event_person_links (
				id             INTEGER PRIMARY KEY AUTOINCREMENT,
				event_id       INTEGER NOT NULL REFERENCES soldiers(id) ON DELETE CASCADE,
				person_id      INTEGER NOT NULL REFERENCES soldiers(id) ON DELETE CASCADE,
				sync_id        TEXT,
				event_sync_id  TEXT,
				person_sync_id TEXT,
				created_at     DATETIME DEFAULT CURRENT_TIMESTAMP,
				UNIQUE (event_id, person_id)
			)`); err != nil {
				return err
			}
			if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_event_person_links_event ON event_person_links(event_id)`); err != nil {
				return err
			}
			if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_event_person_links_person ON event_person_links(person_id)`); err != nil {
				return err
			}
			if _, err := tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_event_person_links_sync_id ON event_person_links(sync_id)`); err != nil {
				return err
			}

			// Sub-block B: 12 RENAME COLUMN statements. Each guarded by
			// columnExists so the block is idempotent on a v60 fresh
			// install (where the new column name is already inline).
			renames := []struct{ table, from, to string }{
				{"records", "soldier_id", "person_record_id"},
				{"records", "soldier_sync_id", "person_sync_id"},
				{"images", "soldier_id", "person_record_id"},
				{"images", "soldier_sync_id", "person_sync_id"},
				{"scratchpad_cache", "soldier_id", "person_record_id"},
				{"scratchpad_cache", "soldier_sync_id", "person_sync_id"},
				{"research_tasks", "soldier_id", "person_record_id"},
				{"research_tasks", "soldier_sync_id", "person_sync_id"},
				{"merge_review_conflicts", "local_soldier_id", "local_record_id"},
				{"merge_review_conflicts", "left_soldier_id", "left_record_id"},
				{"merge_review_conflicts", "right_soldier_id", "right_record_id"},
				{"duplicate_audit_findings", "left_soldier_id", "left_record_id"},
				{"duplicate_audit_findings", "right_soldier_id", "right_record_id"},
			}
			for _, r := range renames {
				exists, err := columnExists(tx, r.table, r.from)
				if err != nil {
					return err
				}
				if !exists {
					continue
				}
				stmt := fmt.Sprintf(`ALTER TABLE %s RENAME COLUMN %s TO %s`, r.table, r.from, r.to)
				if _, err := tx.Exec(stmt); err != nil {
					return err
				}
			}

			// Sub-block C (FTS5 trigger DROP+RECREATE) was here
			// before block-60 was moved to slot 1.5. It called
			// ensureSoldierFTS — but that helper references
			// columns like biography + maiden_name + relationship_label
			// that block-2 (the ADD COLUMN loop) hasn't added yet
			// when block-60 runs at slot 1.5. Block-14 already
			// calls ensureSoldierFTS AFTER block-2 completes, so
			// the FTS5 trigger rebuild is deferred there. Removed
			// from this block.

			// Sub-block D: backfill person_sync_id on records/images.
			// Runs AFTER sub-block B so the column names are
			// post-rename. The UPDATE references soldiers.sync_id
			// which block-2 (ADD COLUMN loop) adds to pre-v52
			// archives; the soldiersSyncIDExists guard makes this
			// sub-block a no-op for archives that pre-date the
			// sync_id column.
			if exists, err := columnExists(tx, "soldiers", "sync_id"); err != nil {
				return err
			} else if exists {
				if _, err := tx.Exec(`UPDATE records
					SET person_sync_id = (
						SELECT soldiers.sync_id
						FROM soldiers
						WHERE soldiers.id = records.person_record_id
					)
					WHERE person_sync_id IS NULL OR TRIM(person_sync_id) = ''`); err != nil {
					return err
				}
				if _, err := tx.Exec(`UPDATE images
					SET person_sync_id = (
						SELECT soldiers.sync_id
						FROM soldiers
						WHERE soldiers.id = images.person_record_id
					)
					WHERE person_sync_id IS NULL OR TRIM(person_sync_id) = ''`); err != nil {
					return err
					}
			}

			// The Phase 1 distributed-merge backfill: populate
			// sync_id + person_sync_id + node_prefix + node_id
			// on v1-v53 archives that pre-date the sync_id
			// discipline. Runs AFTER the renames so the UPDATEs
			// hit the post-rename column names. Idempotent on
			// fresh installs (every UPDATE is a no-op and the
			// system_config seed already ran inline).
			if err := applyPhase1DistributedMerge(tx); err != nil {
				return err
			}

			// FTS5 setup. The old block-14 handled this; with the
			// v1-v53 chain collapsed, the soldiers_fts table +
			// its 6 triggers need to be created here for v54
			// archives. The CREATE VIRTUAL TABLE / CREATE TRIGGER
			// statements are idempotent (the helper DROPs them
			// first then re-creates) so fresh installs (where
			// block-1's inline schema will eventually grow to
			// include them) are also safe.
			if err := ensureSoldierFTS(tx); err != nil {
				return err
			}

			return nil
		},
		Down: refuseDown,
	},
	// Block 2 (block-61) — v61 per-Event Source Records table
	// (issue #340). Replaces the v60 slot #329 hack of writing
	// Event sources into the shared `records` table, which
	// collided with the replaceRecords REPLACE-only semantics
	// in SoldierService.Update and silently destroyed attached
	// sources on every Event Edit. The new event_sources table
	// is keyed by event_id (soldiers.id for entry_type='event'
	// rows) and mirrors the records column shape so the
	// EventService can swap implementations without changing
	// handler / viewmodel signatures.
	//
	// Reversibility: Reversible. The Up path is pure additive
	// (CREATE TABLE IF NOT EXISTS + 2 CREATE INDEX IF NOT
	// EXISTS). The Down path drops the unique index, the event
	// index, and the table itself. No data migration needed
	// because the orphan rows from v60's records-table reuse
	// are inert after the v61 EventService rewrite (slice 2).
	{
		ID:            "block-2-event-sources",
		Reversibility: Reversible,
		Reason: "Pure additive: CREATE TABLE IF NOT EXISTS event_sources + 2 CREATE INDEX IF NOT EXISTS. Inverse: DROP INDEX + DROP TABLE. No data migration; the v60 records-table orphans become inert after slice 2's service rewrite.",
		Up: func(tx *sql.Tx) error {
			if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS event_sources (
				id              INTEGER PRIMARY KEY AUTOINCREMENT,
				sync_id         TEXT,
				event_id        INTEGER NOT NULL REFERENCES soldiers(id) ON DELETE CASCADE,
				event_sync_id   TEXT,
				record_type     TEXT NOT NULL,
				app_id          TEXT NOT NULL,
				details         TEXT NOT NULL DEFAULT ''
			)`); err != nil {
				return err
			}
			if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_event_sources_event ON event_sources(event_id)`); err != nil {
				return err
			}
			if _, err := tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_event_sources_sync_id ON event_sources(sync_id)`); err != nil {
				return err
			}
			return nil
		},
		Down: func(tx *sql.Tx) error {
			if _, err := tx.Exec(`DROP INDEX IF EXISTS idx_event_sources_sync_id`); err != nil {
				return err
			}
			if _, err := tx.Exec(`DROP INDEX IF EXISTS idx_event_sources_event`); err != nil {
				return err
			}
			if _, err := tx.Exec(`DROP TABLE IF EXISTS event_sources`); err != nil {
				return err
			}
			return nil
		},
	},
	// Block 3 (block-62) — Article Records schema (issue #321
	// slice 1). Adds the articles + article_refs tables that
	// Article CRUD lives in; Article rows are siblings of
	// soldiers rows, not subtypes, so they get their own table
	// per locked decision #2 (same-DB parallel tables) +
	// locked decision #3 (two-table split lets ref columns
	// grow position / kind later without rewriting articles).
	//
	// articles columns:
	//   id                  -- primary key (SQLite row id; the URL
	//                          segment for /articles/{id})
	//   sync_id             -- per-row distributed-merge UUID; mirrors
	//                          the soldiers.sync_id pattern
	//   display_id          -- ART-NNNNN; minted by db.NextArticleID
	//   title               -- required, trimmed
	//   subtitle            -- optional, trimmed
	//   body_md             -- the markdown source (raw)
	//   body_html           -- the sanitized rendered HTML; the
	//                          slice-1 create path stores the raw md
	//                          verbatim in body_html so the first read
	//                          shows the body without a renderer.
	//                          Slice 2 will replace this column-write
	//                          with a goldmark + bluemonday pass.
	//   created_at / updated_at -- timestamps
	//   snapshot_of_id      -- nullable; non-null on rows that are
	//                          a "Save copy" snapshot of another row.
	//                          Slice 2.5 fills this in.
	//   is_snapshot         -- 0/1 mirror of (snapshot_of_id IS NOT
	//                          NULL); exists so the read path can
	//                          filter snapshots without a join.
	//
	// article_refs columns:
	//   id                  -- primary key
	//   article_id          -- FK to articles(id) ON DELETE CASCADE
	//   article_sync_id     -- mirror for distributed merge
	//   person_record_id    -- FK to soldiers(id) ON DELETE CASCADE
	//   person_record_sync_id -- mirror
	//   person_display_id   -- denormalized cache of the DXD/EVT id
	//                          so the Cite-in reverse-lookup + the
	//                          PDF resolver can render without a join.
	//   position            -- reserved for future "reorder refs"; defaults
	//                          to 0 today; Slice 3+ may surface a reorder
	//                          UI mirroring #368's source-record reorder.
	//
	// Reversibility: Reversible. The Up path is pure additive
	// (CREATE TABLE IF NOT EXISTS + 4 CREATE INDEX IF NOT
	// EXISTS). The Down path drops the 4 indexes + the 2 tables.
	// No data migration; the v61 → v62 path leaves existing
	// soldiers / records / event_sources / event_person_links
	// rows alone.
	{
		ID:            "block-3-articles",
		Reversibility: Reversible,
		Reason: "Pure additive: CREATE TABLE IF NOT EXISTS articles + article_refs + 4 CREATE INDEX IF NOT EXISTS. Inverse: DROP INDEX + DROP TABLE for each. No data migration; Articles are a greenfield entity (issue #321 locked decision #2).",
		Up: func(tx *sql.Tx) error {
			if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS articles (
				id                   INTEGER PRIMARY KEY AUTOINCREMENT,
				sync_id              TEXT,
				display_id           TEXT NOT NULL UNIQUE,
				title                TEXT NOT NULL,
				subtitle             TEXT NOT NULL DEFAULT '',
				body_md              TEXT NOT NULL DEFAULT '',
				body_html            TEXT NOT NULL DEFAULT '',
				created_at           TEXT NOT NULL,
				updated_at           TEXT NOT NULL,
				snapshot_of_id       INTEGER,
				is_snapshot          INTEGER NOT NULL DEFAULT 0
			)`); err != nil {
				return err
			}
			if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_articles_sync_id ON articles(sync_id)`); err != nil {
				return err
			}
			if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_articles_snapshot_of ON articles(snapshot_of_id)`); err != nil {
				return err
			}
			if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS article_refs (
				id                       INTEGER PRIMARY KEY AUTOINCREMENT,
				article_id               INTEGER NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
				article_sync_id          TEXT,
				person_record_id         INTEGER NOT NULL REFERENCES soldiers(id) ON DELETE CASCADE,
				person_record_sync_id    TEXT,
				person_display_id        TEXT NOT NULL,
				position                 INTEGER NOT NULL DEFAULT 0
			)`); err != nil {
				return err
			}
			if _, err := tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_article_refs_article_person ON article_refs(article_id, person_record_id)`); err != nil {
				return err
			}
			if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_article_refs_person ON article_refs(person_record_id)`); err != nil {
				return err
			}
			return nil
		},
		Down: func(tx *sql.Tx) error {
			if _, err := tx.Exec(`DROP INDEX IF EXISTS idx_article_refs_person`); err != nil {
				return err
			}
			if _, err := tx.Exec(`DROP INDEX IF EXISTS idx_article_refs_article_person`); err != nil {
				return err
			}
			if _, err := tx.Exec(`DROP TABLE IF EXISTS article_refs`); err != nil {
				return err
			}
			if _, err := tx.Exec(`DROP INDEX IF EXISTS idx_articles_snapshot_of`); err != nil {
				return err
			}
			if _, err := tx.Exec(`DROP INDEX IF EXISTS idx_articles_sync_id`); err != nil {
				return err
			}
			if _, err := tx.Exec(`DROP TABLE IF EXISTS articles`); err != nil {
				return err
			}
			return nil
		},
	},
	// Block 4 (block-63) — Source Record reorder column
	// (issue #368, slice 1). Adds `sort_order INTEGER NOT NULL
	// DEFAULT 0` to both Source Record tables so the read paths
	// can order rows by user intent instead of insertion order.
	// The PATCH /soldiers/{id}/sources/{sourceId}/position and
	// PATCH /events/{id}/sources/{sourceId}/position endpoints
	// land in slice 3; slice 1 is the tracer bullet (schema +
	// read-path ORDER BY change only) so existing writes
	// continue to insert with sort_order=0 (the default) and
	// existing archives preserve current display order via the
	// backfill that assigns sort_order = id.
	//
	// Reversibility: Reversible. ADD COLUMN is reversible
	// (DROP COLUMN works because sort_order has no FK + no
	// inbound references). The backfill UPDATE is a no-op
	// reverse on DOWN (column gets dropped; rows lose their
	// sort_order values; the read path flips back to
	// ORDER BY id on code revert).
	{
		ID:            "block-63-source-sort-order",
		Reversibility: Reversible,
		Reason:        "Pure additive: ALTER TABLE ADD COLUMN sort_order on records + event_sources, with backfill UPDATE that sets sort_order = id so existing archives preserve current display order. No FK, no inbound references, no data loss on DOWN.",
		Up: func(tx *sql.Tx) error {
			recordsExists, err := columnExists(tx, "records", "sort_order")
			if err != nil {
				return err
			}
			if !recordsExists {
				if _, err := tx.Exec(`ALTER TABLE records ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0`); err != nil {
					return err
				}
			}
			// Backfill: every existing row gets sort_order = id
			// so the first read after upgrade preserves the
			// current id-order display. WHERE sort_order = 0 OR
			// sort_order IS NULL is defensive — on a fresh
			// install the inline schema carries DEFAULT 0 so
			// every row already has sort_order = 0, and the
			// UPDATE rewrites them all to id (still correct).
			if _, err := tx.Exec(`UPDATE records SET sort_order = id WHERE sort_order = 0 OR sort_order IS NULL`); err != nil {
				return err
			}
			eventSourcesExists, err := columnExists(tx, "event_sources", "sort_order")
			if err != nil {
				return err
			}
			if !eventSourcesExists {
				if _, err := tx.Exec(`ALTER TABLE event_sources ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0`); err != nil {
					return err
				}
			}
			if _, err := tx.Exec(`UPDATE event_sources SET sort_order = id WHERE sort_order = 0 OR sort_order IS NULL`); err != nil {
				return err
			}
			return nil
		},
		Down: func(tx *sql.Tx) error {
			recordsExists, err := columnExists(tx, "records", "sort_order")
			if err != nil {
				return err
			}
			if recordsExists {
				if _, err := tx.Exec(`ALTER TABLE records DROP COLUMN sort_order`); err != nil {
					return err
				}
			}
			eventSourcesExists, err := columnExists(tx, "event_sources", "sort_order")
			if err != nil {
				return err
			}
			if eventSourcesExists {
				if _, err := tx.Exec(`ALTER TABLE event_sources DROP COLUMN sort_order`); err != nil {
					return err
				}
			}
			return nil
		},
	},
	// Block 5 (block-64) — Row provenance columns on soldiers
	// (issue #377, slice 1). Adds `created_by_version TEXT NOT
	// NULL DEFAULT ''` + `created_by_import_path TEXT NOT NULL
	// DEFAULT ''` to the soldiers table so every row carries the
	// DixieData release + code path that wrote it. The motivating
	// case is the #376 diagnostic trail for soldier id=411: 30
	// minutes of git-archaeology + timestamp cross-referencing
	// would have collapsed into one query with these columns.
	//
	// Both columns default to '' so the backfill is a no-op on
	// fresh installs (the inline schema carries the columns
	// already) and pre-v64 rows land with '' rather than NULL.
	// The backfill UPDATE rewrites '' to "unknown" so the read
	// side can distinguish "never stamped" from "explicitly
	// empty" — defensive future-proofing since today every path
	// either stamps or leaves empty.
	//
	// Slice 2 (next session) wires every Create/Update call site
	// to stamp the appropriate path. Slice 1 only lands the
	// schema + struct + read path — the per-handler stamping is
	// mechanical (one of 8 path strings) and benefits from a
	// fresh-context review pass per the tracer-bullet rule.
	//
	// Reversibility: Reversible. ADD COLUMN is reversible
	// (DROP COLUMN works because created_by_version +
	// created_by_import_path are not FK-constrained and have
	// no inbound references). The backfill UPDATE is a no-op
	// reverse on DOWN (column gets dropped; rows lose their
	// provenance values; the read path stops returning them).
	{
		ID:            "block-64-row-provenance",
		Reversibility: Reversible,
		Reason:        "Pure additive: ALTER TABLE ADD COLUMN created_by_version + created_by_import_path on soldiers (both NOT NULL DEFAULT ''), with backfill UPDATE that rewrites '' to 'unknown' for pre-v64 rows. No FK, no inbound references, no data loss on DOWN.",
		Up: func(tx *sql.Tx) error {
			versionExists, err := columnExists(tx, "soldiers", "created_by_version")
			if err != nil {
				return err
			}
			if !versionExists {
				if _, err := tx.Exec(`ALTER TABLE soldiers ADD COLUMN created_by_version TEXT NOT NULL DEFAULT ''`); err != nil {
					return err
				}
			}
			if _, err := tx.Exec(`UPDATE soldiers SET created_by_version = 'unknown' WHERE created_by_version = ''`); err != nil {
				return err
			}
			pathExists, err := columnExists(tx, "soldiers", "created_by_import_path")
			if err != nil {
				return err
			}
			if !pathExists {
				if _, err := tx.Exec(`ALTER TABLE soldiers ADD COLUMN created_by_import_path TEXT NOT NULL DEFAULT ''`); err != nil {
					return err
				}
			}
			if _, err := tx.Exec(`UPDATE soldiers SET created_by_import_path = 'unknown' WHERE created_by_import_path = ''`); err != nil {
				return err
			}
			return nil
		},
		Down: func(tx *sql.Tx) error {
			versionExists, err := columnExists(tx, "soldiers", "created_by_version")
			if err != nil {
				return err
			}
			if versionExists {
				if _, err := tx.Exec(`ALTER TABLE soldiers DROP COLUMN created_by_version`); err != nil {
					return err
				}
			}
			pathExists, err := columnExists(tx, "soldiers", "created_by_import_path")
			if err != nil {
				return err
			}
			if pathExists {
				if _, err := tx.Exec(`ALTER TABLE soldiers DROP COLUMN created_by_import_path`); err != nil {
					return err
				}
			}
			return nil
		},
	},
}

// reverseAddColumnLoop is the inverse of Block 2 — it drops every
// column the forward loop added, guarded by columnExists so the
// inverse is safe to re-run on a partially-downgraded DB. The
// column list matches the forward loop exactly; order does not
// matter (DROP COLUMN is independent per column).
//
// SQLite >=3.35 supports DROP COLUMN for unindexed, non-FK columns.
// The FK-constrained columns in this list (entry_type,
// spouse_soldier_id, import_batch_id) require a table rebuild;
// the runner reports the SQLite error verbatim if DROP COLUMN
// fails on those — the operator can either skip Block 2's inverse
// or accept the failure. SQLite <3.35 will reject every DROP
// COLUMN; the applySchema call sites enforce a minimum SQLite
// version elsewhere (per internal/db/db.go:30-36).
func reverseAddColumnLoop(tx *sql.Tx) error {
	columns := []struct {
		table  string
		column string
	}{
		{"soldiers", "buried_in"},
		{"soldiers", "pension_id"},
		{"soldiers", "application_id"},
		{"soldiers", "prefix"},
		{"soldiers", "show_prefix_before_name"},
		{"soldiers", "middle_name"},
		{"soldiers", "suffix"},
		{"soldiers", "rank_in"},
		{"soldiers", "rank_out"},
		{"soldiers", "pension_state"},
		{"soldiers", "confederate_home_status"},
		{"soldiers", "confederate_home_name"},
		{"soldiers", "sync_id"},
		{"soldiers", "entry_type"},
		{"soldiers", "spouse_soldier_id"},
		{"soldiers", "relationship_label"},
		{"soldiers", "maiden_name"},
		{"soldiers", "birth_date"},
		{"soldiers", "death_date"},
		{"soldiers", "biography"},
		{"soldiers", "pdf_excerpt_override"},
		{"soldiers", "needs_review"},
		{"soldiers", "review_reason"},
		{"soldiers", "added_by"},
		{"soldiers", "last_edited_by"},
		{"soldiers", "last_edited_fields"},
		{"soldiers", "last_edited_at"},
		{"soldiers", "updated_at"},
		{"soldiers", "import_batch_id"},
		{"records", "sync_id"},
		{"records", "soldier_sync_id"},
		{"images", "sync_id"},
		{"images", "soldier_sync_id"},
		{"images", "is_primary"},
	}
	for _, c := range columns {
		exists, err := columnExists(tx, c.table, c.column)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		if _, err := tx.Exec(`ALTER TABLE ` + c.table + ` DROP COLUMN ` + c.column); err != nil {
			return err
		}
	}
	return nil
}

// applyAddColumnLoop runs the 31-entry ALTER TABLE ADD COLUMN loop.
// Kept as a package-private helper so the Migration.Up closures stay
// readable. The loop is guarded by columnExists so each statement runs
// at most once per archive lifetime.
func applyAddColumnLoop(tx *sql.Tx) error {
	for _, migration := range []struct {
		table     string
		column    string
		sql       string
		alsoCheck string
	}{
		{table: "soldiers", column: "buried_in", sql: `ALTER TABLE soldiers ADD COLUMN buried_in TEXT`},
		{table: "soldiers", column: "pension_id", sql: `ALTER TABLE soldiers ADD COLUMN pension_id TEXT`},
		{table: "soldiers", column: "application_id", sql: `ALTER TABLE soldiers ADD COLUMN application_id TEXT`},
		{table: "soldiers", column: "prefix", sql: `ALTER TABLE soldiers ADD COLUMN prefix TEXT`},
		{table: "soldiers", column: "show_prefix_before_name", sql: `ALTER TABLE soldiers ADD COLUMN show_prefix_before_name BOOLEAN DEFAULT 0`},
		{table: "soldiers", column: "middle_name", sql: `ALTER TABLE soldiers ADD COLUMN middle_name TEXT`},
		{table: "soldiers", column: "suffix", sql: `ALTER TABLE soldiers ADD COLUMN suffix TEXT`},
		{table: "soldiers", column: "rank_in", sql: `ALTER TABLE soldiers ADD COLUMN rank_in TEXT`},
		{table: "soldiers", column: "rank_out", sql: `ALTER TABLE soldiers ADD COLUMN rank_out TEXT`},
		{table: "soldiers", column: "pension_state", sql: `ALTER TABLE soldiers ADD COLUMN pension_state TEXT`},
		{table: "soldiers", column: "confederate_home_status", sql: `ALTER TABLE soldiers ADD COLUMN confederate_home_status TEXT DEFAULT 'N/A'`},
		{table: "soldiers", column: "confederate_home_name", sql: `ALTER TABLE soldiers ADD COLUMN confederate_home_name TEXT`},
		{table: "soldiers", column: "sync_id", sql: `ALTER TABLE soldiers ADD COLUMN sync_id TEXT`},
		{table: "soldiers", column: "entry_type", sql: `ALTER TABLE soldiers ADD COLUMN entry_type TEXT NOT NULL DEFAULT 'soldier'`},
		{table: "soldiers", column: "spouse_soldier_id", sql: `ALTER TABLE soldiers ADD COLUMN spouse_soldier_id INTEGER REFERENCES soldiers(id) ON DELETE SET NULL`},
		{table: "soldiers", column: "relationship_label", sql: `ALTER TABLE soldiers ADD COLUMN relationship_label TEXT`},
		{table: "soldiers", column: "maiden_name", sql: `ALTER TABLE soldiers ADD COLUMN maiden_name TEXT`},
		{table: "soldiers", column: "birth_date", sql: `ALTER TABLE soldiers ADD COLUMN birth_date TEXT`},
		{table: "soldiers", column: "death_date", sql: `ALTER TABLE soldiers ADD COLUMN death_date TEXT`},
		{table: "soldiers", column: "biography", sql: `ALTER TABLE soldiers ADD COLUMN biography TEXT`},
		{table: "soldiers", column: "pdf_excerpt_override", sql: `ALTER TABLE soldiers ADD COLUMN pdf_excerpt_override TEXT`},
		{table: "soldiers", column: "needs_review", sql: `ALTER TABLE soldiers ADD COLUMN needs_review BOOLEAN DEFAULT 0`},
		{table: "soldiers", column: "review_reason", sql: `ALTER TABLE soldiers ADD COLUMN review_reason TEXT`},
		{table: "soldiers", column: "added_by", sql: `ALTER TABLE soldiers ADD COLUMN added_by TEXT`},
		{table: "soldiers", column: "last_edited_by", sql: `ALTER TABLE soldiers ADD COLUMN last_edited_by TEXT`},
		{table: "soldiers", column: "last_edited_fields", sql: `ALTER TABLE soldiers ADD COLUMN last_edited_fields TEXT`},
		{table: "soldiers", column: "last_edited_at", sql: `ALTER TABLE soldiers ADD COLUMN last_edited_at DATETIME`},
		{table: "soldiers", column: "updated_at", sql: `ALTER TABLE soldiers ADD COLUMN updated_at DATETIME`},
		{table: "soldiers", column: "import_batch_id", sql: `ALTER TABLE soldiers ADD COLUMN import_batch_id TEXT REFERENCES import_batches(id) ON DELETE SET NULL`},
		{table: "records", column: "sync_id", sql: `ALTER TABLE records ADD COLUMN sync_id TEXT`},
		// Fresh installs get person_sync_id inline (block-1). Block-60
		// renames soldier_sync_id → person_sync_id on v59→v60 upgrades
		// BEFORE block-2 runs. On legacy v1-v58 upgrades neither column
		// exists yet, so block-2 adds soldier_sync_id and block-60
		// renames it later. The dual-check below skips the ADD on
		// fresh installs (where the new column already exists) AND
		// on v59→v60 upgrades (where block-60 has already renamed).
		{table: "records", column: "soldier_sync_id", sql: `ALTER TABLE records ADD COLUMN soldier_sync_id TEXT`, alsoCheck: "person_sync_id"},
		{table: "images", column: "sync_id", sql: `ALTER TABLE images ADD COLUMN sync_id TEXT`},
		{table: "images", column: "soldier_sync_id", sql: `ALTER TABLE images ADD COLUMN soldier_sync_id TEXT`, alsoCheck: "person_sync_id"},
		{table: "images", column: "is_primary", sql: `ALTER TABLE images ADD COLUMN is_primary BOOLEAN DEFAULT 0`},
		// v60 (issue #320): Event Record subtype columns. Added via
		// the applyAddColumnLoop so the v1 → v60 upgrade path picks
		// them up. Fresh installs get them inline in the const schema.
		{table: "soldiers", column: "kind", sql: `ALTER TABLE soldiers ADD COLUMN kind TEXT`},
		{table: "soldiers", column: "begin_date", sql: `ALTER TABLE soldiers ADD COLUMN begin_date TEXT`},
		{table: "soldiers", column: "end_date", sql: `ALTER TABLE soldiers ADD COLUMN end_date TEXT`},
		{table: "soldiers", column: "description", sql: `ALTER TABLE soldiers ADD COLUMN description TEXT`},
	} {
		exists, err := columnExists(tx, migration.table, migration.column)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if migration.alsoCheck != "" {
			newNameExists, err := columnExists(tx, migration.table, migration.alsoCheck)
			if err != nil {
				return err
			}
			if newNameExists {
				continue
			}
		}
		if _, err := tx.Exec(migration.sql); err != nil {
			return err
		}
	}
	return nil
}

// applySoldiersNormalization runs Block 6 — the seven inline UPDATE
// statements on soldiers that normalize NULL/empty fields and rewrite
// placeholder strings to canonical forms. See Block 6's catalogue entry
// in docs/migrations/reversibility.md for the per-UPDATE reversibility
// breakdown.
func applySoldiersNormalization(tx *sql.Tx) error {
	statements := []string{
		`UPDATE soldiers SET confederate_home_status = 'N/A' WHERE LOWER(TRIM(COALESCE(confederate_home_status, ''))) IN ('', 'none', 'na', 'n/a', 'not recorded')`,
		`UPDATE soldiers SET pension_state = 'N/A' WHERE LOWER(TRIM(COALESCE(pension_state, ''))) IN ('', 'none', 'na', 'n/a', 'not recorded')`,
		`UPDATE soldiers SET needs_review = 0 WHERE needs_review IS NULL`,
		`UPDATE soldiers SET review_reason = '' WHERE review_reason IS NULL`,
		`UPDATE soldiers SET show_prefix_before_name = 0 WHERE show_prefix_before_name IS NULL`,
		`UPDATE soldiers SET confederate_home_name = '' WHERE confederate_home_name IS NULL`,
		`UPDATE soldiers SET confederate_home_name = '' WHERE confederate_home_status = 'N/A'`,
	}
	for _, stmt := range statements {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

// reverseSoldiersNormalization is the best-effort inverse of Block
// 6. Per the catalogue, the four NULL-coalesce UPDATEs are
// reversible in isolation; the two placeholder-string rewrites
// (confederate_home_status, pension_state) and the line 457-459
// confederate_home_name overwrite are NOT reversible. We apply
// the four reversible inverses here and skip the rest — the
// runner's "what was lost" manifest lists the skipped UPDATEs
// so the operator can re-apply them post-down if they want.
func reverseSoldiersNormalization(tx *sql.Tx) error {
	// Inverse of: needs_review = 0 WHERE needs_review IS NULL
	// (only flip back rows that are still 0; rows legitimately 0
	// from user input stay 0 — the over-correction is the same
	// shape as the forward pass had, and the operator can
	// distinguish by checking last_edited_at post-down).
	if _, err := tx.Exec(`UPDATE soldiers SET needs_review = NULL WHERE needs_review = 0`); err != nil {
		return err
	}
	// Inverse of: review_reason = '' WHERE review_reason IS NULL
	if _, err := tx.Exec(`UPDATE soldiers SET review_reason = NULL WHERE review_reason = ''`); err != nil {
		return err
	}
	// Inverse of: show_prefix_before_name = 0 WHERE ... IS NULL
	if _, err := tx.Exec(`UPDATE soldiers SET show_prefix_before_name = NULL WHERE show_prefix_before_name = 0`); err != nil {
		return err
	}
	// Inverse of: confederate_home_name = '' WHERE ... IS NULL
	// (NOT the line 457-459 overwrite — that one is irreversible)
	if _, err := tx.Exec(`UPDATE soldiers SET confederate_home_name = NULL WHERE TRIM(confederate_home_name) = ''`); err != nil {
		return err
	}
	// The two placeholder-string rewrites (confederate_home_status
	// 'none' -> 'N/A', pension_state 'none' -> 'N/A') are NOT
	// reversed — the original pre-state is lost, the catalog
	// classifies these as Irreversible. The "what was lost"
	// manifest emitted by the CLI runner calls them out.
	return nil
}

// applyImagesIsPrimary runs Block 8 — the two UPDATE statements on
// images: NULL-coalesce + MIN(id) primary election. Groups by the
// FK column the table currently has (soldier_id on v54-or-earlier,
// person_record_id on v60+). Idempotent on fresh installs.
func applyImagesIsPrimary(tx *sql.Tx) error {
	if _, err := tx.Exec(`UPDATE images SET is_primary = 0 WHERE is_primary IS NULL`); err != nil {
		return err
	}
	groupCol := "person_record_id"
	if exists, err := columnExists(tx, "images", "soldier_id"); err != nil {
		return err
	} else if exists {
		groupCol = "soldier_id"
	}
	stmt := fmt.Sprintf(`UPDATE images SET is_primary = 1 WHERE id IN (
		SELECT MIN(id)
		FROM images
		GROUP BY %s
		HAVING MAX(CASE WHEN is_primary = 1 THEN 1 ELSE 0 END) = 0
	)`, groupCol)
	if _, err := tx.Exec(stmt); err != nil {
		return err
	}
	return nil
}

// Migrations returns the ordered slice of forward migration blocks
// that applySchema executes. Exported so the future `dixiedata debug
// schema-reversibility` audit harness can print the catalogue and
// the future `migrate down <target>` runner can iterate in reverse.
//
// Callers MUST NOT mutate the returned slice.
func Migrations() []Migration {
	return migrations
}

// refuseDown is the shared Down function for all Irreversible
// blocks. It returns ErrMigrationIrreversible so the applyDownSchema
// runner can surface a precise refusal with the blocking block ID.
// The CLI unwraps this via errors.Is and prints the block's Reason
// in the "what was lost" manifest.
func refuseDown(tx *sql.Tx) error {
	return ErrMigrationIrreversible
}

// reverseSchemaBaseline is the inverse of Block 1. It drops every
// table + index that the inline `schema` constant creates, in the
// reverse of declaration order so that foreign-key children are
// dropped before their parents. The archive_meta seed rows are
// deleted first so that archive_meta itself can be dropped last
// (after every other table that might reference it has been
// dropped). Each DROP uses IF EXISTS so the inverse is safe to
// re-run on a partially-downgraded DB.
//
// IMPORTANT: this function drops EVERY table in the schema constant.
// It is the caller's responsibility (applyDownSchema) to ensure the
// operator has supplied a restore-point ID when crossing the v58 /
// v59 boundaries — those tables are required by live read code in
// the running binary, and the runner must refuse to drop them
// while the binary is still serving.
func reverseSchemaBaseline(tx *sql.Tx) error {
	// archive_meta seed first (so the table can be dropped later).
	if _, err := tx.Exec(`DELETE FROM archive_meta WHERE archive_kind IN ('shared_archive', 'backup_archive', 'static_archive')`); err != nil {
		return err
	}
	// Tables in reverse of declaration order (FK children first).
	// The schema constant declares (schema.go:19-340):
	//   1. schema_version, 2. soldiers, 3. records, 4. images,
	//   5. merge_review_sessions, 6. import_batches,
	//   7. merge_review_conflicts, 8. shared_merge_aliases,
	//   9. duplicate_audit_findings, 10. research_tasks,
	//   11. export_templates, 12. share_queue_presets,
	//   13. research_collections, 14. research_collection_items,
	//   15. calendar_items, 16. tags, 17. person_record_tags,
	//   18. archive_meta.
	// Reverse order = archive_meta first, then person_record_tags
	// (FK -> tags), then tags, etc. The exact FK dependency graph
	// is documented at schema.go:121 (shared_merge_aliases ->
	// soldiers) and schema.go:240 (person_record_tags -> tags).
	dropOrder := []string{
		"archive_meta",
		"person_record_tags",
		"tags",
		"calendar_items",
		"research_collection_items",
		"research_collections",
		"share_queue_presets",
		"export_templates",
		"research_tasks",
		"duplicate_audit_findings",
		"shared_merge_aliases",
		"merge_review_conflicts",
		"import_batches",
		"merge_review_sessions",
		"images",
		"records",
		"soldiers",
		"schema_version",
	}
	for _, table := range dropOrder {
		if _, err := tx.Exec(`DROP TABLE IF EXISTS ` + table); err != nil {
			return err
		}
	}
	return nil
}