// Package sqlite provides the SQLite-backed implementations of
// the repository interfaces declared in internal/db/repo.
//
// This file pins the contract for ArticleRecordRepo (issue #613
// slice 4). Articles live in their own `articles` table
// (created by migration block-3-articles). Slice 4 covers 5
// methods: the read paths (GetByID, List) and the write paths
// (Create, Update, Delete). The cross-table methods (refs,
// snapshots, images) stay in ArticleService for slice 5+.
package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
)

// newArticleTestDB opens an in-memory SQLite with the slice-4
// articles schema (id, sync_id, display_id, title, subtitle,
// body_md, body_html, created_at, updated_at, snapshot_of_id,
// is_snapshot) + the minimum cross-table surface the service
// methods still touch (none in slice-4 scope; the test schema
// deliberately stops at articles).
func newArticleTestDB(t *testing.T) *db.DB {
	t.Helper()
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open in-memory: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	const schema = `
		CREATE TABLE articles (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			sync_id         TEXT NOT NULL DEFAULT '',
			display_id      TEXT NOT NULL UNIQUE,
			title           TEXT NOT NULL DEFAULT '',
			subtitle        TEXT NOT NULL DEFAULT '',
			body_md         TEXT NOT NULL DEFAULT '',
			body_html       TEXT NOT NULL DEFAULT '',
			created_at      TEXT NOT NULL DEFAULT '',
			updated_at      TEXT NOT NULL DEFAULT '',
			snapshot_of_id  INTEGER,
			is_snapshot     INTEGER NOT NULL DEFAULT 0
		);
	`
	if _, err := conn.Exec(schema); err != nil {
		t.Fatalf("apply schema: %v", err)
	}

	return db.NewFromExisting(conn)
}

// TestArticleRecordRepo_Create_ReturnsInsertID asserts Create
// inserts a row + returns the generated id.
func TestArticleRecordRepo_Create_ReturnsInsertID(t *testing.T) {
	d := newArticleTestDB(t)
	r := NewArticleRecordRepo(d)

	conn := d.Conn()
	tx, err := conn.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	id, err := r.Create(context.Background(), tx, models.Article{
		DisplayID: "ART-0001",
		Title:     "Test Article",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id <= 0 {
		t.Errorf("Create returned id = %d, want positive", id)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	row, err := r.GetByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetByID after Create: %v", err)
	}
	if row == nil {
		t.Fatalf("GetByID returned nil row")
	}
}

// TestArticleRecordRepo_GetByID_NotFound asserts the no-row
// case surfaces sql.ErrNoRows on Scan.
func TestArticleRecordRepo_GetByID_NotFound(t *testing.T) {
	d := newArticleTestDB(t)
	r := NewArticleRecordRepo(d)
	row, err := r.GetByID(context.Background(), 9999)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if row == nil {
		t.Fatalf("GetByID: returned nil row")
	}
	dest := make([]any, len(strings.Split(ArticleRecordSelectColumns, ",")))
	for i := range dest {
		dest[i] = new(sql.NullString)
	}
	if scanErr := row.Scan(dest...); scanErr != sql.ErrNoRows {
		t.Errorf("Scan err = %v, want sql.ErrNoRows", scanErr)
	}
}

// TestArticleRecordRepo_List_Pagination asserts List returns
// the expected total + page-1 size.
func TestArticleRecordRepo_List_Pagination(t *testing.T) {
	d := newArticleTestDB(t)
	conn := d.Conn()
	tx, err := conn.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	r := NewArticleRecordRepo(d)
	for i := 0; i < 15; i++ {
		if _, err := r.Create(context.Background(), tx, models.Article{
			DisplayID: "ART-" + itoa(i+1),
			Title:     "Article " + itoa(i),
		}); err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	rows, total, err := r.List(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 15 {
		t.Errorf("total = %d, want 15", total)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		count++
	}
	if count != 10 {
		t.Errorf("page 1 row count = %d, want 10", count)
	}
}

// TestArticleRecordRepo_Update_AffectsOneRow asserts Update
// modifies the row + returns rowsAffected=1.
func TestArticleRecordRepo_Update_AffectsOneRow(t *testing.T) {
	d := newArticleTestDB(t)
	r := NewArticleRecordRepo(d)

	conn := d.Conn()
	tx, err := conn.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	id, err := r.Create(context.Background(), tx, models.Article{
		DisplayID: "ART-0001",
		Title:     "Original",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	affected, err := r.Update(context.Background(), tx, models.Article{
		ID:        id,
		DisplayID: "ART-0001",
		Title:     "Updated",
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if affected != 1 {
		t.Errorf("Update rowsAffected = %d, want 1", affected)
	}
}

// TestArticleRecordRepo_Update_NotFound asserts Update on a
// missing id returns rowsAffected=0 with no error.
func TestArticleRecordRepo_Update_NotFound(t *testing.T) {
	d := newArticleTestDB(t)
	r := NewArticleRecordRepo(d)

	conn := d.Conn()
	tx, err := conn.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	affected, err := r.Update(context.Background(), tx, models.Article{
		ID:        9999,
		DisplayID: "ART-9999",
		Title:     "Ghost",
	})
	if err != nil {
		t.Fatalf("Update on missing id: %v", err)
	}
	if affected != 0 {
		t.Errorf("Update rowsAffected = %d, want 0", affected)
	}
}

// TestArticleRecordRepo_Delete_AffectsOneRow asserts Delete
// removes the row + returns rowsAffected=1.
func TestArticleRecordRepo_Delete_AffectsOneRow(t *testing.T) {
	d := newArticleTestDB(t)
	r := NewArticleRecordRepo(d)

	conn := d.Conn()
	tx, err := conn.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	id, err := r.Create(context.Background(), tx, models.Article{
		DisplayID: "ART-0001",
		Title:     "Delete me",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	tx2, err := conn.Begin()
	if err != nil {
		t.Fatalf("Begin tx2: %v", err)
	}
	defer tx2.Rollback()

	affected, err := r.Delete(context.Background(), tx2, id)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if affected != 1 {
		t.Errorf("Delete rowsAffected = %d, want 1", affected)
	}
}

// TestArticleRecordRepo_Delete_NotFound asserts Delete on a
// missing id returns rowsAffected=0 with no error.
func TestArticleRecordRepo_Delete_NotFound(t *testing.T) {
	d := newArticleTestDB(t)
	r := NewArticleRecordRepo(d)

	conn := d.Conn()
	tx, err := conn.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()

	affected, err := r.Delete(context.Background(), tx, 9999)
	if err != nil {
		t.Fatalf("Delete on missing id: %v", err)
	}
	if affected != 0 {
		t.Errorf("Delete rowsAffected = %d, want 0", affected)
	}
}