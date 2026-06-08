package models

type Soldier struct {
	ID                    int64    `json:"id"`
	DisplayID             string   `json:"display_id"`
	SyncID                string   `json:"sync_id"`
	EntryType             string   `json:"entry_type"`
	SpouseSoldierID       int64    `json:"spouse_soldier_id"`
	RelationshipLabel     string   `json:"relationship_label"`
	SpouseName            string   `json:"spouse_name"`
	MaidenName            string   `json:"maiden_name"`
	IsGenerated           bool     `json:"is_generated"`
	PensionID             string   `json:"pension_id"`
	ApplicationID         string   `json:"application_id"`
	Prefix                string   `json:"prefix"`
	ShowPrefixBeforeName  bool     `json:"show_prefix_before_name"`
	FirstName             string   `json:"first_name"`
	MiddleName            string   `json:"middle_name"`
	LastName              string   `json:"last_name"`
	Suffix                string   `json:"suffix"`
	Rank                  string   `json:"rank"`
	RankIn                string   `json:"rank_in"`
	RankOut               string   `json:"rank_out"`
	Unit                  string   `json:"unit"`
	PensionState          string   `json:"pension_state"`
	ConfederateHomeStatus string   `json:"confederate_home_status"`
	ConfederateHomeName   string   `json:"confederate_home_name"`
	DeathYear             int      `json:"death_year"`
	DeathMonth            int      `json:"death_month"`
	DeathDay              int      `json:"death_day"`
	BirthDate             string   `json:"birth_date"`
	DeathDate             string   `json:"death_date"`
	BirthInfo             string   `json:"birth_info"`
	BuriedIn              string   `json:"buried_in"`
	Biography             string   `json:"biography"`
	PDFExcerptOverride    string   `json:"pdf_excerpt_override"`
	Notes                 string   `json:"notes"`
	NeedsReview           bool     `json:"needs_review"`
	ReviewReason          string   `json:"review_reason"`
	AddedBy               string   `json:"added_by"`
	LastEditedBy          string   `json:"last_edited_by"`
	LastEditedFields      string   `json:"last_edited_fields"`
	LastEditedAt          string   `json:"last_edited_at"`
	CreatedAt             string   `json:"created_at"`
	UpdatedAt             string   `json:"updated_at"`
	SearchMatchField      string   `json:"-"`
	SearchMatchSnippet    string   `json:"-"`
	SpouseDisplayID       string   `json:"-"`
	BackLinkURL           string   `json:"-"`
	BackLinkLabel         string   `json:"-"`
	RecordCount           int      `json:"-"`
	ImageCount            int      `json:"-"`
	Records               []Record `json:"records,omitempty"`
	Images                []Image  `json:"images,omitempty"`
}

type ArchiveCounts struct {
	TotalSoldiers     int `json:"total_soldiers"`
	TotalWivesWidows  int `json:"total_wives_widows"`
	TotalLinkedPeople int `json:"total_linked_people"`
}

func (c ArchiveCounts) TotalRecords() int {
	return c.TotalSoldiers + c.TotalWivesWidows + c.TotalLinkedPeople
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
	RelationshipLabel     string
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
	RelationshipLabel   []string
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

type Record struct {
	ID            int64  `json:"id"`
	SyncID        string `json:"sync_id"`
	SoldierID     int64  `json:"soldier_id"`
	SoldierSyncID string `json:"soldier_sync_id"`
	RecordType    string `json:"record_type"`
	AppID         string `json:"app_id"`
	Details       string `json:"details"`
}

type Image struct {
	ID            int64  `json:"id"`
	SyncID        string `json:"sync_id"`
	SoldierID     int64  `json:"soldier_id"`
	SoldierSyncID string `json:"soldier_sync_id"`
	FileName      string `json:"file_name"`
	FilePath      string `json:"file_path"`
	Caption       string `json:"caption"`
	IsPrimary     bool   `json:"is_primary"`
	ResolvedPath  string `json:"-"`
}

type Quote struct {
	Author  string   `json:"author"`
	Text    string   `json:"text"`
	Context string   `json:"context"`
	Tags    []string `json:"tags,omitempty"`
}

const (
	CalendarItemTypeEvent   = "event"
	CalendarItemTypeHoliday = "holiday"
)

type CalendarItem struct {
	ID        int64  `json:"id"`
	ItemType  string `json:"item_type"`
	Month     int    `json:"month"`
	Day       int    `json:"day"`
	Title     string `json:"title"`
	Notes     string `json:"notes"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type GoogleSettings struct {
	ClientID      string `json:"client_id"`
	ClientSecret  string `json:"client_secret"`
	CalendarID    string `json:"calendar_id"`
	DriveFolderID string `json:"drive_folder_id"`
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
	ManagedCalendarID     string
	TestCalendarID        string
	LastSyncedAt          string
	OutOfSync             bool
	DriftAdded            int
	DriftUpdated          int
	DriftRemoved          int
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

type UserIdentity struct {
	FirstName  string
	MiddleName string
	LastName   string
	BirthYear  int
	NodePrefix string
}

type InitialSetupForm struct {
	FirstName     string
	MiddleName    string
	LastName      string
	BirthYear     string
	PrefixPreview string
	ErrorMessage  string
}
