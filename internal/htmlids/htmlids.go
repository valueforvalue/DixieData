// Package htmlids declares the stable string identifiers that
// DixieData templ files emit as `id="..."` attributes and that
// htmxattr.Mux targets reference via `hx-target="#..."`.
//
// htmlids is intentionally separate from the uiids package: uiids
// tracks logical surface identifiers (dotted notation, used by the
// breadcrumb / dev badge / debug toolbox for telemetry), while
// htmlids tracks the literal HTML id strings that selectors point
// at. A future refactor might unify them, but they answer
// different questions today (what IS the page vs WHAT is the
// element id) and conflating them would force one notation to
// leak into the other.
//
// Slots in the registry grow alongside htmxattr.Mux call sites
// that target them. Adding a new Mux{Target: "#X"} requires
// registering X here so the dev-build panic in validateTarget
// does not fire.
package htmlids

const (
	// BrowseResults is the id of the browse results table panel
	// that browse.templ emits. Targeted by htmx polling on the
	// filter form (browse.templ:38).
	BrowseResults = "browse-results"

	// SoldierList is the id of the quick-search results table
	// that soldier_card.templ emits. Targeted by the input
	// throttled poll (soldier_card.templ:95) and the alphabet
	// browse button (soldier_card.templ:105).
	SoldierList = "soldier-list"
)

// Selector describes one registered HTML id. Mirrors uiids.Surface
// but uses dashed IDs instead of dotted ones.
type Selector struct {
	ID          string
	Kind        string
	Description string
}

// Registry is the canonical inventory of HTML ids DixieData
// templates emit and htmxattr.Mux targets reference. Has(id)
// reports membership.
var Registry = []Selector{
	{ID: BrowseResults, Kind: "results-region", Description: "Browse results panel on /browse (targeted by the filter form htmx poll)."},
	{ID: SoldierList, Kind: "results-region", Description: "Quick-search results panel on /soldiers (targeted by the input poll + alphabet browse button)."},
}

// Has reports whether id is a registered selector. Called by
// htmxattr.validateTarget at render time; an unknown id fires a
// dev-build panic so the typo is caught before the page ships.
func Has(id string) bool {
	for _, s := range Registry {
		if s.ID == id {
			return true
		}
	}
	return false
}
