package templates

// Issue #423 Slice 3: surface the row provenance footer on
// the Soldier detail page. The footer renders:
//   - "Created by DixieData v1.2.N via <path>" when at least
//     one of CreatedByVersion or CreatedByImportPath is set
//     (issue #377 slice 3 UI surface).
//   - "Restored at <human-readable timestamp>" when RestoredAt
//     is set (issue #423 slice 3).
//
// Both lines are hidden when their source fields are empty.
// The whole footer is hidden when no provenance field is
// populated so a row that pre-dates v64 (no CreatedBy* stamp,
// no RestoredAt stamp) does not surface a "Created by
// unknown" / "Restored at unknown" line — absence IS the
// signal.

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

// TestSoldierDetailFooterHiddenWhenNoProvenance pins the
// "absence IS the signal" semantics: a row with no
// provenance fields renders NO footer at all. A user
// landing on a pre-v64 row sees a clean detail page, not a
// "Created by unknown via unknown" line.
func TestSoldierDetailFooterHiddenWhenNoProvenance(t *testing.T) {
	var buf bytes.Buffer
	err := SoldierDetail(viewmodel.PersonRecord{
		ID:        411,
		DisplayID: "DXD-00411",
		FirstName: "James",
		LastName:  "Gillespie",
	}, nil, nil).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	if strings.Contains(content, `id="panel.soldier.detail.provenance"`) {
		t.Errorf("detail page rendered the provenance footer for a row with no provenance fields; want hidden")
	}
	if strings.Contains(content, "Created by DixieData") {
		t.Errorf("detail page rendered 'Created by DixieData' for a row with no provenance fields; want hidden")
	}
	if strings.Contains(content, "Restored at") {
		t.Errorf("detail page rendered 'Restored at' for a row with no provenance fields; want hidden")
	}
}

// TestSoldierDetailFooterRendersCreatedLine pins the v64
// "Created by DixieData v1.2.N via <path>" line shape.
// Covers the four cases of (version, path) presence:
// both, only version, only path, neither (the last case is
// the "hidden" test above).
func TestSoldierDetailFooterRendersCreatedLine(t *testing.T) {
	cases := []struct {
		name    string
		version string
		path    string
		want    string
	}{
		{"both", "v1.2.64", "create_soldier", "Created by DixieData v1.2.64 via create_soldier"},
		{"only version", "v1.2.64", "", "Created by DixieData v1.2.64"},
		{"only path", "", "memorial_json_import", "Created by DixieData via memorial_json_import"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := SoldierDetail(viewmodel.PersonRecord{
				ID:                411,
				DisplayID:         "DXD-00411",
				FirstName:         "James",
				LastName:          "Gillespie",
				CreatedByVersion:  tc.version,
				CreatedByImportPath: tc.path,
			}, nil, nil).Render(context.Background(), &buf)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			content := buf.String()
			if !strings.Contains(content, `id="panel.soldier.detail.provenance"`) {
				t.Errorf("provenance footer not rendered; want one")
			}
			if !strings.Contains(content, tc.want) {
				t.Errorf("detail page does not contain %q; full content:\n%s", tc.want, content)
			}
		})
	}
}

// TestSoldierDetailFooterRendersRestoredLine pins the v65
// "Restored at <human-readable timestamp>" line shape. Uses
// a fixed RFC3339 timestamp so the test is deterministic
// across timezones (the helper converts to local time for
// display).
func TestSoldierDetailFooterRendersRestoredLine(t *testing.T) {
	var buf bytes.Buffer
	err := SoldierDetail(viewmodel.PersonRecord{
		ID:        411,
		DisplayID: "DXD-00411",
		FirstName: "James",
		LastName:  "Gillespie",
		RestoredAt: "2026-07-08T12:00:00Z",
	}, nil, nil).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	if !strings.Contains(content, `id="panel.soldier.detail.provenance"`) {
		t.Errorf("provenance footer not rendered; want one")
	}
	if !strings.Contains(content, "Restored at ") {
		t.Errorf("detail page does not contain 'Restored at'; want one")
	}
	// The helper renders the timestamp via Local().Format with
	// "Jan 2, 2006 at 3:04 PM MST", so we assert on the
	// year + month + day rather than the exact string (the
	// timezone offset is machine-dependent).
	if !strings.Contains(content, "2026") || !strings.Contains(content, "Jul") {
		t.Errorf("detail page does not contain a parseable local-time 2026/Jul string; full content:\n%s", content)
	}
}

// TestSoldierDetailFooterRendersBothLines proves the two
// lines coexist: a row that was created via the form AND
// later carried over a restore point shows both lines.
func TestSoldierDetailFooterRendersBothLines(t *testing.T) {
	var buf bytes.Buffer
	err := SoldierDetail(viewmodel.PersonRecord{
		ID:                  411,
		DisplayID:           "DXD-00411",
		FirstName:           "James",
		LastName:            "Gillespie",
		CreatedByVersion:    "v1.2.64",
		CreatedByImportPath: "create_soldier",
		RestoredAt:          "2026-07-08T12:00:00Z",
	}, nil, nil).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	if !strings.Contains(content, "Created by DixieData v1.2.64 via create_soldier") {
		t.Errorf("missing v64 created line; full content:\n%s", content)
	}
	if !strings.Contains(content, "Restored at ") {
		t.Errorf("missing v65 restored line; full content:\n%s", content)
	}
}
