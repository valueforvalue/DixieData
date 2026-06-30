package records

import (
	"fmt"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
)

func newTestDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestSoldierService_CreateWithGeneratedID(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	s, err := svc.Create(models.Soldier{
		FirstName: "Robert",
		LastName:  "Lee",
		Rank:      "General",
		Unit:      "Army of Northern Virginia",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if s.DisplayID != "DXD-00001" {
		t.Errorf("expected DXD-00001, got %s", s.DisplayID)
	}
	if !s.IsGenerated {
		t.Error("expected IsGenerated=true")
	}
	if s.SyncID == "" {
		t.Fatal("expected SyncID")
	}
}

func TestSoldierService_CreateSetsAuditFields(t *testing.T) {
	d := newTestDB(t)
	configureExportIdentity(t, d)
	svc := NewSoldierService(d)

	s, err := svc.Create(models.Soldier{FirstName: "Robert", LastName: "Lee"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if s.AddedBy != "S. Carter" {
		t.Fatalf("AddedBy = %q", s.AddedBy)
	}
	if s.LastEditedBy != "S. Carter" {
		t.Fatalf("LastEditedBy = %q", s.LastEditedBy)
	}
	if s.LastEditedFields != "created" {
		t.Fatalf("LastEditedFields = %q", s.LastEditedFields)
	}
	if s.LastEditedAt == "" {
		t.Fatal("expected LastEditedAt")
	}
}

func TestSoldierService_CreateWithPensionID(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	s, err := svc.Create(models.Soldier{
		DisplayID: "PENSION-9999",
		FirstName: "Stonewall",
		LastName:  "Jackson",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if s.DisplayID != "PENSION-9999" {
		t.Errorf("expected PENSION-9999, got %s", s.DisplayID)
	}
	if s.IsGenerated {
		t.Error("expected IsGenerated=false for explicit pension ID")
	}
}

func TestSoldierService_CreateWithExplicitDXDIDMarksGenerated(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	s, err := svc.Create(models.Soldier{
		DisplayID: "DXD-00001",
		FirstName: "John",
		LastName:  "Mosby",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if !s.IsGenerated {
		t.Fatal("expected IsGenerated=true for DXD IDs")
	}
	if s.DisplayID != "DXD-00001" {
		t.Fatalf("DisplayID = %q", s.DisplayID)
	}
}

func TestSoldierService_GetByID(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	created, err := svc.Create(models.Soldier{
		FirstName: "James",
		LastName:  "Longstreet",
		Rank:      "Lieutenant General",
		Unit:      "First Corps",
		DeathYear: 1904, DeathMonth: 1, DeathDay: 2,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := svc.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.FirstName != "James" || got.LastName != "Longstreet" {
		t.Errorf("got %s %s, want James Longstreet", got.FirstName, got.LastName)
	}
	if got.DeathMonth != 1 || got.DeathDay != 2 {
		t.Errorf("got death %d/%d, want 1/2", got.DeathMonth, got.DeathDay)
	}
}

func TestSoldierService_PersistsNewIdentityFields(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	created, err := svc.Create(models.Soldier{
		PensionID:             "P12345",
		ApplicationID:         "A12345",
		Prefix:                "Capt.",
		ShowPrefixBeforeName:  true,
		FirstName:             "John",
		MiddleName:            "Bell",
		LastName:              "Hood",
		Suffix:                "Jr.",
		RankIn:                "Colonel",
		RankOut:               "Lieutenant General",
		PensionState:          "Texas",
		ConfederateHomeStatus: "Staffer",
		ConfederateHomeName:   "Texas Confederate Home",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := svc.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Prefix != "Capt." || !got.ShowPrefixBeforeName || got.MiddleName != "Bell" || got.Suffix != "Jr." || got.RankIn != "Colonel" || got.RankOut != "Lieutenant General" || got.PensionState != "Texas" || got.PensionID != "P12345" || got.ApplicationID != "A12345" || got.ConfederateHomeStatus != "Staffer" || got.ConfederateHomeName != "Texas Confederate Home" {
		t.Fatalf("unexpected new fields: %#v", got)
	}
	if got.Rank != "Lieutenant General" {
		t.Fatalf("Rank = %q, want rank_out mirror", got.Rank)
	}
	if got.SyncID == "" || got.UpdatedAt == "" {
		t.Fatalf("expected identity/date fields, got %#v", got)
	}
}

func TestSoldierService_GetByIDHandlesNullNewFields(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	result, err := d.Conn().Exec(`INSERT INTO soldiers (display_id, is_generated, first_name, last_name, rank, unit, death_year, death_month, death_day, birth_info, buried_in, notes) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"CSA-NULLTEST", false, "Null", "Case", "Sergeant", "Test Unit", 0, 0, 0, "", "", "",
	)
	if err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}

	got, err := svc.GetByID(id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Prefix != "" || got.ShowPrefixBeforeName || got.MiddleName != "" || got.Suffix != "" || got.RankIn != "" || got.RankOut != "" || got.PensionState != "N/A" || got.PensionID != "" || got.ApplicationID != "" || got.ConfederateHomeStatus != "N/A" || got.ConfederateHomeName != "" {
		t.Fatalf("expected empty strings for NULL fields, got %#v", got)
	}
}

func TestSoldierService_NormalizesConfederateHomeFields(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	created, err := svc.Create(models.Soldier{
		FirstName:             "James",
		LastName:              "Buckner",
		ConfederateHomeStatus: "none",
		ConfederateHomeName:   "Should Clear",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if created.ConfederateHomeStatus != "N/A" || created.ConfederateHomeName != "" {
		t.Fatalf("created = %#v", created)
	}
}

func TestSoldierService_NormalizesPensionStateNA(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	created, err := svc.Create(models.Soldier{
		FirstName:    "James",
		LastName:     "Buckner",
		PensionState: "None",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if created.PensionState != "N/A" {
		t.Fatalf("created pension state = %q, want %q", created.PensionState, "N/A")
	}
}

func TestSoldierService_FormSuggestions(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	first, err := svc.Create(models.Soldier{
		Prefix:       "Capt.",
		FirstName:    "Albert",
		LastName:     "Smith",
		Suffix:       "Jr.",
		RankIn:       "Private",
		RankOut:      "Corporal",
		Unit:         "Co. A, 1st Texas Infantry",
		PensionState: "Texas",
		BuriedIn:     "Oakwood Cemetery",
		Records: []models.Record{
			{RecordType: "Pension"},
			{RecordType: "Muster Roll"},
		},
	})
	if err != nil {
		t.Fatalf("Create first: %v", err)
	}
	if _, err := svc.Create(models.Soldier{
		FirstName:    "Benjamin",
		LastName:     "Jones",
		RankIn:       "Private",
		Rank:         "Sergeant",
		Unit:         "Co. A, 1st Texas Infantry",
		PensionState: "Texas",
		BuriedIn:     "Oakwood Cemetery",
		Records: []models.Record{
			{RecordType: "Pension"},
		},
	}); err != nil {
		t.Fatalf("Create second: %v", err)
	}

	suggestions, err := svc.FormSuggestions()
	if err != nil {
		t.Fatalf("FormSuggestions: %v", err)
	}
	if len(suggestions.Prefix) != 1 || suggestions.Prefix[0] != "Capt." {
		t.Fatalf("prefix suggestions = %#v", suggestions.Prefix)
	}
	if len(suggestions.Suffix) != 1 || suggestions.Suffix[0] != "Jr." {
		t.Fatalf("suffix suggestions = %#v", suggestions.Suffix)
	}
	if len(suggestions.RankIn) != 1 || suggestions.RankIn[0] != "Private" {
		t.Fatalf("rank_in suggestions = %#v", suggestions.RankIn)
	}
	if len(suggestions.RankOut) != 2 || suggestions.RankOut[0] != "Corporal" || suggestions.RankOut[1] != "Sergeant" {
		t.Fatalf("rank_out suggestions = %#v", suggestions.RankOut)
	}
	if len(suggestions.RecordType) != 2 || suggestions.RecordType[0] != "Muster Roll" || suggestions.RecordType[1] != "Pension" {
		t.Fatalf("record_type suggestions = %#v", suggestions.RecordType)
	}

	updated := *first
	updated.Unit = "Co. B, 4th Texas Cavalry"
	if err := svc.Update(updated); err != nil {
		t.Fatalf("Update: %v", err)
	}

	afterUpdate, err := svc.FormSuggestions()
	if err != nil {
		t.Fatalf("FormSuggestions after update: %v", err)
	}
	if len(afterUpdate.Unit) != 2 || afterUpdate.Unit[1] != "Co. B, 4th Texas Cavalry" {
		t.Fatalf("unit suggestions after update = %#v", afterUpdate.Unit)
	}
}

func TestSoldierService_PersistsBuriedInAndRecords(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	created, err := svc.Create(models.Soldier{
		FirstName: "James",
		LastName:  "Archer",
		BuriedIn:  "Oakwood Cemetery",
		Records: []models.Record{
			{RecordType: "Service Record", AppID: "APP-1", Details: "Filed with the adjutant."},
			{RecordType: "Burial Ledger", AppID: "APP-2", Details: "Lists grave location."},
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := svc.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.BuriedIn != "Oakwood Cemetery" {
		t.Fatalf("BuriedIn = %q", got.BuriedIn)
	}
	if len(got.Records) != 2 {
		t.Fatalf("records len = %d", len(got.Records))
	}
	for _, record := range got.Records {
		if record.SyncID == "" || record.SoldierSyncID != got.SyncID {
			t.Fatalf("record identity mismatch: %#v soldier=%#v", record, got)
		}
	}
}

func TestSoldierService_PersistsBiography(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	created, err := svc.Create(models.Soldier{
		FirstName:          "James",
		LastName:           "Archer",
		Biography:          " Served with distinction and later returned to Virginia. ",
		PDFExcerptOverride: " Tight export excerpt. ",
		Notes:              "Internal research trail.",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := svc.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Biography != "Served with distinction and later returned to Virginia." {
		t.Fatalf("Biography = %q", got.Biography)
	}
	if got.PDFExcerptOverride != "Tight export excerpt." {
		t.Fatalf("PDFExcerptOverride = %q", got.PDFExcerptOverride)
	}
	if got.Notes != "Internal research trail." {
		t.Fatalf("Notes = %q", got.Notes)
	}
}

func TestSoldierService_AddImagePersistsIdentityFields(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	created, err := svc.Create(models.Soldier{
		FirstName: "Thomas",
		LastName:  "Green",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.AddImage(created.ID, "portrait.png", `images\green\portrait.png`, "Portrait"); err != nil {
		t.Fatalf("AddImage: %v", err)
	}

	got, err := svc.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if len(got.Images) != 1 {
		t.Fatalf("images len = %d", len(got.Images))
	}
	if got.Images[0].SyncID == "" || got.Images[0].SoldierSyncID != got.SyncID {
		t.Fatalf("image identity mismatch: %#v soldier=%#v", got.Images[0], got)
	}
	if !got.Images[0].IsPrimary {
		t.Fatalf("first imported image should be primary: %#v", got.Images[0])
	}
}

func TestNormalizeDisplayIDPreservesCanonicalDisplayIDs(t *testing.T) {
	for _, test := range []struct {
		name      string
		displayID string
		want      string
	}{
		{name: "current canonical", displayID: "STC38-00020", want: "STC38-00020"},
		{name: "legacy dxd", displayID: "DXD-00019", want: "DXD-00019"},
		{name: "legacy prefixed dxd", displayID: "TDM65-DXD-00019", want: "DXD-00019"},
		{name: "already recursively wrapped", displayID: "JCM87-TDM65-DXD-00019", want: "DXD-00019"},
		{name: "manual prefixed value stays put", displayID: "PENSION-9999", want: "PENSION-9999"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := normalizeDisplayID(test.displayID, "STC38"); got != test.want {
				t.Fatalf("normalizeDisplayID(%q) = %q, want %q", test.displayID, got, test.want)
			}
		})
	}
}

func TestSoldierService_Update(t *testing.T) {
	d := newTestDB(t)
	configureExportIdentity(t, d)
	svc := NewSoldierService(d)

	created, err := svc.Create(models.Soldier{FirstName: "Jubal", LastName: "Early"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	created.Notes = "Updated note"
	created.Prefix = "Gen."
	created.MiddleName = "A."
	created.Suffix = "Sr."
	created.RankIn = "Private"
	created.RankOut = "Major"
	created.PensionState = "Georgia"
	if err := svc.Update(*created); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := svc.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID after update: %v", err)
	}
	if got.Prefix != "Gen." || got.Suffix != "Sr." {
		t.Fatalf("expected prefix/suffix after update, got %#v", got)
	}
	if got.Notes != "Updated note" {
		t.Errorf("got notes %q, want 'Updated note'", got.Notes)
	}
	if got.MiddleName != "A." || got.RankIn != "Private" || got.RankOut != "Major" || got.PensionState != "Georgia" {
		t.Fatalf("updated fields missing: %#v", got)
	}
	if got.LastEditedBy != "S. Carter" {
		t.Fatalf("LastEditedBy = %q", got.LastEditedBy)
	}
	if got.LastEditedAt == "" {
		t.Fatal("expected LastEditedAt after update")
	}
	for _, field := range []string{
		`Prefix changed from "N/A" to "Gen.".`,
		`Middle Name changed from "N/A" to "A.".`,
		`Suffix changed from "N/A" to "Sr.".`,
		`Rank In changed from "N/A" to "Private".`,
		`Rank Out changed from "N/A" to "Major".`,
		`Pension State changed from "N/A" to "Georgia".`,
		`Notes changed from "N/A" to "Updated note".`,
	} {
		if !strings.Contains(got.LastEditedFields, field) {
			t.Fatalf("LastEditedFields = %q, missing %s", got.LastEditedFields, field)
		}
	}
}

func TestSoldierService_CreateWifeEntry(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	soldier, err := svc.Create(models.Soldier{FirstName: "John", LastName: "Taylor", RankOut: "Captain"})
	if err != nil {
		t.Fatalf("Create soldier: %v", err)
	}

	spouse, err := svc.Create(models.Soldier{
		EntryType:       "wife",
		SpouseSoldierID: soldier.ID,
		FirstName:       "Martha",
		LastName:        "Taylor",
		MaidenName:      "Cole",
	})
	if err != nil {
		t.Fatalf("Create spouse: %v", err)
	}

	got, err := svc.GetByID(spouse.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.EntryType != "wife" || got.SpouseSoldierID != soldier.ID || got.MaidenName != "Cole" {
		t.Fatalf("unexpected spouse record: %#v", got)
	}
	if got.SpouseName != "John Taylor" {
		t.Fatalf("SpouseName = %q", got.SpouseName)
	}
	if got.Rank != "" || got.Unit != "" || got.PensionID != "" {
		t.Fatalf("unexpected soldier-only data on spouse record: %#v", got)
	}
}

func TestSoldierService_CreatePersonRecordEntry(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	soldier, err := svc.Create(models.Soldier{FirstName: "John", LastName: "Taylor", RankOut: "Captain"})
	if err != nil {
		t.Fatalf("Create soldier: %v", err)
	}

	linked, err := svc.Create(models.Soldier{
		EntryType:         "linked_person",
		SpouseSoldierID:   soldier.ID,
		RelationshipLabel: "Brother",
		FirstName:         "Samuel",
		LastName:          "Taylor",
	})
	if err != nil {
		t.Fatalf("Create person record: %v", err)
	}

	got, err := svc.GetByID(linked.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.EntryType != "linked_person" || got.SpouseSoldierID != soldier.ID || got.RelationshipLabel != "Brother" {
		t.Fatalf("unexpected person record: %#v", got)
	}
	if got.SpouseName != "John Taylor" {
		t.Fatalf("SpouseName = %q", got.SpouseName)
	}
	if got.MaidenName != "" || got.Rank != "" || got.Unit != "" {
		t.Fatalf("unexpected spouse or soldier-only data on person record: %#v", got)
	}
}

func TestSoldierService_RejectsSpouseLinkedToNonSoldier(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	soldier, err := svc.Create(models.Soldier{FirstName: "John", LastName: "Taylor"})
	if err != nil {
		t.Fatalf("Create soldier: %v", err)
	}
	wife, err := svc.Create(models.Soldier{
		EntryType:       "wife",
		SpouseSoldierID: soldier.ID,
		FirstName:       "Anna",
		LastName:        "Taylor",
	})
	if err != nil {
		t.Fatalf("Create wife: %v", err)
	}

	_, err = svc.Create(models.Soldier{
		EntryType:       "widow",
		SpouseSoldierID: wife.ID,
		FirstName:       "Clara",
		LastName:        "Taylor",
	})
	if err == nil || !strings.Contains(err.Error(), "soldier record") {
		t.Fatalf("expected spouse validation error, got %v", err)
	}
}

func TestSoldierService_MarriageCandidatesExcludeSpouseEntries(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	soldier, err := svc.Create(models.Soldier{FirstName: "Thomas", LastName: "Hill"})
	if err != nil {
		t.Fatalf("Create soldier: %v", err)
	}
	if _, err := svc.Create(models.Soldier{
		EntryType:       "widow",
		SpouseSoldierID: soldier.ID,
		FirstName:       "Sarah",
		LastName:        "Hill",
	}); err != nil {
		t.Fatalf("Create widow: %v", err)
	}

	candidates, err := svc.MarriageCandidates()
	if err != nil {
		t.Fatalf("MarriageCandidates: %v", err)
	}
	if len(candidates) != 1 || candidates[0].ID != soldier.ID {
		t.Fatalf("unexpected candidates: %#v", candidates)
	}
}

func TestSoldierService_WidowEntryKeepsOwnPensionIdentifiers(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	soldier, err := svc.Create(models.Soldier{FirstName: "John", LastName: "Taylor"})
	if err != nil {
		t.Fatalf("Create soldier: %v", err)
	}

	widow, err := svc.Create(models.Soldier{
		EntryType:       "widow",
		SpouseSoldierID: soldier.ID,
		FirstName:       "Mary",
		LastName:        "Taylor",
		MaidenName:      "Cole",
		PensionID:       "WP-42",
		ApplicationID:   "WA-42",
	})
	if err != nil {
		t.Fatalf("Create widow: %v", err)
	}

	got, err := svc.GetByID(widow.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.PensionID != "WP-42" || got.ApplicationID != "WA-42" {
		t.Fatalf("widow pension identifiers not persisted: %#v", got)
	}
}

func TestSoldierService_UpdateReplacesRecords(t *testing.T) {
	d := newTestDB(t)
	configureExportIdentity(t, d)
	svc := NewSoldierService(d)

	created, err := svc.Create(models.Soldier{
		FirstName: "Jubal",
		LastName:  "Early",
		Records:   []models.Record{{RecordType: "Roster", AppID: "APP-1", Details: "Old details"}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	created.Records = []models.Record{{RecordType: "Parole", AppID: "APP-2", Details: "Updated details"}}
	created.BuriedIn = "Lynchburg Cemetery"
	if err := svc.Update(*created); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := svc.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID after update: %v", err)
	}
	if got.BuriedIn != "Lynchburg Cemetery" {
		t.Fatalf("BuriedIn = %q", got.BuriedIn)
	}
	if len(got.Records) != 1 || got.Records[0].RecordType != "Parole" {
		t.Fatalf("records = %#v", got.Records)
	}
	if !strings.Contains(got.LastEditedFields, "Records updated.") {
		t.Fatalf("LastEditedFields = %q", got.LastEditedFields)
	}
	if got.LastEditedAt == "" {
		t.Fatal("expected LastEditedAt after record update")
	}
}

func TestSoldierService_Delete(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	created, err := svc.Create(models.Soldier{FirstName: "P.G.T.", LastName: "Beauregard"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := svc.Delete(created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = svc.GetByID(created.ID)
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestSoldierService_DeleteImages(t *testing.T) {
	d := newTestDB(t)
	configureExportIdentity(t, d)
	svc := NewSoldierService(d)

	created, err := svc.Create(models.Soldier{FirstName: "John", LastName: "Mosby"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.AddImage(created.ID, "front.png", `images\mosby\front.png`, "Front"); err != nil {
		t.Fatalf("AddImage front: %v", err)
	}
	if err := svc.AddImage(created.ID, "back.png", `images\mosby\back.png`, "Back"); err != nil {
		t.Fatalf("AddImage back: %v", err)
	}

	got, err := svc.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if len(got.Images) != 2 {
		t.Fatalf("images len = %d, want 2", len(got.Images))
	}
	if !got.Images[0].IsPrimary {
		t.Fatalf("expected first image to be primary before delete: %#v", got.Images)
	}

	if err := svc.DeleteImages(created.ID, []int64{got.Images[0].ID}); err != nil {
		t.Fatalf("DeleteImages: %v", err)
	}

	updated, err := svc.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID after delete images: %v", err)
	}
	if len(updated.Images) != 1 {
		t.Fatalf("images len = %d, want 1", len(updated.Images))
	}
	if updated.Images[0].FileName != "back.png" && updated.Images[0].FileName != "front.png" {
		t.Fatalf("remaining image = %#v", updated.Images[0])
	}
	if !updated.Images[0].IsPrimary {
		t.Fatalf("remaining image should be promoted to primary: %#v", updated.Images[0])
	}
	if updated.LastEditedBy != "S. Carter" || updated.LastEditedFields != "Images updated." {
		t.Fatalf("unexpected image audit trail: %#v", updated)
	}
	if updated.LastEditedAt == "" {
		t.Fatal("expected LastEditedAt after image delete")
	}
}

func TestSoldierService_GetImageByID(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	created, err := svc.Create(models.Soldier{FirstName: "Turner", LastName: "Ashby"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.AddImage(created.ID, "portrait.png", `images\ashby\portrait.png`, "Portrait"); err != nil {
		t.Fatalf("AddImage: %v", err)
	}

	soldier, err := svc.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	image, err := svc.GetImageByID(soldier.Images[0].ID)
	if err != nil {
		t.Fatalf("GetImageByID: %v", err)
	}
	if image.FileName != "portrait.png" {
		t.Fatalf("FileName = %q", image.FileName)
	}
	if image.Caption != "Portrait" {
		t.Fatalf("Caption = %q", image.Caption)
	}
	if !image.IsPrimary {
		t.Fatalf("expected image to be primary: %#v", image)
	}
}

func TestSoldierService_AddImagePreservesEmptyCaption(t *testing.T) {
	d := newTestDB(t)
	configureExportIdentity(t, d)
	svc := NewSoldierService(d)

	created, err := svc.Create(models.Soldier{FirstName: "Thomas", LastName: "Silence"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.AddImage(created.ID, "portrait.png", `images\silence\portrait.png`, ""); err != nil {
		t.Fatalf("AddImage: %v", err)
	}

	soldier, err := svc.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	image, err := svc.GetImageByID(soldier.Images[0].ID)
	if err != nil {
		t.Fatalf("GetImageByID: %v", err)
	}
	if image.Caption != "" {
		t.Fatalf("Caption = %q, want empty", image.Caption)
	}
}

func TestSoldierService_SetPrimaryImage(t *testing.T) {
	d := newTestDB(t)
	configureExportIdentity(t, d)
	svc := NewSoldierService(d)

	created, err := svc.Create(models.Soldier{FirstName: "George", LastName: "Pickett"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.AddImage(created.ID, "front.png", `images\pickett\front.png`, "Front"); err != nil {
		t.Fatalf("AddImage front: %v", err)
	}
	if err := svc.AddImage(created.ID, "profile.png", `images\pickett\profile.png`, "Profile"); err != nil {
		t.Fatalf("AddImage profile: %v", err)
	}

	soldier, err := svc.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if len(soldier.Images) != 2 {
		t.Fatalf("images len = %d", len(soldier.Images))
	}
	target := soldier.Images[1].ID
	if err := svc.SetPrimaryImage(created.ID, target); err != nil {
		t.Fatalf("SetPrimaryImage: %v", err)
	}

	updated, err := svc.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID after SetPrimaryImage: %v", err)
	}
	if len(updated.Images) != 2 {
		t.Fatalf("images len = %d", len(updated.Images))
	}
	if updated.Images[0].ID != target || !updated.Images[0].IsPrimary || updated.Images[1].IsPrimary {
		t.Fatalf("primary image not updated: %#v", updated.Images)
	}
	if updated.LastEditedFields != "Primary image updated." {
		t.Fatalf("unexpected image audit trail: %#v", updated)
	}
}

func TestSoldierService_List(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	var firstID int64
	for i := 0; i < 5; i++ {
		soldier := models.Soldier{FirstName: "Soldier", LastName: "Test"}
		if i == 0 {
			soldier.FirstName = "Aaron"
			soldier.Records = []models.Record{{RecordType: "Pension", AppID: "A-1", Details: "Filed in 1901"}}
		}
		created, err := svc.Create(soldier)
		if err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
		if i == 0 {
			firstID = created.ID
		}
	}
	if err := svc.AddImage(firstID, "portrait.jpg", `images\portrait.jpg`, "Portrait"); err != nil {
		t.Fatalf("AddImage: %v", err)
	}

	soldiers, total, err := svc.List(1, 3)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 5 {
		t.Errorf("total=%d, want 5", total)
	}
	if len(soldiers) != 3 {
		t.Errorf("page size=%d, want 3", len(soldiers))
	}
	if soldiers[0].RecordCount != 1 || soldiers[0].ImageCount != 1 {
		t.Fatalf("list should include record/image counts, got %#v", soldiers[0])
	}
}

func TestSoldierService_ArchiveCounts(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	soldier, err := svc.Create(models.Soldier{FirstName: "John", LastName: "Taylor"})
	if err != nil {
		t.Fatalf("Create soldier: %v", err)
	}

	for _, spouse := range []models.Soldier{
		{EntryType: "wife", SpouseSoldierID: soldier.ID, FirstName: "Martha", LastName: "Taylor"},
		{EntryType: "widow", SpouseSoldierID: soldier.ID, FirstName: "Sarah", LastName: "Hill"},
	} {
		if _, err := svc.Create(spouse); err != nil {
			t.Fatalf("Create spouse: %v", err)
		}
	}

	counts, err := svc.ArchiveCounts()
	if err != nil {
		t.Fatalf("ArchiveCounts: %v", err)
	}
	if counts.TotalSoldiers != 1 || counts.TotalWivesWidows != 2 {
		t.Fatalf("unexpected archive counts: %#v", counts)
	}
}

func TestSoldierService_SearchPage(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	_, err := svc.Create(models.Soldier{
		FirstName: "Nathan",
		LastName:  "Forrest",
		Unit:      "Forrest's Cavalry",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, err = svc.Create(models.Soldier{
		FirstName: "Joseph",
		LastName:  "Johnston",
		Unit:      "Army of Tennessee",
		DisplayID: "PENSION-4242",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	results, total, err := svc.SearchPage("Forrest", 1, 10)
	if err != nil {
		t.Fatalf("SearchPage: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("SearchPage returned %d results, want 1", len(results))
	}
	if total != 1 {
		t.Errorf("SearchPage total=%d, want 1", total)
	}
	if results[0].LastName != "Forrest" {
		t.Errorf("got %s, want Forrest", results[0].LastName)
	}
	if results[0].SearchMatchField != "Name" || !strings.Contains(results[0].SearchMatchSnippet, "Nathan") {
		t.Fatalf("expected quick search match metadata, got field=%q snippet=%q", results[0].SearchMatchField, results[0].SearchMatchSnippet)
	}

	displayIDResults, total, err := svc.SearchPage("4242", 1, 10)
	if err != nil {
		t.Fatalf("SearchPage display_id: %v", err)
	}
	if total != 1 || len(displayIDResults) != 1 {
		t.Fatalf("display_id search got total=%d len=%d, want 1/1", total, len(displayIDResults))
	}
	if !strings.HasSuffix(displayIDResults[0].DisplayID, "PENSION-4242") {
		t.Fatalf("got display_id %q", displayIDResults[0].DisplayID)
	}

	_, err = svc.Create(models.Soldier{
		FirstName: "Lewis",
		LastName:  "Armistead",
		BuriedIn:  "Hollywood Cemetery",
	})
	if err != nil {
		t.Fatalf("Create buried soldier: %v", err)
	}
	burialResults, total, err := svc.SearchPage("Hollywood", 1, 10)
	if err != nil {
		t.Fatalf("SearchPage buried_in: %v", err)
	}
	if total != 1 || len(burialResults) != 1 {
		t.Fatalf("burial search got total=%d len=%d, want 1/1", total, len(burialResults))
	}

	rankResults, total, err := svc.SearchPage("Tennessee", 1, 10)
	if err != nil {
		t.Fatalf("SearchPage unit: %v", err)
	}
	if total != 1 || len(rankResults) != 1 || !strings.HasSuffix(rankResults[0].DisplayID, "PENSION-4242") {
		t.Fatalf("unit search got total=%d len=%d results=%#v", total, len(rankResults), rankResults)
	}
}

func TestSoldierService_ListByEntryTypes(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	soldier, err := svc.Create(models.Soldier{FirstName: "Thomas", LastName: "Avery", EntryType: "soldier"})
	if err != nil {
		t.Fatalf("Create soldier: %v", err)
	}
	if _, err := svc.Create(models.Soldier{FirstName: "Sarah", LastName: "Avery", EntryType: "wife", SpouseSoldierID: soldier.ID}); err != nil {
		t.Fatalf("Create wife: %v", err)
	}
	if _, err := svc.Create(models.Soldier{FirstName: "Martha", LastName: "Avery", EntryType: "widow", SpouseSoldierID: soldier.ID}); err != nil {
		t.Fatalf("Create widow: %v", err)
	}

	results, total, err := svc.ListByEntryTypes([]string{"wife", "widow"}, 1, 10)
	if err != nil {
		t.Fatalf("ListByEntryTypes: %v", err)
	}
	if total != 2 || len(results) != 2 {
		t.Fatalf("unexpected grouped spouse results: total=%d len=%d results=%#v", total, len(results), results)
	}
	for _, result := range results {
		if result.EntryType != "wife" && result.EntryType != "widow" {
			t.Fatalf("unexpected entry type in grouped spouse results: %#v", result)
		}
	}
}

func TestSoldierService_RecentByIDsPreservesOrder(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	first, err := svc.Create(models.Soldier{DisplayID: "REC-0001", FirstName: "First", LastName: "Record"})
	if err != nil {
		t.Fatalf("Create first: %v", err)
	}
	second, err := svc.Create(models.Soldier{DisplayID: "REC-0002", FirstName: "Second", LastName: "Record"})
	if err != nil {
		t.Fatalf("Create second: %v", err)
	}

	results, err := svc.RecentByIDs([]int64{second.ID, first.ID}, 10)
	if err != nil {
		t.Fatalf("RecentByIDs: %v", err)
	}
	if len(results) != 2 || results[0].ID != second.ID || results[1].ID != first.ID {
		t.Fatalf("recent results did not preserve order: %#v", results)
	}
}

func TestSoldierService_ByIDsPreservesOrder(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	first, err := svc.Create(models.Soldier{DisplayID: "BID-0001", FirstName: "First", LastName: "Record"})
	if err != nil {
		t.Fatalf("Create first: %v", err)
	}
	second, err := svc.Create(models.Soldier{DisplayID: "BID-0002", FirstName: "Second", LastName: "Record"})
	if err != nil {
		t.Fatalf("Create second: %v", err)
	}
	third, err := svc.Create(models.Soldier{DisplayID: "BID-0003", FirstName: "Third", LastName: "Record"})
	if err != nil {
		t.Fatalf("Create third: %v", err)
	}

	// Reverse order to prove the SQL result order does not win.
	results, err := svc.ByIDs([]int64{third.ID, first.ID, second.ID})
	if err != nil {
		t.Fatalf("ByIDs: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("ByIDs returned %d results, want 3", len(results))
	}
	if results[0].ID != third.ID || results[1].ID != first.ID || results[2].ID != second.ID {
		t.Fatalf("ByIDs did not preserve caller order: %d, %d, %d", results[0].ID, results[1].ID, results[2].ID)
	}
}

func TestSoldierService_ByIDsEmpty(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	results, err := svc.ByIDs(nil)
	if err != nil {
		t.Fatalf("ByIDs(nil): %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("ByIDs(nil) returned %d results, want 0", len(results))
	}

	results, err = svc.ByIDs([]int64{})
	if err != nil {
		t.Fatalf("ByIDs([]): %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("ByIDs([]) returned %d results, want 0", len(results))
	}
}

func TestSoldierService_ByIDsMissingSilentlySkipped(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	real, err := svc.Create(models.Soldier{DisplayID: "BID-0004", FirstName: "Real", LastName: "Record"})
	if err != nil {
		t.Fatalf("Create real: %v", err)
	}

	results, err := svc.ByIDs([]int64{real.ID, 999999, real.ID, 888888})
	if err != nil {
		t.Fatalf("ByIDs: %v", err)
	}
	// 1 distinct real id; the duplicates dedupe; the missing ids
	// are silently dropped. Caller's order is preserved.
	if len(results) != 1 {
		t.Fatalf("ByIDs returned %d results, want 1 (deduped real only)", len(results))
	}
	if results[0].ID != real.ID {
		t.Fatalf("ByIDs returned id %d, want %d", results[0].ID, real.ID)
	}
}

func TestSoldierService_ByIDsLargeBatch(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	const total = 750
	ids := make([]int64, 0, total)
	for i := 0; i < total; i++ {
		s, err := svc.Create(models.Soldier{
			DisplayID: fmt.Sprintf("BID-%04d", i+1),
			FirstName: fmt.Sprintf("F%04d", i),
			LastName:  "Batch",
		})
		if err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
		ids = append(ids, s.ID)
	}

	results, err := svc.ByIDs(ids)
	if err != nil {
		t.Fatalf("ByIDs large: %v", err)
	}
	if len(results) != total {
		t.Fatalf("ByIDs large returned %d, want %d", len(results), total)
	}
	for i, got := range results {
		if got.ID != ids[i] {
			t.Fatalf("ByIDs large order broken at %d: got %d want %d", i, got.ID, ids[i])
		}
	}
}

func TestSoldierService_UnitCamaraderieGraph(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	central, err := svc.Create(models.Soldier{
		DisplayID: "CAM-0001",
		FirstName: "Andrew",
		LastName:  "Cole",
		Unit:      "Co. A, 1st Texas Infantry",
	})
	if err != nil {
		t.Fatalf("Create central: %v", err)
	}
	exact, err := svc.Create(models.Soldier{
		DisplayID: "CAM-0002",
		FirstName: "Thomas",
		LastName:  "Reed",
		Unit:      "Co. A, 1st Texas Infantry",
	})
	if err != nil {
		t.Fatalf("Create exact: %v", err)
	}
	companyVariant, err := svc.Create(models.Soldier{
		DisplayID: "CAM-0003",
		FirstName: "Samuel",
		LastName:  "Lane",
		Unit:      "Company A 1st Texas Infantry",
	})
	if err != nil {
		t.Fatalf("Create company variant: %v", err)
	}
	regimentPeer, err := svc.Create(models.Soldier{
		DisplayID: "CAM-0004",
		FirstName: "Henry",
		LastName:  "West",
		Unit:      "Co. B, 1st Texas Infantry",
	})
	if err != nil {
		t.Fatalf("Create regiment peer: %v", err)
	}
	if _, err := svc.Create(models.Soldier{
		DisplayID:       "CAM-0005",
		FirstName:       "Martha",
		LastName:        "Cole",
		EntryType:       "widow",
		SpouseSoldierID: central.ID,
		Unit:            "Co. A, 1st Texas Infantry",
	}); err != nil {
		t.Fatalf("Create widow: %v", err)
	}

	graph, err := svc.UnitCamaraderieGraph(central.ID)
	if err != nil {
		t.Fatalf("UnitCamaraderieGraph: %v", err)
	}
	if graph.Central.ID != central.ID {
		t.Fatalf("unexpected central record: %#v", graph.Central)
	}
	if len(graph.SameUnit) != 1 || graph.SameUnit[0].Soldier.ID != exact.ID {
		t.Fatalf("unexpected same-unit peers: %#v", graph.SameUnit)
	}
	if len(graph.SameCompanyVariant) != 1 || graph.SameCompanyVariant[0].Soldier.ID != companyVariant.ID {
		t.Fatalf("unexpected company-variant peers: %#v", graph.SameCompanyVariant)
	}
	if len(graph.SameRegiment) != 1 || graph.SameRegiment[0].Soldier.ID != regimentPeer.ID {
		t.Fatalf("unexpected same-regiment peers: %#v", graph.SameRegiment)
	}
}

func TestSoldierService_ServiceTimeline(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	created, err := svc.Create(models.Soldier{
		DisplayID: "TLM-0001",
		FirstName: "Andrew",
		LastName:  "Cole",
		Unit:      "1st Texas Infantry",
		BirthDate: "05/12/1838",
		DeathDate: "11/03/1904",
		BuriedIn:  "Oak Hill Cemetery",
		Records: []models.Record{
			{RecordType: "Muster Roll", AppID: "APP-1", Details: "Enlisted on 03/11/1862 at Austin."},
			{RecordType: "Parole", AppID: "APP-2", Details: "Paroled in April 1865 at Marshall, Texas."},
			{RecordType: "Pension", AppID: "APP-3", Details: "Filed in 1901 after moving back to Texas."},
			{RecordType: "Letter", AppID: "APP-4", Details: "Family correspondence with no year listed."},
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	timeline, err := svc.ServiceTimeline(created.ID)
	if err != nil {
		t.Fatalf("ServiceTimeline: %v", err)
	}
	if len(timeline.Events) != 6 {
		t.Fatalf("expected 6 timeline events, got %d: %#v", len(timeline.Events), timeline.Events)
	}
	if timeline.Events[0].Title != "Birth" || timeline.Events[1].Title != "Muster Roll" || timeline.Events[2].Title != "Parole" || timeline.Events[3].Title != "Pension" || timeline.Events[4].Title != "Death" || timeline.Events[5].Title != "Burial recorded" {
		t.Fatalf("unexpected timeline order: %#v", timeline.Events)
	}
	if timeline.ExactEventCount != 3 || timeline.InferredEventCount != 3 {
		t.Fatalf("unexpected event confidence counts: exact=%d inferred=%d", timeline.ExactEventCount, timeline.InferredEventCount)
	}
	if len(timeline.UndatedRecords) != 1 || timeline.UndatedRecords[0].RecordType != "Letter" {
		t.Fatalf("unexpected undated records: %#v", timeline.UndatedRecords)
	}
}

func TestSoldierService_ResearchLogLifecycle(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	created, err := svc.Create(models.Soldier{
		DisplayID: "RLG-0001",
		FirstName: "Andrew",
		LastName:  "Cole",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := svc.AddResearchTask(created.ID, "Locate pension file", "Check state archive holdings.", "pension"); err != nil {
		t.Fatalf("AddResearchTask: %v", err)
	}

	log, err := svc.ResearchLog(created.ID)
	if err != nil {
		t.Fatalf("ResearchLog: %v", err)
	}
	if log.OpenCount != 1 || len(log.Tasks) != 1 {
		t.Fatalf("unexpected research log counts: %#v", log)
	}
	if len(log.Suggestions) == 0 {
		t.Fatalf("expected missing-evidence suggestions")
	}

	if err := svc.ResolveResearchTask(created.ID, log.Tasks[0].ID); err != nil {
		t.Fatalf("ResolveResearchTask: %v", err)
	}
	log, err = svc.ResearchLog(created.ID)
	if err != nil {
		t.Fatalf("ResearchLog after resolve: %v", err)
	}
	if log.OpenCount != 0 || log.ResolvedCount != 1 || log.Tasks[0].Status != "resolved" {
		t.Fatalf("unexpected resolved research log: %#v", log)
	}
}

func TestSoldierService_ResearchPackForSoldier(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	central, err := svc.Create(models.Soldier{
		DisplayID:    "PACK-0001",
		FirstName:    "Andrew",
		LastName:     "Cole",
		PensionState: "Texas",
		BirthInfo:    "Born 1838 in Orange County, Texas.",
	})
	if err != nil {
		t.Fatalf("Create central: %v", err)
	}
	if _, err := svc.Create(models.Soldier{
		DisplayID:    "PACK-0002",
		FirstName:    "Thomas",
		LastName:     "Reed",
		PensionState: "Texas",
		Unit:         "1st Texas Infantry",
		BuriedIn:     "Oak Hill Cemetery",
	}); err != nil {
		t.Fatalf("Create state match: %v", err)
	}
	if _, err := svc.Create(models.Soldier{
		DisplayID: "PACK-0003",
		FirstName: "Samuel",
		LastName:  "Lane",
		BirthInfo: "Born 1840 in Orange County, Texas.",
		Unit:      "2nd Texas Infantry",
		BuriedIn:  "Evergreen Cemetery",
	}); err != nil {
		t.Fatalf("Create county match: %v", err)
	}

	statePack, err := svc.ResearchPackForSoldier(central.ID, "state")
	if err != nil {
		t.Fatalf("ResearchPackForSoldier state: %v", err)
	}
	if statePack.PlaceLabel != "Texas" || len(statePack.Related) != 2 {
		t.Fatalf("unexpected state pack: %#v", statePack)
	}

	countyPack, err := svc.ResearchPackForSoldier(central.ID, "county")
	if err != nil {
		t.Fatalf("ResearchPackForSoldier county: %v", err)
	}
	if countyPack.PlaceLabel != "Orange County" || len(countyPack.Related) != 1 || countyPack.Related[0].DisplayID != "PACK-0003" {
		t.Fatalf("unexpected county pack: %#v", countyPack)
	}
}

func TestSoldierService_ResearchCollections(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	created, err := svc.Create(models.Soldier{
		DisplayID: "COL-0001",
		FirstName: "Andrew",
		LastName:  "Cole",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.CreateResearchCollection("Orange County Cluster", "County-focused follow-up list."); err != nil {
		t.Fatalf("CreateResearchCollection: %v", err)
	}

	hub, err := svc.ResearchCollectionsHub(created.ID)
	if err != nil {
		t.Fatalf("ResearchCollectionsHub: %v", err)
	}
	if len(hub.Collections) != 1 || hub.Collections[0].ContainsCurrent {
		t.Fatalf("unexpected research collections hub: %#v", hub)
	}
	if err := svc.AddSoldierToResearchCollection(hub.Collections[0].ID, created.ID); err != nil {
		t.Fatalf("AddSoldierToResearchCollection: %v", err)
	}
	detail, err := svc.ResearchCollectionDetail(hub.Collections[0].ID, created.ID)
	if err != nil {
		t.Fatalf("ResearchCollectionDetail: %v", err)
	}
	if detail.Collection.ItemCount != 1 || len(detail.Members) != 1 || detail.Members[0].ID != created.ID {
		t.Fatalf("unexpected research collection detail: %#v", detail)
	}
}

func TestSoldierService_SearchPageMatchesPensionState(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	_, err := svc.Create(models.Soldier{
		DisplayID:    "PENSION-5150",
		FirstName:    "Mary",
		LastName:     "Bennett",
		PensionState: "Alabama",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	results, total, err := svc.SearchPage("Alabama", 1, 10)
	if err != nil {
		t.Fatalf("SearchPage pension_state: %v", err)
	}
	if total != 1 || len(results) != 1 {
		t.Fatalf("pension state search got total=%d len=%d", total, len(results))
	}
	if results[0].SearchMatchField != "Pension State" || results[0].SearchMatchSnippet != "Alabama" {
		t.Fatalf("unexpected match metadata: %#v", results[0])
	}
}

func TestSoldierService_SearchPageUsesContainsMatchingForNamesAndUnits(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	_, err := svc.Create(models.Soldier{
		DisplayID: "PENSION-5200",
		FirstName: "Nathan",
		LastName:  "Forrest",
		Unit:      "Army of Tennessee",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	nameResults, total, err := svc.SearchPage("orre", 1, 10)
	if err != nil {
		t.Fatalf("SearchPage partial name: %v", err)
	}
	if total != 1 || len(nameResults) != 1 {
		t.Fatalf("partial name search got total=%d len=%d", total, len(nameResults))
	}

	unitResults, total, err := svc.SearchPage("nnes", 1, 10)
	if err != nil {
		t.Fatalf("SearchPage partial unit: %v", err)
	}
	if total != 1 || len(unitResults) != 1 {
		t.Fatalf("partial unit search got total=%d len=%d", total, len(unitResults))
	}
}

func TestSoldierService_SearchPagePaginates(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	for i := 0; i < 5; i++ {
		_, err := svc.Create(models.Soldier{
			FirstName: "Searchable",
			LastName:  "Soldier",
			Unit:      "Archive Unit",
			DisplayID: fmt.Sprintf("PENSION-%04d", i),
		})
		if err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	results, total, err := svc.SearchPage("Searchable", 2, 2)
	if err != nil {
		t.Fatalf("SearchPage: %v", err)
	}
	if total != 5 {
		t.Fatalf("total=%d want 5", total)
	}
	if len(results) != 2 {
		t.Fatalf("len=%d want 2", len(results))
	}
}

func TestSoldierService_SearchPageMatchesBiographyNotesAndScratchPad(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	soldier, err := svc.Create(models.Soldier{
		DisplayID: "PENSION-7777",
		FirstName: "Thomas",
		LastName:  "Green",
		Biography: "Veteran served through Atlanta campaign and returned home in 1865.",
		Notes:     "Camp ledger mentions a silver pocket watch.",
		BirthDate: "01/01/1840",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	biographyResults, total, err := svc.SearchPage("Atlanta campaign", 1, 10)
	if err != nil {
		t.Fatalf("SearchPage biography: %v", err)
	}
	if total != 1 || len(biographyResults) != 1 {
		t.Fatalf("biography search got total=%d len=%d", total, len(biographyResults))
	}
	if biographyResults[0].SearchMatchField != "Biography" || !strings.Contains(biographyResults[0].SearchMatchSnippet, "Atlanta campaign") {
		t.Fatalf("unexpected biography match metadata: %#v", biographyResults[0])
	}

	noteResults, total, err := svc.SearchPage("pocket watch", 1, 10)
	if err != nil {
		t.Fatalf("SearchPage notes: %v", err)
	}
	if total != 1 || len(noteResults) != 1 {
		t.Fatalf("notes search got total=%d len=%d", total, len(noteResults))
	}
	if noteResults[0].SearchMatchField != "Notes" || !strings.Contains(noteResults[0].SearchMatchSnippet, "pocket watch") {
		t.Fatalf("unexpected notes match metadata: %#v", noteResults[0])
	}

	if err := d.SaveScratchpad(soldier.DisplayID, "Private memo about the Roswell depot."); err != nil {
		t.Fatalf("SaveScratchpad: %v", err)
	}

	scratchResults, total, err := svc.SearchPage("Roswell", 1, 10)
	if err != nil {
		t.Fatalf("SearchPage scratchpad: %v", err)
	}
	if total != 1 || len(scratchResults) != 1 {
		t.Fatalf("scratchpad search got total=%d len=%d", total, len(scratchResults))
	}
	if scratchResults[0].SearchMatchField != "Scratch Pad" || !strings.Contains(scratchResults[0].SearchMatchSnippet, "Roswell depot") {
		t.Fatalf("unexpected scratchpad match metadata: %#v", scratchResults[0])
	}
}

func TestSoldierService_AdvancedSearch(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	_, err := svc.Create(models.Soldier{
		DisplayID:  "PENSION-2024",
		FirstName:  "Robert",
		LastName:   "Taylor",
		Rank:       "Captain",
		Unit:       "1st Georgia Infantry",
		BuriedIn:   "Rose Hill Cemetery",
		DeathYear:  1864,
		DeathMonth: 5,
		DeathDay:   6,
	})
	if err != nil {
		t.Fatalf("Create matching soldier: %v", err)
	}
	_, err = svc.Create(models.Soldier{
		DisplayID: "PENSION-2025",
		FirstName: "Henry",
		LastName:  "Walker",
		Rank:      "Private",
		Unit:      "7th Texas Infantry",
	})
	if err != nil {
		t.Fatalf("Create non-matching soldier: %v", err)
	}

	results, total, err := svc.AdvancedSearch(models.SoldierSearch{
		Mode:       "advanced",
		DisplayID:  "2024",
		Rank:       "Captain",
		BuriedIn:   "Rose Hill",
		DeathYear:  "1864",
		DeathMonth: "5",
		DeathDay:   "6",
	}, 1, 10)
	if err != nil {
		t.Fatalf("AdvancedSearch: %v", err)
	}
	if total != 1 || len(results) != 1 {
		t.Fatalf("AdvancedSearch total=%d len=%d", total, len(results))
	}
	if !strings.HasSuffix(results[0].DisplayID, "PENSION-2024") {
		t.Fatalf("got display_id %q", results[0].DisplayID)
	}
}

func TestSoldierService_AdvancedSearchRequiresAllFilters(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	_, err := svc.Create(models.Soldier{
		DisplayID: "PENSION-3001",
		FirstName: "Thomas",
		LastName:  "Carter",
		Rank:      "Captain",
		Unit:      "4th Alabama Cavalry",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	results, total, err := svc.AdvancedSearch(models.SoldierSearch{
		Mode:      "advanced",
		FirstName: "Thomas",
		Unit:      "Georgia",
	}, 1, 10)
	if err != nil {
		t.Fatalf("AdvancedSearch: %v", err)
	}
	if total != 0 || len(results) != 0 {
		t.Fatalf("expected no advanced search matches, got total=%d len=%d", total, len(results))
	}
}

func TestSoldierService_AdvancedSearchByDeathYearOnly(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	_, err := svc.Create(models.Soldier{
		DisplayID: "PENSION-4001",
		FirstName: "Henry",
		LastName:  "Dawson",
		DeathYear: 1862,
	})
	if err != nil {
		t.Fatalf("Create first soldier: %v", err)
	}
	_, err = svc.Create(models.Soldier{
		DisplayID: "PENSION-4002",
		FirstName: "Walter",
		LastName:  "Hughes",
		DeathYear: 1863,
	})
	if err != nil {
		t.Fatalf("Create second soldier: %v", err)
	}

	results, total, err := svc.AdvancedSearch(models.SoldierSearch{
		Mode:      "advanced",
		DeathYear: "1862",
	}, 1, 10)
	if err != nil {
		t.Fatalf("AdvancedSearch: %v", err)
	}
	if total != 1 || len(results) != 1 || !strings.HasSuffix(results[0].DisplayID, "PENSION-4001") {
		t.Fatalf("death-year search got total=%d len=%d results=%#v", total, len(results), results)
	}
}

func TestSoldierService_AdvancedSearchExpandedFields(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	_, err := svc.Create(models.Soldier{
		DisplayID:             "PENSION-5100",
		FirstName:             "Anna",
		MiddleName:            "Maria",
		LastName:              "Bennett",
		MaidenName:            "Carter",
		RankIn:                "Private",
		RankOut:               "Captain",
		Unit:                  "1st Texas Infantry",
		PensionState:          "Texas",
		ConfederateHomeStatus: "Staffer",
		ConfederateHomeName:   "Texas Confederate Home",
		BirthDate:             "05/06/1838",
		DeathYear:             1864,
		BuriedIn:              "Oakwood Cemetery",
		Records: []models.Record{
			{RecordType: "Pension Ledger"},
		},
	})
	if err != nil {
		t.Fatalf("Create matching soldier: %v", err)
	}
	_, err = svc.Create(models.Soldier{
		DisplayID:             "PENSION-5101",
		FirstName:             "Anne",
		LastName:              "Miller",
		MaidenName:            "Wilson",
		RankIn:                "Sergeant",
		RankOut:               "Major",
		Unit:                  "3rd Arkansas Cavalry",
		PensionState:          "Arkansas",
		ConfederateHomeStatus: "Inmate",
		ConfederateHomeName:   "Arkansas Confederate Home",
		BirthDate:             "07/08/1842",
		DeathYear:             1862,
		BuriedIn:              "Maple Grove Cemetery",
		Records: []models.Record{
			{RecordType: "Muster Roll"},
		},
	})
	if err != nil {
		t.Fatalf("Create non-matching soldier: %v", err)
	}

	results, total, err := svc.AdvancedSearch(models.SoldierSearch{
		Mode:                  "advanced",
		FirstName:             "Ann",
		MiddleName:            "Mari",
		LastName:              "Benne",
		MaidenName:            "Cart",
		RankIn:                "Priv",
		RankOut:               "Capt",
		Unit:                  "Texas",
		RecordType:            "Ledger",
		PensionState:          "Texas",
		ConfederateHomeStatus: "Staffer",
		ConfederateHomeName:   "Confederate Home",
		BuriedIn:              "Oakwood",
		BirthYear:             "1838",
		DeathYear:             "1864",
	}, 1, 10)
	if err != nil {
		t.Fatalf("AdvancedSearch expanded fields: %v", err)
	}
	if total != 1 || len(results) != 1 || !strings.HasSuffix(results[0].DisplayID, "PENSION-5100") {
		t.Fatalf("expanded advanced search got total=%d len=%d results=%#v", total, len(results), results)
	}
}

func TestSoldierService_AdvancedSearchYearRanges(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	_, err := svc.Create(models.Soldier{
		DisplayID: "PENSION-5201",
		FirstName: "George",
		LastName:  "Lane",
		BirthDate: "00/00/1838",
		DeathYear: 1864,
	})
	if err != nil {
		t.Fatalf("Create first soldier: %v", err)
	}
	_, err = svc.Create(models.Soldier{
		DisplayID: "PENSION-5202",
		FirstName: "Walter",
		LastName:  "Hughes",
		BirthDate: "00/00/1844",
		DeathYear: 1861,
	})
	if err != nil {
		t.Fatalf("Create second soldier: %v", err)
	}

	results, total, err := svc.AdvancedSearch(models.SoldierSearch{
		Mode:        "advanced",
		BirthYear:   "1837",
		BirthYearTo: "1839",
		DeathYear:   "1863",
		DeathYearTo: "1865",
	}, 1, 10)
	if err != nil {
		t.Fatalf("AdvancedSearch year ranges: %v", err)
	}
	if total != 1 || len(results) != 1 || !strings.HasSuffix(results[0].DisplayID, "PENSION-5201") {
		t.Fatalf("year range search got total=%d len=%d results=%#v", total, len(results), results)
	}
}

func TestSoldierService_AdvancedSearchReviewStatusAndQueue(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	clean, err := svc.Create(models.Soldier{FirstName: "Clean", LastName: "Record"})
	if err != nil {
		t.Fatalf("Create clean soldier: %v", err)
	}
	flagged, err := svc.Create(models.Soldier{FirstName: "Flagged", LastName: "Record"})
	if err != nil {
		t.Fatalf("Create flagged soldier: %v", err)
	}
	if err := svc.SetReviewStatus(flagged.ID, true, "Potential duplicate from import"); err != nil {
		t.Fatalf("SetReviewStatus: %v", err)
	}

	reviewResults, total, err := svc.AdvancedSearch(models.SoldierSearch{Mode: "advanced", ReviewStatus: "review"}, 1, 10)
	if err != nil {
		t.Fatalf("AdvancedSearch review status: %v", err)
	}
	if total != 1 || len(reviewResults) != 1 || reviewResults[0].ID != flagged.ID {
		t.Fatalf("review filter returned total=%d len=%d results=%#v", total, len(reviewResults), reviewResults)
	}

	cleanResults, total, err := svc.AdvancedSearch(models.SoldierSearch{Mode: "advanced", ReviewStatus: "clean"}, 1, 10)
	if err != nil {
		t.Fatalf("AdvancedSearch clean status: %v", err)
	}
	if total != 1 || len(cleanResults) != 1 || cleanResults[0].ID != clean.ID {
		t.Fatalf("clean filter returned total=%d len=%d results=%#v", total, len(cleanResults), cleanResults)
	}

	queue, total, err := svc.ReviewQueue(1, 10)
	if err != nil {
		t.Fatalf("ReviewQueue: %v", err)
	}
	if total != 1 || len(queue) != 1 || queue[0].ID != flagged.ID || queue[0].ReviewReason != "Potential duplicate from import" {
		t.Fatalf("review queue returned total=%d len=%d results=%#v", total, len(queue), queue)
	}

	if err := svc.MarkReviewResolved(flagged.ID); err != nil {
		t.Fatalf("MarkReviewResolved: %v", err)
	}
	queue, total, err = svc.ReviewQueue(1, 10)
	if err != nil {
		t.Fatalf("ReviewQueue after resolve: %v", err)
	}
	if total != 0 || len(queue) != 0 {
		t.Fatalf("review queue should be empty after resolve: total=%d len=%d results=%#v", total, len(queue), queue)
	}
	counts, err := svc.ArchiveCounts()
	if err != nil {
		t.Fatalf("ArchiveCounts: %v", err)
	}
	if counts.TotalSoldiers != 2 || counts.TotalWivesWidows != 0 {
		t.Fatalf("review flags should not affect archive counts: %#v", counts)
	}
}

func TestSoldierService_AdvancedSearchByEntryType(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	soldier, err := svc.Create(models.Soldier{FirstName: "Thomas", LastName: "Avery", EntryType: "soldier"})
	if err != nil {
		t.Fatalf("Create soldier: %v", err)
	}
	wife, err := svc.Create(models.Soldier{FirstName: "Sarah", LastName: "Avery", EntryType: "wife", SpouseSoldierID: soldier.ID})
	if err != nil {
		t.Fatalf("Create wife: %v", err)
	}
	widow, err := svc.Create(models.Soldier{FirstName: "Martha", LastName: "Avery", EntryType: "widow", SpouseSoldierID: soldier.ID})
	if err != nil {
		t.Fatalf("Create widow: %v", err)
	}
	linked, err := svc.Create(models.Soldier{FirstName: "James", LastName: "Avery", EntryType: "linked_person", SpouseSoldierID: soldier.ID, RelationshipLabel: "Brother"})
	if err != nil {
		t.Fatalf("Create person record: %v", err)
	}

	soldierResults, total, err := svc.AdvancedSearch(models.SoldierSearch{Mode: "advanced", EntryType: "soldier"}, 1, 10)
	if err != nil {
		t.Fatalf("AdvancedSearch soldier entry type: %v", err)
	}
	if total != 1 || len(soldierResults) != 1 || soldierResults[0].ID != soldier.ID {
		t.Fatalf("soldier entry-type filter returned total=%d len=%d results=%#v", total, len(soldierResults), soldierResults)
	}

	wifeResults, total, err := svc.AdvancedSearch(models.SoldierSearch{Mode: "advanced", EntryType: "wife"}, 1, 10)
	if err != nil {
		t.Fatalf("AdvancedSearch wife entry type: %v", err)
	}
	if total != 1 || len(wifeResults) != 1 || wifeResults[0].ID != wife.ID {
		t.Fatalf("wife entry-type filter returned total=%d len=%d results=%#v", total, len(wifeResults), wifeResults)
	}

	widowResults, total, err := svc.AdvancedSearch(models.SoldierSearch{Mode: "advanced", EntryType: "widow"}, 1, 10)
	if err != nil {
		t.Fatalf("AdvancedSearch widow entry type: %v", err)
	}
	if total != 1 || len(widowResults) != 1 || widowResults[0].ID != widow.ID {
		t.Fatalf("widow entry-type filter returned total=%d len=%d results=%#v", total, len(widowResults), widowResults)
	}

	linkedResults, total, err := svc.AdvancedSearch(models.SoldierSearch{Mode: "advanced", EntryType: "linked_person"}, 1, 10)
	if err != nil {
		t.Fatalf("AdvancedSearch person record entry type: %v", err)
	}
	if total != 1 || len(linkedResults) != 1 || linkedResults[0].ID != linked.ID || linkedResults[0].RelationshipLabel != "Brother" {
		t.Fatalf("linked-person entry-type filter returned total=%d len=%d results=%#v", total, len(linkedResults), linkedResults)
	}

	relationshipResults, total, err := svc.AdvancedSearch(models.SoldierSearch{Mode: "advanced", RelationshipLabel: "brother"}, 1, 10)
	if err != nil {
		t.Fatalf("AdvancedSearch relationship label: %v", err)
	}
	if total != 1 || len(relationshipResults) != 1 || relationshipResults[0].ID != linked.ID {
		t.Fatalf("relationship-label filter returned total=%d len=%d results=%#v", total, len(relationshipResults), relationshipResults)
	}
}

func TestSoldierService_ManualComparison(t *testing.T) {
	d := newTestDB(t)
	svc := NewSoldierService(d)

	left, err := svc.Create(models.Soldier{
		DisplayID: "CMP-0001",
		FirstName: "John",
		LastName:  "Morris",
		Unit:      "4th Texas Infantry",
		BirthDate: "00/00/1838",
	})
	if err != nil {
		t.Fatalf("Create left soldier: %v", err)
	}
	right, err := svc.Create(models.Soldier{
		DisplayID: "CMP-0002",
		FirstName: "Jon",
		LastName:  "Morris",
		Unit:      "4th Texas Infantry",
		BirthDate: "00/00/1839",
	})
	if err != nil {
		t.Fatalf("Create right soldier: %v", err)
	}

	comparison, err := svc.ManualComparison(left.ID, right.ID)
	if err != nil {
		t.Fatalf("ManualComparison: %v", err)
	}
	if comparison.PageTitle != "Person Record Comparison" || comparison.BackHref != "/soldiers" || comparison.BackLabel != "Back" {
		t.Fatalf("unexpected comparison header metadata: %#v", comparison)
	}
	if comparison.LeftSoldier.ID != left.ID || comparison.RightSoldier.ID != right.ID {
		t.Fatalf("unexpected comparison soldiers: %#v", comparison)
	}
	highlighted := false
	for _, field := range comparison.Fields {
		if field.Label == "First Name" && field.Highlighted && field.LeftValue == "John" && field.RightValue == "Jon" {
			highlighted = true
			break
		}
	}
	if !highlighted {
		t.Fatalf("expected differing fields to be highlighted: %#v", comparison.Fields)
	}
}

func TestSoldierService_CountNeedsReview(t *testing.T) {
	svc := NewSoldierService(newTestDB(t))

	// Empty archive: count is 0.
	count, err := svc.CountNeedsReview()
	if err != nil {
		t.Fatalf("CountNeedsReview (empty): %v", err)
	}
	if count != 0 {
		t.Fatalf("empty archive count = %d, want 0", count)
	}

	// Flag 3 records.
	for i := 0; i < 3; i++ {
		_, err := svc.Create(models.Soldier{
			DisplayID:   fmt.Sprintf("CNT-%03d", i),
			FirstName:   "Count",
			LastName:    "Test",
			NeedsReview: true,
		})
		if err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	// Add 2 unflagged records (should not count).
	for i := 0; i < 2; i++ {
		_, err := svc.Create(models.Soldier{
			DisplayID: fmt.Sprintf("OK-%03d", i),
			FirstName: "Ok",
			LastName:  "Test",
		})
		if err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	count, err = svc.CountNeedsReview()
	if err != nil {
		t.Fatalf("CountNeedsReview (with mixed): %v", err)
	}
	if count != 3 {
		t.Errorf("count = %d, want 3 (only flagged records)", count)
	}
}
