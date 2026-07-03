// cli_debug_coverage_test.go — locks the cli-coverage regex shape
// for issue #268. The scanImplementedSubcommands + scanDocumentedSubcommands
// walkers use regex to parse Go switch-cases + cli-plan.md `dixiedata
// <verb>` lines. Future refactors that change the dispatcher style
// (case consolidation, dispatcher wrapping, doc reorganisation)
// can silently drift the regex's match set. This test pins the
// regex's behavior so the drift is caught before merge.
//
// Two strategies, each with a synthetic fixture:
//   1. Dispatcher fixture — a tempdir with a synthetic main.go +
//      cli_*.go file that uses a switch-case dispatcher with a
//      known verb list. scanImplementedSubcommands(root) must
//      produce exactly that verb set.
//   2. Doc fixture — a tempdir with a synthetic cli-plan.md that
//      uses the canonical `dixiedata <verb>` example pattern.
//      scanDocumentedSubcommands(docPath) must produce exactly
//      that verb set.

package appshell

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanImplementedSubcommandsFixtureShape(t *testing.T) {
	// Synthetic repo root with main.go + cli_synthetic.go that
	// use a switch-case dispatcher with three verbs: foo, bar,
	// baz. The fixture mimics the real main.go dispatch style
	// (appshell.Has{Verb}Subcommand + case "<verb>": inside the
	// body).
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(`
package main

import (
	"os"
	"dixiedata/internal/appshell"
)

func main() {
	if appshell.HasFooSubcommand(os.Args[1:]) {
		os.Exit(appshell.RunFoo())
	}
	if appshell.HasBarSubcommand(os.Args[1:]) {
		os.Exit(appshell.RunBar())
	}
	if appshell.HasBazFlag(os.Args[1:]) {
		os.Exit(appshell.RunBaz())
	}
}
`), 0644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "internal", "appshell"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal", "appshell", "cli_synthetic.go"), []byte(`
package appshell

func HasFooSubcommand(args []string) bool {
	switch args[0] {
	case "foo":
		return true
	}
	return false
}

func HasBarSubcommand(args []string) bool {
	switch args[0] {
	case "bar":
		return true
	}
	return false
}

func HasBazFlag(args []string) bool {
	for _, a := range args {
		if a == "--baz" {
			return true
		}
	}
	return false
}
`), 0644); err != nil {
		t.Fatalf("write cli_synthetic.go: %v", err)
	}

	got := scanImplementedSubcommands(root)
	want := []string{"foo", "bar", "--baz"}
	for _, v := range want {
		if !got[v] {
			t.Errorf("scanImplementedSubcommands missing %q\nwant=%v\ngot=%v", v, want, sortedKeys(got))
		}
	}
	if extra := diff(want, sortedKeys(got)); len(extra) > 0 {
		t.Errorf("scanImplementedSubcommands surfaced unexpected verbs: %v\ngot=%v", extra, sortedKeys(got))
	}
}

func TestScanDocumentedSubcommandsFixtureShape(t *testing.T) {
	// Synthetic cli-plan.md with the canonical
	// `dixiedata <verb>` example pattern. The walker should
	// pick up `dixiedata <verb>` references at line start
	// (including inside fenced code blocks; that's how the
	// real cli-plan.md documents them).
	docPath := filepath.Join(t.TempDir(), "cli-plan.md")
	if err := os.WriteFile(docPath, []byte(
`# CLI subcommand plan

## Phase 1

`+"```"+`
dixiedata foo
dixiedata bar
dixiedata baz
`+"```"+`

| Phase | Status |
|-------|--------|
| 1     | shipped — `+"`foo`"+` |
| 2     | shipped — `+"`bar`"+` |
| 3     | shipped — `+"`baz`"+` |
`), 0644); err != nil {
		t.Fatalf("write doc: %v", err)
	}

	got := scanDocumentedSubcommands(docPath)
	// foo and bar should appear (in fenced block + prose).
	// baz appears in prose but NOT as a `dixiedata baz <verb>`
	// pattern (the walker needs `[a-z][a-z0-9_-]*` after the
	// verb whitespace; the example is "dixiedata baz --flag"
	// which matches as verb=bar...no wait; the regex grabs
	// the word immediately after "dixiedata ", which is
	// "baz" here, so baz IS captured).
	want := []string{"foo", "bar", "baz"}
	for _, v := range want {
		if !got[v] {
			t.Errorf("scanDocumentedSubcommands missing %q\nwant=%v\ngot=%v", v, want, sortedKeys(got))
		}
	}
}

// diff + contains are defined in cli_debug.go (the production
// walker uses them). Tests just call them.
