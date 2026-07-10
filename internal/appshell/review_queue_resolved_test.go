package appshell

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestReviewQueueResolvedTabDispatchesAwayFromOpen exercises the
// data path added in issue #461: handleReviewQueue must dispatch
// on tab=resolved and serve the Resolved tab body, not the Open
// list. Catches regressions where the placeholder is reintroduced
// or the tab query-param is silently ignored.
func TestReviewQueueResolvedTabDispatchesAwayFromOpen(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	client := server.Client()
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}

	// Seed two flagged records so the Open tab has rows.
	for i := 0; i < 2; i++ {
		if _, err := app.soldiers.Create(openTestSoldier(i)); err != nil {
			t.Fatalf("Create flagged %d: %v", i, err)
		}
	}
	// Seed one resolved finding so the Resolved tab has a row.
	if findingID := seedResolvedDuplicateFinding(t, app); findingID == 0 {
		t.Fatal("expected a resolved finding; got id=0")
	}

	openResp, err := client.Get(server.URL + "/review-queue")
	if err != nil {
		t.Fatalf("Open tab GET: %v", err)
	}
	defer openResp.Body.Close()
	if openResp.StatusCode != http.StatusOK {
		t.Fatalf("Open tab status=%d, want 200", openResp.StatusCode)
	}
	openBody, _ := io.ReadAll(openResp.Body)

	resolvedResp, err := client.Get(server.URL + "/review-queue?tab=resolved")
	if err != nil {
		t.Fatalf("Resolved tab GET: %v", err)
	}
	defer resolvedResp.Body.Close()
	if resolvedResp.StatusCode != http.StatusOK {
		t.Fatalf("Resolved tab status=%d, want 200", resolvedResp.StatusCode)
	}
	resolvedBody, _ := io.ReadAll(resolvedResp.Body)

	open := string(openBody)
	resolved := string(resolvedBody)
	if open == resolved {
		t.Fatalf("Open and Resolved tabs returned identical bodies; tab dispatch is broken")
	}
	for _, needle := range []string{
		"Review Queue",
		"data-review-queue-tab=\"open\"",
		"data-review-queue-tab=\"resolved\"",
	} {
		if !strings.Contains(open, needle) {
			t.Errorf("Open tab body missing %q", needle)
		}
	}
	for _, needle := range []string{
		"Duplicate-audit findings",
		`tab="resolved"`,
	} {
		if !strings.Contains(resolved, needle) {
			t.Errorf("Resolved tab body missing %q", needle)
		}
	}
}

// TestReviewQueueResolvedTabEmptyStillRenders asserts the empty-
// state copy (no resolved entries yet) returns a 200 + the
// expected hint text rather than blowing up on the new code path.
func TestReviewQueueResolvedTabEmptyStillRenders(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	client := server.Client()
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}

	resp, err := client.Get(server.URL + "/review-queue?tab=resolved")
	if err != nil {
		t.Fatalf("Resolved tab GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Resolved tab status=%d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	content := string(body)
	if !strings.Contains(content, "No resolved entries yet") {
		t.Errorf("expected empty-state copy; got body:\n%s", content)
	}
}

// openTestSoldier returns a flagged Person Record for Open-tab seeding.
func openTestSoldier(i int) (soldier personRecord) {
	soldier.DisplayID = fmt.Sprintf("RQ-OPEN-%03d", i)
	soldier.FirstName = "Queue"
	soldier.LastName = fmt.Sprintf("Open-%d", i)
	soldier.EntryType = "soldier"
	soldier.NeedsReview = true
	soldier.ReviewReason = "Test seed for Open tab"
	return
}

// seedResolvedDuplicateFinding runs the duplicate audit on two
// similar records, resolves the finding via the
// FindingsForSoldiers lookup, and returns the resulting
// finding ID so the caller can assert the data surfaced on the
// Resolved tab. Returns 0 when nothing was created.
func seedResolvedDuplicateFinding(t *testing.T, app *App) int64 {
	t.Helper()
	left, err := app.soldiers.Create(personRecord{
		DisplayID:    "RQ-RES-LEFT-1",
		FirstName:    "Resolvee",
		LastName:     "Pleasants",
		BirthDate:    "01/01/1840",
		Unit:         "9th OK Infantry",
		EntryType:    "soldier",
		NeedsReview:  true,
		ReviewReason: "Test seed for left pair",
	})
	if err != nil {
		t.Fatalf("seedResolved left: %v", err)
	}
	if _, err := app.soldiers.Create(personRecord{
		DisplayID:    "RQ-RES-RIGHT-1",
		FirstName:    "Resolvey",
		LastName:     "Pleasants",
		BirthDate:    "01/01/1840",
		Unit:         "9th OK Infantry",
		EntryType:    "soldier",
		NeedsReview:  true,
		ReviewReason: "Test seed for right pair",
	}); err != nil {
		t.Fatalf("seedResolved right: %v", err)
	}
	if _, err := app.audit.RunDuplicateAudit(); err != nil {
		t.Fatalf("RunDuplicateAudit: %v", err)
	}
	openFindings, err := app.audit.FindingsForPersonRecords([]int64{left.ID})
	if err != nil {
		t.Fatalf("FindingsForSoldiers: %v", err)
	}
	rows, ok := openFindings[left.ID]
	if !ok || len(rows) == 0 {
		return 0
	}
	if err := app.audit.ResolveFinding(rows[0].ID); err != nil {
		t.Fatalf("ResolveFinding: %v", err)
	}
	return rows[0].ID
}
