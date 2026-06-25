package appshell

// respond.go centralises the user-facing error response envelope for htmx
// handlers. It replaces the 100+ sites that previously wrote
// `http.Error(w, err.Error(), ...)` or `fmt.Fprintf(w, "...%v", err)` and
// leaked raw Go errors and SQLite constraint text into the rendered page.
//
// The envelope is:
//
//   - HTTP status code chosen from the Kind
//   - `X-DixieData-Toast` + `X-DixieData-Toast-Type` headers so the htmx
//     client shows a uniform toast (success/warning/error/info) on the
//     page that initiated the request
//   - a short user-facing message in the response body so the page can
//     swap it into the targeted region (typically via hx-target)
//
// The raw error is logged server-side via slog and never returned to the
// client. Callers should prefer `respondError` over `setToastHeaderWithType`
// for failure paths so the toast fires from one place; success paths keep
// using `setToastHeader` because they have no error to log.
//
// Tracking: replaces audit findings 1.1, 1.2, 1.3, 1.4, 1.9 from the
// 2026-06-24 full audit (issue #88). The full sweep across every handler
// is staged across follow-up PRs; this PR introduces the helper plus a
// representative sample of replacements.

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
)

// ErrorKind is the user-facing category for an error response. The kind
// drives the HTTP status code, the toast kind, and the log level.
type ErrorKind string

const (
	KindValidation ErrorKind = "validation" // 400 — bad user input
	KindNotFound   ErrorKind = "not_found"   // 404
	KindConflict   ErrorKind = "conflict"    // 409 — duplicate, schema collision
	KindForbidden  ErrorKind = "forbidden"   // 403
	KindUnavailable ErrorKind = "unavailable" // 503 — disk, IO, dependency missing
	KindInternal   ErrorKind = "internal"    // 500 — fallback for everything else
)

// statusForKind maps a kind to the HTTP status code returned to the
// client. Kept private because callers should not pick a status code
// directly; they should pick a kind.
func statusForKind(k ErrorKind) int {
	switch k {
	case KindValidation:
		return http.StatusBadRequest
	case KindNotFound:
		return http.StatusNotFound
	case KindConflict:
		return http.StatusConflict
	case KindForbidden:
		return http.StatusForbidden
	case KindUnavailable:
		return http.StatusServiceUnavailable
	case KindInternal:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

// toastKindForKind maps a kind to the toast kind the htmx client uses
// to style the message.
func toastKindForKind(k ErrorKind) string {
	switch k {
	case KindValidation:
		return "warning"
	case KindConflict:
		return "warning"
	case KindForbidden:
		return "error"
	case KindUnavailable:
		return "error"
	default:
		return "error"
	}
}

// defaultMessageForKind returns a generic user-facing message when the
// caller has no specific copy to provide. Callers should almost always
// pass their own message so the user knows what to fix.
func defaultMessageForKind(k ErrorKind) string {
	switch k {
	case KindValidation:
		return "The submitted data was invalid."
	case KindNotFound:
		return "That record was not found."
	case KindConflict:
		return "This action conflicts with existing data."
	case KindForbidden:
		return "You are not allowed to do that."
	case KindUnavailable:
		return "A required resource is unavailable. Try again."
	default:
		return "Something went wrong. Please try again."
	}
}

// respondError writes a user-facing error response and logs the raw
// error. It does NOT leak err.Error() to the client; the body and toast
// use the provided userMessage (or a generic default if empty).
//
// Behaviour:
//
//   - HTTP status code = statusForKind(kind)
//   - X-DixieData-Toast-Type = toastKindForKind(kind)
//   - X-DixieData-Toast = userMessage (or generic default)
//   - body = userMessage (htmx swaps this into hx-target)
//
// The raw err is logged at slog.Error with the kind and a stable "audit"
// field so the audit harness can grep for these without parsing log
// lines. Pass nil for err if no underlying error exists.
func respondError(w http.ResponseWriter, r *http.Request, kind ErrorKind, userMessage string, err error) {
	if w == nil {
		return
	}
	message := strings.TrimSpace(userMessage)
	if message == "" {
		message = defaultMessageForKind(kind)
	}
	status := statusForKind(kind)
	// Headers must be set before WriteHeader; set them now and rely on
	// the caller NOT having already written the status. We tolerate
	// double-write by checking via a sentinel header we control.
	if w.Header().Get("X-DixieData-RespondError-Marker") == "" {
		w.Header().Set("X-DixieData-RespondError-Marker", "1")
		w.Header().Set("X-DixieData-Toast", message)
		w.Header().Set("X-DixieData-Toast-Type", toastKindForKind(kind))
		w.WriteHeader(status)
	}
	_, _ = fmt.Fprint(w, message)

	// Log the raw error server-side. Use a stable attribute name so the
	// audit harness and any future log-based dashboards can filter.
	if err != nil {
		slog.Error("appshell: request failed",
			"audit", "respond-error",
			"kind", string(kind),
			"path", requestPath(r),
			"method", requestMethod(r),
			"err", err.Error(),
		)
	}
}

// respondValidation is a shorthand for 400 validation errors. Use this
// when the user submitted a bad value (parse error, missing required
// field, out-of-range number, etc.).
func respondValidation(w http.ResponseWriter, r *http.Request, userMessage string, err error) {
	respondError(w, r, KindValidation, userMessage, err)
}

// respondNotFound is a shorthand for 404. Use this when the requested
// resource does not exist or has been deleted.
func respondNotFound(w http.ResponseWriter, r *http.Request, userMessage string, err error) {
	respondError(w, r, KindNotFound, userMessage, err)
}

// respondInternal is a shorthand for 500. Use this as the catch-all when
// no specific kind fits. Always provide a userMessage; the raw err is
// logged but never sent to the client.
func respondInternal(w http.ResponseWriter, r *http.Request, userMessage string, err error) {
	respondError(w, r, KindInternal, userMessage, err)
}

// respondUnavailable is a shorthand for 503. Use this for disk IO
// failures, missing dependencies, or transient infrastructure issues.
func respondUnavailable(w http.ResponseWriter, r *http.Request, userMessage string, err error) {
	respondError(w, r, KindUnavailable, userMessage, err)
}

// respondConflict is a shorthand for 409. Use this for unique-key
// collisions, optimistic-locking failures, and merge-review conflicts
// surfaced to the user.
func respondConflict(w http.ResponseWriter, r *http.Request, userMessage string, err error) {
	respondError(w, r, KindConflict, userMessage, err)
}

// requestPath returns r.URL.Path or "<no-request>" if r is nil. Keeps
// the log line safe when called from a deferred recover.
func requestPath(r *http.Request) string {
	if r == nil || r.URL == nil {
		return "<no-request>"
	}
	return r.URL.Path
}

// requestMethod returns r.Method or "<no-request>" if r is nil.
func requestMethod(r *http.Request) string {
	if r == nil {
		return "<no-request>"
	}
	return r.Method
}