package debug

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
)

// HeaderRequestID is the HTTP header used for inbound + outbound
// request-id propagation.
const HeaderRequestID = "X-Request-Id"

// ctxKey is unexported so callers can't collide with our context keys.
type ctxKey int

const ctxKeyRequestID ctxKey = iota

// NewRequestID returns a fresh 16-character hex string (8 random bytes).
func NewRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "0000000000000000"
	}
	return hex.EncodeToString(b[:])
}

// WithRequestID returns a child context carrying the given request id.
func WithRequestID(ctx context.Context, id string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, ctxKeyRequestID, id)
}

// RequestIDFromContext returns the request id stored on the context,
// or "" if none.
func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(ctxKeyRequestID).(string)
	return v
}

// Middleware generates (or preserves) a request id for every inbound
// request, stores it on the context, and echoes it in the response
// header. MUST be the outermost middleware so all downstream handlers
// + the recover middleware can see the id.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get(HeaderRequestID))
		if len(id) != 16 || !isHex(id) {
			id = NewRequestID()
		}
		w.Header().Set(HeaderRequestID, id)
		ctx := WithRequestID(r.Context(), id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func isHex(s string) bool {
	if len(s) != 16 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		case c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}