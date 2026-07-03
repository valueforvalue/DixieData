package db

import (
	"database/sql"
	"errors"
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
//   Block 1  - Inline `tx.Exec(schema)` constant (CREATE TABLE +
//              CREATE INDEX + archive_meta seed)
//   Block 2  - ALTER TABLE ADD COLUMN loop (29 + 2 + 3 entries)
//   Block 3  - is_generated flip for DXD-NNNNN rows
//   Block 4  - phase1DistributedMergeMigration (system_config +
//              sync_id generation + FK backfills)
//   Block 5  - phase2CanonicalDatesMigration (entry_type default,
//              death_date printf, birth_date sentinel collapse)
//   Block 6  - confederate_home_status / pension_state / needs_review
//              / review_reason / show_prefix_before_name /
//              confederate_home_name normalization chain
//   Block 7  - last_edited_at backfill
//   Block 8  - images.is_primary NULL-coalesce + MIN(id) primary
//              election
//   Block 9  - idx_soldiers_spouse index
//   Block 10 - idx_soldiers_import_batch index
//   Block 11 - migrateNodePrefixConfiguration
//   Block 12 - migrateSanitizedDisplayIDs (HIGH RISK Irreversible)
//   Block 13 - migrateCanonicalDateData (HIGH RISK Irreversible)
//   Block 14 - ensureSoldierFTS (idempotent FTS5 rebuild + orphan
//              DELETE; FTS rebuild itself is Reversible, the orphan
//              DELETE is PartiallyReversible — we classify the block
//              as PartiallyReversible per the conservative rule)
//   Block 15 - ensureArchiveMetaSeed (v58, issue #183)
//   Block 16 - migrateEntryTypeDiscipline (v55, issue #106; log
//              table only — the CHECK constraint the v55.md doc
//              claims was added was never actually added at the SQL
//              level, see the catalogue for the doc-vs-code mismatch)
//   Block 17 - research_log.evidence_type rename (v55, issue #106)
//
// Block 18 (the terminal `PRAGMA user_version` write) is NOT in the
// slice — it's bookkeeping applied by applySchema after the slice
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
	// Block 2 - ALTER TABLE ADD COLUMN loop. Each entry is Reversible
	// individually (DROP COLUMN). The loop as a whole is Reversible.
	// SQLite >=3.35 supports DROP COLUMN for unindexed, non-FK columns;
	// the FK-constrained columns (entry_type, spouse_soldier_id,
	// import_batch_id) require the table-rebuild dance on DOWN, which
	// is the future DOWN runner's problem to solve.
	{
		ID:            "block-2-add-column-loop",
		Reversibility: Reversible,
		Reason: "29 ADD COLUMN on soldiers + 2 on records + 3 on images, all guarded by columnExists. Inverse: ALTER TABLE DROP COLUMN per entry.",
		Up: func(tx *sql.Tx) error {
			return applyAddColumnLoop(tx)
		},
		Down: func(tx *sql.Tx) error {
			return reverseAddColumnLoop(tx)
		},
	},
	// Block 3 - is_generated flip for legacy DXD-NNNNN rows. The inverse
	// is mechanical but over-corrects (demotes user-created DXD-NNNNN
	// rows). Classified PartiallyReversible per the catalogue.
	{
		ID:            "block-3-is-generated-flip",
		Reversibility: PartiallyReversible,
		Reason: "UPDATE flips is_generated=1 for DXD-NNNNN rows; no 'system-issued' marker. Inverse over-corrects user-created rows.",
		Up: func(tx *sql.Tx) error {
			_, err := tx.Exec(`UPDATE soldiers SET is_generated = 1 WHERE is_generated = 0 AND display_id GLOB 'DXD-[0-9][0-9][0-9][0-9][0-9]'`)
			return err
		},
		Down: func(tx *sql.Tx) error {
			// Best-effort inverse: set is_generated=0 for every
			// DXD-NNNNN row currently flagged is_generated=1.
			// Over-corrects user-created rows that legitimately
			// had is_generated=1, but the alternative (leaving
			// is_generated=1 on rows that the migration flipped)
			// is the wrong default — the operator can re-flip
			// any user-created rows post-down via SQL.
			_, err := tx.Exec(`UPDATE soldiers SET is_generated = 0 WHERE is_generated = 1 AND display_id GLOB 'DXD-[0-9][0-9][0-9][0-9][0-9]'`)
			return err
		},
	},
	// Block 4 - phase1DistributedMergeMigration. Generates random
	// sync_ids via syncIDSQL (non-deterministic randomblob). Reversible
	// only for never-shared single-node archives; Irreversible for any
	// archive that has produced a .ddsa export (the receiving side's
	// shared_merge_aliases would orphan). Per issue #273 decision Q1,
	// the DOWN runner refuses past Block 4 unconditionally.
	{
		ID:            "block-4-phase1-distributed-merge",
		Reversibility: Irreversible,
		Reason: "Generates non-deterministic sync_ids via randomblob; .ddsa exports carry those IDs as cross-node conflict-resolution keys. DOWN past this block is refused unconditionally.",
		Up: func(tx *sql.Tx) error {
			_, err := tx.Exec(phase1DistributedMergeMigration)
			return err
		},
		Down: refuseDown,
	},
	// Block 5 - phase2CanonicalDatesMigration. Sentinel collapse +
	// printf stringification of partial dates. Pre-state is discarded.
	{
		ID:            "block-5-phase2-canonical-dates",
		Reversibility: Irreversible,
		Reason: "Collapses '00/00/0000'/NULL/empty into indistinguishable post-states; death_date printf is lossy for partial dates.",
		Up: func(tx *sql.Tx) error {
			_, err := tx.Exec(phase2CanonicalDatesMigration)
			return err
		},
		Down: refuseDown,
	},
	// Block 6 - Inline UPDATE normalization chain on soldiers. Mixed:
	// the NULL-coalesce cases (needs_review, review_reason,
	// show_prefix_before_name, confederate_home_name) are Reversible in
	// isolation; the placeholder-string rewrites (confederate_home_status
	// 'none' -> 'N/A', pension_state 'none' -> 'N/A') and the
	// confederate_home_name='' WHERE status='N/A' overwrite are
	// Irreversible. The catalogue classifies the block as
	// PartiallyReversible (conservative — the reversibility class is
	// not uniform across the seven UPDATEs).
	{
		ID:            "block-6-soldiers-normalization",
		Reversibility: PartiallyReversible,
		Reason: "Seven UPDATEs: NULL-coalesce is reversible; placeholder rewrites ('none' -> 'N/A') and line 457-459 overwrite are irreversible.",
		Up: func(tx *sql.Tx) error {
			return applySoldiersNormalization(tx)
		},
		Down: func(tx *sql.Tx) error {
			return reverseSoldiersNormalization(tx)
		},
	},
	// Block 7 - last_edited_at backfill. Mechanical inverse but no
	// per-row NULL marker; over-corrects.
	{
		ID:            "block-7-last-edited-at-backfill",
		Reversibility: PartiallyReversible,
		Reason: "COALESCE backfill from updated_at/created_at/CURRENT_TIMESTAMP; no per-row NULL marker; inverse over-corrects.",
		Up: func(tx *sql.Tx) error {
			_, err := tx.Exec(`UPDATE soldiers SET last_edited_at = COALESCE(NULLIF(updated_at, ''), NULLIF(created_at, ''), CURRENT_TIMESTAMP) WHERE last_edited_at IS NULL OR TRIM(last_edited_at) = ''`)
			return err
		},
		Down: func(tx *sql.Tx) error {
			// Best-effort: set last_edited_at to '' for every
			// row where the forward path set it via COALESCE.
			// Cannot distinguish which fallback fired
			// (updated_at / created_at / CURRENT_TIMESTAMP)
			// per-row, so we use '' as the most-conservative
			// inverse (the original was NULL/empty).
			_, err := tx.Exec(`UPDATE soldiers SET last_edited_at = '' WHERE last_edited_at IS NOT NULL AND TRIM(last_edited_at) <> ''`)
			return err
		},
	},
	// Block 8 - images.is_primary NULL-coalesce + MIN(id) primary
	// election. The NULL-coalesce is reversible; the MIN(id) election
	// is irreversible (pre-state 'no image was primary for this
	// soldier' is lost). Catalogue: PartiallyReversible.
	{
		ID:            "block-8-images-is-primary",
		Reversibility: PartiallyReversible,
		Reason: "NULL-coalesce is reversible; MIN(id) primary election is irreversible (no marker for 'no image was primary pre-migration').",
		Up: func(tx *sql.Tx) error {
			return applyImagesIsPrimary(tx)
		},
		Down: func(tx *sql.Tx) error {
			// Inverse of the NULL-coalesce only. The MIN(id)
			// election is irreversible — those images stay
			// is_primary=1 unless the operator manually resets
			// them post-down.
			_, err := tx.Exec(`UPDATE images SET is_primary = NULL WHERE is_primary = 0`)
			return err
		},
	},
	// Block 9 - idx_soldiers_spouse index. Pure additive.
	{
		ID:            "block-9-idx-soldiers-spouse",
		Reversibility: Reversible,
		Reason: "CREATE INDEX IF NOT EXISTS; inverse is DROP INDEX IF EXISTS.",
		Up: func(tx *sql.Tx) error {
			_, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_soldiers_spouse ON soldiers(spouse_soldier_id)`)
			return err
		},
		Down: func(tx *sql.Tx) error {
			_, err := tx.Exec(`DROP INDEX IF EXISTS idx_soldiers_spouse`)
			return err
		},
	},
	// Block 10 - idx_soldiers_import_batch index. Pure additive.
	{
		ID:            "block-10-idx-soldiers-import-batch",
		Reversibility: Reversible,
		Reason: "CREATE INDEX IF NOT EXISTS; inverse is DROP INDEX IF EXISTS.",
		Up: func(tx *sql.Tx) error {
			_, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_soldiers_import_batch ON soldiers(import_batch_id, created_at DESC)`)
			return err
		},
		Down: func(tx *sql.Tx) error {
			_, err := tx.Exec(`DROP INDEX IF EXISTS idx_soldiers_import_batch`)
			return err
		},
	},
	// Block 11 - migrateNodePrefixConfiguration. Fires only when
	// user_identity_complete='1'; overwrites any pre-existing
	// node_prefix via ON CONFLICT DO UPDATE. Cannot distinguish
	// pre-existing default 'DXD' from a user-derived value.
	{
		ID:            "block-11-node-prefix-configuration",
		Reversibility: PartiallyReversible,
		Reason: "ON CONFLICT DO UPDATE overwrites any pre-existing node_prefix; cannot distinguish default 'DXD' from a user-derived value post-fact.",
		Up: func(tx *sql.Tx) error {
			return migrateNodePrefixConfiguration(tx)
		},
		// No Down: the forward path's overwrite is the data-loss
		// event; reversing would either re-overwrite (no-op) or
		// require a side table the schema doesn't carry. The
		// runner's "what was lost" manifest calls this out.
		Down: func(tx *sql.Tx) error {
			// Best-effort no-op: do not touch system_config.
			// The operator can manually reset node_prefix
			// post-down if they have a snapshot to copy from.
			return nil
		},
	},
	// Block 12 - migrateSanitizedDisplayIDs. HIGH RISK Irreversible.
	// No display_id_history, no audit log, in-memory slice only.
	// SanitizeID is non-bijective for zero-padded 5-digit sequences;
	// collision resolution mints a fresh sequence with no relationship
	// to the original.
	{
		ID:            "block-12-sanitized-display-ids",
		Reversibility: Irreversible,
		Reason: "Lossy display_id overwrite with no side-table backup. Collision resolution mints fresh sequences with no original relationship. DOWN requires --force-irreversible + restore-point acknowledgement.",
		Up: func(tx *sql.Tx) error {
			return migrateSanitizedDisplayIDs(tx)
		},
		Down: refuseDown,
	},
	// Block 13 - migrateCanonicalDateData. HIGH RISK Irreversible.
	// Consumes birth_info narrative without re-writing; NormalizeCanonical
	// is non-bijective (zero-padded MM/DD/YYYY, two-digit years rejected).
	{
		ID:            "block-13-canonical-date-data",
		Reversibility: Irreversible,
		Reason: "Lossy normalization of birth/death dates with no side-table backup. birth_info narrative text consumed but not re-written. NormalizeCanonical is non-bijective. DOWN requires --force-irreversible + restore-point acknowledgement.",
		Up: func(tx *sql.Tx) error {
			return migrateCanonicalDateData(tx)
		},
		Down: refuseDown,
	},
	// Block 14 - ensureSoldierFTS. The FTS rebuild itself is
	// Reversible (DROP + CREATE VIRTUAL TABLE + INSERT...SELECT is an
	// idempotent cycle whose net effect is zero data loss as long as
	// soldiers and scratchpad_cache are intact). The orphan DELETE on
	// schema.go:588 is PartiallyReversible. Catalogue classifies the
	// block as PartiallyReversible (conservative).
	{
		ID:            "block-14-ensure-soldier-fts",
		Reversibility: PartiallyReversible,
		Reason: "FTS rebuild is idempotent (Reversible); orphan DELETE FROM scratchpad_cache (line 588) is PartiallyReversible — orphan scratch_pad text is lost.",
		Up: func(tx *sql.Tx) error {
			return ensureSoldierFTS(tx)
		},
		Down: func(tx *sql.Tx) error {
			// Inverse is the same idempotent cycle: DROP +
			// CREATE + INSERT...SELECT. No data is lost because
			// FTS5 is a derived index — the source-of-truth
			// (soldiers + scratchpad_cache) is unchanged.
			// The orphan scratch_pad rows the forward path
			// deleted (line 588) cannot be recovered — they're
			// permanently lost. Documented in the manifest.
			return ensureSoldierFTS(tx)
		},
	},
	// Block 15 - ensureArchiveMetaSeed. Three INSERT OR IGNORE seed
	// rows. Pure additive.
	{
		ID:            "block-15-archive-meta-seed",
		Reversibility: Reversible,
		Reason: "Three INSERT OR IGNORE INTO archive_meta seed rows (v58, issue #183). Inverse: DELETE FROM archive_meta WHERE archive_kind IN (...).",
		Up: func(tx *sql.Tx) error {
			return ensureArchiveMetaSeed(tx)
		},
		Down: func(tx *sql.Tx) error {
			_, err := tx.Exec(`DELETE FROM archive_meta WHERE archive_kind IN ('shared_archive', 'backup_archive', 'static_archive')`)
			return err
		},
	},
	// Block 16 - migrateEntryTypeDiscipline. v55 (issue #106).
	// Creates a log table + sentinel row. IMPORTANT: docs/migrations/v55.md
	// claims a CHECK constraint was added at v55, but the inline
	// CREATE TABLE soldiers at schema.go:25-65 declares entry_type TEXT
	// NOT NULL DEFAULT 'soldier' WITHOUT a CHECK constraint. The comment
	// at schema.go:824-836 explicitly acknowledges the doc-vs-code
	// mismatch ("Pragmatic approach for v55: rely on application-level
	// validation"). Block 16 is Reversible at the SQL level — only the
	// log table is created; nothing in soldiers.entry_type is actually
	// constrained.
	{
		ID:            "block-16-entry-type-discipline",
		Reversibility: Reversible,
		Reason: "Creates soldiers_entry_type_check_log table + sentinel row. The CHECK constraint v55.md claims was added was never actually added at the SQL level (doc-vs-code mismatch); block is Reversible.",
		Up: func(tx *sql.Tx) error {
			return migrateEntryTypeDiscipline(tx)
		},
		Down: func(tx *sql.Tx) error {
			_, err := tx.Exec(`DROP TABLE IF EXISTS soldiers_entry_type_check_log`)
			return err
		},
	},
	// Block 17 - research_log.evidence_type rename ('archive' ->
	// 'local_archive'). v55 (issue #106). Pre-rename value cannot
	// be distinguished from a row that always had 'local_archive'.
	// Tolerates missing table via isNoSuchTableError fallback.
	{
		ID:            "block-17-research-log-evidence-rename",
		Reversibility: Irreversible,
		Reason: "One-way value rename; pre-rename 'archive' cannot be distinguished from a row that always had 'local_archive'. Tolerates missing research_log table.",
		Up: func(tx *sql.Tx) error {
			_, err := tx.Exec(`UPDATE research_log SET evidence_type = 'local_archive' WHERE evidence_type = 'archive'`)
			if err != nil && isNoSuchTableError(err) {
				return nil
			}
			return err
		},
		Down: refuseDown,
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
		table  string
		column string
		sql    string
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
		{table: "records", column: "soldier_sync_id", sql: `ALTER TABLE records ADD COLUMN soldier_sync_id TEXT`},
		{table: "images", column: "sync_id", sql: `ALTER TABLE images ADD COLUMN sync_id TEXT`},
		{table: "images", column: "soldier_sync_id", sql: `ALTER TABLE images ADD COLUMN soldier_sync_id TEXT`},
		{table: "images", column: "is_primary", sql: `ALTER TABLE images ADD COLUMN is_primary BOOLEAN DEFAULT 0`},
	} {
		exists, err := columnExists(tx, migration.table, migration.column)
		if err != nil {
			return err
		}
		if exists {
			continue
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
// images: NULL-coalesce + MIN(id) primary election.
func applyImagesIsPrimary(tx *sql.Tx) error {
	if _, err := tx.Exec(`UPDATE images SET is_primary = 0 WHERE is_primary IS NULL`); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE images SET is_primary = 1 WHERE id IN (
		SELECT MIN(id)
		FROM images
		GROUP BY soldier_id
		HAVING MAX(CASE WHEN is_primary = 1 THEN 1 ELSE 0 END) = 0
	)`); err != nil {
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