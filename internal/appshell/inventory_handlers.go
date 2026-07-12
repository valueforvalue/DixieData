// inventory_handlers.go holds the /inventory HTTP handler.
//
// Issue #491: the Local Archive inventory page. Carries the full
// archive rollup (Person Record subtypes + Event Records + Articles
// + Tags) at a basic level than the per-attribute analytics on
// /insights. The Calendar header shows 3 Person Record subtype
// cards (Soldiers / Spouse Records / Linked Persons) that drill
// into /browse; /inventory carries the same 3 plus the 3
// non-Person categories (Event Records / Articles / Tags) so a
// reader can see the full DB shape at a glance.
//
// The page is public (no picker-guard, no setup gate beyond the
// normal setupRequired middleware) — it's a static rollup, not a
// workflow launcher. Accessible from the Share & Review mega-
// menu's "Review & Research" group as "Archive Inventory" (issue
// #491).
package appshell

import (
	"context"
	"fmt"
	"net/http"

	"github.com/valueforvalue/DixieData/internal/presentation"
	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

// InventoryPerKindRollup is the per-kind breakdown the inventory
// page renders alongside the headline numbers. Each kind carries
// its count + a single source-of-truth pointer to the matching
// listing page so the inventory cards can drill in.
type InventoryPerKindRollup struct {
	KindLabel  string
	Count      int
	DrillHref  string
	DrillLabel string
}

func (a *App) handleInventory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	counts, err := a.soldiers.ArchiveCounts()
	if err != nil {
		respondInternal(w, r, "Could not load archive counts.", err)
		return
	}
	// Per-Event-Record kind counts (the per-kind drilldown that
	// /insights doesn't carry — /insights is per-attribute, /inventory
	// is per-kind). Cheap: one query that groups by kind. Failures
	// degrade gracefully (empty list, no error toast) so the rest
	// of the page still renders.
	eventKinds := a.inventoryEventKinds(r.Context())
	articleKinds := a.inventoryArticleKinds(r.Context())
	tagKinds := a.inventoryTagKinds(r.Context())
	view := viewmodel.InventoryView{
		Counts: viewmodel.ArchiveCounts{
			SoldierCount:       counts.TotalSoldiers,
			SpouseRecordCount:  counts.TotalWivesWidows,
			PersonRecordCount:  counts.TotalLinkedPeople,
			EventRecordCount:   counts.EventRecords,
			ArticleRecordCount: counts.Articles,
			TagCount:           counts.Tags,
		},
		EventKinds:  eventKinds,
		ArticleRefs: articleKinds,
		TagKinds:    tagKinds,
	}
	// Issue #384-style wrap.
	if err := presentation.InventoryView(view).Render(r.Context(), w); err != nil {
		respondErrorFragment(w, r, KindInternal, "Could not render the archive inventory page.", err)
	}
}

// inventoryEventKinds returns one rollup per Event Record kind
// (Battle, Campaign, Death, etc. — free-text per locked decision
// from issue #320). Returns nil if the events service is unset
// (e.g. NewApp() in the HTTP-only test path that doesn't call
// Startup) so the page degrades to "no events yet" empty state.
func (a *App) inventoryEventKinds(ctx context.Context) []viewmodel.InventoryKindCount {
	if a.events == nil {
		return nil
	}
	rollup, err := a.events.KindRollup()
	if err != nil {
		// Degrade silently — the rest of the page still renders.
		return nil
	}
	_ = ctx
	out := make([]viewmodel.InventoryKindCount, 0, len(rollup))
	for _, item := range rollup {
		out = append(out, viewmodel.InventoryKindCount{
			Label: item.Kind,
			Count: item.Count,
		})
	}
	return out
}

// inventoryArticleKinds returns one rollup per Article (by title
// prefix or by Snapshot vs Live split). Today we render one
// row per live article so the reader can see the titles at a
// glance; the per-Article count is implicit (each row = 1).
// Failures degrade to nil.
func (a *App) inventoryArticleKinds(ctx context.Context) []viewmodel.InventoryKindCount {
	if a.articles == nil {
		return nil
	}
	articles, _, err := a.articles.List(1, 100)
	if err != nil {
		return nil
	}
	_ = ctx
	out := make([]viewmodel.InventoryKindCount, 0, len(articles))
	for _, article := range articles {
		if article.IsSnapshot {
			continue
		}
		label := article.Title
		if label == "" {
			label = fmt.Sprintf("Article %s", article.DisplayID)
		}
		out = append(out, viewmodel.InventoryKindCount{
			Label: label,
			Count: 1,
		})
	}
	return out
}

// inventoryTagKinds returns one rollup per Tag. Distinct from
// the per-Tag member counts on the /tags page — here we just
// want the names so the inventory can list them. Failures
// degrade to nil.
func (a *App) inventoryTagKinds(ctx context.Context) []viewmodel.InventoryKindCount {
	if a.tags == nil {
		return nil
	}
	tags, err := a.tags.List(ctx)
	if err != nil {
		return nil
	}
	out := make([]viewmodel.InventoryKindCount, 0, len(tags))
	for _, tag := range tags {
		out = append(out, viewmodel.InventoryKindCount{
			Label: tag.Name,
			Count: 1,
		})
	}
	return out
}
