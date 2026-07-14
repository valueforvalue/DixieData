package supportuploader

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestUploadFeedback_HappyPath_ParsesTicketID pins the success
// contract (issue #544 acceptance criterion 1): the server
// returns 200 + a JSON body with a `ticket_id` field; the
// uploader returns the parsed id (not the raw body). Uses
// httptest.NewServer so the test exercises the real
// multipart + http.Client path.
func TestUploadFeedback_HappyPath_ParsesTicketID(t *testing.T) {
	var serverSaw struct {
		Method        string
		ContentType   string
		FormFields    map[string]string
		FileName      string
		FileSize      int64
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverSaw.Method = r.Method
		serverSaw.ContentType = r.Header.Get("Content-Type")
		// Parse multipart so we can verify what the uploader
		// actually sent across the wire.
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
			return
		}
		serverSaw.FormFields = map[string]string{}
		for k, v := range r.MultipartForm.Value {
			if len(v) > 0 {
				serverSaw.FormFields[k] = v[0]
			}
		}
		if files := r.MultipartForm.File["bundle"]; len(files) > 0 {
			f := files[0]
			serverSaw.FileName = f.Filename
			serverSaw.FileSize = f.Size
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"ticket_id": "TICKET-12345"})
	}))
	defer srv.Close()

	// Build a tiny fake bundle on disk so the helper has
	// something to attach.
	tmpDir := t.TempDir()
	bundlePath := filepath.Join(tmpDir, "bug-report.zip")
	if err := os.WriteFile(bundlePath, []byte("fake-bundle-content-for-test"), 0o644); err != nil {
		t.Fatalf("WriteFile bundle: %v", err)
	}

	entry := map[string]any{
		"submitted_at": "2026-07-13T22:00:00Z",
		"category":     "bug",
		"message":      "Sample feedback message",
		"contact_name": "Tester",
		"app_version":  "v1.2.23",
	}
	id, err := UploadFeedback(context.Background(), entry, bundlePath, srv.URL)
	if err != nil {
		t.Fatalf("UploadFeedback: %v", err)
	}
	if id != "TICKET-12345" {
		t.Errorf("ticket id = %q; want %q", id, "TICKET-12345")
	}
	if serverSaw.Method != "POST" {
		t.Errorf("server saw method = %q; want POST", serverSaw.Method)
	}
	if !strings.HasPrefix(serverSaw.ContentType, "multipart/form-data") {
		t.Errorf("Content-Type = %q; want multipart/form-data prefix", serverSaw.ContentType)
	}
	// The metadata is sent as a single JSON-blob part named
	// "metadata" (cleaner than per-field parts for a generic
	// receiver framework). The server-side handler unmarshals
	// it back into a typed struct. Assert the blob contains
	// the expected fields verbatim.
	metadataRaw := serverSaw.FormFields["metadata"]
	if metadataRaw == "" {
		t.Fatal("metadata part missing or empty")
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(metadataRaw), &parsed); err != nil {
		t.Fatalf("metadata not valid JSON: %v (raw: %q)", err, metadataRaw)
	}
	if parsed["category"] != "bug" {
		t.Errorf("metadata.category = %v; want \"bug\"", parsed["category"])
	}
	if parsed["message"] != "Sample feedback message" {
		t.Errorf("metadata.message = %v; want \"Sample feedback message\"", parsed["message"])
	}
	if serverSaw.FileName != "bug-report.zip" {
		t.Errorf("bundle filename = %q; want bug-report.zip", serverSaw.FileName)
	}
	if serverSaw.FileSize != int64(len("fake-bundle-content-for-test")) {
		t.Errorf("bundle size = %d; want %d", serverSaw.FileSize, len("fake-bundle-content-for-test"))
	}
}

// TestUploadFeedback_BundlePathMissing pins the bundle-missing
// error path (issue #544 acceptance criterion 1): a missing
// bundle file returns an error including the path so the
// caller can surface a useful message to the user. This is
// distinct from a 5xx from the server; the helper fails fast
// on local-file errors before opening the HTTP connection.
func TestUploadFeedback_BundlePathMissing(t *testing.T) {
	_, err := UploadFeedback(
		context.Background(),
		map[string]any{"message": "test"},
		filepath.Join(t.TempDir(), "does-not-exist.zip"),
		"http://127.0.0.1:1/", // never dialed
	)
	if err == nil {
		t.Fatal("UploadFeedback: expected error for missing bundle, got nil")
	}
	if !strings.Contains(err.Error(), "does-not-exist.zip") {
		t.Errorf("error %q does not name the missing bundle path", err.Error())
	}
}

// TestUploadFeedback_ServerReturns4xx_PinsStatusCode pins the
// 4xx error path (issue #544 acceptance criterion 1): the
// server's 4xx response surfaces as an error including the
// status code + body snippet so the caller can show a useful
// toast. A 4xx is the user/server-input error class; the
// caller should NOT retry.
func TestUploadFeedback_ServerReturns4xx_PinsStatusCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "endpoint not configured for this archive", http.StatusBadRequest)
	}))
	defer srv.Close()

	bundlePath := filepath.Join(t.TempDir(), "bundle.zip")
	if err := os.WriteFile(bundlePath, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := UploadFeedback(
		context.Background(),
		map[string]any{"message": "test"},
		bundlePath,
		srv.URL,
	)
	if err == nil {
		t.Fatal("UploadFeedback: expected error on 4xx, got nil")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error %q does not mention status 400", err.Error())
	}
}

// TestUploadFeedback_ServerReturns5xx_PinsStatusCode pins the
// 5xx error path (issue #544 acceptance criterion 1): the
// server's 5xx surfaces as an error including the status code
// so the caller can show a useful toast. A 5xx is the
// server-side error class; the caller should NOT retry (a
// future retry loop is out of scope per the issue body).
func TestUploadFeedback_ServerReturns5xx_PinsStatusCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "support endpoint down", http.StatusInternalServerError)
	}))
	defer srv.Close()

	bundlePath := filepath.Join(t.TempDir(), "bundle.zip")
	if err := os.WriteFile(bundlePath, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := UploadFeedback(
		context.Background(),
		map[string]any{"message": "test"},
		bundlePath,
		srv.URL,
	)
	if err == nil {
		t.Fatal("UploadFeedback: expected error on 5xx, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error %q does not mention status 500", err.Error())
	}
}

// TestUploadFeedback_ServerMissingTicketID pins the
// malformed-response contract: the server returned 200 but
// no `ticket_id` field in the JSON body. The uploader
// surfaces this as an error rather than returning the empty
// string (which the caller would silently accept as "ticket
// # not assigned" -- the wrong signal for the user).
func TestUploadFeedback_ServerMissingTicketID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"received"}`))
	}))
	defer srv.Close()

	bundlePath := filepath.Join(t.TempDir(), "bundle.zip")
	if err := os.WriteFile(bundlePath, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := UploadFeedback(
		context.Background(),
		map[string]any{"message": "test"},
		bundlePath,
		srv.URL,
	)
	if err == nil {
		t.Fatal("UploadFeedback: expected error when ticket_id missing, got nil")
	}
	if !strings.Contains(err.Error(), "ticket_id") {
		t.Errorf("error %q does not mention missing ticket_id field", err.Error())
	}
}

// TestUploadFeedback_RespectsContextDeadline pins the
// timeout contract (issue #544 acceptance criterion 1): a
// hanging server breaches the 30s timeout (the locked value
// per the issue body) and returns a context-deadline error
// rather than blocking forever. Use a 100ms deadline in the
// test (faster than 30s) so the test runs fast while still
// exercising the same code path.
func TestUploadFeedback_RespectsContextDeadline(t *testing.T) {
	gate := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-gate // block until test cleanup closes the gate
	}))
	defer func() {
		close(gate) // unblock the server goroutine after the request has returned
		srv.Close()
	}()

	bundlePath := filepath.Join(t.TempDir(), "bundle.zip")
	if err := os.WriteFile(bundlePath, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := UploadFeedback(
		ctx,
		map[string]any{"message": "test"},
		bundlePath,
		srv.URL,
	)
	if err == nil {
		t.Fatal("UploadFeedback: expected timeout error, got nil")
	}
	// Either the URL/client reported a deadline-exceeded
	// error or the request returned with an error; both are
	// acceptable for "didn't block forever".
	if !strings.Contains(err.Error(), "deadline") &&
		!strings.Contains(err.Error(), "timeout") &&
		!strings.Contains(err.Error(), "context") {
		t.Errorf("error %q does not mention deadline/timeout/context", err.Error())
	}
}

// TestUploadFeedback_BundlePartFilename pins the wire-format
// contract: the multipart file part is named "bundle" (not
// "file" or "attachment") so the server-side receiver has a
// stable, well-known field name. The filename field within
// the part preserves the user's on-disk name.
func TestUploadFeedback_BundlePartFilename(t *testing.T) {
	var partName string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reader, err := r.MultipartReader()
		if err != nil {
			http.Error(w, "no multipart", http.StatusBadRequest)
			return
		}
		for {
			p, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				http.Error(w, "next: "+err.Error(), http.StatusBadRequest)
				return
			}
			if p.FileName() != "" {
				partName = p.FormName()
				_ = p.Close()
				break
			}
			_ = p.Close()
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ticket_id":"x"}`))
	}))
	defer srv.Close()

	bundlePath := filepath.Join(t.TempDir(), "my-report.zip")
	if err := os.WriteFile(bundlePath, []byte("payload"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := UploadFeedback(
		context.Background(),
		map[string]any{"message": "test"},
		bundlePath,
		srv.URL,
	); err != nil {
		t.Fatalf("UploadFeedback: %v", err)
	}
	if partName != "bundle" {
		t.Errorf("multipart file part name = %q; want %q", partName, "bundle")
	}
}

