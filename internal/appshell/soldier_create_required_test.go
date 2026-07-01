package appshell

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

// TestHandleCreateSoldier_EmptyNameMarksForReview (issue #151)
// flips the server-side guard added in PR #149 from a hard 400 to
// a soft confirm-and-mark-for-review path. The browser tooltip via
// `required` is dropped (entry_form.templ); the JS interceptor in
// frontend/app.js surfaces a confirm() and, on accept, appends the
// confirm_empty_name=1 marker. The server-side handler tests:
//   - Empty names + confirm marker → 200 + NeedsReview=true
//   - First-name only               → 200 + NeedsReview=false
//   - Last-name only                → 200 + NeedsReview=false
//   - Empty names + no marker       → 400 (catches the bypass)
func TestHandleCreateSoldier_EmptyNameMarksForReview(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	// Case 1: Empty names + confirm marker.
	t.Run("empty_with_confirm_marks_review", func(t *testing.T) {
		form := url.Values{}
		form.Set("display_id", fmt.Sprintf("DXD-EMPTY-CONFIRM-%d", counter(t)))
		form.Set("entry_type", "soldier")
		form.Set("confirm_empty_name", "1")
		resp, err := http.PostForm(server.URL+"/soldiers", form)
		if err != nil {
			t.Fatalf("POST /soldiers (empty + confirm): %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("POST /soldiers (empty + confirm) status %d, want 200", resp.StatusCode)
		}
		if got := resp.Header.Get("X-DixieData-Redirect"); !strings.HasPrefix(got, "/soldiers/") {
			t.Errorf("X-DixieData-Redirect = %q, want /soldiers/{id}", got)
		}
	})

	// Case 2: First-name only.
	t.Run("first_name_only_not_marked", func(t *testing.T) {
		form := url.Values{}
		form.Set("display_id", fmt.Sprintf("DXD-FIRST-%d", counter(t)))
		form.Set("entry_type", "soldier")
		form.Set("first_name", "TestFirst")
		resp, err := http.PostForm(server.URL+"/soldiers", form)
		if err != nil {
			t.Fatalf("POST /soldiers (first only): %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("POST /soldiers (first only) status %d, want 200", resp.StatusCode)
		}
	})

	// Case 3: Last-name only.
	t.Run("last_name_only_not_marked", func(t *testing.T) {
		form := url.Values{}
		form.Set("display_id", fmt.Sprintf("DXD-LAST-%d", counter(t)))
		form.Set("entry_type", "soldier")
		form.Set("last_name", "TestLast")
		resp, err := http.PostForm(server.URL+"/soldiers", form)
		if err != nil {
			t.Fatalf("POST /soldiers (last only): %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("POST /soldiers (last only) status %d, want 200", resp.StatusCode)
		}
	})

	// Case 4: Empty names + no marker → 400.
	t.Run("empty_without_confirm_returns_400", func(t *testing.T) {
		form := url.Values{}
		form.Set("display_id", fmt.Sprintf("DXD-EMPTY-BARE-%d", counter(t)))
		form.Set("entry_type", "soldier")
		resp, err := http.PostForm(server.URL+"/soldiers", form)
		if err != nil {
			t.Fatalf("POST /soldiers (empty bare): %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("POST /soldiers (empty bare) status %d, want 400", resp.StatusCode)
		}
	})
}

func counter(t *testing.T) int64 {
	t.Helper()
	return atomic.AddInt64(createCounter, 1)
}

var createCounter = new(int64)
