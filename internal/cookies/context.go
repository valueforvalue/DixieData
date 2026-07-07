// Package cookies provides signed-cookie helpers for cross-request
// client state. The only consumer today is the Research & Review
// Person picker (issue #378); the package is intentionally narrow
// so future helpers (each with their own cookie name and payload
// shape) don't grow implicit coupling through shared state.
package cookies

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// CookieName is the public cookie surface for the Research & Review
// person context. Exported so test + handler code can reference a
// single source of truth.
const CookieName = "dd_person_ctx"

// PersonCtx is the decoded cookie payload. Only fields the picker
// needs are exposed; internal HMAC signing details stay private.
type PersonCtx struct {
	PersonID int64
	SetAt    time.Time
}

// CookieMaxAge is the browser-side lifetime. 30 days keeps the
// picker "continue" shortcut warm across sessions without persisting
// forever.
const CookieMaxAge = 30 * 24 * 3600

// KeyFileName is the on-disk key filename inside the DixieData
// cookies directory (sibling of the data dir, see appdata.CookiesRoot).
const KeyFileName = "person_ctx.key"

// KeyPath returns the absolute path to the cookie key file inside
// the supplied cookies directory. Callers pass
// appdata.CookiesDir(appdata.DefaultDir()).
func KeyPath(cookiesDir string) string {
	return filepath.Join(cookiesDir, KeyFileName)
}

// WritePersonCtx writes a signed dd_person_ctx cookie carrying the
// supplied person ID. Secure flag is disabled (loopback builds stay
// usable without TLS). Use WritePersonCtxFromRequest when the request
// is in scope and Secure-on-non-loopback matters.
func WritePersonCtx(w http.ResponseWriter, key []byte, personID int64) error {
	return writePersonCtx(w, key, personID, false)
}

// WritePersonCtxFromRequest writes a signed dd_person_ctx cookie with
// Secure=true when the request host is not loopback. Used by the
// picker handler so production deployments upgrade transparently.
func WritePersonCtxFromRequest(w http.ResponseWriter, key []byte, r *http.Request, personID int64) error {
	return writePersonCtx(w, key, personID, !isLoopbackHost(r))
}

func writePersonCtx(w http.ResponseWriter, key []byte, personID int64, secure bool) error {
	if len(key) != 32 {
		return fmt.Errorf("cookies: HMAC key must be 32 bytes, got %d", len(key))
	}
	if personID <= 0 {
		return fmt.Errorf("cookies: personID must be positive, got %d", personID)
	}

	setAt := time.Now().UTC()
	value, err := encode(key, personID, setAt)
	if err != nil {
		return fmt.Errorf("cookies: encode: %w", err)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   CookieMaxAge,
	})
	return nil
}

// ReadPersonCtx parses and verifies the dd_person_ctx cookie on r.
// Returns ok=false when the cookie is missing, malformed, tampered,
// or signed with a different key. Callers should treat false as
// "no context" and route the user through the picker.
func ReadPersonCtx(r *http.Request, key []byte) (PersonCtx, bool) {
	if len(key) != 32 {
		return PersonCtx{}, false
	}
	c, err := r.Cookie(CookieName)
	if err != nil {
		return PersonCtx{}, false
	}
	personID, setAt, ok := decode(key, c.Value)
	if !ok {
		return PersonCtx{}, false
	}
	return PersonCtx{PersonID: personID, SetAt: setAt}, true
}

// ClearPersonCtx writes a dd_person_ctx cookie with MaxAge=-1 so the
// browser drops it. The picker clears on user "Change Person" action.
func ClearPersonCtx(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// Encode payload: base64( personID(8 bytes BE) || setAtUnix(8 bytes BE) || hmac(32 bytes) ).
// HMAC input is the 16-byte payload; the cookie wire format is base64
// of payload+mac. No plaintext ID is exposed.
func encode(key []byte, personID int64, setAt time.Time) (string, error) {
	if len(key) != 32 {
		return "", errors.New("cookies: HMAC key must be 32 bytes")
	}
	var payload [16]byte
	binary.BigEndian.PutUint64(payload[0:8], uint64(personID))
	binary.BigEndian.PutUint64(payload[8:16], uint64(setAt.Unix()))

	mac := hmac.New(sha256.New, key)
	if _, err := mac.Write(payload[:]); err != nil {
		return "", err
	}
	sum := mac.Sum(nil)

	combined := make([]byte, 0, len(payload)+len(sum))
	combined = append(combined, payload[:]...)
	combined = append(combined, sum...)

	return base64.RawURLEncoding.EncodeToString(combined), nil
}

func decode(key []byte, wire string) (int64, time.Time, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(wire)
	if err != nil {
		return 0, time.Time{}, false
	}
	if len(raw) != 16+32 {
		return 0, time.Time{}, false
	}
	mac := hmac.New(sha256.New, key)
	if _, err := mac.Write(raw[:16]); err != nil {
		return 0, time.Time{}, false
	}
	expected := mac.Sum(nil)
	if !hmac.Equal(expected, raw[16:]) {
		return 0, time.Time{}, false
	}
	personID := int64(binary.BigEndian.Uint64(raw[0:8]))
	setAtUnix := int64(binary.BigEndian.Uint64(raw[8:16]))
	return personID, time.Unix(setAtUnix, 0).UTC(), true
}

// isLoopbackHost returns true when r.Host is a loopback address
// (localhost, 127.0.0.1, [::1]). Production deployments always
// upgrade to Secure cookies; loopback stays usable without TLS.
//
// Strips the optional :port suffix using net.SplitHostPort which
// handles bracketed IPv6 correctly (avoids the colon-in-IPv6
// ambiguity that bites naive indexOf logic). Also strips the
// IPv6 brackets so net.ParseIP can parse the bare address.
func isLoopbackHost(r *http.Request) bool {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if len(host) >= 2 && host[0] == '[' && host[len(host)-1] == ']' {
		host = host[1 : len(host)-1]
	}
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// EnsureKey returns the HMAC key for the cookie, creating a fresh
// 32-byte random key file inside cookiesDir if none exists. Returns
// the same key on subsequent calls (deterministic across restarts).
// A key file shorter than 32 bytes is treated as corrupt and
// rejected (security: a partial-write must not silently downgrade).
//
// cookiesDir MUST be a sibling of the data directory
// (appdata.CookiesRoot), never a child — see the rationale in
// appdata.CookiesRoot about .ddbak restore renaming dataDir.
func EnsureKey(cookiesDir string) ([]byte, error) {
	keyPath := KeyPath(cookiesDir)

	existing, err := os.ReadFile(keyPath)
	if errors.Is(err, os.ErrNotExist) {
		return generateAndPersistKey(keyPath)
	}
	if err != nil {
		return nil, fmt.Errorf("cookies: read key file: %w", err)
	}
	if len(existing) != 32 {
		return nil, fmt.Errorf("cookies: key file at %s is %d bytes; want 32 (corrupt?)", keyPath, len(existing))
	}
	return existing, nil
}

func generateAndPersistKey(keyPath string) ([]byte, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(randReader, key); err != nil {
		return nil, fmt.Errorf("cookies: read random key: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
		return nil, fmt.Errorf("cookies: mkdir: %w", err)
	}
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		return nil, fmt.Errorf("cookies: write key file: %w", err)
	}
	return key, nil
}

// writeKeyFile is a test-only helper that bypasses MkdirAll so
// tests can pin corruption scenarios.
func writeKeyFile(dir string, key []byte) error {
	return os.WriteFile(KeyPath(dir), key, 0o600)
}

// keyPathInfo is a test-only helper.
func keyPathInfo(path string) (os.FileInfo, error) {
	return os.Stat(path)
}

// DecodePersonIDFromString parses a request body field. Used by
// the picker handler (POST /research/select) to validate person_id
// before writing the cookie. Returns 0 + error on bad input.
func DecodePersonIDFromString(raw string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("cookies: parse person id %q: %w", raw, err)
	}
	if id <= 0 {
		return 0, fmt.Errorf("cookies: person id must be positive, got %d", id)
	}
	return id, nil
}