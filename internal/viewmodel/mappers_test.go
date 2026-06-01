package viewmodel

import (
	"testing"

	"github.com/valueforvalue/DixieData/internal/records"
)

func TestBrowseStateFromDomainLeavesBlankPensionStateEmpty(t *testing.T) {
	state := BrowseStateFromDomain(records.BrowseRequest{
		Page:         1,
		PageSize:     100,
		Scope:        "all",
		Sort:         "display_id_asc",
		PensionState: "",
	}, 320)

	if state.PensionState != "" {
		t.Fatalf("blank browse pension state mapped to %q", state.PensionState)
	}
}
