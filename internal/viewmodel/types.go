package viewmodel

type PersonRecord struct {
	ID                    int64
	DisplayID             string
	SyncID                string
	EntryType             string
	LinkedSoldierID       int64
	SpouseName            string
	MaidenName            string
	IsGenerated           bool
	PensionID             string
	ApplicationID         string
	Prefix                string
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
}

type SourceRecord struct {
	ID                 int64
	SyncID             string
	PersonRecordID     int64
	PersonRecordSyncID string
	SourceRecordType   string
	AppID              string
	Details            string
}

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

type ArchiveCounts struct {
	SoldierCount      int
	SpouseRecordCount int
}

type Quote struct {
	Author  string
	Text    string
	Context string
	Tags    []string
}

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
}

type PersonRecordFormSuggestions struct {
	RankIn           []string
	RankOut          []string
	Unit             []string
	Prefix           []string
	Suffix           []string
	PensionState     []string
	BuriedIn         []string
	ConfederateHome  []string
	SourceRecordType []string
}

type ScrapedRelative struct {
	Name       string
	MemorialID string
	URL        string
	BirthYear  string
	DeathYear  string
}

type FindAGraveScrapeState struct {
	Input           string
	SourceLabel     string
	ErrorMessage    string
	WarningLines    []string
	Spouses         []ScrapedRelative
	ConfidenceScore int
}

type InitialSetupForm struct {
	FirstName     string
	MiddleName    string
	LastName      string
	BirthYear     string
	PrefixPreview string
	ErrorMessage  string
}

type GoogleSettings struct {
	ClientID      string
	ClientSecret  string
	CalendarID    string
	DriveFolderID string
}

type GoogleStatus struct {
	Settings              GoogleSettings
	Connected             bool
	HasClientID           bool
	HasSecret             bool
	HasToken              bool
	SharedClientAvailable bool
	SharedClientSource    string
	UsingSharedClient     bool
}

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

type DuplicateAuditSummary struct {
	OpenFindings        int
	ResolvedFindings    int
	LastRunAt           string
	SimilarityThreshold int
}

type DuplicateAuditFindingSummary struct {
	ID                  int64
	OtherPersonRecordID int64
	OtherDisplayID      string
	OtherName           string
	Reason              string
}

type ReviewQueueEntry struct {
	PersonRecord      PersonRecord
	DuplicateFindings []DuplicateAuditFindingSummary
}

type DuplicateAuditComparisonField struct {
	Key         string
	Label       string
	LeftValue   string
	RightValue  string
	Highlighted bool
}

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

type AnalyticsCount struct {
	Label string
	Count int
}

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

type UnitCamaraderieGraph struct {
	CentralSoldier     PersonRecord
	UnitLabel          string
	RegimentLabel      string
	CompanyLabel       string
	SameUnit           []UnitCamaraderieConnection
	SameCompanyVariant []UnitCamaraderieConnection
	SameRegiment       []UnitCamaraderieConnection
}

type UnitCamaraderieConnection struct {
	Soldier      PersonRecord
	Relation     string
	Strength     int
	StrengthText string
}

type ServiceTimeline struct {
	SubjectSoldier       PersonRecord
	TimelineEvents       []TimelineEvent
	UndatedSourceRecords []SourceRecord
	StartLabel           string
	EndLabel             string
	ExactEventCount      int
	InferredEventCount   int
}

type TimelineEvent struct {
	Title           string
	DateLabel       string
	Description     string
	SourceLabel     string
	Category        string
	ConfidenceLabel string
	Approximate     bool
}

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

type ResearchTaskSuggestion struct {
	Title        string
	Notes        string
	EvidenceType string
}

type ResearchLog struct {
	SubjectPersonRecord PersonRecord
	Tasks               []ResearchTask
	Suggestions         []ResearchTaskSuggestion
	OpenCount           int
	ResolvedCount       int
}

type ResearchPack struct {
	AnchorPersonRecord   PersonRecord
	Scope                string
	PlaceLabel           string
	Description          string
	RelatedPersonRecords []PersonRecord
	TopUnits             []AnalyticsCount
	TopCemeteries        []AnalyticsCount
	OpenReviewCount      int
}

type ResearchCollection struct {
	ID              int64
	Name            string
	Description     string
	CreatedAt       string
	UpdatedAt       string
	ItemCount       int
	ContainsCurrent bool
}

type ResearchCollectionHub struct {
	CurrentPersonRecord *PersonRecord
	Collections         []ResearchCollection
}

type ResearchCollectionDetail struct {
	Collection          ResearchCollection
	CurrentPersonRecord *PersonRecord
	PersonRecords       []PersonRecord
}

type MergeReviewLedger struct {
	SubjectPersonRecord PersonRecord
	Entries             []MergeReviewLedgerEntry
	OpenCount           int
	ResolvedCount       int
}

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

type OrphanedImage struct {
	RelativePath string
	Size         int64
	ModifiedAt   string
}

type Soldier = PersonRecord
type Record = SourceRecord
type SoldierSearch = PersonRecordSearch
type SoldierFormSuggestions = PersonRecordFormSuggestions
type ServiceTimelineEvent = TimelineEvent
type SourceConflictLedger = MergeReviewLedger
type SourceConflictLedgerEntry = MergeReviewLedgerEntry
