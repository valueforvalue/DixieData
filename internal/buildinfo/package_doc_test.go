package buildinfo

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

// TestNoWrongStarterDocComments is the regression gate for Go Doc
// audit Phase 2. It walks every non-test .go file under internal/
// and checks that every exported identifier's doc comment starts
// with the identifier name (or a recognized variant).
//
// A "wrong-starter" is a comment like `// --- NewApp ---` on a
// func NewApp, or `// NormalizeName ...` on a func NormalizeTagName.
// go doc accepts these comments but the rendered synopsis line
// begins with the wrong word; tooling that ingests godoc output
// loses the identifier-name anchor.
//
// Heuristic: the contiguous `//` block immediately above an
// exported identifier, with the first word (after stripping
// separators like `// --- foo ---` and `// === foo ===`) matching
// the identifier name. Multi-paragraph comments split by blank
// lines are checked against the LAST paragraph (which is the one
// actually bound to the identifier by go doc's convention).
//
// Filter: skip function-body prose like `// (no fmt usage here)`.
// We catch those by requiring the first word of the doc comment
// to start with the identifier name; if the comment is just
// parenthetical or general prose, it fails the check and the
// operator can decide whether to delete it or rewrite it as a
// proper doc comment.
//
// This is intentionally strict: false positives are acceptable
// because the failure message prints the file:line + identifier
// name + first word, which is enough context to fix.
func TestNoWrongStarterDocComments(t *testing.T) {
	root := repoRoot(t)
	var findings []wrongStarterFinding
	for _, path := range allGoFiles(t, root) {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatalf("filepath.Rel: %v", err)
		}
		if strings.HasPrefix(rel, ".scratch"+string(filepath.Separator)) {
			continue
		}
		if strings.HasPrefix(rel, "cmd"+string(filepath.Separator)) {
			continue
		}
		findings = append(findings, scanWrongStarters(path)...)
	}
	if len(findings) > 0 {
		for _, f := range findings {
			t.Errorf("wrong-starter doc comment at %s:%d for %s: first word = %q",
				f.File, f.Line, f.Name, f.First)
		}
	}
}

type wrongStarterFinding struct {
	File  string
	Line  int
	Name  string
	First string
}

// allGoFiles returns every non-test .go file under root, skipping
// directories that should not be scanned (.scratch, etc.).
func allGoFiles(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == ".scratch" || base == "vendor" || base == "node_modules" || base == "build" || base == "release" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		out = append(out, path)
		return nil
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	return out
}

// scanWrongStarters returns every exported-identifier-in-this-file
// whose doc comment's first word doesn't match the identifier name.
func scanWrongStarters(path string) []wrongStarterFinding {
	var findings []wrongStarterFinding
	text, err := os.ReadFile(path)
	if err != nil {
		return findings
	}
	lines := strings.Split(string(text), "\n")

	// Top-level decl lines start at column 0 (no leading whitespace).
	// We only care about top-level exported funcs/types/vars/consts.
	declRe := regexp.MustCompile(`^(func|type|var|const)\s+(?:\([^)]+\)\s+)?([A-Z]\w*)`)

	for i, line := range lines {
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		m := declRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name := m[2]

		// Walk backwards to find the contiguous doc-comment block.
		j := i - 1
		for j >= 0 && strings.TrimSpace(lines[j]) == "" {
			j--
		}
		var block []string
		for j >= 0 && strings.HasPrefix(strings.TrimSpace(lines[j]), "//") {
			trimmed := strings.TrimSpace(lines[j])
			// Skip pure-separator lines (`// --- foo ---`,
			// `// === foo ===`) and Issue-group markers
			// (`// Issue #XXX: ...`) — these are not the
			// identifier's doc comment, they're prose.
			stripped := strings.TrimLeft(trimmed, "/")
			stripped = strings.TrimSpace(stripped)
			if strings.HasPrefix(stripped, "---") ||
				strings.HasPrefix(stripped, "===") ||
				strings.HasPrefix(stripped, "Issue #") {
				j--
				continue
			}
			block = append([]string{trimmed}, block...)
			j--
		}
		if len(block) == 0 {
			continue // no doc comment, handled by Phase 3
		}

		// First word of the first line of the block.
		first := strings.TrimLeft(block[0], "/")
		first = strings.TrimSpace(first)
		wordRe := regexp.MustCompile(`^(\w+)`)
		if wm := wordRe.FindStringSubmatch(first); wm != nil {
			fw := wm[1]
			if !strings.HasPrefix(fw, name) {
				findings = append(findings, wrongStarterFinding{
					File:  path,
					Line:  i + 1,
					Name:  name,
					First: fw,
				})
			}
		}
	}
	return findings
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

// isPredominantlyTemplGenerated returns true when a majority of
// the non-test .go files in the directory start with the templ
// generator's "Code generated by templ; DO NOT EDIT" header. These
// packages are templ-rendered components; adding doc comments to
// the generated symbols is churn that disappears on the next
// `make tpl`, so we skip them from the coverage floor.
func isPredominantlyTemplGenerated(t *testing.T, dir string) bool {
	t.Helper()
	entries, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("Glob %s: %v", dir, err)
	}
	total := 0
	templ := 0
	for _, e := range entries {
		base := filepath.Base(e)
		if strings.HasSuffix(base, "_test.go") {
			continue
		}
		total++
		data, err := os.ReadFile(e)
		if err != nil {
			t.Fatalf("ReadFile %s: %v", e, err)
		}
		if strings.Contains(string(data), "Code generated by templ") {
			templ++
		}
	}
	if total == 0 {
		return false
	}
	return templ*2 >= total // >=50%
}

// docCoverageForPackage runs the audit script for a single package
// directory and returns (total_exported, documented, coverage_pct).
// Uses go list + go doc to compute the numbers — the same source
// of truth the audit script uses, but inlined here so the test
// runs in-process without spawning Python.
func docCoverageForPackage(t *testing.T, pkgPath string) (int, int, float64) {
	t.Helper()
	// Get every exported identifier in the package via go doc.
	cmd := exec.Command("go", "doc", "-all", "-short", pkgPath)
	cmd.Dir = repoRoot(t)
	out, err := cmd.Output()
	if err != nil {
		// Package has no documentation yet (e.g. all unexported).
		// Treat as 0/0.
		return 0, 0, 100.0
	}
	text := string(out)
	lines := strings.Split(text, "\n")

	// Heuristic: count exported symbols. go doc -all lists each
	// exported func/type/var/const as a line starting with `func`,
	// `type`, `var`, or `const`. We count those AND lines that
	// start with one of those followed by the identifier (the
	// format go doc uses for indented methods).
	exportedCount := 0
	documentedCount := 0
	declRe := regexp.MustCompile(`^(func|type|var|const)\s+([A-Z]\w*)`)
	methodRe := regexp.MustCompile(`^\s+(func|type)\s+(\(\w+[\*\s\w]*\)\s+)?([A-Z]\w*)`)
	for i, line := range lines {
		if declRe.MatchString(line) {
			exportedCount++
			// go doc -all -short puts the doc text BELOW the decl
			// line, indented. The first non-empty indented line
			// after the decl IS the doc. If the next line is
			// empty, no doc. If it's at column 0, no doc.
			hasDoc := false
			if i+1 < len(lines) {
				next := lines[i+1]
				trimmed := strings.TrimSpace(next)
				indented := strings.HasPrefix(next, " ") || strings.HasPrefix(next, "\t")
				if trimmed != "" && indented {
					hasDoc = true
				}
			}
			if hasDoc {
				documentedCount++
			}
			continue
		}
		if m := methodRe.FindStringSubmatch(line); m != nil {
			exportedCount++
			// Same BELOW-the-decl check (methods also get indented
			// doc text under go doc -all -short).
			hasDoc := false
			if i+1 < len(lines) {
				next := lines[i+1]
				trimmed := strings.TrimSpace(next)
				indented := strings.HasPrefix(next, " ") || strings.HasPrefix(next, "\t")
				if trimmed != "" && indented {
					hasDoc = true
				}
			}
			if hasDoc {
				documentedCount++
			}
		}
	}

	if exportedCount == 0 {
		return 0, 0, 100.0
	}
	pct := float64(documentedCount) / float64(exportedCount) * 100.0
	return exportedCount, documentedCount, pct
}

// TestPerPackageDocCoverageFloor is the regression gate for the
// Go Doc coverage requirement (CONTEXT.md §Laws: "Exported Go
// identifiers carry doc comments"). For every Go package under
// internal/ and pkg/ with at least 5 exported identifiers,
// asserts that the documented fraction is at least 70%. Packages
// below the floor are listed in the failure output with their
// (exported, documented, pct) counts so the operator knows
// exactly which packages to address.
//
// The 70% per-package floor is a regression gate, not a target;
// the working rule is "aim for 100% on every new PR" (per the
// law). The test exists so a future commit that strips docs in
// bulk gets caught; it is not a license to land a 70% patch.
//
// The test uses `go doc -all -short` to count (under-counts vs.
// the audit script for packages that re-export many type
// aliases); the audit script
// (.scratch/audit/go_doc_audit.py) is the source of truth for
// the overall metric. This test is the regression gate, not
// the audit.
//
// Packages with < 5 exported identifiers are skipped because the
// percentage metric is too noisy for tiny packages.
func TestPerPackageDocCoverageFloor(t *testing.T) {
	root := repoRoot(t)
	packages := discoverPackages(t, root)

	type result struct {
		pkg       string
		exported  int
		documented int
		pct       float64
	}
	var fails []result
	for _, pkg := range packages {
		rel, err := filepath.Rel(root, pkg)
		if err != nil {
			t.Fatalf("filepath.Rel: %v", err)
		}
		// Skip module root, cmd/* (unexported main), .scratch/,
		// packages with no .go source, packages whose every file
		// has a //go:build directive, and packages whose files
		// are predominantly templ-generated (the templ generator
		// emits an exported `templ.Component` per template, and
		// adding doc comments to those is churn that disappears
		// on the next `make tpl`).
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
		if isPredominantlyTemplGenerated(t, pkg) {
			continue
		}

		exp, doc, pct := docCoverageForPackage(t, pkg)
		if exp < 5 {
			continue
		}
		if pct < 70.0 {
			fails = append(fails, result{rel, exp, doc, pct})
		}
	}
	if len(fails) > 0 {
		for _, f := range fails {
			t.Errorf("package %q has %d/%d (%.1f%%) exported identifiers documented; below the 70%% floor (see CONTEXT.md §Laws: 'Exported Go identifiers carry doc comments')",
				f.pkg, f.documented, f.exported, f.pct)
		}
	}
}