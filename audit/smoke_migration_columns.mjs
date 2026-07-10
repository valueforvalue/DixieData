// audit/smoke_migration_columns.mjs — regression net for issue #435.
//
// The v54→v60 migration renamed 12 columns across 5 tables
// (`soldier_id` → `person_record_id`, `soldier_sync_id` → `person_sync_id`,
// and the 4 `*_soldier_id` → `*_record_id` columns on
// merge_review_conflicts + duplicate_audit_findings). The renames
// are guarded by `columnExists` so the migration is idempotent on
// fresh v60 DBs. BUT: any production code path that still references
// the OLD column name will fail on a fresh v60 DB with `no such
// column: <old_name>`.
//
// This probe extracts the rename map from `internal/db/migrations.go`
// (the same source-of-truth that ships the rename statements) and
// greps the production Go tree for SQL string literals that still
// reference the OLD column names. Any hit is a failure.
//
// Run with:
//
//   node audit/smoke_migration_columns.mjs
//
// Exits 0 on green, 1 on any leftover hit. Wired into `make
// lint` alongside the swallowed-errors probe. Refs issue #435.
import { strict as assert } from "node:assert";
import { readFileSync, readdirSync, statSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = join(__dirname, "..");

const MIGRATIONS = join(ROOT, "internal/db/migrations.go");
const APPSHELL = join(ROOT, "internal/appshell");
const RECORDS = join(ROOT, "internal/records");
const ARCHIVE = join(ROOT, "internal/archive");
const DB_PKG = join(ROOT, "internal/db");
const CMD_GOLD = join(ROOT, "cmd/gold-master");
const PKG_RENDER = join(ROOT, "pkg/render");

let pass = 0;
let fail = 0;

function test(name, fn) {
  try {
    fn();
    pass++;
    console.log(`  ✓ ${name}`);
  } catch (err) {
    fail++;
    console.log(`  ✗ ${name}`);
    console.log(`    ${err.message}`);
  }
}

// --- 1. Extract the rename map from migrations.go ---
//
// The renames live in the v54→v60 sub-block as a slice literal of
// {table, from, to} structs. We parse just enough of the source to
// pull those three fields per row. Robust enough for the current
// shape; if the migration table shape ever changes, this regex
// breaks loudly (the assert below catches a zero-result parse).
const migrationsSrc = readFileSync(MIGRATIONS, "utf8");
const renameRe = /\{\s*"([a-z_]+)"\s*,\s*"([a-z_]+)"\s*,\s*"([a-z_]+)"\s*\}/g;
const renames = [];
let m;
while ((m = renameRe.exec(migrationsSrc)) !== null) {
  renames.push({ table: m[1], from: m[2], to: m[3] });
}
test(`migrations.go declares at least 10 renames (got ${renames.length})`, () => {
  assert.ok(renames.length >= 10, `expected ≥10 renames, parsed ${renames.length}; the migration block may have changed shape`);
});

// --- 2. Grep production Go source for OLD column references ---
//
// We look for SQL string-literal patterns that reference `<table>.<oldcol>`
// or `<oldcol>` as an identifier in an INSERT/UPDATE/SELECT/WHERE/ON
// context. The probe errs on the side of false positives (any line
// containing both the table name + the old column name is a hit) so
// human review is required on a red.
//
// Excluded paths: the migration source itself (where the rename is
// declared), test files, and tool sources. The probe only inspects
// the production binary's Go code.
const PROD_DIRS = [APPSHELL, RECORDS, ARCHIVE, DB_PKG, CMD_GOLD, PKG_RENDER];
// migrations.go is excluded because it is the SOURCE OF TRUTH
// for the renames — grep'ing it for the OLD names is a tautology.
const EXCLUDE_FILES = new Set([MIGRATIONS]);

function walkGo(dir) {
  const out = [];
  let entries;
  try {
    entries = readdirSync(dir);
  } catch (_) {
    return out;
  }
  for (const e of entries) {
    const full = join(dir, e);
    if (EXCLUDE_FILES.has(full)) {
      continue;
    }
    let s;
    try {
      s = statSync(full);
    } catch (_) {
      continue;
    }
    if (s.isDirectory()) {
      out.push(...walkGo(full));
    } else if (e.endsWith(".go") && !e.endsWith("_test.go")) {
      out.push(full);
    } else if (e.endsWith("_test.go")) {
      // Issue #448 slice 2: also walk *_test.go files, but
      // surface findings as warnings (not failures). Tests may
      // legitimately exercise OLD column names against a v54
      // fixture DB, but drift should still be visible.
      testFiles.push(full);
    }
  }
  return out;
}

const files = [];
const testFiles = [];
for (const d of PROD_DIRS) {
  files.push(...walkGo(d));
}

const offenders = [];
for (const r of renames) {
  // SQL keywords that must appear in the same line to count as
  // a SQL reference (vs. a JSON map key, Go field name, or
  // comment). The probe errs on the side of false positives when
  // a line is on the boundary; the human review is short.
  const sqlKw = "(SELECT|INSERT|UPDATE|DELETE|FROM|JOIN|WHERE|ON|ORDER|GROUP|VALUES|SET|REFERENCES|INTO)";
  // Match table + oldcol with a SQL keyword between them OR
  // oldcol as a bare identifier in a context where a SQL
  // statement is on the same line.
  const re = new RegExp(
    `\\b${r.table}\\b[\\s\\S]{0,300}\\b${r.from}\\b|\\b${r.from}\\b[\\s\\S]{0,100}\\b${r.table}\\b[\\s\\S]{0,50}${sqlKw}\\b`,
    "i"
  );
  for (const f of files) {
    const src = readFileSync(f, "utf8");
    const lines = src.split("\n");
    for (let i = 0; i < lines.length; i++) {
      const line = lines[i];
      // Skip lines that are clearly not SQL: pure map keys
      // (e.g. `"soldier_id": soldier.ID,`), comments, struct
      // field tags. The probe still matches an `INSERT INTO
      // <table> (<oldcol>, ...)` line which is the bug class
      // we're hunting.
      if (line.trim().startsWith("//") || line.trim().startsWith("*")) {
        continue;
      }
      if (re.test(line)) {
        offenders.push({
          file: f.replace(ROOT + "\\", "").replace(ROOT + "/", ""),
          line: i + 1,
          table: r.table,
          from: r.from,
          to: r.to,
          text: line.trim().slice(0, 120),
        });
      }
    }
  }
}

test(`no production code references renamed columns (checked ${renames.length} renames across ${files.length} files)`, () => {
  if (offenders.length > 0) {
    const lines = offenders
      .map((o) => `    ${o.file}:${o.line}  ${o.table}.${o.from} (rename → ${o.to})\n      ${o.text}`)
      .join("\n");
    throw new Error(`Found ${offenders.length} leftover references to renamed columns:\n${lines}`);
  }
});

// Issue #448 slice 2: scan *_test.go files for renamed-column
// references and emit a warn-level finding (NOT a failure).
// Tests may legitimately exercise OLD column names against a v54
// fixture DB, but drift should still be visible to reviewers.
const testOffenders = [];
for (const r of renames) {
  const sqlKw = "(SELECT|INSERT|UPDATE|DELETE|FROM|JOIN|WHERE|ON|ORDER|GROUP|VALUES|SET|REFERENCES|INTO)";
  const re = new RegExp(
    `\\b${r.table}\\b[\\s\\S]{0,300}\\b${r.from}\\b|\\b${r.from}\\b[\\s\\S]{0,100}\\b${r.table}\\b[\\s\\S]{0,50}${sqlKw}\\b`,
    "i"
  );
  for (const f of testFiles) {
    const src = readFileSync(f, "utf8");
    const lines = src.split("\n");
    for (let i = 0; i < lines.length; i++) {
      const line = lines[i];
      if (line.trim().startsWith("//") || line.trim().startsWith("*")) {
        continue;
      }
      if (re.test(line)) {
        testOffenders.push({
          file: f.replace(ROOT + "\\", "").replace(ROOT + "/", ""),
          line: i + 1,
          table: r.table,
          from: r.from,
          to: r.to,
          text: line.trim().slice(0, 120),
        });
      }
    }
  }
}

test(`warn: test files reference renamed columns (checked ${testFiles.length} *_test.go files)`, () => {
  if (testOffenders.length > 0) {
    const lines = testOffenders
      .map((o) => `    ${o.file}:${o.line}  ${o.table}.${o.from} (rename → ${o.to})\n      ${o.text}`)
      .join("\n");
    console.warn(`WARN: ${testOffenders.length} *_test.go files reference renamed columns (review for drift):\n${lines}`);
  }
});

// --- 3. (Future) Verify the migration's rename block parses for
// both the OLD and NEW column names against the schema. Today this
// is implicit in the smoke harness (the gold-master portability
// audit exercises both names via fresh + restored DBs). When the
// next rename lands, add an explicit assertion that the schema
// exports the NEW column name and the old name is gone.

console.log(`\n${pass} passed, ${fail} failed`);

if (fail > 0) {
  process.exit(1);
}
