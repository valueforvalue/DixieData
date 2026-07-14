// Package supportuploader POSTs a feedback entry + (optionally)
// a freshly-built bug-report bundle as multipart/form-data to a
// configurable support endpoint. The endpoint URL lives in
// records.LocalSettings.SupportEndpoint (added in issue #544);
// when it's empty, the helper is dormant (the UI greys the
// "Send to support" buttons).
//
// The local feedback-log JSONL is ALWAYS written first via
// appshell.appendFeedbackEntry (the same path the
// "Save Feedback" button takes today) so the user has a local
// trail even when the upload fails. The uploader is a
// best-effort add-on; the user's local copy is the source of
// truth per issue #121's logs-outside-data-dir convention.
//
// Why a small package rather than appshell helpers? Per issue
// #544 acceptance criterion "no http.Client shared between
// uploader + updater" -- separating the uploader from the
// in-place updater (`internal/update/updater.go`) keeps the
// two http.Client instances (with different timeout policies
// + different error contracts) independent. A package boundary
// also makes the helper testable without spinning up a Wails
// app.
package supportuploader

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// uploadTimeout is the locked deadline (issue #544 acceptance
// criterion 1): the helper waits up to 30s for the support
// endpoint to acknowledge. Power users behind a slow proxy
// can ship a smaller bundle or check the endpoint manually;
// an open-ended timeout would block the dispatcher forever.
const uploadTimeout = 30 * time.Second

// UploadFeedback POSTs the feedback entry (as a JSON metadata
// part named "metadata") + the bug-report bundle (as a file
// part named "bundle") to endpoint. Returns the support ticket
// id parsed from the response body's `ticket_id` field.
//
// The entry parameter is any JSON-marshalable value (the
// caller passes the appshell.feedbackEntry struct or its
// viewmodel-shaped projection); the helper marshals it as
// pretty-printed JSON so the support endpoint sees a readable
// payload.
//
// Errors:
//   - bundlePath missing / unreadable -> error names the path
//   - http.Client.Do failed -> error includes the network cause
//   - non-2xx response (4xx, 5xx) -> error includes the status code + body snippet
//   - response body missing ticket_id -> error names the missing field
//   - context deadline -> error wraps the timeout
func UploadFeedback(ctx context.Context, entry any, bundlePath, endpoint string) (string, error) {
	if endpoint == "" {
		return "", errors.New("support endpoint URL is empty; configure it in Settings → Support & Diagnostics")
	}
	// Bundle is OPTIONAL (issue #544 v1 ships the entry-only
	// path; the bundle path is a follow-up). Empty bundlePath
	// = skip the bundle part and send metadata only.
	var bundleName string
	if bundlePath != "" {
		if _, err := os.Stat(bundlePath); err != nil {
			// Fail fast on missing bundle before opening the
			// HTTP connection -- the file-not-found error names
			// the path so the caller can surface 'bundle not
			// built yet' rather than a generic upload failure.
			return "", fmt.Errorf("bundle file not found (%s): %w", bundlePath, err)
		}
		bundleName = filepath.Base(bundlePath)
	}

	// Build the multipart body in-memory. The bundle is small
	// enough (< 50 MB in the common case per the issue body)
	// that streaming the file directly is unnecessary; the
	// bytes.Buffer approach keeps the helper one-shot and
	// avoids the http.Request.Body lifetime dance.
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// metadata: the feedback entry as pretty JSON.
	entryJSON, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal feedback entry: %w", err)
	}
	if err := writeJSONPart(writer, "metadata", entryJSON); err != nil {
		return "", fmt.Errorf("write metadata part: %w", err)
	}

	// bundle: the bug-report zip as a file part. Filename
	// preserves the user's on-disk name so the support
	// endpoint can name the attachment correctly. Empty
	// bundlePath = entry-only upload (issue #544 v1).
	if bundlePath != "" {
		if err := writeFilePart(writer, "bundle", bundleName, bundlePath); err != nil {
			return "", fmt.Errorf("write bundle part: %w", err)
		}
	}

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("close multipart writer: %w", err)
	}

	// Apply the deadline. The issue's "30s timeout" is a
	// soft contract; if the caller passes a tighter ctx
	// (e.g. the test suite), we honour that instead.
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, uploadTimeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return "", fmt.Errorf("build upload request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	// Per issue #544 acceptance: "no http.Client shared
	// between uploader + the existing in-place updater".
	// The 30s timeout lives on the request context above;
	// the http.Client itself stays minimal (no shared
	// state with the in-place updater's client).
	client := &http.Client{}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("upload request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("support endpoint returned status %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var parsed struct {
		TicketID string `json:"ticket_id"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("support endpoint response is not JSON (ticket_id parse): %w (body: %s)", err, truncate(string(respBody), 200))
	}
	if parsed.TicketID == "" {
		return "", fmt.Errorf("support endpoint response missing ticket_id field (body: %s)", truncate(string(respBody), 200))
	}
	return parsed.TicketID, nil
}

// writeJSONPart writes the metadata JSON as a multipart form
// part with the given name. Used for the feedback entry.
func writeJSONPart(w *multipart.Writer, name string, payload []byte) error {
	part, err := w.CreateFormField(name)
	if err != nil {
		return err
	}
	_, err = part.Write(payload)
	return err
}

// writeFilePart opens bundlePath and streams its contents
// into a multipart file part named fieldname with filename
// displayName. The file is read in full (the bundle is
// small enough per the issue body's "< 50 MB" guidance).
func writeFilePart(w *multipart.Writer, fieldname, displayName, bundlePath string) error {
	file, err := os.Open(bundlePath)
	if err != nil {
		return err
	}
	defer file.Close()
	part, err := w.CreateFormFile(fieldname, displayName)
	if err != nil {
		return err
	}
	_, err = io.Copy(part, file)
	return err
}

// truncate caps a string at n bytes for error-message
// readability. Returns the whole string if shorter than n.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}