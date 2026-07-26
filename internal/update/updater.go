// Package update checks, downloads, applies, and rolls back DixieData application updates.
package update

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/valueforvalue/DixieData/internal/appdata"
	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/debug"
)

const (
	updateSourceConfigKey = "update_source_url"
	defaultSourceURL      = "https://api.github.com/repos/valueforvalue/DixieData/releases/latest"
	maxUpdateSourceBytes  = 2 << 20
)

// ApplyPhase identifies which stage of the in-place update
// pipeline a progress callback is reporting. The UI switches on
// Phase to decide which label + progress-bar shape to render
// (issue #661). Phase strings are intentionally short + distinct
// so the templ/JS renderers can match them without ambiguity.
type ApplyPhase string

const (
	PhaseIdle         ApplyPhase = "idle"
	PhaseDownload     ApplyPhase = "downloading"
	PhaseVerify       ApplyPhase = "verifying"
	PhaseExtract      ApplyPhase = "extracting"
	PhaseRestorePoint ApplyPhase = "creating-restore-point"
	PhaseApplyStarted ApplyPhase = "applying"
	PhaseError        ApplyPhase = "error"
)

// UpdateProgress is the payload passed to the in-place update
// progress callback. The callback fires at least once per phase
// transition and at least once per network chunk during the
// download (subject to network buffering). TotalBytes is -1
// when the server did not send a Content-Length header (chunked
// or unknown-length response); BytesDownloaded is always
// monotonically non-decreasing within a single prepare pipeline.
type UpdateProgress struct {
	Phase           ApplyPhase
	BytesDownloaded int64
	TotalBytes      int64
	Message         string
	Error           string
}

var versionPattern = regexp.MustCompile(`(?i)v?(\d+)\.(\d+)\.(\d+)`)

type configStore interface {
	SystemConfig(key string) (string, error)
	SetSystemConfig(key, value string) error
}

type Service struct {
	config          configStore
	dataDir         string
	restorePoints   *RestorePointManager
	archiveWriter   RestorePointArchiveWriter
	client          *http.Client
	executablePath  func() (string, error)
	now             func() time.Time
	// sourceURLOverride carries the user-set update source URL
	// after the appshell one-shot migration has moved the value
	// from `system_config.update_source_url` into the external
	// `config.json`. Set via SetSourceURL (see issue #660
	// amendment #1). When non-empty, sourceSettings() returns
	// this value instead of reading from system_config. The
	// SaveSource method writes through to system_config AND
	// this override so future boots continue to see the value
	// even if config.json is lost.
	sourceURLOverride string
	// checkURLOverride carries the configured default update
	// check URL (config.Services.UpdateCheckURL). When
	// non-empty, sourceSettings() returns it instead of the
	// hardcoded `defaultSourceURL` constant. Set via
	// SetCheckURL from appshell.reloadServices.
	checkURLOverride string
}

type SettingsState struct {
	CurrentVersion     string
	BuildIdentity      string
	SourceURL          string
	EffectiveSourceURL string
	UsingDefaultSource bool
	CanApply           bool
	DisabledReason     string
	LastApply          *ApplyStatus
	NoticeMessage      string
	NoticeKind         string
}

type ApplyStatus struct {
	Status    string `json:"status"`
	Version   string `json:"version"`
	Message   string `json:"message"`
	AppliedAt string `json:"applied_at"`
}

type CheckResult struct {
	CurrentVersion   string
	AvailableVersion string
	UpdateAvailable  bool
	// NeedsReinstall is true when the latest release has a
	// different update-flow version (U) than the installed
	// binary (issue #266). The user must reinstall; the
	// in-place update flow can't safely apply. Off when U
	// matches.
	NeedsReinstall  bool
	DownloadURL     string
	NotesURL        string
	ReleaseNotes    string
	PublishedAt     string
	SourceLabel     string
	CanApply        bool
	DisabledReason  string
}

type PreparedUpdate struct {
	Version    string
	ScriptPath string
}

type resolvedRelease struct {
	version      string
	downloadURL  string
	notesURL     string
	releaseNotes string
	publishedAt  string
	checksumSHA  string
	assetKind    string
	sourceLabel  string
}

type githubRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	HTMLURL     string `json:"html_url"`
	Body        string `json:"body"`
	PublishedAt string `json:"published_at"`
	Assets      []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
		ContentType        string `json:"content_type"`
	} `json:"assets"`
}

type updateManifest struct {
	Version      string `json:"version"`
	AssetURL     string `json:"asset_url"`
	DownloadURL  string `json:"download_url"`
	URL          string `json:"url"`
	SHA256       string `json:"sha256"`
	AssetSHA256  string `json:"asset_sha256"`
	ChecksumSHA  string `json:"checksum_sha256"`
	NotesURL     string `json:"notes_url"`
	HTMLURL      string `json:"html_url"`
	ReleaseNotes string `json:"release_notes"`
	Body         string `json:"body"`
	PublishedAt  string `json:"published_at"`
}

func NewService(config configStore, dataDir string, archiveWriter RestorePointArchiveWriter) *Service {
	service := &Service{
		config:        config,
		dataDir:       dataDir,
		restorePoints: NewRestorePointManager(dataDir),
		archiveWriter: archiveWriter,
		client: &http.Client{
			Timeout: 45 * time.Second,
		},
		executablePath: os.Executable,
		now:            time.Now,
	}
	service.restorePoints.now = func() time.Time {
		return service.now()
	}
	return service
}

// SetHTTPTimeout replaces the HTTP client timeout used for update
// checks. Call after NewService if the timeout should be read from
// app config (issue #636).
func (s *Service) SetHTTPTimeout(d time.Duration) {
	s.client.Timeout = d
}

func (s *Service) Settings() (SettingsState, error) {
	rawURL, effectiveURL, usingDefault, err := s.sourceSettings()
	if err != nil {
		return SettingsState{}, err
	}
	executablePath, err := s.executablePath()
	if err != nil {
		return SettingsState{}, err
	}
	canApply, disabledReason := updateEligibility(executablePath)
	return SettingsState{
		CurrentVersion:     buildinfo.AppVersion,
		BuildIdentity:      buildinfo.BuildIdentity(),
		SourceURL:          rawURL,
		EffectiveSourceURL: effectiveURL,
		UsingDefaultSource: usingDefault,
		CanApply:           canApply,
		DisabledReason:     disabledReason,
		LastApply:          s.loadApplyStatus(),
	}, nil
}

func (s *Service) SaveSource(rawURL string) (SettingsState, error) {
	normalized, err := normalizeSourceURL(rawURL)
	if err != nil {
		return SettingsState{}, err
	}
	// Issue #660 amendment #1: the user-set update source URL
	// now lives in `config.Services.UpdateSourceURL` (which
	// survives .ddbak imports) instead of the SQLite
	// `system_config` table. The appshell calls config.Save
	// with the new value + reloadServices, which calls
	// SetSourceURL on the Service. SaveSource updates the
	// override directly as a back-compat path for tests +
	// any callers that don't go through reloadServices; the
	// appshell handler does the canonical write through
	// config.json.
	s.sourceURLOverride = normalized
	return s.Settings()
}

func (s *Service) Check() (CheckResult, error) {
	release, err := s.resolveRelease()
	if err != nil {
		return CheckResult{}, err
	}
	settings, err := s.Settings()
	if err != nil {
		return CheckResult{}, err
	}
	comparison, err := compareVersions(release.version, settings.CurrentVersion)
	if err != nil {
		return CheckResult{}, err
	}
	// Update is offered only when the release is newer AND the
	// installed binary's U matches the release's U. A U
	// mismatch surfaces as updateOffered=false (the UI then
	// flips NeedsReinstall=true via the NeedsReinstall field,
	// which is set in the caller when !Compatible). See
	// issue #266 for the rule rationale.
	updateOffered := comparison.Newer && comparison.Compatible
	return CheckResult{
		CurrentVersion:   settings.CurrentVersion,
		AvailableVersion: release.version,
		UpdateAvailable:  updateOffered,
		NeedsReinstall:   !comparison.Compatible,
		DownloadURL:      release.downloadURL,
		NotesURL:         release.notesURL,
		ReleaseNotes:     release.releaseNotes,
		PublishedAt:      release.publishedAt,
		SourceLabel:      release.sourceLabel,
		CanApply:         settings.CanApply,
		DisabledReason:   settings.DisabledReason,
	}, nil
}

func (s *Service) PrepareLatest() (PreparedUpdate, error) {
	return s.PrepareLatestWithProgress(nil)
}

// PrepareLatestWithProgress is the progress-reporting variant of
// PrepareLatest (issue #661). The callback fires at each phase
// boundary (download, verify, extract, restore-point, applying)
// and per-chunk during the download. Pass nil for the no-op
// path (PrepareLatest delegates here).
func (s *Service) PrepareLatestWithProgress(progress func(UpdateProgress)) (PreparedUpdate, error) {
	settings, err := s.Settings()
	if err != nil {
		s.failPrepare(progress, "could not load settings: %v", err)
		return PreparedUpdate{}, err
	}
	if !settings.CanApply {
		s.failPrepare(progress, settings.DisabledReason, nil)
		return PreparedUpdate{}, errors.New(settings.DisabledReason)
	}
	release, err := s.resolveRelease()
	if err != nil {
		s.failPrepare(progress, "could not resolve latest release: %v", err)
		return PreparedUpdate{}, err
	}
	comparison, err := compareVersions(release.version, settings.CurrentVersion)
	if err != nil {
		s.failPrepare(progress, "could not compare versions: %v", err)
		return PreparedUpdate{}, err
	}
	if !comparison.Newer || !comparison.Compatible {
		s.failPrepare(progress, "no newer update is available", nil)
		return PreparedUpdate{}, fmt.Errorf("no newer update is available")
	}

	executablePath, err := s.executablePath()
	if err != nil {
		s.failPrepare(progress, "could not locate the executable: %v", err)
		return PreparedUpdate{}, err
	}
	executableName := filepath.Base(executablePath)
	installDir := filepath.Dir(executablePath)

	workRoot := filepath.Join(appdata.UpdateDownloadsDir(s.dataDir), "update-"+s.now().UTC().Format("20060102T150405"))
	stageRoot := filepath.Join(workRoot, "stage")
	if err := os.MkdirAll(stageRoot, 0o755); err != nil {
		s.failPrepare(progress, "could not create staging dir: %v", err)
		return PreparedUpdate{}, err
	}

	artifactName := downloadFileName(release.downloadURL, release.assetKind)
	artifactPath := filepath.Join(workRoot, artifactName)
	if err := s.downloadFileWithProgress(release.downloadURL, artifactPath, progress); err != nil {
		return PreparedUpdate{}, err
	}
	if strings.TrimSpace(release.checksumSHA) != "" {
		s.emitPhase(progress, PhaseVerify, "Verifying checksum…")
		if err := verifyFileChecksum(artifactPath, release.checksumSHA); err != nil {
			s.failPrepare(progress, "checksum verification failed: %v", err)
			return PreparedUpdate{}, err
		}
	}

	switch release.assetKind {
	case "zip":
		s.emitPhase(progress, PhaseExtract, "Extracting update…")
		if err := extractZip(artifactPath, stageRoot); err != nil {
			s.failPrepare(progress, "extract failed: %v", err)
			return PreparedUpdate{}, err
		}
		stageRoot, err = normalizeStageRoot(stageRoot, executableName)
		if err != nil {
			s.failPrepare(progress, "stage normalization failed: %v", err)
			return PreparedUpdate{}, err
		}
	case "exe":
		s.emitPhase(progress, PhaseExtract, "Copying update…")
		targetExe := filepath.Join(stageRoot, executableName)
		if err := copyFile(artifactPath, targetExe); err != nil {
			s.failPrepare(progress, "copy failed: %v", err)
			return PreparedUpdate{}, err
		}
	default:
		s.failPrepare(progress, "unsupported update asset type", nil)
		return PreparedUpdate{}, fmt.Errorf("unsupported update asset type")
	}

	if _, err := os.Stat(filepath.Join(stageRoot, executableName)); err != nil {
		s.failPrepare(progress, "staged update is missing %s", err)
		return PreparedUpdate{}, fmt.Errorf("staged update is missing %s", executableName)
	}

	restorePoint, err := s.restorePoints.Create(CreateRestorePointInput{
		SourceAppVersion:    settings.CurrentVersion,
		TargetAppVersion:    release.version,
		SourceBuildIdentity: settings.BuildIdentity,
		TargetBuildIdentity: "",
	}, s.archiveWriter, func(outputDir string) error {
		return snapshotInstalledBuild(installDir, s.dataDir, outputDir)
	})
	if err != nil {
		s.failPrepare(progress, "create restore point: %v", err)
		return PreparedUpdate{}, fmt.Errorf("create restore point: %w", err)
	}
	s.emitPhase(progress, PhaseRestorePoint, "Creating restore point…")
	if err := s.restorePoints.SaveLaunchState(restorePoint); err != nil {
		s.failPrepare(progress, "write restore point launch state: %v", err)
		return PreparedUpdate{}, fmt.Errorf("write restore point launch state: %w", err)
	}

	resultPath := appdata.UpdateApplyResultPath(s.dataDir)
	if err := os.MkdirAll(filepath.Dir(resultPath), 0o755); err != nil {
		s.failPrepare(progress, "could not create result dir: %v", err)
		return PreparedUpdate{}, err
	}
	scriptPath := filepath.Join(workRoot, "apply-update.ps1")
	if err := writeApplyScript(scriptPath, applyScriptOptions{
		ProcessID:          os.Getpid(),
		StageDir:           stageRoot,
		InstallDir:         installDir,
		ExecutableName:     executableName,
		ResultPath:         resultPath,
		LaunchStatePath:    appdata.UpdateRestorePointStatePath(s.dataDir),
		TargetVersion:      release.version,
		SourceVersion:      settings.CurrentVersion,
		FeedbackLogPath:    appdata.FeedbackLogPath(s.dataDir),
		FeedbackArchiveDir: appdata.FeedbackLogArchiveDir(s.dataDir),
	}); err != nil {
		s.failPrepare(progress, "write apply script: %v", err)
		return PreparedUpdate{}, fmt.Errorf("write apply script: %w", err)
	}

	s.emitPhase(progress, PhaseApplyStarted, "Applying update…")
	return PreparedUpdate{
		Version:    release.version,
		ScriptPath: scriptPath,
	}, nil
}

// emitPhase is a small helper that fires the progress callback
// with the given phase + message. No-op when the callback is
// nil (the PrepareLatest back-compat path).
func (s *Service) emitPhase(progress func(UpdateProgress), phase ApplyPhase, message string) {
	if progress == nil {
		return
	}
	progress(UpdateProgress{Phase: phase, Message: message})
}

// failPrepare fires the progress callback with PhaseError and
// a formatted message. Returns nothing — callers still
// propagate the original error from the same `return` that
// called failPrepare. The formatted message is computed via
// fmt.Sprintf so the template is safe even when the
// underlying error is nil (the %v branch returns "<nil>").
func (s *Service) failPrepare(progress func(UpdateProgress), format string, err error) {
	if progress == nil {
		return
	}
	msg := fmt.Sprintf(format, err)
	progress(UpdateProgress{Phase: PhaseError, Message: msg, Error: msg})
}

func (s *Service) sourceSettings() (string, string, bool, error) {
	override := strings.TrimSpace(s.sourceURLOverride)
	if override != "" {
		return override, override, false, nil
	}
	value, err := s.config.SystemConfig(updateSourceConfigKey)
	if err != nil {
		return "", "", false, err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		// No user-set source; fall back to the configured
		// default (issue #660 audit gap #1). If the appshell
		// never wired the override (test path), use the
		// built-in defaultSourceURL constant.
		checkURL := strings.TrimSpace(s.checkURLOverride)
		if checkURL == "" {
			checkURL = defaultSourceURL
		}
		return "", checkURL, true, nil
	}
	return value, value, false, nil
}

// SetSourceURL updates the in-memory override so sourceSettings
// returns it on the next call (issue #660 amendment #1). The
// appshell calls this from reloadServices after the one-shot
// migration copies the legacy system_config row into
// config.Services.UpdateSourceURL. Does NOT touch the
// system_config row directly — SaveSource does that.
func (s *Service) SetSourceURL(rawURL string) {
	s.sourceURLOverride = strings.TrimSpace(rawURL)
}

// SetCheckURL updates the in-memory override for the bundled
// update check URL (issue #660 audit gap #1). When non-empty,
// sourceSettings() returns it instead of the hardcoded
// `defaultSourceURL` constant. Lets a packager point the app
// at a fork's release feed without recompiling.
func (s *Service) SetCheckURL(rawURL string) {
	s.checkURLOverride = strings.TrimSpace(rawURL)
}

func (s *Service) resolveRelease() (resolvedRelease, error) {
	_, effectiveURL, _, err := s.sourceSettings()
	if err != nil {
		return resolvedRelease{}, err
	}
	if isDirectAssetURL(effectiveURL) {
		return directReleaseFromURL(effectiveURL)
	}
	request, err := http.NewRequest(http.MethodGet, effectiveURL, nil)
	if err != nil {
		return resolvedRelease{}, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", buildinfo.AppName+"-updater/"+buildinfo.AppVersion)

	response, err := s.client.Do(request)
	if err != nil {
		return resolvedRelease{}, err
	}
	defer debug.DeferCloseLog(response.Body, "resolveRelease.response")
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return resolvedRelease{}, fmt.Errorf("update source returned %s", response.Status)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxUpdateSourceBytes))
	if err != nil {
		return resolvedRelease{}, err
	}
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return resolvedRelease{}, fmt.Errorf("update source returned an empty response")
	}

	release, err := githubReleaseFromJSON(body)
	if err == nil {
		release.sourceLabel = "GitHub latest release"
		return release, nil
	}
	release, err = manifestReleaseFromJSON(body, effectiveURL)
	if err == nil {
		release.sourceLabel = "Custom update manifest"
		return release, nil
	}
	return resolvedRelease{}, fmt.Errorf("unsupported update source format")
}

func normalizeSourceURL(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid update source URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("update source URL must use http or https")
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return "", fmt.Errorf("invalid update source URL")
	}
	return parsed.String(), nil
}

func updateEligibility(executablePath string) (bool, string) {
	if appdata.IsDevelopmentBuild(executablePath) {
		return false, "Self-update is disabled for development builds running from build\\bin. Use scripts\\build-release.ps1 instead."
	}
	return true, ""
}

func directReleaseFromURL(rawURL string) (resolvedRelease, error) {
	version, err := versionFromString(rawURL)
	if err != nil {
		return resolvedRelease{}, fmt.Errorf("direct update URLs must include a version like 1.2.3 in the file name or path")
	}
	assetKind := assetKindFromURL(rawURL)
	if assetKind == "" {
		return resolvedRelease{}, fmt.Errorf("direct update URLs must point to a .zip or .exe file")
	}
	return resolvedRelease{
		version:     version,
		downloadURL: rawURL,
		assetKind:   assetKind,
		sourceLabel: "Direct update artifact",
	}, nil
}

func githubReleaseFromJSON(body []byte) (resolvedRelease, error) {
	var release githubRelease
	if err := json.Unmarshal(body, &release); err != nil {
		return resolvedRelease{}, err
	}
	if strings.TrimSpace(release.TagName) == "" && strings.TrimSpace(release.Name) == "" {
		return resolvedRelease{}, fmt.Errorf("not a GitHub release payload")
	}
	assetURL, assetKind, err := selectGitHubAsset(release.Assets)
	if err != nil {
		return resolvedRelease{}, err
	}
	version, err := versionFromString(strings.TrimSpace(release.TagName) + " " + strings.TrimSpace(release.Name))
	if err != nil {
		return resolvedRelease{}, fmt.Errorf("release tag does not include a supported version")
	}
	return resolvedRelease{
		version:      version,
		downloadURL:  assetURL,
		assetKind:    assetKind,
		notesURL:     strings.TrimSpace(release.HTMLURL),
		releaseNotes: strings.TrimSpace(release.Body),
		publishedAt:  strings.TrimSpace(release.PublishedAt),
	}, nil
}

func manifestReleaseFromJSON(body []byte, baseURL string) (resolvedRelease, error) {
	var manifest updateManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return resolvedRelease{}, err
	}
	downloadURL := firstNonEmpty(manifest.AssetURL, manifest.DownloadURL, manifest.URL)
	if strings.TrimSpace(downloadURL) == "" {
		return resolvedRelease{}, fmt.Errorf("manifest is missing asset_url")
	}
	version, err := versionFromString(manifest.Version)
	if err != nil {
		return resolvedRelease{}, fmt.Errorf("manifest version is invalid")
	}
	downloadURL, err = resolveRelativeURL(baseURL, downloadURL)
	if err != nil {
		return resolvedRelease{}, fmt.Errorf("manifest asset_url is invalid")
	}
	assetKind := assetKindFromURL(downloadURL)
	if assetKind == "" {
		return resolvedRelease{}, fmt.Errorf("manifest asset_url must point to a .zip or .exe file")
	}
	notesURL := firstNonEmpty(manifest.NotesURL, manifest.HTMLURL)
	if strings.TrimSpace(notesURL) != "" {
		notesURL, err = resolveRelativeURL(baseURL, notesURL)
		if err != nil {
			return resolvedRelease{}, fmt.Errorf("manifest notes_url is invalid")
		}
	}
	return resolvedRelease{
		version:      version,
		downloadURL:  downloadURL,
		assetKind:    assetKind,
		notesURL:     notesURL,
		releaseNotes: strings.TrimSpace(firstNonEmpty(manifest.ReleaseNotes, manifest.Body)),
		publishedAt:  strings.TrimSpace(manifest.PublishedAt),
		checksumSHA:  strings.TrimSpace(firstNonEmpty(manifest.SHA256, manifest.AssetSHA256, manifest.ChecksumSHA)),
	}, nil
}

func selectGitHubAsset(assets []struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	ContentType        string `json:"content_type"`
}) (string, string, error) {
	if len(assets) == 0 {
		return "", "", fmt.Errorf("release does not include any downloadable assets")
	}
	var zipURL, exeURL string
	for _, asset := range assets {
		name := strings.TrimSpace(asset.Name)
		downloadURL := strings.TrimSpace(asset.BrowserDownloadURL)
		if name == "" || downloadURL == "" {
			continue
		}
		kind := assetKindFromURL(name)
		if kind == "" {
			continue
		}
		lowerName := strings.ToLower(name)
		if strings.HasPrefix(lowerName, "dixiedata-release-") && strings.HasSuffix(lowerName, ".zip") {
			return downloadURL, "zip", nil
		}
		if kind == "zip" && zipURL == "" {
			zipURL = downloadURL
		}
		if strings.EqualFold(name, "DixieData.exe") {
			exeURL = downloadURL
		}
		if kind == "exe" && exeURL == "" {
			exeURL = downloadURL
		}
	}
	if zipURL != "" {
		return zipURL, "zip", nil
	}
	if exeURL != "" {
		return exeURL, "exe", nil
	}
	return "", "", fmt.Errorf("release does not include a .zip or .exe asset")
}

// CompareResult captures the outcome of compareVersions.
// Install-safe when Compatible is true AND the release is
// newer than the installed binary (higher N). Compatible=false
// means the user must reinstall (U mismatch in either direction).
// Newer=true means the release N is greater than installed N
// (only meaningful when Compatible=true; downgrades are
// rejected as !Compatible).
type CompareResult struct {
	Compatible bool
	Newer      bool
}

// compareVersions decides whether an installed binary can
// auto-update to a release.
//
// Inputs are version strings in either the new shape
// (v1.{U}.{N}) or the legacy shape (v1.2.{N}). Legacy strings
// parse to U=1 by default per issue #266 decision 1, so every
// release published before #266 maps cleanly to (U=1, N=N).
//
// Rules (per #266):
//   - U mismatch (release.U > installed.U OR release.U <
//     installed.U): the update-flow shape changed; the
//     installed binary can't safely apply the release. The
//     user must reinstall. (Q2 + Q4.)
//   - U match + N match: identical versions; not an update.
//   - U match + release.N > installed.N: auto-update path.
//   - U match + release.N < installed.N: downgrade reject
//     (the user is running a newer release than what's
//     distributed; treat as compatible but not newer so the
//     UI doesn't offer it; per Q4 the rule is symmetric).

func compareVersions(left, right string) (CompareResult, error) {
	leftParts, err := parseVersion(left)
	if err != nil {
		return CompareResult{}, err
	}
	rightParts, err := parseVersion(right)
	if err != nil {
		return CompareResult{}, err
	}
	// U mismatch in either direction forces a reinstall.
	if leftParts.updateFlow != rightParts.updateFlow {
		return CompareResult{Compatible: false, Newer: leftParts.release > rightParts.release}, nil
	}
	// U matches. N is the release counter; release > installed
	// = "newer" (user can auto-update). release == installed
	// = same build (not an update). release < installed
	// = downgrade (compatible but not newer, so the UI
	// doesn't surface it as an offer).
	if leftParts.release > rightParts.release {
		return CompareResult{Compatible: true, Newer: true}, nil
	}
	return CompareResult{Compatible: true, Newer: false}, nil
}

// parsedVersion captures the three numbers from a version
// string with semantic meaning baked in. Replaces the
// unnamed [3]int the previous walker used so the call sites
// read obviously and the rule application above stays
// readable.
type parsedVersion struct {
	major      int
	updateFlow int // U — gates auto-update
	release    int // N — release counter (independent of schema)
}

func parseVersion(value string) (parsedVersion, error) {
	normalized, err := versionFromString(value)
	if err != nil {
		return parsedVersion{}, err
	}
	parts := strings.Split(normalized, ".")
	if len(parts) != 3 {
		return parsedVersion{}, fmt.Errorf("invalid version")
	}
	var parsed parsedVersion
	numbers := [3]*int{&parsed.major, &parsed.updateFlow, &parsed.release}
	for index, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil {
			return parsedVersion{}, fmt.Errorf("invalid version")
		}
		*numbers[index] = number
	}
	// Legacy v1.2.{N} strings: the historical "2" was a
	// placeholder for what is now U. Per #266 decision 1, treat
	// that placeholder as U=1 so existing releases continue to
	// compare. The new shape v1.{U}.{N} uses the literal U;
	// the historical shape is rewritten on first emit
	// (bump-version.ps1 --bump-update-flow follow-up).
	if isLegacyVersionShape(normalized) {
		parsed.updateFlow = 1
	}
	return parsed, nil
}

// isLegacyVersionShape returns true for v1.2.{N} shapes. The
// new shape v1.{U}.{N} with a real U value is anything else.
// The "2" is the historical placeholder; new releases will
// never have it as the middle number unless U literally
// equals 2 (a U=1.3.0 release that shipps with U=2 — at which
// point every operator knows U bumped and reinstall is the
// expected path; parseVersion will then leave the literal 2
// as U=2, which matches the new shape, and the legacy
// special-case is silent).
func isLegacyVersionShape(normalized string) bool {
	return strings.HasPrefix(normalized, "1.2.")
}

func versionFromString(value string) (string, error) {
	match := versionPattern.FindStringSubmatch(strings.TrimSpace(value))
	if len(match) != 4 {
		return "", fmt.Errorf("version not found")
	}
	normalized := fmt.Sprintf("%s.%s.%s", match[1], match[2], match[3])
	// Reject inputs that contain a fourth `.` segment after
	// the matched version (e.g. "1.2.3.4") so we don't silently
	// truncate. versionPattern only captures three groups, so
	// the input could otherwise pass without notice.
	remainder := strings.TrimSpace(value)
	if strings.Contains(strings.TrimPrefix(remainder, "v"), normalized+".") {
		return "", fmt.Errorf("version must be exactly 3 segments")
	}
	return normalized, nil
}

func assetKindFromURL(rawURL string) string {
	lower := strings.ToLower(strings.TrimSpace(rawURL))
	switch {
	case strings.HasSuffix(lower, ".zip"):
		return "zip"
	case strings.HasSuffix(lower, ".exe"):
		return "exe"
	default:
		return ""
	}
}

func isDirectAssetURL(rawURL string) bool {
	return assetKindFromURL(rawURL) != ""
}

func resolveRelativeURL(baseURL, rawValue string) (string, error) {
	base, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", err
	}
	relative, err := url.Parse(strings.TrimSpace(rawValue))
	if err != nil {
		return "", err
	}
	return base.ResolveReference(relative).String(), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func downloadFileName(downloadURL, assetKind string) string {
	parsed, err := url.Parse(downloadURL)
	if err == nil {
		if base := filepath.Base(parsed.Path); base != "." && base != "/" && base != `\` && strings.TrimSpace(base) != "" {
			return base
		}
	}
	if assetKind == "exe" {
		return "DixieData.exe"
	}
	return "DixieData-update.zip"
}

func (s *Service) downloadFile(downloadURL, destinationPath string) error {
	return s.downloadFileWithProgress(downloadURL, destinationPath, nil)
}

// downloadFileWithProgress downloads the update zip with per-chunk
// progress reporting (issue #661). The callback fires once at
// the start of the transfer with TotalBytes from the response's
// Content-Length header (-1 if absent), once for each chunk
// copied from the response body, and once at the end with the
// final BytesDownloaded. On error, the callback fires a final
// time with Phase=PhaseError + a non-empty Error string.
//
// The existing downloadFile delegates here with a nil callback
// (the no-op path) so any caller that doesn't need progress
// reporting keeps the original behavior.
func (s *Service) downloadFileWithProgress(downloadURL, destinationPath string, callback func(UpdateProgress)) error {
	request, err := http.NewRequest(http.MethodGet, downloadURL, nil)
	if err != nil {
		if callback != nil {
			callback(UpdateProgress{Phase: PhaseError, Error: err.Error()})
		}
		return err
	}
	request.Header.Set("User-Agent", buildinfo.AppName+"-updater/"+buildinfo.AppVersion)
	response, err := s.client.Do(request)
	if err != nil {
		if callback != nil {
			callback(UpdateProgress{Phase: PhaseError, Error: err.Error()})
		}
		return err
	}
	defer debug.DeferCloseLog(response.Body, "downloadFileWithProgress.response")
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		downloadErr := fmt.Errorf("update download returned %s", response.Status)
		if callback != nil {
			callback(UpdateProgress{Phase: PhaseError, Error: downloadErr.Error()})
		}
		return downloadErr
	}
	totalBytes := response.ContentLength
	if totalBytes < 0 {
		totalBytes = -1
	}
	if callback != nil {
		callback(UpdateProgress{
			Phase:           PhaseDownload,
			BytesDownloaded: 0,
			TotalBytes:      totalBytes,
			Message:         "Downloading update…",
		})
	}
	file, err := os.Create(destinationPath)
	if err != nil {
		if callback != nil {
			callback(UpdateProgress{Phase: PhaseError, Error: err.Error()})
		}
		return err
	}
	defer debug.DeferCloseLog(file, "downloadFileWithProgress.file")
	// Wrap the response body in a counting reader so the callback
	// fires per-chunk. CopyBuffer ensures we always read in
	// 32 KiB blocks (matches http.Transport's default buffer size)
	// even if the server flushes smaller chunks.
	var downloaded int64
	reader := &countingReader{r: response.Body, n: &downloaded}
	if _, err := io.CopyBuffer(file, reader, make([]byte, 32*1024)); err != nil {
		if callback != nil {
			callback(UpdateProgress{
				Phase:           PhaseError,
				BytesDownloaded: downloaded,
				TotalBytes:      totalBytes,
				Error:           err.Error(),
			})
		}
		return err
	}
	if callback != nil {
		callback(UpdateProgress{
			Phase:           PhaseDownload,
			BytesDownloaded: downloaded,
			TotalBytes:      totalBytes,
			Message:         "Download complete",
		})
	}
	return nil
}

// countingReader wraps an io.Reader to track total bytes read.
// The updater's download progress callback fires per Read() call
// (one per network chunk) so the UI can show live byte counts.
type countingReader struct {
	r io.Reader
	n *int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if n > 0 {
		*c.n += int64(n)
	}
	return n, err
}

func verifyFileChecksum(filePath, expectedHex string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer debug.DeferCloseLog(file, "verifyFileChecksum.file")
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	actual := strings.ToLower(hex.EncodeToString(hash.Sum(nil)))
	expected := strings.ToLower(strings.TrimSpace(expectedHex))
	if actual != expected {
		return fmt.Errorf("download checksum mismatch")
	}
	return nil
}

func extractZip(zipPath, destinationRoot string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer debug.DeferCloseLog(reader, "extractZip.reader")
	rootPrefix := strings.ToLower(destinationRoot + string(os.PathSeparator))
	for _, file := range reader.File {
		relativePath := filepath.Clean(filepath.FromSlash(file.Name))
		if relativePath == "." {
			continue
		}
		targetPath := filepath.Join(destinationRoot, relativePath)
		targetLower := strings.ToLower(targetPath)
		if targetLower != strings.ToLower(destinationRoot) && !strings.HasPrefix(targetLower, rootPrefix) {
			return fmt.Errorf("unsafe file path inside update archive: %s", file.Name)
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}
		source, err := file.Open()
		if err != nil {
			return err
		}
		destination, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, file.Mode())
		if err != nil {
			source.Close()
			return err
		}
		_, copyErr := io.Copy(destination, source)
		closeErr := destination.Close()
		source.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func normalizeStageRoot(stageRoot, executableName string) (string, error) {
	var matches []string
	err := filepath.Walk(stageRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		if strings.EqualFold(info.Name(), executableName) {
			matches = append(matches, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("update archive does not contain %s", executableName)
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("update archive contains multiple %s files", executableName)
	}
	return filepath.Dir(matches[0]), nil
}

func copyFile(sourcePath, destinationPath string) error {
	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
		return err
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer debug.DeferCloseLog(source, "copyFile.source")
	destination, err := os.Create(destinationPath)
	if err != nil {
		return err
	}
	defer debug.DeferCloseLog(destination, "copyFile.destination")
	if _, err := io.Copy(destination, source); err != nil {
		return err
	}
	return destination.Close()
}

func (s *Service) loadApplyStatus() *ApplyStatus {
	content, err := os.ReadFile(appdata.UpdateApplyResultPath(s.dataDir))
	if err != nil {
		return nil
	}
	var status ApplyStatus
	if err := json.Unmarshal(content, &status); err != nil {
		return nil
	}
	return &status
}

type applyScriptOptions struct {
	ProcessID          int
	StageDir           string
	InstallDir         string
	ExecutableName     string
	ResultPath         string
	LaunchStatePath    string
	TargetVersion      string
	SourceVersion      string
	FeedbackLogPath    string
	FeedbackArchiveDir string
}

type RollbackScriptOptions struct {
	ProcessID         int
	InstallDir        string
	InstalledBuildDir string
	DataDir           string
	ExecutableName    string
	LaunchStatePath   string
}

func writeApplyScript(scriptPath string, options applyScriptOptions) error {
	content := fmt.Sprintf(`$ErrorActionPreference = 'Stop'
$processId = %d
$stageDir = %s
$installDir = %s
$resultPath = %s
$launchStatePath = %s
$executableName = %s
$targetVersion = %s
$sourceVersion = %s
$feedbackLogPath = %s
$feedbackArchiveDir = %s
$targetExe = Join-Path $installDir $executableName
$backupExe = Join-Path $installDir ($executableName + '.bak')
$oauthName = 'google-oauth-defaults.json'
$oauthSource = Join-Path $stageDir $oauthName
$oauthTarget = Join-Path $installDir $oauthName
$archivedFeedbackPath = $null

function Write-Result([string]$status, [string]$message) {
    $payload = @{
        status = $status
        version = $targetVersion
        message = $message
        applied_at = (Get-Date).ToUniversalTime().ToString('o')
    } | ConvertTo-Json -Compress
    $resultDir = Split-Path -Parent $resultPath
    New-Item -ItemType Directory -Path $resultDir -Force | Out-Null
    Set-Content -LiteralPath $resultPath -Value $payload -Encoding UTF8
}

function Archive-FeedbackLog() {
    if (-not (Test-Path $feedbackLogPath)) {
        return
    }
    $safeVersion = [Regex]::Replace($sourceVersion, '[^A-Za-z0-9._-]', '-')
    if ([string]::IsNullOrWhiteSpace($safeVersion)) {
        $safeVersion = 'unknown-version'
    }
    $archiveDir = Join-Path $feedbackArchiveDir $safeVersion
    New-Item -ItemType Directory -Path $archiveDir -Force | Out-Null
    $archivePath = Join-Path $archiveDir ('feedback-log-' + (Get-Date).ToUniversalTime().ToString('yyyyMMdd-HHmmss') + '.jsonl')
    Move-Item -LiteralPath $feedbackLogPath -Destination $archivePath -Force
    $script:archivedFeedbackPath = $archivePath
}

function Restore-FeedbackLog() {
    if ([string]::IsNullOrWhiteSpace($script:archivedFeedbackPath)) {
        return
    }
    if ((Test-Path $script:archivedFeedbackPath) -and (-not (Test-Path $feedbackLogPath))) {
        $feedbackDir = Split-Path -Parent $feedbackLogPath
        New-Item -ItemType Directory -Path $feedbackDir -Force | Out-Null
        Move-Item -LiteralPath $script:archivedFeedbackPath -Destination $feedbackLogPath -Force
    }
}

try {
    if ($processId -gt 0) {
        try {
            Wait-Process -Id $processId -Timeout 45 -ErrorAction Stop
        } catch {
            Start-Sleep -Seconds 2
        }
    }
    Start-Sleep -Milliseconds 750

    if (Test-Path $backupExe) {
        Remove-Item -LiteralPath $backupExe -Force -ErrorAction SilentlyContinue
    }
    if (Test-Path $targetExe) {
        Move-Item -LiteralPath $targetExe -Destination $backupExe -Force
    }

    Get-ChildItem -LiteralPath $stageDir -Force | Where-Object { $_.Name -ne $oauthName } | ForEach-Object {
        Copy-Item -LiteralPath $_.FullName -Destination $installDir -Recurse -Force
    }
    if (-not (Test-Path $oauthTarget) -and (Test-Path $oauthSource)) {
        Copy-Item -LiteralPath $oauthSource -Destination $oauthTarget -Force
    }

    if (-not (Test-Path $targetExe)) {
        throw 'Updated executable was not copied into place.'
    }
    if (Test-Path $backupExe) {
        Remove-Item -LiteralPath $backupExe -Force -ErrorAction SilentlyContinue
    }

    Archive-FeedbackLog
    Write-Result -status 'success' -message ('Applied update to v' + $targetVersion + '.')
    Start-Process -FilePath $targetExe | Out-Null
} catch {
    Restore-FeedbackLog
    if ((-not (Test-Path $targetExe)) -and (Test-Path $backupExe)) {
        Move-Item -LiteralPath $backupExe -Destination $targetExe -Force
    }
    if (Test-Path $launchStatePath) {
        Remove-Item -LiteralPath $launchStatePath -Force -ErrorAction SilentlyContinue
    }
    Write-Result -status 'failed' -message $_.Exception.Message
    exit 1
}
`, options.ProcessID, psLiteral(options.StageDir), psLiteral(options.InstallDir), psLiteral(options.ResultPath), psLiteral(options.LaunchStatePath), psLiteral(options.ExecutableName), psLiteral(options.TargetVersion), psLiteral(options.SourceVersion), psLiteral(options.FeedbackLogPath), psLiteral(options.FeedbackArchiveDir))
	return os.WriteFile(scriptPath, []byte(content), 0o644)
}

func WriteRollbackScript(scriptPath string, options RollbackScriptOptions) error {
	content := fmt.Sprintf(`$ErrorActionPreference = 'Stop'
$processId = %d
$installDir = %s
$installedBuildDir = %s
$dataDir = %s
$launchStatePath = %s
$executableName = %s
$targetExe = Join-Path $installDir $executableName

try {
    if ($processId -gt 0) {
        try {
            Wait-Process -Id $processId -Timeout 45 -ErrorAction Stop
        } catch {
            Start-Sleep -Seconds 2
        }
    }
    Start-Sleep -Milliseconds 750

    Get-ChildItem -LiteralPath $installDir -Force | Where-Object { $_.FullName -ne $dataDir } | ForEach-Object {
        Remove-Item -LiteralPath $_.FullName -Recurse -Force
    }
    Get-ChildItem -LiteralPath $installedBuildDir -Force | ForEach-Object {
        Copy-Item -LiteralPath $_.FullName -Destination $installDir -Recurse -Force
    }

    if (-not (Test-Path $targetExe)) {
        throw 'Restored executable was not copied into place.'
    }
    if (Test-Path $launchStatePath) {
        Remove-Item -LiteralPath $launchStatePath -Force -ErrorAction SilentlyContinue
    }
    Start-Process -FilePath $targetExe | Out-Null
} catch {
    exit 1
}
`, options.ProcessID, psLiteral(options.InstallDir), psLiteral(options.InstalledBuildDir), psLiteral(options.DataDir), psLiteral(options.LaunchStatePath), psLiteral(options.ExecutableName))
	return os.WriteFile(scriptPath, []byte(content), 0o644)
}

func snapshotInstalledBuild(installDir, dataDir, outputDir string) error {
	installDir = filepath.Clean(strings.TrimSpace(installDir))
	dataDir = filepath.Clean(strings.TrimSpace(dataDir))
	outputDir = filepath.Clean(strings.TrimSpace(outputDir))
	if installDir == "" || outputDir == "" {
		return fmt.Errorf("install and output directories are required")
	}
	if err := os.RemoveAll(outputDir); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}
	return filepath.Walk(installDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		cleanPath := filepath.Clean(path)
		if cleanPath == dataDir {
			return filepath.SkipDir
		}
		relativePath, err := filepath.Rel(installDir, cleanPath)
		if err != nil {
			return err
		}
		if relativePath == "." {
			return nil
		}
		targetPath := filepath.Join(outputDir, relativePath)
		if info.IsDir() {
			return os.MkdirAll(targetPath, info.Mode())
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}
		if err := copyFile(cleanPath, targetPath); err != nil {
			return err
		}
		return os.Chmod(targetPath, info.Mode())
	})
}

func psLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
