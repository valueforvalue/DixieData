package models

type Soldier struct {
	ID          int64    `json:"id"`
	DisplayID   string   `json:"display_id"`
	IsGenerated bool     `json:"is_generated"`
	FirstName   string   `json:"first_name"`
	LastName    string   `json:"last_name"`
	Rank        string   `json:"rank"`
	Unit        string   `json:"unit"`
	DeathYear   int      `json:"death_year"`
	DeathMonth  int      `json:"death_month"`
	DeathDay    int      `json:"death_day"`
	BirthInfo   string   `json:"birth_info"`
	Notes       string   `json:"notes"`
	CreatedAt   string   `json:"created_at"`
	Records     []Record `json:"records,omitempty"`
	Images      []Image  `json:"images,omitempty"`
}

type SoldierSearch struct {
	Mode       string
	Query      string
	DisplayID  string
	FirstName  string
	LastName   string
	Rank       string
	Unit       string
	DeathYear  string
	DeathMonth string
	DeathDay   string
}

type Record struct {
	ID         int64  `json:"id"`
	SoldierID  int64  `json:"soldier_id"`
	RecordType string `json:"record_type"`
	AppID      string `json:"app_id"`
	Details    string `json:"details"`
}

type Image struct {
	ID        int64  `json:"id"`
	SoldierID int64  `json:"soldier_id"`
	FileName  string `json:"file_name"`
	FilePath  string `json:"file_path"`
	Caption   string `json:"caption"`
}
