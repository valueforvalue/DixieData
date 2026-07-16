package components

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

// TestTermDisclosure_RendersTrigger pins the trigger's
// minimum ARIA + data-* contract (issue #564 slice 2):
//
//   - <button> tag (not a <div>).
//   - aria-expanded="false" baseline.
//   - aria-controls points at the panel's id.
//   - data-term-disclosure-trigger="<slug>" hook.
//   - data-term-disclosure-short="<short>" payload.
//
// RED at HEAD: the templ doesn't exist yet — compile error.
// GREEN at slice 2 GREEN step: this test renders cleanly.
func TestTermDisclosure_RendersTrigger(t *testing.T) {
	var buf bytes.Buffer
	err := TermDisclosure("display-id", "The canonical user-facing identifier.", templ.NopComponent).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	for _, want := range []string{
		`<button`,
		`type="button"`,
		`aria-expanded="false"`,
		`aria-controls="term-disclosure-panel-display-id"`,
		`data-term-disclosure-trigger="display-id"`,
		`data-term-disclosure-short="The canonical user-facing identifier."`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("term-disclosure trigger missing %q\n  rendered: %s", want, content)
		}
	}
}

// TestTermDisclosure_RendersHiddenPanel pins the panel's
// hidden + role + id contract. The panel is hidden by
// default and carries the popover short + the "Read in
// glossary" link to the /about kebab anchor.
func TestTermDisclosure_RendersHiddenPanel(t *testing.T) {
	var buf bytes.Buffer
	err := TermDisclosure("person-record", "A primary archive entry.", templ.NopComponent).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	for _, want := range []string{
		`id="term-disclosure-panel-person-record"`,
		`role="region"`,
		`aria-label="Glossary disclosure"`,
		`data-term-disclosure-panel="person-record"`,
		`hidden`,
		`data-term-disclosure-short="A primary archive entry."`,
		`href="/about#about.glossary-person-record"`,
		`data-term-disclosure-read-in-glossary="person-record"`,
		`>Read in glossary<`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("term-disclosure panel missing %q\n  rendered: %s", want, content)
		}
	}
}

// TestTermDisclosureReadInGlossaryAnchorURL pins the
// exact /about#about.glossary-<slug> cross-link shape.
// The link is the same anchor pattern the canonical
// /about row uses (issue #564 slice 1); a future
// refactor that changes the prefix would break both
// surfaces simultaneously — this test fires so the
// click doesn't 404 mid-session.
func TestTermDisclosureReadInGlossaryAnchorURL(t *testing.T) {
	var buf bytes.Buffer
	if err := TermDisclosure("shared-archive", "A merge-oriented package.", templ.NopComponent).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	want := `href="/about#about.glossary-shared-archive"`
	if !strings.Contains(content, want) {
		t.Errorf("Read in glossary link missing %q\n  rendered: %s", want, content)
	}
}

// TestTermDisclosureShortLookup pins the slice-2 helper's
// 2 known slugs + the unknown-slug-misses-cleanly contract.
// Future slices extend the switch; this test fires if a
// future refactor accidentally drops one of the 2 entries.
// TestTermDisclosureExportedWrappers pins the exported
// wrappers (slice 3+ added 3 more: source-record, claim,
// finding). The wrappers are the public API callers use
// so the helper itself stays private.
func TestTermDisclosureExportedWrappers(t *testing.T) {
	if got := TermDisclosureDisplayIDShort(); got == "" {
		t.Errorf("TermDisclosureDisplayIDShort returned empty")
	}
	if got := TermDisclosurePersonRecordShort(); got == "" {
		t.Errorf("TermDisclosurePersonRecordShort returned empty")
	}
	if got := TermDisclosureSourceRecordShort(); got == "" {
		t.Errorf("TermDisclosureSourceRecordShort returned empty")
	}
	if got := TermDisclosureTagShort(); got == "" {
		t.Errorf("TermDisclosureTagShort returned empty")
	}
	if got := TermDisclosureClaimShort(); got == "" {
		t.Errorf("TermDisclosureClaimShort returned empty")
	}
	if got := TermDisclosureFindingShort(); got == "" {
		t.Errorf("TermDisclosureFindingShort returned empty")
	}
}

func TestTermDisclosureShortLookup(t *testing.T) {
	tests := []struct {
		slug string
		want string
	}{
		{slug: "display-id", want: "The canonical user-facing identifier for a Person Record."},
		{slug: "person-record", want: "A primary archive entry for one person (soldier, wife, widow, linked person, or event)."},
		{slug: "source-record", want: "An attached evidence item that documents or supports a Person Record (pension, application, etc.)."},
		{slug: "tag", want: "A user-defined free-text label applied to a Person Record for ad-hoc grouping."},
		{slug: "claim", want: "An assertion about a person that is extracted from a Source Record."},
		{slug: "finding", want: "A researcher-endorsed conclusion reached by weighing one or more Claims."},
		{slug: "definitely-not-a-real-slug", want: ""},
	}
	for _, tt := range tests {
		got := termDisclosureShort(tt.slug)
		if got != tt.want {
			t.Errorf("termDisclosureShort(%q) = %q, want %q", tt.slug, got, tt.want)
		}
	}
}
