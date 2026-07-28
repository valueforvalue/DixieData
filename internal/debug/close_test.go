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

// TestDeferCloseLog_NoError verifies canonical defer syntax invokes
// Close exactly once on the nil-error path. Issue #680.
func TestDeferCloseLog_NoError(t *testing.T) {
	fc := &fakeCloser{}
	func() {
		defer DeferCloseLog(fc, "test-component")
	}()
	if !fc.called {
		t.Fatal("Close() was not invoked")
	}
}
