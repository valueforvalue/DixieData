// article_service.go covers the slice-1 Article CRUD (issue #321).
// The minimal service here is the tracer bullet: Create + GetByID
// only. Slice 2 will fill in Update + Delete + ResolveRefs + the
// ref-row helpers; slice 2.5 will fill in Snapshot/Restore/Delete.
//
// Design choices mirroring event_service.go:228-249 (CreateEvent):
//   * Title is required and trimmed; the slice-1 Create handler
//     rejects blank title with 400.
//   * BodyMD is the markdown source as supplied; the slice-1
//     Create path stores it verbatim in BOTH body_md AND body_html
//     so the /articles/{id} detail page renders without a
//     markdown renderer. Slice 2 will replace the body_html
//     column-write with a goldmark + bluemonday pass.
//   * DisplayID is auto-minted by db.NextArticleID (the ART- scope
//     per locked decision #4). If the caller supplied a DisplayID
//     (the slice-2 Update path may), we use it as-is; else we mint.
//   * UpdatedAt == CreatedAt at create time; Update path
//     stamps UpdatedAt on every write.
package records

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/db/repo"
	sqliterepo "github.com/valueforvalue/DixieData/internal/db/repo/sqlite"
	"github.com/valueforvalue/DixieData/internal/debug"
	"github.com/valueforvalue/DixieData/internal/models"
)

// ErrArticleTitleRequired is returned by Create when the title
// is blank after trim. The slice-1 create handler maps this to
// a 400; future slices can carry the same error if the front
// end calls ArticleService directly.
var ErrArticleTitleRequired = errors.New("article title is required")

// ErrArticleNotFound is returned by GetByID when no row exists.
var ErrArticleNotFound = errors.New("article not found")

// ArticleService is the slice-1 CRUD service for Article rows.
// The DB access is borrowed from the SoldierService (the same
// pattern EventService uses at event_service.go:54-64); that
// keeps Article CRUD on the same SQLite connection without
// inventing a per-service DB accessor.
type ArticleService struct {
	soldiers    *SoldierService
	articleRepo repo.ArticleRecordRepo
	renderer    *MarkdownRenderer
	registry    ArticleRegistry

	defaultPageSize int
	maxPageSize     int
}

// SetPageSizes configures the default and maximum page sizes for
// the article list view (issue #639). Call during service wiring.
func (a *ArticleService) SetPageSizes(defaultSize, maxSize int) {
	a.defaultPageSize = defaultSize
	a.maxPageSize = maxSize
}

// SetMarkdownRenderer swaps the markdown renderer after
// construction (issue #660 amendment #2). Used by the appshell
// to thread a renderer with the configured ThemeConfig into the
// service after NewArticleService has already returned (so
// reloadServices can apply a fresh theme on every launch
// without rebuilding the service).
func (a *ArticleService) SetMarkdownRenderer(r *MarkdownRenderer) {
	a.renderer = r
}
// ArticleRegistry uses a renderer to pre-render the article's PDF. The contract is a
// subset of *render.Registry; the Render method takes a
// record-type string + data map + writer and the impl
// resolves the template + PrintSettings internally.
//
// The interface lives in internal/records (not pkg/render)
// to avoid an import cycle: pkg/render depends on
// internal/records, so internal/records cannot import
// pkg/render.
type ArticleRegistry interface {
	RenderArticle(ctx context.Context, recordType string, orientation string, data map[string]any, w io.Writer) error
}

// SetArticleRegistry wires the typst-backed Registry into
// the ArticleService. Called once at startup; nil clears
// the wiring.
func (a *ArticleService) SetArticleRegistry(reg ArticleRegistry) {
	a.registry = reg
}

// NewArticleService constructs the slice-1 ArticleService.
// soldierService is borrowed (not copied) so the Article
// service shares the same DB connection pool + migrations +
// PRAGMA state that Person Record + Event Record paths use.
// Slice 2 may split this seam if the test suite outgrows the
// shared pool; for v1 the shared DB connection is the right
// trade-off.
//
// The optional markdownRenderer is consulted on Create +
// Update so the body_html column carries a sanitized render
// rather than the slice-1 verbatim copy. slice-1 callers
// (the slice-1 service tests) pass nil to keep the
// verbatim-md path active for backwards compatibility with
// the slice-1 RED test contract.
func NewArticleService(soldierService *SoldierService, markdownRenderer ...*MarkdownRenderer) *ArticleService {
	svc := &ArticleService{
		soldiers:    soldierService,
		articleRepo: sqliterepo.NewArticleRecordRepo(soldierService.db),
	}
	if len(markdownRenderer) > 0 && markdownRenderer[0] != nil {
		svc.renderer = markdownRenderer[0]
	}
	return svc
}

// Count returns the total number of LIVE Article rows in the
// Local Archive. Snapshots (is_snapshot = 1) are excluded —
// they're per-Article Revisions and don't count as distinct
// archive entries (locked decision #11). Powers the /inventory
// page + the Calendar header archive rollup (issue #491).
func (a *ArticleService) Count() (int, error) {
	var n int
	if err := a.soldiers.db.Conn().QueryRow(
		`SELECT COUNT(*) FROM articles WHERE is_snapshot = 0`,
	).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// Create inserts a new Article row (live branch — SnapshotOfID
// is nil, IsSnapshot is false) and returns the persisted model.
// Returns ErrArticleTitleRequired when title is blank after
// trim; passes other errors through unchanged.
//
// The created row's DisplayID is auto-minted via db.NextArticleID
// (ART-NNNNN); the slice-2 Update path is the only place that
// writes a caller-supplied DisplayID.
func (a *ArticleService) Create(article models.Article) (*models.Article, error) {
	title := strings.TrimSpace(article.Title)
	if title == "" {
		return nil, ErrArticleTitleRequired
	}
	subtitle := strings.TrimSpace(article.Subtitle)
	bodyMD := article.BodyMD
	bodyHTML := a.renderBodyHTML(article.BodyMD) // slice 1: render=verbatim; slice 2 swap in goldmark+bluemonday.

	if article.SyncID == "" {
		article.SyncID = uuid.NewString()
	}
	if article.DisplayID == "" {
		id, err := a.soldiers.db.NextArticleID()
		if err != nil {
			return nil, fmt.Errorf("NextArticleID: %w", err)
		}
		article.DisplayID = id
	}
	now := time.Now().UTC().Format(time.RFC3339)
	article.Title = title
	article.Subtitle = subtitle
	article.BodyMD = bodyMD
	article.BodyHTML = bodyHTML
	article.CreatedAt = now
	article.UpdatedAt = now
	article.IsSnapshot = false
	article.SnapshotOfID = nil

	// Slice 4 of issue #613: the INSERT goes through the
	// ArticleRecordRepo seam. Pre-INSERT normalization (title
	// trim, sync_id mint, display_id mint, render body HTML,
	// timestamp stamping) stays in this service layer.
	id, err := a.articleRepo.Create(context.Background(), a.soldiers.db.Conn(), article)
	if err != nil {
		return nil, fmt.Errorf("insert article: %w", err)
	}
	article.ID = id
	return &article, nil
}

// GetByID returns the Article row with the given SQLite row id.
// Returns ErrArticleNotFound when no row matches; passes other
// errors through unchanged.
//
// Snapshot rows are filtered out (is_snapshot = 0 only) because
// the slice-1 detail page is the live article's page; slice 2's
// full read path adds a helper that returns a snapshot row when
// the URL carries the snapshot-of id. The filter also keeps
// "save copy" rows invisible from the live list until slice 2.5
// adds the Snapshot lifecycle.
//
// Slice 4 of issue #613: the SELECT goes through the
// ArticleRecordRepo seam. The legacy "is_snapshot = 0" filter
// (live-only; snapshots are excluded so the detail page
// reads cleanly) is preserved — the service fetches via the
// repo's plain GetByID, then verifies IsSnapshot at the
// model layer. The legacy behavior "returns ErrArticleNotFound
// when the row exists but is_snapshot = 1" is preserved
// because the post-fetch IsSnapshot check rejects it.
func (a *ArticleService) GetByID(id int64) (*models.Article, error) {
	if id < 1 {
		return nil, ErrArticleNotFound
	}
	row, err := a.articleRepo.GetByID(context.Background(), id)
	if err != nil {
		return nil, err
	}
	var (
		art          models.Article
		snapshotOfID sql.NullInt64
		isSnapshot   int
	)
	if err := row.Scan(
		&art.ID, &art.SyncID, &art.DisplayID, &art.Title, &art.Subtitle,
		&art.BodyMD, &art.BodyHTML, &art.CreatedAt, &art.UpdatedAt,
		&snapshotOfID, &isSnapshot,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrArticleNotFound
		}
		return nil, fmt.Errorf("scan article %d: %w", id, err)
	}
	if snapshotOfID.Valid {
		v := snapshotOfID.Int64
		art.SnapshotOfID = &v
	}
	art.IsSnapshot = isSnapshot != 0
	if art.IsSnapshot {
		return nil, ErrArticleNotFound
	}
	return &art, nil
}

// slice 2 additions (issue #321). The slice-1 file ended after
// GetByID; this chunk adds the missing CRUD surface
// (List + GetByDisplayID + Update + Delete) + the ref-row
// helpers (ScanRefs + AttachRef + DetachRef) + the
// markdown-token parser (ResolveRefs). Slice 2 ships all of
// them in one go so the slice-1 RED test (TestHandleArticleCRUD_RoundTrip)
// can grow into a slice-2 service test that exercises Update +
// Delete + ref attach + detach.
//
// Per the slice-2 scope clause: ref resolution is the
// markdown-token parser; ref attach/detach is the
// article_refs CRUD. Both are required for slice 3's picker
// modal + the Cite-in reverse-lookup to be functional.

// ArticleRef is a per-row payload of the article_refs
// junction table (issue #321 slice 2). One Article can
// cite N Person Records. The row carries the article id +
// the person id + a denormalized display_id cache so the
// Cite-in reverse-lookup + the PDF / Static-HTML resolver
// can render without a join to the soldiers table.
//
// ArticleID + PersonRecordID are non-null FKs. SyncID
// mirrors (ArticleSyncID + PersonRecordSyncID) travel with
// the row for distributed-merge parity with the soldier-side
// junction tables.
//
// Position is reserved for a future "reorder refs" surface
// mirroring #368's source-record reorder. Default is 0 today;
// slice 3+ may surface a reorder UI.
type ArticleRef struct {
	ID               int64
	ArticleID        int64
	ArticleSyncID    string
	PersonRecordID   int64
	PersonRecordSyncID string
	PersonDisplayID  string
	Position         int
}

// ResolvedRef is a parse-side projection of an in-body
// #person/D-00123 token (issue #321 slice 2 + locked
// decision #6). ArticleID + ArticleSyncID identify the
// owning article. Token is the verbatim display id token
// from the markdown source (e.g. "D-00123") so the
// renderer can quote it back. PersonRecordID + PersonDisplayID
// are the resolved values (populated by looking up Token in
// the soldiers table). Resolved is false when the token
// references a Display ID that does not exist; the renderer
// emits "⚠ [Unknown: D-00123]" per the slice-2 + locked-
// decision #6 fail-loud contract.
type ResolvedRef struct {
	ArticleID         int64
	ArticleSyncID     string
	Token             string
	PersonRecordID    int64
	PersonDisplayID   string
	Resolved          bool
}

// List returns the slice-1+ live-branch articles sorted
// updated_at desc (most-recently-edited first), paginated
// by page (1-indexed) + pageSize (clamped to [1, 100]).
// The total count is returned so the /articles list view
// can render a paginator. Snapshot rows are excluded.
//
// Slice 4 of issue #613: the COUNT + paginated SELECT go
// through the ArticleRecordRepo seam. The legacy
// `is_snapshot = 0` filter is preserved by adding the
// repo's raw count + total; the post-fetch IsSnapshot
// check rejects snapshots from the result slice.
func (a *ArticleService) List(page, pageSize int) ([]models.Article, int, error) {
	if page < 1 {
		page = 1
	}
	defSz := a.defaultPageSize
	if defSz <= 0 {
		defSz = 25
	}
	maxSz := a.maxPageSize
	if maxSz <= 0 {
		maxSz = 100
	}
	if pageSize < 1 {
		pageSize = defSz
	}
	if pageSize > maxSz {
		pageSize = maxSz
	}
	rows, total, err := a.articleRepo.List(context.Background(), page, pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("list articles: %w", err)
	}
	defer debug.DeferCloseLog(rows, "List.rows")
	out := make([]models.Article, 0, pageSize)
	for rows.Next() {
		var (
			art          models.Article
			snapshotOfID sql.NullInt64
			isSnapshot   int
		)
		if err := rows.Scan(
			&art.ID, &art.SyncID, &art.DisplayID, &art.Title, &art.Subtitle,
			&art.BodyMD, &art.BodyHTML, &art.CreatedAt, &art.UpdatedAt,
			&snapshotOfID, &isSnapshot,
		); err != nil {
			return nil, 0, fmt.Errorf("scan article row: %w", err)
		}
		if snapshotOfID.Valid {
			v := snapshotOfID.Int64
			art.SnapshotOfID = &v
		}
		art.IsSnapshot = isSnapshot != 0
		out = append(out, art)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// GetByDisplayID mirrors SoldierService.GetByDisplayID: case-
// insensitive lookup by the ART- prefix; returns
// ErrArticleNotFound when no row matches. Snapshot rows are
// filtered out so a Display ID lookup always returns the
// live-branch article.
func (a *ArticleService) GetByDisplayID(displayID string) (*models.Article, error) {
	trimmed := strings.TrimSpace(displayID)
	if trimmed == "" {
		return nil, ErrArticleNotFound
	}
	row := a.soldiers.db.Conn().QueryRow(
		`SELECT id, sync_id, display_id, title, subtitle, body_md, body_html,
		        created_at, updated_at, snapshot_of_id, is_snapshot
		 FROM articles WHERE upper(display_id) = upper(?) AND is_snapshot = 0`, trimmed)
	var (
		art          models.Article
		snapshotOfID sql.NullInt64
		isSnapshot   int
	)
	if err := row.Scan(
		&art.ID, &art.SyncID, &art.DisplayID, &art.Title, &art.Subtitle,
		&art.BodyMD, &art.BodyHTML, &art.CreatedAt, &art.UpdatedAt,
		&snapshotOfID, &isSnapshot,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrArticleNotFound
			}
		return nil, fmt.Errorf("scan article by display id %q: %w", trimmed, err)
	}
	if snapshotOfID.Valid {
		v := snapshotOfID.Int64
		art.SnapshotOfID = &v
	}
	art.IsSnapshot = isSnapshot != 0
	return &art, nil
}

// Update mutates the mutable slice-1 surface of an Article:
// Title + Subtitle + BodyMD + BodyHTML. CreatedAt is left
// alone; UpdatedAt is stamped to the current RFC3339 time.
// Snapshot rows are rejected (a snapshot is read-only).
// Returns ErrArticleNotFound when the row does not exist;
// returns ErrArticleTitleRequired when the new title is blank;
// returns ErrArticleSnapshot when the target is a snapshot row.
func (a *ArticleService) Update(article models.Article) error {
	title := strings.TrimSpace(article.Title)
	if title == "" {
		return ErrArticleTitleRequired
	}
	subtitle := strings.TrimSpace(article.Subtitle)
	bodyMD := article.BodyMD
	bodyHTML := a.renderBodyHTML(article.BodyMD) // slice 2 still stores verbatim; slice 2.5 swaps in goldmark+bluemonday
	now := time.Now().UTC().Format(time.RFC3339)
	article.Title = title
	article.Subtitle = subtitle
	article.BodyMD = bodyMD
	article.BodyHTML = bodyHTML
	article.UpdatedAt = now

	// Slice 4 of issue #613: the UPDATE goes through the
	// ArticleRecordRepo seam. The pre-UPDATE normalization
	// (title trim, body render, timestamp) stays in this
	// service. The legacy `is_snapshot = 0` filter is
	// preserved inside the repo's Update.
	n, err := a.articleRepo.Update(context.Background(), a.soldiers.db.Conn(), article)
	if err != nil {
		return fmt.Errorf("update article %d: %w", article.ID, err)
	}
	if n == 0 {
		// row not found OR is_snapshot = 1.
		// disambiguate so the handler maps the snapshot case to 409
		// and the not-found case to 404.
		var isSnapshot int
		if err := a.soldiers.db.Conn().QueryRow(`SELECT is_snapshot FROM articles WHERE id = ?`, article.ID).Scan(&isSnapshot); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrArticleNotFound
			}
			return fmt.Errorf("scan article %d: %w", article.ID, err)
		}
		if isSnapshot != 0 {
			return ErrArticleSnapshot
		}
		return ErrArticleNotFound
	}
	return nil
}

// Delete removes an Article row by id. article_refs rows
// cascade via the FK ON DELETE CASCADE constraint so the
// caller does not need to detach refs manually. Snapshot
// rows are rejected (use DeleteSnapshot from the slice-2.5
// lifecycle surface instead). Returns ErrArticleNotFound
// when the id does not exist; returns ErrArticleSnapshot
// when the target is a snapshot.
func (a *ArticleService) Delete(id int64) error {
	// Issue #666: cascade-delete the snapshot rows first.
	// Snapshots live in the same `articles` table with
	// snapshot_of_id set; there is no FK from snapshot to
	// parent so we must clean them up explicitly. Without
	// this, deleting an article leaves orphaned snapshot
	// rows pointing at a non-existent parent id, and the
	// "Revisions" tab on a different article would render
	// ghost entries in the per-article-snapshot list if
	// the user's query joined on the deleted id.
	conn := a.soldiers.db.Conn()
	if _, err := conn.Exec(`DELETE FROM articles WHERE snapshot_of_id = ?`, id); err != nil {
		return fmt.Errorf("delete article %d snapshots: %w", id, err)
	}
	// Slice 4 of issue #613: the DELETE goes through the
	// ArticleRecordRepo seam. The legacy `is_snapshot = 0`
	// filter is preserved inside the repo's Delete.
	n, err := a.articleRepo.Delete(context.Background(), a.soldiers.db.Conn(), id)
	if err != nil {
		return fmt.Errorf("delete article %d: %w", id, err)
	}
	if n == 0 {
		var isSnapshot int
		if err := a.soldiers.db.Conn().QueryRow(`SELECT is_snapshot FROM articles WHERE id = ?`, id).Scan(&isSnapshot); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrArticleNotFound
			}
			return fmt.Errorf("scan article %d: %w", id, err)
		}
		if isSnapshot != 0 {
			return ErrArticleSnapshot
		}
		return ErrArticleNotFound
	}
	return nil
}

// AddImage attaches an image to the given article. The caller
// has already saved the file under dataDir/images/articles/<displayID>/
// (via appdata.ArticleImageDir) + computed the relative path
// (e.g. "images/articles/ART-00001/portrait-1.jpg"). This
// method only inserts the metadata row + links it to the
// article via the (article_id, kind='article') filter that
// the picker modal reads.
//
// Issue #612 slice 2: the kind='article' discriminator + the
// per-article file_path prefix are the slice-1 schema change
// doing the actual work. Without them, an article picker
// would either see every image in the archive (a UX disaster)
// or the per-Person-Record queries would have to be retrofitted
// with a NOT-EXISTS clause (a perf disaster). The single
// WHERE article_id = ? AND kind = 'article' filter is
// O(log n) thanks to the slice-1 index.
func (a *ArticleService) AddImage(articleID int64, fileName, filePath, caption string) error {
	imageSyncID, err := db.NewSyncID()
	if err != nil {
		return err
	}
	// Use sql.NullInt64 for the article_id so the column
	// (which is nullable at the schema level for the legacy
	// person_record_id path) maps cleanly.
	var nid sql.NullInt64
	if articleID > 0 {
		nid = sql.NullInt64{Int64: articleID, Valid: true}
	}
	_, err = a.soldiers.db.Conn().Exec(
		`INSERT INTO images (sync_id, article_id, kind, file_name, file_path, caption) VALUES (?, ?, 'article', ?, ?, ?)`,
		imageSyncID,
		nid,
		fileName,
		filePath,
		caption,
	)
	return err
}

// ImagesForArticle returns the images attached to the given
// article (kind='article'), ordered by is_primary DESC + id
// (matches the per-Person-Record gallery sort). The picker
// modal reads this query directly to populate the "Pick
// existing" tab; the upload-tab path inserts via AddImage.
//
// The kind='article' filter is the slice-1 discriminator
// doing the work: a Soldier's portrait never leaks into
// the article picker, even though both rows live in the
// same `images` table. The reverse (a chapter illustration
// in the soldier gallery) is blocked by the legacy
// per-Person-Record query's implicit person_record_id IS
// NOT NULL filter + the same kind='person' filter the
// slice-3 picker will add for symmetry.
func (a *ArticleService) ImagesForArticle(articleID int64) ([]models.Image, error) {
	rows, err := a.soldiers.db.Conn().Query(
		`SELECT id, sync_id, person_record_id, person_sync_id, article_id, kind, file_name, file_path, caption, is_primary FROM images WHERE article_id = ? AND kind = 'article' ORDER BY is_primary DESC, id`,
		articleID,
	)
	if err != nil {
		return nil, err
	}
	defer debug.DeferCloseLog(rows, "ArticleService.ImagesForArticle.rows")
	var out []models.Image
	for rows.Next() {
		var img models.Image
		var personID sql.NullInt64
		var personSyncID sql.NullString
		var articleID sql.NullInt64
		if err := rows.Scan(&img.ID, &img.SyncID, &personID, &personSyncID, &articleID, &img.Kind, &img.FileName, &img.FilePath, &img.Caption, &img.IsPrimary); err != nil {
			return nil, err
		}
		if articleID.Valid {
			id := articleID.Int64
			img.ArticleID = &id
		}
		if personID.Valid {
			img.PersonRecordID = personID.Int64
		}
		if personSyncID.Valid {
			img.PersonSyncID = personSyncID.String
		}
		out = append(out, img)
	}
	return out, rows.Err()
}

// GetSnapshotByID returns the snapshot row with the given
// SQLite row id (is_snapshot = 1). Used by the slice-2.5
// Revisions-tab list + the DeleteSnapshot handler. The
// regular GetByID filters snapshots out; this method
// inverts the filter so callers that own snapshot-row
// surface area can find them. Returns ErrArticleNotFound
// when the row does not exist OR is a live-branch row.

// ListSnapshots returns every snapshot row pointing at the
// given live article id, sorted by created_at DESC (most-
// recently-snapshotted first; matches the slice-2 List
// pagination order so the Revisions tab feels consistent).
//
// The slice-3 Revisions tab uses this list to render the
// per-snapshot row with Restore + Delete affordances. Empty
// input is rejected so a confused caller (e.g. a future
// caller that lost the articleID context) fails fast; the
// articleID comes from the URL path in the slice-3 handler
// so a zero value would mean a route registration bug.
//
// Returns an empty slice (not nil) when the article has no
// snapshots -- the templ loop renders cleanly without a nil
// guard. The empty slice + a nil error together signal "no
// revisions yet" without distinguishing that from "lookup
// failed" (which returns an error).
func (a *ArticleService) ListSnapshots(articleID int64) ([]models.Article, error) {
	if articleID < 1 {
		return nil, fmt.Errorf("ListSnapshots: article id must be positive")
	}
	rows, err := a.soldiers.db.Conn().Query(
		`SELECT id, sync_id, display_id, title, subtitle, body_md, body_html,
		        created_at, updated_at, snapshot_of_id, is_snapshot
		 FROM articles
		 WHERE snapshot_of_id = ? AND is_snapshot = 1
		 ORDER BY created_at DESC, id DESC`,
		articleID,
	)
	if err != nil {
		return nil, fmt.Errorf("ListSnapshots %d: %w", articleID, err)
	}
	defer debug.DeferCloseLog(rows, "ListSnapshots.rows")
	out := make([]models.Article, 0)
	for rows.Next() {
		var (
			art          models.Article
			snapshotOfID sql.NullInt64
			isSnapshot   int
		)
		if err := rows.Scan(
			&art.ID, &art.SyncID, &art.DisplayID, &art.Title, &art.Subtitle,
			&art.BodyMD, &art.BodyHTML, &art.CreatedAt, &art.UpdatedAt,
			&snapshotOfID, &isSnapshot,
		); err != nil {
			return nil, fmt.Errorf("ListSnapshots scan: %w", err)
		}
		if snapshotOfID.Valid {
			v := snapshotOfID.Int64
			art.SnapshotOfID = &v
		}
		art.IsSnapshot = isSnapshot != 0
		out = append(out, art)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ListSnapshots rows: %w", err)
	}
	return out, nil
}

func (a *ArticleService) GetSnapshotByID(id int64) (*models.Article, error) {
	if id < 1 {
		return nil, ErrArticleNotFound
	}
	row := a.soldiers.db.Conn().QueryRow(
		`SELECT id, sync_id, display_id, title, subtitle, body_md, body_html,
		        created_at, updated_at, snapshot_of_id, is_snapshot
		 FROM articles WHERE id = ? AND is_snapshot = 1`, id)
	var (
		art          models.Article
		snapshotOfID sql.NullInt64
		isSnapshot   int
	)
	if err := row.Scan(
		&art.ID, &art.SyncID, &art.DisplayID, &art.Title, &art.Subtitle,
		&art.BodyMD, &art.BodyHTML, &art.CreatedAt, &art.UpdatedAt,
		&snapshotOfID, &isSnapshot,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrArticleNotFound
		}
		return nil, fmt.Errorf("scan snapshot %d: %w", id, err)
	}
	if snapshotOfID.Valid {
		v := snapshotOfID.Int64
		art.SnapshotOfID = &v
	}
	art.IsSnapshot = isSnapshot != 0
	return &art, nil
}

// ScanRefs returns every article_refs row attached to the
// given article, ordered by id (the live insertion order).
// Used by slice 3's Refs panel on the detail page and by
// the slice-4 PDF / Static-HTML renderer to enumerate the
// cites-in-this-article list.
func (a *ArticleService) ScanRefs(articleID int64) ([]ArticleRef, error) {
	rows, err := a.soldiers.db.Conn().Query(
		`SELECT id, article_id, article_sync_id, person_record_id,
		        person_record_sync_id, person_display_id, position
		 FROM article_refs WHERE article_id = ?
		 ORDER BY id`, articleID)
	if err != nil {
		return nil, fmt.Errorf("scan refs for article %d: %w", articleID, err)
	}
	defer debug.DeferCloseLog(rows, "ScanRefs.rows")
	out := make([]ArticleRef, 0)
	for rows.Next() {
		var r ArticleRef
		if err := rows.Scan(&r.ID, &r.ArticleID, &r.ArticleSyncID,
			&r.PersonRecordID, &r.PersonRecordSyncID,
			&r.PersonDisplayID, &r.Position); err != nil {
			return nil, fmt.Errorf("scan ref row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// AttachRef attaches a Person Record to an Article by id.
// The person_record_id is verified against the soldiers
// table (FK enforces it; we add a friendly error here so
// the handler can map an unknown display id to 404 rather
// than a raw SQLITE_CONSTRAINT failure). Returns the new
// article_refs row id, or ErrArticleNotFound if the article
// is missing, or ErrRefPersonNotFound if the person row is
// missing.
func (a *ArticleService) AttachRef(articleID, personRecordID int64) (int64, error) {
	if articleID < 1 {
		return 0, ErrArticleNotFound
	}
	if personRecordID < 1 {
		return 0, ErrRefPersonNotFound
	}
	// Lookup article (GetByID rejects snapshots, which is
	// what we want -- snapshots are read-only).
	if _, err := a.GetByID(articleID); err != nil {
		return 0, err
	}
	// Lookup the soldier row to derive the sync_id mirror
	// + the denormalized display_id cache.
	var (
		syncID     string
		personDisp string
	)
	if err := a.soldiers.db.Conn().QueryRow(
		`SELECT sync_id, display_id FROM soldiers WHERE id = ?`, personRecordID).
		Scan(&syncID, &personDisp); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrRefPersonNotFound
		}
		return 0, fmt.Errorf("lookup person %d: %w", personRecordID, err)
	}
	// INSERT OR IGNORE on (article_id, person_record_id) so
	// a duplicate attach is a no-op (returns -1 from
	// RowsAffected in that case; the caller can detect the
	// "already attached" path by checking the returned id
	// against a pre-existing lookup if needed).
	var articleSyncID string
	if err := a.soldiers.db.Conn().QueryRow(`SELECT sync_id FROM articles WHERE id = ?`, articleID).Scan(&articleSyncID); err != nil {
		return 0, fmt.Errorf("lookup article sync_id %d: %w", articleID, err)
	}
	res, err := a.soldiers.db.Conn().Exec(
		`INSERT OR IGNORE INTO article_refs
		  (article_id, article_sync_id, person_record_id, person_record_sync_id,
		   person_display_id, position)
		 VALUES (?, ?, ?, ?, ?, 0)`,
		articleID, articleSyncID, personRecordID, syncID, personDisp)
	if err != nil {
		return 0, fmt.Errorf("insert article_ref: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("attach ref rows: %w", err)
	}
	if n == 1 {
		id, err := res.LastInsertId()
		if err != nil {
			return 0, fmt.Errorf("attach ref LastInsertId: %w", err)
		}
		return id, nil
	}
	// Already attached (the unique index
	// idx_article_refs_article_person made INSERT OR IGNORE
	// a no-op). Look up the existing row id so callers can
	// detect that the attach was a no-op.
	var existingID int64
	if err := a.soldiers.db.Conn().QueryRow(
		`SELECT id FROM article_refs
		 WHERE article_id = ? AND person_record_id = ?`,
		articleID, personRecordID).Scan(&existingID); err != nil {
		return 0, fmt.Errorf("lookup existing ref: %w", err)
	}
	return existingID, nil
}

// DetachRef removes the (article_id, person_record_id)
// article_refs row. No-op if the row does not exist (the
// handler then returns 204 to match REST idempotency).
// Returns ErrArticleNotFound if the article itself is missing.
func (a *ArticleService) DetachRef(articleID, personRecordID int64) error {
	if articleID < 1 {
		return ErrArticleNotFound
	}
	if personRecordID < 1 {
		return ErrRefPersonNotFound
	}
	if _, err := a.GetByID(articleID); err != nil {
		return err
	}
	_, err := a.soldiers.db.Conn().Exec(
		`DELETE FROM article_refs
		 WHERE article_id = ? AND person_record_id = ?`,
		articleID, personRecordID)
	return err
}

// ResolveRefs parses the article body's markdown links
// matching the #person/D-00123 custom-scheme pattern
// (issue #321 locked decision #5), looks each one up in
// the soldiers table, and returns the resolved-set. Unknown
// Display IDs are returned with Resolved=false (fail-loud;
// locked decision #6 -- the PDF / Static-HTML renderer
// emits "⚠ [Unknown: D-00123]" for unresolved tokens).
//
// The parser is intentionally narrow: it only matches a
// literal `[...](#person/...)` link inside the markdown
// source. Inline text mentions of "D-00123" that are not
// wrapped in a markdown link are NOT promoted to a cite --
// only the picker-inserted tokens (locked decision #14)
// count as authoritative cite anchors.
//
// The function reads the live article row by id (not by
// an in-memory body_md argument) so callers can pass an id
// directly. For the slice-4 PDF / Static-HTML renderer,
// the path is: Read article -> call ResolveRefs(articleID)
// -> substitute the rendered HTML output's links with the
// resolved values.
//
// The ArticleSyncID field on every ResolvedRef is a
// convenience for callers that need to write the resolved
// set back to a join row; today no caller does, but the
// field is cheap and adds clarity to the projection.
func (a *ArticleService) ResolveRefs(articleID int64) ([]ResolvedRef, error) {
	article, err := a.GetByID(articleID)
	if err != nil {
		return nil, err
	}
	tokens := scanPersonRefsFromBody(article.BodyMD)
	if len(tokens) == 0 {
		return []ResolvedRef{}, nil
	}
	out := make([]ResolvedRef, 0, len(tokens))
	for _, token := range tokens {
		// Look up the Person Record by display id; the
		// case-insensitive match mirrors GetByDisplayID.
		var (
			personID  int64
			syncID    string
			displayID string
		)
		err := a.soldiers.db.Conn().QueryRow(
			`SELECT id, sync_id, display_id FROM soldiers WHERE upper(display_id) = upper(?)`,
			token).Scan(&personID, &syncID, &displayID)
		if errors.Is(err, sql.ErrNoRows) {
			out = append(out, ResolvedRef{
				ArticleID:     article.ID,
				ArticleSyncID: article.SyncID,
				Token:         token,
				Resolved:      false,
			})
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("resolve ref %q: %w", token, err)
		}
		out = append(out, ResolvedRef{
			ArticleID:       article.ID,
			ArticleSyncID:   article.SyncID,
			Token:           token,
			PersonRecordID:  personID,
			PersonDisplayID: displayID,
			Resolved:        true,
		})
	}
	return out, nil
}

// scanPersonRefsFromBody tokenizes the markdown source for
// `[text](#person/D-00123)` patterns and returns the display
// ids in source order. Each token appears once per unique
// link target (de-dup) so the renderer sees one
// resolve-output per distinct Person Record. We use a
// regex-based scan so a body with N references to the same
// Person Record produces N entries (one per link) -- a
// future "reorder refs" surface mirroring #368 may care
// about that distinction; the slice-2 PDF / Static-HTML
// renderer de-dups at its own layer.
//
// The parser uses a multi-line regex anchored on `\(` +
// `#person/` + capture. It explicitly skips escaped
// brackets + fenced code blocks to avoid picking up
// documentation snippets (the picker-inserted tokens are
// raw markdown links with no escape characters).
//
// Regex choice: a simple pattern keeps the parser testable
// without depending on a markdown library at slice 2; the
// slice-4 export pipeline swaps in goldmark + bluemonday
// anyway, so this parser's role shrinks to "tokenize
// picker-inserted tokens for the Cite-in reverse-lookup +
// the slice-3 live preview."

// regex for the picker-inserted token pattern. Anchored on
// a literal ( + #person/ + capture group + closing ). This is
// intentionally minimal: the picker (slice 3) inserts exactly
// one token form (the [Display Text](#person/D-00123) shape)
// so the regex matches one token per inline link, not nested
// markdown or escape sequences.
var personRefLinkRE = regexp.MustCompile(`\(#person/([A-Za-z0-9_-]+)\)`)

// renderBodyHTML returns the sanitized HTML render of the
// markdown source. Falls back to the verbatim copy when
// the slice-3.6 renderer is not configured (slice-1 + 2
// callers pass nil so the body_html column carries the
// verbatim md verbatim, preserving the slice-1 round-trip
// contract). When the renderer IS configured (slice-3.6+
// path), goldmark + bluemonday Strict strip raw HTML so a
// malicious body cannot XSS-es via a templ.Raw mount.
func (a *ArticleService) renderBodyHTML(bodyMD string) string {
	if a.renderer == nil {
		return bodyMD
	}
	rendered, err := a.renderer.Render(bodyMD)
	if err != nil {
		return bodyMD
	}
	return rendered
}

// RenderBodyHTML is the public wrapper over renderBodyHTML.
// The slice-3.6 live-preview handler calls this on every
// keystroke (250ms debounce) so the preview pane reflects
// the current source. Returns "" when bodyMD is empty so
// the handler can substitute the guidance message.
func (a *ArticleService) RenderBodyHTML(bodyMD string) string {
	return a.renderBodyHTML(bodyMD)
}

// scanPersonRefsFromBody returns the display-id tokens from the
// markdown source in source order. De-dup is at the regex
// level (one match per link); the slice-2 callers may de-dup
// again if they care.
func scanPersonRefsFromBody(body string) []string {
	matches := personRefLinkRE.FindAllStringSubmatch(body, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) >= 2 {
			out = append(out, m[1])
		}
	}
	return out
}

// ErrArticleSnapshot is returned by Update + Delete when the
// target row is a snapshot (is_snapshot = 1). Snapshots are
// read-only; callers must use the slice-2.5 Snapshot lifecycle
// (Restore + DeleteSnapshot) instead. The slice-2 handler maps
// this to 409 Conflict so the front end can render a distinct
// "this is a snapshot" message.
var ErrArticleSnapshot = errors.New("article is a snapshot")

// ErrRefPersonNotFound is returned by AttachRef + DetachRef
// when the target person_record_id does not exist (or has
// been deleted). The handler maps to 404. Distinct from
// ErrArticleNotFound so the front end can show the right
// error message ("Person not found" vs "Article not found").
var ErrRefPersonNotFound = errors.New("person record not found for article ref")


// Snapshot duplicates the live-branch article referenced by
// srcID into a new row with is_snapshot = 1 and
// snapshot_of_id = srcID. The duplicated row gets a fresh
// ART-NNNNN Display ID minted by NextArticleID (per slice
// 2.5 the snapshot is its own archive entry, not a
// status-flip of the live row -- the snapshot row carries
// the same SyncID lineage but a different display id so the
// user can browse snapshot history without colliding with
// the live ART- namespace).
//
// Returns ErrArticleSnapshot when srcID is itself a snapshot
// (snapshot-of-snapshot rejected per slice-2.5 spec); returns
// ErrArticleNotFound when srcID does not exist.
//
// The snapshot row's title + subtitle + body_md + body_html
// are copied verbatim from the live row at create time.
// Subsequent Restore calls copy the snapshot's CURRENT
// fields back to the live branch; Restore is the only path
// that mutates the live row's content from outside Update.
//
// The snapshot inherits refs via the FK only on attach
// calls (slice 3 may extend this); the slice-2.5 surface
// does NOT copy article_refs into the snapshot -- a
// snapshot is a content snapshot, not a refs-snapshot. The
// slice-5 archive integration is the path that captures
// refs in the bundle, not the snapshot.
//
// sync_id is RE-minted for the snapshot (a snapshot is its
// own archive entity). A future slice may copy sync_id + add
// a per-snapshot lineage if distributed-merge needs to
// reconcile them; slice 2.5 starts simple.
func (a *ArticleService) Snapshot(srcID int64) (*models.Article, error) {
	if srcID < 1 {
		return nil, ErrArticleNotFound
	}
	// Read the source row directly (live OR snapshot) -- a
	// GetByID call would filter snapshots out and produce
	// the wrong ErrArticleNotFound sentinel, masking the
	// snapshot-of-snapshot rejection case the slice-2.5
	// spec explicitly calls out.
	row := a.soldiers.db.Conn().QueryRow(
		`SELECT id, sync_id, display_id, title, subtitle, body_md, body_html,
		        created_at, updated_at, snapshot_of_id, is_snapshot
		 FROM articles WHERE id = ?`, srcID)
	var (
		art          models.Article
		snapshotOfID sql.NullInt64
		isSnapshot   int
	)
	if err := row.Scan(
		&art.ID, &art.SyncID, &art.DisplayID, &art.Title, &art.Subtitle,
		&art.BodyMD, &art.BodyHTML, &art.CreatedAt, &art.UpdatedAt,
		&snapshotOfID, &isSnapshot,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrArticleNotFound
		}
		return nil, fmt.Errorf("Snapshot scan src %d: %w", srcID, err)
	}
	if isSnapshot != 0 {
		return nil, ErrArticleSnapshot
	}
	src := art
	now := time.Now().UTC().Format(time.RFC3339)
	dispID, err := a.soldiers.db.NextArticleID()
	if err != nil {
		return nil, fmt.Errorf("Snapshot NextArticleID: %w", err)
	}
	res, err := a.soldiers.db.Conn().Exec(
		`INSERT INTO articles
		  (sync_id, display_id, title, subtitle, body_md, body_html,
		   created_at, updated_at, snapshot_of_id, is_snapshot)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`,
		uuid.NewString(), dispID,
		src.Title, src.Subtitle, src.BodyMD, src.BodyHTML,
		now, now, src.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("Snapshot insert: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("Snapshot LastInsertId: %w", err)
	}
	// Read back for the caller (gives back the same DisplayID
	// + snapshot_of_id we'd otherwise need to re-derive).
	return &models.Article{
		ID:           id,
		SyncID:       "",
		DisplayID:    dispID,
		Title:        src.Title,
		Subtitle:     src.Subtitle,
		BodyMD:       src.BodyMD,
		BodyHTML:     src.BodyHTML,
		CreatedAt:    now,
		UpdatedAt:    now,
		SnapshotOfID: &srcID,
		IsSnapshot:   true,
	}, nil
}

// Restore overwrites the live-branch row that the snapshot
// refers to with the snapshot's current fields. The snapshot
// row stays in place (per slice 2.5's "user-managed" Revisions
// contract: snapshot stays after restore so the user can
// restore again later or browse the saved history).
//
// Returns ErrArticleSnapshot when snapshotID is a live-branch
// row (the inverse case: not-a-snapshot cannot restore);
// returns ErrArticleNotFound when snapshotID does not exist.
//
// Calls Update under the hood to centralize the stamped-
// updated_at path (which Update does for the slice-2 live
// branch). After Restore the live row's body_md + body_html
// match the snapshot's; the live row's DisplayID stays put.
func (a *ArticleService) Restore(snapshotID int64) error {
	if snapshotID < 1 {
		return ErrArticleNotFound
	}
	row := a.soldiers.db.Conn().QueryRow(
		`SELECT id, snapshot_of_id, is_snapshot FROM articles WHERE id = ?`, snapshotID)
	var (
		id          int64
		snapshotOf  sql.NullInt64
		isSnapshot  int
	)
	if err := row.Scan(&id, &snapshotOf, &isSnapshot); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrArticleNotFound
		}
		return fmt.Errorf("Restore scan: %w", err)
	}
	if isSnapshot == 0 {
		return ErrArticleSnapshot
	}
	if !snapshotOf.Valid {
		return fmt.Errorf("snapshot %d has no snapshot_of_id (data corruption)", snapshotID)
	}
	// Read snapshot fields + Update the live row.
	snap := a.soldiers.db.Conn().QueryRow(
		`SELECT title, subtitle, body_md, body_html FROM articles WHERE id = ?`, snapshotID)
	var (
		title, subtitle, bodyMD, bodyHTML string
	)
	if err := snap.Scan(&title, &subtitle, &bodyMD, &bodyHTML); err != nil {
		return fmt.Errorf("Restore scan fields: %w", err)
	}
	liveID := snapshotOf.Int64
	if err := a.Update(models.Article{
		ID:       liveID,
		Title:    title,
		Subtitle: subtitle,
		BodyMD:   bodyMD,
		BodyHTML: bodyHTML,
	}); err != nil {
		return fmt.Errorf("Restore -> Update live %d: %w", liveID, err)
	}
	return nil
}

// DeleteSnapshot removes the snapshot row at snapshotID.
// The live branch the snapshot refers to is unaffected --
// Restore is the only path that mutates the live row from
// outside Update, DeleteSnapshot is delete-of-snapshot-only
// per slice 2.5's "Revisions tab: list + Delete + Restore"
// surface.
//
// Returns ErrArticleSnapshot when snapshotID is a live-branch
// row (live rows use the regular Delete path; DeleteSnapshot
// is snapshots-only); returns ErrArticleNotFound when the
// row does not exist. The cascade behavior matches Delete:
// any future per-snapshot refs table would cascade here.
//
// Idempotent: deleting a non-existent snapshot row returns
// ErrArticleNotFound (so the handler maps a stale DELETE to
// 404, not 200 -- distinct from DetachRef's idempotency).
func (a *ArticleService) DeleteSnapshot(snapshotID int64) error {
	if snapshotID < 1 {
		return ErrArticleNotFound
	}
	res, err := a.soldiers.db.Conn().Exec(`DELETE FROM articles WHERE id = ? AND is_snapshot = 1`, snapshotID)
	if err != nil {
		return fmt.Errorf("DeleteSnapshot %d: %w", snapshotID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("DeleteSnapshot %d rows: %w", snapshotID, err)
	}
	if n == 1 {
		return nil
	}
	// Disambiguate: not-found vs not-a-snapshot.
	var isSnapshot int
	if err := a.soldiers.db.Conn().QueryRow(`SELECT is_snapshot FROM articles WHERE id = ?`, snapshotID).Scan(&isSnapshot); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrArticleNotFound
		}
		return fmt.Errorf("DeleteSnapshot disambiguate scan: %w", err)
	}
	if isSnapshot == 0 {
		return ErrArticleSnapshot
	}
	return ErrArticleNotFound
}

// CitedInArticles returns the live-branch articles (is_snapshot
// = 0) that cite the given Person Record via the article_refs
// junction table, sorted updated_at DESC (most-recently-edited
// first; matches the slice-2 List ordering). The slice-3.8
// Person Record detail "Cited in" panel consumes this list.
//
// The filter excludes snapshot rows because snapshots are
// historical artifacts, not load-bearing articles -- a
// researcher who clicks through from the Person Record detail
// expects to land on the current article body, not a snapshot.
// Per the slice-2.5 design, snapshot rows are read-only by
// design; the "Cited in" panel honors that.
//
// Returns an empty slice (not nil) when no articles cite the
// person so the panel renders the empty state without a nil
// guard. Negative personID is rejected so a confused caller
// (e.g. a future slice that lost the personID context) fails
// fast rather than returning every article in the archive.
func (a *ArticleService) CitedInArticles(personID int64) ([]models.Article, error) {
	if personID < 1 {
		return nil, fmt.Errorf("CitedInArticles: person id must be positive")
	}
	rows, err := a.soldiers.db.Conn().Query(
		`SELECT a.id, a.sync_id, a.display_id, a.title, a.subtitle,
		        a.body_md, a.body_html, a.created_at, a.updated_at,
		        a.snapshot_of_id, a.is_snapshot
		 FROM articles a
		 JOIN article_refs ar ON ar.article_id = a.id
		 WHERE ar.person_record_id = ? AND a.is_snapshot = 0
		 ORDER BY a.updated_at DESC, a.id DESC`,
		personID,
	)
	if err != nil {
		return nil, fmt.Errorf("CitedInArticles %d: %w", personID, err)
	}
	defer debug.DeferCloseLog(rows, "CitedInArticles.rows")
	out := make([]models.Article, 0)
	for rows.Next() {
		var (
			art          models.Article
			snapshotOfID sql.NullInt64
			isSnapshot   int
		)
		if err := rows.Scan(
			&art.ID, &art.SyncID, &art.DisplayID, &art.Title, &art.Subtitle,
			&art.BodyMD, &art.BodyHTML, &art.CreatedAt, &art.UpdatedAt,
			&snapshotOfID, &isSnapshot,
		); err != nil {
			return nil, fmt.Errorf("CitedInArticles scan: %w", err)
		}
		if snapshotOfID.Valid {
			v := snapshotOfID.Int64
			art.SnapshotOfID = &v
		}
		art.IsSnapshot = isSnapshot != 0
		out = append(out, art)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("CitedInArticles rows: %w", err)
	}
	return out, nil
}

// PDFResult is the bytes + filename return value for the
// article RenderPDF helper (issue #321 slice 4.2). The
// ArticleService.RenderPDF method pre-renders the PDF body
// before the caller's file dialog opens, so the result
// needs to travel as in-memory bytes + a suggested filename
// (the dialog uses the filename as DefaultFilename).
//
// Today only ArticleService uses this shape; EventPDF +
// SoldierPDF keep writing directly to a path inside their
// export jobs (so a long render doesn't block the request
// goroutine). ArticlePDF is small enough to pre-render
// without the job-enqueue overhead.
type PDFResult struct {
	Bytes    []byte
	Filename string
}

// RenderPDF pre-renders the article's PDF body to bytes
// and returns them alongside a slugified filename. The
// handler hands the bytes to a SaveFileDialog-driven write
// (no job-enqueue needed because the render is short).
// orientation is "portrait" or "landscape"; both resolve
// to templates/article_<orientation>.typ via the Registry's
// defaultTemplateName.
//
// Errors:
//   - ErrArticleNotFound    when the article id does not exist or is a snapshot
//   - other render errors  propagate from the registry
//
// The resolvedRefs slice is pre-projected from ResolveRefs
// so the typst template does no DB lookups -- same shape
// as ExportEventPDF's `linked []models.Soldier` parameter.
func (a *ArticleService) RenderPDF(articleID int64, orientation string) (*PDFResult, error) {
	if a.registry == nil {
		return nil, fmt.Errorf("RenderPDF: registry not configured")
	}
	article, err := a.GetByID(articleID)
	if err != nil {
		return nil, err
	}
	tokens, err := a.ResolveRefs(articleID)
	if err != nil {
		return nil, fmt.Errorf("ResolveRefs %d: %w", articleID, err)
	}
	resolvedRefs := make([]map[string]any, 0, len(tokens))
	for _, tok := range tokens {
		displayID := tok.PersonDisplayID
		if displayID == "" {
			displayID = tok.Token
		}
		name := displayID
		if tok.Resolved {
			if s, lookupErr := a.soldiers.GetByID(tok.PersonRecordID); lookupErr == nil && s != nil {
				fullName := strings.TrimSpace(strings.Join([]string{strings.TrimSpace(s.FirstName), strings.TrimSpace(s.LastName)}, " "))
				if fullName != "" {
					name = fullName
				}
			}
		}
		resolvedRefs = append(resolvedRefs, map[string]any{
			"display_id": displayID,
			"name":       name,
			"resolved":   tok.Resolved,
		})
	}
	opts := renderDefaultPDFOptions(orientation)
	// Render the markdown body to typst markup so the PDF
	// template can inline it as styled content (see
	// internal/records/markdown_typst.go for the converter
	// + the linked GitHub issue for the original bug). typst
	// can't render arbitrary HTML natively, so we walk the
	// goldmark AST instead of round-tripping through the
	// sanitized HTML -- the same source feeds both the web
	// render (BodyHTML) and the PDF render (article.body_typst).
	bodyTypst, typstErr := a.renderer.RenderTypst(article.BodyMD)
	if typstErr != nil {
		return nil, fmt.Errorf("RenderPDF %d: typst body: %w", articleID, typstErr)
	}
	// The article template reads body_typst via
	// `a.at("body_typst", default: "")` where `a` is the
	// `article` map. Embed the typst body INSIDE the article
	// map (not at the top level of the data payload) so the
	// template picks it up. Mirrors the original `body_html`
	// shape from before commit 6cb6e40.
	articleWithBody := *article
	articlePayload := map[string]any{
		"id":             articleWithBody.ID,
		"sync_id":        articleWithBody.SyncID,
		"display_id":     articleWithBody.DisplayID,
		"title":          articleWithBody.Title,
		"subtitle":       articleWithBody.Subtitle,
		"body_md":        articleWithBody.BodyMD,
		"body_html":      articleWithBody.BodyHTML,
		"body_typst":     bodyTypst,
		"created_at":     articleWithBody.CreatedAt,
		"updated_at":     articleWithBody.UpdatedAt,
		"is_snapshot":    articleWithBody.IsSnapshot,
		"snapshot_of_id":  articleWithBody.SnapshotOfID,
	}
	_ = articleWithBody
	data := map[string]any{
		"article":       articlePayload,
		"resolved_refs": resolvedRefs,
		"options":       opts,
		"branding":      map[string]string{},
	}
	var buf bytes.Buffer
	if err := a.registry.RenderArticle(contextBackground(), "article", normalizeOrientation(orientation), data, &buf); err != nil {
		return nil, fmt.Errorf("RenderPDF %d: %w", articleID, err)
	}
	return &PDFResult{
		Bytes:    buf.Bytes(),
		Filename: SlugifyArticleFilename(*article, orientation),
	}, nil
}

// articlePDFOptions is the small subset of PDFOptions the
// article template's data.json needs. Local type to avoid
// importing pkg/render (cycle: pkg/render -> internal/records).
type articlePDFOptions struct {
	Orientation     string `json:"orientation"`
	PrinterFriendly bool   `json:"printerFriendly"`
	IncludeImages   bool   `json:"includeImages"`
}

// renderDefaultPDFOptions builds the article PDFOptions payload
// the typst template reads. Mirrors the appshell's
// PDFOptionsFromForm helper but stays inside the service so
// the RenderPDF path is self-contained.
func renderDefaultPDFOptions(orientation string) articlePDFOptions {
	return articlePDFOptions{
		Orientation:     normalizeOrientation(orientation),
		PrinterFriendly: true,
		IncludeImages:   false,
	}
}

// SlugifyArticleFilename builds the suggested filename:
// "Article-ART-NNNNN-<title-slug>-<orientation>.pdf". The
// title slug is lowercased, stripped of non-alphanumerics,
// and truncated to 60 characters so the filename stays
// readable in a Windows file dialog. Falls back to the
// DisplayID alone when the title is blank.
//
// Exported (issue #533) so the appshell handler can compute
// the suggested filename BEFORE the worker runs (so the
// native SaveFileDialog shows a sensible default) without
// doing a pre-render of the PDF body.
func SlugifyArticleFilename(article models.Article, orientation string) string {
	short := "landscape"
	if normalizeOrientation(orientation) == "P" {
		short = "portrait"
	}
	slug := slugify(article.Title)
	if slug == "" {
		return fmt.Sprintf("Article-%s-%s.pdf", article.DisplayID, short)
	}
	return fmt.Sprintf("Article-%s-%s-%s.pdf", article.DisplayID, slug, short)
}

// slugify lower-cases + strips non-alphanumeric + collapses
// runs of whitespace to single hyphens. Truncates to 60 chars.
func slugify(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return ""
	}
	var b strings.Builder
	prevHyphen := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevHyphen = false
		case r == ' ' || r == '-' || r == '_':
			if !prevHyphen && b.Len() > 0 {
				b.WriteByte('-')
				prevHyphen = true
			}
		}
		if b.Len() >= 60 {
			break
		}
	}
	out := strings.TrimRight(b.String(), "-")
	return out
}

// normalizeOrientation maps "portrait" / "landscape" / "" /
// anything else to the canonical "P" / "L" the typst template
// reads via opts.at("orientation").
func normalizeOrientation(o string) string {
	switch strings.ToLower(strings.TrimSpace(o)) {
	case "p", "portrait":
		return "P"
	default:
		return "L"
	}
}

// contextBackground returns a context.Background(). The slice-4
// pre-render path runs synchronously on the request goroutine
// so a cancelled context would defeat the purpose of the
// pre-render -- it must run to completion.
func contextBackground() context.Context { return context.Background() }

// RenderStaticHTML returns a self-contained HTML rendering of
// the article (issue #321 slice 4.3). One file, no external
// assets: the body_html column already carries the sanitized
// HTML from Create/Update (goldmark + bluemonday), and the
// inline ResolveRefs output renders the in-body Person Record
// tokens as <a href> links (no JS, no separate CSS).
//
// The returned HTML is suitable for a static archive index
// card (slice 5.3 ships this into window.DIXIE_DATA) or a
// download-the-static-archive-as-single-file surface.
//
// Errors:
//   - ErrArticleNotFound    when the article id does not exist or is a snapshot
//   - other errors          propagate from the service
func (a *ArticleService) RenderStaticHTML(articleID int64) (string, error) {
	article, err := a.GetByID(articleID)
	if err != nil {
		return "", err
	}
	tokens, err := a.ResolveRefs(articleID)
	if err != nil {
		return "", fmt.Errorf("ResolveRefs %d: %w", articleID, err)
	}
	var refsBuf strings.Builder
	if len(tokens) > 0 {
		refsBuf.WriteString(`<section class="article-refs"><h3>Cited Person Records</h3><ul>`)
		for _, tok := range tokens {
			displayID := tok.PersonDisplayID
			if displayID == "" {
				displayID = tok.Token
			}
			if tok.Resolved {
				refsBuf.WriteString(fmt.Sprintf(
					`<li><a href="/soldiers/%d" data-person-display-id="%s">%s</a></li>`,
					tok.PersonRecordID, displayID, displayID,
				))
			} else {
				refsBuf.WriteString(fmt.Sprintf(
					`<li><span class="article-ref-unknown" title="Display ID not found in the archive.">⚠ Unknown: %s</span></li>`,
					displayID,
				))
			}
		}
		refsBuf.WriteString("</ul></section>")
	}
	articleBody := article.BodyHTML
	if articleBody == "" {
		articleBody = article.BodyMD
	}
	return fmt.Sprintf(
		`<article class="article-static" data-article-id="%d" data-article-display-id="%s">`+
			`<header><h2>%s</h2>%s</header>`+
			`<div class="article-body">%s</div>%s`+
			`</article>`,
		article.ID, article.DisplayID,
		escapeHTML(article.Title),
		subtitleHTML(article.Subtitle),
		articleBody,
		refsBuf.String(),
	), nil
}

// escapeHTML escapes the few characters that could break the
// surrounding HTML wrapper. The body_html column is already
// sanitized (goldmark + bluemonday Strict) so it's emitted
// verbatim; only the title + subtitle need escaping.
func escapeHTML(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&#39;",
	)
	return r.Replace(s)
}

// subtitleHTML wraps the subtitle in a <p> when non-empty.
func subtitleHTML(subtitle string) string {
	if strings.TrimSpace(subtitle) == "" {
		return ""
	}
	return `<p class="article-subtitle">` + escapeHTML(subtitle) + `</p>`
}
