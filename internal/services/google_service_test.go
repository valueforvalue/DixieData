package services

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/valueforvalue/DixieData/internal/models"
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
	event := googleCalendarEvent(models.Soldier{
		DisplayID:  "PENSION-42",
		FirstName:  "Robert",
		LastName:   "Lee",
		Rank:       "General",
		Unit:       "Army of Northern Virginia",
		BuriedIn:   "Hollywood Cemetery",
		DeathYear:  1862,
		DeathMonth: 5,
		DeathDay:   13,
	})

	expectedDate := nextGoogleAnniversaryDate(models.Soldier{DeathMonth: 5, DeathDay: 13}, time.Now()).Format("2006-01-02")
	if event.Start == nil || event.Start.DateTime == "" || !strings.HasPrefix(event.Start.DateTime, expectedDate+"T09:00:00") {
		t.Fatalf("start = %#v", event.Start)
	}
	if event.End == nil || event.End.DateTime == "" {
		t.Fatalf("end = %#v", event.End)
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
