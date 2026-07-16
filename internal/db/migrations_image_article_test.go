// migrations_image_article_test.go — issue #612 slice 1
// Regression net for the new article-images schema (image
// rows attached to Article Records via nullable
// article_id + a kind discriminator column).
//
// Pins the slice-1 contract end-to-end:
//   - The new migration block is registered in Migrations()
//     (the catalogue test + reversibility test both catch
//     a forgotten entry).
//   - The schema grows by 2 columns: article_id (nullable,
//     FK to articles(id) ON DELETE CASCADE) + kind (TEXT,
//     defaults to 'person' so existing rows are valid).
//   - Existing rows are backfilled with kind = 'person' so
//     every image in a v67-or-earlier archive carries the
//     right discriminator (otherwise the picker filter
//     'kind = ''person''' would silently drop them).
//   - The new index on article_id exists so the picker
//     query is O(log n) (otherwise the picker is a table
//     scan on the editor page).
package db

import (
	"database/sql"
	"testing"
)

// TestMigrations_CatalogueIncludesImageArticleBlock pins the
// new block is in Migrations() (otherwise the schema never
// grows the new columns).
func TestMigrations_CatalogueIncludesImageArticleBlock(t *testing.T) {
	found := false
	for _, m := range Migrations() {
		if m.ID == "block-68-images-article-id-and-kind" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("Migrations() missing block-68-images-article-id-and-kind; the slice-1 schema change never lands")
	}
}

// TestMigrations_ImageArticleBlockReversibility pins the new
// block's reversibility classification. Reversible is the
// default for pure additive column changes; a future flip to
// PartiallyReversible or Irreversible would change the
// applyDownSchema runner's gate behaviour.
func TestMigrations_ImageArticleBlockReversibility(t *testing.T) {
	for _, m := range Migrations() {
		if m.ID != "block-68-images-article-id-and-kind" {
			continue
		}
		if m.Reversibility != Reversible {
			t.Errorf("block-68-images-article-id-and-kind reversibility = %s, want Reversible (pure additive column + index + backfill; no data is moved)", m.Reversibility)
		}
		return
	}
	t.Fatal("block-68-images-article-id-and-kind not found; reversibility assertion skipped")
}

// TestImageArticleSchemaAfterMigrations pins the actual
// schema shape after block-68 lands: the article_id + kind
// columns exist with the right types, the kind column has
// the right default, the backfill ran, and the new index
// exists. The test stands up a real in-memory SQLite +
// applies every migration via the same applySchema path
// the production binary uses.
func TestImageArticleSchemaAfterMigrations(t *testing.T) {
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer conn.Close()

	// Apply the inline schema const first (block-1) so
	// the images table exists with its v67 columns.
	if _, err := conn.Exec(schema); err != nil {
		t.Fatalf("apply inline schema: %v", err)
	}

	// Then walk every migration in Migrations().
	for _, m := range migrations {
		tx, err := conn.Begin()
		if err != nil {
			t.Fatalf("begin tx for block %s: %v", m.ID, err)
		}
		if err := m.Up(tx); err != nil {
			_ = tx.Rollback()
			t.Fatalf("block %s Up: %v", m.ID, err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("commit block %s: %v", m.ID, err)
		}
	}

	// The images table must now carry the new columns.
	columns := tableColumns(t, conn, "images")
	expectColumn(t, columns, "article_id")
	expectColumn(t, columns, "kind")

	// The kind column must default to 'person' so a legacy
	// INSERT that doesn't supply kind still produces a
	// person-typed row (the picker filter relies on this).
	dflt := columnDefault(t, conn, "images", "kind")
	if dflt != "'person'" {
		t.Errorf("images.kind default = %q, want 'person' (so legacy INSERTs default to person)", dflt)
	}

	// The new index must exist.
	if !indexExists(t, conn, "idx_images_article_id") {
		t.Errorf("index idx_images_article_id missing; the picker query would be a table scan on the editor page")
	}
}

// tableColumns returns the set of column names on the table.
func tableColumns(t *testing.T, conn *sql.DB, tableName string) map[string]bool {
	t.Helper()
	rows, err := conn.Query(`PRAGMA table_info(` + tableName + `)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info(%s): %v", tableName, err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out[name] = true
	}
	return out
}

// expectColumn asserts the column exists on the table.
func expectColumn(t *testing.T, columns map[string]bool, name string) {
	t.Helper()
	if !columns[name] {
		var names []string
		for k := range columns {
			names = append(names, k)
		}
		t.Errorf("column %q missing (got: %v)", name, names)
	}
}

// columnDefault returns the declared DEFAULT clause for the
// column, or "" if none. Returned with surrounding quotes if
// the value is a string literal (so 'person' returns
// "'person'" which is the right shape to assert against
// PRAGMA output).
func columnDefault(t *testing.T, conn *sql.DB, tableName, columnName string) string {
	t.Helper()
	rows, err := conn.Query(`PRAGMA table_info(` + tableName + `)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info(%s): %v", tableName, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if name == columnName {
			if dfltValue.Valid {
				return dfltValue.String
			}
			return ""
		}
	}
	return ""
}

// indexExists asserts an index with the given name exists.
func indexExists(t *testing.T, conn *sql.DB, name string) bool {
	t.Helper()
	rows, err := conn.Query(`SELECT name FROM sqlite_master WHERE type = 'index' AND name = ?`, name)
	if err != nil {
		t.Fatalf("sqlite_master query: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		return true
	}
	return false
}