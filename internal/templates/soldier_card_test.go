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

func TestLinkifiedSegments(t *testing.T) {
	segments := linkifiedSegments("See https://example.com/test, then http://example.org.")
	if len(segments) != 6 {
		t.Fatalf("segment count = %d, want 6", len(segments))
	}
	if segments[1].Href != "https://example.com/test" || segments[1].Text != "https://example.com/test" {
		t.Fatalf("first link = %#v", segments[1])
	}
	if segments[2].Text != "," {
		t.Fatalf("separator = %#v", segments[2])
	}
	if segments[3].Text != " then " {
		t.Fatalf("middle text = %#v", segments[3])
	}
	if segments[4].Href != "http://example.org" {
		t.Fatalf("second link = %#v", segments[4])
	}
	if segments[5].Text != "." {
		t.Fatalf("trailing punctuation = %#v", segments[5])
	}
}

func TestLinkifiedLinesPreservesBlankLines(t *testing.T) {
	lines := linkifiedLines("first line\n\nhttps://example.com")
	if len(lines) != 3 {
		t.Fatalf("line count = %d, want 3", len(lines))
	}
	if len(lines[1]) != 1 || lines[1][0].Text != "" {
		t.Fatalf("blank line = %#v", lines[1])
	}
	if len(lines[2]) != 1 || lines[2][0].Href != "https://example.com" {
		t.Fatalf("link line = %#v", lines[2])
	}
}
