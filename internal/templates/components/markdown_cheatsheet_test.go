// markdown_cheatsheet_test.go — issue #565
// Render + a11y invariants for the Article editor's
// Markdown syntax cheatsheet component. Pins:
//
//   - The component renders a Foldout (issue #264 primitive)
//     with the trigger label "Markdown syntax" + the chevron
//   - The Foldout panel has the registry surface ID
//     uiids.PanelArticleMarkdownCheatsheet
//   - The panel is hidden on initial render
//   - Every article.CheatSheetRow is rendered as a
//     <li role="none"> containing a menuitem with the
//     syntax, effect, and example text
//   - The rows render in the order articles.Rows() returns
//     them (visual reading order matters for the popover)
//   - The Copy example button is identified by
//     data-md-cheatsheet-copy-key="{row.Key}" so the JS
//     initializer can wire the clipboard handler
//
// This test runs alongside the existing foldout test suite
// (foldout_test.go) — the Foldout primitive guarantees
// aria-haspopup, aria-expanded, aria-controls, the menu
// role, and the children-rendering regression net from
// issue #456. The cheatsheet component inherits that
// contract.
package components

import (
	"bytes"
	"context"
	"html"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/articles"
	"github.com/valueforvalue/DixieData/internal/uiids"
)

// htmlEscape mirrors html.EscapeString for the four chars
// templ escapes when rendering text into attribute / body
// contexts. The drift test on the row's Syntax field uses
// this so the rendered-output assertion matches the wire
// shape the user sees, not the raw source string.
func htmlEscape(s string) string {
	return html.EscapeString(s)
}

func TestMarkdownCheatsheet_RendersFoldoutTrigger(t *testing.T) {
	var buf bytes.Buffer
	if err := MarkdownCheatsheet().Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()

	// The trigger must say "Markdown syntax" so the user
	// can find the popover next to Preview / Save Article.
	// The Foldout primitive appends a chevron <span> after
	// the label, so the trigger's text content reads
	// "Markdown syntax ▾" with a space between label and
	// chevron.
	if !strings.Contains(got, "Markdown syntax") {
		t.Errorf("trigger label missing; got:\n%s", got)
	}

	// Foldout ARIA contract — the trigger is a button with
	// the haspopup + expanded + controls attrs the existing
	// Foldout primitive owns.
	for _, want := range []string{
		`<button`,
		`type="button"`,
		`aria-haspopup="menu"`,
		`aria-expanded="false"`,
		`data-foldout-trigger="` + uiids.PanelArticleMarkdownCheatsheet + `"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("trigger missing %q\nfull render:\n%s", want, got)
		}
	}
}

func TestMarkdownCheatsheet_RendersPanelSurfaceID(t *testing.T) {
	var buf bytes.Buffer
	if err := MarkdownCheatsheet().Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()

	// The panel must carry the registry surface ID so
	// htmx swap targets + audit probes can identify it.
	if !strings.Contains(got, `id="`+uiids.PanelArticleMarkdownCheatsheet+`"`) {
		t.Errorf("panel missing registry id %q\nfull render:\n%s", uiids.PanelArticleMarkdownCheatsheet, got)
	}
	if !strings.Contains(got, `data-foldout-panel="`+uiids.PanelArticleMarkdownCheatsheet+`"`) {
		t.Errorf("panel missing foldout data-attr\nfull render:\n%s", got)
	}

	// The panel must start hidden — opening the popover
	// is the JS initializer's job, not the template's.
	if !strings.Contains(got, "foldout-panel") || !strings.Contains(got, "hidden") {
		t.Errorf("panel must be hidden on initial render\nfull render:\n%s", got)
	}
}

func TestMarkdownCheatsheet_RendersAllRows(t *testing.T) {
	var buf bytes.Buffer
	if err := MarkdownCheatsheet().Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()

	rows := articles.Rows()
	if len(rows) == 0 {
		t.Fatal("articles.Rows() returned no rows; data not implemented")
	}

	// Every row must appear in the rendered output with its
	// effect label, syntax, example, and a Copy example
	// button identified by data-md-cheatsheet-copy-key.
	for _, r := range rows {
		copyMarker := `data-md-cheatsheet-copy-key="` + r.Key + `"`
		if !strings.Contains(got, copyMarker) {
			t.Errorf("row %q missing Copy example button (%s)\nfull render:\n%s", r.Key, copyMarker, got)
		}
		if !strings.Contains(got, r.Syntax) {
			// Syntax field is rendered as text inside a
			// <span>; HTML special chars get escaped
			// (e.g. "<" -> "&lt;"). The escaped form is
			// what the user actually sees; the raw form
			// is the source of truth for the copy-to-
			// clipboard handler. Test the escaped form so
			// the assertion matches the wire shape.
			escaped := htmlEscape(r.Syntax)
			if !strings.Contains(got, escaped) {
				t.Errorf("row %q: rendered output missing syntax %q (escaped %q)\nfull render:\n%s", r.Key, r.Syntax, escaped, got)
			}
		}
		if !strings.Contains(got, r.Effect) {
			t.Errorf("row %q: rendered output missing effect label %q\nfull render:\n%s", r.Key, r.Effect, got)
		}
	}
}

func TestMarkdownCheatsheet_RowsInOrder(t *testing.T) {
	// Visual reading order is part of the UX: basic
	// primitives first (heading, paragraph, emphasis,
	// strong), advanced features last (table, hr). The
	// popover scrolls top-to-bottom, so the row order in
	// articles.Rows() must match the render order.
	var buf bytes.Buffer
	if err := MarkdownCheatsheet().Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()

	rows := articles.Rows()
	prevIdx := -1
	for _, r := range rows {
		marker := `data-md-cheatsheet-copy-key="` + r.Key + `"`
		idx := strings.Index(got, marker)
		if idx < 0 {
			t.Fatalf("row %q missing from render", r.Key)
		}
		if idx <= prevIdx {
			t.Errorf("row %q renders out of order (idx=%d, prev=%d); popover reading order must match articles.Rows()", r.Key, idx, prevIdx)
		}
		prevIdx = idx
	}
}

func TestMarkdownCheatsheet_RowWrapperHasCorrectARIA(t *testing.T) {
	// Each row is a <li role="none"> wrapping a
	// <button role="menuitem"> (the Copy example button).
	// The WAI-ARIA menu pattern requires the role="none"
	// on the <li> so the menu role on the <ul> is not
	// shadowed by a competing <li> role. This matches the
	// foldout_test.go children contract.
	var buf bytes.Buffer
	if err := MarkdownCheatsheet().Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()

	if !strings.Contains(got, `<li role="none"`) {
		t.Errorf("rows must be wrapped in <li role=\"none\"> per the WAI-ARIA menu pattern\nfull render:\n%s", got)
	}
	if !strings.Contains(got, `role="menuitem"`) {
		t.Errorf("Copy example button must carry role=\"menuitem\" for the WAI-ARIA menu pattern\nfull render:\n%s", got)
	}
}

func TestMarkdownCheatsheet_DoesNotEmitFormSubmitMarkers(t *testing.T) {
	// The Copy example button is a per-row action. It
	// must NOT participate in the form-submit dispatcher
	// (the Wails patch-method bug from issue #428) and
	// must NOT carry data-dixie-submit. This is the
	// regression net for the dispatcher safety contract.
	var buf bytes.Buffer
	if err := MarkdownCheatsheet().Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	if strings.Contains(got, "data-dixie-submit") {
		t.Errorf("MarkdownCheatsheet must not emit data-dixie-submit; the Copy example button is not a form submit\nfull render:\n%s", got)
	}
}

// TestMarkdownCheatsheet_RendersLivePreview — issue #610 slice 1.
// Every cheatsheet row ships a tiny rendered preview of what
// its Markdown syntax produces. The cheatsheet currently
// shows Syntax + Effect + Example as text; the live preview
// makes the cheatsheet self-documenting so the user can see
// "**bold**" → actual <strong>bold</strong> without having
// to mentally translate.
//
// The preview is rendered server-side via the same goldmark
// pipeline the article editor uses (records.MarkdownRenderer).
// The component-level helper is in
// components/markdown_cheatsheet.go (RenderPreview); the row
// itself is left untouched so the articles package stays a
// leaf node (no records import).
func TestMarkdownCheatsheet_RendersLivePreview(t *testing.T) {
	var buf bytes.Buffer
	if err := MarkdownCheatsheet().Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()

	// Spot-check three rows whose rendered HTML shape is
	// unambiguous: heading renders to <h2> with the example
	// text; strong renders to <strong>; link renders to <a>
	// with the href. If any preview is missing, the user
	// sees raw syntax instead of the rendered output.
	type previewCheck struct {
		rowKey     string
		wantMarker string
	}
	checks := []previewCheck{
		{"heading", "<h2>The Battle of Gettysburg</h2>"},
		{"strong", "<strong>important</strong>"},
		{"link", `<a href="https://www.nps.gov/gett/">NPS Gettysburg</a>`},
	}
	for _, c := range checks {
		marker := `data-md-cheatsheet-preview-key="` + c.rowKey + `"`
		idx := strings.Index(got, marker)
		if idx < 0 {
			t.Errorf("row %q: missing preview surface marker %q\nfull render:\n%s", c.rowKey, marker, got)
			continue
		}
		// The preview content lives in the same <li> as the
		// marker; look ahead 2KB for the want-marker so a
		// future marker from a different row doesn't satisfy.
		end := idx + 2048
		if end > len(got) {
			end = len(got)
		}
		region := got[idx:end]
		if !strings.Contains(region, c.wantMarker) {
			t.Errorf("row %q: preview missing %q\npreview region:\n%s", c.rowKey, c.wantMarker, region)
		}
	}
}

// TestRenderPreview_PureHelper — issue #610 slice 1.
// Pin the RenderPreview helper's contract: input is a Markdown
// source string, output is the goldmark-rendered HTML
// (bluemonday-sanitized). Empty input returns empty string
// (matches the records.MarkdownRenderer.Render contract).
func TestRenderPreview_PureHelper(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string // substring of rendered output
	}{
		{"heading", "## Title", "<h2>Title</h2>"},
		{"strong", "**x**", "<strong>x</strong>"},
		{"empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := RenderPreview(c.input)
			if c.want == "" && got != "" {
				t.Errorf("RenderPreview(%q) = %q; want empty", c.input, got)
			}
			if c.want != "" && !strings.Contains(got, c.want) {
				t.Errorf("RenderPreview(%q) = %q; want substring %q", c.input, got, c.want)
			}
		})
	}
}

// TestMarkdownCheatsheet_RendersInsertButton — issue #610 slice 3.
// Every cheatsheet row ships an Insert-at-cursor button as the
// primary row action. Clicking the row inserts the example at
// the cursor in <textarea id="article-body"> via
// window.__dixieInsertTextAtCursor. The button is identified
// by data-md-cheatsheet-insert-key="<row.Key>" +
// data-md-cheatsheet-insert-value="<row.Example>" so the JS
// initializer can wire the click handler.
func TestMarkdownCheatsheet_RendersInsertButton(t *testing.T) {
	var buf bytes.Buffer
	if err := MarkdownCheatsheet().Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()

	rows := articles.Rows()
	if len(rows) == 0 {
		t.Fatal("articles.Rows() returned no rows; data not implemented")
	}

	for _, r := range rows {
		insertMarker := `data-md-cheatsheet-insert-key="` + r.Key + `"`
		if !strings.Contains(got, insertMarker) {
			t.Errorf("row %q: missing Insert button (%s)\nfull render:\n%s", r.Key, insertMarker, got)
		}
		// The insert-value attr must carry the row's example
		// (HTML-escaped when the source contains special chars
		// like <, >, &, "). Spot-check the first 20 chars of
		// the example so the assertion catches "wrong example"
		// without breaking on attribute encoding. Multi-line
		// examples (code-block, unordered-list, etc.) use \n
		// in the attribute value, which is fine.
		examplePrefix := r.Example
		if len(examplePrefix) > 20 {
			examplePrefix = examplePrefix[:20]
		}
		if examplePrefix == "" {
			continue
		}
		valueMarker := `data-md-cheatsheet-insert-value="` + htmlEscape(examplePrefix)
		if !strings.Contains(got, valueMarker) {
			t.Errorf("row %q: missing insert-value attr carrying example prefix %q\nfull render:\n%s", r.Key, examplePrefix, got)
		}
	}
}

// TestMarkdownCheatsheet_KeepsCopyButton — issue #610 slice 3.
// The Copy example button (originally the row's primary action
// in #565) stays as a secondary affordance alongside the new
// Insert button. Both data attrs must render so the existing
// initializeMarkdownCheatsheet() clipboard wiring continues to
// work unchanged.
func TestMarkdownCheatsheet_KeepsCopyButton(t *testing.T) {
	var buf bytes.Buffer
	if err := MarkdownCheatsheet().Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()

	rows := articles.Rows()
	for _, r := range rows {
		copyKey := `data-md-cheatsheet-copy-key="` + r.Key + `"`
		if !strings.Contains(got, copyKey) {
			t.Errorf("row %q: Copy button missing (data-md-cheatsheet-copy-key)\nfull render:\n%s", r.Key, got)
		}
	}
}

// TestMarkdownCheatsheet_InsertAndCopyButtonsAreSiblings — issue
// #610 slice 3. The Insert menuitem and Copy button are siblings
// inside the <li role="none">; clicking one doesn't bubble through
// the other. This test asserts the structural separation: each
// row has both data attrs and they belong to different buttons.
func TestMarkdownCheatsheet_InsertAndCopyButtonsAreSiblings(t *testing.T) {
	var buf bytes.Buffer
	if err := MarkdownCheatsheet().Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()

	rows := articles.Rows()
	for _, r := range rows {
		insertKey := `data-md-cheatsheet-insert-key="` + r.Key + `"`
		// Use the FIRST occurrence of the row's insert marker
		// and the FIRST occurrence of the row's copy marker
		// AFTER the insert marker — this guarantees both
		// markers belong to the same row (subsequent rows
		// would have a different Key). Skip the global
		// copyIdx = strings.Index(...) approach because a
		// previous row's Copy button sits BEFORE the current
		// row's Insert marker in the byte stream.
		insertIdx := strings.Index(got, insertKey)
		if insertIdx < 0 {
			t.Errorf("row %q: insert marker not found", r.Key)
			continue
		}
		copyKey := `data-md-cheatsheet-copy-key="` + r.Key + `"`
		copyIdx := strings.Index(got[insertIdx:], copyKey)
		if copyIdx < 0 {
			t.Errorf("row %q: copy marker not found after insert marker (insertIdx=%d)", r.Key, insertIdx)
			continue
		}
		copyIdx += insertIdx
		// The current row's outer </li> sits between the Copy
		// marker and the next row's <li>. The preview HTML can
		// contain NESTED </li> (e.g. the unordered-list row's
		// preview is <ul><li>one</li></ul>) — we must walk past
		// those nested closes. Track the open/close depth
		// starting from the row's <li> opening.
		liOpenIdx := strings.LastIndex(got[:insertIdx], "<li")
		if liOpenIdx < 0 {
			t.Errorf("row %q: could not find row's <li opening", r.Key)
			continue
		}
		// Walk forward from the row's <li opening, tracking
		// the open/close depth. Stop at depth 0 — that's the
		// matching </li> for the row.
		depth := 0
		liCloseIdx := -1
		cursor := liOpenIdx
		for cursor < len(got) {
			nextOpen := strings.Index(got[cursor:], "<li")
			nextClose := strings.Index(got[cursor:], "</li>")
			if nextClose < 0 {
				break
			}
			if nextOpen >= 0 && nextOpen < nextClose {
				depth++
				cursor += nextOpen + len("<li")
			} else {
				depth--
				cursor += nextClose + len("</li>")
				if depth == 0 {
					liCloseIdx = cursor
					break
				}
			}
		}
		if liCloseIdx < 0 {
			t.Errorf("row %q: could not locate matching </li> for row's <li", r.Key)
			continue
		}
		if copyIdx >= liCloseIdx {
			t.Errorf("row %q: Copy marker (idx=%d) lands AFTER the row's </li> (idx=%d) — Copy button is on a different row", r.Key, copyIdx, liCloseIdx)
		}
	}
}
