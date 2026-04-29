package templates

import (
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

func TestPageRequestURL(t *testing.T) {
	if got := pageRequestURL(models.SoldierSearch{Mode: "basic"}, 2); got != "/soldiers/search?page=2" {
		t.Fatalf("blank query URL = %q", got)
	}
	if got := pageRequestURL(models.SoldierSearch{Mode: "basic", Query: "PENSION 42"}, 3); got != "/soldiers/search?page=3&q=PENSION+42" {
		t.Fatalf("query URL = %q", got)
	}
	if got := pageRequestURL(models.SoldierSearch{Mode: "advanced", DisplayID: "PENSION-42", LastName: "Lee"}, 1); got != "/soldiers/search/advanced?display_id=PENSION-42&last_name=Lee&page=1" {
		t.Fatalf("advanced URL = %q", got)
	}
}

func TestImageURL(t *testing.T) {
	if got := string(imageURL(`images\generated\test.svg`)); got != "/media/images/generated/test.svg" {
		t.Fatalf("relative image URL = %q", got)
	}
	if got := string(imageURL(`C:\data\images\test.svg`)); got != "file:///C:/data/images/test.svg" {
		t.Fatalf("absolute image URL = %q", got)
	}
}
