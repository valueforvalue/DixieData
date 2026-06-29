package appshell

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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
	// Block the first call's dialog inside the helper so the
	// second goroutine has time to race past the dup check
	// before the slot is released. Without this barrier the
	// override returns immediately and the first call's defer
	// fires before the second call arrives, so the dup-check
	// would never be exercised.
	release := make(chan struct{})
	app.SetOpenFileDialogOverride(func(_ any) (string, error) {
		invocations.Add(1)
		<-release
		return "/tmp/example.ddshare", nil
	})

	opts := runtime.OpenDialogOptions{
		Filters: []runtime.FileFilter{
			{DisplayName: "DixieData shared archive", Pattern: "*.ddshare"},
		},
	}
	dupKey := guardedOpenFileDialogKey("shared_archive", opts)

	var wg sync.WaitGroup
	admitted := make(chan bool, 2)
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			_, ad, _ := app.guardedOpenFileDialog(dupKey, opts)
			admitted <- ad
		}()
	}

	// Wait for the first goroutine to reach the dialog override
	// so the second goroutine actually races the dup check while
	// the slot is held. Without this barrier close(release) can
	// fire before goroutine 1 even starts and the test passes
	// for the wrong reason (slot released before second call).
	for invocations.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	close(release)
	wg.Wait()
	close(admitted)

	admitCount := 0
	for v := range admitted {
		if v {
			admitCount++
		}
	}
	if admitCount != 1 {
		t.Fatalf("expected exactly 1 admit, got %d", admitCount)
	}
	if got := invocations.Load(); got != 1 {
		t.Fatalf("OpenFileDialog must be invoked exactly once; got %d", got)
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
	release := make(chan struct{})
	app.SetOpenMultipleFilesDialogOverride(func(_ any) ([]string, error) {
		invocations.Add(1)
		<-release
		return []string{"/tmp/a.png", "/tmp/b.jpg"}, nil
	})

	opts := runtime.OpenDialogOptions{
		Filters: []runtime.FileFilter{
			{DisplayName: "Image files", Pattern: "*.png;*.jpg"},
		},
	}
	dupKey := guardedOpenMultipleFilesDialogKey("import_images", opts)

	var wg sync.WaitGroup
	admitted := make(chan bool, 2)
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			_, ad, _ := app.guardedOpenMultipleFilesDialog(dupKey, opts)
			admitted <- ad
		}()
	}

	for invocations.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	close(release)
	wg.Wait()
	close(admitted)

	admitCount := 0
	for v := range admitted {
		if v {
			admitCount++
		}
	}
	if admitCount != 1 {
		t.Fatalf("expected exactly 1 admit, got %d", admitCount)
	}
	if got := invocations.Load(); got != 1 {
		t.Fatalf("OpenMultipleFilesDialog must be invoked exactly once; got %d", got)
	}
}

// TestOpenDialogGuardKeysDifferentiateByKind ensures the kind
// prefix on the dupKey keeps two different imports from
// blocking each other. A user can preview a memorial JSON
// while a shared-archive import is in flight; the two must
// not collide on the same dedup slot.
func TestOpenDialogGuardKeysDifferentiateByKind(t *testing.T) {
	optsA := runtime.OpenDialogOptions{
		Filters: []runtime.FileFilter{{DisplayName: "JSON", Pattern: "*.json"}},
	}
	optsB := runtime.OpenDialogOptions{
		Filters: []runtime.FileFilter{{DisplayName: "Archive", Pattern: "*.ddshare"}},
	}
	keyA := guardedOpenFileDialogKey("memorial_preview", optsA)
	keyB := guardedOpenFileDialogKey("shared_archive", optsB)
	if keyA == keyB {
		t.Fatalf("different kinds must produce different guard keys")
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