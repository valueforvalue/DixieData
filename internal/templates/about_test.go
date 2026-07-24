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
// TestAboutViewRendersFiveSections pins the post-#598 +
// post-#564 slice 1 surface: 5 sections, not 4. The
// Release history section is gone (the recent-commits
// section, #594, replaces "what just landed"); the new
// Glossary section renders the full canonical term
// registry.
func TestAboutViewRendersFiveSections(t *testing.T) {
	view := viewmodel.AboutView{
		AppName:    "DixieData",
		Version:    "1.1.4",
		Codename:   "First Manassas",
		Schema:     67,
		Commit:     "abc123",
		Branch:     "stable",
		BuiltAt:    "2026-07-15T00:00:00Z",
		LicenseURL: "https://github.com/valueforvalue/DixieData/blob/abc123/LICENSE",
	}
	var buf bytes.Buffer
	if err := AboutView(view).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	for _, want := range []string{
		`id="about.identity"`,
		`id="about.license"`,
		`id="about.glossary"`,
		`id="about.recent"`,
		`id="about.activity"`,
		`data-about-identity`,
		`data-about-license`,
		`data-about-glossary`,
		`data-about-recent`,
		`data-about-activity`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("about page missing %q", want)
		}
	}
	// Defensive RED: the release history section is gone.
	if strings.Contains(content, `id="about.history"`) {
		t.Errorf("about page still renders the release history section — issue #598 (dropped)")
	}
}

// TestAboutViewInPageNav pins the on-this-page nav strip.
// Five anchor links to the section IDs (issue #598: Release
// history section is gone; issue #564: Glossary section is
// added between License + credits and Recent commits).
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
		`href="#about.glossary"`,
		`href="#about.recent"`,
		`href="#about.activity"`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("in-page nav missing %q", want)
		}
	}
	// Defensive RED: the release history nav link is gone.
	if strings.Contains(content, `href="#about.history"`) {
		t.Errorf("in-page nav still links to #about.history — issue #598 (dropped)")
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

// TestAboutViewNoReleaseHistorySection pins the issue #598
// post-drop surface: the Release history section is gone.
// The templ no longer renders `data-about-history` or any
// `data-about-release` card. The recent commits section
// (#594) is the replacement for "what just landed".
//
// RED (before Slice 3 lands): the templ still renders
// `aboutHistorySection`; this test fails.
// GREEN (after Slice 3 lands): the section is gone.
func TestAboutViewNoReleaseHistorySection(t *testing.T) {
	view := viewmodel.AboutView{
		AppName: "DixieData", Version: "1.1.4", Codename: "First Manassas", Schema: 67,
	}
	var buf bytes.Buffer
	if err := AboutView(view).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	for _, want := range []string{
		`id="about.history"`,
		`data-about-history`,
		`data-about-history-empty`,
		`data-about-history-collapse`,
		`data-about-release`,
		`data-about-nav-history`,
	} {
		if strings.Contains(content, want) {
			t.Errorf("about page still renders %q after issue #598 (release history dropped)", want)
		}
	}
}
// TestAboutViewRendersGlossarySection pins the Glossary
// section's render contract (issue #564 slice 1 part 2).
//
// RED (this commit lands before the templ emits the
// section): the templ renders no `id="about.glossary"` or
// `data-about-glossary`, so this test's positive assertions
// fail. GREEN lands in the same slice's part 3 when the
// templ gains the @aboutGlossarySection call.
//
// Contract:
//   - The section anchor `id="about.glossary"` is present.
//   - The wrapper `data-about-glossary` hook is present.
//   - At least one row renders with `id="about.glossary-<slug>"`
//     (the anchor pattern the related-term cross-links use).
//   - Every row renders the kebab-Case-slug `data-about-glossary-term="<slug>"`
//     hook for the future disclosure popover's click target.
func TestAboutViewRendersGlossarySection(t *testing.T) {
	// Build a fixture: 3 terms covering the registry contract.
	// The full registry has 36 terms; this fixture exercises
	// the same render code path without bringing every term
	// into the test file.
	view := viewmodel.AboutView{
		AppName: "DixieData", Version: "1.1.4", Codename: "First Manassas", Schema: 67,
		Glossary: []viewmodel.GlossaryEntry{
			{Slug: "person-record", Term: "Person Record", Short: "A primary archive entry.", Full: "A primary archive entry for one person."},
			{Slug: "display-id", Term: "Display ID", Short: "The canonical user-facing identifier.", Full: "The canonical user-facing identifier."},
			{Slug: "shared-archive", Term: "Shared Archive", Short: "A merge-oriented archive package.", Full: "A merge-oriented archive package exchanged between users."},
		},
	}
	var buf bytes.Buffer
	if err := AboutView(view).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	for _, want := range []string{
		`id="about.glossary"`,
		`data-about-glossary`,
		`id="about.glossary-person-record"`,
		`id="about.glossary-display-id"`,
		`id="about.glossary-shared-archive"`,
		`data-about-glossary-term="person-record"`,
		`data-about-glossary-term="display-id"`,
		`data-about-glossary-term="shared-archive"`,
		`>Person Record<`,
		`>Display ID<`,
		`>Shared Archive<`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("about page missing glossary marker %q (issue #564 slice 1)", want)
		}
	}
}

// TestAboutViewActivityHeatmapEmitsHostAndLoading pins the
// templ-side contract that the JS-side heatmap renderer
// (issue #601) consumes. The templ MUST emit:
//
//   - `data-about-activity-heatmap` on the host div (the
//     renderer reads this to find the element).
//   - `data-about-activity-heatmap-data="{ ... }"` on the
//     same host div (the renderer reads the JSON payload
//     from this attribute).
//   - `data-about-activity-heatmap-loading` on the inner
//     placeholder paragraph (the renderer removes it after
//     painting the SVG).
//
// The templ contract pins the data plumbing; the JS
// renderer in `frontend/app.js::paintAboutActivityHeatmap`
// is a separate concern. The slice-1 RED probe
// (`audit/smoke_about.mjs`) asserts the JS side; this
// test pins the templ side so a future refactor of the
// attribute names trips the test before the JS
// silently no-ops.
func TestAboutViewActivityHeatmapEmitsHostAndLoading(t *testing.T) {
	view := viewmodel.AboutView{
		AppName: "DixieData", Version: "1.1.4", Codename: "First Manassas", Schema: 67,
		Activity: &viewmodel.ActivitySnapshotView{
			GeneratedAt:      "2026-07-20",
			FirstCommitDate:  "2025-07-15",
			LatestCommitDate: "2026-07-15",
			TotalCommits:     1213,
			TotalContributors: 7,
			HeatmapData:      `{"2026-07-15":3,"2026-07-14":1}`,
			TopContributors:  nil,
			PerRelease:       nil,
			IssuesClosed:     viewmodel.IssuesClosedView{},
		},
	}
	var buf bytes.Buffer
	if err := AboutView(view).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	for _, want := range []string{
		`data-about-activity-heatmap`,
		`data-about-activity-heatmap-data="`,
		`data-about-activity-heatmap-loading`,
		`Loading heatmap...`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("about templ activity-heatmap wiring missing %q (#601 slice 1)", want)
		}
	}
}

// TestAboutViewRecentListBoundedHeight pins the issue
// #603 contract: the Recent commits <ul> wraps in a
// bounded height + inner scroll viewport so the list
// does not push the rest of /about down when the
// build carries many baked commits.
//
//   - The wrapper carries max-h-96 + sm:max-h-[32rem] +
//     overflow-y-auto so it acts as the scroll viewport.
//   - The data-about-recent-list hook is on the inner
//     <ul> (audit invariant).
//   - Every baked row reaches the DOM (the bounded
//     container clips visually, not the content list).
func TestAboutViewRecentListBoundedHeight(t *testing.T) {
	view := viewmodel.AboutView{
		AppName: "DixieData", Version: "1.1.4", Codename: "First Manassas", Schema: 67,
		RecentCommits: []viewmodel.RecentCommitView{
			{Hash: "abc1234567890def1234567890def1234567890", ShortHash: "abc1234", Date: "2026-07-15", Author: "Jeremy Morris", Subject: "first commit"},
			{Hash: "def2345678901def2345678901def2345678901ab", ShortHash: "def2345", Date: "2026-07-14", Author: "Jeremy Morris", Subject: "second commit"},
			{Hash: "3456789012def3456789012def3456789012def3", ShortHash: "3456789", Date: "2026-07-13", Author: "Jeremy Morris", Subject: "third commit"},
		},
	}
	var buf bytes.Buffer
	if err := AboutView(view).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	for _, want := range []string{
		`data-about-recent-list`,
		`max-h-96`,
		`sm:max-h-[32rem]`,
		`overflow-y-auto`,
		`>abc1234<`,
		`>def2345<`,
		`>3456789<`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("about recent-list wrapper missing %q (#603)", want)
		}
	}
}

// TestAboutViewGlossaryBoundedHeight pins the issue
// #604 contract: the Glossary <dl> wraps in a bounded
// height + inner scroll viewport so the registry's
// 36 terms do not push the rest of /about down.
//
//   - The wrapper carries max-h-96 + sm:max-h-[32rem] +
//     overflow-y-auto so it acts as the scroll viewport.
//   - The data-about-glossary-list hook stays on the
//     inner <dl> (audit invariant).
//   - At least one kebab-anchor term reaches the DOM
//     (the bounded container must not drop content).
func TestAboutViewGlossaryBoundedHeight(t *testing.T) {
	view := viewmodel.AboutView{
		AppName: "DixieData", Version: "1.1.4", Codename: "First Manassas", Schema: 67,
		Glossary: []viewmodel.GlossaryEntry{
			{Slug: "person-record", Term: "Person Record", Short: "A primary archive entry.", Full: "A primary archive entry for one person."},
			{Slug: "display-id", Term: "Display ID", Short: "The canonical user-facing identifier.", Full: "The canonical user-facing identifier."},
			{Slug: "shared-archive", Term: "Shared Archive", Short: "A merge-oriented archive package.", Full: "A merge-oriented archive package exchanged between users."},
		},
	}
	var buf bytes.Buffer
	if err := AboutView(view).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	for _, want := range []string{
		`data-about-glossary-list`,
		`max-h-96`,
		`sm:max-h-[32rem]`,
		`overflow-y-auto`,
		`id="about.glossary-person-record"`,
		`id="about.glossary-display-id"`,
		`id="about.glossary-shared-archive"`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("about glossary wrapper missing %q (#604)", want)
		}
	}
}

// TestAboutActivityIssuesClosedUncategorizedBucket pins
// the issue #650 contract: the /about page's
// "Issues closed by type" widget renders the uncategorized
// bucket as a separate bar slice + legend row when the bake
// reports a non-zero UncategorizedCount. The total-count
// line names the uncategorized count so the user reads the
// "Total + uncategorized" composition at a glance (pre-#650
// the unlabeled / non-canonical-Type issues were silently
// dropped from the total).
func TestAboutActivityIssuesClosedUncategorizedBucket(t *testing.T) {
	view := viewmodel.AboutView{
		AppName: "DixieData", Version: "1.1.4", Codename: "First Manassas", Schema: 67,
		Activity: &viewmodel.ActivitySnapshotView{
			GeneratedAt:      "2026-07-20",
			FirstCommitDate:  "2025-07-15",
			LatestCommitDate: "2026-07-15",
			TotalCommits:     1213,
			TotalContributors: 7,
			HeatmapData:      `{}`,
			TopContributors:  nil,
			PerRelease:       nil,
			IssuesClosed: viewmodel.IssuesClosedView{
				TotalClosed:         420,
				UncategorizedCount: 30,
				GeneratedAt:         "2026-07-20T00:00:00Z",
				ByType: []viewmodel.TypeBucket{
					{Type: "bug", Count: 200},
					{Type: "enhancement", Count: 190},
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
		// Total-count line names the uncategorized count.
		`data-about-activity-issues-total-count`,
		`420 closed issues (30 with no Type label).`,
		// Uncategorized bucket reaches the bar markup.
		`data-about-activity-issues-bucket="uncategorized"`,
		// Uncategorized legend row reaches the markup.
		`data-about-activity-issues-legend="uncategorized"`,
		// Existing bug + enhancement buckets still rendered.
		`data-about-activity-issues-bucket="bug"`,
		`data-about-activity-issues-bucket="enhancement"`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("about activity issues-closed uncategorized missing %q (#650)", want)
		}
	}
}

// TestAboutActivityIssuesClosedZeroUncategorized pins the
// "all issues have a Type label" case: when
// UncategorizedCount == 0 the total-count line omits the
// "(N with no Type label)" parenthetical (the simpler
// "N closed issues." form is used) and no uncategorized
// bar slice or legend row reaches the DOM. Issue #650.
func TestAboutActivityIssuesClosedZeroUncategorized(t *testing.T) {
	view := viewmodel.AboutView{
		AppName: "DixieData", Version: "1.1.4", Codename: "First Manassas", Schema: 67,
		Activity: &viewmodel.ActivitySnapshotView{
			GeneratedAt:      "2026-07-20",
			FirstCommitDate:  "2025-07-15",
			LatestCommitDate: "2026-07-15",
			TotalCommits:     1213,
			TotalContributors: 7,
			HeatmapData:      `{}`,
			TopContributors:  nil,
			PerRelease:       nil,
			IssuesClosed: viewmodel.IssuesClosedView{
				TotalClosed:         100,
				UncategorizedCount: 0,
				GeneratedAt:         "2026-07-20T00:00:00Z",
				ByType: []viewmodel.TypeBucket{
					{Type: "bug", Count: 60},
					{Type: "enhancement", Count: 40},
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
		`100 closed issues.`,
		`data-about-activity-issues-bucket="bug"`,
		`data-about-activity-issues-bucket="enhancement"`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("about activity issues-closed zero-uncategorized missing %q (#650)", want)
		}
	}
	for _, banned := range []string{
		`data-about-activity-issues-bucket="uncategorized"`,
		`data-about-activity-issues-legend="uncategorized"`,
		`with no Type label`,
	} {
		if strings.Contains(content, banned) {
			t.Errorf("about activity issues-closed zero-uncategorized should NOT contain %q (#650)", banned)
		}
	}
}
