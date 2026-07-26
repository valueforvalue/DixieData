package update

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/valueforvalue/DixieData/internal/testtemp"
)

type stubConfigStore struct {
	values map[string]string
}

func (s *stubConfigStore) SystemConfig(key string) (string, error) {
	return s.values[key], nil
}

func (s *stubConfigStore) SetSystemConfig(key, value string) error {
	if s.values == nil {
		s.values = map[string]string{}
	}
	s.values[key] = value
	return nil
}

func TestGitHubReleaseFromJSONPrefersReleaseZip(t *testing.T) {
	payload := githubRelease{
		TagName: "v1.2.23",
		HTMLURL: "https://github.com/valueforvalue/DixieData/releases/tag/v1.2.23",
		Body:    "Release notes",
		Assets: []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
			ContentType        string `json:"content_type"`
		}{
			{Name: "DixieData.exe", BrowserDownloadURL: "https://example.com/DixieData.exe"},
			{Name: "DixieData-release-1.2.23.zip", BrowserDownloadURL: "https://example.com/DixieData-release-1.2.23.zip"},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	release, err := githubReleaseFromJSON(body)
	if err != nil {
		t.Fatalf("githubReleaseFromJSON: %v", err)
	}
	if release.downloadURL != "https://example.com/DixieData-release-1.2.23.zip" {
		t.Fatalf("downloadURL=%q", release.downloadURL)
	}
	if release.assetKind != "zip" {
		t.Fatalf("assetKind=%q", release.assetKind)
	}
}

func TestManifestReleaseFromJSONResolvesRelativeAssetURL(t *testing.T) {
	body := []byte(`{"version":"1.2.24","asset_url":"downloads/DixieData-release-1.2.24.zip","sha256":"abc123"}`)
	release, err := manifestReleaseFromJSON(body, "https://updates.example.com/releases/latest.json")
	if err != nil {
		t.Fatalf("manifestReleaseFromJSON: %v", err)
	}
	if release.downloadURL != "https://updates.example.com/releases/downloads/DixieData-release-1.2.24.zip" {
		t.Fatalf("downloadURL=%q", release.downloadURL)
	}
	if release.checksumSHA != "abc123" {
		t.Fatalf("checksumSHA=%q", release.checksumSHA)
	}
}

func TestDirectReleaseFromURLRequiresEmbeddedVersion(t *testing.T) {
	_, err := directReleaseFromURL("https://updates.example.com/DixieData-release.zip")
	if err == nil {
		t.Fatal("expected direct release URL without a version to fail")
	}
}

func TestCompareVersions(t *testing.T) {
	// Legacy v1.2.{N} strings: the historical placeholder
	// "2" is the U position; parseVersion rewrites it to U=1
	// per #266 decision 1.
	comparison, err := compareVersions("v1.2.24", "v1.2.23")
	if err != nil {
		t.Fatalf("compareVersions: %v", err)
	}
	if !comparison.Compatible {
		t.Fatalf("U should match under legacy v1.2.* mapping; got %+v", comparison)
	}
	if !comparison.Newer {
		t.Fatalf("v1.2.24 should be newer than v1.2.23; got %+v", comparison)
	}
}

func TestNormalizeStageRootFindsSingleExecutable(t *testing.T) {
	stageRoot := testtemp.New(t).Path()
	appRoot := filepath.Join(stageRoot, "DixieData")
	if err := os.MkdirAll(appRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	target := filepath.Join(appRoot, "DixieData.exe")
	if err := os.WriteFile(target, []byte("exe"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	normalized, err := normalizeStageRoot(stageRoot, "DixieData.exe")
	if err != nil {
		t.Fatalf("normalizeStageRoot: %v", err)
	}
	if normalized != appRoot {
		t.Fatalf("normalized=%q want %q", normalized, appRoot)
	}
}

func TestWriteApplyScriptPreservesOAuthDefaults(t *testing.T) {
	scriptPath := filepath.Join(testtemp.New(t).Path(), "apply-update.ps1")
	err := writeApplyScript(scriptPath, applyScriptOptions{
		ProcessID:          123,
		StageDir:           `C:\updates\stage`,
		InstallDir:         `C:\Program Files\DixieData`,
		ExecutableName:     "DixieData.exe",
		ResultPath:         `C:\data\.dixiedata\updates\apply-result.json`,
		TargetVersion:      "1.2.24",
		SourceVersion:      "1.2.23",
		FeedbackLogPath:    `C:\data\.dixiedata\logs\feedback-log.jsonl`,
		FeedbackArchiveDir: `C:\data\.dixiedata\logs\feedback-history`,
	})
	if err != nil {
		t.Fatalf("writeApplyScript: %v", err)
	}
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	text := string(content)
	for _, needle := range []string{
		"google-oauth-defaults.json",
		"if (-not (Test-Path $oauthTarget) -and (Test-Path $oauthSource))",
		"Move-Item -LiteralPath $targetExe -Destination $backupExe -Force",
		"Archive-FeedbackLog",
		"Restore-FeedbackLog",
		"$feedbackLogPath = 'C:\\data\\.dixiedata\\logs\\feedback-log.jsonl'",
		"$feedbackArchiveDir = 'C:\\data\\.dixiedata\\logs\\feedback-history'",
		"Write-Result -status 'failed'",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("script missing %q", needle)
		}
	}
}

func TestSettingsDisablesApplyForDevelopmentBuild(t *testing.T) {
	store := &stubConfigStore{}
	root := testtemp.New(t).Path()
	buildBin := filepath.Join(root, "build", "bin")
	if err := os.MkdirAll(buildBin, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "wails.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	service := NewService(store, testtemp.New(t).Path(), nil)
	service.executablePath = func() (string, error) {
		return filepath.Join(buildBin, "DixieData.exe"), nil
	}

	settings, err := service.Settings()
	if err != nil {
		t.Fatalf("Settings: %v", err)
	}
	if settings.CanApply {
		t.Fatalf("expected development build to disable self-update")
	}
	if !strings.Contains(settings.DisabledReason, "build\\bin") {
		t.Fatalf("DisabledReason=%q", settings.DisabledReason)
	}
}

func TestWriteApplyScriptClearsLaunchStateOnFailure(t *testing.T) {
	scriptPath := filepath.Join(testtemp.New(t).Path(), "apply-update.ps1")
	err := writeApplyScript(scriptPath, applyScriptOptions{
		ProcessID:          123,
		StageDir:           `C:\updates\stage`,
		InstallDir:         `C:\Program Files\DixieData`,
		ExecutableName:     "DixieData.exe",
		ResultPath:         `C:\data\.dixiedata\updates\apply-result.json`,
		LaunchStatePath:    `C:\data\.dixiedata\updates\restore-point-state.json`,
		TargetVersion:      "1.2.24",
		SourceVersion:      "1.2.23",
		FeedbackLogPath:    `C:\data\.dixiedata\logs\feedback-log.jsonl`,
		FeedbackArchiveDir: `C:\data\.dixiedata\logs\feedback-history`,
	})
	if err != nil {
		t.Fatalf("writeApplyScript: %v", err)
	}
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "$launchStatePath = 'C:\\data\\.dixiedata\\updates\\restore-point-state.json'") {
		t.Fatalf("script missing launch state path: %s", text)
	}
	if !strings.Contains(text, "Remove-Item -LiteralPath $launchStatePath -Force -ErrorAction SilentlyContinue") {
		t.Fatalf("script missing launch-state cleanup: %s", text)
	}
	if !strings.Contains(text, "Move-Item -LiteralPath $feedbackLogPath -Destination $archivePath -Force") {
		t.Fatalf("script missing feedback-log archive move: %s", text)
	}
}

func TestWriteRollbackScriptRestoresInstalledBuildAndClearsLaunchState(t *testing.T) {
	scriptPath := filepath.Join(testtemp.New(t).Path(), "rollback.ps1")
	err := WriteRollbackScript(scriptPath, RollbackScriptOptions{
		ProcessID:         456,
		InstallDir:        `C:\Program Files\DixieData`,
		InstalledBuildDir: `C:\data\.dixiedata\updates\restore-points\restore-point-1\installed-build`,
		DataDir:           `C:\Program Files\DixieData\.dixiedata`,
		ExecutableName:    "DixieData.exe",
		LaunchStatePath:   `C:\data\.dixiedata\updates\restore-point-state.json`,
	})
	if err != nil {
		t.Fatalf("WriteRollbackScript: %v", err)
	}
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	text := string(content)
	for _, needle := range []string{
		"Get-ChildItem -LiteralPath $installDir -Force | Where-Object { $_.FullName -ne $dataDir }",
		"Get-ChildItem -LiteralPath $installedBuildDir -Force | ForEach-Object",
		"Remove-Item -LiteralPath $launchStatePath -Force -ErrorAction SilentlyContinue",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("script missing %q", needle)
		}
	}
}

func TestSnapshotInstalledBuildSkipsDataDir(t *testing.T) {
	installDir := testtemp.New(t).Path()
	dataDir := filepath.Join(installDir, ".dixiedata")
	if err := os.MkdirAll(filepath.Join(dataDir, "updates"), 0o755); err != nil {
		t.Fatalf("MkdirAll(dataDir): %v", err)
	}
	if err := os.WriteFile(filepath.Join(installDir, "DixieData.exe"), []byte("exe"), 0o644); err != nil {
		t.Fatalf("WriteFile(exe): %v", err)
	}
	if err := os.WriteFile(filepath.Join(installDir, "pdfium.dll"), []byte("dll"), 0o644); err != nil {
		t.Fatalf("WriteFile(pdfium): %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "updates", "should-not-copy.txt"), []byte("data"), 0o644); err != nil {
		t.Fatalf("WriteFile(data): %v", err)
	}
	outputDir := filepath.Join(testtemp.New(t).Path(), "installed-build")
	if err := snapshotInstalledBuild(installDir, dataDir, outputDir); err != nil {
		t.Fatalf("snapshotInstalledBuild: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "DixieData.exe")); err != nil {
		t.Fatalf("snapshot missing exe: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "pdfium.dll")); err != nil {
		t.Fatalf("snapshot missing support file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputDir, ".dixiedata")); !os.IsNotExist(err) {
		t.Fatalf("data dir should be excluded, err = %v", err)
	}
}

// TestVersionFromStringStripsPreReleaseSuffix pins the RC cohort
// workflow contract (issue #654). The updater's versionFromString
// regex captures only the 3 numeric segments; the pre-release
// suffix (-rc1, -rc2, -beta1, etc.) is dropped before the
// numeric comparison. This lets a manifest advertising
// "1.1.4-rc1" install on a user running 1.1.4 (the cohort
// opt-in path) while keeping the chrome string
// "DixieData v1.1.4-rc1" intact on the same binary.
func TestVersionFromStringStripsPreReleaseSuffix(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"1.1.4", "1.1.4"},
		{"v1.1.4", "1.1.4"},
		{"1.1.4-rc1", "1.1.4"},
		{"1.1.4-rc2", "1.1.4"},
		{"1.1.4-beta1", "1.1.4"},
		{"v1.1.4-rc1", "1.1.4"},
	}
	for _, c := range cases {
		got, err := versionFromString(c.in)
		if err != nil {
			t.Errorf("versionFromString(%q) returned error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("versionFromString(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}

// TestVersionFromStringRejectsFourthSegment pins the negative
// case. A four-segment version like "1.2.3.4" must be rejected
// (the existing guard at internal/update/updater.go's
// versionFromString), not silently truncated to "1.2.3".
// This is the contract that protects the cohort from a typo
// in the manifest's version field.
func TestVersionFromStringRejectsFourthSegment(t *testing.T) {
	for _, in := range []string{"1.2.3.4", "v1.2.3.4", "1.2.3.4-rc1"} {
		if _, err := versionFromString(in); err == nil {
			t.Errorf("versionFromString(%q) should have errored (fourth segment not allowed)", in)
		}
	}
}

// TestDownloadFileWithProgressFiresCallbackPerChunk pins the
// progress-callback contract for issue #661 (download progress
// bar for in-place update). The download function must invoke
// the callback at least twice per transfer (start + end) and
// report monotonically-increasing BytesDownloaded + the final
// TotalBytes so the UI can show "Downloading... X MB / Y MB"
// updates.
func TestDownloadFileWithProgressFiresCallbackPerChunk(t *testing.T) {
	const totalBytes int64 = 4096 // 4 chunks of 1024 bytes
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "4096")
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		chunk := make([]byte, 1024)
		for i := 0; i < 4; i++ {
			w.Write(chunk)
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer server.Close()

	service := NewService(&stubConfigStore{}, testtemp.New(t).Path(), nil)
	destPath := filepath.Join(testtemp.New(t).Path(), "download.bin")

	var (
		mu       sync.Mutex
		calls    []UpdateProgress
		progress UpdateProgress
	)
	callback := func(p UpdateProgress) {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, p)
		progress = p
	}

	if err := service.downloadFileWithProgress(server.URL, destPath, callback); err != nil {
		t.Fatalf("downloadFileWithProgress: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(calls) < 2 {
		t.Errorf("progress callback fired %d times; want >= 2 (start + end at minimum)", len(calls))
	}
	if progress.BytesDownloaded != totalBytes {
		t.Errorf("final BytesDownloaded = %d; want %d", progress.BytesDownloaded, totalBytes)
	}
	if progress.TotalBytes != totalBytes {
		t.Errorf("final TotalBytes = %d; want %d", progress.TotalBytes, totalBytes)
	}
	if progress.Phase != PhaseDownload {
		t.Errorf("final Phase = %q; want %q", progress.Phase, PhaseDownload)
	}
	var prev int64
	for i, call := range calls {
		if call.BytesDownloaded < prev {
			t.Errorf("call %d BytesDownloaded=%d < previous %d (non-monotonic)", i, call.BytesDownloaded, prev)
		}
		prev = call.BytesDownloaded
	}
}

// TestDownloadFileWithProgressReportsPhaseError pins the
// error-phase contract: if the download fails (e.g. 500 from
// server), the callback's final invocation must carry
// Phase=PhaseError + a non-empty Error string. The apply handler
// reads the final phase to decide what to render into the
// progress target.
func TestDownloadFileWithProgressReportsPhaseError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("boom"))
	}))
	defer server.Close()

	service := NewService(&stubConfigStore{}, testtemp.New(t).Path(), nil)
	destPath := filepath.Join(testtemp.New(t).Path(), "download.bin")

	var (
		mu    sync.Mutex
		final UpdateProgress
	)
	callback := func(p UpdateProgress) {
		mu.Lock()
		defer mu.Unlock()
		final = p
	}

	if err := service.downloadFileWithProgress(server.URL, destPath, callback); err == nil {
		t.Fatalf("expected error on 500 response")
	}

	mu.Lock()
	defer mu.Unlock()
	if final.Phase != PhaseError {
		t.Errorf("final Phase = %q; want %q", final.Phase, PhaseError)
	}
	if final.Error == "" {
		t.Errorf("final Error is empty; want non-empty (error message)")
	}
}

// TestApplyProgressPhasesAreDistinct pins the phase-name
// contract. Every phase string used by the progress pipeline
// must be distinct so the UI can switch on Phase to decide
// which label + progress-bar shape to render. Drift between
// phase constants would cause the UI to render the wrong label
// (e.g. "Downloading..." when the pipeline is actually
// verifying the checksum).
func TestApplyProgressPhasesAreDistinct(t *testing.T) {
	phases := []ApplyPhase{
		PhaseIdle,
		PhaseDownload,
		PhaseVerify,
		PhaseExtract,
		PhaseRestorePoint,
		PhaseApplyStarted,
		PhaseError,
	}
	seen := map[ApplyPhase]bool{}
	for _, p := range phases {
		if seen[p] {
			t.Errorf("phase %q duplicated", p)
		}
		seen[p] = true
	}
}
