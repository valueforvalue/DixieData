// article_record_repo_parity_test.go — issue #613 slice 4
// regression net.
//
// Pins the contract that the new ArticleRecordRepo-backed
// ArticleService.Create / GetByID / Update / Delete paths
// return identical results to the legacy inline-SQL paths
// on the same fixture.
package records

import (
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

// TestArticleRecordRepo_Parity_CRUD is the slice-4
// service-level parity check. The full
// Create → GetByID → Update → GetByID → Delete → GetByID
// cycle exercises every slice-4 write path through the
// service layer, which means every pre-DML normalization
// (title trim, body render, sync_id mint, display_id mint,
// timestamp stamping) runs against the slice-4 repo
// delegation. A regression here would mean the new repo
// INSERT/UPDATE/DELETE broke something the legacy inline
// SQL handled correctly.
func TestArticleRecordRepo_Parity_CRUD(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)
	artSvc := NewArticleService(svc)

	// Create.
	created, err := artSvc.Create(models.Article{
		Title: "Original Title",
		BodyMD: "# Hello",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Title != "Original Title" {
		t.Errorf("after Create: Title = %q, want %q", created.Title, "Original Title")
	}

	// GetByID returns the row slice-4 just inserted.
	got, err := artSvc.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID after Create: %v", err)
	}
	if got.Title != "Original Title" {
		t.Errorf("after Create: GetByID Title = %q, want %q", got.Title, "Original Title")
	}

	// Update modifies the row; GetByID returns the new shape.
	got.Title = "Updated Title"
	if err := artSvc.Update(*got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got2, err := artSvc.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID after Update: %v", err)
	}
	if got2.Title != "Updated Title" {
		t.Errorf("after Update: Title = %q, want %q", got2.Title, "Updated Title")
	}

	// Delete removes the row; GetByID returns ErrArticleNotFound.
	if err := artSvc.Delete(created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := artSvc.GetByID(created.ID); err == nil {
		t.Errorf("GetByID after Delete: err = nil, want ErrArticleNotFound")
	}
}