// settings_data_init_test.go -- issue #593.
//
// Pins the contract between the /settings/data Initialize
// Local Archive form and the POST /settings/initialize handler.
// The original #584 mega-menu split lifted the markup from the
// monolithic SettingsView into SettingsDataPanel but renamed
// the form field from confirmation_word to confirmation in
// transit. The handler (settings_handlers.go:369) reads
// r.FormValue("confirmation_word") so the field-name mismatch
// caused the handler to fall through to the cancel branch on
// every submit -- the user typed INITIALIZE, the page returned
// a plain-text "Initialization cancelled" response, the archive
// was unchanged.
//
// This test pins the contract two ways:
//
//   1. Renders the SettingsDataView and asserts the rendered
//      HTML contains name="confirmation_word" (the contract
//      field name, not the typo).
//   2. Submits a POST with the field name parsed from the
//      rendered form and asserts the response is NOT the
//      cancel-branch plain text -- proving the form and the
//      handler agree on the field name end-to-end.
//
// The end-to-end submit uses the cancel branch as the tripwire
// because exercising the full initializeLocalData() service
// requires a working dataDir + db. The cancel-branch tripwire
// catches the form/handler mismatch without that surface area.

package appshell

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/presentation"
	"github.com/valueforvalue/DixieData/internal/testtemp"
)

// TestSettingsDataFormFieldNameMatchesHandler pins the form's
// field-name attribute to the value the handler reads. The
// legacy SettingsView form used name="confirmation_word"; the
// #584 SettingsDataPanel form (issue #593) initially used
// name="confirmation" which the handler did NOT read -- the
// handler's cancel branch returned plain text and the archive
// was never reset.
func TestSettingsDataFormFieldNameMatchesHandler(t *testing.T) {
	var buf bytes.Buffer
	if err := presentation.SettingsDataView("INITIALIZE").Render(context.Background(), &buf); err != nil {
		t.Fatalf("SettingsDataView.Render: %v", err)
	}
	content := buf.String()
	// Field name the handler reads (settings_handlers.go:369).
	const want = `name="confirmation_word"`
	if !strings.Contains(content, want) {
		t.Errorf("rendered /settings/data form missing %q; the handler reads this field name and a mismatch causes a silent fall-through to the cancel branch (issue #593)", want)
	}
}

// TestSettingsDataFormSubmitReachesHandlerSuccessPath is the
// end-to-end tripwire. Renders the form, parses the form's
// action URL + the form's field name, POSTs with the parsed
// values, and asserts the handler did NOT return the cancel
// branch's plain-text "Initialization cancelled" response.
//
// We use the cancel branch as the tripwire (not a successful
// archive reset) because exercising initializeLocalData()
// requires a fully wired App with a working dataDir + db. The
// cancel-branch tripwire catches the form/handler field-name
// mismatch without that surface area and runs in ~10ms.
//
// If the form's field name matches the handler's read key,
// the handler either succeeds (success path: X-DixieData-
// Redirect header set) or fails on a downstream service call
// (no archive reset, 500 + error page). Either way, the
// cancel-branch plain text is absent.
//
// If the form's field name does NOT match (the bug shape
// from #593), the handler returns the cancel branch's plain
// text "Initialization cancelled. Type INITIALIZE to confirm."
// and this test fails.
func TestSettingsDataFormSubmitReachesHandlerSuccessPath(t *testing.T) {
	// Render the form.
	var buf bytes.Buffer
	if err := presentation.SettingsDataView("INITIALIZE").Render(context.Background(), &buf); err != nil {
		t.Fatalf("SettingsDataView.Render: %v", err)
	}
	content := buf.String()

	// Parse the form's action URL.
	actionURL := parseFormAction(content)
	if actionURL == "" {
		t.Fatalf("could not find <form action=\"...\"> in rendered /settings/data; the page should post to %q", "/settings/initialize")
	}

	// Parse the form's field name. The bug shape from #593 was
	// the field name diverging from what the handler reads.
	fieldName := parseFormFieldName(content)
	if fieldName == "" {
		t.Fatalf("could not find <input ... name=\"...\"> in rendered /settings/data form")
	}

	// Wire an App whose dataDir ends in .dixiedata so the
	// handler's downstream initializeLocalData() can be
	// exercised if the field name matches. Use a fresh temp
	// dir; the directory's parent must exist for MkdirTemp
	// to succeed inside initializeLocalData().
	dataDir := filepath.Join(testtemp.New(t).Path(), ".dixiedata")
	app := NewApp()
	app.dataDir = dataDir
	app.setupRoutes()

	// Submit with the parsed field name + parsed action URL.
	form := url.Values{fieldName: {"INITIALIZE"}}
	req := httptest.NewRequest(http.MethodPost, actionURL, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	// initializeLocalData() may have opened a fresh DB at
	// a.dataDir; close it before t.TempDir cleanup so Windows
	// can release the file handle. The tripwire assertion
	// below doesn't care about the DB state.
	if app.database != nil {
		_ = app.database.Close()
		app.database = nil
	}

	// The cancel branch returns plain text starting with
	// "Initialization cancelled." -- the bug shape. If the
	// form field name matches the handler, the handler
	// proceeds past the cancel branch (success path sets
	// X-DixieData-Redirect + 200; service-failure path
	// returns 500 + error page; either way, the cancel text
	// is absent).
	body := rec.Body.String()
	if strings.Contains(body, "Initialization cancelled") {
		t.Errorf("handler returned the cancel-branch plain text; the form's field name %q does not match what the handler reads (issue #593). body: %s", fieldName, firstNChars(body, 400))
	}
}

// parseFormAction extracts the action attribute from the first
// <form> in the rendered HTML. The Initialize form is the only
// form on the /settings/data page, so a single match suffices.
var formActionRe = regexp.MustCompile(`<form[^>]*\baction="([^"]+)"`)

func parseFormAction(html string) string {
	m := formActionRe.FindStringSubmatch(html)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// parseFormFieldName extracts the name attribute from the first
// text <input> in the rendered form. The Initialize form has
// one text input (the confirmation field); we want to read its
// name attribute exactly as the browser would.
var formInputNameRe = regexp.MustCompile(`<input[^>]*\btype="text"[^>]*\bname="([^"]+)"`)

func parseFormFieldName(html string) string {
	m := formInputNameRe.FindStringSubmatch(html)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func firstNChars(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}