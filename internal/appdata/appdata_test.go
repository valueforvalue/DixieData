package appdata

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWebViewUserDataPathPreservesStableDefaultAndIsolatesRCArchives(t *testing.T) {
	appDataRoot := t.TempDir()
	archiveA := filepath.Join(t.TempDir(), ".dixiedata")
	archiveB := filepath.Join(t.TempDir(), ".dixiedata")

	if got, err := WebViewUserDataPath("", archiveA, appDataRoot); err != nil || got != "" {
		t.Fatalf("stable WebViewUserDataPath = %q, %v; want empty Wails default", got, err)
	}

	rc1A, err := WebViewUserDataPath("rc1", archiveA, appDataRoot)
	if err != nil {
		t.Fatalf("RC1 archive A: %v", err)
	}
	rc2A, err := WebViewUserDataPath("rc2", archiveA, appDataRoot)
	if err != nil {
		t.Fatalf("RC2 archive A: %v", err)
	}
	rc1B, err := WebViewUserDataPath("rc1", archiveB, appDataRoot)
	if err != nil {
		t.Fatalf("RC1 archive B: %v", err)
	}

	if rc1A == "" {
		t.Fatal("RC WebViewUserDataPath must be explicit; empty path shares stable profile")
	}
	if rc1A != rc2A {
		t.Fatalf("RC profile changed across RC tags: rc1=%q rc2=%q", rc1A, rc2A)
	}
	if rc1A == rc1B {
		t.Fatalf("distinct Local Archives share RC profile %q", rc1A)
	}
	if !strings.HasPrefix(rc1A, filepath.Join(appDataRoot, "DixieData-RC")+string(filepath.Separator)) {
		t.Fatalf("RC profile %q is outside DixieData-RC root", rc1A)
	}
	if strings.Contains(strings.ToLower(rc1A), strings.ToLower(archiveA)) {
		t.Fatalf("RC profile leaks raw Local Archive path: %q", rc1A)
	}
}

func TestProjectRootFromFindsNearestWailsConfig(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "build", "bin")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "wails.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, ok := projectRootFrom(nested)
	if !ok {
		t.Fatal("expected project root to be found")
	}
	if got != root {
		t.Fatalf("root=%q want %q", got, root)
	}
}

func TestIsDevelopmentBuildRecognizesBuildBinExecutable(t *testing.T) {
	root := t.TempDir()
	buildBin := filepath.Join(root, "build", "bin")
	if err := os.MkdirAll(buildBin, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "wails.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if !IsDevelopmentBuild(filepath.Join(buildBin, "DixieData.exe")) {
		t.Fatalf("expected build\\bin executable to be treated as a development build")
	}
	if IsDevelopmentBuild(filepath.Join(root, "DixieData.exe")) {
		t.Fatalf("expected installed executable path to not be treated as a development build")
	}
}

func TestRecordImageDirUsesSanitizedDisplayID(t *testing.T) {
	absolute, relative := RecordImageDir(`C:\repo\.dixiedata`, `PENSION/42 A`)
	if absolute != `C:\repo\.dixiedata\images\P\E\PENSION-42-A` {
		t.Fatalf("absolute=%q", absolute)
	}
	if relative != `images\P\E\PENSION-42-A` {
		t.Fatalf("relative=%q", relative)
	}
}

func TestScratchpadPathsUseSanitizedDisplayID(t *testing.T) {
	textPath, statePath := ScratchpadPaths(`C:\repo\.dixiedata`, `PENSION/42 A`)
	if textPath != `C:\repo\.dixiedata\scratchpads\PENSION-42-A.txt` {
		t.Fatalf("textPath=%q", textPath)
	}
	if statePath != `C:\repo\.dixiedata\scratchpads\PENSION-42-A.json` {
		t.Fatalf("statePath=%q", statePath)
	}
}

func TestFeedbackLogPathUsesLogsDirectory(t *testing.T) {
	// App logs and feedback archives live in .dixiedata-logs/, a
	// SIBLING of the data directory, not a child. This split is
	// what makes .ddbak restore atomic on Windows: the restore
	// code path renames the entire .dixiedata directory to a
	// -previous-* sibling and renames a staged directory in its
	// place, which fails on Windows while any file handle inside
	// .dixiedata is still open. Logs being outside the data dir
	// means the rename never touches them.
	if got := FeedbackLogPath(`C:\repo\.dixiedata`); got != `C:\repo\.dixiedata-logs\feedback-log.jsonl` {
		t.Fatalf("FeedbackLogPath=%q", got)
	}
	if got := FeedbackLogArchiveDir(`C:\repo\.dixiedata`); got != `C:\repo\.dixiedata-logs\feedback-history` {
		t.Fatalf("FeedbackLogArchiveDir=%q", got)
	}
	if got := FeedbackLogArchiveVersionDir(`C:\repo\.dixiedata`, `v1.2.37 / prior`); got != `C:\repo\.dixiedata-logs\feedback-history\v1-2-37-prior` {
		t.Fatalf("FeedbackLogArchiveVersionDir=%q", got)
	}
}

func TestUpdatePathsUseUpdatesDirectory(t *testing.T) {
	if got := UpdatesDir(`C:\repo\.dixiedata`); got != `C:\repo\.dixiedata\updates` {
		t.Fatalf("UpdatesDir=%q", got)
	}
	if got := UpdateDownloadsDir(`C:\repo\.dixiedata`); got != `C:\repo\.dixiedata\updates\downloads` {
		t.Fatalf("UpdateDownloadsDir=%q", got)
	}
	if got := UpdateApplyResultPath(`C:\repo\.dixiedata`); got != `C:\repo\.dixiedata\updates\apply-result.json` {
		t.Fatalf("UpdateApplyResultPath=%q", got)
	}
}
