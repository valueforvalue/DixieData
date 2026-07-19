package components

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

// TestButtonVariants consolidates the per-kind snapshot tests into one
// table-driven case. Each row pins the rendered HTML for one
// ButtonKind → CSS class pairing. The byte-stability rule is the
// load-bearing contract: every site that swaps a legacy class for
// @Button must keep its existing snapshot tests green, so the
// rendered surface is provably unchanged.
func TestButtonVariants(t *testing.T) {
	cases := []struct {
		name string
		kind ButtonKind
		text string
		want string
	}{
		{
			name: "primary",
			kind: ButtonPrimary,
			text: "Save",
			want: `<button type="button" class="primary-button">Save</button>`,
		},
		{
			name: "secondary",
			kind: ButtonSecondary,
			text: "Cancel",
			want: `<button type="button" class="secondary-button">Cancel</button>`,
		},
		{
			name: "ghost",
			kind: ButtonGhost,
			text: "Help",
			want: `<button type="button" class="ghost-link">Help</button>`,
		},
		{
			name: "danger",
			kind: ButtonDanger,
			text: "Delete",
			want: `<button type="button" class="danger-button">Delete</button>`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := Button(c.text, c.kind, "", nil).Render(context.Background(), &buf); err != nil {
				t.Fatalf("Render: %v", err)
			}
			if got := buf.String(); got != c.want {
				t.Fatalf("%s button snapshot drift:\n got: %q\nwant: %q", c.name, got, c.want)
			}
		})
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
		"type":       "submit",
		"name":       "save",
		"data-id":    "42",
		"hx-post":    "/soldiers",
		"hx-target":  "#main",
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

// TestButton_AttrsPassThroughSafeURL (issue #365) pins the regression:
// caller-supplied attrs whose value is templ.SafeURL (the wrapper that
// marks a URL as already-sanitized) must render the same as a plain
// string value. The bug surfaces only for templ.SafeURL because the
// templ SDK attribute-spread filters values it does not recognize as
// primitives -- templ.SafeURL (a typed string named value) should be
// equivalent to a plain string for render, but the spread currently
// drops it. Until the primitive handles the wrapper, callers must
// convert to a plain string OR drop the wrapper (unsafe -- templ.SafeURL
// exists to mark URLs as already-sanitized, side-stepping the templ
// sanitizer).
func TestButton_AttrsPassThroughSafeURL(t *testing.T) {
	var buf bytes.Buffer
	err := Button("Add Images From Computer", ButtonPrimary, "", templ.Attributes{
		"data-action":         templ.SafeURL("/events/123/images/import"),
		"data-dixie-submit":   "true",
		"data-progress-label": "Importing images\u2026",
	}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	for _, needle := range []string{
		`data-action="/events/123/images/import"`,
		`data-dixie-submit="true"`,
		"data-progress-label=\"Importing images\u2026\"",
		`class="primary-button"`,
	} {
		if !strings.Contains(got, needle) {
			t.Fatalf("SafeURL-attrs button missing %q\nfull: %s", needle, got)
		}
	}
}
