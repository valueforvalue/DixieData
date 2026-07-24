// bake-activity/main_test.go -- issue #650 regression net.
//
// Pins the structured GitHub /issues parsing + pagination
// contract:
//
//   - Each page's JSON is decoded into ClosedIssue records
//     (number, labels, pull_request discriminator).
//   - Pull requests (pull_request non-null) are flagged.
//   - Pagination terminates on the record count (perPage),
//     not on the flattened label output. A page with all
//     unlabeled issues does NOT stop the loop.
//   - The first page with fewer than perPage records ends
//     pagination.

package main

import (
	"fmt"
	"testing"
)

// TestParseClosedIssuesPage_BasicIssue pins the happy path:
// one issue with one label, no pull_request discriminator.
func TestParseClosedIssuesPage_BasicIssue(t *testing.T) {
	body := []byte(`[{"number":1,"labels":[{"name":"bug"}]}]`)
	got, err := parseClosedIssuesPage(body)
	if err != nil {
		t.Fatalf("parseClosedIssuesPage: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d records; want 1", len(got))
	}
	if got[0].Number != 1 {
		t.Errorf("Number = %d; want 1", got[0].Number)
	}
	if len(got[0].Labels) != 1 || got[0].Labels[0] != "bug" {
		t.Errorf("Labels = %v; want [bug]", got[0].Labels)
	}
	if got[0].IsPullRequest {
		t.Errorf("IsPullRequest = true; want false")
	}
}

// TestParseClosedIssuesPage_PullRequestFlagged pins the PR
// discrimination: a record with a non-null pull_request
// object is flagged. The /about page counts issues only,
// so the downstream aggregation must skip these.
func TestParseClosedIssuesPage_PullRequestFlagged(t *testing.T) {
	body := []byte(`[{"number":2,"labels":[{"name":"bug"}],"pull_request":{"url":"https://api.github.com/.../pulls/2"}}]`)
	got, err := parseClosedIssuesPage(body)
	if err != nil {
		t.Fatalf("parseClosedIssuesPage: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d records; want 1", len(got))
	}
	if !got[0].IsPullRequest {
		t.Errorf("IsPullRequest = false; want true")
	}
}

// TestParseClosedIssuesPage_NoLabels pins the unlabeled
// issue contract: a record with no labels is preserved with
// the empty Labels slice. The downstream aggregation
// counts it in TotalClosed + UncategorizedCount.
func TestParseClosedIssuesPage_NoLabels(t *testing.T) {
	body := []byte(`[{"number":3,"labels":[]}]`)
	got, err := parseClosedIssuesPage(body)
	if err != nil {
		t.Fatalf("parseClosedIssuesPage: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d records; want 1", len(got))
	}
	if len(got[0].Labels) != 0 {
		t.Errorf("Labels = %v; want empty", got[0].Labels)
	}
	if got[0].IsPullRequest {
		t.Errorf("IsPullRequest = true; want false")
	}
}

// TestParseClosedIssuesPage_MultipleLabels pins the
// multi-label contract: every label name is preserved in
// the order it appeared in the source. The downstream
// aggregation picks the alphabetically first canonical
// Type label.
func TestParseClosedIssuesPage_MultipleLabels(t *testing.T) {
	body := []byte(`[{"number":4,"labels":[{"name":"enhancement"},{"name":"bug"},{"name":"area:frontend"}]}]`)
	got, err := parseClosedIssuesPage(body)
	if err != nil {
		t.Fatalf("parseClosedIssuesPage: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d records; want 1", len(got))
	}
	want := []string{"enhancement", "bug", "area:frontend"}
	if len(got[0].Labels) != len(want) {
		t.Fatalf("Labels length = %d; want %d", len(got[0].Labels), len(want))
	}
	for i, w := range want {
		if got[0].Labels[i] != w {
			t.Errorf("Labels[%d] = %q; want %q", i, got[0].Labels[i], w)
		}
	}
}

// TestParseClosedIssuesPage_EmptyArray pins the empty-
// array contract: an empty page returns an empty (non-nil)
// slice. The pagination loop uses this as a termination
// signal.
func TestParseClosedIssuesPage_EmptyArray(t *testing.T) {
	body := []byte(`[]`)
	got, err := parseClosedIssuesPage(body)
	if err != nil {
		t.Fatalf("parseClosedIssuesPage: %v", err)
	}
	if got == nil {
		t.Errorf("parseClosedIssuesPage([]) = nil; want empty slice")
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d; want 0", len(got))
	}
}

// TestParseClosedIssuesPage_MixedRecords pins the
// "issues + PRs in the same page" contract: each record
// is independently tagged.
func TestParseClosedIssuesPage_MixedRecords(t *testing.T) {
	body := []byte(`[
		{"number":1,"labels":[{"name":"bug"}]},
		{"number":2,"labels":[{"name":"bug"}],"pull_request":{"url":"x"}},
		{"number":3,"labels":[]}
	]`)
	got, err := parseClosedIssuesPage(body)
	if err != nil {
		t.Fatalf("parseClosedIssuesPage: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d records; want 3", len(got))
	}
	if got[0].IsPullRequest {
		t.Errorf("record 1 IsPullRequest = true; want false")
	}
	if !got[1].IsPullRequest {
		t.Errorf("record 2 IsPullRequest = false; want true")
	}
	if got[2].IsPullRequest {
		t.Errorf("record 3 IsPullRequest = true; want false")
	}
	if len(got[2].Labels) != 0 {
		t.Errorf("record 3 Labels = %v; want empty", got[2].Labels)
	}
}

// TestFetchClosedIssuesPaged_SingleFullPage pins the
// single-page path: a pages function that returns one
// full page (perPage records) must terminate after the
// second page returns empty (the canonical "you've
// reached the end" signal from the GitHub API).
func TestFetchClosedIssuesPaged_SingleFullPage(t *testing.T) {
	body := buildPageBody(100, 1)
	pages := [][]byte{body, []byte(`[]`)}
	pulled := func(page int) ([]byte, error) {
		if page-1 >= len(pages) {
			return []byte(`[]`), nil
		}
		return pages[page-1], nil
	}
	got, err := fetchClosedIssuesPaged(pulled, 100, 50)
	if err != nil {
		t.Fatalf("fetchClosedIssuesPaged: %v", err)
	}
	if len(got) != 100 {
		t.Errorf("got %d issues; want 100", len(got))
	}
}

// TestFetchClosedIssuesPaged_ShortLastPage pins the
// "last page is short" path: when the upstream returns
// fewer than perPage records, pagination terminates.
// Pinned here because the pre-#650 implementation
// stopped on empty flattened label output (a page with
// all unlabeled issues), which would have over-counted
// early-terminated repos.
func TestFetchClosedIssuesPaged_ShortLastPage(t *testing.T) {
	page1 := buildPageBody(100, 1)
	page2 := buildPageBody(37, 101)
	pages := [][]byte{page1, page2}
	pulled := func(page int) ([]byte, error) {
		if page-1 >= len(pages) {
			return []byte(`[]`), nil
		}
		return pages[page-1], nil
	}
	got, err := fetchClosedIssuesPaged(pulled, 100, 50)
	if err != nil {
		t.Fatalf("fetchClosedIssuesPaged: %v", err)
	}
	if len(got) != 137 {
		t.Errorf("got %d issues; want 137 (100 + 37)", len(got))
	}
}

// TestFetchClosedIssuesPaged_AllUnlabeledPagesDoNotStopLoop
// pins the regression that motivated the page-count
// pagination fix. Pre-#650 the loop terminated on empty
// flattened label output; a page containing ONLY unlabeled
// issues would have produced an empty `.[].labels[].name`
// jq result and stopped the loop. The new pagination
// terminates on perPage records, so an all-unlabeled page
// keeps the loop going.
func TestFetchClosedIssuesPaged_AllUnlabeledPagesDoNotStopLoop(t *testing.T) {
	page1 := buildUnlabeledPage(100)
	page2 := buildPageBody(50, 101) // labeled issues start at 101
	pages := [][]byte{page1, page2}
	pulled := func(page int) ([]byte, error) {
		if page-1 >= len(pages) {
			return []byte(`[]`), nil
		}
		return pages[page-1], nil
	}
	got, err := fetchClosedIssuesPaged(pulled, 100, 50)
	if err != nil {
		t.Fatalf("fetchClosedIssuesPaged: %v", err)
	}
	if len(got) != 150 {
		t.Errorf("got %d issues; want 150 (100 unlabeled + 50 labeled)", len(got))
	}
	// First 100 are unlabeled; next 50 are labeled "bug".
	for i := 0; i < 100; i++ {
		if len(got[i].Labels) != 0 {
			t.Errorf("issue %d: Labels = %v; want empty", i+1, got[i].Labels)
		}
	}
	for i := 100; i < 150; i++ {
		if len(got[i].Labels) != 1 || got[i].Labels[0] != "bug" {
			t.Errorf("issue %d: Labels = %v; want [bug]", i+1, got[i].Labels)
		}
	}
}

// TestFetchClosedIssuesPaged_RespectsMaxPages pins the
// safety cap: the loop must not exceed maxPages even when
// the upstream signal says "more pages exist".
func TestFetchClosedIssuesPaged_RespectsMaxPages(t *testing.T) {
	// Every page returns perPage records; the loop should
	// stop at maxPages even though no empty page arrives.
	pulled := func(page int) ([]byte, error) {
		return buildPageBody(100, (page-1)*100+1), nil
	}
	got, err := fetchClosedIssuesPaged(pulled, 100, 3)
	if err != nil {
		t.Fatalf("fetchClosedIssuesPaged: %v", err)
	}
	if len(got) != 300 {
		t.Errorf("got %d issues; want 300 (maxPages=3 * perPage=100)", len(got))
	}
}

// TestFetchClosedIssuesPaged_EmptyFirstPage pins the
// "no closed issues at all" path: a single empty page
// returns an empty (non-nil) slice.
func TestFetchClosedIssuesPaged_EmptyFirstPage(t *testing.T) {
	pulled := func(page int) ([]byte, error) {
		return []byte(`[]`), nil
	}
	got, err := fetchClosedIssuesPaged(pulled, 100, 50)
	if err != nil {
		t.Fatalf("fetchClosedIssuesPaged: %v", err)
	}
	if got == nil {
		t.Errorf("result is nil; want empty slice")
	}
	if len(got) != 0 {
		t.Errorf("len = %d; want 0", len(got))
	}
}

// buildPageBody emits a JSON array of n labeled issues
// starting at startNumber, each with the "bug" label.
func buildPageBody(n, startNumber int) []byte {
	body := []byte("[")
	for i := 0; i < n; i++ {
		num := startNumber + i
		if i > 0 {
			body = append(body, ',')
		}
		body = append(body, []byte(fmt.Sprintf(`{"number":%d,"labels":[{"name":"bug"}]}`, num))...)
	}
	body = append(body, ']')
	return body
}

// buildUnlabeledPage emits a JSON array of n issues with
// no labels. Used to pin the "all-unlabeled page does not
// stop the loop" invariant.
func buildUnlabeledPage(n int) []byte {
	body := []byte("[")
	for i := 0; i < n; i++ {
		if i > 0 {
			body = append(body, ',')
		}
		body = append(body, []byte(fmt.Sprintf(`{"number":%d,"labels":[]}`, i+1))...)
	}
	body = append(body, ']')
	return body
}
