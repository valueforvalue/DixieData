// Command lintrules runs the DixieData swallowed-error analyzers
// against the current Go module. It is the Go half of the lint
// enforcement layer that keeps the #384 + #436 sweeps from
// regressing (see docs/adr/0010-lint-enforcement.md and issue #438).
//
// Usage (from the DixieData repo root, NOT from tools/lintrules):
//
//	go run ./tools/lintrules/cmd/lintrules ./...
//
// Or from inside tools/lintrules:
//
//	go run ./cmd/lintrules ../...
//
// Note: unitchecker-based analyzers must be invoked through
// `go vet` semantics — calling the compiled binary directly
// is not supported. The Makefile target `lint-swallowed-errors`
// invokes the analyzer as `go run` against the DixieData
// module.
package main

import (
	"github.com/valueforvalue/DixieData/tools/lintrules/deferclose"
	"golang.org/x/tools/go/analysis/unitchecker"
)

func main() {
	unitchecker.Main(
		deferclose.Analyzer,
		// baretempl.Analyzer — added in slice 2 (issue #438).
	)
}
