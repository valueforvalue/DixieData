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
		Images:                ImagesFromModels(input.Images),
	}
}

func SoldierFromModel(input models.Soldier) PersonRecord {
	return PersonRecordFromModel(input)
}

func PersonRecordPtrFromModel(input *models.Soldier) *PersonRecord {
	if input == nil {
		return nil
	}
	mapped := PersonRecordFromModel(*input)
	return &mapped
}

func SoldierPtrFromModel(input *models.Soldier) *PersonRecord {
	return PersonRecordPtrFromModel(input)
}

func PersonRecordsFromModels(inputs []models.Soldier) []PersonRecord {
	items := make([]PersonRecord, 0, len(inputs))
	for _, input := range inputs {
		items = append(items, PersonRecordFromModel(input))
	}
	return items
}

func SoldiersFromModels(inputs []models.Soldier) []PersonRecord {
	return PersonRecordsFromModels(inputs)
}

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

func ExportRecordOptionsFromModels(inputs []models.Soldier) []ExportRecordOption {
	items := make([]ExportRecordOption, 0, len(inputs))
	for _, input := range inputs {
		items = append(items, ExportRecordOptionFromModel(input))
	}
	return items
}

func SourceRecordFromModel(input models.Record) SourceRecord {
	return SourceRecord{
		ID:                 input.ID,
		SyncID:             input.SyncID,
		PersonRecordID:     input.SoldierID,
		PersonRecordSyncID: input.SoldierSyncID,
		SourceRecordType:   input.RecordType,
		AppID:              input.AppID,
		Details:            input.Details,
	}
}

func RecordFromModel(input models.Record) SourceRecord {
	return SourceRecordFromModel(input)
}

func SourceRecordsFromModels(inputs []models.Record) []SourceRecord {
	items := make([]SourceRecord, 0, len(inputs))
	for _, input := range inputs {
		items = append(items, SourceRecordFromModel(input))
	}
	return items
}

func RecordsFromModels(inputs []models.Record) []SourceRecord {
	return SourceRecordsFromModels(inputs)
}

func ImageFromModel(input models.Image) Image {
	return Image{
		ID:                 input.ID,
		SyncID:             input.SyncID,
		PersonRecordID:     input.SoldierID,
		PersonRecordSyncID: input.SoldierSyncID,
		FileName:           input.FileName,
		FilePath:           input.FilePath,
		Caption:            input.Caption,
		IsPrimary:          input.IsPrimary,
		CompressedAt:       input.CompressedAt,
		OriginalBytes:      input.OriginalBytes,
		CompressedBytes:    input.CompressedBytes,
		ResolvedPath:       input.ResolvedPath,
	}
}

func ImagesFromModels(inputs []models.Image) []Image {
	items := make([]Image, 0, len(inputs))
	for _, input := range inputs {
		items = append(items, ImageFromModel(input))
	}
	return items
}

func ArchiveCountsFromModel(input models.ArchiveCounts) ArchiveCounts {
	return ArchiveCounts{
		SoldierCount:      input.TotalSoldiers,
		SpouseRecordCount: input.TotalWivesWidows,
		PersonRecordCount: input.TotalLinkedPeople,
	}
}

func QuoteFromModel(input models.Quote) Quote { return Quote(input) }

func CalendarDaySummaryFromDomain(input records.CalendarDaySummary) CalendarDaySummary {
	return CalendarDaySummary{
		AnniversaryCount: input.AnniversaryCount,
		EventCount:       input.EventCount,
		HolidayCount:     input.HolidayCount,
	}
}

func CalendarDaySummariesFromDomain(inputs map[int]records.CalendarDaySummary) map[int]CalendarDaySummary {
	items := make(map[int]CalendarDaySummary, len(inputs))
	for day, input := range inputs {
		items[day] = CalendarDaySummaryFromDomain(input)
	}
	return items
}

func CalendarItemFromModel(input models.CalendarItem) CalendarItem {
	return CalendarItem{
		ID:       input.ID,
		ItemType: input.ItemType,
		Title:    input.Title,
		Notes:    input.Notes,
	}
}

func CalendarItemsFromModels(inputs []models.CalendarItem) []CalendarItem {
	items := make([]CalendarItem, 0, len(inputs))
	for _, input := range inputs {
		items = append(items, CalendarItemFromModel(input))
	}
	return items
}

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

func QuotesFromModels(inputs []models.Quote) []Quote {
	items := make([]Quote, 0, len(inputs))
	for _, input := range inputs {
		items = append(items, QuoteFromModel(input))
	}
	return items
}

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
	}
}

func BrowseStateFromDomain(input records.BrowseRequest, total int) BrowseState {
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
	}
}

func normalizeOptionalText(value string) string {
	return strings.TrimSpace(value)
}

func SoldierSearchFromModel(input models.SoldierSearch) PersonRecordSearch {
	return PersonRecordSearchFromModel(input)
}

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

func SoldierFormSuggestionsFromModel(input models.SoldierFormSuggestions) PersonRecordFormSuggestions {
	return PersonRecordFormSuggestionsFromModel(input)
}

func InitialSetupFormFromModel(input models.InitialSetupForm) InitialSetupForm {
	return InitialSetupForm(input)
}

func ScrapedRelativeFromModel(input models.ScrapedRelative) ScrapedRelative {
	return ScrapedRelative(input)
}

func FindAGraveScrapeStateFromModel(input models.FindAGraveScrapeState) FindAGraveScrapeState {
	spouses := make([]ScrapedRelative, 0, len(input.Spouses))
	for _, spouse := range input.Spouses {
		spouses = append(spouses, ScrapedRelativeFromModel(spouse))
	}
	return FindAGraveScrapeState{
		Input:           input.Input,
		SourceLabel:     input.SourceLabel,
		ErrorMessage:    input.ErrorMessage,
		WarningLines:    append([]string(nil), input.WarningLines...),
		Spouses:         spouses,
		ConfidenceScore: input.ConfidenceScore,
	}
}

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

func MergeReviewConflictFromModel(input models.MergeReviewConflict) MergeReviewConflict {
	return MergeReviewConflict{
		ID:                input.ID,
		SessionID:         input.SessionID,
		ConflictType:      input.ConflictType,
		Reason:            input.Reason,
		LocalRecordID:     input.LocalSoldierID,
		LocalDisplayID:    input.LocalDisplayID,
		IncomingDisplayID: input.SourceDisplayID,
		Resolution:        input.Resolution,
		CreatedAt:         input.CreatedAt,
		LocalRecord:       PersonRecordPtrFromModel(input.LocalSoldier),
		IncomingRecord:    PersonRecordFromModel(input.SourceSoldier),
	}
}

func MergeReviewConflictsFromModels(inputs []models.MergeReviewConflict) []MergeReviewConflict {
	items := make([]MergeReviewConflict, 0, len(inputs))
	for _, input := range inputs {
		items = append(items, MergeReviewConflictFromModel(input))
	}
	return items
}

func DuplicateAuditFindingSummaryFromDomain(input records.DuplicateAuditFindingSummary) DuplicateAuditFindingSummary {
	return DuplicateAuditFindingSummary{
		ID:                  input.ID,
		OtherPersonRecordID: input.OtherSoldierID,
		OtherDisplayID:      input.OtherDisplayID,
		OtherName:           input.OtherName,
		Reason:              input.Reason,
	}
}

func DuplicateAuditComparisonFieldFromDomain(input records.DuplicateAuditComparisonField) DuplicateAuditComparisonField {
	return DuplicateAuditComparisonField(input)
}

func DuplicateAuditSummaryFromDomain(input records.DuplicateAuditSummary) DuplicateAuditSummary {
	return DuplicateAuditSummary(input)
}

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

func AnalyticsCountFromDomain(input records.AnalyticsCount) AnalyticsCount {
	return AnalyticsCount(input)
}

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

func UnitCamaraderieGraphFromDomain(input records.UnitCamaraderieGraph) UnitCamaraderieGraph {
	mapConnections := func(values []records.UnitCamaraderieConnection) []UnitCamaraderieConnection {
		items := make([]UnitCamaraderieConnection, 0, len(values))
		for _, value := range values {
			items = append(items, UnitCamaraderieConnection{
				Soldier:      PersonRecordFromModel(value.Soldier),
				Relation:     value.Relation,
				Strength:     value.Strength,
				StrengthText: value.StrengthText,
			})
		}
		return items
	}
	return UnitCamaraderieGraph{
		CentralPersonRecord: PersonRecordFromModel(input.Central),
		UnitLabel:           input.UnitLabel,
		RegimentLabel:       input.RegimentLabel,
		CompanyLabel:        input.CompanyLabel,
		SameUnit:            mapConnections(input.SameUnit),
		SameCompanyVariant:  mapConnections(input.SameCompanyVariant),
		SameRegiment:        mapConnections(input.SameRegiment),
	}
}

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

func ResearchPackFromDomain(input records.ResearchPack) ResearchPack {
	mapCounts := func(values []records.AnalyticsCount) []AnalyticsCount {
		items := make([]AnalyticsCount, 0, len(values))
		for _, value := range values {
			items = append(items, AnalyticsCountFromDomain(value))
		}
		return items
	}
	return ResearchPack{
		AnchorPersonRecord:   PersonRecordFromModel(input.Central),
		Scope:                input.Scope,
		PlaceLabel:           input.PlaceLabel,
		Description:          input.Description,
		RelatedPersonRecords: PersonRecordsFromModels(input.Related),
		TopUnits:             mapCounts(input.TopUnits),
		TopCemeteries:        mapCounts(input.TopCemeteries),
		OpenReviewCount:      input.OpenReviewCount,
	}
}

func ResearchCollectionFromDomain(input records.ResearchCollection) ResearchCollection {
	return ResearchCollection(input)
}

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

func ResearchCollectionDetailFromDomain(input records.ResearchCollectionDetail) ResearchCollectionDetail {
	return ResearchCollectionDetail{
		Collection:          ResearchCollectionFromDomain(input.Collection),
		CurrentPersonRecord: PersonRecordPtrFromModel(input.Current),
		PersonRecords:       PersonRecordsFromModels(input.Members),
	}
}

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

func SourceConflictLedgerFromDomain(input archive.SourceConflictLedger) MergeReviewLedger {
	return MergeReviewLedgerFromDomain(input)
}

func OrphanedImagesFromDomain(inputs []archive.OrphanedImage) []OrphanedImage {
	items := make([]OrphanedImage, 0, len(inputs))
	for _, input := range inputs {
		items = append(items, OrphanedImage(input))
	}
	return items
}

// CompressibleImagesFromDomain maps archive.CompressibleImage into the
// viewmodel layer for templates.
func CompressibleImagesFromDomain(inputs []archive.CompressibleImage) []CompressibleImage {
	items := make([]CompressibleImage, 0, len(inputs))
	for _, input := range inputs {
		items = append(items, CompressibleImage(input))
	}
	return items
}

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

func DataQualityApplyResultFromDomain(input records.DataQualityApplyResult) DataQualityApplyResult {
	return DataQualityApplyResult{
		Selected:       input.Selected,
		Flagged:        input.Flagged,
		AlreadyInQueue: input.AlreadyInQueue,
		NotFound:       input.NotFound,
	}
}

func CalendarFromModels(input map[int][]models.Soldier) map[int][]PersonRecord {
	calendar := make(map[int][]PersonRecord, len(input))
	for day, personRecords := range input {
		calendar[day] = PersonRecordsFromModels(personRecords)
	}
	return calendar
}
