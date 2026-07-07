package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

// TestEventSourcesListFragmentRendersReorderControls proves the
// up/down/position controls render for every Event Source on
// the Event detail page (issue #368 slice 3). Mirrors the
// soldier-side test in soldier_card_test.go.
func TestEventSourcesListFragmentRendersReorderControls(t *testing.T) {
	var buf bytes.Buffer
	err := EventSourcesListFragment(42, []viewmodel.SourceRecord{
		{ID: 200, SourceRecordType: "Roster", AppID: "APP-X", Details: "first"},
		{ID: 201, SourceRecordType: "Parole", AppID: "APP-Y", Details: "second"},
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	for _, sourceID := range []string{"200", "201"} {
		expected := `action="/events/42/sources/` + sourceID + `/position"`
		if !strings.Contains(content, expected) {
			t.Fatalf("event source %s reorder form action not found; expected %s", sourceID, expected)
		}
	}
	if !strings.Contains(content, `name="position"`) {
		t.Fatalf("expected position input fields; not found in content")
	}
}
