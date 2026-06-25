package models

import (
	"fmt"
	"strconv"
	"strings"
)

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
	// LinkedSpouseDisplayID is set by the typst payload path
	// (internal/archive/export_service.go) when the soldier has
	// a SpouseSoldierID. The typst template uses it to render
	// the linked spouse reference as the user-facing DXD-XXXXX
	// identifier instead of the internal SQL primary key. The
	// `json:"-"` tag means this field is never serialized by
	// the default JSON encoder; the export service manually
	// injects it into the typst data payload via the registry
	// (see the registry's typed-soldier passthrough in
	// pkg/render).
	LinkedSpouseDisplayID string   `json:"linked_spouse_display_id,omitempty"`
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
	// IsArchiveEmpty and TotalRecordCount are populated by the handler
	// from ArchiveCounts so the search results template can show a
	// Setup card on first-run / truly-empty archives. See audit
	// issue #98 from the 2026-06-24 sweep.
	IsArchiveEmpty   bool
	TotalRecordCount int
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
	// ResolvedPath is the absolute path on the user's filesystem.
	// It is set by the appshell before each export so the renderer
	// does not have to know where the data dir lives. We DO
	// serialize it (json:"resolved_path") because the tune tool
	// round-trips soldier data through JSON, and dropping it
	// would mean the image-staging step in the renderer could
	// not find the file. The appshell and the tune tool set it
	// just before calling the renderer, and the value is
	// transient — the renderer copies the file into its
	// workdir and the resolved path never reaches a user-facing
	// output (it lives only in the typst workdir's data.json,
	// which is cleaned up after the compile).
	ResolvedPath  string `json:"resolved_path"`
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
	ClientID                string                   `json:"client_id"`
	ClientSecret            string                   `json:"client_secret"`
	CalendarID              string                   `json:"calendar_id"`
	DriveFolderID           string                   `json:"drive_folder_id"`
	ManagedEventPreferences CalendarEventPreferences `json:"managed_event_preferences"`
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

const (
	CalendarEventTitlePresetMemorial = "memorial_full_name"
	CalendarEventTitlePresetNameLead = "full_name_memorial"
	CalendarEventTitlePresetDisplay  = "display_id_full_name"
)

type CalendarEventPreferences struct {
	TitlePreset         string `json:"title_preset,omitempty"`
	StartTime           string `json:"start_time,omitempty"`
	ReminderPrimary     string `json:"reminder_primary,omitempty"`
	ReminderSecondary   string `json:"reminder_secondary,omitempty"`
	IncludeRecordID     bool   `json:"include_record_id"`
	IncludeUnit         bool   `json:"include_unit"`
	IncludeBuriedIn     bool   `json:"include_buried_in"`
	IncludeOriginalDate bool   `json:"include_original_date"`
}

func DefaultCalendarEventPreferences() CalendarEventPreferences {
	return CalendarEventPreferences{
		TitlePreset:         CalendarEventTitlePresetMemorial,
		StartTime:           "09:00",
		ReminderPrimary:     "1d",
		ReminderSecondary:   "1h",
		IncludeRecordID:     true,
		IncludeUnit:         true,
		IncludeBuriedIn:     true,
		IncludeOriginalDate: true,
	}
}

func NormalizeCalendarEventPreferences(input CalendarEventPreferences) CalendarEventPreferences {
	normalized := DefaultCalendarEventPreferences()
	switch strings.TrimSpace(input.TitlePreset) {
	case CalendarEventTitlePresetNameLead, CalendarEventTitlePresetDisplay:
		normalized.TitlePreset = strings.TrimSpace(input.TitlePreset)
	}
	if validCalendarEventTime(input.StartTime) {
		normalized.StartTime = strings.TrimSpace(input.StartTime)
	}
	if _, ok := CalendarReminderMinutes(strings.TrimSpace(input.ReminderPrimary)); ok {
		normalized.ReminderPrimary = strings.TrimSpace(input.ReminderPrimary)
	}
	if _, ok := CalendarReminderMinutes(strings.TrimSpace(input.ReminderSecondary)); ok {
		normalized.ReminderSecondary = strings.TrimSpace(input.ReminderSecondary)
	}
	if normalized.ReminderPrimary != "none" && normalized.ReminderPrimary == normalized.ReminderSecondary {
		normalized.ReminderSecondary = "none"
	}
	if input.IncludeRecordID || input.IncludeUnit || input.IncludeBuriedIn || input.IncludeOriginalDate {
		normalized.IncludeRecordID = input.IncludeRecordID
		normalized.IncludeUnit = input.IncludeUnit
		normalized.IncludeBuriedIn = input.IncludeBuriedIn
		normalized.IncludeOriginalDate = input.IncludeOriginalDate
	}
	return normalized
}

func CalendarReminderMinutes(value string) (int64, bool) {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "none":
		return 0, true
	case "1h":
		return 60, true
	case "3h":
		return 3 * 60, true
	case "12h":
		return 12 * 60, true
	case "1d":
		return 24 * 60, true
	case "2d":
		return 2 * 24 * 60, true
	case "1w":
		return 7 * 24 * 60, true
	default:
		return 0, false
	}
}

func CalendarTimeComponents(value string) (int, int, bool) {
	trimmed := strings.TrimSpace(value)
	parts := strings.Split(trimmed, ":")
	if len(parts) != 2 {
		return 0, 0, false
	}
	hour, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, false
	}
	minute, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, false
	}
	if hour < 5 || hour > 23 {
		return 0, 0, false
	}
	if minute < 0 || minute > 59 || minute%15 != 0 {
		return 0, 0, false
	}
	return hour, minute, true
}

func CalendarTimeLabel(value string) string {
	hour, minute, ok := CalendarTimeComponents(value)
	if !ok {
		return "09:00"
	}
	return fmt.Sprintf("%02d:%02d", hour, minute)
}

func validCalendarEventTime(value string) bool {
	_, _, ok := CalendarTimeComponents(value)
	return ok
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
