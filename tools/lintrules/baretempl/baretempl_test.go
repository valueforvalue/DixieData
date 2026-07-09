package baretempl_test

import (
	"testing"

	"github.com/valueforvalue/DixieData/tools/lintrules/baretempl"
	"golang.org/x/tools/go/analysis/analysistest"
)

// TestBareTempl is the regression net for the baretempl
// analyzer (issue #438, ADR 0010). Fixtures under
// testdata/src/bare_templ/ encode the expected diagnostics.
func TestBareTempl(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), baretempl.Analyzer, "bare_templ")
}
