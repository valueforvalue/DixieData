package appdata

import (
	"os"
	"path/filepath"
	"testing"
)

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
	if got := FeedbackLogPath(`C:\repo\.dixiedata`); got != `C:\repo\.dixiedata\logs\feedback-log.jsonl` {
		t.Fatalf("FeedbackLogPath=%q", got)
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
