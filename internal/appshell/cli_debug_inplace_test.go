// cli_debug_inplace_test.go — locks the in-place-safety walker
// regex shape for issue #268. `classifyAddedLine(file, line)`
// returns (kind, severity, reason) for any diff `+` line that
// matches a destructive pattern (DROP TABLE / DROP COLUMN /
// RENAME / DELETE FROM in a schema file, route registration
// in routes.go). Future regex refinements risk false positives
// (a SQL comment matching DROP TABLE) or false negatives (a
// migrations file outside internal/db/ that the conservative
// isSchemaFile() path misses).
//
// This test pins the behavior the issue body called out:
//   - A diff with a `DROP TABLE` in `internal/db/` is flagged HIGH.
//   - A diff with an `r.Get(` in `routes.go` is flagged MEDIUM.
//   - A diff with `DROP TABLE` in a comment is NOT flagged.
//   - A diff with `r.Get(` in `routes_test.go` is NOT flagged
//     (the pattern is `routes.go`, not `routes_test.go`).
//   - A diff with `DROP TABLE` in `internal/records/`
//     (not a schema file under the conservative gate) is NOT
//     flagged.

package appshell

import (
	"testing"
)

func TestClassifyAddedLine_DropTableInMigrationFile_High(t *testing.T) {
	kind, severity, _ := classifyAddedLine(
		"internal/db/migrate_2026_08_01_drop_users.sql",
		"DROP TABLE users;",
	)
	if kind != "schema_drop_table" {
		t.Errorf("kind=%q want schema_drop_table", kind)
	}
	if severity != "high" {
		t.Errorf("severity=%q want high", severity)
	}
}

func TestClassifyAddedLine_RenameInMigrationFile_High(t *testing.T) {
	kind, severity, _ := classifyAddedLine(
		"internal/db/migrate_2026_08_02_rename.sql",
		"ALTER TABLE users RENAME TO accounts;",
	)
	if kind != "schema_rename" {
		t.Errorf("kind=%q want schema_rename", kind)
	}
	if severity != "high" {
		t.Errorf("severity=%q want high", severity)
	}
}

func TestClassifyAddedLine_DeleteFromInMigrationFile_High(t *testing.T) {
	kind, severity, _ := classifyAddedLine(
		"internal/db/migrate_2026_08_03_prune.sql",
		"DELETE FROM audit_log WHERE created_at < '2024-01-01';",
	)
	if kind != "schema_delete" {
		t.Errorf("kind=%q want schema_delete", kind)
	}
	if severity != "high" {
		t.Errorf("severity=%q want high", severity)
	}
}

func TestClassifyAddedLine_RouteRegistration_Medium(t *testing.T) {
	kind, severity, _ := classifyAddedLine(
		"internal/appshell/routes.go",
		`r.Get("/soldiers/{id}", a.handleSoldier)`,
	)
	if kind != "handler_registration" {
		t.Errorf("kind=%q want handler_registration", kind)
	}
	if severity != "medium" {
		t.Errorf("severity=%q want medium", severity)
	}
}

func TestClassifyAddedLine_RouteRegistrationAllMethods(t *testing.T) {
	cases := []string{
		`r.Get("/foo", a.h)`,
		`r.Post("/foo", a.h)`,
		`r.Put("/foo", a.h)`,
		`r.Delete("/foo", a.h)`,
		`r.Patch("/foo", a.h)`,
		`r.Handle("/foo", a.h)`,
	}
	for _, line := range cases {
		kind, severity, _ := classifyAddedLine("internal/appshell/routes.go", line)
		if kind != "handler_registration" || severity != "medium" {
			t.Errorf("line=%q kind=%q severity=%q want handler_registration/medium", line, kind, severity)
		}
	}
}

func TestClassifyAddedLine_DropTableInComment_NotFlagged(t *testing.T) {
	// SQL comment line: -- DROP TABLE foo
	// Pre-fix: classifier matches DROP TABLE regardless of
	// whether it's a comment. Post-fix: skip comments.
	kind, _, _ := classifyAddedLine(
		"internal/db/migrate_2026_08_04_commented.sql",
		"-- DROP TABLE users; never actually runs",
	)
	if kind != "" {
		t.Errorf("comment line should not be flagged (kind=%q); walker must skip -- and /* ... */ lines", kind)
	}
}

func TestClassifyAddedLine_DropTableInBlockComment_NotFlagged(t *testing.T) {
	kind, _, _ := classifyAddedLine(
		"internal/db/migrate_2026_08_05_blocked.sql",
		"/* DROP TABLE users; the plan was to drop, then we kept them */",
	)
	if kind != "" {
		t.Errorf("block comment line should not be flagged (kind=%q); walker must skip */...*/ lines", kind)
	}
}

func TestClassifyAddedLine_RouteInRoutesTest_NotFlagged(t *testing.T) {
	// routes_test.go ends with "_test.go" not "routes.go", so
	// the conservative isHandlerChange path is naturally
	// skipped. Pin this so a future refactor doesn't
	// widen the suffix.
	kind, _, _ := classifyAddedLine(
		"internal/appshell/routes_test.go",
		`r.Get("/foo", a.h)`,
	)
	if kind != "" {
		t.Errorf("routes_test.go is not routes.go; line must NOT be flagged (kind=%q)", kind)
	}
}

func TestClassifyAddedLine_DropTableInNonSchemaFile_NotFlagged(t *testing.T) {
	// records/ is not under internal/db/ or docs/migrations/,
	// so the conservative isSchemaFile gate returns false.
	// If a string DROP TABLE appears in a Go test fixture,
	// the walker must not flag it.
	kind, _, _ := classifyAddedLine(
		"internal/records/soldiers_test.go",
		`DROP TABLE fixture_users;`,
	)
	if kind != "" {
		t.Errorf("SQL in a Go test fixture is not a migration; must not be flagged (kind=%q)", kind)
	}
}

func TestClassifyAddedLine_DropColumnInMigrationFile_High(t *testing.T) {
	kind, severity, _ := classifyAddedLine(
		"internal/db/migrate_2026_08_06.sql",
		"ALTER TABLE soldiers DROP COLUMN old_field;",
	)
	if kind != "schema_drop_column" {
		t.Errorf("kind=%q want schema_drop_column", kind)
	}
	if severity != "high" {
		t.Errorf("severity=%q want high", severity)
	}
}

func TestClassifyAddedLine_PlainGoLine_NotFlagged(t *testing.T) {
	// Regression guard: random internal/appshell/*.go edits
	// should not trip the walker. The walker fires only on
	// routes.go or internal/db/ files; a fresh handler body
	// in another file should pass clean.
	kind, _, _ := classifyAddedLine(
		"internal/appshell/soldiers_handlers.go",
		`return nil`,
	)
	if kind != "" {
		t.Errorf("non-destructive line should not flag (kind=%q)", kind)
	}
}
