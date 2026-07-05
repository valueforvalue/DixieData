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
