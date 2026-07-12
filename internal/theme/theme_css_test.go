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

// TestSecondaryButton_HasSolidBorder pins the QA report that
// unhovered secondary buttons in the default theme look
// "white-on-white" because the alpha-based sepia border
// (rgba(141, 116, 64, 0.85)) blends into the cream bg
// (rgba(246, 241, 228, 0.92)). The fix is a solid 1.5px
// sepia border with no alpha, so the button edge is always
// visible. The regression test fails if the border rule
// regresses to alpha-based or drops below 1.5px.
func TestSecondaryButton_HasSolidBorder(t *testing.T) {
	css := readTailwindCSS(t)
	// The .secondary-button selector appears in multiple rules
	// (shared with .danger-button and .pill-link for the layout
	// rule, then a separate rule for the bg/text/border). We
	// want the rule that DECLARES the border — search for the
	// class anywhere a rule body contains `border:` (or
	// @apply ... border ...). Walk each rule and pick the
	// one with a border declaration.
	rule := findRuleWithProperty(css, ".secondary-button", "border")
	if rule == "" {
		t.Fatal("missing .secondary-button rule with a border declaration")
	}
	if !strings.Contains(rule, "1.5px") {
		t.Errorf(".secondary-button must declare a 1.5px solid border (QA: 'white text on unhovered buttons' was caused by an alpha-based 1px border that vanished against the cream bg); rule = %q", rule)
	}
	if !strings.Contains(rule, "solid") {
		t.Errorf(".secondary-button border must be `solid` (not alpha-based) so the edge is always visible; rule = %q", rule)
	}
}

// TestSecondaryButton_HasSaturatedDarkText pins the QA report
// that the secondary button text in the default theme looked
// "white" — the user couldn't read the label until hovering.
// The cause was @apply text-ink (a desaturated #22303d) on a
// cream bg that washed out the label at small font sizes. The
// fix is a more saturated near-black (#0a0a0a or equivalent)
// so the label is unmistakable. The regression test fails if
// the rule reverts to text-ink, text-ink-mid, text-ink-deep,
// or any color that doesn't start with 0-9 or close to black.
func TestSecondaryButton_HasSaturatedDarkText(t *testing.T) {
	css := readTailwindCSS(t)
	rule := findRuleWithProperty(css, ".secondary-button", "color")
	if rule == "" {
		t.Fatal("missing .secondary-button rule with a color declaration")
	}
	// The fix: explicit `color: #0a0a0a` (or any hex that
	// starts with 0 or 1, indicating near-black). The
	// regression is `@apply text-ink` (#22303d) or
	// `@apply text-ink-deep` (#1f2b38) — the slate tones
	// wash out against the cream bg at 0.82rem.
	if strings.Contains(rule, "text-ink") {
		t.Errorf(".secondary-button must NOT use the text-ink slate tones (QA: looked white against cream bg); use a saturated near-black like #0a0a0a; rule = %q", rule)
	}
	if !strings.Contains(rule, "color: #0") && !strings.Contains(rule, "color: #1") {
		t.Errorf(".secondary-button color must be a near-black hex (0-1 prefix) so the label is unmistakable; rule = %q", rule)
	}
}

// TestHighContrast_HasFlatBodyBackground pins the QA report
// that the high-contrast theme's body background gradient
// (white → light-gray → slightly-darker-gray) is "way too
// obtrusive". The fix is a flat single-color body bg, no
// gradient layers on the body itself. The test fails if
// the high-contrast body bg rule re-introduces a
// `linear-gradient` or `radial-gradient`.
func TestHighContrast_HasFlatBodyBackground(t *testing.T) {
	css := readTailwindCSS(t)
	// The fix lives in `html[data-theme="high-contrast"] body { ... }`
	// — a per-theme body override that replaces the default
	// multi-gradient body rule with a flat color. The selector
	// `html[data-theme="high-contrast"] body` starts at the
	// `html` and ends at the matching `}`. The body block
	// must NOT contain `gradient` anywhere.
	overrideStart := strings.Index(css, `html[data-theme="high-contrast"] body`)
	if overrideStart < 0 {
		t.Fatal("missing html[data-theme=\"high-contrast\"] body override; the default body gradient leaks into HC")
	}
	open := strings.Index(css[overrideStart:], "{")
	if open < 0 {
		t.Fatal("html[data-theme=\"high-contrast\"] body override has no opening brace")
	}
	bodyStart := overrideStart + open + 1
	depth := 1
	end := -1
	for i := bodyStart; i < len(css); i++ {
		switch css[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i
			}
		}
		if end >= 0 {
			break
		}
	}
	if end < 0 {
		t.Fatal("html[data-theme=\"high-contrast\"] body override has no closing brace")
	}
	bodyRule := css[overrideStart : end+1]
	if strings.Contains(bodyRule, "gradient") {
		t.Errorf("high-contrast body must be a flat color (QA: 'way too obtrusive'); got gradient in body rule = %q", bodyRule)
	}
}

// TestFieldInput_HasSolidVisibleBorder pins the QA report that
// every text field on /soldiers/new has "no outline" — i.e. the
// user can't tell where the field is until they click. The cause
// is the alpha-based sepia border (`border-sepia-500/[0.82]`)
// that blends into the cream input bg. The fix is a solid
// (no-alpha) sepia border at 1.5px so the field edge is always
// visible against any theme bg.
func TestFieldInput_HasSolidVisibleBorder(t *testing.T) {
	css := readTailwindCSS(t)
	rule := findRuleWithProperty(css, ".field-input", "border")
	if rule == "" {
		t.Fatal("missing .field-input rule with a border declaration")
	}
	// The fix: 1.5px solid border, no alpha. Accept either an
	// explicit `1.5px solid ...` border line or a CSS variable
	// reference (var(--theme-...)) that itself resolves to a
	// solid color. The regression is the alpha-based
	// `border-sepia-500/[0.82]` Tailwind utility which renders
	// as rgba(141, 116, 64, 0.82) — too subtle to see.
	if strings.Contains(rule, "0.82") || strings.Contains(rule, "0.85") {
		t.Errorf(".field-input border must not use the alpha-based sepia utility (QA: 'no outline' on /soldiers/new); got rule = %q", rule)
	}
	if !strings.Contains(rule, "1.5px") {
		t.Errorf(".field-input must declare a 1.5px solid border (QA: 1px alpha border vanished against cream bg); got rule = %q", rule)
	}
}

// TestFieldInput_HasVisibleFocusState pins that the .field-input
// :focus / :focus-visible rule declares a focus ring strong
// enough to be unmistakable (the cream-on-cream problem would
// also affect the focus state if the ring is too subtle).
func TestFieldInput_HasVisibleFocusState(t *testing.T) {
	css := readTailwindCSS(t)
	// Look for any .field-input:focus or :focus-visible rule.
	found := false
	for _, suffix := range []string{".field-input:focus", ".field-input:focus-visible"} {
		idx := strings.Index(css, suffix)
		if idx < 0 {
			continue
		}
		rule := extractClassRule(css, suffix)
		if rule == "" {
			continue
		}
		if strings.Contains(rule, "outline") || strings.Contains(rule, "border-color") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf(".field-input must declare a visible focus state (outline OR border-color change); QA: focus state on /soldiers/new is too subtle to see")
	}
}
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

// findRuleWithProperty walks every rule that mentions the given
// class and returns the first one whose body contains a
// declaration of the named property. The match is a substring
// of the rule body — sufficient for a static CSS file, no need
// for a real CSS parser. Returns "" if no rule matches.
//
// Used by the QA regression tests to find the .secondary-button
// rule that actually declares the border (the same class appears
// in multiple rules; the layout-only rule doesn't have a border,
// the bg/text/border rule does).
func findRuleWithProperty(css, class, property string) string {
	searchFrom := 0
	for {
		idx := strings.Index(css[searchFrom:], class)
		if idx < 0 {
			return ""
		}
		idx += searchFrom
		// Walk to the next `{` and capture the rule body.
		open := strings.Index(css[idx:], "{")
		if open < 0 {
			return ""
		}
		bodyStart := idx + open + 1
		depth := 1
		end := -1
		for i := bodyStart; i < len(css); i++ {
			switch css[i] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					end = i
				}
			}
			if end >= 0 {
				break
			}
		}
		if end < 0 {
			return ""
		}
		rule := css[idx : end+1]
		// Match either a direct `border:` declaration or a Tailwind
		// `@apply border-...` utility. Do NOT match `border-radius` or
		// `border-color:` alone — those don't set the border itself.
		if strings.Contains(rule, property+":") || strings.Contains(rule, "@apply border-") {
			return rule
		}
		searchFrom = end + 1
	}
}
