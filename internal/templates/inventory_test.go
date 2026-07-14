// inventory_test.go — regression net for issue #491's
// /inventory page. Asserts the page renders the full Local
// Archive rollup (Person Record subtypes + Event Records +
// Articles + Tags) with the per-kind drilldown sections, the
// clickable headline cards, and the crosslink to /insights.
package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

// TestInventoryView_RendersHeadlineCards pins the 6 headline
// cards the page surfaces (3 Person Record subtypes + 3
// non-Person categories: Event Records / Articles / Tags).
// Each card carries a drilldown href that lands the reader
// on the matching listing page.
func TestInventoryView_RendersHeadlineCards(t *testing.T) {
	view := viewmodel.InventoryView{
		Counts: viewmodel.ArchiveCounts{
			SoldierCount:       605,
			SpouseRecordCount:  58,
			PersonRecordCount:  2,
			EventRecordCount:   12,
			ArticleRecordCount: 3,
			TagCount:           8,
		},
	}
	var buf bytes.Buffer
	if err := InventoryView(view).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	cards := []struct {
		label string
		href  string
	}{
		{"Soldiers", "/browse?entry_type=soldier"},
		{"Spouse Records", "/browse?entry_type=spouse"},
		{"Linked Persons", "/browse?entry_type=linked_person"},
		{"Event Records", "/events"},
		{"Articles", "/articles"},
		{"Tags", "/tags"},
	}
	for _, card := range cards {
		if !strings.Contains(content, card.label) {
			t.Errorf("inventory page missing card label %q", card.label)
		}
		if !strings.Contains(content, card.href) {
			t.Errorf("inventory page missing drilldown href %q for card %q", card.href, card.label)
		}
	}
}

// TestInventoryView_HeadlineCountText pins the headline
// number rendered as "{N} archive entries" (TotalEntities()).
// The "Across {N} tags" subtitle also surfaces and must
// match the TagCount.
func TestInventoryView_HeadlineCountText(t *testing.T) {
	view := viewmodel.InventoryView{
		Counts: viewmodel.ArchiveCounts{
			SoldierCount:       10,
			SpouseRecordCount:  5,
			PersonRecordCount:  2,
			EventRecordCount:   3,
			ArticleRecordCount: 1,
			TagCount:           4,
		},
	}
	var buf bytes.Buffer
	if err := InventoryView(view).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	// TotalEntities() = 10+5+2+3+1 = 21.
	if !strings.Contains(content, "21 archive entries") {
		t.Errorf("inventory page missing '21 archive entries' headline (TotalEntities() = 21; got HTML head: %q)", strings.SplitN(content, "\n", 5))
	}
	if !strings.Contains(content, "Across 4 tags") {
		t.Errorf("inventory page missing 'Across 4 tags' subtitle")
	}
}

// TestInventoryView_PerKindRollup pins the per-kind
// drilldown sections. The page surfaces one per-kind row per
// Event Record kind, one row per live Article, and one pill
// per Tag. Empty-state branches (count == 0) are independently
// tested below.
func TestInventoryView_PerKindRollup(t *testing.T) {
	view := viewmodel.InventoryView{
		Counts: viewmodel.ArchiveCounts{
			SoldierCount:       5,
			SpouseRecordCount:  1,
			PersonRecordCount:  1,
			EventRecordCount:   4,
			ArticleRecordCount: 2,
			TagCount:           3,
		},
		EventKinds: []viewmodel.InventoryKindCount{
			{Label: "Battle", Count: 3},
			{Label: "Campaign", Count: 1},
		},
		ArticleRefs: []viewmodel.InventoryKindCount{
			{Label: "On the Battle of Gettysburg"},
			{Label: "Letters from the front"},
		},
		TagKinds: []viewmodel.InventoryKindCount{
			{Label: "Gettysburg"},
			{Label: "1st Texas"},
			{Label: "virtual-cemetery"},
		},
	}
	var buf bytes.Buffer
	if err := InventoryView(view).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	// Per-kind rollup section.
	if !strings.Contains(content, "Per-kind rollup") {
		t.Errorf("inventory page missing per-kind rollup section")
	}
	for _, kind := range []string{"Battle", "Campaign"} {
		if !strings.Contains(content, kind) {
			t.Errorf("inventory page missing per-kind row %q", kind)
		}
	}
	// Articles list.
	if !strings.Contains(content, "Articles") {
		t.Errorf("inventory page missing Articles section")
	}
	for _, article := range []string{"On the Battle of Gettysburg", "Letters from the front"} {
		if !strings.Contains(content, article) {
			t.Errorf("inventory page missing article row %q", article)
		}
	}
	// Tags pill cloud.
	if !strings.Contains(content, "Tag names") {
		t.Errorf("inventory page missing Tags section")
	}
	for _, tag := range []string{"Gettysburg", "1st Texas", "virtual-cemetery"} {
		if !strings.Contains(content, tag) {
			t.Errorf("inventory page missing tag pill %q", tag)
		}
	}
}

// TestInventoryView_EmptyStateRollupsHidden pins the
// empty-state branches: when a category has count == 0, the
// per-kind rollup section is hidden (so an empty archive
// doesn't render three "No rows" headers).
func TestInventoryView_EmptyStateRollupsHidden(t *testing.T) {
	view := viewmodel.InventoryView{
		Counts: viewmodel.ArchiveCounts{
			SoldierCount:       5,
			SpouseRecordCount:  0,
			PersonRecordCount:  0,
			EventRecordCount:   0,
			ArticleRecordCount: 0,
			TagCount:           0,
		},
	}
	var buf bytes.Buffer
	if err := InventoryView(view).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	for _, hidden := range []string{"Event Records by Kind", "Live articles", "Tag names"} {
		if strings.Contains(content, hidden) {
			t.Errorf("inventory page rendered %q for an empty category (section should be hidden)", hidden)
		}
	}
	// With soldiers > 0, the zero-state card is hidden.
	if strings.Contains(content, "Your Local Archive is empty") {
		t.Errorf("inventory page rendered zero-state card while entities > 0")
	}
}

// TestInventoryView_EmptyArchiveZeroState pins the
// zero-state branch: when both TotalEntities() and TagCount are
// 0, the page surfaces a "Your Local Archive is empty" card
// instead of the per-kind rollup sections.
func TestInventoryView_EmptyArchiveZeroState(t *testing.T) {
	view := viewmodel.InventoryView{
		Counts: viewmodel.ArchiveCounts{}, // all zeros
	}
	var buf bytes.Buffer
	if err := InventoryView(view).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	if !strings.Contains(content, "Your Local Archive is empty") {
		t.Errorf("inventory page missing the zero-state card when both entities and tags are 0")
	}
}

// TestInventoryView_CrosslinkToInsights pins the
// /insights crosslink the page surfaces. The inventory page
// is the basic rollup; per-attribute analytics live on
// /insights. The crosslink keeps the two surfaces linked.
func TestInventoryView_CrosslinkToInsights(t *testing.T) {
	view := viewmodel.InventoryView{
		Counts: viewmodel.ArchiveCounts{SoldierCount: 1},
	}
	var buf bytes.Buffer
	if err := InventoryView(view).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	if !strings.Contains(content, `href="/insights"`) {
		t.Errorf("inventory page missing crosslink href to /insights (basic rollup -> per-attribute analytics)")
	}
}

// TestArchiveCounts_TotalEntities pins the total-entity
// counter that powers the "21 archive entries" headline. The
// sum is soldiers + spouse records + linked persons + event
// records + articles (Tags are a labeling primitive, not an
// archive entry, and don't count).
func TestArchiveCounts_TotalEntities(t *testing.T) {
	c := viewmodel.ArchiveCounts{
		SoldierCount:       605,
		SpouseRecordCount:  58,
		PersonRecordCount:  2,
		EventRecordCount:   12,
		ArticleRecordCount: 3,
		TagCount:           8,
	}
	// TotalEntities = 605+58+2+12+3 = 680. Tags are excluded.
	if got := c.TotalEntities(); got != 680 {
		t.Errorf("TotalEntities() = %d; want 680 (Tags are a labeling primitive, not an archive entry)", got)
	}
	// TotalRecords is the older sum (Person Record subtypes only) — used
	// by the empty-state partial. Should still be 605+58+2 = 665.
	if got := c.TotalRecords(); got != 665 {
		t.Errorf("TotalRecords() = %d; want 665 (Person Record subtypes only)", got)
	}
}

// ---- Issue #580: Inventory activity metrics section ----

// TestInventoryView_RendersMetricsSection pins the new
// activity rollup section. When the Metrics carries at least
// one active day, the page renders the summary dates + the
// per-day table. The data-inventory-metrics anchor is the
// audit hook.
func TestInventoryView_RendersMetricsSection(t *testing.T) {
	view := viewmodel.InventoryView{
		Counts: viewmodel.ArchiveCounts{SoldierCount: 3},
		Metrics: viewmodel.InventoryMetrics{
			EntriesPerDay: map[string]int{
				"2026-07-10": 1,
				"2026-07-12": 2,
			},
			FirstEntryDate:  "2026-07-10",
			LatestEntryDate: "2026-07-12",
			ActiveDayCount:  2,
			TotalsByType: viewmodel.InventoryMetricTotals{
				Soldiers: 3,
			},
		},
	}
	var buf bytes.Buffer
	if err := InventoryView(view).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	for _, want := range []string{
		`data-inventory-metrics`,
		`Activity metrics`,
		`data-inventory-metrics-first`,
		"2026-07-10",
		`data-inventory-metrics-latest`,
		"2026-07-12",
		`data-inventory-metrics-active-days`,
		"2", // active day count
		`data-inventory-metrics-days`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("inventory page missing %q in metrics render", want)
		}
	}
}

// TestInventoryView_MetricsOmitsTableForSingleDay pins the
// single-day state: the per-day table is suppressed (the
// summary dates above already convey the single-day state).
// Without this suppression a one-row table reads as noise.
func TestInventoryView_MetricsOmitsTableForSingleDay(t *testing.T) {
	view := viewmodel.InventoryView{
		Counts: viewmodel.ArchiveCounts{SoldierCount: 1},
		Metrics: viewmodel.InventoryMetrics{
			EntriesPerDay:   map[string]int{"2026-07-14": 1},
			FirstEntryDate:  "2026-07-14",
			LatestEntryDate: "2026-07-14",
			ActiveDayCount:  1,
			TotalsByType: viewmodel.InventoryMetricTotals{
				Soldiers: 1,
			},
		},
	}
	var buf bytes.Buffer
	if err := InventoryView(view).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	if !strings.Contains(content, `data-inventory-metrics`) {
		t.Errorf("inventory page missing metrics section anchor in single-day state")
	}
	if strings.Contains(content, `data-inventory-metrics-days`) {
		t.Errorf("inventory page rendered per-day table for a single-day state; the table should suppress when len(EntriesPerDay) <= 1")
	}
}

// TestInventoryView_MetricsOrderIsChronological pins the
// storage contract: the per-day bucket keys are YYYY-MM-DD
// strings (lexicographic sort = chronological sort). Verify
// the render output orders the day rows chronologically.
func TestInventoryView_MetricsOrderIsChronological(t *testing.T) {
	view := viewmodel.InventoryView{
		Counts: viewmodel.ArchiveCounts{SoldierCount: 6},
		Metrics: viewmodel.InventoryMetrics{
			EntriesPerDay: map[string]int{
				"2026-06-15": 1,
				"2026-05-01": 1,
				"2026-06-30": 1,
			},
			FirstEntryDate:  "2026-05-01",
			LatestEntryDate: "2026-06-30",
			ActiveDayCount:  3,
			TotalsByType: viewmodel.InventoryMetricTotals{
				Soldiers: 3,
			},
		},
	}
	var buf bytes.Buffer
	if err := InventoryView(view).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	// Slice the content to the per-day table so the summary
	// dates card (which lists First entry + Latest entry) doesn't
	// pull the first occurrence out of order.
	tableStart := strings.Index(content, `data-inventory-metrics-days`)
	if tableStart < 0 {
		t.Fatalf("per-day table marker not found in rendered output")
	}
	table := content[tableStart:]
	p05 := strings.Index(table, "2026-05-01")
	p615 := strings.Index(table, "2026-06-15")
	p630 := strings.Index(table, "2026-06-30")
	if p05 < 0 || p615 < 0 || p630 < 0 {
		t.Fatalf("not all day keys in per-day table: may=%d jun15=%d jun30=%d", p05, p615, p630)
	}
	if !(p05 < p615 && p615 < p630) {
		t.Errorf("day rows not chronological: may=%d jun15=%d jun30=%d", p05, p615, p630)
	}
}

// TestInventoryView_EmptyArchiveOmitsMetrics pins the empty
// archive state: no per-kind drilldowns, no per-day table,
// just the zero-state card. ActiveDayCount=0 hides the
// section header per the templ branch.
func TestInventoryView_EmptyArchiveOmitsMetrics(t *testing.T) {
	view := viewmodel.InventoryView{
		Counts: viewmodel.ArchiveCounts{},
	}
	var buf bytes.Buffer
	if err := InventoryView(view).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	if strings.Contains(content, `data-inventory-metrics`) {
		t.Errorf("inventory page rendered the metrics section for an empty archive (ActiveDayCount=0 should suppress it)")
	}
}
