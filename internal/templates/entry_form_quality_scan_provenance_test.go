package templates

// Issue #423 Slice 3 (UI surface): the data-quality scan
// results on the settings page show the row's provenance
// fields next to each flagged row's name. Pin:
//   - import path is rendered when populated
//   - restore timestamp is rendered when populated
//   - both lines coexist when both are populated
//   - the line is hidden when both fields are empty
//     (absence IS the signal, not a "via unknown" line)

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

func TestSettingsQualityScanResultsShowsProvenanceLine(t *testing.T) {
	cases := []struct {
		name       string
		issue      viewmodel.DataQualityIssue
		want       string
		mustNotHave string
	}{
		{
			name: "import path only",
			issue: viewmodel.DataQualityIssue{
				PersonRecordID: 411,
				DisplayID:      "DXD-00411",
				Name:           "James Gillespie",
				ImportPath:     "memorial_json_import",
			},
			want: "via memorial_json_import",
		},
		{
			name: "restored at only",
			issue: viewmodel.DataQualityIssue{
				PersonRecordID: 411,
				DisplayID:      "DXD-00411",
				Name:           "James Gillespie",
				RestoredAt:     "2026-07-08T12:00:00Z",
			},
			want: "restored at ",
		},
		{
			name: "both",
			issue: viewmodel.DataQualityIssue{
				PersonRecordID: 411,
				DisplayID:      "DXD-00411",
				Name:           "James Gillespie",
				ImportPath:     "create_soldier",
				RestoredAt:     "2026-07-08T12:00:00Z",
			},
			want: "via create_soldier",
		},
		{
			name: "neither",
			issue: viewmodel.DataQualityIssue{
				PersonRecordID: 411,
				DisplayID:      "DXD-00411",
				Name:           "James Gillespie",
			},
			mustNotHave: "via ",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := SettingsQualityScanResults(viewmodel.DataQualityScanResult{
				Mode:           "high-confidence",
				ScannedRecords: 1,
				IssueCount:     1,
				Groups: []viewmodel.DataQualityIssueGroup{{
					Group:  "Identity & Naming",
					Count:  1,
					Issues: []viewmodel.DataQualityIssue{tc.issue},
				}},
			}).Render(context.Background(), &buf)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			content := buf.String()
			if tc.want != "" && !strings.Contains(content, tc.want) {
				t.Errorf("expected %q in content; full content:\n%s", tc.want, content)
			}
			if tc.mustNotHave != "" && strings.Contains(content, tc.mustNotHave) {
				t.Errorf("did not expect %q in content; full content:\n%s", tc.mustNotHave, content)
			}
		})
	}
}
