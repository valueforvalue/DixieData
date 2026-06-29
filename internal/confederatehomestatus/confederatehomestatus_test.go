package confederatehomestatus

import "testing"

// TestNormalizeKnownCanonicalValues pins the contract for the
// 4 canonical values the form lets you submit.
func TestNormalizeKnownCanonicalValues(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Inmate", "Inmate"},
		{"Staffer", "Staffer"},
		{"Trustee", "Trustee"},
		{"N/A", "N/A"},
	}
	for _, c := range cases {
		if got := Normalize(c.in); got != c.want {
			t.Errorf("Normalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestNormalizeLegacyNAVariants pins the contract for the known
// legacy variants that should all collapse to "N/A".
func TestNormalizeLegacyNAVariants(t *testing.T) {
	legacy := []string{"", "none", "na", "n/a", "N/A", "Not recorded", "NONE", "  None  "}
	for _, in := range legacy {
		if got := Normalize(in); got != "N/A" {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, "N/A")
		}
	}
}

// TestNormalizeUnknownValuePreserves covers the regression found
// when reviewing issue #23 (2026-06-29): the default branch used
// to silently rewrite ANY value (including legitimate unknowns
// like "Resident", "Applicant", "Volunteer") to "N/A", destroying
// stored data. The contract should be: unknown values pass through
// (trimmed) so that browse/filter on a stored unknown value still
// matches the stored unknown value.
//
// The only exception: if a value is *truly* empty after trim, it
// should normalize to "N/A" (the canonical empty bucket). That's
// handled by the legacy case above (empty string -> "N/A").
func TestNormalizeUnknownValuePreserves(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Resident", "Resident"},
		{"Applicant", "Applicant"},
		{"Volunteer", "Volunteer"},
		{"Visitor", "Visitor"},
		{"  Padded  ", "Padded"},
	}
	for _, c := range cases {
		if got := Normalize(c.in); got != c.want {
			t.Errorf("Normalize(%q) = %q, want %q (preserved for browse/filter round-trip)", c.in, got, c.want)
		}
	}
}
