package appshell

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

// TestLayoutOpenReviewMenuitemFlaggedFromCalendar covers the
// follow-up to issue #460: the red review-state treatment on the
// "Open Review Queue" menuitem must ship on the FIRST paint of
// every page (landing /calendar, browse, deep-link to a Person
// Record, etc.) when the archive has pending review items — not
// only on /review-queue itself.
//
// Before the follow-up the flag was set only inside
// handleReviewQueue, so users landing on /calendar or any other
// surface saw a neutral menuitem and didn't realise the red
// number on the R&R foldout trigger was theirs to act on. The
// fix hoists SetLayoutHasOpenReview into the per-request
// lifecycle wrapper (lifecycle.go ServeHTTP) so every response
// that uses Layout inherits the treatment when count > 0.
func TestLayoutOpenReviewMenuitemFlaggedFromCalendar(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	client := server.Client()
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}

	// Seed TWO flagged records so count > 0.
	for i := 0; i < 2; i++ {
		if _, err := app.soldiers.Create(models.Soldier{
			DisplayID:    fmt.Sprintf("FOLLOWUP-%03d", i),
			FirstName:    "First",
			LastName:     fmt.Sprintf("Paint-%d", i),
			EntryType:    "soldier",
			NeedsReview:  true,
			ReviewReason: "Test seed for global flag",
		}); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}

	for _, path := range []string{"/calendar", "/browse", "/soldiers"} {
		path := path
		t.Run(path, func(t *testing.T) {
			resp, err := client.Get(server.URL + path)
			if err != nil {
				t.Fatalf("GET %s: %v", path, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET %s status=%d, want 200", path, resp.StatusCode)
			}
			body, _ := io.ReadAll(resp.Body)
			content := string(body)
			if !strings.Contains(content, `data-research-menu-review-queue`) {
				t.Fatalf("%s response missing the Open Review Queue menuitem", path)
			}
			if !strings.Contains(content, `data-research-review-has-count`) {
				t.Fatalf("%s response missing data-research-review-has-count on the menuitem (the red treatment must land on every page when count > 0)", path)
			}
		})
	}
}

// TestLayoutOpenReviewMenuitemNeutralWhenNoPendingItems covers
// the inverse: when no Person Records are flagged for review,
// the per-request lifecycle clears the menuitem flag and the
// page renders the neutral pill-link surface.
func TestLayoutOpenReviewMenuitemNeutralWhenNoPendingItems(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	client := server.Client()
	resp, err := client.Get(server.URL + "/calendar")
	if err != nil {
		t.Fatalf("GET /calendar: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	content := string(body)
	if !strings.Contains(content, `data-research-menu-review-queue`) {
		t.Fatalf("/calendar response missing the Open Review Queue menuitem")
	}
	if strings.Contains(content, `data-research-review-has-count`) {
		t.Fatalf("/calendar response carried the flag while no records are flagged for review; count was zero so the menuitem should be neutral:\n%s", content)
	}
}
