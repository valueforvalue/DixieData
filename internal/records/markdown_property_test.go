// markdown_property_test.go — rapid property tests for the
// MarkdownRenderer (issue #635). Catches sanitisation
// regressions that the hand-written table cases in
// markdown_test.go miss: idempotence of the sanitize→render
// chain, no-panic for any input, and XSS-vector absence.
//
// Generated runs: rapid's default 100 iters per Check.
//
// Refs issue #635.

package records

import (
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// TestPropertyMarkdownSanitizeIdempotent: for any string,
// Sanitize(Sanitize(Render(x))) == Sanitize(Render(x)).
// The bluemonday sanitizer applied to its own output must be
// stable — a second sanitize pass on already-sanitized HTML
// must not change the output. Catches the class where a
// sanitizer strips something on the second pass that it left
// on the first (e.g. a policy that allows <p> in isolation
// but strips nested <p> inside another tag).
func TestPropertyMarkdownSanitizeIdempotent(t *testing.T) {
	r := NewMarkdownRenderer()
	rapid.Check(t, func(t *rapid.T) {
		s := rapid.String().Draw(t, "source")
		first, err := r.Render(s)
		if err != nil {
			t.Fatalf("Render(%q) first pass: %v", s, err)
		}
		// Apply sanitize directly (not re-render — goldmark
		// treats already-rendered HTML as markdown input and
		// the result is not idempotent through the full
		// render chain). The invariant is about the sanitizer.
		second := r.sanitizer.Sanitize(first)
		if first != second {
			t.Fatalf("sanitize not idempotent:\n  first:  %q\n  second: %q", first, second)
		}
	})
}

// TestPropertyMarkdownRenderNeverPanics: Render must never
// panic on any string input. Catches the class where a
// future refactor delegates to a goldmark extension or
// bluemonday policy that panics on control chars, null
// bytes, or multi-byte boundary violations.
func TestPropertyMarkdownRenderNeverPanics(t *testing.T) {
	r := NewMarkdownRenderer()
	rapid.Check(t, func(t *rapid.T) {
		s := rapid.String().Draw(t, "source")
		defer func() {
			if rec := recover(); rec != nil {
				t.Fatalf("Render(%q) panicked: %v", s, rec)
			}
		}()
		_, _ = r.Render(s)
	})
}

// TestPropertyMarkdownNoXSS: Render output must never contain
// known XSS vectors (<script>, <iframe>, <style>, onerror=).
// Uses rapid.String() to generate adversarial input that may
// include raw HTML fragments. The custom bluemonday policy
// must strip them.
func TestPropertyMarkdownNoXSS(t *testing.T) {
	r := NewMarkdownRenderer()
	rapid.Check(t, func(t *rapid.T) {
		s := rapid.String().Draw(t, "source")
		out, err := r.Render(s)
		if err != nil {
			t.Fatalf("Render(%q): %v", s, err)
		}
		lower := strings.ToLower(out)
		for _, banned := range []string{"<script", "<iframe", "<style", "onerror="} {
			if strings.Contains(lower, banned) {
				t.Fatalf("Render output contains banned %q\nsource: %q\noutput: %q", banned, s, out)
			}
		}
	})
}

// TestPropertyMarkdownEmptySourceIdempotent: empty or
// whitespace-only source produces output that is stable
// under re-sanitization. Separated from the idempotent
// test so shrinking converges on the simplest failing input.
func TestPropertyMarkdownEmptySourceIdempotent(t *testing.T) {
	r := NewMarkdownRenderer()
	rapid.Check(t, func(t *rapid.T) {
		s := rapid.StringMatching(`\s*`).Draw(t, "whitespace")
		out, err := r.Render(s)
		if err != nil {
			t.Fatalf("Render whitespace: %v", err)
		}
		second := r.sanitizer.Sanitize(out)
		if out != second {
			t.Fatalf("sanitize not idempotent on whitespace output: %q → %q → %q", s, out, second)
		}
	})
}
