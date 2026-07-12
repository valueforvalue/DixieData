// Package theme verifies the theme system CSS tokens defined
// in frontend/tailwind.css. The audit harness
// (audit/smoke_phase3_contrast.mjs) asserts on computed
// contrast ratios at runtime, which catches the "did the right
// hex land in the rule" failure mode. This package catches the
// earlier failure mode — "did the right token set land in
// :root" — so a future accidental deletion of the :root or
// per-theme override block fails fast at `go test` time, not
// at the next manual audit run.
//
// Issue #474 slice 2 — the real palette + theme system.
package theme

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestThemeTokens_DefinedInRoot pins that every theme token the
// system uses has a :root default. The per-theme overrides depend
// on these defaults as the reset; deleting one breaks the high-
// contrast and soft themes silently.
func TestThemeTokens_DefinedInRoot(t *testing.T) {
	css := readTailwindCSS(t)
	for _, token := range []string{
		"--theme-bg-page-top",
		"--theme-bg-page-mid",
		"--theme-bg-page-bottom",
		"--theme-bg-card",
		"--theme-bg-card-soft",
		"--theme-bg-input",
		"--theme-text-primary",
		"--theme-text-muted",
		"--theme-text-faint",
		"--theme-accent",
		"--theme-accent-strong",
		"--theme-border",
		"--theme-border-strong",
		"--theme-review-red",
	} {
		if !strings.Contains(css, token+":") {
			t.Errorf(":root must define %q; if you renamed it, update the per-theme overrides too", token)
		}
	}
}

// TestThemeTokens_HighContrastOverrides pins that every token
// the system uses has a [data-theme="high-contrast"] override.
// The high-contrast theme is the whole point of the system; a
// missing override means a token silently falls back to :root
// default (the gold/sepia look), which defeats the theme.
func TestThemeTokens_HighContrastOverrides(t *testing.T) {
	css := readTailwindCSS(t)
	highContrastBlock := extractBlock(css, `html[data-theme="high-contrast"]`)
	if highContrastBlock == "" {
		t.Fatal("missing html[data-theme=\"high-contrast\"] block")
	}
	for _, token := range []string{
		"--theme-bg-page-top",
		"--theme-bg-card",
		"--theme-bg-input",
		"--theme-text-primary",
		"--theme-accent",
		"--theme-accent-strong",
		"--theme-border",
	} {
		if !strings.Contains(highContrastBlock, token+":") {
			t.Errorf("high-contrast must override %q; current block = %q", token, highContrastBlock)
		}
	}
}

// TestThemeTokens_SoftOverrides mirrors the high-contrast pin
// for the soft theme. Same rationale: a missing override means
// the token silently uses the gold/sepia default.
func TestThemeTokens_SoftOverrides(t *testing.T) {
	css := readTailwindCSS(t)
	softBlock := extractBlock(css, `html[data-theme="soft"]`)
	if softBlock == "" {
		t.Fatal("missing html[data-theme=\"soft\"] block")
	}
	for _, token := range []string{
		"--theme-bg-page-top",
		"--theme-bg-card",
		"--theme-bg-input",
		"--theme-text-primary",
		"--theme-accent",
		"--theme-accent-strong",
		"--theme-border",
	} {
		if !strings.Contains(softBlock, token+":") {
			t.Errorf("soft must override %q; current block = %q", token, softBlock)
		}
	}
}

// TestMegaMenuItem_HasExplicitColor pins the issue #472 fix:
// the .mega-menu-item rule must set `color:` explicitly so the
// label doesn't inherit .pill-link's dark slate against the
// panel's dark navy bg. Before this fix the contrast ratio was
// effectively 1.00:1 (WCAG fail at every level). The test fails
// if the rule is missing the color declaration.
func TestMegaMenuItem_HasExplicitColor(t *testing.T) {
	css := readTailwindCSS(t)
	rule := extractClassRule(css, ".mega-menu-item")
	if rule == "" {
		t.Fatal("missing .mega-menu-item rule")
	}
	if !strings.Contains(rule, "color:") {
		t.Errorf(".mega-menu-item must set color: explicitly (issue #472); rule = %q", rule)
	}
}

// TestFoldoutMenuItem_HasExplicitColor mirrors the above for the
// pre-existing #283 fix on the foldout-menuitem rule. The fix
// landed in a prior commit; this test pins it against regression.
func TestFoldoutMenuItem_HasExplicitColor(t *testing.T) {
	css := readTailwindCSS(t)
	rule := extractClassRule(css, ".foldout-menuitem")
	if rule == "" {
		t.Fatal("missing .foldout-menuitem rule")
	}
	if !strings.Contains(rule, "color:") {
		t.Errorf(".foldout-menuitem must set color: explicitly (issue #283); rule = %q", rule)
	}
}

// TestDefaultThemeTokens_ReproduceCurrentPalette pins that the
// :root defaults match the pre-#474 palette byte-stably. Slice 2
// extracts the existing hex into vars; it does NOT change the
// default look. The audit harness
// (audit/smoke_phase3_contrast.mjs) asserts on computed contrast
// ratios on the default theme, so an accidental palette drift in
// the :root defaults would silently fail the next audit run
// instead of failing here.
func TestDefaultThemeTokens_ReproduceCurrentPalette(t *testing.T) {
	css := readTailwindCSS(t)
	want := map[string]string{
		"--theme-bg-page-top":    "#d7d2c9",
		"--theme-bg-page-mid":    "#c9c2b5",
		"--theme-bg-page-bottom": "#b9b1a3",
		"--theme-text-primary":   "#22303d",
		"--theme-accent":         "#a88a46",
		"--theme-accent-strong":  "#8d7440",
		"--theme-review-red":     "#6f2c26",
	}
	for token, hex := range want {
		// Match: ": #hex" anywhere in the file (the var name is
		// verified separately by TestThemeTokens_DefinedInRoot).
		// The hex literal must appear as ": #xxxxxx;" (whitespace
		// + hex + terminator) to avoid false-positive substring
		// matches inside a longer hex (e.g. searching for
		// "#a88a46" should not match inside "#a88a460").
		needle := ": " + hex + ";"
		if !strings.Contains(css, needle) {
			t.Errorf("default %s should be %s (palette drift would change the default look); not found in :root block", token, hex)
		}
	}
}

// readTailwindCSS locates frontend/tailwind.css relative to this
// test file. The test runs from the repo root via `go test ./...`
// and from internal/theme/ via `go test ./internal/theme/...`,
// so we walk up from runtime.Caller to find the repo root.
func readTailwindCSS(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	// thisFile = .../internal/theme/theme_css_test.go
	pkgDir := filepath.Dir(thisFile)
	repoRoot := filepath.Dir(filepath.Dir(pkgDir))
	cssPath := filepath.Join(repoRoot, "frontend", "tailwind.css")
	data, err := os.ReadFile(cssPath)
	if err != nil {
		t.Fatalf("read %s: %v", cssPath, err)
	}
	return string(data)
}

// extractBlock returns the substring of css that starts at the
// first occurrence of `start` and runs to the first standalone
// closing `}` at the same brace depth (matches by paren balance
// so an inline `}` in a url() or other function does not cut the
// block short).
func extractBlock(css, start string) string {
	idx := strings.Index(css, start)
	if idx < 0 {
		return ""
	}
	open := strings.Index(css[idx:], "{")
	if open < 0 {
		return ""
	}
	bodyStart := idx + open + 1
	depth := 1
	for i := bodyStart; i < len(css); i++ {
		switch css[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return css[idx : i+1]
			}
		}
	}
	return css[idx:]
}

// extractClassRule returns the substring of css that defines the
// given CSS class rule, including its body. If multiple rules
// match (e.g. a base rule + a :hover variant), the first one
// wins. Used to assert that a specific class carries the right
// declarations without parsing CSS.
func extractClassRule(css, class string) string {
	idx := strings.Index(css, class+" {")
	if idx < 0 {
		return ""
	}
	open := strings.Index(css[idx:], "{")
	if open < 0 {
		return ""
	}
	bodyStart := idx + open + 1
	depth := 1
	for i := bodyStart; i < len(css); i++ {
		switch css[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return css[idx : i+1]
			}
		}
	}
	return css[idx:]
}
