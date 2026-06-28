package appshell

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// TestSetInfoToastHeaderWritesInfoKind locks down the helper used
// by every in-progress toast site (image import, shared archive
// import, memorial JSON import, Google Drive / Sheets exports,
// duplicate audit, bulk reviews, orphan cleanup). Regression test
// for issue #132: the header must set kind=info so the frontend
// classifies the toast for auto-dismiss and the heading reads
// "Heads up" rather than the misleading "Success".
func TestSetInfoToastHeaderWritesInfoKind(t *testing.T) {
	rec := httptest.NewRecorder()
	setInfoToastHeader(rec, "Importing 5 image(s)…")

	if got := rec.Header().Get("X-DixieData-Toast-Type"); got != "info" {
		t.Errorf("X-DixieData-Toast-Type = %q, want info", got)
	}
	if got := rec.Header().Get("X-DixieData-Toast"); got != "Importing 5 image(s)…" {
		t.Errorf("X-DixieData-Toast = %q, want %q", got, "Importing 5 image(s)…")
	}
}

// TestSetInfoToastHeaderSkipsEmpty verifies the empty-message
// guard short-circuits so we don't accidentally write an empty
// toast header that the frontend would render as a blank card.
func TestSetInfoToastHeaderSkipsEmpty(t *testing.T) {
	rec := httptest.NewRecorder()
	setInfoToastHeader(rec, "   ")
	if got := rec.Header().Get("X-DixieData-Toast"); got != "" {
		t.Errorf("empty/whitespace message must not set X-DixieData-Toast; got %q", got)
	}
}

// TestSetInfoToastHeaderMatchesSuccessKindContract ensures the
// info toast behaves like the success toast for auto-dismiss:
// both kinds must end up as the same X-DixieData-Toast-Type
// category on the wire so the frontend can use one switch for
// both. Success is the existing contract; info is the new path
// for in-progress messages.
func TestSetInfoToastHeaderMatchesSuccessKindContract(t *testing.T) {
	success := httptest.NewRecorder()
	setToastHeader(success, "Saved.")
	info := httptest.NewRecorder()
	setInfoToastHeader(info, "Importing…")

	successType := strings.TrimSpace(success.Header().Get("X-DixieData-Toast-Type"))
	infoType := strings.TrimSpace(info.Header().Get("X-DixieData-Toast-Type"))
	if successType == "" || infoType == "" || successType == infoType {
		t.Fatalf("success kind=%q info kind=%q — must be distinct so the frontend can render different headings, but both must be non-empty", successType, infoType)
	}
	if successType != "success" {
		t.Errorf("success kind = %q, want success", successType)
	}
	if infoType != "info" {
		t.Errorf("info kind = %q, want info", infoType)
	}
}