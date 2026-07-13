package templates

// record_details_support.go provides the linkifier used by Source
// Record details on live surfaces (issue #541). It mirrors the
// behavior of templates/common/record_card.typ::render-link and
// internal/archive/static_archive.go::printRenderLink: external
// URLs become a "Click to view" anchor with the URL hidden;
// non-URL text passes through unchanged; trailing punctuation
// detaches so "see https://example.com." does not render the
// period inside the anchor.
//
// The contract is the same on every surface that calls this
// helper. Keep the four rules below aligned with the JS twin in
// static_archive.go and the Typst twin in record_card.typ.

import (
	"net/url"
	"regexp"
	"strings"
)

// recordDetailsPattern matches the same URL shape as
// linkedTextPattern, but only on lines that look like a bare URL
// (after trimming). Keeping it local to this file avoids
// accidentally broadening LinkedText's matcher.
var recordDetailsPattern = regexp.MustCompile(`^\s*(https?://[^\s<]+)\s*$`)

// trailingPunct is the set of characters detached from the end
// of a URL anchor so "https://example.com." renders the period
// outside the <a>. Same set as renderLinkedText in
// static_archive.go (line 1735).
const recordDetailsTrailingPunct = ".,;:!?)]}"

// recordDetailsAnchorText is the single canonical phrase used by
// every live surface that wraps a Source Record URL. Keep in
// sync with record_card.typ::render-link and the ::after hint in
// static_archive.go.
const recordDetailsAnchorText = "Click to view"

// recordDetailsSegment mirrors textSegment in linked_text_support.go
// but is scoped to the Source Record details contract: when the
// field is a single URL, IsURL is true and Href/Text hold the
// cleaned URL; otherwise the field is rendered as plain text.
type recordDetailsSegment struct {
	Text     string
	Href     string
	IsURL    bool
	Suffix   string // trailing punctuation detached from the URL
}

// recordDetailsSegments splits a Source Record `details` field
// into render segments. Single-URL fields collapse to one
// IsURL=true segment with the URL cleaned and trailing punct
// detached. Freeform text passes through as one segment.
func recordDetailsSegments(text string) []recordDetailsSegment {
	if strings.TrimSpace(text) == "" {
		return []recordDetailsSegment{{Text: text}}
	}
	m := recordDetailsPattern.FindStringSubmatch(text)
	if m == nil {
		return []recordDetailsSegment{{Text: text}}
	}
	raw := strings.TrimSpace(m[1])
	cleanURL, suffix := splitRecordDetailsURL(raw)
	if cleanURL == "" {
		return []recordDetailsSegment{{Text: text}}
	}
	return []recordDetailsSegment{{Href: cleanURL, IsURL: true, Suffix: suffix}}
}

// splitRecordDetailsURL returns the URL with at most one trailing
// punctuation character stripped and the detached character as a
// separate string. Mirrors the spirit of splitURLSuffix in
// linked_text_support.go but caps the strip at one character so
// "https://example.com)" inside "(...)" yields Href="https://example.com"
// and Suffix=")", not an empty Href. The full TrimRight used by
// the linked_text helper is too aggressive when the field is a
// single-URL field whose trailing character is a closing bracket
// belonging to the surrounding sentence.
func splitRecordDetailsURL(value string) (string, string) {
	if value == "" {
		return "", ""
	}
	last := rune(value[len(value)-1])
	if !strings.ContainsRune(recordDetailsTrailingPunct, last) {
		return value, ""
	}
	return value[:len(value)-1], string(last)
}

// isRecordDetailsURLAcceptable guards against absurdly long URLs
// the same way render-link does in Typst. URLs over 4000 chars
// fall through to plain text on every surface.
func isRecordDetailsURLAcceptable(u string) bool {
	return u != "" && len(u) <= 4000
}

// safeRecordDetailsHref runs url.Parse as a sanity check before
// the templ layer renders the href attribute. Empty / unparsable
// values are treated as "not a URL" by the caller.
func safeRecordDetailsHref(u string) string {
	parsed, err := url.Parse(u)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	return u
}
