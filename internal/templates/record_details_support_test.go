package templates

// Tests for record_details_support.go (issue #541). The contract:
// single-URL Source Record details fields collapse to a
// "Click to view" anchor with trailing punctuation detached.
// Freeform text passes through unchanged.

import (
	"strings"
	"testing"
)

func TestRecordDetailsSegments_SingleURL(t *testing.T) {
	segs := recordDetailsSegments("https://www.fold3.com/page/document/hzp-123456789/")
	if len(segs) != 1 {
		t.Fatalf("want 1 segment, got %d", len(segs))
	}
	if !segs[0].IsURL {
		t.Fatalf("want IsURL=true, got false; seg=%+v", segs[0])
	}
	if segs[0].Href != "https://www.fold3.com/page/document/hzp-123456789/" {
		t.Fatalf("href not preserved: %q", segs[0].Href)
	}
	if segs[0].Suffix != "" {
		t.Fatalf("want empty suffix, got %q", segs[0].Suffix)
	}
}

func TestRecordDetailsSegments_TrailingPunctuationDetaches(t *testing.T) {
	cases := []struct {
		in       string
		wantHref string
		wantSuf  string
	}{
		{"https://example.com.", "https://example.com", "."},
		{"https://example.com,", "https://example.com", ","},
		{"https://example.com;", "https://example.com", ";"},
		{"https://example.com:", "https://example.com", ":"},
		{"https://example.com!", "https://example.com", "!"},
		{"https://example.com?", "https://example.com", "?"},
		{"https://example.com)", "https://example.com", ")"},
		{"https://example.com]", "https://example.com", "]"},
		{"https://example.com}", "https://example.com", "}"},
		// Multiple trailing chars: only ONE detaches (cap at one).
		// Remaining punct stays glued to the URL. This matches the
		// "Click to view." UX requirement — one trailing period.
		{"https://example.com..", "https://example.com.", "."},
		// Three trailing dots: only ONE detaches (cap at one char).
		// The remaining dots stay inside the href; the user's
		// expectation is "Click to view." not "(Click to view)...".
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			segs := recordDetailsSegments(tc.in)
			if len(segs) != 1 {
				t.Fatalf("want 1 segment, got %d", len(segs))
			}
			if segs[0].Href != tc.wantHref {
				t.Errorf("href: want %q, got %q", tc.wantHref, segs[0].Href)
			}
			if segs[0].Suffix != tc.wantSuf {
				t.Errorf("suffix: want %q, got %q", tc.wantSuf, segs[0].Suffix)
			}
			if !segs[0].IsURL {
				t.Error("want IsURL=true")
			}
		})
	}
}

func TestRecordDetailsSegments_FreeformTextPassesThrough(t *testing.T) {
	cases := []string{
		"Died of pneumonia, Rock Island Barracks, IL.",
		"see https://example.com for details", // URL mid-sentence → NOT a single-URL field
		"https:// example.com",                // space in URL → not a URL
		"fold3.com/page/123",                  // no scheme → not a URL
		"",
		"   ",
	}
	for _, tc := range cases {
		t.Run(tc, func(t *testing.T) {
			segs := recordDetailsSegments(tc)
			if len(segs) != 1 {
				t.Fatalf("want 1 segment, got %d", len(segs))
			}
			if segs[0].IsURL {
				t.Errorf("want IsURL=false for freeform text %q", tc)
			}
			if segs[0].Href != "" {
				t.Errorf("want empty Href, got %q", segs[0].Href)
			}
			if segs[0].Text != tc {
				t.Errorf("text passthrough: want %q, got %q", tc, segs[0].Text)
			}
		})
	}
}

func TestRecordDetailsSegments_OnlyHTTPSchemesAccepted(t *testing.T) {
	// javascript: / ftp: / file: must NOT be treated as URLs by this helper.
	// Even if they pass the regex, safeRecordDetailsHref rejects them.
	cases := []string{
		"javascript:alert(1)",
		"ftp://example.com/file",
		"file:///etc/passwd",
	}
	for _, tc := range cases {
		t.Run(tc, func(t *testing.T) {
			segs := recordDetailsSegments(tc)
			// Either: regex didn't match (IsURL false), OR safeRecordDetailsHref
			// will reject in the templ layer. The templ layer is the gate.
			// Here we only assert that if IsURL=true, the scheme is http/https.
			if segs[0].IsURL {
				if !strings.HasPrefix(segs[0].Href, "http://") && !strings.HasPrefix(segs[0].Href, "https://") {
					t.Errorf("non-http(s) scheme passed through: %q", segs[0].Href)
				}
			}
			// And the gate function rejects them:
			if safeRecordDetailsHref(tc) != "" {
				t.Errorf("safeRecordDetailsHref must reject %q", tc)
			}
		})
	}
}

func TestRecordDetailsSegments_URLTooLongRejected(t *testing.T) {
	long := "https://example.com/" + strings.Repeat("a", 4000)
	if isRecordDetailsURLAcceptable(long) {
		t.Error("URL over 4000 chars must be rejected by isRecordDetailsURLAcceptable")
	}
	// 4000 total chars exactly is the boundary — accepted.
	exact := "https://example.com/" + strings.Repeat("a", 4000-len("https://example.com/"))
	if !isRecordDetailsURLAcceptable(exact) {
		t.Error("URL exactly 4000 chars must be accepted")
	}
	// 4001 chars: rejected.
	over := "https://example.com/" + strings.Repeat("a", 4001-len("https://example.com/"))
	if isRecordDetailsURLAcceptable(over) {
		t.Error("URL over 4000 chars must be rejected")
	}
}

func TestRecordDetailsAnchorTextCanonical(t *testing.T) {
	// Pin the exact phrase. If this changes, the audit assertions
	// in audit/smoke_static_archive_revamp.mjs must move with it.
	if recordDetailsAnchorText != "Click to view" {
		t.Fatalf("anchor copy drifted: %q", recordDetailsAnchorText)
	}
}
