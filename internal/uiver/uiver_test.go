package uiver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestIsV2_Default verifies that a context that did not flow through
// Middleware reports v1 (false). This is the production path for every
// Wails request — the Wails runtime never sends ?ui=v2.
func TestIsV2_Default(t *testing.T) {
	if IsV2(context.Background()) {
		t.Fatal("background context should report v1 (false), got v2")
	}
	if IsV2(context.TODO()) {
		t.Fatal("TODO context should report v1 (false), got v2")
	}
}

// TestIsV2_Explicit verifies a context with v2=true reports v2.
func TestIsV2_Explicit(t *testing.T) {
	ctx := context.WithValue(context.Background(), Key{}, true)
	if !IsV2(ctx) {
		t.Fatal("v2=true context should report v2 (true), got v1")
	}
}

// TestMiddleware_NoQuery verifies that requests without ?ui=v2 stay on v1.
func TestMiddleware_NoQuery(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/soldiers", nil)
	var seenV2 bool
	Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenV2 = IsV2(r.Context())
	})).ServeHTTP(httptest.NewRecorder(), req)
	if seenV2 {
		t.Fatal("request without ?ui=v2 should report v1, got v2")
	}
}

// TestMiddleware_V2 verifies that ?ui=v2 flips the flag on.
func TestMiddleware_V2(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/soldiers?ui=v2", nil)
	var seenV2 bool
	Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenV2 = IsV2(r.Context())
	})).ServeHTTP(httptest.NewRecorder(), req)
	if !seenV2 {
		t.Fatal("?ui=v2 request should report v2, got v1")
	}
}

// TestMiddleware_OtherValues verifies that ?ui=anything-else stays v1.
// Only the literal "v2" flips the flag. v1, v3, beta, etc. all stay v1.
func TestMiddleware_OtherValues(t *testing.T) {
	for _, v := range []string{"v1", "V2", "v2x", "true", "1"} {
		t.Run(v, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/soldiers?ui="+v, nil)
			var seenV2 bool
			Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seenV2 = IsV2(r.Context())
			})).ServeHTTP(httptest.NewRecorder(), req)
			if seenV2 {
				t.Fatalf("?ui=%s should report v1, got v2", v)
			}
		})
	}
}