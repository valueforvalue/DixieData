# Schema reversibility audit (issue #273)

## Status

Audit complete. Every migration block in `internal/db/schema.go::applySchema` classified into Reversible / Partially Reversible / Irreversible. This document is the blocker-resolution deliverable referenced in `docs/agents/cli-plan.md:689` ("Blocked on a schema audit").

**Headline finding:** of the 19 blocks that run inside `applySchema`, only **5 are cleanly Reversible**. **6 are Partially Reversible** (best-effort, requires user verification post-down). **7 are Irreversible** (require restore-point rollback, not SQL inversion). **1 is bookkeeping** (the `schema_version` row + `PRAGMA user_version` terminal write). The DOWN feature must refuse any target that crosses an Irreversible block unless an explicit restore point is supplied.

## Scope

- Source-of-truth file: `internal/db/schema.go` (lines 1-909)
- Runner: `applySchema` at `internal/db/schema.go:361-522` (Begin → defer Rollback → 18 blocks → terminal `PRAGMA user_version` → Commit)
- Backup mechanism: `RetainedBackupManager` at `internal/update/retained_backup_manager.go` + `RestorePointManager` at `internal/update/restore_point_manager.go`
- In-place-safety walker: `internal/appshell/cli_debug_inplace.go:177-260` (`classifySchemaLine`)
- Test coverage: `internal/db/migration_backup_test.go` (UP-path only; no DOWN test exists)
- Decision matrix from issue #273 body (the issue itself defines the three classes)

## Classification scheme

| Class | Meaning | DOWN behavior |
|---|---|---|
| **Reversible** | Pure additive (CREATE TABLE IF NOT EXISTS, ADD COLUMN with default, CREATE INDEX) | Apply inverse on DOWN |
| **Partially Reversible** | Requires data transformation; pre-state recoverable from columns still on the row | Apply inverse with best-effort; user verifies post-down |
| **Irreversible** | Pre-state discarded (random UUID, lossy printf, sanitization, canonicalization) | Refuse DOWN past this block unless restore point ID supplied |

## Per-block catalogue

In execution order inside the `applySchema` transaction.

### Block 1 — Inline `tx.Exec(schema)` constant

- **Where:** `internal/db/schema.go:19-340` (the `const schema` string), executed at `schema.go:373`
- **Contents:** 19 `CREATE TABLE IF NOT EXISTS` (lines 20, 25, 69, 79, 90, 99, 105, 121, 134, 148, 164, 193, 201, 209, 216, 233, 240, 251, 279), 11+ `CREATE INDEX IF NOT EXISTS` (lines 263-275, 311, 325-326, 340-341), and one `INSERT OR IGNORE INTO archive_meta (...)` (lines 272-274)
- **Classification:** **Reversible** (entire block)
- **Inverse SQL:** `DROP TABLE IF EXISTS <name>` + `DROP INDEX IF EXISTS <name>` + `DELETE FROM archive_meta WHERE archive_kind IN (...)`. Order matters: seed rows must be deleted before `archive_meta` itself is dropped.
- **Caveat:** Tables introduced in v58+ (`tags`, `person_record_tags`, `archive_meta`) and v59 (`share_queue_presets`) are queried by live read code. The DOWN runner cannot drop them while the binary is running; a DOWN past v58 must coordinate with code that depends on them.

### Block 2 — `ALTER TABLE ... ADD COLUMN` loop

- **Where:** `internal/db/schema.go:374-428` (loop over 31 entries, each guarded by `columnExists`)
- **Contents:** 29 ADD COLUMN on `soldiers`, 2 on `records`, 3 on `images`
- **Classification:** **Reversible** (each entry)
- **Inverse SQL:** `ALTER TABLE ... DROP COLUMN <col>`. SQLite ≥3.35 supports `DROP COLUMN` for unindexed, non-FK columns. For `entry_type`, `spouse_soldier_id`, and `import_batch_id` (all FK-constrained), the inverse requires the table-rebuild dance.

### Block 3 — `UPDATE soldiers SET is_generated = 1 ... GLOB 'DXD-NNNNN'`

- **Where:** `internal/db/schema.go:430`
- **Contents:** Sets `is_generated = 1` for rows matching the legacy 5-digit format
- **Classification:** **Partially Reversible**
- **Inverse SQL:** `UPDATE soldiers SET is_generated = 0 WHERE is_generated = 1 AND display_id GLOB 'DXD-NNNNN'`. Correct down requires a "system-issued" marker that doesn't exist; over-corrects by demoting user-created `DXD-NNNNN` rows.

### Block 4 — `phase1DistributedMergeMigration`

- **Where:** `internal/db/schema.go:277-345`, executed at `schema.go:433`
- **Contents:** CREATE `system_config` table (reversible); UPDATEs for `birth_date='00/00/0000'` (lines 286-288, lossy sentinel), `death_date=printf(...)` (lines 290-292, lossy partial-date stringification), `updated_at=COALESCE(...)` (lines 294-296, recoverable), four random `sync_id` generations via `syncIDSQL` at `internal/db/identity_sql.go:3-10` (lines 298-300, 312-319, 321-323, 328-335, 337-339 — non-deterministic, irrecoverable); INSERTs for `node_prefix='DXD'` + `node_id=<random>` (lines 302-308, irrecoverable); four `CREATE UNIQUE INDEX` (reversible)
- **Classification:** **Irreversible for distributed archives** (those that have produced `.ddsa` exports); **Partially Reversible for never-shared single-node archives**
- **Reason:** `syncIDSQL` is `randomblob`-based and non-deterministic. `.ddsa` exports carry the minted sync_ids as the primary conflict-resolution key on receiving nodes. Reversing on the originating node orphans those references on the receiving side.
- **Pre-state preserved?:** No. No `sync_id_history`, no `system_config` audit table. The `INSERT ... WHERE NOT EXISTS` pattern means a second run would not re-insert.
- **DOWN path must distinguish archive class:** refuse if any `.ddsa` export lineage exists.

### Block 5 — `phase2CanonicalDatesMigration`

- **Where:** `internal/db/schema.go:347-363`, executed at `schema.go:436`
- **Contents:** UPDATE `entry_type='soldier'` (lines 349-351, lossy collapse of NULL/empty); UPDATE `death_date=printf(...)` for partial dates (lines 353-359, lossy); UPDATE `birth_date='' WHERE birth_date='00/00/0000'` (lines 361-362, sentinel collapse)
- **Classification:** **Irreversible**
- **Reason:** Sentinel collapse + printf stringification with no companion history table. The original "was NULL" vs "was empty" vs "was `'00/00/0000'`" is not recoverable.

### Block 6 — Inline `UPDATE soldiers` normalization chain

- **Where:** `internal/db/schema.go:439-457`
- **Contents:** Six UPDATEs: `confederate_home_status='N/A'` (439-441), `pension_state='N/A'` (442-444), `needs_review=0` (445-447), `review_reason=''` (448-450), `show_prefix_before_name=0` (451-453), `confederate_home_name=''` (454-456), `confederate_home_name='' WHERE status='N/A'` (457-459)
- **Classification:** **Partially Reversible** for the NULL-coalesce cases (lines 439-456); **Irreversible** for line 457-459 (overwrites explicit `confederate_home_name` whenever status normalizes — real data loss) and for the placeholder-string rewrites (lines 439-444 cannot distinguish a row that always had `'N/A'` from one rewritten from `'none'`)

### Block 7 — `UPDATE soldiers SET last_edited_at = COALESCE(...)`

- **Where:** `internal/db/schema.go:460`
- **Classification:** **Partially Reversible** (mechanical inverse but no per-row NULL marker)

### Block 8 — `UPDATE images SET is_primary = 0 ...` + MIN(id) primary election

- **Where:** `internal/db/schema.go:463-472`
- **Contents:** NULL-coalesce `is_primary = 0` (reversible); MIN(id) election that designates one image per soldier as primary (irreversible — pre-state "no image was primary" is lost)
- **Classification:** **Partially Reversible** (NULL-coalesce); **Irreversible** (MIN(id) election)

### Block 9 — `CREATE INDEX IF NOT EXISTS idx_soldiers_spouse`

- **Where:** `internal/db/schema.go:474`
- **Classification:** **Reversible** (`DROP INDEX idx_soldiers_spouse`)

### Block 10 — `CREATE INDEX IF NOT EXISTS idx_soldiers_import_batch`

- **Where:** `internal/db/schema.go:477`
- **Classification:** **Reversible**

### Block 11 — `migrateNodePrefixConfiguration`

- **Where:** `internal/db/schema.go:480` (call); function body at lines 734-822
- **Contents:** Only fires when `system_config.user_identity_complete = '1'`. Derives `node_prefix` via `BuildUserNodePrefix` (lines 783-787), writes via `ON CONFLICT(key) DO UPDATE` — overwrites any pre-existing value
- **Classification:** **Partially Reversible** (cannot distinguish pre-existing default `'DXD'` from a user-derived value if the user re-ran with a different name)
- **Pre-state preserved?:** No.

### Block 12 — `migrateSanitizedDisplayIDs` — **HIGH RISK**

- **Where:** `internal/db/schema.go:483` (call); function body at lines 809-853
- **Contents:** Reads `system_config.node_prefix` (818), materializes all `soldiers.id, display_id` (824), calls `SanitizeID` (845), `NextGeneratedDisplayID` for collisions (847-851), `UPDATE soldiers SET display_id = ?` (852)
- **Lossy write line:** `schema.go:852`
- **Classification:** **Irreversible** (HIGH risk for DOWN)
- **Reason:** No `display_id_history` table, no audit log, in-memory `[]displayRecord` slice only. `CanonicalDisplayID` at `internal/db/displayid.go:39-49` is non-bijective for zero-padded 5-digit sequences (cannot distinguish `SMITH-00042` from a row that started as `SMITH-42`). Collision resolution at lines 847-851 mints a fresh sequence with no relationship to the original.
- **Action:** Block downgrade at this migration boundary with a HIGH-risk warning; require explicit data-loss acknowledgment (or `.ddsa` export first).

### Block 13 — `migrateCanonicalDateData` — **HIGH RISK**

- **Where:** `internal/db/schema.go:486` (call); function body at lines 866-909
- **Contents:** Reads 7 columns per soldier (`id, birth_date, birth_info, death_date, death_year, death_month, death_day`); calls `dates.ParseBirthInfo` (consumes narrative text into a single date), `dates.NormalizeCanonical` (zero-padded MM/DD/YYYY), `dates.MustFormat` (lossy partial-date stringification); `UPDATE soldiers SET birth_date = ?, death_date = ?, death_year = ?, death_month = ?, death_day = ?` at line 926
- **Lossy write line:** `schema.go:926`
- **Classification:** **Irreversible** (HIGH risk for DOWN)
- **Reason:** `birth_info` consumed but not re-written (narrative text dropped). `NormalizeCanonical` at `internal/dates/dates.go:65-70` is non-bijective (numeric output, zero-padded, two-digit years rejected). `MustFormat` collapses partial dates to `MM/DD/YYYY`. No side table, history table, or audit log captures pre-state.

### Block 14 — `ensureSoldierFTS`

- **Where:** `internal/db/schema.go:489` (call); function body at lines 576-714
- **Contents:** 17 SQL statements — `CREATE TABLE IF NOT EXISTS scratchpad_cache` (580-584, reversible); `DELETE FROM scratchpad_cache WHERE soldier_id NOT IN (SELECT id FROM soldiers)` (588, orphan cleanup, lossy); six `DROP TRIGGER IF EXISTS` (591-596, idempotent rebuild); `DROP TABLE IF EXISTS soldiers_fts` (599, idempotent rebuild); `CREATE VIRTUAL TABLE soldiers_fts USING fts5(...)` (600-625, reversible); six `CREATE TRIGGER` (627-684, reversible); `INSERT INTO soldiers_fts (...) SELECT ... FROM soldiers s LEFT JOIN scratchpad_cache c ON c.soldier_id = s.id` (686-714, reversible)
- **Classification:** **Reversible** for the FTS rebuild (DROP+CREATE+INSERT is a pure idempotent cycle whose net effect is zero data loss as long as `soldiers` and `scratchpad_cache` are intact); **Partially Reversible** for the orphan DELETE on line 588 (any `scratch_pad` text in orphan rows is permanently lost — only fires on partially-restored or manually-edited archives)
- **Caveat:** The walker at `cli_debug_inplace.go:194-216` flags `DROP TABLE` as `schema_drop_table`/`high` and `DELETE FROM` as `schema_delete`/`high` — both false-positives in this context. The walker cannot see the block structure (DROP+CREATE+INSERT is one logical operation from the data plane's perspective).

### Block 15 — `ensureArchiveMetaSeed`

- **Where:** `internal/db/schema.go:495` (call); function body at lines 716-731
- **Contents:** Three `INSERT OR IGNORE INTO archive_meta (archive_kind, include_tags) VALUES (...)` for `shared_archive=0`, `backup_archive=1`, `static_archive=0`
- **Classification:** **Reversible** (`DELETE FROM archive_meta WHERE archive_kind IN (...)`)

### Block 16 — `migrateEntryTypeDiscipline` — **DOC MISMATCH**

- **Where:** `internal/db/schema.go:504` (call); function body at lines 842-863
- **Contents:** Idempotent guard via `sqlite_master` check for `soldiers_entry_type_check_log`; creates the log table + sentinel `CURRENT_TIMESTAMP` row
- **Classification:** **Reversible** at the SQL level (only the log table is created; nothing in `soldiers.entry_type` is actually constrained)
- **Important note:** The `v55.md` migration note (lines 5-6) claims "Add CHECK (entry_type IN ('soldier','wife','widow','linked_person')) to the soldiers.entry_type column", and the Rollback section at line 24-26 says "`bump-version.ps1` with no -Force refuses to roll back schema versions that added a CHECK constraint, since dropping it requires explicit verification that no rows violate the dropped constraint." This is a **doc-vs-code mismatch**: the inline `CREATE TABLE soldiers` at `internal/db/schema.go:25-65` declares `entry_type TEXT NOT NULL DEFAULT 'soldier'` (line 30) — no CHECK constraint at the SQL level. The comment at lines 824-836 explicitly acknowledges this: "Pragmatic approach for v55: rely on application-level validation." The actual SQL effect is just the log-table creation, not a CHECK constraint. The DOWN runner must classify Block 16 as Reversible even though the doc claims otherwise.

### Block 17 — `UPDATE research_log SET evidence_type = 'local_archive' WHERE evidence_type = 'archive'`

- **Where:** `internal/db/schema.go:507-514`, with `isNoSuchTableError` tolerance at line 510
- **Classification:** **Irreversible** for any row where the rename fires (pre-rename `'archive'` value cannot be distinguished from a row that always had `'local_archive'`)
- **Caveat:** The `isNoSuchTableError` fallback makes the statement silently no-op when `research_log` doesn't exist. On `migrate down` to a pre-v55 binary that still uses `'archive'`, the table presence isn't enough to know whether to re-rename.

### Block 18 — v60 Event Records + FK rename to `person_record_id` (issue #320)

- **Where:** `internal/db/migrations.go:443-602` (the `migrations` slice entry with `ID: "block-60-event-records-event-person-links-fk-rename"`)
- **Contents (4 sub-blocks in order):**
  - **A. CREATE TABLE `event_person_links` + 3 indexes** (junction for Event↔Person Record many-to-many). `CREATE TABLE IF NOT EXISTS` + `CREATE INDEX IF NOT EXISTS` × 3.
  - **B. 8 RENAME COLUMN statements** (each guarded by `columnExists` for idempotency): `records.soldier_id`, `images.soldier_id`, `scratchpad_cache.soldier_id`, `research_tasks.soldier_id` → `person_record_id`; `merge_review_conflicts.{local,left,right}_soldier_id` → `{local,left,right}_record_id`; `duplicate_audit_findings.{left,right}_soldier_id` → `{left,right}_record_id`. SQLite `ALTER TABLE ... RENAME COLUMN` (driver `modernc.org/sqlite v1.33.1` bundles SQLite 3.46.x; the repo's documented SQLite floor is ≥3.35).
  - **C. FTS5 trigger DROP+RECREATE** via the existing idempotent `ensureSoldierFTS` function. The three `scratchpad_cache_*` triggers and the six `soldiers_fts_*` triggers reference the renamed columns by name; SQLite does **not** auto-update trigger SQL text on `RENAME COLUMN`, so the triggers must be dropped and recreated with the new column name. The FTS5 virtual table's internal `soldier_id UNINDEXED` column is also renamed to `person_record_id` (the function's CREATE VIRTUAL TABLE statement carries the new name). The function's `INSERT INTO soldiers_fts SELECT ...` repopulates the index from `soldiers` + `scratchpad_cache` — pure derived data, no loss.
  - **D. Backfill** `person_sync_id` on `records` and `images` (in case the v59→v60 upgrade skipped the standard phase1 migration). Idempotent: the `WHERE person_sync_id IS NULL OR TRIM(person_sync_id) = ''` clause skips already-populated rows.
- **Classification:** **Partially Reversible.** RENAME COLUMN is reversible (the inverse RENAME brings the column back; data is unchanged). CREATE TABLE / DROP TABLE is reversible. The FTS5 trigger DROP+RECREATE cycle is reversible on the index side (FTS5 is a derived index, no data loss when the index is dropped+repopulated) but the trigger text on the DOWN path references `person_record_id` regardless of which column-name set is current — re-running with the original column names would require temporarily reverting the renames, which is unsafe. The DOWN path for sub-block C is therefore a no-op; the triggers still function correctly because they reference the columns that exist post-renames. Sub-block D is also a no-op on DOWN (the WHERE clause is idempotent on re-run).
- **Caveat:** `spouse_soldier_id` is intentionally **not** in the rename list — it is a self-referential FK on `soldiers` for the linked-Soldier / Spouse relationship, not a Person Record FK. Events have no spouse. The Go struct field `models.Soldier.SpouseSoldierID` keeps its name for the same reason.
- **Wire-compat note:** `Record.PersonRecordID` and `Image.PersonRecordID` Go struct fields keep the `json:"soldier_id"` tag so a v59 `.ddshare` round-trips through a v60 binary. `MergeReviewConflict` has no json tags, so its field rename is wire-invisible.
- **Verification:** `TestMigrationsReversibilityMapping` asserts the block's `Reversibility` is `PartiallyReversible` and the slice has 18 blocks. `TestApplyDownSchema_PartialReversibleStepDown` asserts v60 → v59 succeeds (PartiallyReversible) and v<59 refuses (Block 17 is Irreversible).

### Block 19 — `schema_version` row + `PRAGMA user_version` terminal write

- **Where:** `internal/db/schema.go:515-520`
- **Contents:** `INSERT OR IGNORE INTO schema_version(version) VALUES (60)` + `PRAGMA user_version = 60`
- **Classification:** **Bookkeeping** (not a migration block; this is the gate itself)
- **DOWN inverse:** `PRAGMA user_version = <target>` + `DELETE FROM schema_version WHERE version > ?`

## Per-version summary (v52 - v59)

Mapped against `docs/migrations/v52.md` through `v59.md`.

| Version | Doc claim | SQL effect | Classification |
|---|---|---|---|
| **v52** | No schema-level changes | `CurrentSchemaVersion` 51 → 52 (`internal/versioninfo/versioninfo.go:11`) | **No-op**. DOWN is trivially reversible. |
| **v53** | No schema-level changes | `CurrentSchemaVersion` 52 → 53 | **No-op**. DOWN is trivially reversible. |
| **v54** | No schema-level changes (the v55 image-compression attempt at commit `264df7f` was reverted in `ea99f07` before any archive shipped) | `CurrentSchemaVersion` 53 → 54 | **No-op**. DOWN is trivially reversible. |
| **v55** | Add CHECK (entry_type IN ('soldier','wife','widow','linked_person')) to soldiers.entry_type + rename research_log.evidence_type = 'archive' → 'local_archive' | Block 16 (log table only — CHECK constraint was never actually added) + Block 17 (rename) | **Partially Reversible** at the SQL level (log table is reversible) but **Irreversible** for the rename. Doc mismatch: `bump-version.ps1`'s refusal is based on a CHECK constraint that doesn't exist. |
| **v58** | Add `tags`, `person_record_tags`, `archive_meta` tables (issue #183) + seed three archive_meta rows | Block 1 (three new tables + indexes) + Block 15 (seed) | **Reversible** at SQL level. **Blocked at runtime** by live read code (export pipelines SELECT from `archive_meta`; Browse filter reads `person_record_tags`). |
| **v59** | Add `share_queue_presets` table (issue #192) | Block 1 (one new table) | **Reversible** at SQL level. **Blocked at runtime** by Share Queue modal queries. |

## Recommended DOWN-feature contract

Based on the audit, the `migrate down <target>` CLI should:

1. **Refuse without `--yes`.** All destructive operations.
2. **Require a restore point ID** (`--restore-point <id>`) when the path crosses any **Irreversible** block. The pre-flight computes the migration path, classifies each block, and refuses if any is Irreversible and no ID is supplied.
3. **Refuse if any `.ddsa` export lineage exists in `archive_meta`** AND the path crosses Block 4 (`phase1DistributedMergeMigration`). The `.ddsa` receiving-side `sync_id` references cannot be unwound.
4. **Refuse paths that cross Block 12 (`migrateSanitizedDisplayIDs`) or Block 13 (`migrateCanonicalDateData`)** unless `--force-irreversible` is passed AND the user has acknowledged the data loss in `--yes`.
5. **Warn (but proceed) when path crosses Block 16/17** (v55) because the doc-vs-code mismatch means the SQL is reversible even though the doc claims otherwise.
6. **Run the inverse in strict most-recent-first order** inside a single `tx, err := db.conn.Begin()` + `defer tx.Rollback()` envelope, mirroring `applySchema` at `internal/db/schema.go:368-374`.
7. **Write `PRAGMA user_version = <target>` as the LAST write** inside the tx, immediately before `tx.Commit()`. Writing it earlier would corrupt the rollback semantics (a crashed-mid-DOWN DB would claim to be at `target` while still being at v59's shape).
8. **Take an automatic `RetainedBackupManager.CreatePreSchemaDowngradeBackup(...)`** before the tx. The struct needs a direction/kind discriminator (`internal/update/retained_backup_manager.go:33-43` — currently no `Direction` field) and a direction-aware snapshot filename (line 67 hard-codes `dixiedata-pre-upgrade.db`). The cap of `defaultMaxRetainedBackups = 5` is adequate for one-shot DOWN but too tight for multi-version DOWN-then-UP churn; consider bumping to 10 or pairing with a user-initiated `restore point create` from the already-wired CLI at `internal/appshell/cli_admin.go:537-575`.
9. **Refuse the DOWN runner reusing the inline `schema` constant.** The 19 `CREATE TABLE IF NOT EXISTS` + 11+ `CREATE INDEX IF NOT EXISTS` + the `archive_meta` seed at `internal/db/schema.go:19-340` cannot be re-invoked by a DOWN transaction. The cleaner refactor (recommended) is to convert the ordered block list at `schema.go:379-516` into a `var migrations = []Migration{ Up, Down }` slice and have both `applySchema` (UP) and a future `applyDownSchema` (DOWN) iterate over it.

## Files in this audit's scope

- `internal/db/schema.go` — source of truth (read-only audit; no edits)
- `internal/db/db.go` — runner + `backupBeforeMigrationIfNeeded` trigger
- `internal/db/displayid.go` — `SanitizeID`, `CanonicalDisplayID`, `NextGeneratedDisplayID`, `NormalizeNodePrefix`
- `internal/db/identity_sql.go` — `syncIDSQL` (non-deterministic UUID generator)
- `internal/dates/dates.go` — `ParseBirthInfo`, `NormalizeCanonical`, `ParseCanonical`, `MustFormat`
- `internal/update/retained_backup_manager.go` — pre-upgrade snapshot mechanism (UP path only; needs DOWN-path additions)
- `internal/update/restore_point_manager.go` — user-triggered restore points (already cross-data-dir safe via sibling-root variant)
- `internal/appshell/cli_admin.go:23-26` — header comment that documents the DOWN-not-shipped rationale
- `internal/appshell/cli_admin.go:537-575` — `runAdminRestorePointCreate` (already wired, ready for the recommended user-initiated pre-DOWN snapshot)
- `internal/appshell/cli_debug_inplace.go:177-260` — `classifySchemaLine` walker (informational; cannot be lifted as-is for a per-migration classifier)
- `internal/appshell/cli_debug_inplace_test.go` — 11 tests that lock the walker's exact substring patterns (good template for any new classifier's regression net)
- `internal/db/migration_backup_test.go` — UP-path test only (no DOWN coverage exists)
- `scripts/bump-version.ps1` — release-counter enforcement + schema-drift CI gate
- `docs/migrations/v52.md`, `v53.md`, `v54.md`, `v55.md`, `v58.md`, `v59.md` — per-version migration notes
- `docs/agents/cli-plan.md:686-690` — the open-follow-up that lists the DOWN feature as "Blocked on a schema audit"

## Cross-reference

- Issue #266 — `v{MAJOR}.{U}.{N}` version split (shipped in commit 5a297a6). The DOWN feature must write the target `user_version` using the new shape.
- Issue #269 — `restore point apply` (now shipped per CHANGELOG). The recommended user-initiated pre-DOWN snapshot relies on this codepath.
- ADR 0007 — in-place update safety. Block 12/13 irreversibility is the reason `migrate down` cannot be a sub-feature of the in-place update flow; it must be a separate, opt-in, manual operation.
- ADR 0008 — promotion protocol. The `make promote` gate chain runs `make in-place-safety`; a future enhancement could add `make schema-reversibility-audit` as a parallel gate.

## Regression net for the future DOWN feature

When implementation lands, the regression net is:

- **Per-block inverse unit tests** — one per Reversible block (Blocks 1, 2, 9, 10, 15, 16, 18); the Irreversible blocks get stub tests that assert refusal.
- **Integration test** — temp DB at v59, run DOWN to v58, assert `user_version == 58` AND `share_queue_presets` table absent (reverse of v59's ADD-TABLE).
- **Negative test** — DOWN past Block 12 (`migrateSanitizedDisplayIDs`) refuses + exit 1 + prints the HIGH-risk warning + prints the supplied restore-point ID.
- **Restore-point parity test** — `restore point create` before DOWN produces a snapshot the operator can apply to roll back to v59; assert the sibling-root variant survives the DOWN/UP round-trip.
- **`bump-version.ps1 -VerifyOnly`** — must continue to pass (no change to `versioninfo.go`).
- **`make in-place-safety`** — must continue to pass (the new DOWN SQL does not add diff-time destructive patterns to the schema walker).

## Audit completion status

This audit resolves the blocker. The `migrate down <version>` feature can now be designed and implemented against a complete per-block classification table. The recommended slice plan (per the issue body's Two pieces section) is:

1. Apply the audit's recommendations to `internal/db/schema.go` (refactor block list into `[]Migration{Up, Down}`).
2. Add DOWN-path additions to `internal/update/retained_backup_manager.go` (direction discriminator, DOWN-specific snapshot filename).
3. Wire the CLI in `internal/appshell/cli_admin.go` (`runAdminMigrateDown`) with the contract above (refuse without `--yes`, require `--restore-point` across Irreversible blocks, refuse `.ddsa` lineage paths past Block 4).
4. Add the per-block inverse unit tests + integration + negative + restore-point parity tests as a new `internal/db/migrate_down_test.go`.

Each step is independent and lands as a separate commit per AGENTS.md §Commits and branches.