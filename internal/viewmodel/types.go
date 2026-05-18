package viewmodel

type Soldier struct {
	ID                    int64
	DisplayID             string
	SyncID                string
	EntryType             string
	SpouseSoldierID       int64
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
	RecordCount           int
	ImageCount            int
	Records               []Record
	Images                []Image
}

type Record struct {
	ID            int64
	SyncID        string
	SoldierID     int64
	SoldierSyncID string
	RecordType    string
	AppID         string
	Details       string
}

type Image struct {
	ID            int64
	SyncID        string
	SoldierID     int64
	SoldierSyncID string
	FileName      string
	FilePath      string
	Caption       string
	IsPrimary     bool
	ResolvedPath  string
}

type ArchiveCounts struct {
	TotalSoldiers    int
	TotalWivesWidows int
}

type Quote struct {
	Author  string
	Text    string
	Context string
	Tags    []string
}

type SoldierSearch struct {
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
	RecordType            string
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

type SoldierFormSuggestions struct {
	RankIn              []string
	RankOut             []string
	Unit                []string
	Prefix              []string
	Suffix              []string
	PensionState        []string
	BuriedIn            []string
	ConfederateHomeName []string
	RecordType          []string
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
	ID              int64
	SessionID       string
	ConflictType    string
	Reason          string
	LocalSoldierID  int64
	LocalDisplayID  string
	SourceDisplayID string
	Resolution      string
	CreatedAt       string
	LocalSoldier    *Soldier
	SourceSoldier   Soldier
}

type DuplicateAuditSummary struct {
	OpenFindings        int
	ResolvedFindings    int
	LastRunAt           string
	SimilarityThreshold int
}

type DuplicateAuditFindingSummary struct {
	ID             int64
	OtherSoldierID int64
	OtherDisplayID string
	OtherName      string
	Reason         string
}

type ReviewQueueEntry struct {
	Soldier           Soldier
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
	FindingID    int64
	FindingType  string
	PageTitle    string
	BackHref     string
	BackLabel    string
	Reason       string
	Status       string
	LeftSoldier  Soldier
	RightSoldier Soldier
	Fields       []DuplicateAuditComparisonField
}

type AnalyticsCount struct {
	Label string
	Count int
}

type AnalyticsSnapshot struct {
	RecordTypes             ArchiveCounts
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
	Central            Soldier
	UnitLabel          string
	RegimentLabel      string
	CompanyLabel       string
	SameUnit           []UnitCamaraderieConnection
	SameCompanyVariant []UnitCamaraderieConnection
	SameRegiment       []UnitCamaraderieConnection
}

type UnitCamaraderieConnection struct {
	Soldier      Soldier
	Relation     string
	Strength     int
	StrengthText string
}

type ServiceTimeline struct {
	Central            Soldier
	Events             []ServiceTimelineEvent
	UndatedRecords     []Record
	StartLabel         string
	EndLabel           string
	ExactEventCount    int
	InferredEventCount int
}

type ServiceTimelineEvent struct {
	Title           string
	DateLabel       string
	Description     string
	SourceLabel     string
	Category        string
	ConfidenceLabel string
	Approximate     bool
}

type ResearchTask struct {
	ID           int64
	SoldierID    int64
	Title        string
	Notes        string
	EvidenceType string
	Status       string
	CreatedAt    string
	UpdatedAt    string
	ResolvedAt   string
}

type ResearchTaskSuggestion struct {
	Title        string
	Notes        string
	EvidenceType string
}

type ResearchLog struct {
	Central       Soldier
	Tasks         []ResearchTask
	Suggestions   []ResearchTaskSuggestion
	OpenCount     int
	ResolvedCount int
}

type ResearchPack struct {
	Central         Soldier
	Scope           string
	PlaceLabel      string
	Description     string
	Related         []Soldier
	TopUnits        []AnalyticsCount
	TopCemeteries   []AnalyticsCount
	OpenReviewCount int
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
	Current     *Soldier
	Collections []ResearchCollection
}

type ResearchCollectionDetail struct {
	Collection ResearchCollection
	Current    *Soldier
	Members    []Soldier
}

type SourceConflictLedger struct {
	Central       Soldier
	Entries       []SourceConflictLedgerEntry
	OpenCount     int
	ResolvedCount int
}

type SourceConflictLedgerEntry struct {
	ID               int64
	ConflictType     string
	Reason           string
	SourceDisplayID  string
	Resolution       string
	CreatedAt        string
	ResolvedAt       string
	LocalSnapshot    Soldier
	SourceSnapshot   Soldier
	DifferenceFields []string
}

type OrphanedImage struct {
	RelativePath string
	Size         int64
	ModifiedAt   string
}
