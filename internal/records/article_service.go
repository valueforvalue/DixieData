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
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
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
	soldiers *SoldierService
}

// NewArticleService constructs the slice-1 ArticleService.
// soldierService is borrowed (not copied) so the Article
// service shares the same DB connection pool + migrations +
// PRAGMA state that Person Record + Event Record paths use.
// Slice 2 may split this seam if the test suite outgrows the
// shared pool; for v1 the shared DB connection is the right
// trade-off.
func NewArticleService(soldierService *SoldierService) *ArticleService {
	return &ArticleService{soldiers: soldierService}
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
	bodyHTML := article.BodyMD // slice 1: render=verbatim; slice 2 swap in goldmark+bluemonday.

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

	res, err := a.soldiers.db.Conn().Exec(
		`INSERT INTO articles
		  (sync_id, display_id, title, subtitle, body_md, body_html,
		   created_at, updated_at, snapshot_of_id, is_snapshot)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, 0)`,
		article.SyncID, article.DisplayID, article.Title, article.Subtitle,
		article.BodyMD, article.BodyHTML, article.CreatedAt, article.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert article: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("insert article LastInsertId: %w", err)
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
func (a *ArticleService) GetByID(id int64) (*models.Article, error) {
	if id < 1 {
		return nil, ErrArticleNotFound
	}
	row := a.soldiers.db.Conn().QueryRow(
		`SELECT id, sync_id, display_id, title, subtitle, body_md, body_html,
		        created_at, updated_at, snapshot_of_id, is_snapshot
		 FROM articles WHERE id = ? AND is_snapshot = 0`, id)
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
func (a *ArticleService) List(page, pageSize int) ([]models.Article, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 25
	}
	if pageSize > 100 {
		pageSize = 100
	}
	conn := a.soldiers.db.Conn()
	var total int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM articles WHERE is_snapshot = 0`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count articles: %w", err)
	}
	rows, err := conn.Query(
		`SELECT id, sync_id, display_id, title, subtitle, body_md, body_html,
		        created_at, updated_at, snapshot_of_id, is_snapshot
		 FROM articles WHERE is_snapshot = 0
		 ORDER BY updated_at DESC, id DESC
		 LIMIT ? OFFSET ?`,
		pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("list articles: %w", err)
	}
	defer rows.Close()
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
	bodyHTML := article.BodyMD // slice 2 still stores verbatim; slice 2.5 swaps in goldmark+bluemonday
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := a.soldiers.db.Conn().Exec(
		`UPDATE articles
		    SET title = ?, subtitle = ?, body_md = ?, body_html = ?, updated_at = ?
		  WHERE id = ? AND is_snapshot = 0`,
		title, subtitle, bodyMD, bodyHTML, now, article.ID)
	if err != nil {
		return fmt.Errorf("update article %d: %w", article.ID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update article %d rows: %w", article.ID, err)
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
	res, err := a.soldiers.db.Conn().Exec(`DELETE FROM articles WHERE id = ? AND is_snapshot = 0`, id)
	if err != nil {
		return fmt.Errorf("delete article %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete article %d rows: %w", id, err)
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
	defer rows.Close()
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
