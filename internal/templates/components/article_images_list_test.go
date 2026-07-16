package components

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

// TestArticleImagesListFragment_RendersEmptyState pins the
// empty-state surface: when the article has no images, the
// fragment renders a "No images attached yet" message instead
// of an empty <ul> so the picker modal has something to show.
func TestArticleImagesListFragment_RendersEmptyState(t *testing.T) {
	var buf bytes.Buffer
	if err := ArticleImagesListFragment(42, nil).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "No images attached yet") {
		t.Errorf("empty state message missing\nfull render:\n%s", got)
	}
	if strings.Contains(got, "data-article-image-row") {
		t.Errorf("empty state must not render a <li data-article-image-row>\nfull render:\n%s", got)
	}
}

// TestArticleImagesListFragment_RendersInsertButtons pins the
// happy path: each image row carries a button with the
// data-article-image-insert attr + the data-article-image-url
// attr (the picker click handler reads both).
func TestArticleImagesListFragment_RendersInsertButtons(t *testing.T) {
	images := []models.Image{
		{ID: 1, FileName: "portrait.jpg", FilePath: "images/articles/ART-00001/portrait.jpg", Kind: "article"},
		{ID: 2, FileName: "map.jpg", FilePath: "images/articles/ART-00001/map.jpg", Kind: "article"},
	}
	var buf bytes.Buffer
	if err := ArticleImagesListFragment(1, images).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	for _, img := range images {
		insertMarker := `data-article-image-insert`
		if !strings.Contains(got, insertMarker) {
			t.Errorf("missing %q\nfull render:\n%s", insertMarker, got)
		}
		urlMarker := `data-article-image-url="/media/` + img.FilePath + `"`
		if !strings.Contains(got, urlMarker) {
			t.Errorf("missing %q\nfull render:\n%s", urlMarker, got)
		}
		idMarker := `data-article-image-id="` + strconv.FormatInt(img.ID, 10) + `"`
		if !strings.Contains(got, idMarker) {
			t.Errorf("missing %q\nfull render:\n%s", idMarker, got)
		}
	}
}