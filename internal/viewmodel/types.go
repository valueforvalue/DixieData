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
	// v60 (issue #320): Event Record subtype fields. Kind is
	// free-text (e.g. "Battle", "Earthquake", "Hospital Stay");
	// BeginDate / EndDate follow the soldiers MM/DD/YYYY canonical
	// date shape so the existing date filter predicates apply.
	// Description is the long-form write-up (mirrors the
	// Person Record Biography field). All four are also stored
	// on the domain models.Soldier type; the viewmodel copies
	// them so .templ files can render event-only fields without
	// importing internal/models directly.
	Kind                  string
	BeginDate             string
	EndDate               string
	Description           string
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
	// EventSources is populated only for Event Record rows
	// (entry_type = 'event'). Person Records always leave it
	// empty. Issue #340 / v61: per-Event sources live in their
	// own event_sources table; SourceRecords (above) belongs to
	// Person Records.
	EventSources          []SourceRecord
	Images                []Image
	Tags                  []TagOption
	// LinkedPersons is populated only for Event Record rows
	// (entry_type = 'event'). Carries the Person Records linked
	// via event_person_links so the event editor can render an
	// inline link/unlink surface without a second round-trip.
	// Issue #361 slice 2.
	LinkedPersons         []PersonRecord
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
type ArchiveCounts struct {
	SoldierCount      int
	SpouseRecordCount int
	PersonRecordCount int
}

// TotalRecords returns the sum of all Person Record subtypes. Mirrors
// models.ArchiveCounts.TotalRecords so the empty-state partial can
// decide whether the Local Archive is truly empty (issue #98).
func (c ArchiveCounts) TotalRecords() int {
	return c.SoldierCount + c.SpouseRecordCount + c.PersonRecordCount
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
	ID          string
	Kind        string
	KindLabel   string // human label from Job.DisplayLabel()
	Status      string // jobs.StatusDone, StatusError, StatusCancelled, StatusInterrupted
	StatusLabel string // human label (Done / Error / Cancelled / Interrupted)
	Message     string
	ResultPath  string
	StartedAt   string // RFC3339
	FinishedAt  string // RFC3339
	DetailURL   string // /jobs/{id}
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
