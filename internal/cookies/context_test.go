package cookies

import (
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
)

// TestPersonCtxRoundTrip writes a person ID, then reads it back from
// a synthesized request carrying the Set-Cookie value. RED until
// WritePersonCtx + ReadPersonCtx exist.
func TestPersonCtxRoundTrip(t *testing.T) {
	key := mustKey(t)

	w := httptest.NewRecorder()
	if err := WritePersonCtx(w, key, 411); err != nil {
		t.Fatalf("WritePersonCtx: %v", err)
	}

	cookies := w.Result().Cookies()
	var cookie *http.Cookie
	for _, c := range cookies {
		if c.Name == CookieName {
			cookie = c
			break
		}
	}
	if cookie == nil {
		t.Fatalf("WritePersonCtx did not emit %q cookie; got %d cookies", CookieName, len(cookies))
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cookie)

	got, ok := ReadPersonCtx(req, key)
	if !ok {
		t.Fatalf("ReadPersonCtx returned ok=false on freshly-written cookie")
	}
	if got.PersonID != 411 {
		t.Fatalf("ReadPersonCtx PersonID = %d; want 411", got.PersonID)
	}
	if got.SetAt.IsZero() {
		t.Fatalf("ReadPersonCtx SetAt is zero; want non-zero timestamp")
	}
}

// TestPersonCtxRejectsTamperedValue flips a byte in the cookie value
// and asserts ReadPersonCtx returns ok=false. RED until HMAC verify exists.
func TestPersonCtxRejectsTamperedValue(t *testing.T) {
	key := mustKey(t)

	w := httptest.NewRecorder()
	if err := WritePersonCtx(w, key, 411); err != nil {
		t.Fatalf("WritePersonCtx: %v", err)
	}
	cookie := w.Result().Cookies()[0]

	// Flip the last character of the cookie value (inside the HMAC
	// suffix, since payload is short).
	original := cookie.Value
	if !strings.HasSuffix(original, "AA") {
		// Ensure suffix is deterministic; if random base64 ends in
		// something else, just flip a middle byte.
		cookie.Value = original[:len(original)-1] + "X"
	} else {
		cookie.Value = original[:len(original)-1] + "B"
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cookie)

	if _, ok := ReadPersonCtx(req, key); ok {
		t.Fatalf("ReadPersonCtx returned ok=true on tampered cookie; expected verify failure")
	}
}

// TestPersonCtxMissingCookieReturnsFalse asserts a request without
// the cookie yields ok=false (not a panic, not a zero-value PersonCtx).
func TestPersonCtxMissingCookieReturnsFalse(t *testing.T) {
	key := mustKey(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if _, ok := ReadPersonCtx(req, key); ok {
		t.Fatalf("ReadPersonCtx returned ok=true on request with no cookie")
	}
}

// TestPersonCtxMalformedCookieReturnsFalse asserts a garbage cookie
// value (not even base64) does not panic and yields ok=false.
func TestPersonCtxMalformedCookieReturnsFalse(t *testing.T) {
	key := mustKey(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: "!!!not-base64!!!"})
	if _, ok := ReadPersonCtx(req, key); ok {
		t.Fatalf("ReadPersonCtx returned ok=true on malformed cookie")
	}
}

// TestPersonCtxClearEmitsMaxAgeMinusOne asserts ClearPersonCtx writes
// a cookie with MaxAge=-1 so the browser drops it.
func TestPersonCtxClearEmitsMaxAgeMinusOne(t *testing.T) {
	w := httptest.NewRecorder()
	ClearPersonCtx(w)

	var cookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == CookieName {
			cookie = c
			break
		}
	}
	if cookie == nil {
		t.Fatalf("ClearPersonCtx did not emit %q cookie", CookieName)
	}
	if cookie.MaxAge >= 0 {
		t.Fatalf("ClearPersonCtx cookie MaxAge = %d; want < 0", cookie.MaxAge)
	}
	if cookie.Value != "" {
		t.Fatalf("ClearPersonCtx cookie Value = %q; want empty", cookie.Value)
	}
}

// TestPersonCtxCookieAttrs pins the public cookie surface (HttpOnly,
// SameSite, Path, MaxAge) so a future refactor cannot silently weaken
// them. Per #378 the picker reads dd_person_ctx; the cookie must
// stay JS-inaccessible and Lax-scoped to survive cross-site links.
func TestPersonCtxCookieAttrs(t *testing.T) {
	key := mustKey(t)
	w := httptest.NewRecorder()
	if err := WritePersonCtx(w, key, 411); err != nil {
		t.Fatalf("WritePersonCtx: %v", err)
	}
	cookie := w.Result().Cookies()[0]
	if !cookie.HttpOnly {
		t.Fatalf("cookie HttpOnly = false; want true")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie SameSite = %v; want Lax", cookie.SameSite)
	}
	if cookie.Path != "/" {
		t.Fatalf("cookie Path = %q; want /", cookie.Path)
	}
	if cookie.MaxAge != 30*24*3600 {
		t.Fatalf("cookie MaxAge = %d; want 30 days (%d)", cookie.MaxAge, 30*24*3600)
	}
}

// TestPersonCtxSecureOnlyOnNonLoopback asserts Secure flag is true
// when the request host is a non-loopback hostname, false on
// loopback. Belt-and-braces: dev builds on localhost stay usable,
// production hosts always upgrade.
func TestPersonCtxSecureOnlyOnNonLoopback(t *testing.T) {
	key := mustKey(t)

	cases := []struct {
		host string
		want bool
	}{
		{"localhost", false},
		{"127.0.0.1", false},
		{"[::1]", false},
		{"dixiedata.example.com", true},
	}
	for _, tc := range cases {
		t.Run(tc.host, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://"+tc.host+"/", nil)
			w := httptest.NewRecorder()
			if err := WritePersonCtxFromRequest(w, key, req, 411); err != nil {
				t.Fatalf("WritePersonCtxFromRequest: %v", err)
			}
			cookie := w.Result().Cookies()[0]
			if cookie.Secure != tc.want {
				t.Fatalf("host %q: cookie.Secure = %v; want %v", tc.host, cookie.Secure, tc.want)
			}
		})
	}
}

// TestEnsureKeyAutoGeneratesWhenMissing asserts EnsureKey writes a
// fresh 32-byte key file when one is absent. RED until bootstrap exists.
// The dir argument represents the cookies sibling directory
// (appdata.CookiesRoot) — never a child of the data dir, or .ddbak
// restore would hit Windows file-lock conflicts when renaming
// the data dir.
func TestEnsureKeyAutoGeneratesWhenMissing(t *testing.T) {
	dir := t.TempDir()
	keyPath := KeyPath(dir)

	key1, err := EnsureKey(dir)
	if err != nil {
		t.Fatalf("EnsureKey first call: %v", err)
	}
	if len(key1) != 32 {
		t.Fatalf("EnsureKey returned %d bytes; want 32", len(key1))
	}

	// Second call should read the existing key file (deterministic).
	key2, err := EnsureKey(dir)
	if err != nil {
		t.Fatalf("EnsureKey second call: %v", err)
	}
	if string(key1) != string(key2) {
		t.Fatalf("EnsureKey not deterministic across calls; key rotated")
	}

	// File must exist on disk with 0600 perms (on unix; Windows
	// ACLs differ and Go's os.Stat reports 0666 for files written
	// with 0600 requested — the platform doesn't honor the bitmask).
	if runtime.GOOS != "windows" {
		info, err := keyPathInfo(keyPath)
		if err != nil {
			t.Fatalf("key file stat: %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Fatalf("key file perm = %o; want 0600", perm)
		}
	}
}

// TestEnsureKeyRejectsShortKeyFile asserts a key file shorter than 32
// bytes is rejected (so a partial-write never silently downgrades
// security).
func TestEnsureKeyRejectsShortKeyFile(t *testing.T) {
	dir := t.TempDir()
	if err := writeKeyFile(dir, make([]byte, 16)); err != nil {
		t.Fatalf("writeKeyFile: %v", err)
	}
	if _, err := EnsureKey(dir); err == nil {
		t.Fatalf("EnsureKey accepted 16-byte key; want error")
	}
}

// mustKey returns a deterministic 32-byte key for tests that don't
// need bootstrap behavior. Uses a fixed seed so tests don't depend
// on the host filesystem.
func mustKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	return key
}