package update

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	comparison, err := compareVersions("1.2.24", "1.2.23")
	if err != nil {
		t.Fatalf("compareVersions: %v", err)
	}
	if comparison <= 0 {
		t.Fatalf("comparison=%d", comparison)
	}
}

func TestNormalizeStageRootFindsSingleExecutable(t *testing.T) {
	stageRoot := t.TempDir()
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
	scriptPath := filepath.Join(t.TempDir(), "apply-update.ps1")
	err := writeApplyScript(scriptPath, applyScriptOptions{
		ProcessID:      123,
		StageDir:       `C:\updates\stage`,
		InstallDir:     `C:\Program Files\DixieData`,
		ExecutableName: "DixieData.exe",
		ResultPath:     `C:\data\.dixiedata\updates\apply-result.json`,
		TargetVersion:  "1.2.24",
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
		"Write-Result -status 'failed'",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("script missing %q", needle)
		}
	}
}

func TestSettingsDisablesApplyForDevelopmentBuild(t *testing.T) {
	store := &stubConfigStore{}
	root := t.TempDir()
	buildBin := filepath.Join(root, "build", "bin")
	if err := os.MkdirAll(buildBin, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "wails.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	service := NewService(store, t.TempDir())
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
