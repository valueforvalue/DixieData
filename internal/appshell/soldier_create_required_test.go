package appshell

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// TestHandleCreateSoldier_RequiresName pins the server-side guard
// added by the manual UI audit on 2026-06-29. Before the guard, the
// handler accepted records with all-empty name fields, leaving
// anonymous entries in the archive. The form's `required` attribute
// covers the common path via the browser tooltip; this test catches
// the bypass.
func TestHandleCreateSoldier_RequiresName(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	// POST /soldiers with all-empty name fields. Expect 400.
	form := url.Values{}
	form.Set("display_id", "DXD-EMPTY")
	form.Set("entry_type", "soldier")
	// first_name, middle_name, last_name all empty
	resp, err := http.PostForm(server.URL+"/soldiers", form)
	if err != nil {
		t.Fatalf("POST /soldiers: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("POST /soldiers with empty name: got status %d, want 400", resp.StatusCode)
	}

	// POST /soldiers with only first_name. Expect 200 + X-DixieData-Redirect
	// (Option C dispatcher contract; the browser reads the header and
	// window.location.assign's to the target). Unique display_id per
	// case so the soldier Create doesn't collide.
	form.Set("display_id", "DXD-FIRST")
	form.Set("first_name", "TestFirst")
	resp, err = http.PostForm(server.URL+"/soldiers", form)
	if err != nil {
		t.Fatalf("POST /soldiers (first_name only): %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body := make([]byte, 2048)
		n, _ := resp.Body.Read(body)
		t.Errorf("POST /soldiers (first_name only): got status %d, want 200; body: %s", resp.StatusCode, string(body[:n]))
	}
	redirect := resp.Header.Get("X-DixieData-Redirect")
	if !strings.HasPrefix(redirect, "/soldiers/") {
		t.Errorf("POST /soldiers (first_name only): X-DixieData-Redirect = %q, want /soldiers/{id}", redirect)
	}

	// POST /soldiers with only last_name. Expect 200 + X-DixieData-Redirect.
	form.Set("display_id", fmt.Sprintf("DXD-LAST-%d", 1))
	form.Set("first_name", "")
	form.Set("last_name", "TestLast")
	resp, err = http.PostForm(server.URL+"/soldiers", form)
	if err != nil {
		t.Fatalf("POST /soldiers (last_name only): %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("POST /soldiers (last_name only): got status %d, want 200", resp.StatusCode)
	}
	redirect = resp.Header.Get("X-DixieData-Redirect")
	if !strings.HasPrefix(redirect, "/soldiers/") {
		t.Errorf("POST /soldiers (last_name only): X-DixieData-Redirect = %q, want /soldiers/{id}", redirect)
	}
}
