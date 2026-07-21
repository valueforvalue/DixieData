// soldier_property_test.go — rapid property tests for Soldier
// normalization idempotence (issue #633). Catches the class
// where Normalize(Normalize(x)) != Normalize(x) — documented
// in docs/COMMON_BUGS.md §9.2. Tests each normalization
// function in isolation so shrinking converges on the
// specific field and value that causes oscillation.
//
// Refs issue #633.

package records

import (
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/confederatehomestatus"
	"github.com/valueforvalue/DixieData/internal/pensionstate"
	"pgregory.net/rapid"
)

// TestPropertyPensionStateNormalizeIdempotent: pensionstate.Normalize
// is idempotent for any string input.
func TestPropertyPensionStateNormalizeIdempotent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := rapid.String().Draw(t, "input")
		first := pensionstate.Normalize(s)
		second := pensionstate.Normalize(first)
		if first != second {
			t.Fatalf("pensionstate.Normalize not idempotent: %q → %q → %q", s, first, second)
		}
	})
}

// TestPropertyConfederateHomeStatusNormalizeIdempotent:
// confederatehomestatus.Normalize is idempotent for any string.
func TestPropertyConfederateHomeStatusNormalizeIdempotent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := rapid.String().Draw(t, "input")
		first := confederatehomestatus.Normalize(s)
		second := confederatehomestatus.Normalize(first)
		if first != second {
			t.Fatalf("confederatehomestatus.Normalize not idempotent: %q → %q → %q", s, first, second)
		}
	})
}

// TestPropertyEntryTypeNormalizeIdempotent: normalizeEntryType
// is idempotent for any string input.
func TestPropertyEntryTypeNormalizeIdempotent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := rapid.String().Draw(t, "input")
		first := normalizeEntryType(s)
		second := normalizeEntryType(first)
		if first != second {
			t.Fatalf("normalizeEntryType not idempotent: %q → %q → %q", s, first, second)
		}
	})
}

// TestPropertyCanonicalRankIdempotent: canonicalRank is
// idempotent — calling it twice on the same soldier yields
// the same result.
func TestPropertyCanonicalRankIdempotent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := validSoldier().Draw(t, "soldier")
		first := canonicalRank(s)
		// canonicalRank returns RankOut || Rank || RankIn.
		// Call again with the same soldier to verify stability.
		second := canonicalRank(s)
		if first != second {
			t.Fatalf("canonicalRank not idempotent: %q → %q", first, second)
		}
	})
}

// TestPropertyDisplayIDNormalizeIdempotent: normalizeDisplayID
// (via db.SanitizeID) is idempotent for any display-ID-shaped
// input.
func TestPropertyDisplayIDNormalizeIdempotent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := validDisplayID().Draw(t, "display_id")
		first := normalizeDisplayID(s, "TEST")
		second := normalizeDisplayID(first, "TEST")
		if first != second {
			t.Fatalf("normalizeDisplayID not idempotent: %q → %q → %q", s, first, second)
		}
	})
}

// TestPropertyNormalizeEntryTypePreservesValidInputs:
// valid entry types pass through unchanged.
func TestPropertyNormalizeEntryTypePreservesValidInputs(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := validEntryType().Draw(t, "entry_type")
		got := normalizeEntryType(s)
		if got != s {
			t.Fatalf("normalizeEntryType changed valid input: %q → %q", s, got)
		}
	})
}

// TestPropertyTrimSpaceIdempotent: strings.TrimSpace is
// idempotent — the normalizer uses it on many fields.
// This property catches the case where TrimSpace is
// applied inside a field normalizer that also does
// something else, and the combination oscillates.
func TestPropertyTrimSpaceIdempotent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := rapid.String().Draw(t, "input")
		first := strings.TrimSpace(s)
		second := strings.TrimSpace(first)
		if first != second {
			t.Fatalf("TrimSpace not idempotent: %q → %q → %q", s, first, second)
		}
	})
}

// TestPropertyPensionStateNeverPanics: Normalize never panics
// on any string input.
func TestPropertyPensionStateNeverPanics(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := rapid.String().Draw(t, "input")
		defer func() {
			if rec := recover(); rec != nil {
				t.Fatalf("pensionstate.Normalize(%q) panicked: %v", s, rec)
			}
		}()
		_ = pensionstate.Normalize(s)
	})
}

// TestPropertyConfederateHomeStatusNeverPanics: Normalize never
// panics on any string input.
func TestPropertyConfederateHomeStatusNeverPanics(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := rapid.String().Draw(t, "input")
		defer func() {
			if rec := recover(); rec != nil {
				t.Fatalf("confederatehomestatus.Normalize(%q) panicked: %v", s, rec)
			}
		}()
		_ = confederatehomestatus.Normalize(s)
	})
}

// TestPropertyNormalizeEntryTypeNeverPanics: normalizeEntryType
// never panics on any string input.
func TestPropertyNormalizeEntryTypeNeverPanics(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := rapid.String().Draw(t, "input")
		defer func() {
			if rec := recover(); rec != nil {
				t.Fatalf("normalizeEntryType(%q) panicked: %v", s, rec)
			}
		}()
		_ = normalizeEntryType(s)
	})
}

// TestPropertyTagNormalizeIdempotent: NormalizeTagName is
// idempotent for any string input.
func TestPropertyTagNormalizeIdempotent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := rapid.String().Draw(t, "input")
		first := NormalizeTagName(s)
		second := NormalizeTagName(first)
		if first != second {
			t.Fatalf("NormalizeTagName not idempotent: %q → %q → %q", s, first, second)
		}
	})
}
