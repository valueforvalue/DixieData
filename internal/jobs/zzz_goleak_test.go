package jobs

import (
	"testing"

	"github.com/valueforvalue/DixieData/internal/leaktest"
)

// TestMain installs goleak.VerifyTestMain to catch the goroutine
// leak class documented in docs/COMMON_BUGS.md §4.1. A test that
// starts a worker without calling reg.Cancel(id) (or returns
// without draining its subscription) will fail the package
// suite with the leaked goroutine's stack trace.
//
// Refs issue #318 Slice 2.
func TestMain(m *testing.M) { leaktest.Run(m) }
