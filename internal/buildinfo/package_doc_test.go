package buildinfo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestEveryInternalPackageHasSynopsis is the regression check for the
// Go Doc audit Phase 1. It walks every package under internal/ and
// pkg/ and asserts that `go doc` returns a non-empty synopsis
// paragraph — i.e. the package has a `// Package foo ...` comment
// at the top of at least one of its files.
//
// The test is part of the buildinfo package because that's the
// smallest stable package with no other test files; running it as
// part of `go test ./internal/buildinfo` is one line in CI.
//
// If a new package is added without a package-doc comment, this
// test fails until the comment is added. The list below is the
// fixed set of internal + pkg packages at the time the audit
// shipped Phase 1; cmd/ packages are excluded because they have
// unexported `main` packages whose doc comments are not surfaced
// by `go doc ./cmd/<name>`.
//
// Failure mode: prints the package path + the missing-or-empty
// synopsis so the operator can fix it with one `// Package foo
// ...` line.
func TestEveryInternalPackageHasSynopsis(t *testing.T) {
	root := repoRoot(t)
	packages := discoverPackages(t, root)
	if len(packages) == 0 {
		t.Fatal("no packages discovered; check the test's path filters")
	}

	for _, pkg := range packages {
		rel, err := filepath.Rel(root, pkg)
		if err != nil {
			t.Fatalf("filepath.Rel(%q, %q): %v", root, pkg, err)
		}
		// Skip the module root (no Go source files), cmd/*
		// (unexported main packages), the audit scratch
		// directory, any package whose directory has no .go
		// source files, and any package with //go:build
		// directives on every file (go doc cannot render the
		// synopsis for those without the matching tag).
		if rel == "." {
			continue
		}
		if strings.HasPrefix(rel, "cmd"+string(filepath.Separator)) {
			continue
		}
		if strings.HasPrefix(rel, ".scratch"+string(filepath.Separator)) {
			continue
		}
		if !hasGoSource(t, pkg) {
			continue
		}
		if hasOnlyBuildTaggedFiles(t, pkg) {
			continue
		}

		synopsis := goDocSynopsis(t, root, pkg)
		if synopsis == "" {
			t.Errorf("package %q has no // Package foo ... synopsis; add one at the top of any file in the package\n  hint: `// Package <name> <one-sentence purpose>` above the `package <name>` line",
				rel)
		}
	}
}

// goDocSynopsis runs `go doc <pkg-path>` and returns the synopsis
// paragraph (everything after the `package foo // import "..."`
// header line, before the first blank line). Empty string means
// the package has no package-doc comment.
//
// Heuristic: `go doc` for a package WITHOUT a synopsis comment
// prints the header line followed directly by the symbol list
// (func / type / var / const), with no blank line. We detect that
// by checking whether the first non-header line begins with one
// of those Go keywords; if so, the synopsis is empty.
//
// IMPORTANT: `go doc <abs-path>` works fine on Go's CLI but the
// cmd.Dir must be the repo root so go's module resolution finds
// the package.
func goDocSynopsis(t *testing.T, repoRoot, pkgPath string) string {
	t.Helper()
	cmd := exec.Command("go", "doc", pkgPath)
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = string(ee.Stderr)
		}
		t.Fatalf("go doc %s: %v\nstderr: %s", pkgPath, err, stderr)
	}
	text := string(out)

	lines := strings.Split(text, "\n")
	if len(lines) < 2 {
		return ""
	}
	// Drop the header line (`package foo // import "..."`).
	body := lines[1:]
	// Drop any leading blank lines.
	for len(body) > 0 && strings.TrimSpace(body[0]) == "" {
		body = body[1:]
	}
	if len(body) == 0 {
		return ""
	}
	// If the first non-blank line starts with a Go declaration
	// keyword, there is no synopsis — `go doc` is showing the
	// symbol list directly. Empty string = no synopsis.
	first := strings.TrimSpace(body[0])
	switch {
	case strings.HasPrefix(first, "func "),
		strings.HasPrefix(first, "type "),
		strings.HasPrefix(first, "var "),
		strings.HasPrefix(first, "const "):
		return ""
	}
	// Otherwise, the first non-blank lines are the synopsis.
	// Take them up to the next blank line.
	var synopsisLines []string
	for _, line := range body {
		if strings.TrimSpace(line) == "" {
			break
		}
		synopsisLines = append(synopsisLines, strings.TrimSpace(line))
	}
	return strings.Join(synopsisLines, " ")
}

// discoverPackages returns every Go package under the repo root
// using `go list ./...`. This is the canonical Go discovery; we
// don't reinvent it with a filesystem walk because go list handles
// build tags, vendoring, and excluded paths correctly.
//
// IMPORTANT: `go list ./...` walks from the CURRENT working
// directory, not the package containing the test. We must set
// cmd.Dir to the repo root so the discovery finds every package
// in the module, not just the one this test lives in.
func discoverPackages(t *testing.T, root string) []string {
	t.Helper()
	cmd := exec.Command("go", "list", "-f", "{{.Dir}}", "./...")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = string(ee.Stderr)
		}
		t.Fatalf("go list ./...: %v\nstderr: %s", err, stderr)
	}
	var dirs []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			dirs = append(dirs, line)
		}
	}
	return dirs
}

// repoRoot resolves the module root via `go list -m`. This is the
// standard way to find the project root from inside a Go test
// regardless of the working directory the user invokes it from.
func repoRoot(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Dir}}")
	cmd.Dir = "."
	// Try to chdir to the repo root if we're not there. The
	// fallback is `go list -m` which works from anywhere within
	// the module.
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -m: %v", err)
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		t.Fatal("could not resolve module root")
	}
	return root
}

// hasGoSource returns true if the directory contains at least one
// non-test .go source file. Packages that only have _test.go files
// (e.g. internal/architecture, which holds only test code) cannot
// be processed by `go doc` and must be skipped.
func hasGoSource(t *testing.T, dir string) bool {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("Glob %s: %v", dir, err)
	}
	for _, m := range matches {
		base := filepath.Base(m)
		if !strings.HasSuffix(base, "_test.go") {
			return true
		}
	}
	return false
}

// hasOnlyBuildTaggedFiles returns true if every non-test .go file
// in the directory starts with a `//go:build` directive. For those
// packages, `go doc` without the matching build tag cannot render
// the synopsis paragraph (the package appears empty); the test
// skips them to avoid false positives.
//
// This is conservative: if ANY file in the package lacks the
// build tag, the package is checked normally. The only packages
// affected today are build-tag-gated siblings like
// internal/debug/trace which has both `debug` and `!debug`
// variants.
func hasOnlyBuildTaggedFiles(t *testing.T, dir string) bool {
	t.Helper()
	entries, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("Glob %s: %v", dir, err)
	}
	allTagged := true
	anyNonTest := false
	for _, e := range entries {
		base := filepath.Base(e)
		if strings.HasSuffix(base, "_test.go") {
			continue
		}
		anyNonTest = true
		data, err := os.ReadFile(e)
		if err != nil {
			t.Fatalf("ReadFile %s: %v", e, err)
		}
		if !strings.HasPrefix(string(data), "//go:build") {
			allTagged = false
			break
		}
	}
	return anyNonTest && allTagged
}