package findagrave

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseHTMLExtractsMemorialFieldsAndWarnings(t *testing.T) {
	sourcePath := filepath.Join("..", "..", "source.txt")
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	result, err := ParseHTML(string(data), "Parsed from pasted HTML", "")
	if err != nil {
		t.Fatalf("ParseHTML: %v", err)
	}

	if result.FirstName != "Elbert" || result.MiddleName != "Dixon" || result.LastName != "Anderson" {
		t.Fatalf("name mapping = %#v", result)
	}
	if result.BirthDate != "00/00/1825" || result.DeathDate != "00/00/1896" {
		t.Fatalf("date mapping = %#v", result)
	}
	if result.BuriedIn != "Antioch Cemetery, Woodruff, Spartanburg County, South Carolina, USA" {
		t.Fatalf("buried_in = %q", result.BuriedIn)
	}
	if result.MemorialID != "11523031" {
		t.Fatalf("memorial id = %q", result.MemorialID)
	}
	if len(result.Spouses) != 2 {
		t.Fatalf("spouses len = %d", len(result.Spouses))
	}
	if result.Spouses[0].MemorialID != "11523035" || !strings.Contains(result.Spouses[1].Name, "Sara Ann") {
		t.Fatalf("spouses = %#v", result.Spouses)
	}
	if len(result.Warnings) == 0 || !strings.Contains(strings.Join(result.Warnings, " "), "Verify all scraped data manually") {
		t.Fatalf("warnings = %#v", result.Warnings)
	}
}

func TestParseInputRejectsURLMode(t *testing.T) {
	_, err := ParseInput(nil, "https://www.findagrave.com/memorial/11523031/elbert_dixon-anderson")
	if err == nil || !strings.Contains(err.Error(), "URL scraping is disabled") {
		t.Fatalf("error = %v", err)
	}
}
