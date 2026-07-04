package appshell

import (
	"testing"

	"github.com/valueforvalue/DixieData/internal/leaktest"
)

// TestMain installs goleak.VerifyTestMain to catch the goroutine
// leak class documented in docs/COMMON_BUGS.md §4.1. appshell is
// the largest test surface in the codebase (60+ test files) and
// the package most likely to surface real leaks — its handlers
// spawn goroutines for SSE / progress / async jobs.
//
// Refs issue #318 Slice 2.
func TestMain(m *testing.M) { leaktest.Run(m) }
