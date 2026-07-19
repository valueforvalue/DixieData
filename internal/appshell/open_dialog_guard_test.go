package appshell

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	runtime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// TestOpenDialogGuardRejectsConcurrentDuplicates is the
// regression test for the dialog-guard law (the open-dialog
// branch that was deferred when guardedSaveFileDialog landed).
// A second OpenFileDialog call arriving while the first dialog
// is still on screen MUST be rejected before it reaches Wails,
// otherwise WebView2 dies with Chrome_WidgetWin_0. Error = 1412.
// Mirrors TestGuardedSaveFileDialogRejectsConcurrentDuplicates
// but for the open-pickers.
func TestOpenDialogGuardRejectsConcurrentDuplicates(t *testing.T) {
	app := NewApp()
	var invocations atomic.Int32
	// Simulate a held slot by entering the same dupKey the
	// guarded function will use. This deterministically
	// reproduces the race-condition the guard is designed to
	// prevent: a second caller arriving while the first
	// caller's slot is still in flight. The original
	// time.Sleep-based test was racy under heavy scheduler
	// pressure (issue #207); manually holding the slot
	// removes the timing dependency entirely.
	opts := runtime.OpenDialogOptions{
		Filters: []runtime.FileFilter{
			{DisplayName: "DixieData shared archive", Pattern: "*.ddshare"},
		},
	}
	dupKey := guardedOpenFileDialogKey("shared_archive", opts)

	admittedFirst, heldEntry := app.enterInFlight(dupKey)
	if !admittedFirst || heldEntry == nil {
		t.Fatalf("setup: failed to acquire held slot; admitted=%v entry=%v", admittedFirst, heldEntry)
	}
	defer app.leaveInFlight(dupKey, heldEntry)

	// Override is unused in this test — the second call's
	// dup-check must return false before reaching the dialog.
	// (If it reaches the dialog, the test would also fail
	// because the dialog's invocations counter would increment
	// past the expected value.)
	app.SetOpenFileDialogOverride(func(_ any) (string, error) {
		invocations.Add(1)
		return "/tmp/example.ddshare", nil
	})

	path, admitted, ok := app.guardedOpenFileDialog(dupKey, opts)
	if admitted {
		t.Fatalf("guardedOpenFileDialog must return admitted=false when slot is held; got admitted=true path=%q ok=%v", path, ok)
	}
	if ok {
		t.Errorf("held-slot call must return ok=false; got ok=true path=%q", path)
	}
	if path != "" {
		t.Errorf("held-slot call must return empty path; got %q", path)
	}
	if got := invocations.Load(); got != 0 {
		t.Errorf("OpenFileDialog must not be invoked when dup-check rejects; got %d", got)
	}
}

// TestOpenDirectoryGuardRejectsConcurrentDuplicates covers the
// directory picker variant of the guard.
func TestOpenDirectoryGuardRejectsConcurrentDuplicates(t *testing.T) {
	app := NewApp()
	// OpenDirectoryDialog does not currently expose an override
	// hook in internal/appshell/runtime.go, so we exercise the
	// guard by holding the in-flight slot manually. The dup-check
	// is what we are testing; the dialog itself is a passthrough.
	dupKey := guardedOpenDirectoryDialogKey("download_images", runtime.OpenDialogOptions{
		Title: "Choose where to copy the record images",
	})

	// First admit holds the slot.
	admittedFirst, entry := app.enterInFlight(dupKey)
	if !admittedFirst || entry == nil {
		t.Fatalf("expected first admit; got admitted=%v entry=%v", admittedFirst, entry)
	}

	// Second goroutine races the dup check while the slot is held.
	var wg sync.WaitGroup
	admitted := make(chan bool, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, ad, _ := app.guardedOpenDirectoryDialog(dupKey, runtime.OpenDialogOptions{Title: "Choose where to copy the record images"})
		admitted <- ad
	}()
	wg.Wait()
	close(admitted)

	for v := range admitted {
		if v {
			t.Fatalf("second goroutine must NOT have been admitted while slot held")
		}
	}
	// Release so test cleanup doesn't see a stuck slot.
	app.leaveInFlight(dupKey, entry)
}

// TestOpenMultipleFilesGuardRejectsConcurrentDuplicates covers
// the multi-file picker variant. The handler is allowed to
// return (nil, true, false) — admitted=true, ok=false — when
// the user cancels (empty slice from the OS picker), so the
// dup-hit signal is `admitted=false`, not `ok=false`.
func TestOpenMultipleFilesGuardRejectsConcurrentDuplicates(t *testing.T) {
	app := NewApp()
	var invocations atomic.Int32
	// See TestOpenDialogGuardRejectsConcurrentDuplicates for the
	// rationale. Hold the dupKey manually, then assert the
	// guarded call returns admitted=false. This is the
	// deterministic equivalent of the racy two-goroutine race
	// the test used to express.
	opts := runtime.OpenDialogOptions{
		Filters: []runtime.FileFilter{
			{DisplayName: "Image files", Pattern: "*.png;*.jpg"},
		},
	}
	dupKey := guardedOpenMultipleFilesDialogKey("import_images", opts)

	admittedFirst, heldEntry := app.enterInFlight(dupKey)
	if !admittedFirst || heldEntry == nil {
		t.Fatalf("setup: failed to acquire held slot; admitted=%v entry=%v", admittedFirst, heldEntry)
	}
	defer app.leaveInFlight(dupKey, heldEntry)

	app.SetOpenMultipleFilesDialogOverride(func(_ any) ([]string, error) {
		invocations.Add(1)
		return []string{"/tmp/a.png", "/tmp/b.jpg"}, nil
	})

	paths, admitted, ok := app.guardedOpenMultipleFilesDialog(dupKey, opts)
	if admitted {
		t.Fatalf("guardedOpenMultipleFilesDialog must return admitted=false when slot is held; got admitted=true paths=%v ok=%v", paths, ok)
	}
	if ok {
		t.Errorf("held-slot call must return ok=false; got ok=true paths=%v", paths)
	}
	if len(paths) != 0 {
		t.Errorf("held-slot call must return empty paths; got %v", paths)
	}
	if got := invocations.Load(); got != 0 {
		t.Errorf("OpenMultipleFilesDialog must not be invoked when dup-check rejects; got %d", got)
	}
}

// TestOpenDialogGuardReleasesAfterCancel verifies the slot is
// released after the dialog returns, so a subsequent retry
// (after the user cancels) is not blocked by the prior cancel.
func TestOpenDialogGuardReleasesAfterCancel(t *testing.T) {
	app := NewApp()
	app.SetOpenFileDialogOverride(func(_ any) (string, error) {
		return "", nil // simulate cancel
	})

	opts := runtime.OpenDialogOptions{
		Filters: []runtime.FileFilter{{DisplayName: "Archive", Pattern: "*.ddshare"}},
	}
	dupKey := guardedOpenFileDialogKey("shared_archive", opts)

	// First call: cancel.
	path1, admitted1, ok1 := app.guardedOpenFileDialog(dupKey, opts)
	if !admitted1 {
		t.Fatalf("first call must be admitted")
	}
	if ok1 {
		t.Errorf("cancel must return ok=false; got ok=true path=%q", path1)
	}
	if path1 != "" {
		t.Errorf("cancel must return empty path; got %q", path1)
	}

	// Second call: must not be blocked by the cancelled first call.
	app.SetOpenFileDialogOverride(func(_ any) (string, error) {
		return "/tmp/example.ddshare", nil
	})
	path2, admitted2, ok2 := app.guardedOpenFileDialog(dupKey, opts)
	if !admitted2 {
		t.Fatalf("second call after cancel must be admitted")
	}
	if !ok2 {
		t.Errorf("second call must succeed; got ok=false path=%q", path2)
	}
	if path2 != "/tmp/example.ddshare" {
		t.Errorf("second call must return the picked path; got %q", path2)
	}
}

// TestGuardedOpenDialogRecorders makes sure the new helpers
// return the 3-value shape (path, admitted, ok) the handlers
// rely on. A future refactor that flips the signature back to
// 2-value fails this regression net.
func TestGuardedOpenDialogRecorders(t *testing.T) {
	app := NewApp()
	app.SetOpenFileDialogOverride(func(_ any) (string, error) {
		return "/tmp/example.ddshare", nil
	})
	opts := runtime.OpenDialogOptions{
		Filters: []runtime.FileFilter{{DisplayName: "Archive", Pattern: "*.ddshare"}},
	}
	dupKey := guardedOpenFileDialogKey("shared_archive", opts)
	path, admitted, ok := app.guardedOpenFileDialog(dupKey, opts)
	if path == "" || !admitted || !ok {
		t.Errorf("happy-path guard returned path=%q admitted=%v ok=%v; want non-empty path, true, true", path, admitted, ok)
	}

	// And the response writer plumbing (respondDuplicateInFlight) is
	// still the right escape hatch for the dup-hit branch.
	rec := httptest.NewRecorder()
	app.respondDuplicateInFlight(rec, httptest.NewRequest("POST", "/import/shared", nil), dupKey)
	if rec.Code != http.StatusOK {
		t.Errorf("dup-hit must return 200 (Option C contract: dispatchDixieDataForm navigates from X-DixieData-Redirect); got %d", rec.Code)
	}
	if rec.Header().Get("X-DixieData-Redirect") == "" {
		t.Errorf("dup-hit must set X-DixieData-Redirect; got empty")
	}
}

// TestHandleImportBackupDialogGuard covers the bug found in
// audit pass for issue #158: handleImportBackup used to call
// a.OpenFileDialog directly with no guard. A rapid
// double-click on the "Restore from backup" button could land
// two OpenFileDialog calls on the Wails UI thread; WebView2
// loses focus and crashes with Chrome_WidgetWin_0. Error = 1412.
//
// The fix wraps the OpenFileDialog call in enterInFlight +
// defer leaveInFlight (the inline pattern, same as
// handleCalendarPDF), keyed by guardedOpenFileDialogKey so a
// legitimate retry against a different file is still admitted.
//
// This test exercises the HTTP handler directly. The
// importInFlight atomic.Bool is left at its zero value so the
// first branch (the importInFlight.Load() check) does not
// short-circuit before we hit the dialog.
func TestHandleImportBackupDialogGuard(t *testing.T) {
	app := NewApp()

	// The handler builds the dupKey from the same dialogOpts
	// shape every time. We hold the slot manually so the
	// handler's enterInFlight is guaranteed to return false
	// on its first call (see the other guard tests in this
	// file for the rationale — the old time.Sleep / two-
	// goroutine race was flaky under heavy scheduler pressure,
	// issue #207).
	opts := runtime.OpenDialogOptions{
		Filters: []runtime.FileFilter{
			{DisplayName: "DixieData backup archive", Pattern: "*.ddbak"},
			{DisplayName: "Legacy backup archive", Pattern: "*.zip"},
		},
	}
	dupKey := guardedOpenFileDialogKey("backup_import", opts)
	if dupKey == "" {
		t.Fatal("guardedOpenFileDialogKey must produce a non-empty key")
	}
	admittedFirst, heldEntry := app.enterInFlight(dupKey)
	if !admittedFirst || heldEntry == nil {
		t.Fatalf("setup: failed to acquire held slot; admitted=%v entry=%v", admittedFirst, heldEntry)
	}

	// One POST to /import/backup. The slot is held, so the
	// handler's enterInFlight returns false and it issues the
	// dup-hit response (200 + X-DixieData-Redirect, per
	// Option C contract).
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/import/backup", nil)
	app.handleImportBackup(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("dup-hit must return 200 (Option C contract); got %d", rec.Code)
	}
	if rec.Header().Get("X-DixieData-Redirect") == "" {
		t.Errorf("dup-hit must set X-DixieData-Redirect; got empty")
	}

	// Release the slot so the second call is NOT a dup-hit.
	// The second call hits OpenFileDialog which returns "" (cancel)
	// by default, which the handler turns into a 400 with toast.
	// What matters is that it is NOT a dup-hit (no
	// X-DixieData-Redirect).
	app.leaveInFlight(dupKey, heldEntry)

	app.SetOpenFileDialogOverride(func(_ any) (string, error) {
		return "", nil
	})
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/import/backup", nil)
	app.handleImportBackup(rec2, req2)
	if rec2.Header().Get("X-DixieData-Redirect") != "" {
		t.Errorf("second POST must not be a dup-hit; got X-DixieData-Redirect=%q", rec2.Header().Get("X-DixieData-Redirect"))
	}
}
