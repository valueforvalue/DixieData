// Package leaktest provides a shared TestMain wrapper that
// installs goleak.VerifyTestMain as the per-package leak detector.
//
// DixieData's `internal/` packages each run as their own `go test`
// process, so a `TestMain` declared in the root `main_test.go` only
// gates the root package — it has no visibility into
// `internal/jobs`, `internal/appshell`, etc. To get leak coverage
// in the packages where leaks actually occur, each test-bearing
// package that opts in calls `leaktest.Run(m)` from its own
// `TestMain`.
//
// The default matcher ignores a small set of well-known stdlib
// background goroutines so the gate stays useful (i.e. it surfaces
// DixieData leaks, not net/http's persistConn loop). New ignores
// should be added with a comment explaining the source — every
// entry is a future silent regression.
//
// Refs issue #318 Slice 2.
package leaktest

import (
	"os"
	"testing"

	"go.uber.org/goleak"
)

// Run is the per-package TestMain body. Call from a TestMain:
//
//	func TestMain(m *testing.M) { leaktest.Run(m) }
//
// Returns the process exit code via os.Exit.
func Run(m *testing.M) {
	goleak.VerifyTestMain(
		m,
		// net/http background goroutines that the standard library
		// keeps alive for keepalive / read loops. These are not
		// DixieData leaks; suppressing them keeps the signal clean.
		goleak.IgnoreTopFunction("net/http.(*persistConn).writeLoop"),
		goleak.IgnoreTopFunction("net/http.(*persistConn).readLoop"),
		goleak.IgnoreTopFunction("net/http.http2ClientConnReadLoop"),
		goleak.IgnoreTopFunction("net/http.http2ServerConnKeepAlive"),
		goleak.IgnoreTopFunction("net/http.(*http2ClientConn).readLoop"),
		// runtime poller — not DixieData.
		goleak.IgnoreTopFunction("internal/poll.runtime_pollWait"),
	)
	os.Exit(m.Run())
}
