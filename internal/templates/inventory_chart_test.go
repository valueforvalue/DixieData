// inventory_chart_test.go -- issue #583 slice 2.
//
// Pins the templ contract for the new Activity metrics chart
// container: a chart wrapper + a legend with one chip per kind
// + an active-kinds data attribute that the JS renderer (slice 3)
// reads. The summary strip (first/latest/active-days) keeps the
// existing data-* hooks so the smoke probe + the summary card
// stay green across the rewrite.
//
// Per-kind legend chip order is locked: soldier / spouse / linked
// / event / article. The JS renderer reads the order from the
// data-active-kinds attribute (comma-separated kind list) so
// reorder is a templ change + a JS change, never a guess.

package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

// TestInventoryView_ActivityMetricsChartContainer pins the new
// chart container replaces the per-day UL. Slice 3 paints SVG
// into it; slice 2 only proves the wrapper, the legend, and
// the active-kinds attribute are wired.
func TestInventoryView_ActivityMetricsChartContainer(t *testing.T) {
	view := viewmodel.InventoryView{
		Counts: viewmodel.ArchiveCounts{SoldierCount: 3},
		Metrics: viewmodel.InventoryMetrics{
			EntriesPerDay: map[string]int{
				"2026-07-10": 1,
				"2026-07-12": 2,
			},
			EntriesPerDayByKind: map[string]map[string]int{
				"soldier": {"2026-07-10": 1, "2026-07-12": 2},
				"spouse":  {},
				"linked":  {},
				"event":   {},
				"article": {},
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
		`data-inventory-metrics-chart`,
		`data-inventory-metrics-legend`,
		`data-inventory-metrics-legend-chip="soldier"`,
		`data-inventory-metrics-legend-chip="spouse"`,
		`data-inventory-metrics-legend-chip="linked"`,
		`data-inventory-metrics-legend-chip="event"`,
		`data-inventory-metrics-legend-chip="article"`,
		`data-active-kinds="soldier,spouse,linked,event,article"`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("inventory page missing %q in chart container render", want)
		}
	}
	// The legacy per-day UL hook is gone.
	if strings.Contains(content, `data-inventory-metrics-days`) {
		t.Errorf("inventory page still renders the legacy per-day UL (data-inventory-metrics-days); slice 2 should have replaced it")
	}
}

// TestInventoryView_ActivityMetricsSummaryStrip pins the
// existing summary hooks (first/latest/active-days) survive
// the slice-2 templ refactor. The summary card moves above
// the chart but the data-* hooks do not change.
func TestInventoryView_ActivityMetricsSummaryStrip(t *testing.T) {
	view := viewmodel.InventoryView{
		Counts: viewmodel.ArchiveCounts{SoldierCount: 3},
		Metrics: viewmodel.InventoryMetrics{
			EntriesPerDay: map[string]int{
				"2026-07-10": 1,
				"2026-07-12": 2,
			},
			EntriesPerDayByKind: map[string]map[string]int{
				"soldier": {"2026-07-10": 1, "2026-07-12": 2},
				"spouse":  {},
				"linked":  {},
				"event":   {},
				"article": {},
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
		`data-inventory-metrics-first`,
		"2026-07-10",
		`data-inventory-metrics-latest`,
		"2026-07-12",
		`data-inventory-metrics-active-days`,
		"2",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("inventory page missing summary hook %q in chart render", want)
		}
	}
}

// TestInventoryView_ActivityMetricsChartOrder pins the
// summary-strip-then-chart order so the JS renderer can
// assume the chart wrapper is below the summary. The summary
// must appear before the chart container in the rendered HTML.
func TestInventoryView_ActivityMetricsChartOrder(t *testing.T) {
	view := viewmodel.InventoryView{
		Counts: viewmodel.ArchiveCounts{SoldierCount: 3},
		Metrics: viewmodel.InventoryMetrics{
			EntriesPerDay: map[string]int{
				"2026-07-10": 1,
				"2026-07-12": 2,
			},
			EntriesPerDayByKind: map[string]map[string]int{
				"soldier": {"2026-07-10": 1, "2026-07-12": 2},
				"spouse":  {},
				"linked":  {},
				"event":   {},
				"article": {},
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
	first := strings.Index(content, `data-inventory-metrics-first`)
	chart := strings.Index(content, `data-inventory-metrics-chart`)
	if first < 0 || chart < 0 {
		t.Fatalf("missing summary or chart anchor: first=%d chart=%d", first, chart)
	}
	if first > chart {
		t.Errorf("summary appears AFTER chart in render; expected summary-then-chart order (first=%d chart=%d)", first, chart)
	}
}

// TestInventoryView_ActivityMetricsSingleDayStillRenders pins
// the slice-2 chart container renders even for a single-day
// archive (slice 3's JS renderer must handle this case -- an
// x-axis with one column).
func TestInventoryView_ActivityMetricsSingleDayStillRenders(t *testing.T) {
	view := viewmodel.InventoryView{
		Counts: viewmodel.ArchiveCounts{SoldierCount: 1},
		Metrics: viewmodel.InventoryMetrics{
			EntriesPerDay:   map[string]int{"2026-07-14": 1},
			EntriesPerDayByKind: map[string]map[string]int{
				"soldier": {"2026-07-14": 1},
				"spouse":  {},
				"linked":  {},
				"event":   {},
				"article": {},
			},
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
	if !strings.Contains(content, `data-inventory-metrics-chart`) {
		t.Errorf("inventory page missing chart container in single-day state (JS renderer handles single-column case)")
	}
}

// TestInventoryView_EmptyArchiveStillSuppressesMetrics pins
// the existing empty-archive behavior survives the slice-2
// refactor: ActiveDayCount=0 hides the entire section.
func TestInventoryView_EmptyArchiveStillSuppressesMetrics(t *testing.T) {
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