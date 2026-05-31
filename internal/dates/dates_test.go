package dates

import "testing"

func TestNormalizeCanonicalAllowsPartialDates(t *testing.T) {
	got, err := NormalizeCanonical("11/0/1886")
	if err != nil {
		t.Fatalf("NormalizeCanonical: %v", err)
	}
	if got != "11/00/1886" {
		t.Fatalf("got %q", got)
	}
}

func TestParseBirthInfo(t *testing.T) {
	cases := map[string]string{
		"b. Jan. 13, 1842, Blount Co., AL, U.S.A.": "01/13/1842",
		"2 June, 1830, Aberdeenshire, Scotland":    "06/02/1830",
		"b. Missouri, 1839":                        "00/00/1839",
		"b. 1815, Tennessee":                       "00/00/1815",
	}
	for input, want := range cases {
		if got := ParseBirthInfo(input); got != want {
			t.Fatalf("ParseBirthInfo(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestDisplayUnknownUsesUnknownForMissingDates(t *testing.T) {
	if got := DisplayUnknown(""); got != "Unknown" {
		t.Fatalf("DisplayUnknown(empty) = %q, want Unknown", got)
	}
	if got := DisplayUnknown("05/12/1838"); got != "May 12, 1838" {
		t.Fatalf("DisplayUnknown(date) = %q", got)
	}
}
