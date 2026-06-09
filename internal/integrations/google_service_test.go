package integrations

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/valueforvalue/DixieData/internal/models"
	"google.golang.org/api/drive/v3"
)

func TestGoogleService_SaveAndLoadSettings(t *testing.T) {
	service := NewGoogleService(t.TempDir())
	settings := models.GoogleSettings{
		ClientID:      "client-id",
		ClientSecret:  "client-secret",
		CalendarID:    "calendar-id",
		DriveFolderID: "folder-id",
	}
	if err := service.SaveSettings(settings); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	loaded, err := service.LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if loaded != settings {
		t.Fatalf("loaded settings = %#v", loaded)
	}
}

func TestGoogleCalendarEventBuildsYearlyTimedEventWithReminders(t *testing.T) {
	const calendarTimeZone = "America/Chicago"
	event := googleCalendarEventWithTimeZone(models.Soldier{
		DisplayID:  "PENSION-42",
		SyncID:     "sync-42",
		FirstName:  "Robert",
		LastName:   "Lee",
		Rank:       "General",
		Unit:       "Army of Northern Virginia",
		BuriedIn:   "Hollywood Cemetery",
		DeathYear:  1862,
		DeathMonth: 5,
		DeathDay:   13,
	}, calendarTimeZone)

	location, err := time.LoadLocation(calendarTimeZone)
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	expectedDate := nextGoogleAnniversaryDate(models.Soldier{DeathMonth: 5, DeathDay: 13}, time.Now().In(location)).Format("2006-01-02")
	if event.Start == nil || event.Start.DateTime == "" || !strings.HasPrefix(event.Start.DateTime, expectedDate+"T09:00:00") {
		t.Fatalf("start = %#v", event.Start)
	}
	if event.Start.TimeZone != calendarTimeZone {
		t.Fatalf("start timezone = %q", event.Start.TimeZone)
	}
	if event.End == nil || event.End.DateTime == "" {
		t.Fatalf("end = %#v", event.End)
	}
	if event.End.TimeZone != calendarTimeZone {
		t.Fatalf("end timezone = %q", event.End.TimeZone)
	}
	if len(event.Recurrence) != 1 || event.Recurrence[0] != "RRULE:FREQ=YEARLY" {
		t.Fatalf("recurrence = %#v", event.Recurrence)
	}
	if event.Reminders == nil || event.Reminders.UseDefault || len(event.Reminders.Overrides) != 2 {
		t.Fatalf("reminders = %#v", event.Reminders)
	}
	if event.ExtendedProperties == nil || event.ExtendedProperties.Private["dixiedata_display_id"] != "PENSION-42" {
		t.Fatalf("extended properties = %#v", event.ExtendedProperties)
	}
	if event.ExtendedProperties.Private["dixiedata_sync_id"] != "sync-42" {
		t.Fatalf("sync property = %#v", event.ExtendedProperties)
	}
}

func TestGoogleCalendarEventIDFallsBackToLegacyDisplayID(t *testing.T) {
	key, eventID := googleCalendarEventID(map[string]string{
		"DXD-00001": "event-1",
	}, models.Soldier{
		DisplayID: "TDM65-DXD-00001",
		SyncID:    "sync-1",
	})
	if key != "DXD-00001" || eventID != "event-1" {
		t.Fatalf("got key=%q eventID=%q", key, eventID)
	}
}

func TestGoogleService_DisconnectRemovesTokenAndSyncState(t *testing.T) {
	dataDir := t.TempDir()
	service := NewGoogleService(dataDir)

	for _, name := range []string{googleTokenFile, googleCalendarSyncFile} {
		if err := writeJSONFile(filepath.Join(dataDir, name), map[string]string{"a": "b"}); err != nil {
			t.Fatalf("writeJSONFile: %v", err)
		}
	}

	if err := service.Disconnect(); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
}

func TestGoogleService_LoadEffectiveSettingsUsesSharedDefaults(t *testing.T) {
	t.Setenv("DIXIEDATA_GOOGLE_CLIENT_ID", "shared-client")
	t.Setenv("DIXIEDATA_GOOGLE_CLIENT_SECRET", "shared-secret")
	t.Setenv("DIXIEDATA_GOOGLE_CALENDAR_ID", "primary")

	service := NewGoogleService(t.TempDir())
	effective, usingSharedClient, sharedClientAvailable, sharedClientSource, err := service.LoadEffectiveSettings()
	if err != nil {
		t.Fatalf("LoadEffectiveSettings: %v", err)
	}
	if !sharedClientAvailable {
		t.Fatalf("sharedClientAvailable = false")
	}
	if !usingSharedClient {
		t.Fatalf("usingSharedClient = false")
	}
	if sharedClientSource == "" {
		t.Fatalf("sharedClientSource = empty")
	}
	if effective.ClientID != "shared-client" || effective.ClientSecret != "shared-secret" {
		t.Fatalf("effective = %#v", effective)
	}
}

func TestGoogleService_LoadEffectiveSettingsLetsSavedValuesOverrideSharedDefaults(t *testing.T) {
	t.Setenv("DIXIEDATA_GOOGLE_CLIENT_ID", "shared-client")
	t.Setenv("DIXIEDATA_GOOGLE_CLIENT_SECRET", "shared-secret")

	service := NewGoogleService(t.TempDir())
	if err := service.SaveSettings(models.GoogleSettings{
		ClientID:      "custom-client",
		ClientSecret:  "custom-secret",
		CalendarID:    "custom-calendar",
		DriveFolderID: "custom-folder",
	}); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	effective, usingSharedClient, sharedClientAvailable, _, err := service.LoadEffectiveSettings()
	if err != nil {
		t.Fatalf("LoadEffectiveSettings: %v", err)
	}
	if !sharedClientAvailable {
		t.Fatalf("sharedClientAvailable = false")
	}
	if usingSharedClient {
		t.Fatalf("usingSharedClient = true")
	}
	if effective.ClientID != "custom-client" || effective.ClientSecret != "custom-secret" || effective.CalendarID != "custom-calendar" || effective.DriveFolderID != "custom-folder" {
		t.Fatalf("effective = %#v", effective)
	}
}

func TestGoogleSheetUploadNameDropsCSVExtension(t *testing.T) {
	got := googleSheetUploadName("", `C:\Development\DixieData\files\dixiedata-export.csv`)
	if got != "dixiedata-export" {
		t.Fatalf("googleSheetUploadName = %q", got)
	}
}

func TestGoogleSheetUploadNamePrefersExplicitTitle(t *testing.T) {
	got := googleSheetUploadName("DixieData Export", `C:\Development\DixieData\files\dixiedata-export.csv`)
	if got != "DixieData Export" {
		t.Fatalf("googleSheetUploadName = %q", got)
	}
}

func TestGoogleDriveUploadResultUsesSheetsLinkFallback(t *testing.T) {
	result := googleDriveUploadResult(&drive.File{
		Id:       "sheet123",
		Name:     "DixieData Export",
		MimeType: "application/vnd.google-apps.spreadsheet",
	})
	if result.WebViewLink != "https://docs.google.com/spreadsheets/d/sheet123/edit" {
		t.Fatalf("WebViewLink = %q", result.WebViewLink)
	}
}

func TestGoogleService_CalendarDriftStatusCountsAddedUpdatedRemoved(t *testing.T) {
	service := NewGoogleService(t.TempDir())
	baselineSame := models.Soldier{SyncID: "sync-b", DisplayID: "DXD-00002", FirstName: "Beta", LastName: "Same", DeathMonth: 5, DeathDay: 11}
	if err := writeJSONFile(filepath.Join(service.dataDir, googleCalendarSyncFile), GoogleCalendarSyncState{
		LastSyncedAt: "2026-06-08T18:00:00Z",
		LastSyncSignatures: map[string]string{
			"sync-a": "sig-a-old",
			"sync-b": googleCalendarSignature(baselineSame),
			"sync-c": "sig-c",
		},
	}); err != nil {
		t.Fatalf("write sync state: %v", err)
	}

	status, err := service.CalendarDriftStatus([]models.Soldier{
		{SyncID: "sync-a", DisplayID: "DXD-00001", FirstName: "Alpha", LastName: "Updated", DeathMonth: 5, DeathDay: 10},
		{SyncID: "sync-b", DisplayID: "DXD-00002", FirstName: "Beta", LastName: "Same", DeathMonth: 5, DeathDay: 11},
		{SyncID: "sync-new", DisplayID: "DXD-00003", FirstName: "Gamma", LastName: "Added", DeathMonth: 5, DeathDay: 12},
	})
	if err != nil {
		t.Fatalf("CalendarDriftStatus: %v", err)
	}
	if status.LastSyncedAt != "2026-06-08T18:00:00Z" {
		t.Fatalf("LastSyncedAt = %q", status.LastSyncedAt)
	}
	if status.Added != 1 || status.Updated != 1 || status.Removed != 1 || !status.OutOfSync {
		t.Fatalf("status = %#v", status)
	}
}

func TestSyntheticTestEventsProducesThreeDeterministicEntries(t *testing.T) {
	const calendarTimeZone = "America/Chicago"
	events := syntheticTestEvents()
	if len(events) != 3 {
		t.Fatalf("len(events) = %d", len(events))
	}
	seen := map[string]struct{}{}
	for _, event := range events {
		if _, ok := seen[event.Key]; ok {
			t.Fatalf("duplicate test event key %q", event.Key)
		}
		seen[event.Key] = struct{}{}
		googleEvent := event.toCalendarEvent(calendarTimeZone)
		if googleEvent.ExtendedProperties == nil || googleEvent.ExtendedProperties.Private["dixiedata_test"] != "true" {
			t.Fatalf("missing test marker: %#v", googleEvent.ExtendedProperties)
		}
		if googleEvent.Start == nil || googleEvent.Start.TimeZone != calendarTimeZone {
			t.Fatalf("start timezone = %#v", googleEvent.Start)
		}
	}
}
