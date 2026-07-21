// Package viewmodel defines UI-shaped projections of soldier records and maps them to/from domain models.
package viewmodel

// PersonRecord is the UI-shaped projection of a Soldier (the domain
// type from internal/models). It carries only the fields the UI
// surfaces, with nil-safe defaults for everything else, so the
// templ handlers can render without per-field nil-checks. The
// mapping from models.Soldier → viewmodel.PersonRecord is in
// internal/viewmodel/mappers.go.
type PersonRecord struct {
	ID                    int64
	DisplayID             string
	SyncID                string
	EntryType             string
	LinkedSoldierID       int64
	RelationshipLabel     string
	SpouseName            string
	MaidenName            string
	IsGenerated           bool
	PensionID             string
	ApplicationID         string
	Prefix                string
	ShowPrefixBeforeName  bool
	FirstName             string
	MiddleName            string
	LastName              string
	Suffix                string
	Rank                  string
	RankIn                string
	RankOut               string
	Unit                  string
	PensionState          string
	ConfederateHomeStatus string
	ConfederateHomeName   string
	DeathYear             int
	DeathMonth            int
	DeathDay              int
	BirthDate             string
	DeathDate             string
	BirthInfo             string
	BuriedIn              string
	Biography             string
	PDFExcerptOverride    string
	Notes                 string
	NeedsReview           bool
	ReviewReason          string
	AddedBy               string
	LastEditedBy          string
	LastEditedFields      string
	LastEditedAt          string
	CreatedAt             string
	UpdatedAt             string
	SearchMatchField      string
	SearchMatchSnippet    string
	SpouseDisplayID       string
	BackLinkURL           string
	BackLinkLabel         string
	SourceRecordCount     int
	ImageCount            int
	SourceRecords         []SourceRecord
	Images                []Image
	Tags                  []TagOption
	// Issue #377 / #423: row provenance fields. CreatedByVersion
	// is the DixieData release that wrote the row (e.g.
	// "v1.2.64"); CreatedByImportPath is the code path that
	// wrote it (e.g. "create_soldier", "memorial_json_import",
	// "restore_backup_archive"); RestoredAt is the RFC3339
	// timestamp the row was carried over the most recent
	// SQLite-snapshot restore (empty = never restored). The
	// Soldier detail page footer renders the v64 line when
	// either CreatedBy* is non-empty, and a "Restored at
	// <human-readable timestamp>" line when RestoredAt is
	// non-empty. v64 stamps are populated by every Create
	// call site (slice 2 of #377); v65 RestoredAt is populated
	// by restoreSnapshotBackup's bulk UPDATE (slice 2 of #423).
	CreatedByVersion      string
	CreatedByImportPath   string
	RestoredAt            string
}

// EventRecord is the UI-shaped projection of an Event Record
// row (issue #320 / v60). Embeds PersonRecord so the base
// identity (DisplayID, SyncID, Tags, ...) is reachable via
// promotion (event.DisplayID, event.Tags). Adds the six
// Event-only fields:
//
//   - Kind: free-text label ("Battle", "Earthquake", ...).
//   - BeginDate / EndDate: MM/DD/YYYY canonical date shape so
//     the existing date-filter predicates apply.
//   - Description: long-form write-up (mirrors Person Record's
//     Biography field).
//   - EventSources: per-Event sources from the dedicated
//     event_sources table (#340 / v61). Person Records always
//     leave this empty.
//   - LinkedPersons: per-Event Person Records linked via
//     event_person_links (#361 slice 2).
//
// #343 #1 splits EventRecord out of PersonRecord. The split
// follows the architecture-review finding that polymorphism in
// data (5 always-empty fields on PersonRecord for non-Event
// rows) was a shallow-module signal. The deletion-test signal:
// remove EventSources from PersonRecord — does the v61
// sources-wiped-on-Update bug come back? No. The table +
// service carry the load; the viewmodel field is a mirror.
type EventRecord struct {
	PersonRecord
	Kind          string
	BeginDate     string
	EndDate       string
	Description   string
	EventSources  []SourceRecord
	LinkedPersons []PersonRecord
}

// SourceRecord is the UI-shaped projection of a Source Record
// attached to a Person Record. Carries the display-ID / sync-ID
// pairs so the form can render without re-querying, plus the
// type + details the right-rail + the Source Records tab use.
type SourceRecord struct {
	ID                 int64
	SyncID             string
	PersonRecordID     int64
	PersonRecordSyncID string
	SourceRecordType   string
	AppID              string
	Details            string
}

// Image is the UI-shaped projection of a per-soldier image. Mirrors
// models.Image but with display-ready defaults (empty string for
// missing caption, "—" for unknown dates).
type Image struct {
	ID                 int64
	SyncID             string
	PersonRecordID     int64
	PersonRecordSyncID string
	FileName           string
	FilePath           string
	Caption            string
	IsPrimary          bool
	ResolvedPath       string
}

// ArchiveCounts is the UI-shaped rollup surfaced on the Insights
// page header. Same shape as models.ArchiveCounts; distinct type
// so the templ handlers can render without importing the models
// package directly.
//
// Issue #491: the three PersonRecordCount fields are the SAME
// three Person Record subtypes (soldier / spouse / linked_person)
// from models.ArchiveCounts. The three non-Person fields
// (EventRecordCount, ArticleRecordCount, TagCount) are added for
// the new /inventory page + the Calendar header archive rollup
// (issue #491). PersonRecordCount is the linked-person count
// (NOT the Person Record total) — the third Calendar card's label
// was changed from "Person Records" to "Linked Persons" so the
// number reads as the linked-person bucket only. The total
// Person Record count is TotalRecords() = SoldierCount +
// SpouseRecordCount + PersonRecordCount.
type ArchiveCounts struct {
	SoldierCount        int
	SpouseRecordCount   int
	PersonRecordCount   int
	EventRecordCount    int
	ArticleRecordCount  int
	TagCount            int
}

// TotalRecords returns the sum of all Person Record subtypes. Mirrors
// models.ArchiveCounts.TotalRecords so the empty-state partial can
// decide whether the Local Archive is truly empty (issue #98).
func (c ArchiveCounts) TotalRecords() int {
	return c.SoldierCount + c.SpouseRecordCount + c.PersonRecordCount
}

// TotalEntities returns the sum of all five entity kinds the
// /inventory page surfaces (Soldiers + Spouse Records + Linked
// Persons + Event Records + Articles). Tags are excluded
// because they're a labeling primitive, not an archive entry.
// Issue #491.
func (c ArchiveCounts) TotalEntities() int {
	return c.SoldierCount + c.SpouseRecordCount + c.PersonRecordCount + c.EventRecordCount + c.ArticleRecordCount
}

// InventoryKindCount is one bucket in the per-kind rollup the
// /inventory page renders alongside the headline numbers. Used
// for the per-Event-Record kind breakdown, the per-Article title
// list, and the per-Tag name list. Count is always 1 for the
// per-Article / per-Tag lists (one row per entity); the
// per-Event-kind rollup uses Count to surface the most-common
// kind first. Issue #491.
type InventoryKindCount struct {
	Label string
	Count int
}

// InventoryView is the data shape the /inventory page (issue
// #491) renders. Counts carries the same fields as
// ArchiveCounts (the new EventRecordCount / ArticleRecordCount /
// TagCount fields surface on the headline strip). EventKinds /
// ArticleRefs / TagKinds are the per-kind drilldown rows that
// surface below the headline strip. The page is intentionally
// basic — the per-attribute analytics live on /insights.
//
// Issue #580 slice 1: Metrics carries the activity rollup
// derived from the stored creation metadata on primary
// entries (Person Record subtypes + Event Records + live
// Articles). Article Snapshots are excluded by the storage
// query so the activity count matches the Articles card's
// headline number. When Metrics has no rows the UI renders
// a single-line empty state instead of an empty section
// header.
type InventoryView struct {
	Counts      ArchiveCounts
	EventKinds  []InventoryKindCount
	ArticleRefs []InventoryKindCount
	TagKinds    []InventoryKindCount
	Metrics     InventoryMetrics
}

// InventoryMetrics is the activity rollup the /inventory page's
// Metrics section renders. EntriesPerDay is the chronological
// day-bucketed count of primary entries created on each day
// (Soldiers + Spouse Records + Linked Persons + Event Records +
// live Articles, Article Snapshots excluded by the storage
// query). The map keys are ISO date strings ("YYYY-MM-DD") so
// the templ partial sorts lexicographically and renders
// readable per-day labels without time-zone math. FirstEntryDate
// / LatestEntryDate are the ISO-formatted extremes or empty when
// the archive has no primary entries. ActiveDayCount is the
// number of distinct keys in EntriesPerDay. TotalsByType
// cross-checks the headline counts so a render regression where
// the storage query drops an entry type trips the smoke probe.
//
// Issue #580 locked decisions:
//   - Primary entries only — Soldiers / Spouse Records / Linked
//     Persons / Event Records / live Articles.
//   - Article Snapshots excluded (matches the existing inventory
//     headline number for Articles).
//   - Stored creation metadata only — no new tracking columns,
//     no schema migration.
//   - Inline text representation is the source of truth;
//     a chart may supplement but never replace it.
type InventoryMetrics struct {
	EntriesPerDay  map[string]int
	// EntriesPerDayByKind is the per-kind per-day breakdown
	// the /inventory Activity metrics line graph renders
	// (issue #583). Always carries every kind even on an empty
	// archive. See records.InventoryMetricsRaw for the bucket
	// shape.
	EntriesPerDayByKind map[string]map[string]int
	FirstEntryDate string
	LatestEntryDate string
	ActiveDayCount int
	TotalsByType   InventoryMetricTotals
}

// InventoryMetricTotals is the per-type activity breakdown the
// Metrics summary line renders. The numbers match the headline
// counts in the InventoryView.Counts.* so a future storage
// regression that drops an entry type trips the smoke probe.
type InventoryMetricTotals struct {
	Soldiers       int
	SpouseRecords  int
	LinkedPersons  int
	EventRecords   int
	Articles       int
}

// Quote is the UI-shaped projection of a per-soldier quote — a
// verbatim excerpt from a Source Record the user has flagged for
// display on the Person Record detail page.
type Quote struct {
	Author  string
	Text    string
	Context string
	Tags    []string
}

// CalendarDaySummary is the per-day rollup the calendar grid renders:
// the count of anniversaries on that day plus a link to the
// detail view. Used as the grid cell content.
type CalendarDaySummary struct {
	AnniversaryCount int
	EventCount       int
	HolidayCount     int
}

// CalendarItem is one anniversary entry the calendar grid surfaces:
// one soldier + the date + a short label. The grid renders N
// CalendarItems per CalendarDaySummary cell.
type CalendarItem struct {
	ID       int64
	ItemType string
	Title    string
	Notes    string
}

// CalendarItemForm is the editable form payload for adding a new
// anniversary entry to a Person Record. Distinct from CalendarItem
// (the read-side projection) because the form carries extra fields
// (date input shape, source-record picker state).
type CalendarItemForm struct {
	EditingID    int64
	ItemType     string
	Title        string
	Notes        string
	ErrorMessage string
}

// CalendarDayDetail is the per-day detail panel: the full list of
// CalendarItems for the day, sorted by soldier name, plus the
// per-item "open Person Record" link. Shown when the user clicks
// a day cell on the calendar grid.
type CalendarDayDetail struct {
	Month            int
	Day              int
	AllowCustomItems bool
	StatusKind       string
	StatusMessage    string
	Form             CalendarItemForm
	Items            []CalendarItem
	Anniversaries    []PersonRecord
}

// PersonRecordSearch is the structured search-result row the
// browse + soldiers-list pages render. Alias of models.SoldierSearch;
// re-declared here so the templ handlers don't import the models
// package.
type PersonRecordSearch struct {
	Mode                  string
	Query                 string
	Browse                bool
	Recent                bool
	DisplayID             string
	EntryType             string
	FirstName             string
	MiddleName            string
	LastName              string
	MaidenName            string
	RelationshipLabel     string
	Rank                  string
	RankIn                string
	RankOut               string
	Unit                  string
	SourceRecordType      string
	PensionState          string
	ConfederateHomeStatus string
	ConfederateHomeName   string
	BuriedIn              string
	ReviewStatus          string
	BirthDate             string
	BirthYear             string
	BirthYearTo           string
	DeathDate             string
	DeathYear             string
	DeathYearTo           string
	DeathMonth            string
	DeathDay              string
	// IsArchiveEmpty and TotalRecordCount are populated by the handler
	// from models.ArchiveCounts so the search results template can show
	// a Setup card on first-run / truly-empty archives without needing
	// a separate API call. See issue #98 from the 2026-06-24 audit.
	IsArchiveEmpty    bool
	TotalRecordCount  int
}

// BrowseState is the per-request state the browse page uses to
// render: the current filter, sort, page, total-count, and the
// page slice. Built from URL query params by the browse handler
// and passed to the templ renderer as a single struct.
type BrowseState struct {
	Page                  int
	PageSize              int
	Total                 int
	Scope                 string
	Sort                  string
	EntryType             string
	Unit                  string
	BuriedIn              string
	PensionState          string
	ReviewStatus          string
	ConfederateHomeStatus string
	// SelectedTagSet is the normalised-name set of the active
	// tag filter; used by the chip cloud to render the
	// checked-state pill. Always at least an empty (not-nil) map
	// so templates can index without nil checks.
	SelectedTagSet map[string]bool
	// PageSizeOptions is the dropdown option list for the
	// browse page-size selector, driven by config.json (#639).
	PageSizeOptions []int
}

// PersonRecordFormSuggestions is the autocomplete payload the
// Person Record form fetches when the user starts typing in a
// field. Alias of models.SoldierFormSuggestions.
type PersonRecordFormSuggestions struct {
	RankIn            []string
	RankOut           []string
	Unit              []string
	Prefix            []string
	Suffix            []string
	PensionState      []string
	BuriedIn          []string
	ConfederateHome   []string
	SourceRecordType  []string
	RelationshipLabel []string
}

// TagOption is the browse-side view of a tag for the chip cloud:
// minimal payload so 10k-tag rows do not balloon the page payload.
// NormalisedName is what the URL carries; Name is the
// display-cased value.
type TagOption struct {
	ID             int64
	Name           string
	NormalizedName string
}

// InitialSetupForm is the form payload the first-launch setup
// wizard collects: the user's timezone (default: buildinfo
// .CalendarTimeZone), the dataDir confirmation, the optional
// seed-data opt-in. Submitted once at first launch; the resulting
// settings live in records.LocalSettings.
type InitialSetupForm struct {
	FirstName     string
	MiddleName    string
	LastName      string
	BirthYear     string
	PrefixPreview string
	ErrorMessage  string
}

// GoogleSettings is the Google OAuth + sync configuration surfaced
// on the Settings page. Alias of models.GoogleSettings; re-
// declared so the templ form handlers don't import models.
type GoogleSettings struct {
	ClientID                string
	ClientSecret            string
	CalendarID              string
	DriveFolderID           string
	ManagedEventPreferences CalendarEventPreferences
}

// CalendarEventPreferences is the per-user override for the
// default Google Calendar event template (title, description,
// reminder minutes). Stored in records.LocalSettings; surfaced
// on the Calendar Preferences modal.
type CalendarEventPreferences struct {
	TitlePreset         string
	StartTime           string
	ReminderPrimary     string
	ReminderSecondary   string
	IncludeRecordID     bool
	IncludeUnit         bool
	IncludeBuriedIn     bool
	IncludeOriginalDate bool
}

// GoogleStatus is the current-state snapshot the Settings page
// renders: connected / not connected, last sync time, last sync
// result, any drift detected.
type GoogleStatus struct {
	Settings              GoogleSettings
	Connected             bool
	HasClientID           bool
	HasSecret             bool
	HasToken              bool
	SharedClientAvailable bool
	SharedClientSource    string
	UsingSharedClient     bool
	ManagedCalendarID     string
	TestCalendarID        string
	LastSyncedAt          string
	OutOfSync             bool
	DriftAdded            int
	DriftUpdated          int
	DriftRemoved          int
}

// ExportRecordOption is one row in the export-pipeline record-type
// filter: the display label + the canonical value the export
// picks up. Built from records.LocalSettings.ExportRecordFilter.
type ExportRecordOption struct {
	ID                    int64
	DisplayID             string
	DisplayName           string
	EntryType             string
	Unit                  string
	PensionState          string
	ConfederateHomeStatus string
	BuriedIn              string
}

// MergeReviewConflict is one row in the Local-vs-Incoming merge
// review ledger: which field, which side, what value, and the
// user's resolution (keep-local, keep-incoming, keep-both).
type MergeReviewConflict struct {
	ID                int64
	SessionID         string
	ConflictType      string
	Reason            string
	LocalRecordID     int64
	LocalDisplayID    string
	IncomingDisplayID string
	Resolution        string
	CreatedAt         string
	LocalRecord       *PersonRecord
	IncomingRecord    PersonRecord
}

// DuplicateAuditSummary is the headline number rollup for the
// Insights duplicate-audit card: open findings, dismissed, merged,
// total scanned.
type DuplicateAuditSummary struct {
	OpenFindings        int
	ResolvedFindings    int
	LastRunAt           string
	SimilarityThreshold int
}

// DuplicateAuditFindingSummary is one row in the duplicate-audit
// list: the candidate-pair identifiers, the match score, and the
// link to the per-pair compare view.
type DuplicateAuditFindingSummary struct {
	ID                  int64
	OtherPersonRecordID int64
	OtherDisplayID      string
	OtherName           string
	Reason              string
}

// ReviewQueueEntry is one row in the Review Queue page: a
// pending duplicate-audit finding or a pending merge-review
// conflict that the user has not yet resolved.
type ReviewQueueEntry struct {
	PersonRecord      PersonRecord
	DuplicateFindings []DuplicateAuditFindingSummary
}

// ResolvedFindingEntry is one row in the Resolved tab of the
// Review Queue: a duplicate-audit finding whose status flipped
// to 'resolved'. Carries the two side-cached Person Record
// display IDs so the page can render + link back without a
// join. issue #461.
type ResolvedFindingEntry struct {
	ID             int64
	LeftRecordID   int64
	RightRecordID  int64
	LeftDisplayID  string
	RightDisplayID string
	FindingType    string
	Reason         string
	ResolvedAt     string
}

// ResolvedConflictEntry is one row in the Resolved tab of the
// Review Queue: a Local-vs-Incoming merge_review_conflicts row
// whose resolution IS NOT NULL. The viewmodel only carries
// the per-row scalar fields; the per-conflict side-by-side
// detail stays scoped to the Person Record ledger page.
// issue #461.
type ResolvedConflictEntry struct {
	ID                int64
	SessionID         string
	ConflictType      string
	Reason            string
	LocalRecordID     int64
	LocalDisplayID    string
	IncomingDisplayID string
	Resolution        string
	ResolvedAt        string
}

// DuplicateAuditComparisonField is one field-row in the per-pair
// compare view: the field name + the Local value + the incoming
// value + whether they match.
type DuplicateAuditComparisonField struct {
	Key         string
	Label       string
	LeftValue   string
	RightValue  string
	Highlighted bool
}

// DuplicateAuditComparison is the full per-pair compare payload
// the duplicate-audit side-by-side view renders: the two Soldiers
// (Local + candidate) + the per-field comparison list.
type DuplicateAuditComparison struct {
	FindingID         int64
	FindingType       string
	PageTitle         string
	BackHref          string
	BackLabel         string
	Reason            string
	Status            string
	LeftPersonRecord  PersonRecord
	RightPersonRecord PersonRecord
	Fields            []DuplicateAuditComparisonField
}

// AnalyticsCount is one labeled bar in the Insights analytics
// charts: the label + the count. Used by cemeteries, Confederate
// Homes, pensions, units.
type AnalyticsCount struct {
	Label string
	Count int
}

// AnalyticsSnapshot is the full Insights page payload: the
// per-dimension AnalyticsCount lists + the headline ArchiveCounts.
type AnalyticsSnapshot struct {
	PersonRecordTypes       ArchiveCounts
	CemeteryDensity         []AnalyticsCount
	ConfederateHomeStatus   []AnalyticsCount
	ConfederateHomeNames    []AnalyticsCount
	PensionDistribution     []AnalyticsCount
	UnitRepresentation      []AnalyticsCount
	BirthDecadeDistribution []AnalyticsCount
	DeathDecadeDistribution []AnalyticsCount
	DuplicateAudit          DuplicateAuditSummary
}

// ServiceTimeline is the per-soldier chronological service timeline
// the Timeline tab renders: events (enlistment, transfer, wound,
// discharge, death) sorted by date.
type ServiceTimeline struct {
	SubjectPersonRecord  PersonRecord
	TimelineEvents       []TimelineEvent
	UndatedSourceRecords []SourceRecord
	StartLabel           string
	EndLabel             string
	ExactEventCount      int
	InferredEventCount   int
}

// TimelineEvent is one row in ServiceTimeline: the date, the kind
// (enlistment / transfer / etc.), the source record ID, and the
// human-readable description.
type TimelineEvent struct {
	Title           string
	DateLabel       string
	Description     string
	SourceLabel     string
	Category        string
	ConfidenceLabel string
	Approximate     bool
}

// ResearchTask is one open research task the Research Log surfaces:
// the description, the status (open / done / dropped), and the
// source records it's based on.
type ResearchTask struct {
	ID             int64
	PersonRecordID int64
	Title          string
	Notes          string
	EvidenceType   string
	Status         string
	CreatedAt      string
	UpdatedAt      string
	ResolvedAt     string
}

// ResearchTaskSuggestion is a candidate research task the Research
// Log surfaces for the user to accept or dismiss: the suggestion
// text + the basis (which records / dates triggered it).
type ResearchTaskSuggestion struct {
	Title        string
	Notes        string
	EvidenceType string
}

// ResearchLog is the full Research Log payload: the open tasks +
// the dismissed suggestions + the recently-completed tasks. Per-
// soldier (scoped to one Person Record).
type ResearchLog struct {
	SubjectPersonRecord PersonRecord
	Tasks               []ResearchTask
	Suggestions         []ResearchTaskSuggestion
	OpenCount           int
	ResolvedCount       int
}

// ResearchCollection is one named research collection the Research
// Collections Hub page renders: the name, the count of Person
// Records it covers, and the per-collection rollup.
type ResearchCollection struct {
	ID              int64
	Name            string
	Description     string
	CreatedAt       string
	UpdatedAt       string
	ItemCount       int
	ContainsCurrent bool
}

// ResearchCollectionHub is the full Research Collections Hub page
// payload: the list of ResearchCollection + the per-collection
// rollup + the totals.
type ResearchCollectionHub struct {
	CurrentPersonRecord *PersonRecord
	Collections         []ResearchCollection
}

// ResearchCollectionDetail is the per-collection detail page:
// the Person Records in the collection, sorted, plus the per-
// collection rollup.
type ResearchCollectionDetail struct {
	Collection          ResearchCollection
	CurrentPersonRecord *PersonRecord
	PersonRecords       []PersonRecord
}

// MergeReviewLedger is the full Local-vs-Incoming merge review
// ledger: the open conflicts + the resolved conflicts (kept-local
// or kept-incoming) + the per-conflict history.
type MergeReviewLedger struct {
	SubjectPersonRecord PersonRecord
	Entries             []MergeReviewLedgerEntry
	OpenCount           int
	ResolvedCount       int
}

// MergeReviewLedgerEntry is one row in the MergeReviewLedger:
// the conflict, the resolution, the timestamp, and the user who
// resolved it.
type MergeReviewLedgerEntry struct {
	ID                  int64
	ConflictType        string
	Reason              string
	IncomingDisplayID   string
	Resolution          string
	CreatedAt           string
	ResolvedAt          string
	LocalRecordSnapshot PersonRecord
	IncomingSnapshot    PersonRecord
	DifferenceFields    []string
}

// OrphanedImage is one row in the orphan-image cleanup list: the
// relative path under dataDir, the file size, the modification
// time, and whether it's still referenced by any Soldier.
type OrphanedImage struct {
	RelativePath string
	Size         int64
	ModifiedAt   string
}

// DataQualityIssue is one data-quality finding the Settings page
// surfaces: the kind (missing display ID, future birth date, etc.),
// the affected Soldier, and the suggested fix.
type DataQualityIssue struct {
	PersonRecordID int64
	DisplayID      string
	Name           string
	EntryType      string
	Group          string
	Code           string
	Severity       string
	Summary        string
	Detail         string
	// Issue #377 / #423: row provenance fields surfaced in
	// the data-quality scan results so the user can see at
	// a glance whether a flagged row's corruption came from
	// an external import vs. a local edit. ImportPath is the
	// code path that wrote the row (e.g. "create_soldier",
	// "memorial_json_import"); RestoredAt is the RFC3339
	// timestamp the row was carried over the most recent
	// SQLite-snapshot restore (empty = never restored).
	ImportPath      string
	RestoredAt      string
}

// DataQualityIssueGroup is a rollup of DataQualityIssue rows by
// kind: how many issues of this kind exist, and which Soldiers
// are affected.
type DataQualityIssueGroup struct {
	Group  string
	Count  int
	Issues []DataQualityIssue
}

// DataQualityScanResult is the full data-quality scan output:
// the per-kind DataQualityIssueGroup list + the per-affected-
// Soldier rollup + the scan timestamp.
type DataQualityScanResult struct {
	Mode           string
	ScannedRecords int
	IssueCount     int
	Groups         []DataQualityIssueGroup
}

// DataQualityApplyResult is the result of applying an auto-fix
// for one DataQualityIssueGroup: the count of rows fixed, the
// count skipped, and any errors.
type DataQualityApplyResult struct {
	Selected       int
	Flagged        int
	AlreadyInQueue int
	NotFound       int
}

// UpdateSettings is the user-facing in-place-update configuration
// surfaced on the Settings page: the auto-update toggle, the
// update channel (stable / dev), the retained-backup cap, the
// last-checked timestamp.
type UpdateSettings struct {
	CurrentVersion     string
	BuildIdentity      string
	SourceURL          string
	EffectiveSourceURL string
	UsingDefaultSource bool
	CanApply           bool
	DisabledReason     string
	LastApply          *UpdateApplyStatus
	NoticeMessage      string
	NoticeKind         string
}

// UpdateApplyStatus is the in-progress state of an in-place
// update the UI surfaces: the current phase (downloading / applying
// / verifying / done), the percent complete, and any error.
type UpdateApplyStatus struct {
	Status    string
	Version   string
	Message   string
	AppliedAt string
}

// UpdateCheckResult is the response from the in-place update
// availability check: the latest release tag, the release notes
// URL, and whether the user is up to date.
type UpdateCheckResult struct {
	CurrentVersion   string
	AvailableVersion string
	UpdateAvailable  bool
	DownloadURL      string
	NotesURL         string
	ReleaseNotes     string
	PublishedAt      string
	SourceLabel      string
	CanApply         bool
	DisabledReason   string
}

// Type aliases for legacy templ templates that reference the
// shorter / pre-glossary names. Each alias resolves to the
// canonical viewmodel type of the same shape. New code should
// reference the canonical names directly.
type (
	// Soldier aliases viewmodel.PersonRecord (legacy pre-glossary name).
	Soldier = PersonRecord
	// Record aliases viewmodel.SourceRecord (legacy pre-glossary name).
	Record = SourceRecord
	// SoldierSearch aliases viewmodel.PersonRecordSearch.
	SoldierSearch = PersonRecordSearch
	// SoldierFormSuggestions aliases viewmodel.PersonRecordFormSuggestions.
	SoldierFormSuggestions = PersonRecordFormSuggestions
	// ServiceTimelineEvent aliases viewmodel.TimelineEvent.
	ServiceTimelineEvent = TimelineEvent
	// SourceConflictLedger aliases viewmodel.MergeReviewLedger
	// (legacy name; the per-soldier ledger predates the
	// glossary's rename).
	SourceConflictLedger = MergeReviewLedger
	// SourceConflictLedgerEntry aliases viewmodel.MergeReviewLedgerEntry.
	SourceConflictLedgerEntry = MergeReviewLedgerEntry
)

// ShareQueueRow (issue #193) is the per-row view shape used by
// the /share/queue management page. The Order field is the
// stable 1-based index in the queue so the user can sort or
// move rows visually without disturbing the underlying
// localStorage order (the JS re-applies its own user-rearranged
// order on save).
type ShareQueueRow struct {
	Order        int
	PersonRecord PersonRecord
}

// RecentJobEntry (issue #265) is the per-row view shape used
// by the /share landing's "Recent activity" section. The
// ShareView handler builds these from the last N terminal
// jobs in the registry (see jobs.Registry.RecentJobs); the
// templ iterates without a dependency on the jobs package.
//
// FinishedAt is a friendly relative timestamp rendered
// client-side by JS (a fresh app.js helper turns the RFC3339
// string into "2 minutes ago"); the server emits the raw
// value so the templ stays framework-agnostic.
type RecentJobEntry struct {
	ID            string
	Kind          string
	KindLabel     string // human label from Job.DisplayLabel()
	ActivityGroup string // Issue #556 slice 5: KindMeta.ActivityGroup
	Status        string // jobs.StatusDone, StatusError, StatusCancelled, StatusInterrupted
	StatusLabel   string // human label (Done / Error / Cancelled / Interrupted)
	Message       string
	ResultPath    string
	StartedAt     string // RFC3339
	FinishedAt    string // RFC3339
	DetailURL     string // /jobs/{id}
}

// ResearchPickerView is the page-level viewmodel for the Research &
// Review Person picker landing (issue #378 slice 1). Carries the bare
// minimum needed to render the slice-1 shell:
//
//   - CurrentPerson: the person currently in dd_person_ctx cookie context
//     (nil when no cookie). Renders the "Continue: <name> (#id)" shortcut.
//   - RecentPersons: last N viewed persons (slice 1: empty list; slice 3
//     lifts persistence to localStorage).
//   - SearchQuery: the most-recent search string (echoed into the input).
//   - SearchResults: matches for SearchQuery (slice 1: empty; live
//     htmx swap lands in a follow-up slice).
//
// The picker UI lives at internal/templates/research_picker.templ.
type ResearchPickerView struct {
	CurrentPerson *PersonRecord
	RecentPersons []PersonRecord
	SearchQuery   string
	SearchResults []PersonRecord
	NextAction    string
	// SupportedActions is the list of sub-page action names the
	// current person can support (issue #422 slice 2). Used by
	// the picker Continue shortcut to hide sub-pages the soldier
	// lacks data for (e.g. Camaraderie hidden when unit is empty).
	// Empty when no current person is set.
	SupportedActions []string
	// HasCountyInBirth reports whether the current person has a
	// county in birth_info. Used by the picker sub-screen to
	// hide the County option for soldiers without county data.
	HasCountyInBirth bool
}

// AboutView is the data shape the /about page renders
// (issue #585 + #586 + #594 + #598). Identity carries the
// version/codename/schema metadata; Activity is the baked
// repository-activity snapshot (nil in dev builds).
// RecentCommits is the baked per-commit projection for
// the new "Recent commits" section (nil in dev builds;
// the templ renders an empty-state notice in that case).
//
// Issue #598: the Release history section was removed —
// the recent-commits section is the replacement for
// "what just landed." The Releases []ReleaseEntry field
// and the ReleaseEntry struct are gone.
type AboutView struct {
	AppName       string
	Version       string
	Codename      string
	Schema        int
	Commit        string
	Branch        string
	BuiltAt       string
	LicenseURL    string
	Activity      *ActivitySnapshotView
	RecentCommits []RecentCommitView
	// Glossary is the project-wide terminology catalog
	// mirrored from `CONTEXT.md` (issue #564 slice 1).
	// The /about page renders every entry under the
	// `#about.glossary-<slug>` anchor; the (future) in-
	// context disclosure popover on a verified apply site
	// reads the Short field to populate its body.
	//
	// Sourced from `internal/glossary.Registry()`. The view
	// layer is purely a projection — the registry is the
	// single source of truth.
	Glossary []GlossaryEntry
}

// GlossaryEntry is the viewmodel projection of one
// internal/glossary.Term row (the templates package
// cannot import internal/glossary without a cycle, and
// the viewmodel contract is what the templ actually
// consumes — see plan §Module discipline).
//
// All fields are required. The templ partial renders
// Short as the row's preview line and Full as the
// rest of the row body; Related is the "See also"
// links at the bottom of the row.
type GlossaryEntry struct {
	Slug    string
	Term    string
	Short   string
	Full    string
	Related []string
}

// ActivityView is the per-release-commit row the /about
// page's Repository activity section renders inline next to
// the release-history bullets. CommitCount is 0 when the
// release's tag does not exist (CHANGELOG entry without a
// corresponding git tag); the templ partial renders "N/A"
// in that case.
type ActivityView struct {
	Version      string
	Date         string
	CommitCount  int
	Contributors int
	LinesAdded   int
	LinesRemoved int
}

// ActivitySnapshotView is the repository-activity section's
// data shape. HeatmapData is a JSON-encoded string of
// per-day counts (the templ partial hands it to the JS
// heatmap renderer). TopContributors is the viewmodel-
// projected list (max 10). PerRelease is the per-release
// commit rollup. IssuesClosed carries the by-Type breakdown.
type ActivitySnapshotView struct {
	GeneratedAt      string
	FirstCommitDate  string
	LatestCommitDate string
	TotalCommits     int
	TotalContributors int
	HeatmapData      string
	TopContributors  []Contributor
	PerRelease       []ActivityView
	IssuesClosed     IssuesClosedView
}

// RecentCommitView is the viewmodel projection of
// activityhistory.RecentCommit (the templ partial cannot
// import activityhistory without dragging in the build-time
// bake machinery). The view is flat: the templ renders
// each field directly + builds the GitHub permalink URL
// in the row template. Hash is the full 40-char SHA1; the
// templ uses Hash for the permalink URL and ShortHash for
// the visible hash text. Date is the YYYY-MM-DD slice of
// the commit timestamp; the templ renders a relative date
// (or ISO date fallback) via a small JS helper. Subject is
// the first line of the commit message; the templ renders
// it as the row's primary text + as the permalink anchor's
// text.
//
// Issue #594: the per-commit projection lives next to the
// ReleaseEntry projection in AboutView so the templ reads
// both slices from one place.
type RecentCommitView struct {
	Hash      string
	ShortHash string
	Date      string
	Author    string
	Subject   string
}

// Contributor is the viewmodel projection of
// activityhistory.ContributorCount.
type Contributor struct {
	Name  string
	Count int
}

// IssuesClosedView is the viewmodel projection of
// activityhistory.IssuesSummary.
type IssuesClosedView struct {
	TotalClosed int
	GeneratedAt string
	ByType      []TypeBucket
}

// TypeBucket is one entry in the issues-closed-by-Type
// stacked bar. Buckets are sorted by count descending so
// the templ partial renders the largest slice first.
type TypeBucket struct {
	Type  string
	Count int
}

// ConfigView is the viewmodel projection of the application
// config for the /settings/config sub-page (#638).
type ConfigView struct {
	Sections []ConfigSection
}

// ConfigSection is one config group (Window, UI, Limits, etc.)
// rendered as a card on the config sub-page.
type ConfigSection struct {
	Title  string
	Fields []ConfigField
}

// ConfigField is one key-value row in a config section.
type ConfigField struct {
	Label string
	Value string
}
