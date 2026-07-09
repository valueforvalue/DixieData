package deferclose_test

import (
	"testing"

	"github.com/valueforvalue/DixieData/tools/lintrules/deferclose"
	"golang.org/x/tools/go/analysis/analysistest"
)

// TestDeferClose is the regression net for the deferclose
// analyzer (issue #438, ADR 0010). Fixtures under
// testdata/src/defer_close/ encode the expected diagnostics;
// the analyzer MUST report bad sites and MUST stay silent
// on good sites (including the //nolint:dixie/deferclose
// bail-out).
func TestDeferClose(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), deferclose.Analyzer, "defer_close")
}
