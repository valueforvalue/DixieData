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

// TestRestorePointManagerSiblingRootSurvivesDataDirRename locks
// the contract that motivated the sibling manager: the restore
// point lives OUTSIDE the data dir, so archive.replaceDataDir's
// rename of the data dir doesn't take the restore point with
// it. The bug this prevents: Phase 5's pre-import restore
// point landed in <dataDir>/updates/restore-points/<id>/ and
// was destroyed by the import's own staging swap.
//
// Test setup: create a sibling manager rooted at a SIBLING of
// the data dir. Create a restore point. Rename the data dir.
// Assert the restore point is still on disk AND the manager
// can still List/Get it.
func TestRestorePointManagerSiblingRootSurvivesDataDirRename(t *testing.T) {
	dataDir := t.TempDir()
	sibling := filepath.Join(filepath.Dir(dataDir), filepath.Base(dataDir)+"-restore-points")
	manager := NewSiblingRestorePointManager(dataDir, sibling)
	manager.now = func() time.Time {
		return time.Date(2026, time.June, 28, 7, 5, 4, 0, time.UTC)
	}

	record, err := manager.Create(CreateRestorePointInput{
		SourceAppVersion: "1.2.27",
		TargetAppVersion: "1.2.28",
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

	// Sibling manager builds paths WITHOUT the
	// updates/restore-points/ prefix so they resolve under root.
	if strings.HasPrefix(record.LocalArchivePath, "updates/restore-points/") {
		t.Fatalf("sibling record.LocalArchivePath should not carry data-dir prefix; got %q", record.LocalArchivePath)
	}
	if !strings.HasPrefix(record.LocalArchivePath, "restore-point-20260628-070504/") {
		t.Fatalf("record.LocalArchivePath = %q", record.LocalArchivePath)
	}
	siblingArchive := filepath.Join(sibling, filepath.FromSlash(record.LocalArchivePath))
	if _, err := os.Stat(siblingArchive); err != nil {
		t.Fatalf("sibling archive missing: %v", err)
	}

	// Simulate archive.replaceDataDir renaming the data dir
	// into a *-previous-* sibling (the import's staging swap
	// would then RemoveAll that sibling). The restore point in
	// the SIBLING must survive.
	previousDir := filepath.Join(filepath.Dir(dataDir), filepath.Base(dataDir)+"-previous-test")
	if err := os.Rename(dataDir, previousDir); err != nil {
		t.Fatalf("simulated rename: %v", err)
	}
	if _, err := os.Stat(siblingArchive); err != nil {
		t.Fatalf("sibling archive destroyed by data-dir rename: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sibling, "index.json")); err != nil {
		t.Fatalf("sibling index.json destroyed by data-dir rename: %v", err)
	}

	// Cleanup so the in-memory manager state can be exercised
	// from a fresh data dir.
	if err := os.Rename(previousDir, dataDir); err != nil {
		t.Fatalf("restore data dir: %v", err)
	}

	// A fresh sibling manager over the same sibling dir must
	// still List the restore point (re-open test).
	fresh := NewSiblingRestorePointManager(dataDir, sibling)
	listed, err := fresh.List()
	if err != nil {
		t.Fatalf("fresh.List: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != record.ID {
		t.Fatalf("fresh.List = %#v", listed)
	}
}

// TestRestorePointManagerSiblingRootIgnoresLaunchState is a
// paired test: the launch-state file lives at
// <dataDir>/updates/restore-point-state.json, NOT under the
// sibling. A sibling manager that targets a root outside the
// data dir would never see the launch state on update, so
// NewSiblingRestorePointManager should not offer launch-state
// helpers (or, if it does, the test will document which path
// they read). Phase 6 ships the sibling manager for pre-import
// rollback only; the in-place update flow keeps using
// NewRestorePointManager. This test pins the no-launch-state
// contract by ensuring the sibling manager's List/Get work
// and SaveLaunchState writes to the data-dir path.
func TestRestorePointManagerSiblingRootListAndGet(t *testing.T) {
	dataDir := t.TempDir()
	sibling := filepath.Join(filepath.Dir(dataDir), filepath.Base(dataDir)+"-restore-points")
	manager := NewSiblingRestorePointManager(dataDir, sibling)
	manager.now = func() time.Time {
		return time.Date(2026, time.June, 28, 7, 10, 0, 0, time.UTC)
	}

	record, err := manager.Create(CreateRestorePointInput{}, func(outputPath string) error {
		return os.WriteFile(outputPath, []byte("a"), 0o644)
	}, func(outputDir string) error {
		return os.MkdirAll(outputDir, 0o755)
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := manager.Get(record.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != record.ID {
		t.Fatalf("Get returned %q, want %q", got.ID, record.ID)
	}
	if _, err := manager.Get("nonexistent"); err == nil {
		t.Fatalf("expected error for missing restore point")
	}
}
