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