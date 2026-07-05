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
