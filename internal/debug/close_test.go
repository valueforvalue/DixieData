package debug

import "testing"

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
