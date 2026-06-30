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

// TestLayoutReviewCount_Empty verifies that the handler returns an
// empty body when the archive has no flagged records (no badge to
// render). The layout's hx-swap="innerHTML" then leaves the badge
// container empty.
func TestLayoutReviewCount_Empty(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	resp, err := http.Get(server.URL + "/layout/review-count")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if len(body) != 0 {
		t.Errorf("expected empty body, got: %q", body)
	}
}

// TestLayoutReviewCount_WithFlagged verifies the badge HTML is
// returned with the correct count when records are flagged.
func TestLayoutReviewCount_WithFlagged(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	for i := 0; i < 5; i++ {
		_, err := app.soldiers.Create(models.Soldier{
			DisplayID:   fmt.Sprintf("BADGE-%03d", i),
			FirstName:   "Badge",
			LastName:    fmt.Sprintf("Tester-%d", i),
			NeedsReview: true,
		})
		if err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	resp, err := http.Get(server.URL + "/layout/review-count")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	content := string(body)
	for _, needle := range []string{
		`<span`,
		`bg-[#6f2c26]`,
		`text-[#fff8e7]`,
		`>5<`,
		`aria-label="5 records pending review"`,
	} {
		if !strings.Contains(content, needle) {
			t.Errorf("body missing %q\nbody: %s", needle, content)
		}
	}
}

// TestLayoutReviewCount_CapAt99Plus verifies that counts >= 100
// render as "99+" rather than the literal number, keeping the
// badge narrow.
func TestLayoutReviewCount_CapAt99Plus(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	for i := 0; i < 150; i++ {
		_, err := app.soldiers.Create(models.Soldier{
			DisplayID:   fmt.Sprintf("CAP-%04d", i),
			FirstName:   "Cap",
			LastName:    "Test",
			NeedsReview: true,
		})
		if err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	resp, err := http.Get(server.URL + "/layout/review-count")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	content := string(body)
	if !strings.Contains(content, `>99+<`) {
		t.Errorf("body missing >99+<: %s", content)
	}
}

// TestLayoutReviewCount_OnlyFlaggedCounted verifies that records
// without needs_review=true do not contribute to the count.
func TestLayoutReviewCount_OnlyFlaggedCounted(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	for i := 0; i < 3; i++ {
		_, err := app.soldiers.Create(models.Soldier{
			DisplayID: fmt.Sprintf("CLEAN-%03d", i),
			FirstName: "Clean",
			LastName:  "Tester",
		})
		if err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}
	for i := 0; i < 2; i++ {
		_, err := app.soldiers.Create(models.Soldier{
			DisplayID:   fmt.Sprintf("DIRTY-%03d", i),
			FirstName:   "Dirty",
			LastName:    "Tester",
			NeedsReview: true,
		})
		if err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	resp, err := http.Get(server.URL + "/layout/review-count")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	content := string(body)
	if !strings.Contains(content, `>2<`) {
		t.Errorf("body should contain >2< (only flagged), got: %s", content)
	}
}