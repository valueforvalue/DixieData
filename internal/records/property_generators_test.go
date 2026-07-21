// property_generators_test.go — shared rapid.Generator[T]
// factories for the core domain types (issue #632). Models the
// pattern established in internal/dates/dates_property_test.go:
// rapid.Custom(func(t *rapid.T) T { ... }) with composable
// sub-generators. Every generator produces structurally valid
// values that pass the type's own validation/normalisation
// without panicking.
//
// Refs issue #632 Slice 1.

package records

import (
	"fmt"

	"github.com/valueforvalue/DixieData/internal/models"
	"pgregory.net/rapid"
)

// validDisplayID generates a canonical display ID like "DXD-00123"
// or "ABC-99999". The namespace is 3 uppercase letters; the
// sequence is a 5-digit zero-padded number.
func validDisplayID() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		ns := rapid.StringMatching(`[A-Z]{3}`).Draw(t, "ns")
		seq := rapid.IntRange(1, 99999).Draw(t, "seq")
		return fmt.Sprintf("%s-%05d", ns, seq)
	})
}

// validDateString generates a canonical-format date string like
// "01/15/1863". Month 1-12, day 1-28 (avoids month-length edge
// cases), year 1800-1899 (Civil War era).
func validDateString() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		m := rapid.IntRange(1, 12).Draw(t, "month")
		d := rapid.IntRange(1, 28).Draw(t, "day")
		y := rapid.IntRange(1800, 1899).Draw(t, "year")
		return fmt.Sprintf("%02d/%02d/%d", m, d, y)
	})
}

// validEntryType generates one of the canonical entry types:
// soldier, wife, widow, linked_person, event.
func validEntryType() *rapid.Generator[string] {
	return rapid.SampledFrom([]string{
		"soldier", "wife", "widow", "linked_person", "event",
	})
}

// validPensionState generates a pension state value that
// normalizes to itself or "N/A".
func validPensionState() *rapid.Generator[string] {
	return rapid.SampledFrom([]string{
		"N/A", "AL", "AR", "GA", "KY", "MS", "MO", "NC", "SC", "TN", "TX", "VA",
	})
}

// validConfederateHomeStatus generates a confederate home status
// known to the normalizer.
func validConfederateHomeStatus() *rapid.Generator[string] {
	return rapid.SampledFrom([]string{
		"N/A", "Inmate", "Staffer", "Trustee",
	})
}

// validSoldier generates a structurally valid Soldier with
// non-empty identity fields and canonical-format dates.
// Designed for normalization idempotence testing: the generator
// avoids degenerate empty strings in fields that would fail
// validation (DisplayID, entry type, dates).
func validSoldier() *rapid.Generator[models.Soldier] {
	return rapid.Custom(func(t *rapid.T) models.Soldier {
		return models.Soldier{
			DisplayID:             validDisplayID().Draw(t, "display_id"),
			EntryType:             validEntryType().Draw(t, "entry_type"),
			FirstName:             rapid.StringMatching(`[A-Z][a-z]{1,15}`).Draw(t, "first_name"),
			LastName:              rapid.StringMatching(`[A-Z][a-z]{1,20}`).Draw(t, "last_name"),
			Prefix:                rapid.SampledFrom([]string{"", "Dr.", "Rev.", "Capt."}).Draw(t, "prefix"),
			Suffix:                rapid.SampledFrom([]string{"", "Jr.", "Sr.", "III"}).Draw(t, "suffix"),
			MiddleName:            rapid.SampledFrom([]string{"", "A.", "B.", "C."}).Draw(t, "middle_name"),
			MaidenName:            rapid.SampledFrom([]string{"", "Smith", "Jones", "Brown"}).Draw(t, "maiden_name"),
			Rank:                  rapid.SampledFrom([]string{"", "Pvt.", "Cpl.", "Sgt.", "Lt.", "Capt."}).Draw(t, "rank"),
			RankIn:                rapid.SampledFrom([]string{"", "Pvt.", "Cpl.", "Sgt."}).Draw(t, "rank_in"),
			RankOut:               rapid.SampledFrom([]string{"", "Sgt.", "Lt.", "Capt."}).Draw(t, "rank_out"),
			Unit:                  rapid.SampledFrom([]string{"", "Co. A", "Co. B", "Co. K"}).Draw(t, "unit"),
			BirthDate:             validDateString().Draw(t, "birth_date"),
			DeathDate:             validDateString().Draw(t, "death_date"),
			BirthInfo:             rapid.StringMatching(`[A-Z][a-z ]{0,40}`).Draw(t, "birth_info"),
			PensionState:          validPensionState().Draw(t, "pension_state"),
			PensionID:             rapid.SampledFrom([]string{"", "P-12345", "P-67890"}).Draw(t, "pension_id"),
			ApplicationID:         rapid.SampledFrom([]string{"", "A-12345", "A-67890"}).Draw(t, "application_id"),
			ConfederateHomeStatus: validConfederateHomeStatus().Draw(t, "conf_home_status"),
			ConfederateHomeName:   rapid.SampledFrom([]string{"", "Home A", "Home B"}).Draw(t, "conf_home_name"),
			BuriedIn:              rapid.SampledFrom([]string{"", "Cemetery A", "Cemetery B"}).Draw(t, "buried_in"),
			Biography:             rapid.StringMatching(`[A-Za-z ,.]{0,100}`).Draw(t, "biography"),
			Notes:                 rapid.StringMatching(`[A-Za-z ,.]{0,60}`).Draw(t, "notes"),
			RelationshipLabel:     rapid.SampledFrom([]string{"", "spouse", "child"}).Draw(t, "relationship_label"),
			ReviewReason:          rapid.SampledFrom([]string{"", "missing data", "needs citation"}).Draw(t, "review_reason"),
			CreatedByVersion:      rapid.SampledFrom([]string{"", "v1.2.63"}).Draw(t, "created_by_version"),
			CreatedByImportPath:   rapid.SampledFrom([]string{"", "create_soldier", "memorial_json_import"}).Draw(t, "created_by_import_path"),
			Tags:                  rapid.SliceOf(rapid.StringMatching(`[a-z]{2,10}`)).Draw(t, "tags"),
		}
	})
}

// validRecord generates a structurally valid Source Record
// with a known RecordType and non-empty AppID.
func validRecord() *rapid.Generator[models.Record] {
	return rapid.Custom(func(t *rapid.T) models.Record {
		return models.Record{
			RecordType: rapid.SampledFrom([]string{
				"pension", "application", "service", "other",
			}).Draw(t, "record_type"),
			AppID: rapid.StringMatching(`[A-Z0-9]{3,10}`).Draw(t, "app_id"),
			Details: rapid.StringMatching(`[A-Za-z ,.]{0,80}`).Draw(t, "details"),
			SortOrder: int64(rapid.IntRange(0, 100).Draw(t, "sort_order")),
		}
	})
}

// validArticle generates a structurally valid Article with
// a markdown body. Designed for property tests that verify
// markdown rendering + sanitisation invariants.
func validArticle() *rapid.Generator[models.Article] {
	return rapid.Custom(func(t *rapid.T) models.Article {
		return models.Article{
			DisplayID: validDisplayID().Draw(t, "display_id"),
			Title:     rapid.StringMatching(`[A-Z][A-Za-z ]{3,40}`).Draw(t, "title"),
			Subtitle:  rapid.SampledFrom([]string{"", "Part One", "Chapter 2"}).Draw(t, "subtitle"),
			BodyMD:    rapid.StringMatching(`[A-Za-z0-9 \n\t.,;:!?'"\-_#*(){}\[\]<>/|@+=\x60]{0,500}`).Draw(t, "body_md"),
		}
	})
}
