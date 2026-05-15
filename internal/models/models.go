package models

type Soldier struct {
	ID            int64    `json:"id"`
	DisplayID     string   `json:"display_id"`
	SyncID        string   `json:"sync_id"`
	IsGenerated   bool     `json:"is_generated"`
	PensionID     string   `json:"pension_id"`
	ApplicationID string   `json:"application_id"`
	FirstName     string   `json:"first_name"`
	MiddleName    string   `json:"middle_name"`
	LastName      string   `json:"last_name"`
	Rank          string   `json:"rank"`
	RankIn        string   `json:"rank_in"`
	RankOut       string   `json:"rank_out"`
	Unit          string   `json:"unit"`
	PensionState  string   `json:"pension_state"`
	DeathYear     int      `json:"death_year"`
	DeathMonth    int      `json:"death_month"`
	DeathDay      int      `json:"death_day"`
	BirthDate     string   `json:"birth_date"`
	DeathDate     string   `json:"death_date"`
	BirthInfo     string   `json:"birth_info"`
	BuriedIn      string   `json:"buried_in"`
	Notes         string   `json:"notes"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`
	Records       []Record `json:"records,omitempty"`
	Images        []Image  `json:"images,omitempty"`
}

type SoldierSearch struct {
	Mode         string
	Query        string
	Browse       bool
	DisplayID    string
	FirstName    string
	MiddleName   string
	LastName     string
	Rank         string
	Unit         string
	PensionState string
	BuriedIn     string
	DeathYear    string
	DeathMonth   string
	DeathDay     string
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
	ResolvedPath  string `json:"-"`
}

type Quote struct {
	Author  string   `json:"author"`
	Text    string   `json:"text"`
	Context string   `json:"context"`
	Tags    []string `json:"tags,omitempty"`
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
}
