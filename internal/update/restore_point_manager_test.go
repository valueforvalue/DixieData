package update

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRestorePointManagerCreateAndTrackLaunchState(t *testing.T) {
	dataDir := t.TempDir()
	manager := NewRestorePointManager(dataDir)
	manager.now = func() time.Time {
		return time.Date(2026, time.May, 31, 8, 15, 4, 0, time.UTC)
	}

	record, err := manager.Create(CreateRestorePointInput{
		SourceAppVersion:    "1.2.27",
		TargetAppVersion:    "1.2.28",
		SourceBuildIdentity: "commit abc123",
	}, func(outputPath string) error {
		return os.WriteFile(outputPath, []byte("backup"), 0o644)
	}, func(outputDir string) error {
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(outputDir, "DixieData.exe"), []byte("exe"), 0o644)
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !strings.HasPrefix(record.LocalArchivePath, "updates/restore-points/restore-point-20260531-081504/") {
		t.Fatalf("record.LocalArchivePath = %q", record.LocalArchivePath)
	}
	if _, err := os.Stat(filepath.Join(dataDir, filepath.FromSlash(record.LocalArchivePath))); err != nil {
		t.Fatalf("restore archive missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, filepath.FromSlash(record.InstalledBuildPath), "DixieData.exe")); err != nil {
		t.Fatalf("installed build snapshot missing: %v", err)
	}

	if err := manager.SaveLaunchState(record); err != nil {
		t.Fatalf("SaveLaunchState: %v", err)
	}
	state, err := manager.LoadLaunchState()
	if err != nil {
		t.Fatalf("LoadLaunchState: %v", err)
	}
	if state == nil || state.Status != RestorePointLaunchPrepared {
		t.Fatalf("state = %#v", state)
	}
	state, err = manager.MarkLaunchStarting()
	if err != nil {
		t.Fatalf("MarkLaunchStarting: %v", err)
	}
	if state == nil || state.Status != RestorePointLaunchStarting {
		t.Fatalf("state after mark = %#v", state)
	}
	if !state.MatchesCurrentBuild("1.2.28", "commit something else") {
		t.Fatalf("expected version-only match for %#v", state)
	}
	if err := manager.ClearLaunchState(); err != nil {
		t.Fatalf("ClearLaunchState: %v", err)
	}
	state, err = manager.LoadLaunchState()
	if err != nil {
		t.Fatalf("LoadLaunchState after clear: %v", err)
	}
	if state != nil {
		t.Fatalf("expected cleared state, got %#v", state)
	}
}

func TestRestorePointManagerPrunesOlderRestorePointsByCount(t *testing.T) {
	dataDir := t.TempDir()
	manager := NewRestorePointManager(dataDir)
	manager.policy.MaxRestorePoints = 2

	times := []time.Time{
		time.Date(2026, time.May, 31, 8, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 31, 9, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 31, 10, 0, 0, 0, time.UTC),
	}
	for _, currentTime := range times {
		manager.now = func() time.Time { return currentTime }
		_, err := manager.Create(CreateRestorePointInput{
			SourceAppVersion: "1.2.27",
			TargetAppVersion: "1.2.28",
		}, func(outputPath string) error {
			return os.WriteFile(outputPath, []byte(currentTime.Format(time.RFC3339)), 0o644)
		}, func(outputDir string) error {
			if err := os.MkdirAll(outputDir, 0o755); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(outputDir, "DixieData.exe"), []byte("exe"), 0o644)
		})
		if err != nil {
			t.Fatalf("Create(%s): %v", currentTime, err)
		}
	}

	listed, err := manager.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("len(listed) = %d, listed = %#v", len(listed), listed)
	}
	if strings.Contains(listed[0].ID, "080000") || strings.Contains(listed[1].ID, "080000") {
		t.Fatalf("oldest restore point should have been pruned: %#v", listed)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "updates", "restore-points", "restore-point-20260531-080000")); !os.IsNotExist(err) {
		t.Fatalf("oldest restore point directory should be removed, err = %v", err)
	}
}
