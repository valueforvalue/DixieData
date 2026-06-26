package components

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

// TestButton_PrimarySnapshot asserts that @Button("Save", ButtonPrimary, nil)
// renders byte-equivalent HTML to the legacy inline
// <button type="button" class="primary-button">Save</button> form. The
// byte-stability rule is the load-bearing contract: every site that
// swaps a legacy class for @Button must keep its existing snapshot tests
// green, so the rendered surface is provably unchanged.
func TestButton_PrimarySnapshot(t *testing.T) {
	var buf bytes.Buffer
	if err := Button("Save", ButtonPrimary, "", nil).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	want := `<button type="button" class="primary-button">Save</button>`
	if got != want {
		t.Fatalf("primary button snapshot drift:\n got: %q\nwant: %q", got, want)
	}
}

func TestButton_SecondarySnapshot(t *testing.T) {
	var buf bytes.Buffer
	if err := Button("Cancel", ButtonSecondary, "", nil).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got, want := buf.String(), `<button type="button" class="secondary-button">Cancel</button>`; got != want {
		t.Fatalf("secondary button snapshot drift:\n got: %q\nwant: %q", got, want)
	}
}

func TestButton_GhostSnapshot(t *testing.T) {
	var buf bytes.Buffer
	if err := Button("Help", ButtonGhost, "", nil).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got, want := buf.String(), `<button type="button" class="ghost-link">Help</button>`; got != want {
		t.Fatalf("ghost button snapshot drift:\n got: %q\nwant: %q", got, want)
	}
}

func TestButton_DangerSnapshot(t *testing.T) {
	var buf bytes.Buffer
	if err := Button("Delete", ButtonDanger, "", nil).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got, want := buf.String(), `<button type="button" class="danger-button">Delete</button>`; got != want {
		t.Fatalf("danger button snapshot drift:\n got: %q\nwant: %q", got, want)
	}
}

// TestButton_ExtraClass verifies that extraClass is appended after the
// kind class with a single space. Many existing call sites pass combined
// classes like "secondary-button floating-dock-button".
func TestButton_ExtraClass(t *testing.T) {
	var buf bytes.Buffer
	if err := Button("Scratch Pad", ButtonSecondary, "floating-dock-button", nil).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got, want := buf.String(), `<button type="button" class="secondary-button floating-dock-button">Scratch Pad</button>`; got != want {
		t.Fatalf("extra class drift:\n got: %q\nwant: %q", got, want)
	}
}

// TestButton_AttrsPassThrough verifies that caller-supplied attributes
// (type="submit", data-*, aria-*, hx-*) appear on the rendered element.
// The button primitive is intentionally transparent about extra attrs.
func TestButton_AttrsPassThrough(t *testing.T) {
	var buf bytes.Buffer
	err := Button("Submit Form", ButtonPrimary, "", templ.Attributes{
		"type":     "submit",
		"name":     "save",
		"data-id":  "42",
		"hx-post":  "/soldiers",
		"hx-target": "#main",
		"aria-label": "Save soldier record",
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	for _, needle := range []string{
		`type="submit"`,
		`name="save"`,
		`data-id="42"`,
		`hx-post="/soldiers"`,
		`hx-target="#main"`,
		`aria-label="Save soldier record"`,
		`class="primary-button"`,
	} {
		if !strings.Contains(got, needle) {
			t.Fatalf("submit button missing %q\nfull: %s", needle, got)
		}
	}
}

// TestButton_UnknownKindFallback verifies that a typo in the kind
// argument falls back to the secondary-button class instead of emitting
// an unstyled <button>. Catches the most common caller-side bug.
func TestButton_UnknownKindFallback(t *testing.T) {
	var buf bytes.Buffer
	if err := Button("Save", ButtonKind("typo"), "", nil).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got, want := buf.String(), `<button type="button" class="secondary-button">Save</button>`; got != want {
		t.Fatalf("unknown kind fallback drift:\n got: %q\nwant: %q", got, want)
	}
}

// TestButton_TypeNotDuplicatedFromAttrs asserts that passing
// type="submit" via attrs does NOT produce a duplicate type=
// attribute in the rendered HTML. The primitive owns the type
// attribute; the { attrs... } spread must strip it.
//
// Regression: discovered during the soldier_card.templ migration
// (Export JPG rendered as <button type="submit" ... type="submit">).
func TestButton_TypeNotDuplicatedFromAttrs(t *testing.T) {
	var buf bytes.Buffer
	if err := Button("Submit", ButtonPrimary, "", templ.Attributes{
		"type":    "submit",
		"name":    "action",
		"hx-post": "/soldiers",
	}).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	if strings.Count(got, "type=") != 1 {
		t.Fatalf("expected exactly one type= attribute, got %d:\n%s", strings.Count(got, "type="), got)
	}
	if !strings.Contains(got, `type="submit"`) {
		t.Fatalf("missing type=\"submit\":\n%s", got)
	}
}