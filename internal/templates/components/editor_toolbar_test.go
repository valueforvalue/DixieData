package components

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestEditorToolbar_RendersAllButtons — issue #610 slice 4.
// The article editor's toolbar sits between the body label
// and the textarea. Every button is identified by a
// `data-editor-toolbar-action` attr so the JS initializer
// can wire the click handler to insert the matching syntax
// template via window.__dixieInsertTextAtCursor. The toolbar
// ships 9 buttons: Bold, Italic, Heading, Link, Image,
// List, Code, Blockquote, Table.
func TestEditorToolbar_RendersAllButtons(t *testing.T) {
	var buf bytes.Buffer
	if err := EditorToolbar().Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()

	wantActions := []string{
		"bold",
		"italic",
		"heading",
		"link",
		"image",
		"list",
		"code",
		"quote",
		"table",
	}
	for _, action := range wantActions {
		marker := `data-editor-toolbar-action="` + action + `"`
		if !strings.Contains(got, marker) {
			t.Errorf("missing toolbar button for action %q (marker %q)", action, marker)
		}
	}
}

// TestEditorToolbar_TemplateValuesAreMarkdown — issue #610 slice 4.
// Every toolbar button carries a `data-editor-toolbar-template`
// attr with the Markdown template that gets inserted at cursor.
// Templates use placeholder text (e.g. "label", "url") so the
// user can edit them inline; the JS handler does NOT replace
// the placeholders — the user types over them. The Template
// field on CheatSheetRow carries the same example shape; both
// come from the same source of truth (goldmark GFM subset
// mirrored in internal/articles/markdown_cheatsheet.go).
func TestEditorToolbar_TemplateValuesAreMarkdown(t *testing.T) {
	var buf bytes.Buffer
	if err := EditorToolbar().Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()

	// Spot-check each template — the exact template string
	// matters because the JS inserts it verbatim. A typo
	// here would silently ship a wrong Markdown fragment.
	wantTemplates := map[string]string{
		"bold":     "**bold text**",
		"italic":   "*italic text*",
		"heading":  "## Heading",
		"link":     "[label](https://)",
		"image":    "![alt](https://)",
		"list":     "- item",
		"code":     "`code`",
		"quote":    "&gt; quote",
		// table action does NOT carry a static template —
		// the JS opens the modal which generates the
		// template from user-supplied rows × cols.
	}
	for action, template := range wantTemplates {
		// Find the button's opening tag (with the data attr)
		// and read 200 bytes forward to find its template.
		marker := `data-editor-toolbar-action="` + action + `"`
		idx := strings.Index(got, marker)
		if idx < 0 {
			continue // already covered by RendersAllButtons
		}
		region := got[idx:]
		if len(region) > 1024 {
			region = region[:1024]
		}
		wantMarker := `data-editor-toolbar-template="` + template
		if !strings.Contains(region, wantMarker) {
			t.Errorf("action %q: missing template %q (marker %q) in button region", action, template, wantMarker)
		}
	}
}

// TestEditorToolbar_HasNoFormSubmitMarkers — issue #610 slice 4.
// The toolbar buttons are plain text-insert actions; they
// must NOT participate in the form-submit dispatcher
// (the Wails patch-method bug from issue #428) and must
// NOT carry data-dixie-submit. The toolbar lives INSIDE the
// <form> but the buttons are type="button" + non-submit.
func TestEditorToolbar_HasNoFormSubmitMarkers(t *testing.T) {
	var buf bytes.Buffer
	if err := EditorToolbar().Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	if strings.Contains(got, "data-dixie-submit") {
		t.Errorf("EditorToolbar must not emit data-dixie-submit; the toolbar buttons are not form submits")
	}
	if strings.Contains(got, `type="submit"`) {
		t.Errorf("EditorToolbar must not emit type=submit buttons; the toolbar is in-form but inert")
	}
}

// TestEditorToolbar_HasAccessibleLabels — issue #610 slice 4.
// Every button has an aria-label so screen readers announce
// the action (Unicode / single-character button labels don't
// read well). The labels are the human-readable action names,
// not the underlying Markdown.
func TestEditorToolbar_HasAccessibleLabels(t *testing.T) {
	var buf bytes.Buffer
	if err := EditorToolbar().Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()

	wantLabels := []string{
		"Bold",
		"Italic",
		"Heading",
		"Link",
		"Image",
		"List",
		"Code",
		"Quote",
		"Table",
	}
	for _, label := range wantLabels {
		wantMarker := `aria-label="` + label
		if !strings.Contains(got, wantMarker) {
			t.Errorf("toolbar button missing aria-label=%q", label)
		}
	}
}

// TestEditorToolbar_RendersUndoRedo — issue #611 slice 2.
// The toolbar ships an Undo + Redo button alongside the
// existing 9 format buttons. Each is identified by
// data-editor-toolbar-action="undo" / "redo" so the JS
// initializer can wire the click handler to dispatch
// document.execCommand("undo") / "redo". The buttons
// start disabled (the textarea has no undo history on
// first load); the JS poller updates the disabled state
// on input + every 500ms based on queryCommandEnabled.
func TestEditorToolbar_RendersUndoRedo(t *testing.T) {
	var buf bytes.Buffer
	if err := EditorToolbar().Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()

	for _, action := range []string{"undo", "redo"} {
		marker := `data-editor-toolbar-action="` + action + `"`
		if !strings.Contains(got, marker) {
			t.Errorf("toolbar missing %s button (marker %q)", action, marker)
		}
		// No template attr — Undo/Redo are actions, not
		// inserts. The absence is the contract: a future
		// refactor that accidentally adds a template
		// would route the click through the insert path
		// instead of the execCommand path.
		templateMarker := `data-editor-toolbar-template="`
		actionIdx := strings.Index(got, marker)
		if actionIdx < 0 {
			continue
		}
		// Look ahead 300 bytes for the next data-editor-toolbar-
		// template attr; it must NOT appear before the next
		// action (which would mean the Undo/Redo button has
		// a stray template).
		region := got[actionIdx:]
		if len(region) > 300 {
			region = region[:300]
		}
		// The next /button> tag must come before any template
		// marker, AND the button must not be followed by a
		// data-editor-toolbar-template attr.
		nextButtonEnd := strings.Index(region, "</button>")
		if nextButtonEnd < 0 {
			t.Errorf("no </button> for action %q", action)
			continue
		}
		region = region[:nextButtonEnd]
		if strings.Contains(region, templateMarker) {
			t.Errorf("Undo/Redo button %q has a stray data-editor-toolbar-template attr; Undo/Redo are actions, not inserts", action)
		}
	}
}

// TestEditorToolbar_UndoRedoStartDisabled — issue #611 slice 2.
// The Undo + Redo buttons render with the `disabled` attribute
// on first paint. The textarea has no undo history on first
// load, so the buttons should be disabled until the user
// makes their first edit (which adds an undo entry) AND the
// JS poller has had a chance to update the state.
func TestEditorToolbar_UndoRedoStartDisabled(t *testing.T) {
	var buf bytes.Buffer
	if err := EditorToolbar().Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()

	for _, action := range []string{"undo", "redo"} {
		marker := `data-editor-toolbar-action="` + action + `"`
		idx := strings.Index(got, marker)
		if idx < 0 {
			continue
		}
		// Walk back to the enclosing <button ...> opener; the
		// disabled attr is on the button tag itself, not after
		// the data attr.
		buttonOpen := strings.LastIndex(got[:idx], "<button")
		if buttonOpen < 0 {
			t.Errorf("no <button for action %q", action)
			continue
		}
		buttonEnd := strings.Index(got[buttonOpen:], "</button>")
		if buttonEnd < 0 {
			t.Errorf("no </button> for action %q", action)
			continue
		}
		buttonEnd += buttonOpen
		region := got[buttonOpen:buttonEnd]
		if !strings.Contains(region, "disabled") {
			t.Errorf("Undo/Redo button %q must start disabled (no undo history on first paint)\nbutton region:\n%s", action, region)
		}
	}
}

// TestEditorToolbar_UndoRedoHaveAccessibleLabels — issue #611 slice 2.
// The Undo + Redo buttons carry aria-label attributes so
// screen readers announce the action.
func TestEditorToolbar_UndoRedoHaveAccessibleLabels(t *testing.T) {
	var buf bytes.Buffer
	if err := EditorToolbar().Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()

	for _, label := range []string{"Undo", "Redo"} {
		wantMarker := `aria-label="` + label + `"`
		if !strings.Contains(got, wantMarker) {
			t.Errorf("toolbar button missing aria-label=%q", label)
		}
	}
}