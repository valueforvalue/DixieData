// Package glossary is the single source of truth for DixieData's
// domain vocabulary. The registry mirrors the canonical glossary
// in `CONTEXT.md` (the single-source-of-truth document every
// feature, doc, and commit message should match per AGENTS.md).
//
// The registry powers two surfaces:
//
//  1. The full glossary on /about#glossary (templ + JS read),
//     where every term renders with a stable kebab-case anchor.
//  2. The disclosure popover on in-context term mentions
//     (future slice 2), which renders only the Short field.
//
// Both surfaces use the same Term struct, so adding a term is a
// one-file change: drop a Term into the registry, and the /about
// page + every disclosure on a verified apply site pick it up
// the moment the next build runs.
//
// Adding a term: append a new Term literal to Registry(); the
// tests in terms_test.go assert unique Slug, non-empty Fields,
// and valid Anchor. Run `go test ./internal/glossary/` to
// verify; if the term is missing or the slug collides, the build
// fails fast at the unit-test step.
package glossary

import (
	"sort"
	"strings"
)

// Term is one row of the glossary. Short is the one-line
// definition the in-context disclosure popover shows (locked
// decision #3: /about renders Full; disclosures render Short).
// Full is the multi-line paragraph /about renders. Related is
// the list of sibling terms whose kebab-anchor links render at
// the bottom of the term's row (the relationships defined in
// CONTEXT.md "Relationships" section).
//
// Anchor is the kebab-case HTML id (`about.glossary-<slug>`);
// Slug is the kebab term (e.g. "person-record"). The two
// could drift if you change Slug without updating Anchor —
// the unit test in terms_test.go asserts they stay paired.
type Term struct {
	Slug    string
	Term    string
	Short   string
	Full    string
	Related []string
	Anchor  string
}

// AnchorFull is the full HTML id for a term's section on /about.
func (t Term) AnchorFull() string { return "about.glossary-" + t.Slug }

// registry is the unexported package-level term list, sorted
// alphabetically by Term name. Adding a term means appending
// here. Tests assert uniqueness + valid anchor + non-empty
// fields.
var registry = []Term{
	{
		Slug:  "article",
		Term:  "Article",
		Short: "A long-form markdown essay with inline Person Record references (`[Private John Doe](#person/D-00123)`).",
		Full: "A primary archive entry for a long-form, markdown-bodied essay with inline Person Record references. Lives in its own `articles` table. Can be snapshotted (read-only sibling row) for user-managed Revisions. Display ID namespace: ART-NNNNN.",
		Related: []string{"article-reference", "article-snapshot", "person-record"},
	},
	{
		Slug:  "article-reference",
		Term:  "Article Reference",
		Short: "A per-row link from an Article to a Person Record via the `article_refs` junction.",
		Full: "A per-row link from an Article to a Person Record, stored in the `article_refs` junction table. Authored via inline syntax `[Text](#person/D-00123)`; resolved strictly via Display ID — unresolved tokens render as fail-loud warnings.",
		Related: []string{"article", "display-id", "person-record"},
	},
	{
		Slug:  "article-snapshot",
		Term:  "Article Snapshot",
		Short: "A read-only sibling row of an Article, created via the \"Save copy\" button.",
		Full: "Stored in the same `articles` table with `is_snapshot = 1`. The read path filters snapshots so the live detail page is unaffected; the Revisions tab lists + restores + deletes them. Snapshot-of-snapshot is rejected.",
		Related: []string{"article"},
	},
	{
		Slug:  "backup-archive",
		Term:  "Backup Archive",
		Short: "A full replacement archive snapshot used for restore and safekeeping.",
		Full: "A full replacement snapshot of the entire Local Archive, designed for restore and safekeeping. Distinct from a Shared Archive (merge-oriented) or a Static Archive (read-only browser-viewable).",
		Related: []string{"local-archive", "restore-point", "static-archive", "shared-archive"},
	},
	{
		Slug:  "claim",
		Term:  "Claim",
		Short: "An assertion about a person that is extracted from a Source Record.",
		Full: "An assertion about a person that is extracted from a Source Record. Claims are weighted by evidence to support Findings. Not the same as a fact (which would imply the researcher has already vetted it).",
		Related: []string{"finding", "source-record"},
	},
	{
		Slug:  "confederate-home-status",
		Term:  "Confederate Home Status",
		Short: "The archive field that records a person's relationship to a Confederate Home.",
		Full: "The archive field that records a person's relationship to a Confederate Home. The canonical no-status value is `N/A`.",
		Related: []string{"person-record"},
	},
	{
		Slug:  "display-id",
		Term:  "Display ID",
		Short: "The canonical user-facing identifier for a Person Record.",
		Full: "The canonical user-facing identifier for a Person Record. Format: `DXD-NNNNN` for most subtypes; `EVT-NNNNN` for Event Records; `ART-NNNNN` for Articles. Always preferred over the internal numeric ID in user-facing copy.",
		Related: []string{"person-record", "event-record", "article"},
	},
	{
		Slug:  "duplicate-audit",
		Term:  "Duplicate Audit",
		Short: "An archive-wide scan that detects likely duplicate Person Records for human review.",
		Full: "An archive-wide scan that detects likely duplicate Person Records and stages them for the Review Queue. Distinct from Merge Review (which resolves conflicts during import).",
		Related: []string{"review-queue", "merge-review"},
	},
	{
		Slug:  "event-record",
		Term:  "Event Record",
		Short: "A primary archive entry for a dated event in Civil War history (battle, campaign, death, marriage, etc.).",
		Full: "A primary archive entry for a dated event (Battle, Campaign, Death, Marriage, Hospital stay, etc.). Lives in the `soldiers` table with `entry_type = 'event'`. Display ID namespace: `EVT-NNNNN`. Linked to Person Records via the `event_person_links` junction.",
		Related: []string{"person-record", "linked-person", "display-id"},
	},
	{
		Slug:  "finding",
		Term:  "Finding",
		Short: "A researcher-endorsed conclusion reached by weighing one or more Claims.",
		Full: "A researcher-endorsed conclusion reached by weighing one or more Claims. Findings drive Service Timeline rendering. Not the same as a Claim (the weight of one study) or a fact (the researcher's verified take).",
		Related: []string{"claim", "service-timeline", "source-record"},
	},
	{
		Slug:  "linked-person",
		Term:  "Linked Person",
		Short: "A Person Record subtype for a person significant to the research but not a Soldier or Spouse.",
		Full: "A Person Record subtype for a Brother, Daughter, Executor, Witness, attending physician, or civilian contact. Carries a free-text `relationship_label`. Shares the Person Record namespace: `DXD-NNNNN`.",
		Related: []string{"person-record", "event-record", "display-id"},
	},
	{
		Slug:  "local-archive",
		Term:  "Local Archive",
		Short: "The live working collection of Person Records, Source Records, and review state on one machine.",
		Full: "The live working collection on one machine. The default archive type researchers interact with day-to-day. Can be exported as Shared Archives (merge), Backup Archives (restore), Static Archives (publish), or Restore Points (rollback).",
		Related: []string{"backup-archive", "shared-archive", "static-archive", "restore-point", "merge-review"},
	},
	{
		Slug:  "local-record",
		Term:  "Local Record",
		Short: "The Person Record that already exists in the Local Archive during Merge Review.",
		Full: "The Person Record that already exists in the Local Archive at the moment an Incoming Record arrives. The Merge Review workflow compares these side by side and surfaces conflicts to the human reviewer.",
		Related: []string{"incoming-record", "merge-review", "shared-archive"},
	},
	{
		Slug:  "insights",
		Term:  "Insights",
		Short: "The drilldown surface for repository-wide analytics (top units, top cemeteries, related Person Records, duplicates).",
		Full: "The drilldown surface for repository-wide analytics. Replaces the deprecated Research Pack + Unit Camaraderie Graph surfaces (issue #455). Lives on /insights with per-area panels: Overview, Cemeteries, Homes, Pensions, Units, Chronology, Duplicate Audit.",
		Related: []string{"unit-membership", "duplicate-audit", "local-archive"},
	},
	{
		Slug:  "incoming-record",
		Term:  "Incoming Record",
		Short: "The Person Record arriving from a Shared Archive during Merge Review.",
		Full: "The Person Record arriving from a Shared Archive during Merge Review. Paired with a Local Record; the human reviewer decides whether to keep local, accept incoming, or merge them.",
		Related: []string{"local-record", "merge-review", "shared-archive"},
	},
	{
		Slug:  "merge-review",
		Term:  "Merge Review",
		Short: "The workflow for resolving conflicts created by importing a Shared Archive.",
		Full: "The workflow for resolving conflicts introduced by importing a Shared Archive into a Local Archive. Compares a Local Record and an Incoming Record side by side; surfaces field-level conflicts and lets the researcher pick the survivor.",
		Related: []string{"local-record", "incoming-record", "shared-archive", "duplicate-audit"},
	},
	{
		Slug:  "person-record",
		Term:  "Person Record",
		Short: "A primary archive entry for one person (soldier, wife, widow, linked person, or event).",
		Full: "A primary archive entry for one person. Always a Person Record, never a bare \"record.\" Subtypes: Soldier, Spouse Record (Wife / Widow), Linked Person, Event Record. All share the `soldiers` table; the `entry_type` column distinguishes them.",
		Related: []string{"display-id", "source-record", "claim", "finding", "tag", "review-queue"},
	},
	{
		Slug:  "research-collection",
		Term:  "Research Collection",
		Short: "A user-curated grouping of archive material inside a Local Archive for an ongoing research purpose.",
		Full: "A user-curated grouping of archive material assembled inside a Local Archive. Lives inside the Local Archive; not a separate file. Useful for ad-hoc research scopes (e.g. \"Pension files for 5th Texas Infantry\").",
		Related: []string{"local-archive"},
	},
	{
		Slug:  "research-log",
		Term:  "Research Log",
		Short: "A structured record of research activity, findings, or open questions that should remain intelligible over time.",
		Full: "A structured record of research activity intended to remain intelligible across months or years. Distinct from the Scratch Pad (informal, ad-hoc, single-record tied).",
		Related: []string{"scratch-pad", "person-record"},
	},
	{
		Slug:  "research-pack",
		Term:  "Research Pack",
		Short: "A prepared bundle organized around a defined scope such as a county or state.",
		Full: "Deprecated after slim (issue #455, slices 1 + 3). The same Top Units / Top Cemeteries / Related Person Records data lives on Insights. Use Insights drilldown scoped to the research scope instead.",
		Related: []string{"insights", "local-archive"},
	},
	{
		Slug:  "restore-point",
		Term:  "Restore Point",
		Short: "An automatic pre-update recovery bundle that preserves a recoverable Local Archive state.",
		Full: "An automatic pre-update recovery bundle that preserves a recoverable Local Archive state + the previously safe app build for rollback. Distinct from a Backup Archive (manual, user-initiated, full replacement).",
		Related: []string{"backup-archive", "local-archive"},
	},
	{
		Slug:  "review-queue",
		Term:  "Review Queue",
		Short: "The holding area for Person Records that need human attention before they should be treated as clean archive data.",
		Full: "The holding area for Person Records that need human attention before they should be treated as clean archive data. Distinct from the Share Queue (deliberate staging for export), Duplicate Audit (specific scan type), and Merge Review (specific workflow during import).",
		Related: []string{"person-record", "duplicate-audit", "merge-review", "share-queue"},
	},
	{
		Slug:  "scratch-pad",
		Term:  "Scratch Pad",
		Short: "An informal working note tied to a single Person Record.",
		Full: "An informal working note tied to a single Person Record. Distinct from the Research Log (structured, persistent over time). Use the Scratch Pad for transient observations; promote to Research Log when the note should survive the session.",
		Related: []string{"research-log", "person-record"},
	},
	{
		Slug:  "service-event",
		Term:  "Service Event",
		Short: "A Timeline Marker specifically about military service.",
		Full: "A Timeline Marker specifically about military service. The most common Timeline Marker on a Soldier's Service Timeline. Other Timeline Marker kinds include birth, death, marriage, hospital stay.",
		Related: []string{"timeline-marker", "service-timeline"},
	},
	{
		Slug:  "service-timeline",
		Term:  "Service Timeline",
		Short: "An evidence-backed chronological view of a Soldier's known life or service events.",
		Full: "An evidence-backed chronological view of a Soldier's known life or service events, derived from Findings. Not a free-form narrative.",
		Related: []string{"timeline-marker", "service-event", "finding"},
	},
	{
		Slug:  "share-queue",
		Term:  "Share Queue",
		Short: "The in-memory list of Person Records staged for inclusion in a Shared Archive.",
		Full: "The in-memory list of Person Records a researcher has staged for inclusion in a Shared Archive (.ddshare) before exporting. Persisted in browser localStorage so it survives navigation, reloads, and app restarts. Distinct from the Review Queue (records needing human attention).",
		Related: []string{"shared-archive", "review-queue"},
	},
	{
		Slug:  "shared-archive",
		Term:  "Shared Archive",
		Short: "A merge-oriented archive package exchanged between DixieData users.",
		Full: "A merge-oriented archive package exchanged between DixieData users. Importing into a Local Archive opens a Merge Review against the existing Local Records. Distinct from a Backup Archive (replacement snapshot) or a Static Archive (read-only browser viewer).",
		Related: []string{"local-archive", "merge-review", "backup-archive", "static-archive"},
	},
	{
		Slug:  "soldier",
		Term:  "Soldier",
		Short: "A Person Record subtype for the servicemember being researched.",
		Full: "A Person Record subtype for the servicemember being researched. The primary person in a typical Spouse + Soldier pairing. Carries the rank, unit, service-event data.",
		Related: []string{"person-record", "spouse-record"},
	},
	{
		Slug:  "source-record",
		Term:  "Source Record",
		Short: "An attached evidence item that documents or supports a Person Record (pension, application, etc.).",
		Full: "An attached evidence item that documents or supports a Person Record, such as a pension or application record. The substrate Claims are extracted from. Always belongs to a Person Record.",
		Related: []string{"person-record", "claim", "finding"},
	},
	{
		Slug:  "spouse-record",
		Term:  "Spouse Record",
		Short: "A Person Record for the spouse linked to a Soldier.",
		Full: "A Person Record for the spouse linked to a Soldier, regardless of subtype. Subtypes: Wife (described as a wife) or Widow (described as a widow).",
		Related: []string{"person-record", "wife", "widow", "soldier"},
	},
	{
		Slug:  "static-archive",
		Term:  "Static Archive",
		Short: "A read-only browser-viewable archive export.",
		Full: "A read-only browser-viewable archive export. Useful for publishing research without exposing the app. Distinct from a Shared Archive (merge-oriented, DixieData-to-DixieData) or a Backup Archive (replacement snapshot for restore).",
		Related: []string{"local-archive", "shared-archive", "backup-archive"},
	},
	{
		Slug:  "tag",
		Term:  "Tag",
		Short: "A user-defined free-text label applied to a Person Record.",
		Full: "A user-defined free-text label applied to a Person Record for ad-hoc grouping (e.g. a FindAGrave virtual cemetery or a research-project scope). Case-insensitive across the Local Archive; original casing preserved on display. Many-to-many between Tag and Person Record.",
		Related: []string{"person-record"},
	},
	{
		Slug:  "timeline-marker",
		Term:  "Timeline Marker",
		Short: "A dated item shown on a Service Timeline.",
		Full: "A dated item shown on a Service Timeline. The most common subtype is Service Event (military service). Other kinds include birth, death, marriage, hospital stay.",
		Related: []string{"service-timeline", "service-event"},
	},
	{
		Slug:  "unit-camaraderie-graph",
		Term:  "Unit Camaraderie Graph",
		Short: "An inferred relationship network between Soldiers based on shared units, time overlap, or other signals.",
		Full: "Deprecated after slim (issue #455, slices 1 + 3). Use Insights drilldown scoped to unit instead.",
		Related: []string{"insights", "unit-membership"},
	},
	{
		Slug:  "unit-membership",
		Term:  "Unit Membership",
		Short: "The factual claim that a Soldier served in a particular unit.",
		Full: "The factual Claim that a Soldier served in a particular unit. The substrate for the Insights drilldown. Not the same as Unit Camaraderie Graph (which is an inferred relationship, deprecated).",
		Related: []string{"claim", "soldier"},
	},
	{
		Slug:  "wife",
		Term:  "Wife",
		Short: "A Spouse Record subtype used when the person should be described as a wife rather than a widow.",
		Full: "A Spouse Record subtype. The archive will use Wife copy throughout the UI when describing this person.",
		Related: []string{"spouse-record", "widow"},
	},
	{
		Slug:  "widow",
		Term:  "Widow",
		Short: "A Spouse Record subtype used when the person should be described as a widow rather than a wife.",
		Full: "A Spouse Record subtype. The archive will use Widow copy throughout the UI when describing this person.",
		Related: []string{"spouse-record", "wife"},
	},
}

// Registry returns every term in the glossary, sorted
// alphabetically by display Term. The viewmodel layer
// passes this slice to the About templ — every term
// renders at /about#glossary-<slug>.
func Registry() []Term {
	out := make([]Term, len(registry))
	copy(out, registry)
	sort.Slice(out, func(i, j int) bool {
		// Stable, case-insensitive alphabetical order by the
		// human-readable Term name (not Slug — Slugs are
		// kebab-case and would sort by token, not by
		// reading order).
		return strings.ToLower(out[i].Term) < strings.ToLower(out[j].Term)
	})
	return out
}

// LookupBySlug finds a term by its kebab-case slug. Returns
// the zero value + ok=false if the slug isn't in the
// registry. Used by the disclosure popover (slice 2 — when
// the user clicks a term on a verified apply site, the
// popover fetches Short by slug).
func LookupBySlug(slug string) (Term, bool) {
	for _, t := range registry {
		if t.Slug == slug {
			return t, true
		}
	}
	return Term{}, false
}
