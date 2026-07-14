// person_record_picker_test.go — issue #574
// Render invariants for the Person Record picker component.
//
//   - The Cancel button carries the data-person-record-picker-clear
//     attribute (the new initializer hook in app.js's
//     initializePersonRecordPicker) and carries the
//     data-person-record-picker-close attribute (the existing
//     hook).
//   - The Cancel button does NOT carry an inline `onclick=`
//     attribute. Issue #574 retired the lone inline onclick in
//     the codebase; this test pins the invariant so a future
//     refactor doesn't reintroduce it.
//   - The picker shell carries the data-person-record-picker
//     container attribute so the initializer's closest() lookup
//     has something to grab onto.
//
// The behavioral test for the click handler itself (that the
// target div's innerHTML is cleared on click) lives in the
// audit harness — the picker shell is rendered server-side and
// rendered through htmx on the article editor; smoke_articles.mjs
// exercises the full path end-to-end. This Go test pins the
// markup contract.
package components

import (
	"bytes"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

func TestPersonRecordPickerCancelButtonHasClearMarkerAndNoInlineOnclick(t *testing.T) {
	component := PersonRecordPicker(1, "", []viewmodel.PersonRecord{})
	var buf bytes.Buffer
	if err := component.Render(t.Context(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()

	// The shell container attribute is the scope anchor for
	// the clear handler's closest() lookup — without it the
	// initializer cannot find the picker.
	if !strings.Contains(html, `data-person-record-picker`) {
		t.Fatal("picker shell must carry data-person-record-picker anchor")
	}

	// The Cancel button must expose the new initializer hook
	// (`data-person-record-picker-clear`) and the existing
	// close hook (`data-person-record-picker-close`).
	if !strings.Contains(html, `data-person-record-picker-close`) {
		t.Error("Cancel button must carry data-person-record-picker-close")
	}
	if !strings.Contains(html, `data-person-record-picker-clear`) {
		t.Error("Cancel button must carry data-person-record-picker-clear (issue #574)")
	}

	// The Cancel button must NOT carry an inline onclick
	// attribute — issue #574 retired the lone inline JS in
	// the templ codebase. Pin the absence so a future
	// refactor that quietly re-introduces it trips this
	// test.
	if strings.Contains(html, `onclick=`) {
		t.Error("picker shell must not carry any inline onclick attribute (issue #574 DRY contract)")
	}
}
