package persondisplay

import "testing"

func TestSoldierServiceLineOmitsDashBeforeCompanyUnit(t *testing.T) {
	got := SoldierServiceLine("Captain", "", "", "Co. B, 1st Texas Infantry")
	if got != "Captain Co. B, 1st Texas Infantry" {
		t.Fatalf("SoldierServiceLine() = %q, want %q", got, "Captain Co. B, 1st Texas Infantry")
	}
}

func TestSoldierServiceLineOmitsDashBeforeNonCompanyUnit(t *testing.T) {
	got := SoldierServiceLine("Captain", "", "", "1st Virginia Infantry")
	if got != "Captain 1st Virginia Infantry" {
		t.Fatalf("SoldierServiceLine() = %q, want %q", got, "Captain 1st Virginia Infantry")
	}
}
