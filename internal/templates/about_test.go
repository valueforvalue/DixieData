// about_test.go -- issue #585 slice 4.
//
// Pins the AboutView templ contract:
//   - All three sections render with their anchors (#identity,
//     #license, #history).
//   - In-page nav strip has 3 anchor links.
//   - License section renders the summary + "View full
//     license" link.
//   - Credits section renders all 6 minimal credit rows
//     (pdfium / Typst / htmx / Tailwind / Playwright / Go stdlib).
//   - Release history section: when Releases is nil, the empty-
//     state message renders; when Releases is non-empty, the
//     latest version + date appear in the body.
//   - Per-release bullet preview honors the first-3-of-first-
//     non-empty-subsection budget.

package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

// TestAboutViewRendersAllThreeSections pins the section
// anchors. The in-page nav strip + the cross-section links
// all rely on these IDs.
func TestAboutViewRendersAllThreeSections(t *testing.T) {
	view := viewmodel.AboutView{
		AppName:    "DixieData",
		Version:    "1.1.4",
		Codename:   "First Manassas",
		Schema:     67,
		Commit:     "abc123",
		Branch:     "stable",
		BuiltAt:    "2026-07-15T00:00:00Z",
		LicenseURL: "https://github.com/valueforvalue/DixieData/blob/abc123/LICENSE",
		Releases: []viewmodel.ReleaseEntry{
			{Version: "v1.1.4", Codename: "First Manassas", Date: "2026-07-12", Added: []string{"first feature"}},
		},
	}
	var buf bytes.Buffer
	if err := AboutView(view).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	for _, want := range []string{
		`id="about.identity"`,
		`id="about.license"`,
		`id="about.history"`,
		`id="about.activity"`,
		`data-about-identity`,
		`data-about-license`,
		`data-about-history`,
		`data-about-activity`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("about page missing %q", want)
		}
	}
}

// TestAboutViewInPageNav pins the on-this-page nav strip.
// Three anchor links to the section IDs.
func TestAboutViewInPageNav(t *testing.T) {
	view := viewmodel.AboutView{
		AppName:  "DixieData",
		Version:  "1.1.4",
		Codename: "First Manassas",
		Schema:   67,
	}
	var buf bytes.Buffer
	if err := AboutView(view).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	for _, want := range []string{
		`data-about-in-page-nav`,
		`href="#about.identity"`,
		`href="#about.license"`,
		`href="#about.history"`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("in-page nav missing %q", want)
		}
	}
}

// TestAboutViewLicenseSummaryAndLink pins the locked-decision
// copy: summary (MIT-licensed, free to use/modify/distribute) +
// "View full license" link to the GitHub LICENSE blob.
func TestAboutViewLicenseSummaryAndLink(t *testing.T) {
	view := viewmodel.AboutView{
		AppName:    "DixieData",
		Version:    "1.1.4",
		Codename:   "First Manassas",
		Schema:     67,
		LicenseURL: "https://github.com/valueforvalue/DixieData/blob/abc123/LICENSE",
	}
	var buf bytes.Buffer
	if err := AboutView(view).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	if !strings.Contains(content, "MIT-licensed") {
		t.Errorf("license section missing 'MIT-licensed'")
	}
	if !strings.Contains(content, "free to use, modify, and distribute") {
		t.Errorf("license section missing the 'free to use' summary")
	}
	if !strings.Contains(content, "View full license") {
		t.Errorf("license section missing 'View full license' link")
	}
	if !strings.Contains(content, `href="https://github.com/valueforvalue/DixieData/blob/abc123/LICENSE"`) {
		t.Errorf("license section missing the GitHub LICENSE blob link")
	}
	// Inline full license text is NOT rendered per locked decision.
	if strings.Contains(content, "Permission is hereby granted, free of charge") {
		t.Errorf("license section rendered inline MIT full text; locked decision is summary + link only")
	}
}

// TestAboutViewCreditsList pins the 6 minimal credit rows.
// Each carries name + license + one-line purpose.
func TestAboutViewCreditsList(t *testing.T) {
	view := viewmodel.AboutView{
		AppName: "DixieData", Version: "1.1.4", Codename: "First Manassas", Schema: 67,
	}
	var buf bytes.Buffer
	if err := AboutView(view).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	for _, want := range []string{
		`data-about-credit="pdfium"`,
		`data-about-credit="typst"`,
		`data-about-credit="htmx"`,
		`data-about-credit="tailwindcss"`,
		`data-about-credit="playwright"`,
		`data-about-credit="go-stdlib"`,
		// License labels per the credit entry.
		"Apache 2.0", // pdfium, typst, playwright
		"BSD-2-Clause",
		"MIT",
		"BSD-3-Clause",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("credits section missing %q", want)
		}
	}
}

// TestAboutViewEmptyStateWhenBakedNil pins the dev-build
// empty-state copy. The About page renders a friendly
// amber-tinted notice rather than crashing.
func TestAboutViewEmptyStateWhenBakedNil(t *testing.T) {
	view := viewmodel.AboutView{
		AppName: "DixieData", Version: "1.1.4", Codename: "First Manassas", Schema: 67,
		Releases: nil,
	}
	var buf bytes.Buffer
	if err := AboutView(view).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	if !strings.Contains(content, `data-about-history-empty`) {
		t.Errorf("about page missing empty-state wrapper")
	}
	if !strings.Contains(content, "Release history not yet generated") {
		t.Errorf("about page missing empty-state copy")
	}
	if !strings.Contains(content, "make tpl") {
		t.Errorf("empty-state copy missing the `make tpl` instruction")
	}
}

// TestAboutViewBakedReleaseRenders pins that the latest baked
// release's version + date + first bullet land in the body
// when Releases is non-nil. The preview shows the first 3
// bullets of the first non-empty subsection; the 4th bullet
// lives inside the <details> toggle (so the page does contain
// the 4th bullet text, but it's gated by the expand widget).
func TestAboutViewBakedReleaseRenders(t *testing.T) {
	view := viewmodel.AboutView{
		AppName: "DixieData", Version: "1.1.4", Codename: "First Manassas", Schema: 67,
		Releases: []viewmodel.ReleaseEntry{
			{
				Version:  "v1.1.4",
				Codename: "First Manassas",
				Date:     "2026-07-12",
				Added: []string{
					"a measurable feature",
					"a second measurable feature",
					"a third measurable feature",
					"a fourth measurable feature (overflows the preview budget)",
				},
			},
		},
	}
	var buf bytes.Buffer
	if err := AboutView(view).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	if !strings.Contains(content, "v1.1.4") {
		t.Errorf("about page missing latest release version")
	}
	if !strings.Contains(content, "2026-07-12") {
		t.Errorf("about page missing latest release date")
	}
	if !strings.Contains(content, "a measurable feature") {
		t.Errorf("about page missing first bullet")
	}
	// The expand widget is present so the 4th bullet has a
	// surface to live in.
	if !strings.Contains(content, `data-about-release-expand="v1.1.4"`) {
		t.Errorf("about page missing the expand toggle for the overflow bullet")
	}
	if !strings.Contains(content, "Show all 4 bullets") {
		t.Errorf("about page missing the 'Show all N bullets' summary")
	}
}

// TestAboutViewMaxExpandedReleases pins the collapse behaviour:
// with 12 baked releases, the first 10 render fully expanded
// and releases 11-12 sit inside the "show all" <details>.
func TestAboutViewMaxExpandedReleases(t *testing.T) {
	releases := make([]viewmodel.ReleaseEntry, 12)
	for i := range releases {
		releases[i] = viewmodel.ReleaseEntry{
			Version: "v0.0." + versionItoa(i+1),
			Date:    "2026-01-01",
			Added:   []string{"feature " + versionItoa(i+1)},
		}
	}
	view := viewmodel.AboutView{
		AppName: "DixieData", Version: "1.1.4", Codename: "First Manassas", Schema: 67,
		Releases: releases,
	}
	var buf bytes.Buffer
	if err := AboutView(view).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	if !strings.Contains(content, `data-about-history-collapse`) {
		t.Errorf("about page missing collapse wrapper for releases beyond the expanded count")
	}
	if !strings.Contains(content, "Show all 12 releases") {
		t.Errorf("about page missing the 'show all N releases' summary")
	}
	// The 11th release must be inside the collapse, not at the
	// top level (count top-level data-about-release anchors).
	count := strings.Count(content, `data-about-release="v0.0.`)
	if count != 12 {
		// 10 expanded at top level + the 11th + 12th inside the
		// collapse toggle = 12 total release cards.
		t.Errorf("expected 12 release anchors; got %d", count)
	}
}

// versionItoa is a tiny non-fmt helper for the test.
func versionItoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

// TestAboutViewIdentityFieldsRender pins that every identity
// field the viewmodel carries surfaces in the rendered HTML.
func TestAboutViewIdentityFieldsRender(t *testing.T) {
	view := viewmodel.AboutView{
		AppName: "DixieData", Version: "1.1.4", Codename: "First Manassas",
		Schema: 67, Commit: "abc123", Branch: "stable", BuiltAt: "2026-07-15T00:00:00Z",
	}
	var buf bytes.Buffer
	if err := AboutView(view).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	for _, want := range []string{
		`data-about-app`, "DixieData",
		`data-about-version`, "1.1.4",
		`data-about-codename`, "First Manassas",
		`data-about-schema`, "67",
		`data-about-branch`, "stable",
		`data-about-commit`, "abc123",
		`data-about-built`, "2026-07-15T00:00:00Z",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("identity section missing %q", want)
		}
	}
}

// TestAboutViewActivitySectionEmptyState pins the dev-build
// behaviour: when the activity snapshot is nil (no bake has
// run), the section renders the empty-state notice.
func TestAboutViewActivitySectionEmptyState(t *testing.T) {
	view := viewmodel.AboutView{
		AppName: "DixieData", Version: "1.1.4", Codename: "First Manassas", Schema: 67,
		Activity: nil,
	}
	var buf bytes.Buffer
	if err := AboutView(view).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	if !strings.Contains(content, `data-about-activity-empty`) {
		t.Errorf("about page missing activity empty-state wrapper")
	}
	if !strings.Contains(content, "Repository activity not yet baked") {
		t.Errorf("about page missing activity empty-state copy")
	}
	if !strings.Contains(content, "make tpl") {
		t.Errorf("activity empty-state missing the `make tpl` instruction")
	}
}

// TestAboutViewActivitySectionBaked pins the populated
// shape: the heatmap host, the contributors list, the
// per-release list, and the issues-closed stacked bar all
// render. The heatmap itself is client-rendered SVG (a
// follow-up slice); the host + the data attribute are this
// slice's job.
func TestAboutViewActivitySectionBaked(t *testing.T) {
	view := viewmodel.AboutView{
		AppName: "DixieData", Version: "1.1.4", Codename: "First Manassas", Schema: 67,
		Activity: &viewmodel.ActivitySnapshotView{
			GeneratedAt:       "2026-07-15T00:00:00Z",
			FirstCommitDate:   "2025-07-15",
			LatestCommitDate:  "2026-07-15",
			TotalCommits:      1213,
			TotalContributors: 8,
			HeatmapData:       `{"2026-07-15":3,"2026-07-14":5}`,
			TopContributors: []viewmodel.Contributor{
				{Name: "Jeremy Morris", Count: 900},
				{Name: "dependabot[bot]", Count: 200},
			},
			PerRelease: []viewmodel.ActivityView{
				{Version: "v1.2.55", Date: "2026-06-25", CommitCount: 0, Contributors: 0},
				{Version: "v1.2.54", Date: "2026-06-08", CommitCount: 14, Contributors: 3, LinesAdded: 824, LinesRemoved: 312},
			},
			IssuesClosed: viewmodel.IssuesClosedView{
				TotalClosed: 364,
				GeneratedAt: "2026-07-15T00:00:00Z",
				ByType: []viewmodel.TypeBucket{
					{Type: "enhancement", Count: 200},
					{Type: "bug", Count: 100},
				},
			},
		},
	}
	var buf bytes.Buffer
	if err := AboutView(view).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	for _, want := range []string{
		`data-about-activity-summary`,
		`data-about-activity-commits`,
		`1213`,
		`data-about-activity-first`,
		`2025-07-15`,
		`data-about-activity-latest`,
		`2026-07-15`,
		`data-about-activity-heatmap`,
		`data-about-activity-heatmap-data="{`,
		`Jeremy Morris`,
		`data-about-activity-contributor="Jeremy Morris"`,
		`data-about-activity-contributor="dependabot[bot]"`,
		`data-about-activity-per-release-row="v1.2.54"`,
		`data-about-activity-per-release-row="v1.2.55"`,
		`N/A (tag missing)`,
		`data-about-activity-issues-bucket="enhancement"`,
		`data-about-activity-issues-bucket="bug"`,
		`data-about-activity-issues-legend="enhancement"`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("about page activity section missing %q", want)
		}
	}
}

// TestAboutViewRecentSectionEmptyState pins the issue #594
// dev-build shape: when RecentCommits is nil/empty, the
// section renders the empty-state notice + the
// `data-about-recent-empty` attribute the audit probe
// asserts.
func TestAboutViewRecentSectionEmptyState(t *testing.T) {
	view := viewmodel.AboutView{
		AppName: "DixieData", Version: "1.1.4", Codename: "First Manassas", Schema: 67,
		RecentCommits: nil,
	}
	var buf bytes.Buffer
	if err := AboutView(view).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	if !strings.Contains(content, `id="about.recent"`) {
		t.Errorf("about page missing the about.recent section anchor")
	}
	if !strings.Contains(content, `data-about-recent`) {
		t.Errorf("about page missing the data-about-recent attribute")
	}
	if !strings.Contains(content, `data-about-recent-empty`) {
		t.Errorf("about page missing the recent-commits empty-state wrapper")
	}
	if !strings.Contains(content, "Recent commits will appear after the next build") {
		t.Errorf("about page missing the recent-commits empty-state copy")
	}
	if !strings.Contains(content, `data-about-nav-recent`) {
		t.Errorf("about page in-page nav missing the recent-commits link")
	}
}

// TestAboutViewRecentSectionBaked pins the issue #594
// populated shape: when RecentCommits has entries, the
// section renders one <li> per commit with the short hash
// (anchor text), the full hash (data attr for the audit
// probe), the ISO date, the author, and the subject. Each
// hash + subject link to the GitHub commit permalink.
func TestAboutViewRecentSectionBaked(t *testing.T) {
	view := viewmodel.AboutView{
		AppName: "DixieData", Version: "1.1.4", Codename: "First Manassas", Schema: 67,
		RecentCommits: []viewmodel.RecentCommitView{
			{
				Hash:      "abcdef1234567890abcdef1234567890abcdef12",
				ShortHash: "abcdef1",
				Date:      "2026-07-15",
				Author:    "Jeremy Morris",
				Subject:   "fix: the bug",
			},
			{
				Hash:      "1234567890abcdef1234567890abcdef12345678",
				ShortHash: "1234567",
				Date:      "2026-07-14",
				Author:    "Jane Doe",
				Subject:   "feat: the feature",
			},
		},
	}
	var buf bytes.Buffer
	if err := AboutView(view).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	for _, want := range []string{
		`id="about.recent"`,
		`data-about-recent`,
		`data-about-recent-list`,
		`data-about-recent-row="abcdef1234567890abcdef1234567890abcdef12"`,
		`data-about-recent-row="1234567890abcdef1234567890abcdef12345678"`,
		`data-about-recent-hash`,
		`data-about-recent-date`,
		`data-about-recent-author`,
		`data-about-recent-subject`,
		`Jeremy Morris`,
		`Jane Doe`,
		`fix: the bug`,
		`feat: the feature`,
		`https://github.com/valueforvalue/DixieData/commit/abcdef1234567890abcdef1234567890abcdef12`,
		`https://github.com/valueforvalue/DixieData/commit/1234567890abcdef1234567890abcdef12345678`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("about page recent-commits section missing %q", want)
		}
	}
	// Empty-state wrapper must NOT render when the slice is
	// populated (the templ branches on len()).
	if strings.Contains(content, `data-about-recent-empty`) {
		t.Errorf("about page recent-commits rendered empty-state wrapper despite populated slice")
	}
}