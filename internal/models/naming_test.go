package models

import "testing"

func TestSoldierGetFullName(t *testing.T) {
	soldier := Soldier{
		Prefix:     "Capt.",
		FirstName:  "John",
		MiddleName: "Bell",
		LastName:   "Hood",
		Suffix:     "Jr.",
	}
	if got := soldier.GetFullName(); got != "Capt. John Bell Hood, Jr." {
		t.Fatalf("GetFullName() = %q", got)
	}
}

func TestUserIdentityBrandingName(t *testing.T) {
	identity := UserIdentity{FirstName: "Samuel", LastName: "Carter"}
	if got := identity.BrandingName(); got != "S. Carter" {
		t.Fatalf("BrandingName() = %q", got)
	}
}
