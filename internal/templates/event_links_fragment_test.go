// Issue #361 slice 2: Event editor's Linked Persons section
// requires an EventLinksListFragment helper that renders the
// per-row linked Person Record rows (or empty state) for use
// in both the initial page render and the post-detach
// fragment swap. Mirrors the EventSourcesListFragment /
// EventTagsListFragment shape (post-#341 fragment-vs-redirect
// wiring; in-place swap target via data-results-target).
//
// RED today (slice 2 not yet landed): EventLinksListFragment
// is undefined; the test fails to compile. After the helper
// lands, the test asserts:
//   - empty state renders the editor-cross-link copy
//   - populated case renders each linked Person's Display ID
//     pill-link + a per-row Unlink button posting to
//     /events/{id}/links/{personId}/detach with the right
//     data-dixie-submit attrs (not components.Button — see
//     #365 bug)
//   - the fragment renders INSIDE the #data-event-links-list
//     div swap target so the JS dispatcher can replace it
//     in place
package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

func TestEventLinksListFragmentRendersLinkedPersonRows(t *testing.T) {
	var buf bytes.Buffer
	err := EventLinksListFragment(519, []viewmodel.PersonRecord{
		{ID: 1, DisplayID: "SOL-00001", FirstName: "Robert", LastName: "Lee"},
		{ID: 2, DisplayID: "SOL-00002", FirstName: "Stonewall", LastName: "Jackson"},
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	body := buf.String()

	// Each linked Person's Display ID + a per-row Unlink button
	// posting to the detach route. The display IDs come from
	// the seed-data default 'SOL-NNNNN' format; the test
	// checks both the display ID substring AND the detach URL.
	for _, p := range []struct {
		name      string
		displayID string
		id        int64
	}{
		{"lee", "SOL-00001", 1},
		{"jackson", "SOL-00002", 2},
	} {
		detachURL := "data-action=\"/events/519/links/" + intToStr(p.id) + "/detach\""
		if !strings.Contains(body, p.displayID) {
			t.Errorf("[%s] fragment missing Display ID %q", p.name, p.displayID)
		}
		if !strings.Contains(body, detachURL) {
			t.Errorf("[%s] fragment missing Unlink button %q", p.name, detachURL)
		}
		if !strings.Contains(body, "data-dixie-submit=\"true\"") {
			t.Errorf("[%s] fragment Unlink button missing data-dixie-submit=\"true\"", p.name)
		}
	}
}

func TestEventLinksListFragmentRendersEmptyState(t *testing.T) {
	var buf bytes.Buffer
	err := EventLinksListFragment(519, nil).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	body := buf.String()
	if !strings.Contains(body, "No linked Person Records") {
		t.Errorf("empty-state copy missing 'No linked Person Records'")
	}
	// Empty state must NOT render any Unlink buttons.
	if strings.Contains(body, "/detach") {
		t.Errorf("empty-state fragment rendered Unlink buttons")
	}
}

func TestEventLinksListFragmentUsesFullPageNavOnUnlink(t *testing.T) {
	var buf bytes.Buffer
	err := EventLinksListFragment(519, []viewmodel.PersonRecord{
		{ID: 1, DisplayID: "SOL-00001"},
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	body := buf.String()
	// Slice 2 decision: full-page nav (X-DixieData-Redirect)
	// on edit-page attach/detach, NOT in-place fragment swap.
	// The fragment must therefore NOT carry a data-results-target
	// (no swap target on the edit page).
	if strings.Contains(body, "data-results-target") {
		t.Errorf("Unlink button carries data-results-target; slice 2 decision is full-page nav, not in-place swap")
	}
}

// intToStr is a local helper to keep the test readable.
func intToStr(i int64) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	neg := i < 0
	if neg {
		i = -i
	}
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}