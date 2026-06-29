package appshell

import (
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// TestSanitiseToastForHeaderReplacements pins the substitution table
// from docs/adr/0005-toast-header-ascii-safe.md. Every entry must
// round-trip to its ASCII twin; the table is the single source of
// truth for what survives the Windows-1252 header channel.
func TestSanitiseToastForHeaderReplacements(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"ellipsis", "Importing 5 image(s)\u2026", "Importing 5 image(s)..."},
		{"em-dash", "No feedback yet \u2014 nothing to export.", "No feedback yet -- nothing to export."},
		{"en-dash", "high\u2013confidence", "high-confidence"},
		{"single-quote-curly", "don\u2019t", "don't"},
		{"double-quote-curly", "\u201chello\u201d", `"hello"`},
		{"nbsp", "before\u00a0after", "before after"},
		{"right-arrow", "page 1 \u2192 page 2", "page 1 -> page 2"},
		{"check-mark", "Done \u2713", "Done OK"},
		{"middle-dot", "a\u00b7b\u00b7c", "a*b*c"},
		{"section-omitted", "see \u00a7 3", "see  3"},
		{"ascii-only-passthrough", "Saved record for Jose", "Saved record for Jose"},
		{"user-name-accented-passthrough", "Saved record for Jos\u00e9", "Saved record for Jos\u00e9"},
		{"cjk-passthrough", "\u4fdd\u5b58\u5b8c\u6210", "\u4fdd\u5b58\u5b8c\u6210"},
		{"empty-passthrough", "", ""},
		{"multi-replacement-chained", "a\u2026b\u2014c\u2026d", "a...b--c...d"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitiseToastForHeader(tc.in)
			if got != tc.want {
				t.Errorf("sanitiseToastForHeader(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestSetToastHeaderAppliesSanitisation covers the wire contract:
// every byte in X-DixieData-Toast must be <= 0x7F after the helper
// runs, regardless of what Unicode the source contained. Chromium /
// WebView2 decode HTTP/1.x response headers as Windows-1252; any
// byte above 0x7F in the header value gets reinterpreted as a
// separate codepoint, producing mojibake on the rendered toast.
func TestSetToastHeaderAppliesSanitisation(t *testing.T) {
	cases := []struct {
		name        string
		message     string
		wantOnWire  string
		description string
	}{
		{
			name:        "ellipsis-to-wire",
			message:     "Shared archive import started\u2026",
			wantOnWire:  "Shared archive import started...",
			description: "real U+2026 in source must reach the wire as ASCII '...' so the toast renders correctly",
		},
		{
			name:        "em-dash-to-wire",
			message:     "Restoring backup: foo.ddbak \u2014 starting now",
			wantOnWire:  "Restoring backup: foo.ddbak -- starting now",
			description: "real U+2014 in source must reach the wire as ASCII '--' so the toast renders correctly",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			setInfoToastHeader(rec, tc.message)
			got := rec.Header().Get("X-DixieData-Toast")
			if got != tc.wantOnWire {
				t.Errorf("%s: got %q, want %q", tc.description, got, tc.wantOnWire)
			}
			// Defence-in-depth: assert no byte above 0x7F made
			// it through. Catches future regressions where a new
			// punctuation entry is added to source but the
			// table is forgotten.
			for i, b := range []byte(got) {
				if b > 0x7F {
					t.Errorf("byte at position %d is 0x%02X (> 0x7F); Chromium will mangle it: %q", i, b, got)
				}
			}
		})
	}
}

// TestToastHeaderSourceStillContainsUnicode pins the contract from
// ADR 0005: source code keeps the polished Unicode characters; only
// the wire value is ASCII. This is what makes the bug class from
// issue #135 impossible to reintroduce while preserving visual
// polish in the Go source.
func TestToastHeaderSourceStillContainsUnicode(t *testing.T) {
	// Source-level spot check: a representative in-progress toast
	// line in imports_handlers.go must still contain the real
	// U+2026 rune. If a future contributor "fixes" the mojibake
	// by stripping Unicode at the source, this test catches it
	// and forces them to update the table instead.
	data, err := readFileUTF8("imports_handlers.go")
	if err != nil {
		t.Fatalf("read imports_handlers.go: %v", err)
	}
	if !strings.Contains(data, "Shared archive import started\u2026") {
		t.Errorf("imports_handlers.go no longer contains the polished U+2026 ellipsis; if you stripped Unicode from the source, update toastHeaderASCIIReplacements instead")
	}
}

// readFileUTF8 is a tiny helper for the source-level scan above.
func readFileUTF8(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}