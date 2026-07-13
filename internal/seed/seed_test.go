package seed

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/testtemp"
)

func TestGenerateCreatesDatabaseRecordsAndImages(t *testing.T) {
	dataDir := testtemp.New(t).Path()

	summary, err := Generate(Options{
		DataDir:  dataDir,
		Soldiers: 12,
		Seed:     42,
		Reset:    true,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if summary.Soldiers != 12 {
		t.Fatalf("soldiers=%d want 12", summary.Soldiers)
	}
	if summary.Records < 12 {
		t.Fatalf("records=%d want at least 12", summary.Records)
	}
	if summary.Images < 12 {
		t.Fatalf("images=%d want at least 12", summary.Images)
	}

	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer database.Close()

	// Issue #447: events are also soldiers rows (entry_type='event').
	// Use WHERE entry_type = 'soldier' to count only soldier-type rows.
	assertCountWhere(t, database, "soldiers", "entry_type = 'soldier'", 12)
	assertCount(t, database, "records", summary.Records)
	assertCount(t, database, "images", summary.Images)

	// Issue #447: v58-v65 surface — assert non-zero counts on
	// the new entity tables when schema >= 58.
	if summary.Events > 0 {
		assertCountWhere(t, database, "soldiers", "entry_type = 'event'", summary.Events)
		assertCount(t, database, "event_person_links", summary.EventLinks)
		assertCount(t, database, "event_sources", summary.EventSources)
		assertCount(t, database, "articles", summary.Articles)
		assertCount(t, database, "article_refs", summary.ArticleRefs)
		assertCount(t, database, "tags", summary.Tags)
		assertCount(t, database, "person_record_tags", summary.PersonRecordTags)
		if summary.EventLinks == 0 {
			t.Fatalf("event_person_links should be > 0 on v58+ schema")
		}
		if summary.Articles == 0 {
			t.Fatalf("articles should be > 0 on v58+ schema")
		}
		if summary.Tags == 0 {
			t.Fatalf("tags should be > 0 on v58+ schema")
		}
	}

	var displayID, pensionID, applicationID, middleName, rankIn, rankOut, pensionState string
	if err := database.Conn().QueryRow("SELECT display_id, pension_id, application_id, middle_name, rank_in, rank_out, pension_state FROM soldiers ORDER BY id LIMIT 1").Scan(&displayID, &pensionID, &applicationID, &middleName, &rankIn, &rankOut, &pensionState); err != nil {
		t.Fatalf("select generated identifiers: %v", err)
	}
	if displayID == "" || pensionID == "" || applicationID == "" {
		t.Fatalf("expected generated identifiers, got display=%q pension=%q application=%q", displayID, pensionID, applicationID)
	}
	if middleName == "" || rankIn == "" || rankOut == "" || pensionState == "" {
		t.Fatalf("expected new identity fields, got middle=%q rankIn=%q rankOut=%q pensionState=%q", middleName, rankIn, rankOut, pensionState)
	}

	imageFiles := 0
	if err := filepath.WalkDir(filepath.Join(dataDir, "images"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			imageFiles++
		}
		return nil
	}); err != nil {
		t.Fatalf("WalkDir: %v", err)
	}
	if imageFiles != summary.Images {
		t.Fatalf("image files=%d want %d", imageFiles, summary.Images)
	}

	var storedPath string
	if err := database.Conn().QueryRow("SELECT file_path FROM images LIMIT 1").Scan(&storedPath); err != nil {
		t.Fatalf("select image path: %v", err)
	}
	if filepath.IsAbs(storedPath) {
		t.Fatalf("image path should be stored relative, got %q", storedPath)
	}
}

// TestSeedArticles_MarkdownFormat_RendersViaGoldmark (issue #523)
// asserts that --articles-format=markdown produces body_html via
// records.MarkdownRenderer — the same pipeline the Wails app uses on
// save. Pins the CommonMark feature coverage the renderer currently
// supports: headings h1-h6, bold/italic, lists, blockquote, inline +
// fenced code, links, images, paragraphs.
//
// Tables and `---` horizontal rules are in the corpus but the
// renderer uses goldmark.New() with no GFM extension (issue #524
// follow-up). When that lands, add <table>, <tbody>, <hr> to the
// required list below.
func TestSeedArticles_MarkdownFormat_RendersViaGoldmark(t *testing.T) {
	dataDir := testtemp.New(t).Path()

	summary, err := Generate(Options{
		DataDir:        dataDir,
		Soldiers:       12,
		Seed:           42,
		Reset:          true,
		SkipSoldiers:   false,
		Articles:       30, // exercise every corpus entry — at least one will hit each feature
		ArticlesFormat: ArticleBodyMarkdown,
	})
	if err != nil {
		t.Fatalf("Generate (markdown): %v", err)
	}
	if summary.Articles != 30 {
		t.Fatalf("articles=%d want 30", summary.Articles)
	}

	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer database.Close()

	rows, err := database.Conn().Query("SELECT body_html FROM articles")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	var allBodies strings.Builder
	for rows.Next() {
		var bodyHTML string
		if err := rows.Scan(&bodyHTML); err != nil {
			t.Fatalf("scan: %v", err)
		}
		allBodies.WriteString(bodyHTML)
		allBodies.WriteString("\n")
	}
	combined := allBodies.String()

	// CommonMark + image presence checks across the whole corpus.
	// Every check is a substring of the union of rendered bodies
	// so a missing feature trips the test even if other features
	// still render.
	required := []string{
		"<h1>",       // heading
		"<h2>",       // nested heading
		"<ul>",       // unordered list
		"<ol>",       // ordered list
		"<li>",       // list item
		"<strong>",   // bold
		"<em>",       // italic
		"<blockquote>", // blockquote
		"<code>",     // inline code
		"<pre>",      // fenced code block
		"<a href=",   // markdown link → anchor with href
		"<img ",      // image
		`alt="`,      // image alt attribute (preserved by bluemonday)
		"<p>",        // paragraph
	}
	for _, sub := range required {
		if !strings.Contains(combined, sub) {
			t.Errorf("union of all 30 markdown body_html missing %q\n--- snippet (first 500 chars) ---\n%s\n--------------------", sub, firstN(combined, 500))
		}
	}
}

// firstN returns the first n bytes of s as a string for test-failure
// diagnostics. Used to keep the error output bounded.
func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// TestSeedArticles_PlainFormat_PreservesLegacyPath (issue #523)
// regression net for the default --articles-format=plain path. The
// legacy #447 behavior must continue to write body_md as raw prose
// and body_html wrapped in <p>...</p>. Without this guard, a future
// refactor that flips the default to markdown would silently change
// every existing fixture.
func TestSeedArticles_PlainFormat_PreservesLegacyPath(t *testing.T) {
	dataDir := testtemp.New(t).Path()

	_, err := Generate(Options{
		DataDir:      dataDir,
		Soldiers:     12,
		Seed:         42,
		Reset:        true,
		SkipSoldiers: false,
		Articles:     1,
		// ArticlesFormat zero value = ArticleBodyPlain (legacy default).
	})
	if err != nil {
		t.Fatalf("Generate (plain): %v", err)
	}

	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer database.Close()

	var bodyMD, bodyHTML string
	if err := database.Conn().QueryRow("SELECT body_md, body_html FROM articles LIMIT 1").Scan(&bodyMD, &bodyHTML); err != nil {
		t.Fatalf("select: %v", err)
	}

	if !strings.Contains(bodyHTML, "<p>") || !strings.HasSuffix(strings.TrimSpace(bodyHTML), "</p>") {
		t.Errorf("body_html should be <p>-wrapped for plain format, got %q", bodyHTML)
	}
	if strings.Contains(bodyHTML, "<table>") || strings.Contains(bodyHTML, "<h1>") {
		t.Errorf("plain format body_html should not contain markdown-rendered tags, got %q", bodyHTML)
	}
	if bodyMD != bodyHTML[strings.Index(bodyHTML, ">")+1:strings.LastIndex(bodyHTML, "<")] {
		t.Errorf("plain format body_md should equal prose between <p> and </p>: md=%q html=%q", bodyMD, bodyHTML)
	}
}

func assertCount(t *testing.T, database *db.DB, table string, want int) {
	t.Helper()

	var got int
	if err := database.Conn().QueryRow("SELECT COUNT(*) FROM " + table).Scan(&got); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if got != want {
		t.Fatalf("%s count=%d want %d", table, got, want)
	}
}

func assertCountWhere(t *testing.T, database *db.DB, table, where string, want int) {
	t.Helper()

	var got int
	if err := database.Conn().QueryRow("SELECT COUNT(*) FROM " + table + " WHERE " + where).Scan(&got); err != nil {
		t.Fatalf("count %s WHERE %s: %v", table, where, err)
	}
	if got != want {
		t.Fatalf("%s WHERE %s count=%d want %d", table, where, got, want)
	}
}
