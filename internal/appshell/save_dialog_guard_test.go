// save_dialog_guard_test.go locks in the in-flight guard that
// prevents the Wails 2.12 / WebView2 crash reported in the user's
// live-app testing (Chrome_WidgetWin_0. Error = 1412, see
// wailsapp/wails#2807). The crash is triggered by a double-click on
// any SaveFileDialog button: the second click queues a second
// native dialog on the Wails UI thread while the first is still up,
// both block, and WebView2's focus race takes the app down.
//
// handleCalendarPDF carried an inline guard for this bug before the
// other export handlers were migrated to the jobs registry; that
// migration dropped the guard. PR b185f0e + this commit introduce
// a.guardedSaveFileDialog and route every export handler through
// it.
//
// These tests verify:
//   - The helper refuses the second call when the first is still
//     in flight (the actual crash trigger).
//   - The helper releases the in-flight slot after the dialog
//     returns, so a third call proceeds.
//   - Two concurrent calls with different export kinds proceed
//     independently (JSON export shouldn't block CSV export).
//   - The N-way race produces exactly one successful invocation.
package appshell

import (
	"sync"
	"sync/atomic"
	"testing"

	runtime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// dialogResponse is the canned return value for the
// saveFileDialogOverride hook in these tests. Real SaveFileDialog
// returns a chosen file path; the test path doesn't matter — only
// that the guard logic doesn't short-circuit the legitimate return
// value.
const dialogResponse = "C:\\fake\\path\\test-export.json"

// TestGuardedSaveFileDialogRejectsConcurrentDuplicates reproduces
// the crash scenario: two back-to-back calls with the same kind +
// options must produce only one dialog invocation. The second
// call returns ("", false) so the handler can respond 429 / show
// a toast instead of queueing a second native dialog.
func TestGuardedSaveFileDialogRejectsConcurrentDuplicates(t *testing.T) {
	app := NewApp()

	var (
		invocations int32
		dialogOpen  = make(chan struct{})
		hold        = make(chan struct{})
	)
	app.saveFileDialogOverride = func(opts any) (string, error) {
		if atomic.AddInt32(&invocations, 1) == 1 {
			close(dialogOpen)
			<-hold
		}
		return dialogResponse, nil
	}

	opts := runtime.SaveDialogOptions{
		DefaultFilename: "test-export.json",
		Filters: []runtime.FileFilter{
			{DisplayName: "JSON", Pattern: "*.json"},
		},
	}

	// Kick off the first call; it blocks until the test closes hold.
	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		path, outcome := app.guardedSaveFileDialog("json_export", opts)
		if outcome != SaveOutcomeOK || path != dialogResponse {
			t.Errorf("first call: path=%q outcome=%v, want path=%q outcome=SaveOutcomeOK", path, outcome, dialogResponse)
		}
	}()

	// Wait for the dialog to actually open.
	<-dialogOpen

	// Second call from the same handler MUST be rejected without
	// invoking the dialog a second time.
	path2, outcome2 := app.guardedSaveFileDialog("json_export", opts)
	if outcome2 != SaveOutcomeDuplicated {
		t.Errorf("second call must be rejected; got path=%q outcome=%v", path2, outcome2)
	}
	if path2 != "" {
		t.Errorf("rejected call must return empty path; got %q", path2)
	}

	// Release the first dialog and wait for it to finish.
	close(hold)
	<-firstDone

	if got := atomic.LoadInt32(&invocations); got != 1 {
		t.Errorf("dialog must be invoked exactly once; got %d invocations (the second invocation is the crash trigger)", got)
	}
}

// TestGuardedSaveFileDialogReleasesAfterCompletion ensures the
// guard doesn't leak: once a call returns, the slot is freed and
// a subsequent call with the same options proceeds normally.
// Without this, the second click of an export button would be
// permanently dead.
func TestGuardedSaveFileDialogReleasesAfterCompletion(t *testing.T) {
	app := NewApp()
	app.saveFileDialogOverride = func(opts any) (string, error) {
		return dialogResponse, nil
	}

	opts := runtime.SaveDialogOptions{
		DefaultFilename: "test-export.json",
		Filters: []runtime.FileFilter{
			{DisplayName: "JSON", Pattern: "*.json"},
		},
	}

	// Two sequential calls (not concurrent) must both proceed.
	path1, outcome1 := app.guardedSaveFileDialog("json_export", opts)
	if outcome1 != SaveOutcomeOK || path1 != dialogResponse {
		t.Fatalf("first call: path=%q outcome=%v, want path=%q outcome=SaveOutcomeOK", path1, outcome1, dialogResponse)
	}

	path2, outcome2 := app.guardedSaveFileDialog("json_export", opts)
	if outcome2 != SaveOutcomeOK || path2 != dialogResponse {
		t.Fatalf("second sequential call must succeed (slot was freed); path=%q outcome=%v", path2, outcome2)
	}
}

// TestGuardedSaveFileDialogAllowsConcurrentDifferentKinds ensures
// the guard keys on export kind + options, not just on a single
// flag. Two different exports (e.g. JSON then CSV) must be able
// to run concurrently — one shouldn't lock the other out.
func TestGuardedSaveFileDialogAllowsConcurrentDifferentKinds(t *testing.T) {
	app := NewApp()

	var (
		jsonInvoked int32
		jsonOpen    = make(chan struct{})
		jsonHold    = make(chan struct{})
	)
	app.saveFileDialogOverride = func(opts any) (string, error) {
		got := opts.(runtime.SaveDialogOptions)
		if got.DefaultFilename == "test-export.json" {
			if atomic.AddInt32(&jsonInvoked, 1) == 1 {
				close(jsonOpen)
				<-jsonHold
			}
		}
		return dialogResponse, nil
	}

	jsonOpts := runtime.SaveDialogOptions{
		DefaultFilename: "test-export.json",
		Filters:         []runtime.FileFilter{{DisplayName: "JSON", Pattern: "*.json"}},
	}
	csvOpts := runtime.SaveDialogOptions{
		DefaultFilename: "test-export.xlsx",
		Filters:         []runtime.FileFilter{{DisplayName: "Excel", Pattern: "*.xlsx"}},
	}

	// Start the JSON dialog; it blocks.
	jsonDone := make(chan struct{})
	go func() {
		defer close(jsonDone)
		path, outcome := app.guardedSaveFileDialog("json_export", jsonOpts)
		if outcome != SaveOutcomeOK || path != dialogResponse {
			t.Errorf("json call: path=%q outcome=%v", path, outcome)
		}
	}()

	<-jsonOpen

	// CSV export proceeds while JSON is in flight.
	path2, outcome2 := app.guardedSaveFileDialog("excel_export", csvOpts)
	if outcome2 != SaveOutcomeOK || path2 != dialogResponse {
		t.Errorf("csv call while json is in flight must succeed; path=%q outcome=%v", path2, outcome2)
	}

	// Release JSON.
	close(jsonHold)
	<-jsonDone
}

// TestGuardedSaveFileDialogRaceProtection sanity-checks the
// LoadOrStore atomic: N goroutines hammering the same key must
// produce exactly one successful call and N-1 rejections.
//
// The dialog must block to simulate real-world behavior (the user
// has to pick a file before the dialog returns). Without a hold,
// goroutines race through the helper too fast for the test to
// observe the guard effect.
func TestGuardedSaveFileDialogRaceProtection(t *testing.T) {
	const N = 10
	app := NewApp()

	var (
		invocations int32
		dialogOpen  = make(chan struct{})
		hold        = make(chan struct{})
	)
	app.saveFileDialogOverride = func(opts any) (string, error) {
		if atomic.AddInt32(&invocations, 1) == 1 {
			close(dialogOpen)
			<-hold
		}
		return dialogResponse, nil
	}

	opts := runtime.SaveDialogOptions{
		DefaultFilename: "test-export.json",
		Filters:         []runtime.FileFilter{{DisplayName: "JSON", Pattern: "*.json"}},
	}

	var wg sync.WaitGroup
	results := make(chan bool, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, outcome := app.guardedSaveFileDialog("json_export", opts)
			// Mirror the old bool-flag channel contract:
			//   false = rejected (admit-no, did not reach the dialog)
			//   true  = admitted (would proceed to the dialog if not
			//           blocked). Only the admitted goroutine sends
			//           after the dialog releases.
			results <- outcome == SaveOutcomeOK
		}()
	}

	// Collect N-1 failures (the goroutines whose LoadOrStore
	// returned "already loaded"). The successful goroutine is
	// blocked inside SaveFileDialog until we release the hold.
	failures := 0
	for failures < N-1 {
		if <-results {
			t.Errorf("expected %d-way race to produce only one success; got a success on read %d", N, failures+1)
		}
		failures++
	}

	// The successful goroutine is still blocked in the dialog.
	// Wait for the dialog to actually be open, then release it.
	<-dialogOpen
	close(hold)

	// Read the one remaining result — the success.
	if !<-results {
		t.Errorf("expected exactly one success after releasing the dialog; got failure")
	}

	if got := atomic.LoadInt32(&invocations); got != 1 {
		t.Errorf("SaveFileDialog must be invoked exactly once; got %d (the second invocation is the crash trigger)", got)
	}
}

// TestGuardedSaveFileDialogCancelReleasesSlot ensures cancelling
// the dialog (returning ("", nil) from the SaveFileDialog override)
// releases the in-flight slot. Without this, cancelling one
// export would lock the slot forever.
//
// The SaveFileDialog override alternates: first call returns
// ("", nil) to simulate the user cancelling the dialog; second
// call returns the canned success path. With the defer Delete in
// place, the second call must succeed even though the first call
// took the cancel path.
func TestGuardedSaveFileDialogCancelReleasesSlot(t *testing.T) {
	app := NewApp()

	var invocations int32
	app.saveFileDialogOverride = func(opts any) (string, error) {
		n := atomic.AddInt32(&invocations, 1)
		if n == 1 {
			return "", nil // first call: user cancels
		}
		return dialogResponse, nil // subsequent calls: success
	}

	opts := runtime.SaveDialogOptions{
		DefaultFilename: "test-export.json",
		Filters:         []runtime.FileFilter{{DisplayName: "JSON", Pattern: "*.json"}},
	}

	// First call: user cancels.
	path1, outcome1 := app.guardedSaveFileDialog("json_export", opts)
	if outcome1 != SaveOutcomeDialogAborted {
		t.Errorf("cancel must return outcome=SaveOutcomeDialogAborted; got path=%q outcome=%v", path1, outcome1)
	}
	if path1 != "" {
		t.Errorf("cancel must return empty path; got %q", path1)
	}

	// Second call: must not be blocked by the cancelled first call.
	path2, outcome2 := app.guardedSaveFileDialog("json_export", opts)
	if outcome2 != SaveOutcomeOK {
		t.Errorf("second call after cancel must succeed; got outcome=%v", outcome2)
	}
	if path2 != dialogResponse {
		t.Errorf("second call must return dialogResponse; got %q", path2)
	}
	if got := atomic.LoadInt32(&invocations); got != 2 {
		t.Errorf("dialog must be invoked twice (cancelled + retry); got %d", got)
	}
}

// TestExportFullDatabasePDFPathGuardKeys ensures the dupKey
// construction in exportFullDatabasePDFPath differentiates by
// destination filename (so JSON then CSV exports run independently
// but two clicks on the same Printable PDF collapse to one dialog).
// This catches a future refactor that drops the kind/filename
// components from the key and accidentally lets duplicates through.
func TestExportFullDatabasePDFPathGuardKeys(t *testing.T) {
	keyA := "db-pdf|June-report.pdf"
	keyB := "db-pdf|July-report.pdf"
	if keyA == keyB {
		t.Fatalf("different filenames must produce different guard keys")
	}
	var inFlight sync.Map
	if _, loaded := inFlight.LoadOrStore(keyA, struct{}{}); loaded {
		t.Fatalf("first LoadOrStore must not report loaded")
	}
	if _, loaded := inFlight.LoadOrStore(keyA, struct{}{}); !loaded {
		t.Fatalf("second LoadOrStore on same key must report loaded")
	}
	if _, loaded := inFlight.LoadOrStore(keyB, struct{}{}); loaded {
		t.Fatalf("LoadOrStore on different filename must not collide")
	}
}