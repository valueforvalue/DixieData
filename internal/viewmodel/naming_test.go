package viewmodel

import "testing"

func TestPersonRecordGetFullNameHidesPrefixByDefault(t *testing.T) {
	record := PersonRecord{
		Prefix:    "Capt.",
		FirstName: "John",
		LastName:  "Taylor",
	}

	if got := record.GetFullName(); got != "John Taylor" {
		t.Fatalf("GetFullName() = %q", got)
	}
}

func TestPersonRecordGetFullNameShowsPrefixWhenEnabled(t *testing.T) {
	record := PersonRecord{
		Prefix:               "Capt.",
		ShowPrefixBeforeName: true,
		FirstName:            "John",
		LastName:             "Taylor",
		Suffix:               "Jr.",
	}

	if got := record.GetFullName(); got != "Capt. John Taylor, Jr." {
		t.Fatalf("GetFullName() = %q", got)
	}
}
