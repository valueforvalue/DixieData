package debug

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewRequestID_Shape(t *testing.T) {
	id := NewRequestID()
	if len(id) != 16 {
		t.Errorf("id length = %d, want 16: %q", len(id), id)
	}
	if !isHex(id) {
		t.Errorf("id not hex: %q", id)
	}
}

func TestMiddleware_GeneratesWhenAbsent(t *testing.T) {
	var seenID string
	h := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenID = RequestIDFromContext(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if len(seenID) != 16 {
		t.Errorf("ctx id length = %d, want 16: %q", len(seenID), seenID)
	}
	if rec.Header().Get(HeaderRequestID) != seenID {
		t.Errorf("response header %q != ctx id %q", rec.Header().Get(HeaderRequestID), seenID)
	}
}

func TestMiddleware_PreservesHeader(t *testing.T) {
	const preset = "deadbeef00000001"
	h := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := RequestIDFromContext(r.Context()); got != preset {
			t.Errorf("ctx id = %q, want %q", got, preset)
		}
	}))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(HeaderRequestID, preset)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get(HeaderRequestID) != preset {
		t.Errorf("response header = %q, want %q", rec.Header().Get(HeaderRequestID), preset)
	}
}

func TestMiddleware_RejectsBadHeader(t *testing.T) {
	h := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(HeaderRequestID, "short")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get(HeaderRequestID) == "short" {
		t.Errorf("bad header should be replaced, not preserved")
	}
	if len(rec.Header().Get(HeaderRequestID)) != 16 {
		t.Errorf("replacement id length = %d, want 16", len(rec.Header().Get(HeaderRequestID)))
	}
}