// static_archive_test.go — package tests for the static
// archive pipeline (issue #320 + #321 + #490).
package archive

import (
	"archive/zip"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/records"
	"github.com/valueforvalue/DixieData/internal/testtemp"
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

	outPath := filepath.Join(testtemp.New(t).Path(), "static.zip")
	if err := exportSvc.ExportStaticArchive(outPath, testtemp.New(t).Path()); err != nil {
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

// renderIndexForTest renders the static archive index HTML with
// placeholder template data so tests can string-match against
// the markup without seeding a full archive.
func renderIndexForTest(t *testing.T) string {
	t.Helper()
	html, err := renderStaticArchiveIndex(staticArchiveIndexData{
		ArchiveTitle: "Test Archive",
		Version:      "test",
		Build:        "test",
		GeneratedAt:  "2026-01-01",
	})
	if err != nil {
		t.Fatalf("renderStaticArchiveIndex: %v", err)
	}
	return html
}

// TestStaticArchiveIndex_RendersEventsAndArticlesTabs (issue #490)
// asserts the rendered index.html carries tab labels for all three
// entity kinds (Persons, Events, Articles) and the container elements
// the JS uses to render each list.
func TestStaticArchiveIndex_RendersEventsAndArticlesTabs(t *testing.T) {
	html := renderIndexForTest(t)

	// Tab labels — the segmented control in the hero section.
	for _, label := range []string{"Persons", "Events", "Articles"} {
		if !strings.Contains(html, label) {
			t.Errorf("rendered index.html missing tab label %q (issue #490)", label)
		}
	}

	// Tab container elements the JS swaps between. The JS reads
	// bundle.events + bundle.articles and renders into these.
	if !strings.Contains(html, "data-tab=\"events\"") {
		t.Errorf("missing data-tab=\"events\" element for Events tab (issue #490)")
	}
	if !strings.Contains(html, "data-tab=\"articles\"") {
		t.Errorf("missing data-tab=\"articles\" element for Articles tab (issue #490)")
	}
}

// TestStaticArchiveIndex_RendersEventDetailMarkup (issue #490)
// asserts the JS carries a renderEventDetail function that produces
// the Event detail screen (kind, date range, linked persons).
func TestStaticArchiveIndex_RendersEventDetailMarkup(t *testing.T) {
	html := renderIndexForTest(t)

	// The JS must have a function that renders event details.
	if !strings.Contains(html, "function renderEventDetail") {
		t.Errorf("missing renderEventDetail function in index.html JS (issue #490)")
	}
	// The hash router must handle #event= hashes.
	if !strings.Contains(html, "#event=") {
		t.Errorf("hash router missing #event= pattern (issue #490)")
	}
	// Event list rendering function.
	if !strings.Contains(html, "function renderEventRow") {
		t.Errorf("missing renderEventRow function in index.html JS (issue #490)")
	}
}

// TestStaticArchiveIndex_RendersArticleDetailMarkup (issue #490)
// asserts the JS carries a renderArticleDetail function that produces
// the Article detail screen (title, subtitle, body HTML, resolved refs).
func TestStaticArchiveIndex_RendersArticleDetailMarkup(t *testing.T) {
	html := renderIndexForTest(t)

	if !strings.Contains(html, "function renderArticleDetail") {
		t.Errorf("missing renderArticleDetail function in index.html JS (issue #490)")
	}
	if !strings.Contains(html, "#article=") {
		t.Errorf("hash router missing #article= pattern (issue #490)")
	}
	if !strings.Contains(html, "function renderArticleRow") {
		t.Errorf("missing renderArticleRow function in index.html JS (issue #490)")
	}
}

// TestStaticArchiveIndex_LinkedEventsSectionInPersonDetail (issue #490)
// asserts the Person detail screen carries a "Linked Events" section
// that filters bundle.events by linkedDisplayIds membership.
func TestStaticArchiveIndex_LinkedEventsSectionInPersonDetail(t *testing.T) {
	html := renderIndexForTest(t)

	// The renderDetail function (or a helper it calls) must produce
	// a "Linked Events" section heading.
	if !strings.Contains(html, "Linked Events") {
		t.Errorf("Person detail missing \"Linked Events\" section heading (issue #490)")
	}
	// The JS must filter events by linkedDisplayIds.
	if !strings.Contains(html, "linkedDisplayIds") {
		t.Errorf("JS missing linkedDisplayIds filter for Linked Events section (issue #490)")
	}
}

// TestStaticArchiveIndex_ArticleBodyUsesRenderLinkedText (issue #490)
// asserts the Article detail screen renders bodyHtml via the existing
// renderLinkedText helper (reuses the Person Record cross-link machinery).
func TestStaticArchiveIndex_ArticleBodyUsesRenderLinkedText(t *testing.T) {
	html := renderIndexForTest(t)

	// renderArticleDetail must call renderLinkedText for body HTML.
	if !strings.Contains(html, "renderLinkedText") {
		t.Errorf("renderLinkedText helper missing from index.html (issue #490)")
	}
	// The function must exist and be referenced by article rendering.
	if !strings.Contains(html, "function renderLinkedText") {
		t.Errorf("renderLinkedText function definition missing (issue #490)")
	}
}
