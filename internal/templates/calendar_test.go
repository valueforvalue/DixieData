package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

func TestCalendarShowsSplitArchiveCounts(t *testing.T) {
	var buf bytes.Buffer
	err := Calendar(5, map[int][]models.Soldier{}, models.ArchiveCounts{
		TotalSoldiers:    12,
		TotalWivesWidows: 5,
	}, models.Quote{
		Author: "Test Author",
		Text:   "Test quote",
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		">12<",
		">5<",
		"Soldiers",
		"Wives & Widows",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("calendar header missing %s", needle)
		}
	}
}
