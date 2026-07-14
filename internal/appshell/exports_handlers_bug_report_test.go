package appshell

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// TestParseIncludeImagesFormValue pins slice 3 of issue #545:
// the bug-report bundle's "Include images" checkbox defaults to
// checked (real bytes), and unchecking it must flip to placeholder
// mode. The form-field contract is:
//
//   - POST with body `include_images=true`  -> includeImages = true
//   - POST with body `include_images=false` -> includeImages = false
//   - POST with empty body                  -> includeImages = true (default)
//   - POST with body `include_images=`      -> includeImages = false (unchecked)
//
// The default-on behavior is the legacy / backward-compat path:
// callers who haven't added the checkbox yet (older clients, the
// CLI smoke probe) keep getting real bytes bundled.
func TestParseIncludeImagesFormValue(t *testing.T) {
	cases := []struct {
		name           string
		body           string
		contentType    string
		wantIncludeImg bool
	}{
		{
			name:           "checked (include_images=true)",
			body:           "include_images=true",
			contentType:    "application/x-www-form-urlencoded",
			wantIncludeImg: true,
		},
		{
			name:           "unchecked (include_images=false)",
			body:           "include_images=false",
			contentType:    "application/x-www-form-urlencoded",
			wantIncludeImg: false,
		},
		{
			name:           "empty body defaults to include",
			body:           "",
			contentType:    "application/x-www-form-urlencoded",
			wantIncludeImg: true,
		},
		{
			name:           "empty value (checkbox unchecked, no value) defaults to skip",
			body:           "include_images=",
			contentType:    "application/x-www-form-urlencoded",
			wantIncludeImg: false,
		},
		{
			name:           "unknown value defaults to include",
			body:           "include_images=on",
			contentType:    "application/x-www-form-urlencoded",
			wantIncludeImg: true,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/export/bug-report", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.contentType)
			got := parseIncludeImagesFormValue(req)
			if got != tc.wantIncludeImg {
				t.Errorf("parseIncludeImagesFormValue() = %v; want %v", got, tc.wantIncludeImg)
			}
		})
	}
}

// TestParseIncludeImagesFormValue_FormEncodedURLValues pins the
// urlencoded-with-special-chars edge case: a form body with both
// the checkbox value and an unrelated sibling field still parses
// the checkbox correctly. Catches a future regression that
// re-implements the helper with a naive split-on-ampersand.
func TestParseIncludeImagesFormValue_FormEncodedURLValues(t *testing.T) {
	form := url.Values{}
	form.Set("include_images", "true")
	form.Set("support_endpoint", "https://support.example.invalid/upload")

	req := httptest.NewRequest(http.MethodPost, "/export/bug-report", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	if got := parseIncludeImagesFormValue(req); !got {
		t.Errorf("parseIncludeImagesFormValue with sibling fields = false; want true")
	}
}