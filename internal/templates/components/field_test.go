package components

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

// TestField_InputDefault verifies the simplest input call: no type
// (HTML default = text), no extras. Byte-stable against the legacy
// `<input class="field-input">`.
func TestField_InputDefault(t *testing.T) {
	var buf bytes.Buffer
	if err := Field(FieldInput, nil).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	want := `<input class="field-input">`
	if got != want {
		t.Fatalf("default input drift:\n got: %q\nwant: %q", got, want)
	}
}

// TestField_InputWithClass verifies that caller-supplied class tokens
// are appended after "field-input". The legacy pattern was
// `<input class="field-input mt-2 min-h-24" ...>`; preserving order
// keeps snapshot tests green.
func TestField_InputWithClass(t *testing.T) {
	var buf bytes.Buffer
	err := Field(FieldInput, templ.Attributes{
		"class": "field-input mt-2 min-h-24",
		"name":  "first_name",
		"id":    "ef-first_name",
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	want := `<input class="field-input mt-2 min-h-24"`
	if !strings.HasPrefix(got, want) {
		t.Fatalf("input missing class prefix:\n got: %q\nwant prefix: %q", got, want)
	}
	if !strings.Contains(got, `name="first_name"`) || !strings.Contains(got, `id="ef-first_name"`) {
		t.Fatalf("input missing name/id:\n%s", got)
	}
}

// TestField_InputWithType verifies that an explicit type attribute
// appears on the rendered input.
func TestField_InputWithType(t *testing.T) {
	var buf bytes.Buffer
	err := Field(FieldInput, templ.Attributes{
		"type":        "email",
		"name":        "contact_email",
		"placeholder": "Optional",
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, `type="email"`) || !strings.Contains(got, `name="contact_email"`) {
		t.Fatalf("email input missing attrs:\n%s", got)
	}
}

// TestField_Textarea verifies the textarea branch renders with body
// content from attrs["value"]. The legacy form was
// `<textarea class="field-input min-h-24" ...>{value}</textarea>`.
func TestField_Textarea(t *testing.T) {
	var buf bytes.Buffer
	err := Field(FieldTextarea, templ.Attributes{
		"class":       "field-input min-h-24",
		"name":        "findagrave_source",
		"placeholder": "Paste Find a Grave memorial HTML here",
		"value":       "prefilled content",
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	for _, needle := range []string{
		`<textarea`,
		`class="field-input min-h-24"`,
		`name="findagrave_source"`,
		`placeholder="Paste Find a Grave memorial HTML here"`,
		`>prefilled content</textarea>`,
	} {
		if !strings.Contains(got, needle) {
			t.Fatalf("textarea missing %q\nfull: %s", needle, got)
		}
	}
}

// TestField_Select verifies the select branch renders children.
func TestField_Select(t *testing.T) {
	var buf bytes.Buffer
	child := templ.Raw(`<option value="bug">Bug</option>`)
	err := Field(FieldSelect, templ.Attributes{
		"name": "category",
		"class": "field-input",
	}).Render(templ.WithChildren(context.Background(), child), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, `<select`) || !strings.Contains(got, `class="field-input"`) {
		t.Fatalf("select drift:\n%s", got)
	}
	if !strings.Contains(got, `<option value="bug">Bug</option>`) {
		t.Fatalf("select missing child option:\n%s", got)
	}
}