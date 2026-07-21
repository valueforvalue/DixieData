package dates

import (
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// Property tests for internal/dates. Catches the
// docs/COMMON_BUGS.md §9.2 (normalization) class via randomized
// inputs that hand-written table cases miss. Runs alongside the
// existing hand-written table tests in dates_test.go — they
// complement, not replace, the existing coverage.
//
// Generated runs: rapid's default is 100 iters per Check; raise
// via rapid.Check(t, ...) args for CI.
//
// Refs issue #318 Slice 3.

// validDate generates a PartialDate in the canonical "complete"
// range: year in [1800..1899] (Civil War era), month 1..12,
// day 1..28 (avoids month-length edge cases — those have hand
// tests in dates_test.go).
func validDate() *rapid.Generator[PartialDate] {
	return rapid.Custom(func(t *rapid.T) PartialDate {
		return PartialDate{
			Year:  rapid.IntRange(1800, 1899).Draw(t, "year"),
			Month: rapid.IntRange(1, 12).Draw(t, "month"),
			Day:   rapid.IntRange(1, 28).Draw(t, "day"),
		}
	})
}

// partialYearOnly generates a year-only PartialDate (month=day=0).
func partialYearOnly() *rapid.Generator[PartialDate] {
	return rapid.Custom(func(t *rapid.T) PartialDate {
		return PartialDate{Year: rapid.IntRange(1000, 9999).Draw(t, "year")}
	})
}

// TestPropertyParseRoundtrip: for any valid PartialDate,
// Format → Parse → Format returns the same string. Catches
// asymmetric normalisation bugs (e.g. a parser that strips
// leading zeros on one path but not the other).
func TestPropertyParseRoundtrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		p := validDate().Draw(t, "date")
		s := p.Format()
		parsed, err := ParseCanonical(s)
		if err != nil {
			t.Fatalf("ParseCanonical(%q) errored on Format output: %v", s, err)
		}
		if got := parsed.Format(); got != s {
			t.Fatalf("roundtrip mismatch: %q → %q → %q", s, parsed.Format(), got)
		}
		if parsed != p {
			t.Fatalf("roundtrip value mismatch: %+v → %+v", p, parsed)
		}
	})
}

// TestPropertyParseRoundtripYearOnly: same invariant for
// year-only PartialDates (month=day=0). Separate from the
// full-date case because ParseCanonical has special handling
// for the "MM/DD/YYYY" split.
func TestPropertyParseRoundtripYearOnly(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		p := partialYearOnly().Draw(t, "year-only")
		s := p.Format() // "00/00/YYYY"
		parsed, err := ParseCanonical(s)
		if err != nil {
			t.Fatalf("ParseCanonical(%q) errored: %v", s, err)
		}
		if parsed.Year != p.Year {
			t.Fatalf("year-only roundtrip lost year: %+v → %+v", p, parsed)
		}
	})
}

// TestPropertyParseEmptyNeverPanics: for any string (including
// empty, whitespace, multi-byte, control chars), ParseCanonical
// returns without panic. Catches the "out of range" / nil-deref
// class where a future refactor panics on a degenerate input.
func TestPropertyParseEmptyNeverPanics(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := rapid.String().Draw(t, "s")
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("ParseCanonical(%q) panicked: %v", s, r)
			}
		}()
		_, _ = ParseCanonical(s) // result ignored — only panic matters
	})
}

// TestPropertyNormalizeIdempotent: Normalize(Normalize(x)) ==
// Normalize(x) when x is in the parseable subset. Catches
// normalization functions that change representation on each
// pass (e.g. trimming trailing slash, then adding it back).
func TestPropertyNormalizeIdempotent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		p := validDate().Draw(t, "date")
		s := p.Format()
		first, err := NormalizeCanonical(s)
		if err != nil {
			t.Fatalf("NormalizeCanonical(%q): %v", s, err)
		}
		second, err := NormalizeCanonical(first)
		if err != nil {
			t.Fatalf("NormalizeCanonical(%q) (second pass): %v", first, err)
		}
		if first != second {
			t.Fatalf("NormalizeCanonical not idempotent: %q → %q → %q", s, first, second)
		}
	})
}

// TestPropertyDisplayNeverPanics: Display is documented to
// accept "any string" and render a UI-safe label. Catches
// regressions where Display delegates to something that
// panics on degenerate input.
func TestPropertyDisplayNeverPanics(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := rapid.String().Draw(t, "s")
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Display(%q) panicked: %v", s, r)
			}
		}()
		_ = Display(s)
	})
}

// TestPropertyParseBirthInfoExtractsYear: for any string of
// shape "<word(s)> YYYY <word(s)>" (Civil-War birth-line
// prose), ParseBirthInfo extracts the year without panic.
// The hand-written TestParseBirthInfo covers 4 specific shapes;
// this catches the same shape under randomized word boundaries.
func TestPropertyParseBirthInfoExtractsYear(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate: <word> YYYY <word>
		year := rapid.IntRange(1800, 1899).Draw(t, "year")
		prefix := rapid.StringMatching(`[a-zA-Z., ]{1,30}`).Draw(t, "prefix")
		suffix := rapid.StringMatching(`[a-zA-Z., ]{0,30}`).Draw(t, "suffix")
		input := strings.TrimSpace(prefix) + " " +
			itoa(year) + " " +
			strings.TrimSpace(suffix)
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("ParseBirthInfo(%q) panicked: %v", input, r)
			}
		}()
		// We don't assert exact match (the regex has 4 variants
		// with different captures); only that the call returns
		// without panic and contains the year.
		got := ParseBirthInfo(input)
		if got == "" {
			// The 4th regex (\bYYYY\b) is year-only, so any
			// string containing a year should yield at least
			// "00/00/YYYY" — if not, the regex path is broken.
			t.Fatalf("ParseBirthInfo(%q) returned empty; expected year extraction", input)
		}
		if !strings.Contains(got, itoa(year)) {
			t.Fatalf("ParseBirthInfo(%q) = %q, missing year %d", input, got, year)
		}
	})
}

// TestPropertyDisplayStability: for any valid PartialDate,
// Display(Format(d)) is stable — calling Display twice on
// the same formatted output returns the same result.
// Catches the class where Display mutates internal state
// or where ParseCanonical → Display has a divergent
// normalization path.
func TestPropertyDisplayStability(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		p := validDate().Draw(t, "date")
		s := p.Format()
		first := Display(s)
		second := Display(s)
		if first != second {
			t.Fatalf("Display not stable: %q → %q → %q", s, first, second)
		}
	})
}

// TestPropertyCrossFormatConsistency: ParseCanonical accepts
// the output of Format for any valid PartialDate. Catches
// the class where Format produces a string that ParseCanonical
// cannot re-parse (e.g. a missing leading zero that a future
// refactor strips).
func TestPropertyCrossFormatConsistency(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		p := validDate().Draw(t, "date")
		s := p.Format()
		parsed, err := ParseCanonical(s)
		if err != nil {
			t.Fatalf("ParseCanonical rejected Format output %q: %v", s, err)
		}
		if parsed != p {
			t.Fatalf("Parse(Format(d)) != d: %+v → %q → %+v", p, s, parsed)
		}
	})
}

// TestPropertyDisplayNeverPanicsOnValid: Display must never
// panic on a valid formatted date string. (The existing
// TestPropertyDisplayNeverPanics covers random strings;
// this one covers valid dates specifically, so shrinking
// converges on the simplest valid-date that triggers.)
func TestPropertyDisplayNeverPanicsOnValid(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		p := validDate().Draw(t, "date")
		s := p.Format()
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Display(%q) panicked: %v", s, r)
			}
		}()
		_ = Display(s)
	})
}

// itoa is a small helper to avoid pulling strconv into the
// generator scope. rapid's IntRange already returns int; we
// only need it for the year assertion.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}
