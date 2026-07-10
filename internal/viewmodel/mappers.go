// The mapper functions in this file convert domain-layer types
// (records.*, models.*) to their UI-shaped viewmodel projections.
// Each mapper follows the naming convention `<ViewmodelType>From<DomainType>`
// (or `<ViewmodelType>FromDomain` when the source is records.*).
//
// Mappers are mechanical: they copy fields and apply the
// display-ready defaults the templ templates expect. They do not
// query the database or perform validation; they assume the
// source has already been loaded and checked.
//
// Map-mappers (suffix `FromModels` / `FromDomains`) accept a
// slice or map and return the same shape converted element-wise.
package viewmodel

import (
	"strings"

	"github.com/valueforvalue/DixieData/internal/archive"
	"github.com/valueforvalue/DixieData/internal/confederatehomestatus"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/pensionstate"
	"github.com/valueforvalue/DixieData/internal/persondisplay"
	"github.com/valueforvalue/DixieData/internal/records"
)

// PersonRecordFromModel converts a domain-type value into its viewmodel projection.
func PersonRecordFromModel(input models.Soldier) PersonRecord {
	return PersonRecord{
		ID:                    input.ID,
		DisplayID:             input.DisplayID,
		SyncID:                input.SyncID,
		EntryType:             input.EntryType,
		LinkedSoldierID:       input.SpouseSoldierID,
		RelationshipLabel:     input.RelationshipLabel,
		SpouseName:            input.SpouseName,
		MaidenName:            input.MaidenName,
		IsGenerated:           input.IsGenerated,
		PensionID:             input.PensionID,
		ApplicationID:         input.ApplicationID,
		Prefix:                input.Prefix,
		ShowPrefixBeforeName:  input.ShowPrefixBeforeName,
		FirstName:             input.FirstName,
		MiddleName:            input.MiddleName,
		LastName:              input.LastName,
		Suffix:                input.Suffix,
		Rank:                  input.Rank,
		RankIn:                input.RankIn,
		RankOut:               input.RankOut,
		Unit:                  input.Unit,
		PensionState:          pensionstate.Normalize(input.PensionState),
		ConfederateHomeStatus: input.ConfederateHomeStatus,
		ConfederateHomeName:   input.ConfederateHomeName,
		DeathYear:             input.DeathYear,
		DeathMonth:            input.DeathMonth,
		DeathDay:              input.DeathDay,
		BirthDate:             input.BirthDate,
		DeathDate:             input.DeathDate,
		BirthInfo:             input.BirthInfo,
		BuriedIn:              input.BuriedIn,
		Biography:             input.Biography,
		PDFExcerptOverride:    input.PDFExcerptOverride,
		Notes:                 input.Notes,
		// v60 (issue #320): Event Record subtype fields. The
		// source models.Soldier already carries Kind /
		// BeginDate / EndDate / Description; the viewmodel
		// projection copies them so .templ files can render
		// event-only fields without importing internal/models.
		Kind:                  input.Kind,
		BeginDate:             input.BeginDate,
		EndDate:               input.EndDate,
		Description:           input.Description,
		NeedsReview:           input.NeedsReview,
		ReviewReason:          input.ReviewReason,
		AddedBy:               input.AddedBy,
		LastEditedBy:          input.LastEditedBy,
		LastEditedFields:      input.LastEditedFields,
		LastEditedAt:          input.LastEditedAt,
		CreatedAt:             input.CreatedAt,
		UpdatedAt:             input.UpdatedAt,
		SearchMatchField:      input.SearchMatchField,
		SearchMatchSnippet:    input.SearchMatchSnippet,
		SpouseDisplayID:       input.SpouseDisplayID,
		BackLinkURL:           input.BackLinkURL,
		BackLinkLabel:         input.BackLinkLabel,
		SourceRecordCount:     input.RecordCount,
		ImageCount:            input.ImageCount,
		SourceRecords:         SourceRecordsFromModels(input.Records),
		// Issue #340 / v61: Event sources now live in their own
		// table. For Person Records this stays empty; for Events
		// it carries the per-Event sources. The Sources panel on
		// event_detail.templ reads EventSources (slice 5 of the
		// v61 decomposition) instead of SourceRecords.
		EventSources:          SourceRecordsFromModels(input.EventSources),
		Images:                ImagesFromModels(input.Images),
		// Issue #377 / #423: row provenance fields (slice 3
		// surfaces in the Soldier detail page footer + the
		// data-quality scan results). v64 CreatedBy* are
		// stamped at Create time; v65 RestoredAt is stamped
		// by restoreSnapshotBackup's bulk UPDATE. All three
		// default to '' when not populated (e.g. a row
		// written before the v64 migration carries
		// CreatedBy* = "unknown" from the backfill;
		// RestoredAt is empty until the row is carried
		// over a restore point).
		CreatedByVersion:    input.CreatedByVersion,
		CreatedByImportPath: input.CreatedByImportPath,
		RestoredAt:          input.RestoredAt,
	}
}

// SoldierFromModel converts a domain-type value into its viewmodel projection.
func SoldierFromModel(input models.Soldier) PersonRecord {
	return PersonRecordFromModel(input)
}

// PersonRecordPtrFromModel converts a domain-type value into its viewmodel projection.
func PersonRecordPtrFromModel(input *models.Soldier) *PersonRecord {
	if input == nil {
		return nil
	}
	mapped := PersonRecordFromModel(*input)
	return &mapped
}

// SoldierPtrFromModel converts a domain-type value into its viewmodel projection.
func SoldierPtrFromModel(input *models.Soldier) *PersonRecord {
	return PersonRecordPtrFromModel(input)
}

// PersonRecordsFromModels converts a domain-type value into its viewmodel projection.
func PersonRecordsFromModels(inputs []models.Soldier) []PersonRecord {
	items := make([]PersonRecord, 0, len(inputs))
	for _, input := range inputs {
		items = append(items, PersonRecordFromModel(input))
	}
	return items
}

// SoldiersFromModels converts a domain-type value into its viewmodel projection.
func SoldiersFromModels(inputs []models.Soldier) []PersonRecord {
	return PersonRecordsFromModels(inputs)
}

// PersonRecordsFromModelsWithTags builds PersonRecord viewmodels and
// zips in tag data from a map keyed by soldier ID. Existing callers
// that don't need tags continue to use PersonRecordsFromModels;
// browse and browse-results use this variant.
func PersonRecordsFromModelsWithTags(inputs []models.Soldier, tagMap map[int64][]TagOption) []PersonRecord {
	items := make([]PersonRecord, 0, len(inputs))
	for _, input := range inputs {
		rec := PersonRecordFromModel(input)
		if tags, ok := tagMap[input.ID]; ok {
			rec.Tags = tags
		}
		items = append(items, rec)
	}
	return items
}

// ExportRecordOptionFromModel converts a domain-type value into its viewmodel projection.
func ExportRecordOptionFromModel(input models.Soldier) ExportRecordOption {
	displayName := strings.TrimSpace(persondisplay.FullName(persondisplay.NameParts{
		Prefix:               input.Prefix,
		ShowPrefixBeforeName: input.ShowPrefixBeforeName,
		FirstName:            input.FirstName,
		MiddleName:           input.MiddleName,
		LastName:             input.LastName,
		Suffix:               input.Suffix,
	}))
	if displayName == "" {
		displayName = strings.TrimSpace(input.SpouseName)
	}
	if displayName == "" {
		displayName = strings.TrimSpace(input.DisplayID)
	}
	return ExportRecordOption{
		ID:                    input.ID,
		DisplayID:             input.DisplayID,
		DisplayName:           displayName,
		EntryType:             strings.TrimSpace(strings.ToLower(input.EntryType)),
		Unit:                  strings.TrimSpace(input.Unit),
		PensionState:          pensionstate.Normalize(input.PensionState),
		ConfederateHomeStatus: confederatehomestatus.Normalize(input.ConfederateHomeStatus),
		BuriedIn:              strings.TrimSpace(input.BuriedIn),
	}
}

// ExportRecordOptionsFromModels converts a domain-type value into its viewmodel projection.
func ExportRecordOptionsFromModels(inputs []models.Soldier) []ExportRecordOption {
	items := make([]ExportRecordOption, 0, len(inputs))
	for _, input := range inputs {
		items = append(items, ExportRecordOptionFromModel(input))
	}
	return items
}

// SourceRecordFromModel converts a domain-type value into its viewmodel projection.
func SourceRecordFromModel(input models.Record) SourceRecord {
	return SourceRecord{
		ID:                 input.ID,
		SyncID:             input.SyncID,
		PersonRecordID:     input.PersonRecordID,
		PersonRecordSyncID: input.PersonSyncID,
		SourceRecordType:   input.RecordType,
		AppID:              input.AppID,
		Details:            input.Details,
	}
}

// RecordFromModel converts a domain-type value into its viewmodel projection.
func RecordFromModel(input models.Record) SourceRecord {
	return SourceRecordFromModel(input)
}

// SourceRecordsFromModels converts a domain-type value into its viewmodel projection.
func SourceRecordsFromModels(inputs []models.Record) []SourceRecord {
	items := make([]SourceRecord, 0, len(inputs))
	for _, input := range inputs {
		items = append(items, SourceRecordFromModel(input))
	}
	return items
}

// RecordsFromModels converts a domain-type value into its viewmodel projection.
func RecordsFromModels(inputs []models.Record) []SourceRecord {
	return SourceRecordsFromModels(inputs)
}

// ImageFromModel converts a domain-type value into its viewmodel projection.
func ImageFromModel(input models.Image) Image {
	return Image{
		ID:                 input.ID,
		SyncID:             input.SyncID,
		PersonRecordID:     input.PersonRecordID,
		PersonRecordSyncID: input.PersonSyncID,
		FileName:           input.FileName,
		FilePath:           input.FilePath,
		Caption:            input.Caption,
		IsPrimary:          input.IsPrimary,
		ResolvedPath:       input.ResolvedPath,
	}
}

// ImagesFromModels converts a domain-type value into its viewmodel projection.
func ImagesFromModels(inputs []models.Image) []Image {
	items := make([]Image, 0, len(inputs))
	for _, input := range inputs {
		items = append(items, ImageFromModel(input))
	}
	return items
}

// ArchiveCountsFromModel converts a domain-type value into its viewmodel projection.
func ArchiveCountsFromModel(input models.ArchiveCounts) ArchiveCounts {
	return ArchiveCounts{
		SoldierCount:      input.TotalSoldiers,
		SpouseRecordCount: input.TotalWivesWidows,
		PersonRecordCount: input.TotalLinkedPeople,
	}
}

// QuoteFromModel converts a domain-type value into its viewmodel projection.
func QuoteFromModel(input models.Quote) Quote { return Quote(input) }

// CalendarDaySummaryFromDomain converts a domain-type value into its viewmodel projection.
func CalendarDaySummaryFromDomain(input records.CalendarDaySummary) CalendarDaySummary {
	return CalendarDaySummary{
		AnniversaryCount: input.AnniversaryCount,
		EventCount:       input.EventCount,
		HolidayCount:     input.HolidayCount,
	}
}

// CalendarDaySummariesFromDomain converts a domain-type value into its viewmodel projection.
func CalendarDaySummariesFromDomain(inputs map[int]records.CalendarDaySummary) map[int]CalendarDaySummary {
	items := make(map[int]CalendarDaySummary, len(inputs))
	for day, input := range inputs {
		items[day] = CalendarDaySummaryFromDomain(input)
	}
	return items
}

// CalendarItemFromModel converts a domain-type value into its viewmodel projection.
func CalendarItemFromModel(input models.CalendarItem) CalendarItem {
	return CalendarItem{
		ID:       input.ID,
		ItemType: input.ItemType,
		Title:    input.Title,
		Notes:    input.Notes,
	}
}

// CalendarItemsFromModels converts a domain-type value into its viewmodel projection.
func CalendarItemsFromModels(inputs []models.CalendarItem) []CalendarItem {
	items := make([]CalendarItem, 0, len(inputs))
	for _, input := range inputs {
		items = append(items, CalendarItemFromModel(input))
	}
	return items
}

// CalendarDayDetailFromDomain converts a domain-type value into its viewmodel projection.
func CalendarDayDetailFromDomain(input records.CalendarDay, form CalendarItemForm, statusKind, statusMessage string) CalendarDayDetail {
	if strings.TrimSpace(form.ItemType) == "" {
		form.ItemType = models.CalendarItemTypeEvent
	}
	return CalendarDayDetail{
		Month:            input.Month,
		Day:              input.Day,
		AllowCustomItems: input.Day >= 1,
		StatusKind:       statusKind,
		StatusMessage:    statusMessage,
		Form:             form,
		Items:            CalendarItemsFromModels(input.Items),
		Anniversaries:    PersonRecordsFromModels(input.Anniversaries),
	}
}

// QuotesFromModels converts a domain-type value into its viewmodel projection.
func QuotesFromModels(inputs []models.Quote) []Quote {
	items := make([]Quote, 0, len(inputs))
	for _, input := range inputs {
		items = append(items, QuoteFromModel(input))
	}
	return items
}

// PersonRecordSearchFromModel converts a domain-type value into its viewmodel projection.
func PersonRecordSearchFromModel(input models.SoldierSearch) PersonRecordSearch {
	return PersonRecordSearch{
		Mode:                  input.Mode,
		Query:                 input.Query,
		Browse:                input.Browse,
		Recent:                input.Recent,
		DisplayID:             input.DisplayID,
		EntryType:             input.EntryType,
		FirstName:             input.FirstName,
		MiddleName:            input.MiddleName,
		LastName:              input.LastName,
		MaidenName:            input.MaidenName,
		RelationshipLabel:     input.RelationshipLabel,
		Rank:                  input.Rank,
		RankIn:                input.RankIn,
		RankOut:               input.RankOut,
		Unit:                  input.Unit,
		SourceRecordType:      input.RecordType,
		PensionState:          pensionstate.Normalize(input.PensionState),
		ConfederateHomeStatus: input.ConfederateHomeStatus,
		ConfederateHomeName:   input.ConfederateHomeName,
		BuriedIn:              input.BuriedIn,
		ReviewStatus:          input.ReviewStatus,
		BirthDate:             input.BirthDate,
		BirthYear:             input.BirthYear,
		BirthYearTo:           input.BirthYearTo,
		DeathDate:             input.DeathDate,
		DeathYear:             input.DeathYear,
		DeathYearTo:           input.DeathYearTo,
		DeathMonth:            input.DeathMonth,
		DeathDay:              input.DeathDay,
		IsArchiveEmpty:        input.IsArchiveEmpty,
		TotalRecordCount:      input.TotalRecordCount,
	}
}

// BrowseStateFromDomain converts a domain-type value into its viewmodel projection.
func BrowseStateFromDomain(input records.BrowseRequest, total int) BrowseState {
	selected := make(map[string]bool, len(input.Tags))
	for _, t := range input.Tags {
		selected[records.NormalizeTagName(t)] = true
	}
	return BrowseState{
		Page:                  input.Page,
		PageSize:              input.PageSize,
		Total:                 total,
		Scope:                 input.Scope,
		Sort:                  input.Sort,
		EntryType:             input.EntryType,
		Unit:                  input.Unit,
		BuriedIn:              input.BuriedIn,
		PensionState:          normalizeOptionalText(input.PensionState),
		ReviewStatus:          input.ReviewStatus,
		ConfederateHomeStatus: input.ConfederateHomeStatus,
		SelectedTagSet:        selected,
	}
}

// TagsFromModels maps records.Tag slice into the minimal
// Browse-side payload (TagOption). The browse chip cloud only
// needs id, name, normalized name so the rendered page payload
// stays under a few KB even with thousands of tags.
func TagsFromModels(input []records.Tag) []TagOption {
	if len(input) == 0 {
		return []TagOption{}
	}
	out := make([]TagOption, 0, len(input))
	for _, t := range input {
		out = append(out, TagOption{
			ID:             t.ID,
			Name:           t.Name,
			NormalizedName: t.NormalizedName,
		})
	}
	return out
}

func normalizeOptionalText(value string) string {
	return strings.TrimSpace(value)
}

// SoldierSearchFromModel converts a domain-type value into its viewmodel projection.
func SoldierSearchFromModel(input models.SoldierSearch) PersonRecordSearch {
	return PersonRecordSearchFromModel(input)
}

// PersonRecordFormSuggestionsFromModel converts a domain-type value into its viewmodel projection.
func PersonRecordFormSuggestionsFromModel(input models.SoldierFormSuggestions) PersonRecordFormSuggestions {
	return PersonRecordFormSuggestions{
		RankIn:            append([]string(nil), input.RankIn...),
		RankOut:           append([]string(nil), input.RankOut...),
		Unit:              append([]string(nil), input.Unit...),
		Prefix:            append([]string(nil), input.Prefix...),
		Suffix:            append([]string(nil), input.Suffix...),
		PensionState:      append([]string(nil), input.PensionState...),
		BuriedIn:          append([]string(nil), input.BuriedIn...),
		ConfederateHome:   append([]string(nil), input.ConfederateHomeName...),
		SourceRecordType:  append([]string(nil), input.RecordType...),
		RelationshipLabel: append([]string(nil), input.RelationshipLabel...),
	}
}

// SoldierFormSuggestionsFromModel converts a domain-type value into its viewmodel projection.
func SoldierFormSuggestionsFromModel(input models.SoldierFormSuggestions) PersonRecordFormSuggestions {
	return PersonRecordFormSuggestionsFromModel(input)
}

// InitialSetupFormFromModel converts a domain-type value into its viewmodel projection.
func InitialSetupFormFromModel(input models.InitialSetupForm) InitialSetupForm {
	return InitialSetupForm(input)
}

// GoogleSettingsFromModel converts a domain-type value into its viewmodel projection.
func GoogleSettingsFromModel(input models.GoogleSettings) GoogleSettings {
	return GoogleSettings{
		ClientID:      input.ClientID,
		ClientSecret:  input.ClientSecret,
		CalendarID:    input.CalendarID,
		DriveFolderID: input.DriveFolderID,
		ManagedEventPreferences: CalendarEventPreferences{
			TitlePreset:         input.ManagedEventPreferences.TitlePreset,
			StartTime:           input.ManagedEventPreferences.StartTime,
			ReminderPrimary:     input.ManagedEventPreferences.ReminderPrimary,
			ReminderSecondary:   input.ManagedEventPreferences.ReminderSecondary,
			IncludeRecordID:     input.ManagedEventPreferences.IncludeRecordID,
			IncludeUnit:         input.ManagedEventPreferences.IncludeUnit,
			IncludeBuriedIn:     input.ManagedEventPreferences.IncludeBuriedIn,
			IncludeOriginalDate: input.ManagedEventPreferences.IncludeOriginalDate,
		},
	}
}

// GoogleStatusFromModel converts a domain-type value into its viewmodel projection.
func GoogleStatusFromModel(input models.GoogleStatus) GoogleStatus {
	return GoogleStatus{
		Settings:              GoogleSettingsFromModel(input.Settings),
		Connected:             input.Connected,
		HasClientID:           input.HasClientID,
		HasSecret:             input.HasSecret,
		HasToken:              input.HasToken,
		SharedClientAvailable: input.SharedClientAvailable,
		SharedClientSource:    input.SharedClientSource,
		UsingSharedClient:     input.UsingSharedClient,
		ManagedCalendarID:     input.ManagedCalendarID,
		TestCalendarID:        input.TestCalendarID,
		LastSyncedAt:          input.LastSyncedAt,
		OutOfSync:             input.OutOfSync,
		DriftAdded:            input.DriftAdded,
		DriftUpdated:          input.DriftUpdated,
		DriftRemoved:          input.DriftRemoved,
	}
}

// MergeReviewConflictFromModel converts a domain-type value into its viewmodel projection.
func MergeReviewConflictFromModel(input models.MergeReviewConflict) MergeReviewConflict {
	return MergeReviewConflict{
		ID:                input.ID,
		SessionID:         input.SessionID,
		ConflictType:      input.ConflictType,
		Reason:            input.Reason,
		LocalRecordID:     input.LocalRecordID,
		LocalDisplayID:    input.LocalDisplayID,
		IncomingDisplayID: input.SourceDisplayID,
		Resolution:        input.Resolution,
		CreatedAt:         input.CreatedAt,
		LocalRecord:       PersonRecordPtrFromModel(input.LocalSoldier),
		IncomingRecord:    PersonRecordFromModel(input.SourceSoldier),
	}
}

// MergeReviewConflictsFromModels converts a domain-type value into its viewmodel projection.
func MergeReviewConflictsFromModels(inputs []models.MergeReviewConflict) []MergeReviewConflict {
	items := make([]MergeReviewConflict, 0, len(inputs))
	for _, input := range inputs {
		items = append(items, MergeReviewConflictFromModel(input))
	}
	return items
}

// DuplicateAuditFindingSummaryFromDomain converts a domain-type value into its viewmodel projection.
func DuplicateAuditFindingSummaryFromDomain(input records.DuplicateAuditFindingSummary) DuplicateAuditFindingSummary {
	return DuplicateAuditFindingSummary{
		ID:                  input.ID,
		OtherPersonRecordID: input.OtherSoldierID,
		OtherDisplayID:      input.OtherDisplayID,
		OtherName:           input.OtherName,
		Reason:              input.Reason,
	}
}

// DuplicateAuditComparisonFieldFromDomain converts a domain-type value into its viewmodel projection.
func DuplicateAuditComparisonFieldFromDomain(input records.DuplicateAuditComparisonField) DuplicateAuditComparisonField {
	return DuplicateAuditComparisonField(input)
}

// DuplicateAuditSummaryFromDomain converts a domain-type value into its viewmodel projection.
func DuplicateAuditSummaryFromDomain(input records.DuplicateAuditSummary) DuplicateAuditSummary {
	return DuplicateAuditSummary(input)
}

// ReviewQueueEntriesFromDomain converts a domain-type value into its viewmodel projection.
func ReviewQueueEntriesFromDomain(personRecords []models.Soldier, findings map[int64][]records.DuplicateAuditFindingSummary) []ReviewQueueEntry {
	entries := make([]ReviewQueueEntry, 0, len(personRecords))
	for _, personRecord := range personRecords {
		duplicates := findings[personRecord.ID]
		mappedFindings := make([]DuplicateAuditFindingSummary, 0, len(duplicates))
		for _, finding := range duplicates {
			mappedFindings = append(mappedFindings, DuplicateAuditFindingSummaryFromDomain(finding))
		}
		entries = append(entries, ReviewQueueEntry{
			PersonRecord:      PersonRecordFromModel(personRecord),
			DuplicateFindings: mappedFindings,
		})
	}
	return entries
}

// DuplicateAuditComparisonFromDomain converts a domain-type value into its viewmodel projection.
func DuplicateAuditComparisonFromDomain(input records.DuplicateAuditComparison) DuplicateAuditComparison {
	fields := make([]DuplicateAuditComparisonField, 0, len(input.Fields))
	for _, field := range input.Fields {
		fields = append(fields, DuplicateAuditComparisonFieldFromDomain(field))
	}
	return DuplicateAuditComparison{
		FindingID:         input.FindingID,
		FindingType:       input.FindingType,
		PageTitle:         input.PageTitle,
		BackHref:          input.BackHref,
		BackLabel:         input.BackLabel,
		Reason:            input.Reason,
		Status:            input.Status,
		LeftPersonRecord:  PersonRecordFromModel(input.LeftSoldier),
		RightPersonRecord: PersonRecordFromModel(input.RightSoldier),
		Fields:            fields,
	}
}

// AnalyticsCountFromDomain converts a domain-type value into its viewmodel projection.
func AnalyticsCountFromDomain(input records.AnalyticsCount) AnalyticsCount {
	return AnalyticsCount(input)
}

// AnalyticsSnapshotFromDomain converts a domain-type value into its viewmodel projection.
func AnalyticsSnapshotFromDomain(input records.AnalyticsSnapshot) AnalyticsSnapshot {
	mapCounts := func(values []records.AnalyticsCount) []AnalyticsCount {
		items := make([]AnalyticsCount, 0, len(values))
		for _, value := range values {
			items = append(items, AnalyticsCountFromDomain(value))
		}
		return items
	}
	return AnalyticsSnapshot{
		PersonRecordTypes:       ArchiveCountsFromModel(input.RecordTypes),
		CemeteryDensity:         mapCounts(input.CemeteryDensity),
		ConfederateHomeStatus:   mapCounts(input.ConfederateHomeStatus),
		ConfederateHomeNames:    mapCounts(input.ConfederateHomeNames),
		PensionDistribution:     mapCounts(input.PensionDistribution),
		UnitRepresentation:      mapCounts(input.UnitRepresentation),
		BirthDecadeDistribution: mapCounts(input.BirthDecadeDistribution),
		DeathDecadeDistribution: mapCounts(input.DeathDecadeDistribution),
		DuplicateAudit:          DuplicateAuditSummaryFromDomain(input.DuplicateAudit),
	}
}

// ServiceTimelineFromDomain converts a domain-type value into its viewmodel projection.
func ServiceTimelineFromDomain(input records.ServiceTimeline) ServiceTimeline {
	events := make([]TimelineEvent, 0, len(input.Events))
	for _, event := range input.Events {
		events = append(events, TimelineEvent{
			Title:           event.Title,
			DateLabel:       event.DateLabel,
			Description:     event.Description,
			SourceLabel:     event.SourceLabel,
			Category:        event.Category,
			ConfidenceLabel: event.ConfidenceLabel,
			Approximate:     event.Approximate,
		})
	}
	return ServiceTimeline{
		SubjectPersonRecord:  PersonRecordFromModel(input.Central),
		TimelineEvents:       events,
		UndatedSourceRecords: SourceRecordsFromModels(input.UndatedRecords),
		StartLabel:           input.StartLabel,
		EndLabel:             input.EndLabel,
		ExactEventCount:      input.ExactEventCount,
		InferredEventCount:   input.InferredEventCount,
	}
}

// ResearchLogFromDomain converts a domain-type value into its viewmodel projection.
func ResearchLogFromDomain(input records.ResearchLog) ResearchLog {
	tasks := make([]ResearchTask, 0, len(input.Tasks))
	for _, task := range input.Tasks {
		tasks = append(tasks, ResearchTask{
			ID:             task.ID,
			PersonRecordID: task.SoldierID,
			Title:          task.Title,
			Notes:          task.Notes,
			EvidenceType:   task.EvidenceType,
			Status:         task.Status,
			CreatedAt:      task.CreatedAt,
			UpdatedAt:      task.UpdatedAt,
			ResolvedAt:     task.ResolvedAt,
		})
	}
	suggestions := make([]ResearchTaskSuggestion, 0, len(input.Suggestions))
	for _, suggestion := range input.Suggestions {
		suggestions = append(suggestions, ResearchTaskSuggestion(suggestion))
	}
	return ResearchLog{
		SubjectPersonRecord: PersonRecordFromModel(input.Central),
		Tasks:               tasks,
		Suggestions:         suggestions,
		OpenCount:           input.OpenCount,
		ResolvedCount:       input.ResolvedCount,
	}
}

// ResearchCollectionFromDomain converts a domain-type value into its viewmodel projection.
func ResearchCollectionFromDomain(input records.ResearchCollection) ResearchCollection {
	return ResearchCollection(input)
}

// ResearchCollectionHubFromDomain converts a domain-type value into its viewmodel projection.
func ResearchCollectionHubFromDomain(input records.ResearchCollectionHub) ResearchCollectionHub {
	collections := make([]ResearchCollection, 0, len(input.Collections))
	for _, collection := range input.Collections {
		collections = append(collections, ResearchCollectionFromDomain(collection))
	}
	return ResearchCollectionHub{
		CurrentPersonRecord: PersonRecordPtrFromModel(input.Current),
		Collections:         collections,
	}
}

// ResearchCollectionDetailFromDomain converts a domain-type value into its viewmodel projection.
func ResearchCollectionDetailFromDomain(input records.ResearchCollectionDetail) ResearchCollectionDetail {
	return ResearchCollectionDetail{
		Collection:          ResearchCollectionFromDomain(input.Collection),
		CurrentPersonRecord: PersonRecordPtrFromModel(input.Current),
		PersonRecords:       PersonRecordsFromModels(input.Members),
	}
}

// MergeReviewLedgerFromDomain converts a domain-type value into its viewmodel projection.
func MergeReviewLedgerFromDomain(input archive.SourceConflictLedger) MergeReviewLedger {
	entries := make([]MergeReviewLedgerEntry, 0, len(input.Entries))
	for _, entry := range input.Entries {
		entries = append(entries, MergeReviewLedgerEntry{
			ID:                  entry.ID,
			ConflictType:        entry.ConflictType,
			Reason:              entry.Reason,
			IncomingDisplayID:   entry.SourceDisplayID,
			Resolution:          entry.Resolution,
			CreatedAt:           entry.CreatedAt,
			ResolvedAt:          entry.ResolvedAt,
			LocalRecordSnapshot: PersonRecordFromModel(entry.LocalSnapshot),
			IncomingSnapshot:    PersonRecordFromModel(entry.SourceSnapshot),
			DifferenceFields:    append([]string(nil), entry.DifferenceFields...),
		})
	}
	return MergeReviewLedger{
		SubjectPersonRecord: PersonRecordFromModel(input.Central),
		Entries:             entries,
		OpenCount:           input.OpenCount,
		ResolvedCount:       input.ResolvedCount,
	}
}

// SourceConflictLedgerFromDomain converts a domain-type value into its viewmodel projection.
func SourceConflictLedgerFromDomain(input archive.SourceConflictLedger) MergeReviewLedger {
	return MergeReviewLedgerFromDomain(input)
}

// OrphanedImagesFromDomain converts a domain-type value into its viewmodel projection.
func OrphanedImagesFromDomain(inputs []archive.OrphanedImage) []OrphanedImage {
	items := make([]OrphanedImage, 0, len(inputs))
	for _, input := range inputs {
		items = append(items, OrphanedImage(input))
	}
	return items
}

// DataQualityScanResultFromDomain converts a domain-type value into its viewmodel projection.
func DataQualityScanResultFromDomain(input records.DataQualityScanResult) DataQualityScanResult {
	type grouped struct {
		group string
		items []DataQualityIssue
	}
	groups := make([]grouped, 0)
	indexByGroup := map[string]int{}
	for _, issue := range input.Issues {
		group := strings.TrimSpace(issue.Group)
		if group == "" {
			group = "Other"
		}
		mapped := DataQualityIssue{
			PersonRecordID: issue.SoldierID,
			DisplayID:      issue.DisplayID,
			Name:           issue.Name,
			EntryType:      issue.EntryType,
			Group:          group,
			Code:           issue.Code,
			Severity:       issue.Severity,
			Summary:        issue.Summary,
			Detail:         issue.Detail,
			// Issue #377 / #423: row provenance surfaced in
			// the scan results. ImportPath + RestoredAt carry
			// through from the records-layer type so the
			// entry_form.templ settings scan can render
			// "via <path>, restored at <ts>" next to each
			// flagged row's name.
			ImportPath: issue.ImportPath,
			RestoredAt: issue.RestoredAt,
		}
		groupIndex, ok := indexByGroup[group]
		if !ok {
			indexByGroup[group] = len(groups)
			groups = append(groups, grouped{group: group, items: []DataQualityIssue{mapped}})
			continue
		}
		groups[groupIndex].items = append(groups[groupIndex].items, mapped)
	}
	resultGroups := make([]DataQualityIssueGroup, 0, len(groups))
	for _, group := range groups {
		resultGroups = append(resultGroups, DataQualityIssueGroup{
			Group:  group.group,
			Count:  len(group.items),
			Issues: group.items,
		})
	}
	return DataQualityScanResult{
		Mode:           string(input.Mode),
		ScannedRecords: input.ScannedRecords,
		IssueCount:     len(input.Issues),
		Groups:         resultGroups,
	}
}

// DataQualityApplyResultFromDomain converts a domain-type value into its viewmodel projection.
func DataQualityApplyResultFromDomain(input records.DataQualityApplyResult) DataQualityApplyResult {
	return DataQualityApplyResult{
		Selected:       input.Selected,
		Flagged:        input.Flagged,
		AlreadyInQueue: input.AlreadyInQueue,
		NotFound:       input.NotFound,
	}
}

// CalendarFromModels converts a domain-type value into its viewmodel projection.
func CalendarFromModels(input map[int][]models.Soldier) map[int][]PersonRecord {
	calendar := make(map[int][]PersonRecord, len(input))
	for day, personRecords := range input {
		calendar[day] = PersonRecordsFromModels(personRecords)
	}
	return calendar
}

// (intentionally left blank — superseded by WithArchiveCounts on
// PersonRecordSearch; left as a marker so audit #98 progress is visible
// in the file.)

// WithArchiveCounts attaches archive-level counts to a model SoldierSearch
// so the search results template can render the first-run Setup card
// without an extra API call. Returns the same struct for chaining.
// Tracking: issue #98 from the 2026-06-24 audit.
func WithArchiveCounts(s models.SoldierSearch, counts models.ArchiveCounts) models.SoldierSearch {
	s.IsArchiveEmpty = counts.TotalRecords() == 0
	s.TotalRecordCount = counts.TotalRecords()
	return s
}

// ResearchPickerFromContext builds the picker viewmodel from the
// dd_person_ctx cookie + the supplied recents list. currentPerson is nil
// when no cookie (or the cookie points at a deleted Person); the picker
// then renders no "Continue" shortcut.
func ResearchPickerFromContext(currentPerson *models.Soldier, recents []models.Soldier, query string, results []models.Soldier) ResearchPickerView {
	view := ResearchPickerView{
		SearchQuery:   query,
		SearchResults: PersonRecordsFromModels(results),
	}
	if currentPerson != nil {
		rec := PersonRecordFromModel(*currentPerson)
		view.CurrentPerson = &rec
	}
	if len(recents) > 0 {
		view.RecentPersons = PersonRecordsFromModels(recents)
	}
	return view
}
