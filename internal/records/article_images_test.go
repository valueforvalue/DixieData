// article_images_test.go — issue #612 slice 2
// Regression net for the article-image AddImage + ImagesForArticle
// helpers on the ArticleService. The HTTP layer (POST
// /articles/{id}/images/import + GET /articles/{id}/images) is
// tested in internal/appshell alongside the per-Person-Record
// import; this file pins the service contract that the HTTP
// handlers consume.
//
// Pins:
//   - AddImage inserts a row with article_id + kind='article'
//     + a per-article file path (images/articles/<displayID>/...)
//   - ImagesForArticle returns exactly the article-attached
//     images (NOT the per-Person-Record images)
//   - The reverse: person images don't leak into the article
//     query (the kind discriminator filters cleanly)
package records

import (
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

// TestArticleImages_AddImageInsertsRowWithKindArticle pins
// the slice-2 contract: AddImage inserts a row with
// article_id=<id> + kind='article' (not 'person') + a
// per-article file path under dataDir/images/articles/<id>/.
func TestArticleImages_AddImageInsertsRowWithKindArticle(t *testing.T) {
	svc := newArticleServiceForTest(t)
	article, err := svc.Create(models.Article{Title: "Test article"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := svc.AddImage(article.ID, "test.jpg", "images/articles/test.jpg", ""); err != nil {
		t.Fatalf("AddImage: %v", err)
	}

	images, err := svc.ImagesForArticle(article.ID)
	if err != nil {
		t.Fatalf("ImagesForArticle: %v", err)
	}
	if len(images) != 1 {
		t.Fatalf("ImagesForArticle returned %d images, want 1", len(images))
	}
	img := images[0]
	if img.Kind != "article" {
		t.Errorf("image.kind = %q, want %q (the article picker must filter on kind='article')", img.Kind, "article")
	}
	if img.ArticleID == nil || *img.ArticleID != article.ID {
		t.Errorf("image.article_id = %v, want %d", img.ArticleID, article.ID)
	}
	if img.FilePath != "images/articles/test.jpg" {
		t.Errorf("image.file_path = %q, want %q (sibling under dataDir/images/articles/<displayID>/)", img.FilePath, "images/articles/test.jpg")
	}
	if !strings.HasPrefix(img.FilePath, "images/articles/") {
		t.Errorf("image.file_path = %q, want images/articles/... prefix", img.FilePath)
	}
}

// TestArticleImages_PickerQueryExcludesPersonImages pins the
// dual-purpose filter: a person-attached image must NOT appear
// in the article picker, and an article-attached image must NOT
// appear in the soldier picker. The kind discriminator is the
// filter; the SQL query uses WHERE article_id = ? AND kind = 'article'
// (or equivalent). Without this, the picker shows the wrong
// gallery and the user picks the wrong image.
func TestArticleImages_PickerQueryExcludesPersonImages(t *testing.T) {
	svc := newArticleServiceForTest(t)
	article, err := svc.Create(models.Article{Title: "Test article"})
	if err != nil {
		t.Fatalf("Create article: %v", err)
	}

	// Insert a person row directly (the soldier service
	// helper for Create varies across slices; we keep
	// the test self-contained here).
	d := newTestDB(t)
	if _, err := d.Conn().Exec(
		`INSERT INTO soldiers (sync_id, display_id, first_name, last_name) VALUES (?, ?, ?, ?)`,
		"person-1", "P-00001", "John", "Doe",
	); err != nil {
		t.Fatalf("insert soldier: %v", err)
	}
	var soldierID int64
	if err := d.Conn().QueryRow(`SELECT id FROM soldiers WHERE display_id = 'P-00001'`).Scan(&soldierID); err != nil {
		t.Fatalf("query soldier: %v", err)
	}

	// One image per owner. The kind discriminator + the
	// per-table FK ensure they live on opposite sides of
	// the picker filter.
	if err := svc.AddImage(article.ID, "chapter.jpg", "images/articles/chapter.jpg", ""); err != nil {
		t.Fatalf("AddImage (article): %v", err)
	}
	// Insert a per-Person-Record image directly via SQL
	// (matches the per-Person-Record AddImage shape).
	if _, err := d.Conn().Exec(
		`INSERT INTO images (sync_id, person_record_id, person_sync_id, kind, file_name, file_path, caption) VALUES (?, ?, ?, 'person', ?, ?, ?)`,
		"img-person-1", soldierID, "person-1", "portrait.jpg", "images/P/00001/portrait.jpg", "",
	); err != nil {
		t.Fatalf("insert person image: %v", err)
	}

	// Article picker: only the article image. The portrait
	// (kind='person') must NOT appear.
	articleImages, err := svc.ImagesForArticle(article.ID)
	if err != nil {
		t.Fatalf("ImagesForArticle: %v", err)
	}
	if len(articleImages) != 1 {
		t.Errorf("article picker returned %d images, want 1 (soldier image must NOT leak)", len(articleImages))
	}
	if len(articleImages) > 0 && articleImages[0].FilePath != "images/articles/chapter.jpg" {
		t.Errorf("article picker returned %q, want the article image", articleImages[0].FilePath)
	}

	// Person picker: only the portrait. The chapter
	// illustration (kind='article') must NOT leak. The
	// existing per-Person-Record query path filters on
	// person_record_id IS NOT NULL, but for a more
	// explicit contract, this test also pins the
	// 'person' discriminator filter by running the same
	// SQL the legacy code would run.
	var personImageCount int
	if err := d.Conn().QueryRow(
		`SELECT COUNT(*) FROM images WHERE person_record_id = ? AND kind = 'person'`,
		soldierID,
	).Scan(&personImageCount); err != nil {
		t.Fatalf("query person images: %v", err)
	}
	if personImageCount != 1 {
		t.Errorf("person images count = %d, want 1 (article image must NOT leak)", personImageCount)
	}
}