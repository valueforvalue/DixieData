// static_archive_theme_test.go — RED-first regression net for
// issue #494. The static archive (self-contained browser-
// viewable HTML file exported via .ddbak-style share) ships
// with the Soft theme hardcoded as the only palette. There is
// no theme picker, no per-archive localStorage key, no
// aria-pressed attribute, and no high-contrast / classic CSS
// override block. The reader cannot change themes inside the
// archive — it's a snapshot.
//
// Each assertion in this file pins the absence of the legacy
// #475 picker surface AND the presence of the Soft palette at
// :root. A regression that re-adds the picker or strips the
// Soft palette fails the corresponding test.
package archive

import (
	"strings"
	"testing"
)

// TestStaticArchive_BakesSoftTheme pins that the rendered
// index.html starts on the Soft theme via data-theme="soft".
// The first paint is deterministic; the archive is a snapshot
// with no theme switching.
func TestStaticArchive_BakesSoftTheme(t *testing.T) {
	got := renderStaticArchiveHTMLForTest(t)
	if !strings.Contains(got, `data-theme="soft"`) {
		t.Errorf("static archive must bake data-theme=\"soft\" on <html>; got first 400 chars: %s", firstN(got, 400))
	}
}

// TestStaticArchive_NoThemePicker pins the absence of the
// legacy #475 theme-picker UI. A regression that re-adds the
// picker buttons fails this test. The Soft palette is the
// only palette the archive ships; the user cannot switch
// inside the archive.
func TestStaticArchive_NoThemePicker(t *testing.T) {
	got := renderStaticArchiveHTMLForTest(t)
	for _, needle := range []string{
		`data-theme-pick="default"`,
		`data-theme-pick="high-contrast"`,
		`data-theme-pick="soft"`,
		`class="theme-picker"`,
		`class="theme-pick"`,
	} {
		if strings.Contains(got, needle) {
			t.Errorf("static archive must NOT carry %q (theme picker was removed in #494); got first 400 chars: %s", needle, firstN(got, 400))
		}
	}
}

// TestStaticArchive_NoPerArchiveLocalStorageKey pins the
// absence of the per-archive localStorage theme key. Without
// a picker there is no need to persist a reader's pick.
func TestStaticArchive_NoPerArchiveLocalStorageKey(t *testing.T) {
	got := renderStaticArchiveHTMLForTest(t)
	if strings.Contains(got, "dixiedata.static.theme:") {
		t.Errorf("static archive must NOT reference the per-archive localStorage theme key (picker removed in #494); got first 600 chars: %q", firstN(got, 600))
	}
}

// TestStaticArchive_HasSoftPaletteAtRoot pins that the Soft
// palette is the sole CSS palette block, lives at :root, and
// uses the Soft token values from the desktop app's Soft theme.
// The block MUST be at :root (not html[data-theme="soft"]) so
// the implicit default selector hits without needing any
// attribute on <html> — the archive has data-theme="soft" for
// reader clarity, but the cascade is independent.
func TestStaticArchive_HasSoftPaletteAtRoot(t *testing.T) {
	got := renderStaticArchiveHTMLForTest(t)
	// Soft tokens at :root.
	for _, needle := range []string{
		`:root {`,
		`--paper: #f4ecd8;`,
		`--gold: #8d7440;`,
		`--ink: #3b2a1a;`,
	} {
		if !strings.Contains(got, needle) {
			t.Errorf("static archive :root must carry Soft palette token %q; got first 400 chars: %s", needle, firstN(got, 400))
		}
	}
	// No high-contrast override block.
	if strings.Contains(got, `html[data-theme="high-contrast"]`) {
		t.Errorf("static archive must NOT carry an html[data-theme=\"high-contrast\"] block (Soft-only bundle since #494); got first 400 chars: %s", firstN(got, 400))
	}
	// No default/classic override block.
	if strings.Contains(got, `html[data-theme="default"]`) {
		t.Errorf("static archive must NOT carry an html[data-theme=\"default\"] block (Soft-only bundle since #494); got first 400 chars: %s", firstN(got, 400))
	}
	// The legacy :root Default palette tokens must be gone.
	for _, needle := range []string{
		`--paper: #d7d2c9;`,
		`--gold: #a88a46;`,
	} {
		if strings.Contains(got, needle) {
			t.Errorf("static archive must NOT carry legacy Default-palette token %q (Soft-only since #494); got first 400 chars: %s", needle, firstN(got, 400))
		}
	}
}

// renderStaticArchiveHTMLForTest renders the static-archive
// index template with the canonical test fixture data and
// returns the HTML. The test file's data fixtures live as
// package-level constants below so every test gets the same
// deterministic output.
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