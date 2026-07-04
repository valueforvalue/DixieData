package archive

import (
	"testing"

	"github.com/valueforvalue/DixieData/internal/leaktest"
)

// TestMain installs goleak.VerifyTestMain to catch the goroutine
// leak class documented in docs/COMMON_BUGS.md §4.1. archive is
// the integration-test hot spot (backup_service_*, dedup tests)
// and runs the longest suite in the codebase.
//
// Refs issue #318 Slice 2.
func TestMain(m *testing.M) { leaktest.Run(m) }
