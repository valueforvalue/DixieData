package appshell

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestBlockIfFragment pins the contract for blockIfFragment, the
// shared helper that enforces the HX-Request fragment-204 contract
// across all "blocked" branches in App.ServeHTTP. See issue #215.
//
// The helper is the single source of truth for the contract; the
// integration tests (TestAppServeHTTPMuxNotReadyFragment204PlaceholderFullContract,
// TestAppServeHTTPSetupRequiredFragmentReturns204WithRedirectHint,
// TestAppServeHTTPRecoveryFragmentReturns204WithRedirectHint,
// TestAppServeHTTPStartupErrFragmentReturns204WithRedirectHint)
// prove the wiring at each call site.
func TestBlockIfFragment(t *testing.T) {
	tests := []struct {
		name         string
		hxRequest    string // value to set in the HX-Request header; "" = don't set
		redirectTo   string
		nilRequest   bool
		wantReturned bool
		wantStatus   int
		wantRedirect string // expected X-DixieData-Redirect header value
	}{
		{
			name:         "fragment with hint returns 204 + header",
			hxRequest:    "true",
			redirectTo:   "/setup",
			wantReturned: true,
			wantStatus:   http.StatusNoContent,
			wantRedirect: "/setup",
		},
		{
			name:         "fragment with empty hint returns 204 + no header",
			hxRequest:    "true",
			redirectTo:   "",
			wantReturned: true,
			wantStatus:   http.StatusNoContent,
			wantRedirect: "",
		},
		{
			name:         "no HX-Request header does nothing",
			hxRequest:    "",
			redirectTo:   "/setup",
			wantReturned: false,
		},
		{
			name:         "HX-Request: false does nothing",
			hxRequest:    "false",
			redirectTo:   "/setup",
			wantReturned: false,
		},
		{
			name:         "nil request does nothing",
			redirectTo:   "/setup",
			nilRequest:   true,
			wantReturned: false,
		},
		{
			name:         "empty HX-Request value does nothing",
			hxRequest:    "",
			redirectTo:   "/setup",
			wantReturned: false,
		},
		{
			name:         "caller pre-sets redirect header; helper overwrites",
			hxRequest:    "true",
			redirectTo:   "/setup",
			wantReturned: true,
			wantStatus:   http.StatusNoContent,
			wantRedirect: "/setup",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			var r *http.Request
			if !tt.nilRequest {
				r = httptest.NewRequest(http.MethodGet, "/some/path", nil)
				if tt.hxRequest != "" {
					r.Header.Set("HX-Request", tt.hxRequest)
				}
			}
			// Pre-set the header for case 7 to verify the helper
			// is the source of truth.
			if tt.name == "caller pre-sets redirect header; helper overwrites" {
				rec.Header().Set("X-DixieData-Redirect", "/other")
			}

			got := blockIfFragment(rec, r, tt.redirectTo)
			if got != tt.wantReturned {
				t.Fatalf("blockIfFragment returned %v, want %v", got, tt.wantReturned)
			}

			if !tt.wantReturned {
				// No state change expected.
				if rec.Code != 200 { // httptest.NewRecorder default
					t.Fatalf("recorder.Code = %d, want default 200 (no WriteHeader call)", rec.Code)
				}
				if got := rec.Header().Get("X-DixieData-Redirect"); got != "" {
					t.Fatalf("recorder X-DixieData-Redirect = %q, want empty", got)
				}
				return
			}

			if rec.Code != tt.wantStatus {
				t.Fatalf("recorder.Code = %d, want %d", rec.Code, tt.wantStatus)
			}
			if got := rec.Header().Get("X-DixieData-Redirect"); got != tt.wantRedirect {
				t.Fatalf("recorder X-DixieData-Redirect = %q, want %q", got, tt.wantRedirect)
			}
			if rec.Body.Len() != 0 {
				t.Fatalf("recorder body must be empty for 204; got %d bytes: %q", rec.Body.Len(), rec.Body.String())
			}
		})
	}
}