package supportuploader

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// formsparkTestEntry builds a representative feedback entry
// the Formspark helper accepts. Mirrors the appshell.feedbackEntry
// struct shape (the helper takes any JSON-marshalable value).
func formsparkTestEntry() map[string]any {
	return map[string]any{
		"submitted_at":   "2026-07-14T16:00:00Z",
		"page_path":      "/calendar",
		"category":       "bug",
		"contact_name":   "Tester",
		"contact_email":  "tester@example.invalid",
		"message":        "Sample feedback message body.",
		"app_version":    "v1.2.7",
		"build_identity": "dev-21510cf",
		"schema_version": 60,
	}
}

// TestUploadFeedbackFormspark_HappyPath_PostsJSON pins the
// success contract (issue #566 locked decision 4): the server
// returns 200 with a JSON body, and the helper returns nil
// without parsing any ticket id. The request body must be
// JSON, not multipart, and must carry the flattened Formspark
// field set with the synthesised subject line.
func TestUploadFeedbackFormspark_HappyPath_PostsJSON(t *testing.T) {
	var serverSaw struct {
		Method      string
		ContentType string
		Accept      string
		Body        []byte
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverSaw.Method = r.Method
		serverSaw.ContentType = r.Header.Get("Content-Type")
		serverSaw.Accept = r.Header.Get("Accept")
		serverSaw.Body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"subject":"received"}`))
	}))
	defer srv.Close()

	err := UploadFeedbackFormspark(context.Background(), formsparkTestEntry(), srv.URL)
	if err != nil {
		t.Fatalf("UploadFeedbackFormspark: %v", err)
	}
	if serverSaw.Method != "POST" {
		t.Errorf("server saw method = %q; want POST", serverSaw.Method)
	}
	if !strings.HasPrefix(serverSaw.ContentType, "application/json") {
		t.Errorf("request Content-Type = %q; want application/json", serverSaw.ContentType)
	}
	if !strings.HasPrefix(serverSaw.Accept, "application/json") {
		t.Errorf("request Accept = %q; want application/json", serverSaw.Accept)
	}

	var parsed map[string]any
	if err := json.Unmarshal(serverSaw.Body, &parsed); err != nil {
		t.Fatalf("request body is not JSON: %v (raw: %q)", err, string(serverSaw.Body))
	}
	// Subject must be synthesised server-side from category +
	// page_path + contact_email so Formspark's email notification
	// has a useful title (locked decision 3).
	if subj, _ := parsed["subject"].(string); !strings.Contains(subj, "bug") || !strings.Contains(subj, "/calendar") {
		t.Errorf("subject = %q; want a string containing 'bug' and '/calendar'", subj)
	}
	// The flattened Formspark field set.
	if parsed["message"] != "Sample feedback message body." {
		t.Errorf("message = %v; want verbatim body", parsed["message"])
	}
	if parsed["page_path"] != "/calendar" {
		t.Errorf("page_path = %v; want /calendar", parsed["page_path"])
	}
	if parsed["contact_email"] != "tester@example.invalid" {
		t.Errorf("contact_email = %v; want tester@example.invalid", parsed["contact_email"])
	}
	if parsed["contact_name"] != "Tester" {
		t.Errorf("contact_name = %v; want Tester", parsed["contact_name"])
	}
	if parsed["category"] != "bug" {
		t.Errorf("category = %v; want bug", parsed["category"])
	}
	if parsed["app_version"] != "v1.2.7" {
		t.Errorf("app_version = %v; want v1.2.7", parsed["app_version"])
	}
	if parsed["build_identity"] != "dev-21510cf" {
		t.Errorf("build_identity = %v; want dev-21510cf", parsed["build_identity"])
	}
	// schema_version is a JSON number; the helper must serialise
	// it as a number, not a string, so the Formspark dashboard
	// sorts and filters correctly.
	if _, ok := parsed["schema_version"].(float64); !ok {
		t.Errorf("schema_version = %T; want JSON number (float64)", parsed["schema_version"])
	}
}

// TestUploadFeedbackFormspark_AnonymousContact pins the
// "contact_email empty" branch of the subject synthesis:
// the subject must still be non-empty so the email
// notification has a title.
func TestUploadFeedbackFormspark_AnonymousContact(t *testing.T) {
	var serverSaw []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverSaw, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":1}`))
	}))
	defer srv.Close()

	entry := formsparkTestEntry()
	entry["contact_email"] = ""
	entry["contact_name"] = ""
	if err := UploadFeedbackFormspark(context.Background(), entry, srv.URL); err != nil {
		t.Fatalf("UploadFeedbackFormspark: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(serverSaw, &parsed); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if subj, _ := parsed["subject"].(string); subj == "" {
		t.Errorf("subject is empty; want a synthesised title even when contact is anonymous")
	}
	if !strings.Contains(parsed["subject"].(string), "anonymous") {
		t.Errorf("subject = %q; want 'anonymous' marker when no contact is supplied", parsed["subject"])
	}
}

// TestUploadFeedbackFormspark_4xx_SurfacesStatusAndBody pins
// the 4xx error path (locked decision 4): the server's 4xx
// response surfaces as an error that includes the status code
// and a body snippet. The caller uses this to render a toast.
func TestUploadFeedbackFormspark_4xx_SurfacesStatusAndBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "form disabled by owner", http.StatusForbidden)
	}))
	defer srv.Close()

	err := UploadFeedbackFormspark(context.Background(), formsparkTestEntry(), srv.URL)
	if err == nil {
		t.Fatal("UploadFeedbackFormspark: expected error on 403, got nil")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error %q does not mention status 403", err.Error())
	}
	if !strings.Contains(err.Error(), "form disabled") {
		t.Errorf("error %q does not include the response body snippet", err.Error())
	}
}

// TestUploadFeedbackFormspark_5xx_SurfacesStatusAndBody pins
// the 5xx error path. A 5xx is the server-side error class;
// the caller surfaces a failure toast and the local copy stays
// safe (the appshell handler never deletes the local entry on
// remote failure).
func TestUploadFeedbackFormspark_5xx_SurfacesStatusAndBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "formspark internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	err := UploadFeedbackFormspark(context.Background(), formsparkTestEntry(), srv.URL)
	if err == nil {
		t.Fatal("UploadFeedbackFormspark: expected error on 500, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error %q does not mention status 500", err.Error())
	}
}

// TestUploadFeedbackFormspark_429_RateLimited pins the
// rate-limit error path. Formspark documents 429 as the
// shape of the "too many requests" error; the helper must
// surface it cleanly so the toast can show "please retry
// later" rather than a generic "upload failed".
func TestUploadFeedbackFormspark_429_RateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	err := UploadFeedbackFormspark(context.Background(), formsparkTestEntry(), srv.URL)
	if err == nil {
		t.Fatal("UploadFeedbackFormspark: expected error on 429, got nil")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("error %q does not mention status 429", err.Error())
	}
}

// TestUploadFeedbackFormspark_RespectsContextDeadline pins
// the timeout contract (locked decision 3 + slice 1 plan):
// a hanging server breaches the deadline and returns an
// error rather than blocking forever. Use a 100ms deadline
// so the test runs fast while exercising the same code path.
func TestUploadFeedbackFormspark_RespectsContextDeadline(t *testing.T) {
	gate := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-gate
	}))
	defer func() {
		close(gate)
		srv.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := UploadFeedbackFormspark(ctx, formsparkTestEntry(), srv.URL)
	if err == nil {
		t.Fatal("UploadFeedbackFormspark: expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "deadline") &&
		!strings.Contains(err.Error(), "timeout") &&
		!strings.Contains(err.Error(), "context") {
		t.Errorf("error %q does not mention deadline/timeout/context", err.Error())
	}
}

// TestUploadFeedbackFormspark_EmptyEndpointFailsFast pins
// the "endpoint empty" failure mode. The helper must fail
// fast (without dialing) when the URL is blank so a
// misconfigured build cannot silently swallow the failure.
func TestUploadFeedbackFormspark_EmptyEndpointFailsFast(t *testing.T) {
	err := UploadFeedbackFormspark(context.Background(), formsparkTestEntry(), "")
	if err == nil {
		t.Fatal("UploadFeedbackFormspark: expected error on empty endpoint, got nil")
	}
	if !strings.Contains(err.Error(), "endpoint") {
		t.Errorf("error %q does not name the missing endpoint", err.Error())
	}
}

// TestUploadFeedbackFormspark_NetworkError pins the
// "endpoint unreachable" path. Use an unroutable port so
// the dial fails fast (no server running there).
func TestUploadFeedbackFormspark_NetworkError(t *testing.T) {
	err := UploadFeedbackFormspark(context.Background(), formsparkTestEntry(), "http://127.0.0.1:1/")
	if err == nil {
		t.Fatal("UploadFeedbackFormspark: expected error on unreachable host, got nil")
	}
	// The error should mention the network cause (connection
	// refused / no route / etc). Pin loosely so the test is
	// not tied to the exact Go runtime wording.
	if !strings.Contains(err.Error(), "127.0.0.1:1") {
		t.Errorf("error %q does not name the unreachable URL", err.Error())
	}
}

// TestUploadFeedbackFormspark_DefaultEndpointIsPinned pins
// the locked decision that the production endpoint is
// `https://submit-form.com/vJSONT1nB` (Formspark free-plan
// form owned by DixieData). The constant is the wire-format
// contract; changing it requires a CHANGELOG entry.
func TestUploadFeedbackFormspark_DefaultEndpointIsPinned(t *testing.T) {
	if DefaultFormsparkEndpoint != "https://submit-form.com/vJSONT1nB" {
		t.Errorf("DefaultFormsparkEndpoint = %q; want %q", DefaultFormsparkEndpoint, "https://submit-form.com/vJSONT1nB")
	}
}

// TestUploadFeedbackFormspark_OnlyOneRequestPerCall pins the
// "no retry loop" contract. A transient 5xx is a failure;
// the caller (appshell) surfaces the error to the user and
// the user clicks again. A future retry loop is a separate
// issue (locked decision 3: "no retry loop in v1").
func TestUploadFeedbackFormspark_OnlyOneRequestPerCall(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	_ = UploadFeedbackFormspark(context.Background(), formsparkTestEntry(), srv.URL)
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("server saw %d requests; want exactly 1 (no retry loop)", got)
	}
}
