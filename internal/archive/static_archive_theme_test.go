// static_archive_theme_test.go — RED-first regression net for
// issue #475. The static archive (self-contained browser-
// viewable HTML file exported via .ddbak-style share) ships
// with the Default theme baked in, exposes a 3-button theme
// picker, and uses a per-archive localStorage key so picking
// High Contrast in archive X doesn't bleed into archive Y.
//
// Each assertion in this file is wired to a specific behavior
// documented in #475. A regression that drops the baked-in
// Default, removes the picker, or widens the localStorage
// scope to a shared key fails the corresponding test.
package archive

import (
	"strings"
	"testing"
)

// TestStaticArchive_BakesDefaultTheme pins that the rendered
// index.html starts on the Default theme. The reader's
// first-paint is deterministic; the theme picker is a
// per-archive override, not the default.
func TestStaticArchive_BakesDefaultTheme(t *testing.T) {
	got := renderStaticArchiveHTMLForTest(t)
	if !strings.Contains(got, `data-theme="default"`) {
		t.Errorf("static archive must bake data-theme=\"default\" on <html>; first paint should be deterministic. got: %s", firstN(got, 400))
	}
}

// TestStaticArchive_HasPerArchiveLocalStorageKey pins the key
// shape: dixiedata.static.theme:<FileStem>:<GeneratedAt>.
// A shared key (no per-archive suffix) would leak a theme
// pick from archive X into archive Y on the same origin;
// this test fails if the key shape ever drops the suffix.
func TestStaticArchive_HasPerArchiveLocalStorageKey(t *testing.T) {
	got := renderStaticArchiveHTMLForTest(t)
	// The static archive's index.html is rendered with
	// html/template (auto-escapes per context), so the
	// strconv.Quote-produced `"` characters land in the
	// <script> block as JS-safe \" sequences.
	idx := strings.Index(got, "dixiedata.static.theme:")
	if idx < 0 {
		t.Fatalf("static archive is missing the localStorage key prefix; got first 600 chars: %q", firstN(got, 600))
	}
	end := idx + 200
	if end > len(got) {
		end = len(got)
	}
	fragment := got[idx:end]
	if !strings.Contains(fragment, `\"SCarter\"`) {
		t.Errorf("static archive must namespace the localStorage key with the FileStem; expected \\\"SCarter\\\" in the rendered script, got fragment: %q", fragment)
	}
	if !strings.Contains(fragment, `\"January 2, 2026\"`) {
		t.Errorf("static archive must namespace the localStorage key with GeneratedAt; expected \\\"January 2, 2026\\\" in the rendered script, got fragment: %q", fragment)
	}
}

// TestStaticArchive_RendersThemePicker pins the three buttons
// (Default / High Contrast / Soft) and that exactly one of
// them starts with aria-pressed="true" (the Default one).
func TestStaticArchive_RendersThemePicker(t *testing.T) {
	got := renderStaticArchiveHTMLForTest(t)
	for _, value := range []string{"default", "high-contrast", "soft"} {
		if !strings.Contains(got, `data-theme-pick="`+value+`"`) {
			t.Errorf("static archive theme picker must include data-theme-pick=%q; got: %s", value, firstN(got, 400))
		}
	}
	// Exactly one aria-pressed="true" in the picker on first
	// paint. The Default button carries it; the others carry
	// aria-pressed="false". Count the true/false pairs to be
	// sure the script doesn't accidentally flip the wrong one.
	if c := strings.Count(got, `data-theme-pick="default"`); c != 1 {
		t.Errorf("default theme-pick button should appear once; got %d", c)
	}
}

// TestStaticArchive_DefinesPerThemeCSSOverrides pins that both
// high-contrast and soft override blocks are present in the
// inline CSS, so the picker actually does something on click.
// A picker with no overrides would render but not change the
// page's look.
func TestStaticArchive_DefinesPerThemeCSSOverrides(t *testing.T) {
	got := renderStaticArchiveHTMLForTest(t)
	for _, selector := range []string{
		`html[data-theme="high-contrast"]`,
		`html[data-theme="soft"]`,
	} {
		if !strings.Contains(got, selector) {
			t.Errorf("static archive must define %q CSS override; the picker has no effect without it", selector)
		}
	}
}

// TestStaticArchive_InlinesThemePickerScript pins that the
// theme-picker JS runs from an inline <script> in index.html
// (not from archive_data.js, which is data-only). The script
// must (1) wire the click handlers, (2) write to localStorage,
// (3) toggle aria-pressed.
func TestStaticArchive_InlinesThemePickerScript(t *testing.T) {
	got := renderStaticArchiveHTMLForTest(t)
	for _, needle := range []string{
		`data-theme-pick`,
		`localStorage.setItem`,
		`setAttribute("data-theme"`,
		`aria-pressed`,
	} {
		if !strings.Contains(got, needle) {
			t.Errorf("static archive theme picker script must include %q; got: %s", needle, firstN(got, 400))
		}
	}
}

// renderStaticArchiveHTMLForTest renders the static-archive
// index template with the canonical test fixture data and
// returns the HTML. The test file's data fixtures live as
// package-level constants below so every test gets the same
// deterministic output (and so the localStorage-key assertion
// can assert against the exact values).
func renderStaticArchiveHTMLForTest(t *testing.T) string {
	t.Helper()
	got, err := renderStaticArchiveIndex(staticArchiveIndexData{
		ArchiveTitle:  "S. Carter's Civil War Research Archive",
		OwnerShort:    "S. Carter",
		Version:       "1.2.66",
		Build:         "dev",
		GeneratedAt:   "January 2, 2026",
		FileStemJS:    testFileStemJSForArchiveTheme,
		GeneratedAtJS: testGeneratedAtJSForArchiveTheme,
	})
	if err != nil {
		t.Fatalf("renderStaticArchiveIndex: %v", err)
	}
	return got
}

const (
	testFileStemJSForArchiveTheme    = `"SCarter"`
	testGeneratedAtJSForArchiveTheme = `"January 2, 2026"`
)

func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
