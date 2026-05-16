package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/browser"
	"github.com/valueforvalue/DixieData/internal/models"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	gcal "google.golang.org/api/calendar/v3"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

const (
	googleCalendarSyncFile = "google-calendar-sync.json"
	googleDefaultsFile     = "google-oauth-defaults.json"
	googleSettingsFile     = "google-settings.json"
	googleTokenFile        = "google-token.json"
)

type GoogleCalendarSyncState struct {
	EventIDs map[string]string `json:"event_ids"`
}

type GoogleDriveUploadResult struct {
	FileID      string
	WebViewLink string
	Name        string
}

type GoogleCalendarSyncResult struct {
	Created int
	Updated int
	Deleted int
	Skipped int
}

type GoogleCalendarUnsyncResult struct {
	Deleted int
}

type GoogleService struct {
	dataDir string
}

func NewGoogleService(dataDir string) *GoogleService {
	return &GoogleService{dataDir: dataDir}
}

func (g *GoogleService) Status() (models.GoogleStatus, error) {
	settings, err := g.LoadSettings()
	if err != nil {
		return models.GoogleStatus{}, err
	}
	effective, usingSharedClient, sharedClientAvailable, sharedClientSource, err := g.LoadEffectiveSettings()
	if err != nil {
		return models.GoogleStatus{}, err
	}
	token, _ := g.loadToken()
	return models.GoogleStatus{
		Settings:              settings,
		Connected:             token != nil,
		HasClientID:           strings.TrimSpace(effective.ClientID) != "",
		HasSecret:             strings.TrimSpace(effective.ClientSecret) != "",
		HasToken:              token != nil,
		SharedClientAvailable: sharedClientAvailable,
		SharedClientSource:    sharedClientSource,
		UsingSharedClient:     usingSharedClient,
	}, nil
}

func (g *GoogleService) SaveSettings(settings models.GoogleSettings) error {
	if strings.TrimSpace(settings.CalendarID) == "" {
		settings.CalendarID = "primary"
	}
	return writeJSONFile(filepath.Join(g.dataDir, googleSettingsFile), settings)
}

func (g *GoogleService) LoadSettings() (models.GoogleSettings, error) {
	return g.loadSavedSettings()
}

func (g *GoogleService) LoadEffectiveSettings() (models.GoogleSettings, bool, bool, string, error) {
	saved, err := g.loadSavedSettings()
	if err != nil {
		return models.GoogleSettings{}, false, false, "", err
	}
	shared, sharedClientSource, sharedClientAvailable, err := g.loadSharedSettings()
	if err != nil {
		return models.GoogleSettings{}, false, false, "", err
	}
	effective := mergeGoogleSettings(shared, saved)
	usingSharedClient := sharedClientAvailable &&
		strings.TrimSpace(saved.ClientID) == "" &&
		strings.TrimSpace(saved.ClientSecret) == "" &&
		strings.TrimSpace(effective.ClientID) != "" &&
		strings.TrimSpace(effective.ClientSecret) != ""
	if strings.TrimSpace(effective.CalendarID) == "" {
		effective.CalendarID = "primary"
	}
	return effective, usingSharedClient, sharedClientAvailable, sharedClientSource, nil
}

func (g *GoogleService) loadSavedSettings() (models.GoogleSettings, error) {
	path := filepath.Join(g.dataDir, googleSettingsFile)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return models.GoogleSettings{CalendarID: "primary"}, nil
		}
		return models.GoogleSettings{}, err
	}
	var settings models.GoogleSettings
	if err := readJSONFile(path, &settings); err != nil {
		return models.GoogleSettings{}, err
	}
	if strings.TrimSpace(settings.CalendarID) == "" {
		settings.CalendarID = "primary"
	}
	return settings, nil
}

func (g *GoogleService) Disconnect() error {
	for _, name := range []string{googleTokenFile, googleCalendarSyncFile} {
		path := filepath.Join(g.dataDir, name)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func (g *GoogleService) Connect(ctx context.Context) error {
	settings, _, _, _, err := g.LoadEffectiveSettings()
	if err != nil {
		return err
	}
	if strings.TrimSpace(settings.ClientID) == "" || strings.TrimSpace(settings.ClientSecret) == "" {
		return fmt.Errorf("configure shared Google OAuth defaults or save Google client ID and client secret first")
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer listener.Close()

	state, err := randomOAuthState()
	if err != nil {
		return err
	}
	redirectURL := fmt.Sprintf("http://%s/callback", listener.Addr().String())
	config := g.oauthConfig(settings, redirectURL)

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("state") != state {
				errCh <- fmt.Errorf("invalid Google OAuth state")
				http.Error(w, "invalid state", http.StatusBadRequest)
				return
			}
			if authErr := r.URL.Query().Get("error"); authErr != "" {
				errCh <- fmt.Errorf("google authorization failed: %s", authErr)
				http.Error(w, authErr, http.StatusBadRequest)
				return
			}
			code := r.URL.Query().Get("code")
			if code == "" {
				errCh <- fmt.Errorf("google authorization did not return a code")
				http.Error(w, "missing code", http.StatusBadRequest)
				return
			}
			fmt.Fprint(w, `<html><body style="font-family:Arial,sans-serif;padding:2rem;"><h2>Google connected</h2><p>You can return to DixieData.</p></body></html>`)
			codeCh <- code
		}),
	}
	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && serveErr != http.ErrServerClosed {
			errCh <- serveErr
		}
	}()

	authURL := config.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("prompt", "consent"))
	if err := browser.OpenURL(authURL); err != nil {
		server.Shutdown(context.Background())
		return err
	}

	waitCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	var code string
	select {
	case code = <-codeCh:
	case err = <-errCh:
		server.Shutdown(context.Background())
		return err
	case <-waitCtx.Done():
		server.Shutdown(context.Background())
		return fmt.Errorf("google authorization timed out")
	}

	server.Shutdown(context.Background())
	token, err := config.Exchange(ctx, code)
	if err != nil {
		return err
	}
	return g.saveToken(token)
}

func (g *GoogleService) UploadBackup(ctx context.Context, backupPath string) (GoogleDriveUploadResult, error) {
	client, settings, err := g.client(ctx)
	if err != nil {
		return GoogleDriveUploadResult{}, err
	}

	driveSvc, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return GoogleDriveUploadResult{}, err
	}

	file, err := os.Open(backupPath)
	if err != nil {
		return GoogleDriveUploadResult{}, err
	}
	defer file.Close()

	driveFile := &drive.File{Name: filepath.Base(backupPath)}
	if folderID, err := resolveGoogleDriveFolder(ctx, driveSvc, settings.DriveFolderID); err != nil {
		return GoogleDriveUploadResult{}, err
	} else if folderID != "" {
		driveFile.Parents = []string{folderID}
	}

	created, err := driveSvc.Files.Create(driveFile).Media(file).Fields("id,webViewLink,name").Do()
	if err != nil {
		return GoogleDriveUploadResult{}, err
	}
	return googleDriveUploadResult(created), nil
}

func (g *GoogleService) UploadCSVAsSheet(ctx context.Context, csvPath, title string) (GoogleDriveUploadResult, error) {
	client, settings, err := g.client(ctx)
	if err != nil {
		return GoogleDriveUploadResult{}, err
	}

	driveSvc, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return GoogleDriveUploadResult{}, err
	}

	file, err := os.Open(csvPath)
	if err != nil {
		return GoogleDriveUploadResult{}, err
	}
	defer file.Close()

	uploadName := googleSheetUploadName(title, csvPath)
	driveFile := &drive.File{
		Name:     uploadName,
		MimeType: "application/vnd.google-apps.spreadsheet",
	}
	if folderID, err := resolveGoogleDriveFolder(ctx, driveSvc, settings.DriveFolderID); err != nil {
		return GoogleDriveUploadResult{}, err
	} else if folderID != "" {
		driveFile.Parents = []string{folderID}
	}

	created, err := driveSvc.Files.Create(driveFile).
		Media(file, googleapi.ContentType("text/csv")).
		Fields("id,webViewLink,name,mimeType").
		Do()
	if err != nil {
		return GoogleDriveUploadResult{}, err
	}
	return googleDriveUploadResult(created), nil
}

func resolveGoogleDriveFolder(ctx context.Context, driveSvc *drive.Service, folderTarget string) (string, error) {
	folderTarget = strings.TrimSpace(folderTarget)
	if folderTarget == "" {
		return "", nil
	}

	folder, err := driveSvc.Files.Get(folderTarget).Fields("id,mimeType,trashed").Context(ctx).Do()
	if err == nil {
		if folder.MimeType != "application/vnd.google-apps.folder" {
			return "", fmt.Errorf("google drive target is not a folder: %s", folderTarget)
		}
		if folder.Trashed {
			return "", fmt.Errorf("google drive folder is in trash: %s", folderTarget)
		}
		return folder.Id, nil
	}
	if apiErr, ok := err.(*googleapi.Error); !ok || apiErr.Code != http.StatusNotFound {
		return "", err
	}

	query := fmt.Sprintf("mimeType = 'application/vnd.google-apps.folder' and trashed = false and name = '%s'", escapeGoogleDriveQueryValue(folderTarget))
	list, err := driveSvc.Files.List().Q(query).Fields("files(id,name)").PageSize(1).Context(ctx).Do()
	if err != nil {
		return "", err
	}
	if len(list.Files) > 0 && strings.TrimSpace(list.Files[0].Id) != "" {
		return list.Files[0].Id, nil
	}

	created, err := driveSvc.Files.Create(&drive.File{
		Name:     folderTarget,
		MimeType: "application/vnd.google-apps.folder",
	}).Fields("id").Context(ctx).Do()
	if err != nil {
		return "", err
	}
	return created.Id, nil
}

func escapeGoogleDriveQueryValue(value string) string {
	return strings.ReplaceAll(value, "'", "\\'")
}

func googleSheetUploadName(title, csvPath string) string {
	name := strings.TrimSpace(title)
	if name == "" {
		name = strings.TrimSpace(strings.TrimSuffix(filepath.Base(csvPath), filepath.Ext(csvPath)))
	}
	if name == "" {
		name = "DixieData Export"
	}
	return name
}

func googleDriveUploadResult(file *drive.File) GoogleDriveUploadResult {
	if file == nil {
		return GoogleDriveUploadResult{}
	}
	webViewLink := strings.TrimSpace(file.WebViewLink)
	if webViewLink == "" && strings.TrimSpace(file.Id) != "" {
		if file.MimeType == "application/vnd.google-apps.spreadsheet" {
			webViewLink = fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/edit", file.Id)
		} else {
			webViewLink = fmt.Sprintf("https://drive.google.com/file/d/%s/view", file.Id)
		}
	}
	return GoogleDriveUploadResult{
		FileID:      file.Id,
		WebViewLink: webViewLink,
		Name:        file.Name,
	}
}

func (g *GoogleService) SyncCalendar(ctx context.Context, settings models.GoogleSettings, soldiers []models.Soldier) (GoogleCalendarSyncResult, error) {
	client, _, err := g.client(ctx)
	if err != nil {
		return GoogleCalendarSyncResult{}, err
	}
	calSvc, err := gcal.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return GoogleCalendarSyncResult{}, err
	}

	syncState, err := g.loadCalendarSyncState()
	if err != nil {
		return GoogleCalendarSyncResult{}, err
	}
	if syncState.EventIDs == nil {
		syncState.EventIDs = map[string]string{}
	}

	result := GoogleCalendarSyncResult{}
	active := make(map[string]struct{})
	calendarID := settings.CalendarID
	if strings.TrimSpace(calendarID) == "" {
		calendarID = "primary"
	}

	for _, soldier := range soldiers {
		if soldier.DeathMonth < 1 || soldier.DeathDay < 1 {
			result.Skipped++
			continue
		}
		syncKey := googleCalendarSyncKey(soldier)
		active[syncKey] = struct{}{}
		event := googleCalendarEvent(soldier)
		existingKey, existingID := googleCalendarEventID(syncState.EventIDs, soldier)
		if existingID != "" {
			if existingKey != syncKey {
				delete(syncState.EventIDs, existingKey)
				syncState.EventIDs[syncKey] = existingID
			}
			_, err := calSvc.Events.Update(calendarID, existingID, event).Do()
			if err == nil {
				result.Updated++
				continue
			}
			if apiErr, ok := err.(*googleapi.Error); !ok || apiErr.Code != http.StatusNotFound {
				return GoogleCalendarSyncResult{}, err
			}
		}

		created, err := calSvc.Events.Insert(calendarID, event).Do()
		if err != nil {
			return GoogleCalendarSyncResult{}, err
		}
		syncState.EventIDs[syncKey] = created.Id
		result.Created++
	}

	for displayID, eventID := range syncState.EventIDs {
		if _, ok := active[displayID]; ok {
			continue
		}
		if err := calSvc.Events.Delete(calendarID, eventID).Do(); err != nil {
			if apiErr, ok := err.(*googleapi.Error); !ok || apiErr.Code != http.StatusNotFound {
				return GoogleCalendarSyncResult{}, err
			}
		}
		delete(syncState.EventIDs, displayID)
		result.Deleted++
	}

	if err := g.saveCalendarSyncState(syncState); err != nil {
		return GoogleCalendarSyncResult{}, err
	}
	return result, nil
}

func (g *GoogleService) UnsyncCalendar(ctx context.Context) (GoogleCalendarUnsyncResult, error) {
	client, settings, err := g.client(ctx)
	if err != nil {
		return GoogleCalendarUnsyncResult{}, err
	}
	calSvc, err := gcal.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return GoogleCalendarUnsyncResult{}, err
	}
	syncState, err := g.loadCalendarSyncState()
	if err != nil {
		return GoogleCalendarUnsyncResult{}, err
	}
	calendarID := settings.CalendarID
	if strings.TrimSpace(calendarID) == "" {
		calendarID = "primary"
	}

	result := GoogleCalendarUnsyncResult{}
	deletedEventIDs := map[string]struct{}{}

	for _, collect := range []func(context.Context, *gcal.Service, string) ([]string, error){
		func(ctx context.Context, calSvc *gcal.Service, calendarID string) ([]string, error) {
			return collectGoogleCalendarEventIDsForUnsync(ctx, calSvc, calendarID, func(call *gcal.EventsListCall) *gcal.EventsListCall {
				return call.PrivateExtendedProperty("dixiedata=true")
			})
		},
		func(ctx context.Context, calSvc *gcal.Service, calendarID string) ([]string, error) {
			return collectGoogleCalendarEventIDsForUnsync(ctx, calSvc, calendarID, func(call *gcal.EventsListCall) *gcal.EventsListCall {
				return call.Q("DixieData Anniversary:")
			})
		},
	} {
		eventIDs, err := collect(ctx, calSvc, calendarID)
		if err != nil {
			return GoogleCalendarUnsyncResult{}, err
		}
		for _, eventID := range eventIDs {
			if _, seen := deletedEventIDs[eventID]; seen || strings.TrimSpace(eventID) == "" {
				continue
			}
			if err := deleteGoogleCalendarEvent(ctx, calSvc, calendarID, eventID); err != nil {
				return GoogleCalendarUnsyncResult{}, err
			}
			deletedEventIDs[eventID] = struct{}{}
			result.Deleted++
		}
	}

	for displayID, eventID := range syncState.EventIDs {
		if _, seen := deletedEventIDs[eventID]; !seen && strings.TrimSpace(eventID) != "" {
			if err := deleteGoogleCalendarEvent(ctx, calSvc, calendarID, eventID); err != nil {
				return GoogleCalendarUnsyncResult{}, err
			}
			deletedEventIDs[eventID] = struct{}{}
			result.Deleted++
		}
		delete(syncState.EventIDs, displayID)
	}

	if err := g.saveCalendarSyncState(syncState); err != nil {
		return GoogleCalendarUnsyncResult{}, err
	}
	return result, nil
}

func collectGoogleCalendarEventIDsForUnsync(ctx context.Context, calSvc *gcal.Service, calendarID string, configure func(*gcal.EventsListCall) *gcal.EventsListCall) ([]string, error) {
	var eventIDs []string
	pageToken := ""
	for {
		var events *gcal.Events
		err := googleRateLimitRetry(ctx, func() error {
			call := calSvc.Events.List(calendarID).
				ShowDeleted(false).
				SingleEvents(false).
				Fields("items(id),nextPageToken")
			call = configure(call)
			if strings.TrimSpace(pageToken) != "" {
				call = call.PageToken(pageToken)
			}
			var err error
			events, err = call.Do()
			return err
		})
		if err != nil {
			return nil, err
		}
		for _, event := range events.Items {
			if event != nil && strings.TrimSpace(event.Id) != "" {
				eventIDs = append(eventIDs, event.Id)
			}
		}
		if strings.TrimSpace(events.NextPageToken) == "" {
			return eventIDs, nil
		}
		pageToken = events.NextPageToken
	}
}

func deleteGoogleCalendarEvent(ctx context.Context, calSvc *gcal.Service, calendarID, eventID string) error {
	if err := googleRateLimitRetry(ctx, func() error {
		return calSvc.Events.Delete(calendarID, eventID).Do()
	}); err != nil {
		if apiErr, ok := err.(*googleapi.Error); !ok || (apiErr.Code != http.StatusNotFound && apiErr.Code != http.StatusGone) {
			return err
		}
	}
	return nil
}

func googleRateLimitRetry(ctx context.Context, fn func() error) error {
	var err error
	for attempt := 0; attempt < 6; attempt++ {
		err = fn()
		if err == nil || !isGoogleRateLimitError(err) {
			return err
		}
		if attempt == 5 {
			return err
		}
		wait := googleRetryDelay(err, attempt)
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return err
}

func isGoogleRateLimitError(err error) bool {
	apiErr, ok := err.(*googleapi.Error)
	if !ok {
		return false
	}
	if apiErr.Code == http.StatusTooManyRequests {
		return true
	}
	if apiErr.Code != http.StatusForbidden {
		return false
	}
	for _, item := range apiErr.Errors {
		if item.Reason == "rateLimitExceeded" || item.Reason == "userRateLimitExceeded" {
			return true
		}
	}
	return false
}

func googleRetryDelay(err error, attempt int) time.Duration {
	apiErr, ok := err.(*googleapi.Error)
	if ok && apiErr.Header != nil {
		if retryAfter := strings.TrimSpace(apiErr.Header.Get("Retry-After")); retryAfter != "" {
			if seconds, parseErr := strconv.Atoi(retryAfter); parseErr == nil && seconds > 0 {
				return time.Duration(seconds) * time.Second
			}
		}
	}
	delay := time.Second << attempt
	if delay > 15*time.Second {
		return 15 * time.Second
	}
	return delay
}

func (g *GoogleService) client(ctx context.Context) (*http.Client, models.GoogleSettings, error) {
	settings, _, _, _, err := g.LoadEffectiveSettings()
	if err != nil {
		return nil, models.GoogleSettings{}, err
	}
	token, err := g.loadToken()
	if err != nil {
		return nil, models.GoogleSettings{}, err
	}
	if token == nil {
		return nil, models.GoogleSettings{}, fmt.Errorf("google account is not connected")
	}

	config := g.oauthConfig(settings, "http://127.0.0.1")
	tokenSource := config.TokenSource(ctx, token)
	refreshed, err := tokenSource.Token()
	if err != nil {
		return nil, models.GoogleSettings{}, err
	}
	if refreshed.AccessToken != token.AccessToken || refreshed.Expiry != token.Expiry {
		if err := g.saveToken(refreshed); err != nil {
			return nil, models.GoogleSettings{}, err
		}
	}
	return oauth2.NewClient(ctx, tokenSource), settings, nil
}

func (g *GoogleService) oauthConfig(settings models.GoogleSettings, redirectURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     strings.TrimSpace(settings.ClientID),
		ClientSecret: strings.TrimSpace(settings.ClientSecret),
		RedirectURL:  redirectURL,
		Scopes: []string{
			drive.DriveFileScope,
			gcal.CalendarScope,
		},
		Endpoint: google.Endpoint,
	}
}

func (g *GoogleService) loadSharedSettings() (models.GoogleSettings, string, bool, error) {
	if envSettings, ok := loadGoogleSettingsFromEnv(); ok {
		if strings.TrimSpace(envSettings.CalendarID) == "" {
			envSettings.CalendarID = "primary"
		}
		return envSettings, "DIXIEDATA_GOOGLE_* environment variables", true, nil
	}
	for _, path := range googleDefaultsCandidatePaths() {
		if path == "" {
			continue
		}
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return models.GoogleSettings{}, "", false, err
		}
		var settings models.GoogleSettings
		if err := readJSONFile(path, &settings); err != nil {
			return models.GoogleSettings{}, "", false, err
		}
		if strings.TrimSpace(settings.CalendarID) == "" {
			settings.CalendarID = "primary"
		}
		if strings.TrimSpace(settings.ClientID) == "" || strings.TrimSpace(settings.ClientSecret) == "" {
			return models.GoogleSettings{}, "", false, fmt.Errorf("%s must include client_id and client_secret", path)
		}
		return settings, path, true, nil
	}
	return models.GoogleSettings{CalendarID: "primary"}, "", false, nil
}

func loadGoogleSettingsFromEnv() (models.GoogleSettings, bool) {
	clientID := strings.TrimSpace(os.Getenv("DIXIEDATA_GOOGLE_CLIENT_ID"))
	clientSecret := strings.TrimSpace(os.Getenv("DIXIEDATA_GOOGLE_CLIENT_SECRET"))
	if clientID == "" || clientSecret == "" {
		return models.GoogleSettings{}, false
	}
	settings := models.GoogleSettings{
		ClientID:      clientID,
		ClientSecret:  clientSecret,
		CalendarID:    strings.TrimSpace(os.Getenv("DIXIEDATA_GOOGLE_CALENDAR_ID")),
		DriveFolderID: strings.TrimSpace(os.Getenv("DIXIEDATA_GOOGLE_DRIVE_FOLDER_ID")),
	}
	if strings.TrimSpace(settings.CalendarID) == "" {
		settings.CalendarID = "primary"
	}
	return settings, true
}

func googleDefaultsCandidatePaths() []string {
	paths := []string{}
	if exePath, err := os.Executable(); err == nil {
		paths = append(paths, filepath.Join(filepath.Dir(exePath), googleDefaultsFile))
	}
	if wd, err := os.Getwd(); err == nil {
		paths = append(paths, filepath.Join(wd, googleDefaultsFile))
	}
	return paths
}

func mergeGoogleSettings(base, override models.GoogleSettings) models.GoogleSettings {
	merged := base
	if strings.TrimSpace(override.ClientID) != "" {
		merged.ClientID = override.ClientID
	}
	if strings.TrimSpace(override.ClientSecret) != "" {
		merged.ClientSecret = override.ClientSecret
	}
	if strings.TrimSpace(override.CalendarID) != "" {
		merged.CalendarID = override.CalendarID
	}
	if strings.TrimSpace(override.DriveFolderID) != "" {
		merged.DriveFolderID = override.DriveFolderID
	}
	return merged
}

func (g *GoogleService) loadToken() (*oauth2.Token, error) {
	path := filepath.Join(g.dataDir, googleTokenFile)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var token oauth2.Token
	if err := readJSONFile(path, &token); err != nil {
		return nil, err
	}
	return &token, nil
}

func (g *GoogleService) saveToken(token *oauth2.Token) error {
	return writeJSONFile(filepath.Join(g.dataDir, googleTokenFile), token)
}

func (g *GoogleService) loadCalendarSyncState() (GoogleCalendarSyncState, error) {
	path := filepath.Join(g.dataDir, googleCalendarSyncFile)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return GoogleCalendarSyncState{EventIDs: map[string]string{}}, nil
		}
		return GoogleCalendarSyncState{}, err
	}
	var state GoogleCalendarSyncState
	if err := readJSONFile(path, &state); err != nil {
		return GoogleCalendarSyncState{}, err
	}
	if state.EventIDs == nil {
		state.EventIDs = map[string]string{}
	}
	return state, nil
}

func (g *GoogleService) saveCalendarSyncState(state GoogleCalendarSyncState) error {
	return writeJSONFile(filepath.Join(g.dataDir, googleCalendarSyncFile), state)
}

func googleCalendarSyncKey(soldier models.Soldier) string {
	if strings.TrimSpace(soldier.SyncID) != "" {
		return strings.TrimSpace(soldier.SyncID)
	}
	return strings.TrimSpace(soldier.DisplayID)
}

func googleCalendarEventID(eventIDs map[string]string, soldier models.Soldier) (string, string) {
	seen := map[string]struct{}{}
	for _, key := range []string{googleCalendarSyncKey(soldier), strings.TrimSpace(soldier.DisplayID), legacyDisplayID(strings.TrimSpace(soldier.DisplayID))} {
		if strings.TrimSpace(key) == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if eventID := eventIDs[key]; strings.TrimSpace(eventID) != "" {
			return key, eventID
		}
	}
	return "", ""
}

func legacyDisplayID(displayID string) string {
	index := strings.Index(displayID, "-")
	if index <= 0 || index >= len(displayID)-1 {
		return ""
	}
	return displayID[index+1:]
}

func googleCalendarEvent(soldier models.Soldier) *gcal.Event {
	start := nextGoogleAnniversaryDate(soldier, time.Now()).In(time.Local)
	start = time.Date(start.Year(), start.Month(), start.Day(), 9, 0, 0, 0, time.Local)
	end := start.Add(time.Hour)

	description := fmt.Sprintf("Database Number: %s\nUnit: %s\nBuried In: %s\nOriginal Death Date: %s\nGenerated by DixieData.", soldier.DisplayID, soldier.Unit, soldier.BuriedIn, soldierDeathLine(soldier))
	return &gcal.Event{
		Summary:     fmt.Sprintf("DixieData Anniversary: %s", googleSoldierDisplayName(soldier)),
		Description: description,
		Start:       &gcal.EventDateTime{DateTime: start.Format(time.RFC3339)},
		End:         &gcal.EventDateTime{DateTime: end.Format(time.RFC3339)},
		Recurrence:  []string{"RRULE:FREQ=YEARLY"},
		Reminders: &gcal.EventReminders{
			UseDefault: false,
			Overrides: []*gcal.EventReminder{
				{Method: "popup", Minutes: 24 * 60},
				{Method: "popup", Minutes: 60},
			},
		},
		ExtendedProperties: &gcal.EventExtendedProperties{
			Private: map[string]string{
				"dixiedata_display_id": soldier.DisplayID,
				"dixiedata_sync_id":    soldier.SyncID,
				"dixiedata":            "true",
			},
		},
	}
}

func nextGoogleAnniversaryDate(soldier models.Soldier, now time.Time) time.Time {
	base := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	for i := 0; i < 8; i++ {
		year := now.Year() + i
		candidate := time.Date(year, time.Month(soldier.DeathMonth), soldier.DeathDay, 0, 0, 0, 0, time.Local)
		if candidate.Month() != time.Month(soldier.DeathMonth) || candidate.Day() != soldier.DeathDay {
			continue
		}
		if !candidate.Before(base) {
			return candidate
		}
	}
	return time.Date(now.Year(), time.Month(soldier.DeathMonth), soldier.DeathDay, 0, 0, 0, 0, time.Local)
}

func googleSoldierDisplayName(soldier models.Soldier) string {
	name := strings.TrimSpace(strings.Join([]string{strings.TrimSpace(soldier.Rank), strings.TrimSpace(soldier.FirstName), strings.TrimSpace(soldier.LastName)}, " "))
	if name == "" {
		return soldier.DisplayID
	}
	return name
}

func randomOAuthState() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func readJSONFile(path string, target interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func writeJSONFile(path string, value interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
