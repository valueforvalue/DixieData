package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

func TestSourceConflictLedgerViewRendersEntries(t *testing.T) {
	var buf bytes.Buffer
	err := SourceConflictLedgerView(viewmodel.SourceConflictLedger{
		Central: viewmodel.Soldier{
			ID:        17,
			DisplayID: "LED-0017",
			FirstName: "Andrew",
			LastName:  "Cole",
		},
		ResolvedCount: 1,
		Entries: []viewmodel.SourceConflictLedgerEntry{{
			ID:               5,
			ConflictType:     "soldier-update",
			Reason:           "Shared archive changed unit and pension ID.",
			SourceDisplayID:  "SRC-0017",
			Resolution:       "keep-local",
			CreatedAt:        "2026-05-16 18:15:00",
			ResolvedAt:       "2026-05-16 18:16:00",
			LocalSnapshot:    viewmodel.Soldier{DisplayID: "LED-0017", FirstName: "Andrew", LastName: "Cole", Unit: "1st Texas Infantry", PensionID: "P-1"},
			SourceSnapshot:   viewmodel.Soldier{DisplayID: "SRC-0017", FirstName: "Andrew", LastName: "Cole", Unit: "2nd Texas Infantry", PensionID: "P-9"},
			DifferenceFields: []string{"unit", "pension ID"},
		}},
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"Source Conflict Ledger",
		`data-history-back`,
		"SRC-0017",
		"pension ID",
		"Resolved",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("conflict ledger view missing %s", needle)
		}
	}
}
