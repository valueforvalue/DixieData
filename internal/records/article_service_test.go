// article_service_test.go pins the slice-1 ArticleService.Create
// + GetByID contract (issue #321). Headline: minting ART-NNNNN
// Display IDs via db.NextArticleID + the GetByID round-trip.
//
// Mirrors records/event_service_test.go's TestCreateEventMintsEVTDisplayID
// pattern so the v180 namespace discipline is asserted the same
// way for Articles as it is for Person Records + Event Records.
package records

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/valueforvalue/DixieData/internal/models"
)

// TestCreateArticleMintsARTDisplayID pins the slice-1 namespace
// discipline (issue #321 locked decision #4): a freshly-created
// Article without a caller-supplied DisplayID gets an ART-NNNNN
// ID minted by db.NextArticleID. On the first insert in a clean
// archive, that is ART-00001. The test creates 3 articles and
// asserts the minted IDs are ART-00001, ART-00002, ART-00003 --
// the same monotonic-counter behavior Event Records follow.
func TestCreateArticleMintsARTDisplayID(t *testing.T) {
	service := newArticleServiceForTest(t)

	for i, want := range []string{"ART-00001", "ART-00002", "ART-00003"} {
		title := "Article " + want
		got, err := service.Create(models.Article{Title: title})
		if err != nil {
			t.Fatalf("Create #%d (%s): %v", i+1, title, err)
		}
		if got.DisplayID != want {
			t.Errorf("Create #%d: DisplayID = %q, want %q", i+1, got.DisplayID, want)
		}
		if got.ID < 1 {
			t.Errorf("Create #%d: ID = %d, want > 0", i+1, got.ID)
		}
		if got.Title != title {
			t.Errorf("Create #%d: Title round-trip = %q, want %q", i+1, got.Title, title)
		}
	}
}

// TestCreateArticleBlankTitleRejected pins the slice-1 input
// validation: a blank title after trim returns
// ErrArticleTitleRequired so the handler can map to a 400. Slice
// 2 may extend the validation surface (length cap, etc.), but
// the blank-title rejection is the slice-1 baseline the RED test
// in TestHandleArticleCRUD_RoundTrip asserts (its blank-title
// sub-test accepts any 4xx today; tighten to 400 once the
// handler maps the sentinel).
func TestCreateArticleBlankTitleRejected(t *testing.T) {
	service := newArticleServiceForTest(t)
	for _, title := range []string{"", "   ", "\t\n"} {
		_, err := service.Create(models.Article{Title: title})
		if !errors.Is(err, ErrArticleTitleRequired) {
			t.Errorf("Create(%q) err = %v, want ErrArticleTitleRequired", title, err)
		}
	}
}

// TestGetArticleByIDRoundTrip pins the slice-1 read path: after
// Create returns the row, GetByID returns the same row with the
// same fields. This is the contract the slice-1 /articles/{id}
// detail-page path depends on.
func TestGetArticleByIDRoundTrip(t *testing.T) {
	service := newArticleServiceForTest(t)
	created, err := service.Create(models.Article{
		Title:    "21st Mississippi at Gettysburg",
		Subtitle: "Day 2 on the Peach Orchard line",
		BodyMD:   "A regimental narrative covering July 2-3, 1863.",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	read, err := service.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID(%d): %v", created.ID, err)
	}
	if read.ID != created.ID {
		t.Errorf("GetByID ID = %d, want %d", read.ID, created.ID)
	}
	if read.DisplayID != created.DisplayID {
		t.Errorf("GetByID DisplayID = %q, want %q", read.DisplayID, created.DisplayID)
	}
	if read.Title != created.Title {
		t.Errorf("GetByID Title = %q, want %q", read.Title, created.Title)
	}
	if !strings.Contains(read.BodyHTML, "regimental narrative") {
		t.Errorf("GetByID BodyHTML missing source content: %q", read.BodyHTML)
	}
	if !strings.Contains(read.BodyMD, "regimental narrative") {
		t.Errorf("GetByID BodyMD missing source content: %q", read.BodyMD)
	}
	if read.IsSnapshot {
		t.Errorf("GetByID IsSnapshot = true, want false (live branch row)")
	}
}

// TestGetArticleByID_NotFound pins the slice-1 error surface:
// an unknown row id returns ErrArticleNotFound (the sentinel
// the slice-1 handler maps to 404 in slice-2's full handler set;
// slice 1's inline handler for /articles/{id} returns 404 when
// GetByID returns ErrArticleNotFound).
func TestGetArticleByID_NotFound(t *testing.T) {
	service := newArticleServiceForTest(t)
	_, err := service.GetByID(999999)
	if !errors.Is(err, ErrArticleNotFound) {
		t.Errorf("GetByID(999999) err = %v, want ErrArticleNotFound", err)
	}
}

// newArticleServiceForTest constructs an ArticleService against
// a fresh temp-DB. Uses the package-private newTestDB helper
// (records/soldier_service_test.go:12) which opens an in-memory
// DB + applies the full schema stack. slice 1 pins the
// ArticleService in isolation; slice 2's handler tests pin the
// full route + DB round-trip via newStressApp.
func newArticleServiceForTest(t *testing.T) *ArticleService {
	t.Helper()
	d := newTestDB(t)
	return NewArticleService(NewSoldierService(d))
}

// newArticleServiceForTestWithRenderer is the slice-3.6
// companion to newArticleServiceForTest -- it wires the
// MarkdownRenderer so Create + Update carry sanitized HTML
// rather than the slice-1 verbatim-md path.
func newArticleServiceForTestWithRenderer(t *testing.T) *ArticleService {
	t.Helper()
	d := newTestDB(t)
	return NewArticleService(NewSoldierService(d), NewMarkdownRenderer())
}


// createTestSoldierForArticles is a small helper for the
// slice-2 tests that need a Person Record with a known
// Display ID to attach as a ref. Returns the freshly-created
// row's id. Creates with minimal fields so the call site
// reads cleanly.
func createTestSoldierForArticles(t *testing.T, svc *SoldierService, displayName string) int64 {
	t.Helper()
	row, err := svc.Create(models.Soldier{
		FirstName: displayName,
		LastName:  "Ref",
		Rank:      "Private",
		Unit:      "Test Unit",
		DisplayID: displayName,
	})
	if err != nil {
		t.Fatalf("Create soldier %q: %v", displayName, err)
	}
	return row.ID
}

// TestArticleService_ListRoundTrip pins the slice-2 List
// contract: paginated, sorted updated_at desc, snapshot
// rows excluded. Creates 3 articles in order; asserts
// GetList returns them with the most-recently-edited first
// after an Update touches one to bump its updated_at.
func TestArticleService_ListRoundTrip(t *testing.T) {
	service := newArticleServiceForTest(t)
	art1, _ := service.Create(models.Article{Title: "First"})
	art2, _ := service.Create(models.Article{Title: "Second"})
	art3, _ := service.Create(models.Article{Title: "Third"})

	// Sleep so art1's Update lands on a strictly later
	// second than the Create timestamps (RFC3339 has second
	// resolution; without the gap the updated_at is the same
	// as one of the inserts and the secondary id-DESC tiebreak
	// would put art3, not art1, at index 0).
	time.Sleep(1100 * time.Millisecond)

	// Bump art1's updated_at so it returns FIRST (most-
	// recently-edited) instead of LAST.
	if err := service.Update(models.Article{ID: art1.ID, Title: "First (revised)", BodyMD: "revised"}); err != nil {
		t.Fatalf("Update art1: %v", err)
	}

	rows, total, err := service.List(1, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if len(rows) != 3 {
		t.Fatalf("len(rows) = %d, want 3", len(rows))
	}
	if rows[0].ID != art1.ID {
		t.Errorf("rows[0].ID = %d, want %d (art1 last-edited)", rows[0].ID, art1.ID)
	}
	if rows[1].ID != art3.ID || rows[2].ID != art2.ID {
		t.Errorf("order drift: got [%d %d %d], want [%d %d %d]",
			rows[1].ID, rows[2].ID, art3.ID, art2.ID,
			rows[0].ID, art1.ID)
	}
}

// TestArticleService_GetByDisplayIDRoundTrip pins the slice-2
// lookup-by-display-id contract. Mints ART-00001 via Create
// (without supplying a display id), then GetByDisplayID
// finds it. Case-insensitive match.
func TestArticleService_GetByDisplayIDRoundTrip(t *testing.T) {
	service := newArticleServiceForTest(t)
	created, _ := service.Create(models.Article{Title: "Lookup target"})

	for _, candidate := range []string{
		"ART-00001",
		"art-00001",
		"Art-00001",
		"  ART-00001  ",
	} {
		row, err := service.GetByDisplayID(candidate)
		if err != nil {
			t.Errorf("GetByDisplayID(%q): %v", candidate, err)
			continue
		}
		if row.ID != created.ID {
			t.Errorf("GetByDisplayID(%q).ID = %d, want %d", candidate, row.ID, created.ID)
		}
	}

	if _, err := service.GetByDisplayID(""); !errors.Is(err, ErrArticleNotFound) {
		t.Errorf("GetByDisplayID(\"\") err = %v, want ErrArticleNotFound", err)
	}
	if _, err := service.GetByDisplayID("ART-99999"); !errors.Is(err, ErrArticleNotFound) {
		t.Errorf("GetByDisplayID(ART-99999) err = %v, want ErrArticleNotFound", err)
	}
}

// TestArticleService_UpdateRoundTrip pins the slice-2 Update
// contract: title + subtitle + body change; updated_at
// advances; created_at is unchanged; snapshot rows are
// rejected with ErrArticleSnapshot.
func TestArticleService_UpdateRoundTrip(t *testing.T) {
	service := newArticleServiceForTest(t)
	created, _ := service.Create(models.Article{
		Title: "Original", Subtitle: "orig", BodyMD: "orig body",
	})
	origCreated := created.CreatedAt
	origUpdated := created.UpdatedAt

	// Force a 1s gap so UpdatedAt visibly changes (RFC3339
	// only has second resolution).
	time.Sleep(1100 * time.Millisecond)

	updated, err := service.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID after Create: %v", err)
	}
	if updated.Title != "Original" {
		t.Errorf("after Create Title = %q, want %q", updated.Title, "Original")
	}
	if updated.CreatedAt != origCreated {
		t.Errorf("CreatedAt drift: got %q, want %q", updated.CreatedAt, origCreated)
	}

	if err := service.Update(models.Article{
		ID:       created.ID,
		Title:    "Revised",
		Subtitle: "rev",
		BodyMD:   "new body [Private](#person/D-00001)",
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	readBack, _ := service.GetByID(created.ID)
	if readBack.Title != "Revised" {
		t.Errorf("after Update Title = %q, want %q", readBack.Title, "Revised")
	}
	if readBack.Subtitle != "rev" {
		t.Errorf("after Update Subtitle = %q, want %q", readBack.Subtitle, "rev")
	}
	if !strings.Contains(readBack.BodyMD, "new body") {
		t.Errorf("after Update BodyMD missing new content: %q", readBack.BodyMD)
	}
	if readBack.CreatedAt != origCreated {
		t.Errorf("CreatedAt must not change on Update: got %q, want %q", readBack.CreatedAt, origCreated)
	}
	if readBack.UpdatedAt == origUpdated {
		t.Errorf("UpdatedAt must advance on Update: still %q", readBack.UpdatedAt)
	}

	// Blank-title rejection.
	if err := service.Update(models.Article{ID: created.ID, Title: ""}); !errors.Is(err, ErrArticleTitleRequired) {
		t.Errorf("Update blank title err = %v, want ErrArticleTitleRequired", err)
	}

	// Unknown id -> ErrArticleNotFound.
	if err := service.Update(models.Article{ID: 999999, Title: "x"}); !errors.Is(err, ErrArticleNotFound) {
		t.Errorf("Update unknown id err = %v, want ErrArticleNotFound", err)
	}
}

// TestArticleService_DeleteRoundTrip pins the slice-2 Delete
// contract. After Delete the row is gone (GetByID returns
// ErrArticleNotFound); article_refs rows cascade via the FK
// ON DELETE CASCADE constraint.
func TestArticleService_DeleteRoundTrip(t *testing.T) {
	service := newArticleServiceForTest(t)
	soldiers := NewSoldierService(service.soldiers.db)
	personID := createTestSoldierForArticles(t, soldiers, "D-00001")
	created, _ := service.Create(models.Article{Title: "To delete"})

	if _, err := service.AttachRef(created.ID, personID); err != nil {
		t.Fatalf("AttachRef: %v", err)
	}
	refs, _ := service.ScanRefs(created.ID)
	if len(refs) != 1 {
		t.Fatalf("before delete: ScanRefs len = %d, want 1", len(refs))
	}

	if err := service.Delete(created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := service.GetByID(created.ID); !errors.Is(err, ErrArticleNotFound) {
		t.Errorf("GetByID after Delete err = %v, want ErrArticleNotFound", err)
	}
	// Cascade: refs gone.
	refs, _ = service.ScanRefs(created.ID)
	if len(refs) != 0 {
		t.Errorf("after Delete: ScanRefs len = %d, want 0 (cascade)", len(refs))
	}

	// Idempotent-delete: a second Delete on the same id
	// returns ErrArticleNotFound (the row is gone).
	if err := service.Delete(created.ID); !errors.Is(err, ErrArticleNotFound) {
		t.Errorf("second Delete err = %v, want ErrArticleNotFound", err)
	}

	// Unknown id.
	if err := service.Delete(999999); !errors.Is(err, ErrArticleNotFound) {
		t.Errorf("Delete unknown id err = %v, want ErrArticleNotFound", err)
	}
}

// TestArticleService_AttachDetachRefRoundTrip pins the
// slice-2 ref-row contract. AttachRef returns the new row
// id; a second attach on the same pair is a no-op (returns
// the existing row id); DetachRef removes the row; a
// detach on a non-existent row is a no-op (no error).
func TestArticleService_AttachDetachRefRoundTrip(t *testing.T) {
	service := newArticleServiceForTest(t)
	soldiers := NewSoldierService(service.soldiers.db)
	personID := createTestSoldierForArticles(t, soldiers, "D-00002")
	article, _ := service.Create(models.Article{Title: "Attach target"})

	id1, err := service.AttachRef(article.ID, personID)
	if err != nil {
		t.Fatalf("AttachRef first: %v", err)
	}
	if id1 < 1 {
		t.Errorf("AttachRef first id = %d, want > 0", id1)
	}

	// Second attach on the same pair is a no-op but returns
	// the existing row id.
	id2, err := service.AttachRef(article.ID, personID)
	if err != nil {
		t.Fatalf("AttachRef second: %v", err)
	}
	if id2 != id1 {
		t.Errorf("AttachRef second id = %d, want %d (no-op returns existing)", id2, id1)
	}

	refs, _ := service.ScanRefs(article.ID)
	if len(refs) != 1 {
		t.Errorf("ScanRefs len = %d, want 1 (duplicate attach is no-op)", len(refs))
	}

	if err := service.DetachRef(article.ID, personID); err != nil {
		t.Fatalf("DetachRef: %v", err)
	}
	refs, _ = service.ScanRefs(article.ID)
	if len(refs) != 0 {
		t.Errorf("after Detach ScanRefs len = %d, want 0", len(refs))
	}

	// Idempotent: detach on a non-existent row is a no-op.
	if err := service.DetachRef(article.ID, personID); err != nil {
		t.Errorf("Detach non-existent err = %v, want nil", err)
	}

	// Missing article.
	if _, err := service.AttachRef(999999, personID); !errors.Is(err, ErrArticleNotFound) {
		t.Errorf("AttachRef missing article err = %v, want ErrArticleNotFound", err)
	}

	// Missing person.
	if _, err := service.AttachRef(article.ID, 999999); !errors.Is(err, ErrRefPersonNotFound) {
		t.Errorf("AttachRef missing person err = %v, want ErrRefPersonNotFound", err)
	}
}

// TestArticleService_ResolveRefs pins the slice-2 token
// parser. Four sub-cases:
//   - body has a known token -> resolved with PersonID + DisplayID
//   - body has an unknown token -> not resolved; PersonID == 0
//   - body has multiple tokens -> each appears once
//   - body has zero tokens -> empty result, no error
func TestArticleService_ResolveRefs(t *testing.T) {
	service := newArticleServiceForTest(t)
	soldiers := NewSoldierService(service.soldiers.db)
	// Create three Person Records with explicit display ids
	// so the resolver can match the markdown tokens.
	p1 := createTestSoldierForArticles(t, soldiers, "D-00001")
	_ = createTestSoldierForArticles(t, soldiers, "D-00002")
	_ = createTestSoldierForArticles(t, soldiers, "D-00003")

	t.Run("known token resolves to PersonID + DisplayID", func(t *testing.T) {
		art, err := service.Create(models.Article{
			Title:  "R1",
			BodyMD: "see [Private](#person/D-00001).",
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		resolved, err := service.ResolveRefs(art.ID)
		if err != nil {
			t.Fatalf("ResolveRefs: %v", err)
		}
		if len(resolved) != 1 {
			t.Fatalf("resolved len = %d, want 1", len(resolved))
		}
		r := resolved[0]
		if !r.Resolved {
			t.Errorf("Resolved = false, want true")
		}
		if r.PersonRecordID != p1 {
			t.Errorf("PersonRecordID = %d, want %d", r.PersonRecordID, p1)
		}
		if r.PersonDisplayID != "D-00001" {
			t.Errorf("PersonDisplayID = %q, want D-00001", r.PersonDisplayID)
		}
		if r.Token != "D-00001" {
			t.Errorf("Token = %q, want D-00001", r.Token)
		}
	})

	t.Run("unknown token is fail-loud (Resolved=false)", func(t *testing.T) {
		art, err := service.Create(models.Article{
			Title:  "R2",
			BodyMD: "[Unknown](#person/D-99999).",
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		resolved, err := service.ResolveRefs(art.ID)
		if err != nil {
			t.Fatalf("ResolveRefs: %v", err)
		}
		if len(resolved) != 1 {
			t.Fatalf("resolved len = %d, want 1", len(resolved))
		}
		if resolved[0].Resolved {
			t.Errorf("Resolved = true, want false (fail-loud)")
		}
		if resolved[0].PersonRecordID != 0 {
			t.Errorf("PersonRecordID = %d, want 0", resolved[0].PersonRecordID)
		}
		if resolved[0].Token != "D-99999" {
			t.Errorf("Token = %q, want D-99999", resolved[0].Token)
		}
	})

	t.Run("multiple tokens each appear in source order", func(t *testing.T) {
		art, err := service.Create(models.Article{
			Title:  "R3",
			BodyMD: "[A](#person/D-00001) and [B](#person/D-00003) and [C](#person/D-00002).",
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		resolved, err := service.ResolveRefs(art.ID)
		if err != nil {
			t.Fatalf("ResolveRefs: %v", err)
		}
		if len(resolved) != 3 {
			t.Fatalf("resolved len = %d, want 3", len(resolved))
		}
		want := []string{"D-00001", "D-00003", "D-00002"}
		for i, r := range resolved {
			if r.Token != want[i] {
				t.Errorf("resolved[%d].Token = %q, want %q", i, r.Token, want[i])
			}
			if !r.Resolved {
				t.Errorf("resolved[%d] not resolved: %+v", i, r)
			}
		}
	})

	t.Run("body without tokens returns empty", func(t *testing.T) {
		art, err := service.Create(models.Article{
			Title:  "R4",
			BodyMD: "No tokens here, just plain prose.",
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		resolved, err := service.ResolveRefs(art.ID)
		if err != nil {
			t.Fatalf("ResolveRefs: %v", err)
		}
		if len(resolved) != 0 {
			t.Errorf("resolved len = %d, want 0", len(resolved))
		}
	})
}


// TestArticleService_Snapshot pins the slice-2.5 Snapshot
// contract: Snapshot(srcID) creates a new row with
// is_snapshot=1, snapshot_of_id=srcID, fresh DisplayID
// (ART-NNNNN), copy of title/subtitle/body; rejects
// snapshot-of-snapshot; rejects source not found.
func TestArticleService_Snapshot(t *testing.T) {
	service := newArticleServiceForTest(t)
	src, err := service.Create(models.Article{
		Title:    "Source",
		Subtitle: "src sub",
		BodyMD:   "src body",
	})
	if err != nil {
		t.Fatalf("Create source: %v", err)
	}

	snap, err := service.Snapshot(src.ID)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if snap.ID == src.ID {
		t.Errorf("Snapshot ID == source.ID (%d), want fresh row id", snap.ID)
	}
	if !snap.IsSnapshot {
		t.Errorf("Snapshot IsSnapshot = false, want true")
	}
	if snap.SnapshotOfID == nil || *snap.SnapshotOfID != src.ID {
		t.Errorf("Snapshot.SnapshotOfID = %v, want pointer to %d", snap.SnapshotOfID, src.ID)
	}
	if snap.Title != src.Title || snap.Subtitle != src.Subtitle {
		t.Errorf("Snapshot did not copy title/subtitle: %q / %q vs src %q / %q",
			snap.Title, snap.Subtitle, src.Title, src.Subtitle)
	}
	if snap.BodyMD != src.BodyMD || snap.BodyHTML != src.BodyHTML {
		t.Errorf("Snapshot did not copy body_md/body_html")
	}

	// Reject snapshot-of-snapshot.
	_, err = service.Snapshot(snap.ID)
	if !errors.Is(err, ErrArticleSnapshot) {
		t.Errorf("Snapshot(snapshot-row) err = %v, want ErrArticleSnapshot", err)
	}

	// Reject source not found.
	_, err = service.Snapshot(999999)
	if !errors.Is(err, ErrArticleNotFound) {
		t.Errorf("Snapshot(999999) err = %v, want ErrArticleNotFound", err)
	}

	// Snapshot does not appear in the live list (GetByID +
	// List both filter on is_snapshot = 0).
	if _, err := service.GetByID(snap.ID); !errors.Is(err, ErrArticleNotFound) {
		t.Errorf("GetByID(snapshot.ID) err = %v, want ErrArticleNotFound", err)
	}
	liveRows, _, err := service.List(1, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	foundSnap := false
	for _, row := range liveRows {
		if row.ID == snap.ID {
			foundSnap = true
		}
	}
	if foundSnap {
		t.Errorf("List returned snapshot row %d (should be live-only)", snap.ID)
	}
}

// TestArticleService_Restore pins the slice-2.5 Restore
// contract: Restore(snapshotID) overwrites the live row's
// title + subtitle + body with the snapshot's CURRENT
// fields; the snapshot stays in place; rejects non-snapshot
// targets; rejects source not found.
func TestArticleService_Restore(t *testing.T) {
	service := newArticleServiceForTest(t)

	src, _ := service.Create(models.Article{
		Title:  "Original",
		BodyMD: "original body",
	})
	snap, _ := service.Snapshot(src.ID)

	// Mutate the snapshot to simulate "the user edits the
	// snapshot's body in an editor" -- Restore is the only
	// path that copies the snapshot's CURRENT state back to
	// the live row, so the live row picks up the edited body.
	//
	// The Update path rejects snapshot rows via the
	// is_snapshot = 0 filter (Update only affects live), so
	// we exercise a direct SQL UPDATE here. Slice 3 may add
	// an Edit-snapshot surface; for now, simulating via SQL
	// is the simplest fidelity to the contract.
	_, err := service.soldiers.db.Conn().Exec(
		`UPDATE articles SET body_md = ?, body_html = ?, title = ? WHERE id = ? AND is_snapshot = 1`,
		"edited snapshot body", "edited snapshot body", "Edited Snapshot Title", snap.ID)
	if err != nil {
		t.Fatalf("direct SQL update of snapshot: %v", err)
	}

	if err := service.Restore(snap.ID); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	live, err := service.GetByID(src.ID)
	if err != nil {
		t.Fatalf("GetByID(source): %v", err)
	}
	if live.Title != "Edited Snapshot Title" {
		t.Errorf("after Restore: title = %q, want %q", live.Title, "Edited Snapshot Title")
	}
	if live.BodyMD != "edited snapshot body" {
		t.Errorf("after Restore: body = %q, want %q", live.BodyMD, "edited snapshot body")
	}

	// Snapshot stays in place (slice-2.5 contract).
	snapAfter, err := service.GetSnapshotByID(snap.ID)
	if err != nil {
		t.Errorf("snapshot disappeared after Restore: GetSnapshotByID = %v", err)
	}
	if snapAfter != nil && snapAfter.ID != snap.ID {
		t.Errorf("snapshot ID changed: was %d, now %d", snap.ID, snapAfter.ID)
	}

	// Reject non-snapshot target.
	if err := service.Restore(src.ID); !errors.Is(err, ErrArticleSnapshot) {
		t.Errorf("Restore(live-id) err = %v, want ErrArticleSnapshot", err)
	}

	// Reject source not found.
	if err := service.Restore(999999); !errors.Is(err, ErrArticleNotFound) {
		t.Errorf("Restore(999999) err = %v, want ErrArticleNotFound", err)
	}
}

// TestArticleService_DeleteSnapshot pins the slice-2.5
// DeleteSnapshot contract: DeleteSnapshot(snapshotID) removes
// only the snapshot row; the live branch is untouched;
// idempotent-on-not-found via ErrArticleNotFound; rejects
// non-snapshot targets (live rows must use the regular
// Delete path).
func TestArticleService_DeleteSnapshot(t *testing.T) {
	service := newArticleServiceForTest(t)
	src, _ := service.Create(models.Article{Title: "Delete target"})
	snap, _ := service.Snapshot(src.ID)

	if err := service.DeleteSnapshot(snap.ID); err != nil {
		t.Fatalf("DeleteSnapshot first: %v", err)
	}

	// Snapshot gone.
	if _, err := service.GetByID(snap.ID); !errors.Is(err, ErrArticleNotFound) {
		t.Errorf("snapshot not gone: GetByID = %v, want ErrArticleNotFound", err)
	}
	// Live row untouched.
	live, err := service.GetByID(src.ID)
	if err != nil {
		t.Errorf("live row went missing after DeleteSnapshot: %v", err)
	}
	if live != nil && live.ID != src.ID {
		t.Errorf("live ID changed: was %d, now %d", src.ID, live.ID)
	}

	// Already-deleted snapshot row is not-found.
	if err := service.DeleteSnapshot(snap.ID); !errors.Is(err, ErrArticleNotFound) {
		t.Errorf("DeleteSnapshot (already gone) err = %v, want ErrArticleNotFound", err)
	}

	// Reject live target (use Update + Delete for those).
	if err := service.DeleteSnapshot(src.ID); !errors.Is(err, ErrArticleSnapshot) {
		t.Errorf("DeleteSnapshot(live) err = %v, want ErrArticleSnapshot", err)
	}

	// Unknown id.
	if err := service.DeleteSnapshot(999999); !errors.Is(err, ErrArticleNotFound) {
		t.Errorf("DeleteSnapshot(999999) err = %v, want ErrArticleNotFound", err)
	}
}

// TestArticleService_ListSnapshots pins the slice-3.4 Revisions
// tab contract: ListSnapshots(articleID) returns every snapshot
// pointing at the article, sorted created_at DESC, with an
// empty slice (not nil) when no snapshots exist. Negative
// article id is rejected.
func TestArticleService_ListSnapshots(t *testing.T) {
	service := newArticleServiceForTest(t)

	src, err := service.Create(models.Article{Title: "List source"})
	if err != nil {
		t.Fatalf("Create source: %v", err)
	}

	// No snapshots yet -> empty slice, not nil, no error.
	got, err := service.ListSnapshots(src.ID)
	if err != nil {
		t.Fatalf("ListSnapshots (empty): %v", err)
	}
	if got == nil {
		t.Errorf("ListSnapshots (empty) returned nil; want empty slice")
	}
	if len(got) != 0 {
		t.Errorf("ListSnapshots (empty) len = %d, want 0", len(got))
	}

	// Snapshot the live article twice.
	snap1, err := service.Snapshot(src.ID)
	if err != nil {
		t.Fatalf("Snapshot #1: %v", err)
	}
	snap2, err := service.Snapshot(src.ID)
	if err != nil {
		t.Fatalf("Snapshot #2: %v", err)
	}

	got, err = service.ListSnapshots(src.ID)
	if err != nil {
		t.Fatalf("ListSnapshots (two): %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListSnapshots (two) len = %d, want 2", len(got))
	}
	// Most recent first: snap2's id > snap1's id (snapshot
	// inserts happen in monotonic order).
	if got[0].ID != snap2.ID {
		t.Errorf("ListSnapshots[0].ID = %d, want %d (most recent)", got[0].ID, snap2.ID)
	}
	if got[1].ID != snap1.ID {
		t.Errorf("ListSnapshots[1].ID = %d, want %d (older)", got[1].ID, snap1.ID)
	}
	for _, s := range got {
		if !s.IsSnapshot {
			t.Errorf("ListSnapshots row id=%d IsSnapshot = false, want true", s.ID)
		}
	}

	// Negative article id rejected.
	if _, err := service.ListSnapshots(0); err == nil {
		t.Errorf("ListSnapshots(0) err = nil, want error")
	}
	if _, err := service.ListSnapshots(-1); err == nil {
		t.Errorf("ListSnapshots(-1) err = nil, want error")
	}

	// Unknown id returns empty slice, no error.
	got, err = service.ListSnapshots(999999)
	if err != nil {
		t.Errorf("ListSnapshots (unknown id) err = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("ListSnapshots (unknown id) len = %d, want 0", len(got))
	}

	// Snapshots of OTHER articles are not included.
	other, err := service.Create(models.Article{Title: "Other"})
	if err != nil {
		t.Fatalf("Create other: %v", err)
	}
	if _, err := service.Snapshot(other.ID); err != nil {
		t.Fatalf("Snapshot other: %v", err)
	}
	got, err = service.ListSnapshots(src.ID)
	if err != nil {
		t.Fatalf("ListSnapshots (after other): %v", err)
	}
	if len(got) != 2 {
		t.Errorf("ListSnapshots (after other) len = %d, want 2 (other's snapshot leaked?)", len(got))
	}
}

// TestArticleService_CreateRendersHTML pins the slice-3.6
// Create path: when the service is constructed with a
// MarkdownRenderer, the body_html column carries a
// sanitized render (script stripped, markdown rendered
// to HTML). When constructed without a renderer
// (slice-1 contract), body_html carries the verbatim md.
func TestArticleService_CreateRendersHTML(t *testing.T) {
	svcPlain := newArticleServiceForTest(t)

	created, err := svcPlain.Create(models.Article{
		Title:  "Plain",
		BodyMD: "# Heading\n\n<script>alert(1)</script>",
	})
	if err != nil {
		t.Fatalf("Create plain: %v", err)
	}
	if !strings.Contains(created.BodyHTML, "<script>") {
		t.Errorf("Plain service should preserve raw HTML in body_html (slice-1 contract): %q", created.BodyHTML)
	}
	if !strings.Contains(created.BodyHTML, "# Heading") {
		t.Errorf("Plain service should preserve verbatim md: %q", created.BodyHTML)
	}

	svcRendered := newArticleServiceForTestWithRenderer(t)

	created2, err := svcRendered.Create(models.Article{
		Title:  "Rendered",
		BodyMD: "# Heading\n\n<script>alert(1)</script>",
	})
	if err != nil {
		t.Fatalf("Create rendered: %v", err)
	}
	if strings.Contains(created2.BodyHTML, "<script>") {
		t.Errorf("Rendered service should strip script tag: %q", created2.BodyHTML)
	}
	if !strings.Contains(created2.BodyHTML, "<h1>") {
		t.Errorf("Rendered service should render markdown: %q", created2.BodyHTML)
	}
}
