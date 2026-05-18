package viewmodel

import (
	"github.com/valueforvalue/DixieData/internal/archive"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/records"
)

func SoldierFromModel(input models.Soldier) Soldier {
	return Soldier{
		ID:                    input.ID,
		DisplayID:             input.DisplayID,
		SyncID:                input.SyncID,
		EntryType:             input.EntryType,
		SpouseSoldierID:       input.SpouseSoldierID,
		SpouseName:            input.SpouseName,
		MaidenName:            input.MaidenName,
		IsGenerated:           input.IsGenerated,
		PensionID:             input.PensionID,
		ApplicationID:         input.ApplicationID,
		Prefix:                input.Prefix,
		FirstName:             input.FirstName,
		MiddleName:            input.MiddleName,
		LastName:              input.LastName,
		Suffix:                input.Suffix,
		Rank:                  input.Rank,
		RankIn:                input.RankIn,
		RankOut:               input.RankOut,
		Unit:                  input.Unit,
		PensionState:          input.PensionState,
		ConfederateHomeStatus: input.ConfederateHomeStatus,
		ConfederateHomeName:   input.ConfederateHomeName,
		DeathYear:             input.DeathYear,
		DeathMonth:            input.DeathMonth,
		DeathDay:              input.DeathDay,
		BirthDate:             input.BirthDate,
		DeathDate:             input.DeathDate,
		BirthInfo:             input.BirthInfo,
		BuriedIn:              input.BuriedIn,
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
		RecordCount:           input.RecordCount,
		ImageCount:            input.ImageCount,
		Records:               RecordsFromModels(input.Records),
		Images:                ImagesFromModels(input.Images),
	}
}

func SoldierPtrFromModel(input *models.Soldier) *Soldier {
	if input == nil {
		return nil
	}
	mapped := SoldierFromModel(*input)
	return &mapped
}

func SoldiersFromModels(inputs []models.Soldier) []Soldier {
	items := make([]Soldier, 0, len(inputs))
	for _, input := range inputs {
		items = append(items, SoldierFromModel(input))
	}
	return items
}

func RecordFromModel(input models.Record) Record {
	return Record(input)
}

func RecordsFromModels(inputs []models.Record) []Record {
	items := make([]Record, 0, len(inputs))
	for _, input := range inputs {
		items = append(items, RecordFromModel(input))
	}
	return items
}

func ImageFromModel(input models.Image) Image {
	return Image(input)
}

func ImagesFromModels(inputs []models.Image) []Image {
	items := make([]Image, 0, len(inputs))
	for _, input := range inputs {
		items = append(items, ImageFromModel(input))
	}
	return items
}

func ArchiveCountsFromModel(input models.ArchiveCounts) ArchiveCounts {
	return ArchiveCounts(input)
}

func QuoteFromModel(input models.Quote) Quote { return Quote(input) }

func QuotesFromModels(inputs []models.Quote) []Quote {
	items := make([]Quote, 0, len(inputs))
	for _, input := range inputs {
		items = append(items, QuoteFromModel(input))
	}
	return items
}

func SoldierSearchFromModel(input models.SoldierSearch) SoldierSearch { return SoldierSearch(input) }
func SoldierFormSuggestionsFromModel(input models.SoldierFormSuggestions) SoldierFormSuggestions {
	return SoldierFormSuggestions(input)
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
	return GoogleSettings(input)
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
	}
}

func MergeReviewConflictFromModel(input models.MergeReviewConflict) MergeReviewConflict {
	return MergeReviewConflict{
		ID:              input.ID,
		SessionID:       input.SessionID,
		ConflictType:    input.ConflictType,
		Reason:          input.Reason,
		LocalSoldierID:  input.LocalSoldierID,
		LocalDisplayID:  input.LocalDisplayID,
		SourceDisplayID: input.SourceDisplayID,
		Resolution:      input.Resolution,
		CreatedAt:       input.CreatedAt,
		LocalSoldier:    SoldierPtrFromModel(input.LocalSoldier),
		SourceSoldier:   SoldierFromModel(input.SourceSoldier),
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
	return DuplicateAuditFindingSummary(input)
}

func DuplicateAuditComparisonFieldFromDomain(input records.DuplicateAuditComparisonField) DuplicateAuditComparisonField {
	return DuplicateAuditComparisonField(input)
}

func DuplicateAuditSummaryFromDomain(input records.DuplicateAuditSummary) DuplicateAuditSummary {
	return DuplicateAuditSummary(input)
}

func ReviewQueueEntriesFromDomain(soldiers []models.Soldier, findings map[int64][]records.DuplicateAuditFindingSummary) []ReviewQueueEntry {
	entries := make([]ReviewQueueEntry, 0, len(soldiers))
	for _, soldier := range soldiers {
		duplicates := findings[soldier.ID]
		mappedFindings := make([]DuplicateAuditFindingSummary, 0, len(duplicates))
		for _, finding := range duplicates {
			mappedFindings = append(mappedFindings, DuplicateAuditFindingSummaryFromDomain(finding))
		}
		entries = append(entries, ReviewQueueEntry{Soldier: SoldierFromModel(soldier), DuplicateFindings: mappedFindings})
	}
	return entries
}

func DuplicateAuditComparisonFromDomain(input records.DuplicateAuditComparison) DuplicateAuditComparison {
	fields := make([]DuplicateAuditComparisonField, 0, len(input.Fields))
	for _, field := range input.Fields {
		fields = append(fields, DuplicateAuditComparisonFieldFromDomain(field))
	}
	return DuplicateAuditComparison{
		FindingID:    input.FindingID,
		FindingType:  input.FindingType,
		PageTitle:    input.PageTitle,
		BackHref:     input.BackHref,
		BackLabel:    input.BackLabel,
		Reason:       input.Reason,
		Status:       input.Status,
		LeftSoldier:  SoldierFromModel(input.LeftSoldier),
		RightSoldier: SoldierFromModel(input.RightSoldier),
		Fields:       fields,
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
		RecordTypes:             ArchiveCountsFromModel(input.RecordTypes),
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
			items = append(items, UnitCamaraderieConnection{Soldier: SoldierFromModel(value.Soldier), Relation: value.Relation, Strength: value.Strength, StrengthText: value.StrengthText})
		}
		return items
	}
	return UnitCamaraderieGraph{
		Central:            SoldierFromModel(input.Central),
		UnitLabel:          input.UnitLabel,
		RegimentLabel:      input.RegimentLabel,
		CompanyLabel:       input.CompanyLabel,
		SameUnit:           mapConnections(input.SameUnit),
		SameCompanyVariant: mapConnections(input.SameCompanyVariant),
		SameRegiment:       mapConnections(input.SameRegiment),
	}
}

func ServiceTimelineFromDomain(input records.ServiceTimeline) ServiceTimeline {
	events := make([]ServiceTimelineEvent, 0, len(input.Events))
	for _, event := range input.Events {
		events = append(events, ServiceTimelineEvent{
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
		Central:            SoldierFromModel(input.Central),
		Events:             events,
		UndatedRecords:     RecordsFromModels(input.UndatedRecords),
		StartLabel:         input.StartLabel,
		EndLabel:           input.EndLabel,
		ExactEventCount:    input.ExactEventCount,
		InferredEventCount: input.InferredEventCount,
	}
}

func ResearchLogFromDomain(input records.ResearchLog) ResearchLog {
	tasks := make([]ResearchTask, 0, len(input.Tasks))
	for _, task := range input.Tasks {
		tasks = append(tasks, ResearchTask(task))
	}
	suggestions := make([]ResearchTaskSuggestion, 0, len(input.Suggestions))
	for _, suggestion := range input.Suggestions {
		suggestions = append(suggestions, ResearchTaskSuggestion(suggestion))
	}
	return ResearchLog{
		Central:       SoldierFromModel(input.Central),
		Tasks:         tasks,
		Suggestions:   suggestions,
		OpenCount:     input.OpenCount,
		ResolvedCount: input.ResolvedCount,
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
		Central:         SoldierFromModel(input.Central),
		Scope:           input.Scope,
		PlaceLabel:      input.PlaceLabel,
		Description:     input.Description,
		Related:         SoldiersFromModels(input.Related),
		TopUnits:        mapCounts(input.TopUnits),
		TopCemeteries:   mapCounts(input.TopCemeteries),
		OpenReviewCount: input.OpenReviewCount,
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
	return ResearchCollectionHub{Current: SoldierPtrFromModel(input.Current), Collections: collections}
}

func ResearchCollectionDetailFromDomain(input records.ResearchCollectionDetail) ResearchCollectionDetail {
	return ResearchCollectionDetail{
		Collection: ResearchCollectionFromDomain(input.Collection),
		Current:    SoldierPtrFromModel(input.Current),
		Members:    SoldiersFromModels(input.Members),
	}
}

func SourceConflictLedgerFromDomain(input archive.SourceConflictLedger) SourceConflictLedger {
	entries := make([]SourceConflictLedgerEntry, 0, len(input.Entries))
	for _, entry := range input.Entries {
		entries = append(entries, SourceConflictLedgerEntry{
			ID:               entry.ID,
			ConflictType:     entry.ConflictType,
			Reason:           entry.Reason,
			SourceDisplayID:  entry.SourceDisplayID,
			Resolution:       entry.Resolution,
			CreatedAt:        entry.CreatedAt,
			ResolvedAt:       entry.ResolvedAt,
			LocalSnapshot:    SoldierFromModel(entry.LocalSnapshot),
			SourceSnapshot:   SoldierFromModel(entry.SourceSnapshot),
			DifferenceFields: append([]string(nil), entry.DifferenceFields...),
		})
	}
	return SourceConflictLedger{
		Central:       SoldierFromModel(input.Central),
		Entries:       entries,
		OpenCount:     input.OpenCount,
		ResolvedCount: input.ResolvedCount,
	}
}

func OrphanedImagesFromDomain(inputs []archive.OrphanedImage) []OrphanedImage {
	items := make([]OrphanedImage, 0, len(inputs))
	for _, input := range inputs {
		items = append(items, OrphanedImage(input))
	}
	return items
}

func CalendarFromModels(input map[int][]models.Soldier) map[int][]Soldier {
	calendar := make(map[int][]Soldier, len(input))
	for day, soldiers := range input {
		calendar[day] = SoldiersFromModels(soldiers)
	}
	return calendar
}
