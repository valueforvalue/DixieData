package debug

import (
	"errors"
	"testing"
)

// fakeCloser satisfies io.Closer and records whether Close was called.
type fakeCloser struct {
	called bool
	err    error
}

func (f *fakeCloser) Close() error {
	f.called = true
	return f.err
}

// TestDeferCloseLog_NoError verifies the helper invokes Close even on
// the nil-error path. Issue #384 / Slice 9 regression net.
func TestDeferCloseLog_NoError(t *testing.T) {
	fc := &fakeCloser{}
	closeFn := DeferCloseLog(fc, "test-component")
	closeFn()
	if !fc.called {
		t.Fatal("Close() was not invoked")
	}
}

// TestDeferCloseLog_WithError verifies the helper still invokes Close
// when it returns an error. The slog.Warn side is hard to test without
// intercepting slog.Default(); the structural assertion is that the
// closer ran (and didn't panic). The audit/smoke_swallowed_errors.mjs
// probe pins the log line shape via source-scan.
func TestDeferCloseLog_WithError(t *testing.T) {
	fc := &fakeCloser{err: errors.New("disk full")}
	closeFn := DeferCloseLog(fc, "test-component")
	closeFn() // must not panic
	if !fc.called {
		t.Fatal("Close() was not invoked even though it returned an error")
	}
}