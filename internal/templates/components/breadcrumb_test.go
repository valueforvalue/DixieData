package components

import (
	"testing"
)

// TestBreadcrumbCrumbs_KeyRoutes asserts the breadcrumb algorithm
// maps the most-trafficked URL paths to the expected crumb
// sequences. Hand-curated from routes.go; new top-level routes
// need a new assertion here AND a new case in the helper, plus
// the JS port in frontend/debug-toolbox.js.
//
// The "witnesses" framing from issue #309: breadcrumb ==
// dixie.page() == dev badge. If they disagree, that's a bug,
// so this test pins down the breadcrumb half so the JS port
// can be kept in sync.
func TestBreadcrumbCrumbs_KeyRoutes(t *testing.T) {
	cases := []struct {
		name string
		path string
		want []string // just the labels, in order
	}{
		{"root", "/", []string{"Home", "Calendar"}},
		{"calendar", "/calendar", []string{"Home", "Calendar"}},
		{"calendar-month", "/calendar/january", []string{"Home", "Calendar", "January"}},
		{"anniversary", "/anniversary/january/15", []string{"Home", "Anniversaries", "January", "15"}},
		{"soldiers-list", "/soldiers", []string{"Home", "Search"}},
		{"soldier-detail", "/soldiers/42", []string{"Home", "Search", "#42"}},
		{"soldier-tags", "/soldiers/42/tags", []string{"Home", "Search", "#42", "Tags"}},
		{"soldiers-new", "/soldiers/new", []string{"Home", "Search", "Add Person"}},
		{"browse", "/browse", []string{"Home", "Browse"}},
		{"browse-results", "/browse/results", []string{"Home", "Browse", "Results"}},
		{"review-queue", "/review-queue", []string{"Home", "Review Queue"}},
		{"compare", "/review-queue/compare/42", []string{"Home", "Review Queue", "Compare"}},
		{"insights", "/insights", []string{"Home", "Insights"}},
		{"share-landing", "/share", []string{"Home", "Share"}},
		{"share-exports", "/share/exports", []string{"Home", "Share", "Exports"}},
		{"share-imports", "/share/imports", []string{"Home", "Share", "Imports"}},
		{"share-sync", "/share/sync", []string{"Home", "Share", "Sync"}},
		{"share-queue", "/share/queue", []string{"Home", "Share", "Queue"}},
		{"tags-list", "/tags", []string{"Home", "Tags"}},
		{"tag-detail", "/tags/42", []string{"Home", "Tags", "Tag"}},
		{"settings", "/settings", []string{"Home", "Settings"}},
		{"jobs-active", "/jobs/active", []string{"Home", "Jobs"}},
		{"jobs-detail", "/jobs/abc123", []string{"Home", "Jobs", "Job abc123"}},
		{"feedback", "/feedback/submit", []string{"Home", "Feedback"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			crumbs := BreadcrumbCrumbs(tc.path)
			got := make([]string, 0, len(crumbs))
			for _, c := range crumbs {
				got = append(got, c.Label)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("len mismatch for %s: got %v want %v", tc.path, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("label[%d] for %s: got %q want %q", i, tc.path, got[i], tc.want[i])
				}
			}
			// Last crumb should be the current page.
			if !crumbs[len(crumbs)-1].IsCurrent {
				t.Errorf("last crumb for %s not marked IsCurrent: %+v", tc.path, crumbs[len(crumbs)-1])
			}
		})
	}
}

// TestBreadcrumbCrumbs_StripsQueryString asserts the algorithm
// drops ?query=before matching so filter state doesn't produce
// two different breadcrumbs for the same logical page.
func TestBreadcrumbCrumbs_StripsQueryString(t *testing.T) {
	a := BreadcrumbCrumbs("/browse/results?q=foo")
	b := BreadcrumbCrumbs("/browse/results")
	if len(a) != len(b) {
		t.Fatalf("len mismatch with query: %v vs %v", a, b)
	}
	for i := range a {
		if a[i].Label != b[i].Label {
			t.Errorf("label[%d] differs with query: %q vs %q", i, a[i].Label, b[i].Label)
		}
	}
}

// TestBreadcrumbCrumbs_UnknownRoute asserts the fallback
// produces a Home + raw-segment crumb rather than returning
// nil (which would crash the template).
func TestBreadcrumbCrumbs_UnknownRoute(t *testing.T) {
	crumbs := BreadcrumbCrumbs("/totally/unknown/path")
	if len(crumbs) < 2 {
		t.Fatalf("unknown route produced %d crumbs; want at least 2 (Home + fallback)", len(crumbs))
	}
	if crumbs[0].Label != "Home" {
		t.Errorf("first crumb %q, want Home", crumbs[0].Label)
	}
}
