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
)

const (
	updateSourceConfigKey = "update_source_url"
	defaultSourceURL      = "https://api.github.com/repos/valueforvalue/DixieData/releases/latest"
	maxUpdateSourceBytes  = 2 << 20
)

var versionPattern = regexp.MustCompile(`(?i)v?(\d+)\.(\d+)\.(\d+)`)

type configStore interface {
	SystemConfig(key string) (string, error)
	SetSystemConfig(key, value string) error
}

type Service struct {
	config         configStore
	dataDir        string
	restorePoints  *RestorePointManager
	archiveWriter  RestorePointArchiveWriter
	client         *http.Client
	executablePath func() (string, error)
	now            func() time.Time
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
	DownloadURL      string
	NotesURL         string
	ReleaseNotes     string
	PublishedAt      string
	SourceLabel      string
	CanApply         bool
	DisabledReason   string
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
	if err := s.config.SetSystemConfig(updateSourceConfigKey, normalized); err != nil {
		return SettingsState{}, err
	}
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
	return CheckResult{
		CurrentVersion:   settings.CurrentVersion,
		AvailableVersion: release.version,
		UpdateAvailable:  comparison > 0,
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
	settings, err := s.Settings()
	if err != nil {
		return PreparedUpdate{}, err
	}
	if !settings.CanApply {
		return PreparedUpdate{}, errors.New(settings.DisabledReason)
	}
	release, err := s.resolveRelease()
	if err != nil {
		return PreparedUpdate{}, err
	}
	comparison, err := compareVersions(release.version, settings.CurrentVersion)
	if err != nil {
		return PreparedUpdate{}, err
	}
	if comparison <= 0 {
		return PreparedUpdate{}, fmt.Errorf("no newer update is available")
	}

	executablePath, err := s.executablePath()
	if err != nil {
		return PreparedUpdate{}, err
	}
	executableName := filepath.Base(executablePath)
	installDir := filepath.Dir(executablePath)

	workRoot := filepath.Join(appdata.UpdateDownloadsDir(s.dataDir), "update-"+s.now().UTC().Format("20060102T150405"))
	stageRoot := filepath.Join(workRoot, "stage")
	if err := os.MkdirAll(stageRoot, 0o755); err != nil {
		return PreparedUpdate{}, err
	}

	artifactName := downloadFileName(release.downloadURL, release.assetKind)
	artifactPath := filepath.Join(workRoot, artifactName)
	if err := s.downloadFile(release.downloadURL, artifactPath); err != nil {
		return PreparedUpdate{}, err
	}
	if strings.TrimSpace(release.checksumSHA) != "" {
		if err := verifyFileChecksum(artifactPath, release.checksumSHA); err != nil {
			return PreparedUpdate{}, err
		}
	}

	switch release.assetKind {
	case "zip":
		if err := extractZip(artifactPath, stageRoot); err != nil {
			return PreparedUpdate{}, err
		}
		stageRoot, err = normalizeStageRoot(stageRoot, executableName)
		if err != nil {
			return PreparedUpdate{}, err
		}
	case "exe":
		targetExe := filepath.Join(stageRoot, executableName)
		if err := copyFile(artifactPath, targetExe); err != nil {
			return PreparedUpdate{}, err
		}
	default:
		return PreparedUpdate{}, fmt.Errorf("unsupported update asset type")
	}

	if _, err := os.Stat(filepath.Join(stageRoot, executableName)); err != nil {
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
		return PreparedUpdate{}, fmt.Errorf("create restore point: %w", err)
	}
	if err := s.restorePoints.SaveLaunchState(restorePoint); err != nil {
		return PreparedUpdate{}, fmt.Errorf("write restore point launch state: %w", err)
	}

	resultPath := appdata.UpdateApplyResultPath(s.dataDir)
	if err := os.MkdirAll(filepath.Dir(resultPath), 0o755); err != nil {
		return PreparedUpdate{}, err
	}
	scriptPath := filepath.Join(workRoot, "apply-update.ps1")
	if err := writeApplyScript(scriptPath, applyScriptOptions{
		ProcessID:       os.Getpid(),
		StageDir:        stageRoot,
		InstallDir:      installDir,
		ExecutableName:  executableName,
		ResultPath:      resultPath,
		LaunchStatePath: appdata.UpdateRestorePointStatePath(s.dataDir),
		TargetVersion:   release.version,
	}); err != nil {
		return PreparedUpdate{}, err
	}

	return PreparedUpdate{
		Version:    release.version,
		ScriptPath: scriptPath,
	}, nil
}

func (s *Service) sourceSettings() (string, string, bool, error) {
	value, err := s.config.SystemConfig(updateSourceConfigKey)
	if err != nil {
		return "", "", false, err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", defaultSourceURL, true, nil
	}
	return value, value, false, nil
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
	defer response.Body.Close()
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

func compareVersions(left, right string) (int, error) {
	leftParts, err := parseVersion(left)
	if err != nil {
		return 0, err
	}
	rightParts, err := parseVersion(right)
	if err != nil {
		return 0, err
	}
	for index := 0; index < len(leftParts); index++ {
		if leftParts[index] < rightParts[index] {
			return -1, nil
		}
		if leftParts[index] > rightParts[index] {
			return 1, nil
		}
	}
	return 0, nil
}

func parseVersion(value string) ([3]int, error) {
	normalized, err := versionFromString(value)
	if err != nil {
		return [3]int{}, err
	}
	parts := strings.Split(normalized, ".")
	if len(parts) != 3 {
		return [3]int{}, fmt.Errorf("invalid version")
	}
	var parsed [3]int
	for index, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil {
			return [3]int{}, fmt.Errorf("invalid version")
		}
		parsed[index] = number
	}
	return parsed, nil
}

func versionFromString(value string) (string, error) {
	match := versionPattern.FindStringSubmatch(strings.TrimSpace(value))
	if len(match) != 4 {
		return "", fmt.Errorf("version not found")
	}
	return fmt.Sprintf("%s.%s.%s", match[1], match[2], match[3]), nil
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
	request, err := http.NewRequest(http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", buildinfo.AppName+"-updater/"+buildinfo.AppVersion)
	response, err := s.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("update download returned %s", response.Status)
	}
	file, err := os.Create(destinationPath)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(file, response.Body)
	return err
}

func verifyFileChecksum(filePath, expectedHex string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()
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
	defer reader.Close()
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
	defer source.Close()
	destination, err := os.Create(destinationPath)
	if err != nil {
		return err
	}
	defer destination.Close()
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
	ProcessID       int
	StageDir        string
	InstallDir      string
	ExecutableName  string
	ResultPath      string
	LaunchStatePath string
	TargetVersion   string
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
$targetExe = Join-Path $installDir $executableName
$backupExe = Join-Path $installDir ($executableName + '.bak')
$oauthName = 'google-oauth-defaults.json'
$oauthSource = Join-Path $stageDir $oauthName
$oauthTarget = Join-Path $installDir $oauthName

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

    Write-Result -status 'success' -message ('Applied update to v' + $targetVersion + '.')
    Start-Process -FilePath $targetExe | Out-Null
} catch {
    if ((-not (Test-Path $targetExe)) -and (Test-Path $backupExe)) {
        Move-Item -LiteralPath $backupExe -Destination $targetExe -Force
    }
    if (Test-Path $launchStatePath) {
        Remove-Item -LiteralPath $launchStatePath -Force -ErrorAction SilentlyContinue
    }
    Write-Result -status 'failed' -message $_.Exception.Message
    exit 1
}
`, options.ProcessID, psLiteral(options.StageDir), psLiteral(options.InstallDir), psLiteral(options.ResultPath), psLiteral(options.LaunchStatePath), psLiteral(options.ExecutableName), psLiteral(options.TargetVersion))
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
