// static_archive_test.go — package tests for the static
// archive pipeline (issue #320 + #321).
package archive

import (
	"archive/zip"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/records"
)

// TestExportStaticArchive_IncludesArticles pins the slice-5.3
// contract: the static archive index emits
// window.DIXIE_DATA.articles[] alongside the existing records +
// events arrays. Each article carries the body_html + the
// resolvedRefs projection.
func TestExportStaticArchive_IncludesArticles(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	if _, err := d.ConfigureUserIdentity("Samuel", "Thomas", "Carter", 1838); err != nil {
		t.Fatalf("ConfigureUserIdentity: %v", err)
	}
	exportSvc := NewExportService(d, soldierSvc)
	articleSvc := records.NewArticleService(soldierSvc)

	// Seed a person + an article.
	person, err := soldierSvc.Create(models.Soldier{
		DisplayID: "DXD-00099",
		FirstName: "Test",
		LastName:  "Person",
		Rank:      "Private",
		Unit:      "Test Unit",
	})
	if err != nil {
		t.Fatalf("Create person: %v", err)
	}
	art, err := articleSvc.Create(models.Article{Title: "Static target", BodyMD: "# Hello"})
	if err != nil {
		t.Fatalf("Create article: %v", err)
	}
	if _, err := articleSvc.AttachRef(art.ID, person.ID); err != nil {
		t.Fatalf("AttachRef: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "static.zip")
	if err := exportSvc.ExportStaticArchive(outPath, t.TempDir()); err != nil {
		t.Fatalf("ExportStaticArchive: %v", err)
	}
	// The zip contains archive_data.js; extract the file
	// into memory via zip.NewReader (avoids disk extraction).
	zr, err := zip.OpenReader(outPath)
	if err != nil {
		t.Fatalf("zip.OpenReader: %v", err)
	}
	defer zr.Close()
	var data []byte
	for _, f := range zr.File {
		if f.Name == "archive_data.js" {
			rc, rerr := f.Open()
			if rerr != nil {
				t.Fatalf("open archive_data.js: %v", rerr)
			}
			defer rc.Close()
			data, err = io.ReadAll(rc)
			if err != nil {
				t.Fatalf("read archive_data.js: %v", err)
			}
			break
		}
	}
	if data == nil {
		t.Fatalf("archive_data.js not in zip")
	}
	if err != nil {
		t.Fatalf("read archive_data.js: %v", err)
	}
	contents := string(data)
	if !strings.Contains(contents, `"articles"`) {
		t.Errorf("archive_data.js missing articles key")
	}
	if !strings.Contains(contents, `"Static target"`) {
		t.Errorf("archive_data.js missing article title")
	}
	if !strings.Contains(contents, `DXD-00099`) {
		t.Errorf("archive_data.js missing attached person display id")
	}
}
