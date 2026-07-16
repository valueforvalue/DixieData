package components

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestTableBuilderModal_RendersForm — issue #610 slice 5.
// The table builder modal lives at id=\"table-builder-modal\"
// so the editor toolbar's \"Table\" button can find it via
// document.getElementById(\"table-builder-modal\") + call
// showOverlayModal. The modal carries:
//   - role=\"dialog\" + aria-modal=\"true\" (overlay vocabulary)
//   - rows + cols numeric inputs (default 2x2)
//   - an \"Insert\" button + a \"Close\" button
//   - a preview area (initially hidden; JS fills it as the
//     user changes the rows/cols inputs)
func TestTableBuilderModal_RendersForm(t *testing.T) {
	var buf bytes.Buffer
	if err := TableBuilderModal().Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()

	wantMarkers := []string{
		`id="table-builder-modal"`,
		`role="dialog"`,
		`aria-modal="true"`,
		`data-table-builder-rows`,
		`data-table-builder-cols`,
		`data-table-builder-insert`,
		`data-table-builder-close`,
	}
	for _, m := range wantMarkers {
		if !strings.Contains(got, m) {
			t.Errorf("modal missing marker %q\nfull render:\n%s", m, got)
		}
	}
}

// TestTableBuilderModal_DefaultsTo2x2 — issue #610 slice 5.
// The modal opens with 2 rows + 2 cols pre-filled. The JS
// preview generation kicks in immediately, so the user sees
// a 2x2 Markdown table skeleton on first open (matches the
// Slice-5 locked decision: \"default 2x2, then user can edit\").
func TestTableBuilderModal_DefaultsTo2x2(t *testing.T) {
	var buf bytes.Buffer
	if err := TableBuilderModal().Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()

	for _, marker := range []string{`data-table-builder-rows`, `data-table-builder-cols`} {
		idx := strings.Index(got, marker)
		if idx < 0 {
			t.Fatalf("marker %q missing", marker)
		}
		// Walk backward to the enclosing <input opener; the
		// value="2" attr sits before the data-* attr (templ
		// renders attrs in source order: type, id, name,
		// min, max, value, class, data-*).
		inputOpen := strings.LastIndex(got[:idx], "<input")
		if inputOpen < 0 {
			t.Fatalf("no <input for %q", marker)
		}
		region := got[inputOpen:idx]
		if !strings.Contains(region, `value="2"`) {
			t.Errorf("input %q does not carry value=\"2\" default; region:\n%s", marker, region)
		}
	}
}

// TestTableBuilderModal_HasMinMaxBounds — issue #610 slice 5.
// The rows/cols inputs bound 1 <= n <= 20 so the generated
// table stays reasonable. Anything outside is clamped by the
// JS handler at insert time (also asserted in
// data-table-builder-* via the HTML5 min/max attrs so the
// browser surfaces a warning).
func TestTableBuilderModal_HasMinMaxBounds(t *testing.T) {
	var buf bytes.Buffer
	if err := TableBuilderModal().Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()

	for _, input := range []string{
		`data-table-builder-rows`,
		`data-table-builder-cols`,
	} {
		idx := strings.Index(got, input)
		if idx < 0 {
			continue
		}
		// The input tag wraps the data attr. Walk BACKWARD
		// from the marker to find <input ...>, then take the
		// 600 bytes before the marker as the region to search
		// (the data attr is the LAST attribute on the input
		// per templ's render order: id, name, type, min,
		// max, value, class, data-*).
		inputOpen := strings.LastIndex(got[:idx], "<input")
		if inputOpen < 0 {
			t.Errorf("could not find <input for marker %q", input)
			continue
		}
		region := got[inputOpen:idx]
		if !strings.Contains(region, `min="1"`) {
			t.Errorf("input %q missing min=\"1\"\nregion:\n%s", input, region)
		}
		if !strings.Contains(region, `max="20"`) {
			t.Errorf("input %q missing max=\"20\"\nregion:\n%s", input, region)
		}
	}
}

// TestTableBuilderModal_HiddenByDefault — issue #610 slice 5.
// The modal starts hidden. showOverlayModal removes the
// hidden class + adds flex; hideOverlayModal restores hidden.
func TestTableBuilderModal_HiddenByDefault(t *testing.T) {
	var buf bytes.Buffer
	if err := TableBuilderModal().Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()

	if !strings.Contains(got, "table-builder-modal") {
		t.Fatal("modal id missing")
	}
	modalIdx := strings.Index(got, `id="table-builder-modal"`)
	region := got[modalIdx:]
	if len(region) > 200 {
		region = region[:200]
	}
	if !strings.Contains(region, "hidden") {
		t.Errorf("modal must be hidden on initial render\nregion:\n%s", region)
	}
}