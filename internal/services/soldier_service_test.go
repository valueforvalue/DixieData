package services

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

	if s.DisplayID != "TDM65-00001" {
		t.Errorf("expected TDM65-00001, got %s", s.DisplayID)
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

	if s.DisplayID != "TDM65-PENSION-9999" {
		t.Errorf("expected TDM65-PENSION-9999, got %s", s.DisplayID)
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
	if s.DisplayID != "TDM65-DXD-00001" {
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
	if got.Prefix != "Capt." || got.MiddleName != "Bell" || got.Suffix != "Jr." || got.RankIn != "Colonel" || got.RankOut != "Lieutenant General" || got.PensionState != "Texas" || got.PensionID != "P12345" || got.ApplicationID != "A12345" || got.ConfederateHomeStatus != "Staffer" || got.ConfederateHomeName != "Texas Confederate Home" {
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
	if got.Prefix != "" || got.MiddleName != "" || got.Suffix != "" || got.RankIn != "" || got.RankOut != "" || got.PensionState != "" || got.PensionID != "" || got.ApplicationID != "" || got.ConfederateHomeStatus != "None" || got.ConfederateHomeName != "" {
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

	if created.ConfederateHomeStatus != "None" || created.ConfederateHomeName != "" {
		t.Fatalf("created = %#v", created)
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
		`Prefix changed from "Not recorded" to "Gen.".`,
		`Middle Name changed from "Not recorded" to "A.".`,
		`Suffix changed from "Not recorded" to "Sr.".`,
		`Rank In changed from "Not recorded" to "Private".`,
		`Rank Out changed from "Not recorded" to "Major".`,
		`Pension State changed from "Not recorded" to "Georgia".`,
		`Notes changed from "Not recorded" to "Updated note".`,
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
	if !image.IsPrimary {
		t.Fatalf("expected image to be primary: %#v", image)
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

	for i := 0; i < 5; i++ {
		_, err := svc.Create(models.Soldier{
			FirstName: "Soldier",
			LastName:  "Test",
		})
		if err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
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
