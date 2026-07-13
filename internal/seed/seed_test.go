package seed

import (
	"os"
	"path/filepath"
	"strconv"
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
// save. Pins the feature coverage the renderer currently supports:
// CommonMark (headings, lists, blockquote, inline + fenced code,
// links, images, paragraphs) + GFM (tables, autolinks, strikethrough,
// task lists).
//
// The GFM extension is enabled via goldmark.WithExtensions in
// internal/records/markdown.go (issue #525). The corpus (#523)
// exercises each feature across 30 entries; at least one entry
// exercises each feature so the union of all rendered body_html
// contains every required substring.
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

	// CommonMark + GFM presence checks across the whole corpus.
	// Every check is a substring of the union of rendered bodies
	// so a missing feature trips the test even if other features
	// still render. GFM-specific assertions added per issue #525.
	// Note: the corpus (#523) doesn't exercise GFM task lists
	// (`- [ ]` / `- [x]`) yet — that fixture expansion is tracked
	// in issue #525's secondary sub-issue. Renderer support is
	// landed; the corpus will catch up in a follow-up.
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
		"<table>",    // GFM table (issue #525)
		"<thead>",    // GFM table head
		"<tbody>",    // GFM table body
		"<th>",       // GFM table header cell
		"<td>",       // GFM table data cell
		"<del>",      // GFM strikethrough (~~text~~)
		"<hr>",       // GFM horizontal rule (--- syntax)
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

// TestSeed_AdditiveReRunDoesNotCollideOnEventOrArticleDisplayID
// (issue #537) is the regression net for the EVT-01000N / ART-01000N
// hard-coded prefix bug. Two passes back-to-back:
//   - Pass 1: --reset + 5 soldiers + 3 events + 2 articles (uses the
//     legacy default count encoding but with explicit non-zero values
//     so the test is deterministic).
//   - Pass 2: --skip-soldiers + 3 events + 1 article, NO --reset. This
//     is the additive shape that used to fail on
//     `UNIQUE constraint failed: soldiers.display_id`.
//
// Success: pass 2 writes the additional rows without error and the
// EVT-/ART- display_id sequences advance past their pass-1 values
// (no collision, no silent overwrite).
func TestSeed_AdditiveReRunDoesNotCollideOnEventOrArticleDisplayID(t *testing.T) {
	dataDir := testtemp.New(t).Path()

	// Pass 1: full reset, seed the v58-v65 surface.
	summary1, err := Generate(Options{
		DataDir:    dataDir,
		Soldiers:   5,
		Seed:       7,
		Reset:      true,
		Events:     3,
		Articles:   2,
	})
	if err != nil {
		t.Fatalf("pass 1 Generate: %v", err)
	}
	if summary1.Events != 3 || summary1.Articles != 2 {
		t.Fatalf("pass 1 counts: events=%d articles=%d", summary1.Events, summary1.Articles)
	}

	// Capture the pass-1 max sequences so pass 2 can assert it advanced.
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer database.Close()

	maxEventSeqPass1, err := maxSequenceFor(database, "soldiers", "display_id LIKE 'EVT-%'")
	if err != nil {
		t.Fatalf("max event seq pass 1: %v", err)
	}
	if maxEventSeqPass1 < 3 {
		t.Fatalf("pass 1 max EVT sequence=%d, expected >=3", maxEventSeqPass1)
	}
	maxArticleSeqPass1, err := maxSequenceFor(database, "articles", "display_id LIKE 'ART-%'")
	if err != nil {
		t.Fatalf("max article seq pass 1: %v", err)
	}
	if maxArticleSeqPass1 < 2 {
		t.Fatalf("pass 1 max ART sequence=%d, expected >=2", maxArticleSeqPass1)
	}

	// Pass 2: skip-soldiers, additive — the call that used to crash
	// with `UNIQUE constraint failed: soldiers.display_id` on EVT-010000.
	summary2, err := Generate(Options{
		DataDir:      dataDir,
		SkipSoldiers: true,
		Events:       3,
		Articles:     1,
	})
	if err != nil {
		t.Fatalf("pass 2 Generate: %v", err)
	}
	if summary2.Events != 3 || summary2.Articles != 1 {
		t.Fatalf("pass 2 counts: events=%d articles=%d", summary2.Events, summary2.Articles)
	}

	// Confirm the sequences advanced past pass 1 (no overwrite, no
	// collision). The pass-2 max must be strictly greater than the
	// pass-1 max for both EVT- and ART- namespaces.
	maxEventSeqPass2, err := maxSequenceFor(database, "soldiers", "display_id LIKE 'EVT-%'")
	if err != nil {
		t.Fatalf("max event seq pass 2: %v", err)
	}
	if maxEventSeqPass2 <= maxEventSeqPass1 {
		t.Fatalf("pass 2 max EVT sequence=%d did not advance past pass 1 max=%d", maxEventSeqPass2, maxEventSeqPass1)
	}
	maxArticleSeqPass2, err := maxSequenceFor(database, "articles", "display_id LIKE 'ART-%'")
	if err != nil {
		t.Fatalf("max article seq pass 2: %v", err)
	}
	if maxArticleSeqPass2 <= maxArticleSeqPass1 {
		t.Fatalf("pass 2 max ART sequence=%d did not advance past pass 1 max=%d", maxArticleSeqPass2, maxArticleSeqPass1)
	}
}

// maxSequenceFor extracts the trailing integer from every display_id
// in the given table matching the optional where clause, and returns
// the largest one. Used by the #537 regression test to verify the
// EVT-/ART- counters advance across additive seed passes.
func maxSequenceFor(database *db.DB, table, where string) (int, error) {
	q := "SELECT display_id FROM " + table
	if where != "" {
		q += " WHERE " + where
	}
	rows, err := database.Conn().Query(q)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	max := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		seq := trailingInt(id)
		if seq > max {
			max = seq
		}
	}
	return max, rows.Err()
}

// trailingInt parses the trailing decimal run from a display_id like
// `EVT-00012` or `ART-00007`. Returns 0 if no trailing run is found.
func trailingInt(s string) int {
	i := len(s)
	for i > 0 && s[i-1] >= '0' && s[i-1] <= '9' {
		i--
	}
	if i == len(s) {
		return 0
	}
	n, err := strconv.Atoi(s[i:])
	if err != nil {
		return 0
	}
	return n
}
