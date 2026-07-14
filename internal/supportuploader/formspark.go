package supportuploader

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// formsparkUploadTimeout is the locked deadline (issue #566
// slice 1 plan + locked decision 3): the helper waits up to
// 30s for Formspark to acknowledge. Power users behind a slow
// proxy can retry; an open-ended timeout would block the
// dispatcher forever. The appshell handler composes this with
// a 35s outer context so the handler never returns after the
// helper has already returned.
const formsparkUploadTimeout = 30 * time.Second

// UploadFeedbackFormspark POSTs the feedback entry to the
// Formspark endpoint as a single JSON body
// (Content-Type: application/json, Accept: application/json).
// The endpoint echoes the submission as the response body
// (no ticket-id field exists in the Formspark API), so any
// 2xx response is treated as "accepted" (locked decision 4).
//
// Field set sent across the wire (all flat top-level keys,
// no nesting so the Formspark dashboard columns align):
//
//	subject         — synthesised "<category> · <page_path> · <contact_email-or-anonymous>"
//	message         — verbatim user message
//	page_path       — current page when the user opened the modal
//	contact_name    — optional, omitempty
//	contact_email   — optional, omitempty
//	category        — one of bug / feature / research / general
//	app_version     — buildinfo.AppVersion
//	build_identity  — buildinfo.BuildIdentity()
//	schema_version  — int, serialised as a JSON number
//
// Errors:
//   - empty endpoint           → error names the field
//   - marshal failure          → error wraps json.Marshal error
//   - http.Client.Do failure   → error includes the network cause
//   - non-2xx response         → error includes status code + first 200 bytes of body
//   - context deadline         → error wraps the timeout
//
// The helper does NOT retry; a 5xx or 429 surfaces immediately
// so the appshell handler can show a useful toast. The local
// feedback JSONL is written by the caller (appshell) BEFORE
// this helper runs, so a remote failure never loses the report.
func UploadFeedbackFormspark(ctx context.Context, entry any, endpoint string) error {
	if strings.TrimSpace(endpoint) == "" {
		return errors.New("support endpoint URL is empty; DefaultFormsparkEndpoint must be non-empty")
	}

	payload, err := buildFormsparkPayload(entry)
	if err != nil {
		return fmt.Errorf("build formspark payload: %w", err)
	}

	// Apply the deadline. If the caller passes a tighter ctx
	// (e.g. the test suite) we honour that instead. The
	// deadline lives on the request context, not the http.Client,
	// so the lockstep "no http.Client shared with the updater"
	// contract from issue #544 still holds.
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, formsparkUploadTimeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build upload request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Fresh client per the issue #544 contract: no shared
	// state with the in-place updater. The 30s timeout lives
	// on the request context above; the client itself is the
	// standard library's default zero-value.
	client := &http.Client{}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("upload request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("formspark returned status %d: %s", resp.StatusCode, truncateForError(string(respBody), 200))
	}
	// 2xx = accepted. No body inspection (Formspark does not
	// promise a ticket-id field; the dashboard and email
	// notification are the delivery surfaces).
	return nil
}

// formsparkSubject is the synthesised email-notification
// title. Format: "<category> · <page_path> · <contact_email-or-anonymous>".
// The category is always present (the form's select element
// defaults to "bug"); page_path is always present (it is the
// page the user was on when the modal opened); the contact
// marker is "anonymous" when both contact_name and
// contact_email are blank. The format is stable so a future
// search / filter in the Formspark dashboard can match on it.
func formsparkSubject(category, pagePath, contactEmail string) string {
	contact := strings.TrimSpace(contactEmail)
	if contact == "" {
		contact = "anonymous"
	}
	cat := strings.TrimSpace(category)
	if cat == "" {
		cat = "general"
	}
	page := strings.TrimSpace(pagePath)
	if page == "" {
		page = "(no-page)"
	}
	return fmt.Sprintf("%s · %s · %s", cat, page, contact)
}

// formsparkPayload is the wire shape the helper serialises.
// It mirrors the test-side field set so the regression net in
// formspark_test.go stays in lockstep with the implementation.
// Contact fields use omitempty so an anonymous feedback
// submission does not produce a body with empty string
// fields; the dashboard groups anonymous feedback together.
type formsparkPayload struct {
	Subject       string `json:"subject"`
	Message       string `json:"message"`
	PagePath      string `json:"page_path"`
	ContactName   string `json:"contact_name,omitempty"`
	ContactEmail  string `json:"contact_email,omitempty"`
	Category      string `json:"category"`
	AppVersion    string `json:"app_version"`
	BuildIdentity string `json:"build_identity"`
	SchemaVersion int    `json:"schema_version"`
	SubmittedAt   string `json:"submitted_at,omitempty"`
}

// buildFormsparkPayload accepts any JSON-marshalable entry
// (the appshell.feedbackEntry struct or its viewmodel-shaped
// projection) and projects it onto the Formspark wire
// shape. The projection is permissive: missing fields are
// treated as empty strings / zero, so a future caller
// passing a different struct shape still produces a valid
// payload. The subject is synthesised here so the field
// derivation lives in one place.
func buildFormsparkPayload(entry any) ([]byte, error) {
	row := map[string]any{}
	switch v := entry.(type) {
	case map[string]any:
		row = v
	case map[string]string:
		for k, val := range v {
			row[k] = val
		}
	default:
		// Fall back to a generic JSON round-trip for typed
		// structs (appshell.feedbackEntry). One unmarshal +
		// remarshal pass is cheap and keeps the helper
		// decoupled from the appshell package (no import
		// cycle).
		raw, err := json.Marshal(entry)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &row); err != nil {
			return nil, err
		}
	}

	payload := formsparkPayload{
		Subject:       formsparkSubject(stringOf(row, "category"), stringOf(row, "page_path"), stringOf(row, "contact_email")),
		Message:       stringOf(row, "message"),
		PagePath:      stringOf(row, "page_path"),
		ContactName:   stringOf(row, "contact_name"),
		ContactEmail:  stringOf(row, "contact_email"),
		Category:      stringOf(row, "category"),
		AppVersion:    stringOf(row, "app_version"),
		BuildIdentity: stringOf(row, "build_identity"),
		SubmittedAt:   stringOf(row, "submitted_at"),
	}
	if sv, ok := row["schema_version"]; ok {
		switch n := sv.(type) {
		case int:
			payload.SchemaVersion = n
		case int32:
			payload.SchemaVersion = int(n)
		case int64:
			payload.SchemaVersion = int(n)
		case float64:
			payload.SchemaVersion = int(n)
		}
	}
	return json.Marshal(payload)
}

// stringOf returns the value of row[key] coerced to a
// trimmed string, or "" when the key is missing or holds a
// non-string value. The helper is small enough to keep
// inline; centralising the empty-string fallback prevents
// each call site from having to repeat the trim + type
// check.
func stringOf(row map[string]any, key string) string {
	v, ok := row[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

// truncateForError caps a string at n bytes for error-message
// readability. Returns the whole string if shorter than n.
// Mirrors the truncate() helper in supportuploader.go so the
// error shapes from both helpers look the same in the toast.
func truncateForError(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
