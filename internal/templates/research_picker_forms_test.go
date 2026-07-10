// Issue #426 regression net: every POST form in the Research &
// Review picker (internal/templates/research_picker.templ) MUST
// carry data-dixie-submit="true" so the form-submit dispatcher
// (frontend/app.js:5482 — submit listener gated on
// [data-dixie-submit]) intercepts native submits, reads the
// X-DixieData-Redirect response header, and calls
// window.location.assign. Without the attribute, the browser
// POSTs natively and renders the server's 200 + empty-body
// response as a blank page (white screen).
//
// Render the picker view with both the Continue panel and the
// search-results panel populated, scan the rendered HTML for
// every <form ... method="post"> opening tag, and assert each
// carries data-dixie-submit. The test fails fast with the
// offending form index + the rendered tag string so the
// regression is debuggable without re-running the GUI.
package templates

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

func TestResearchPickerFormsOptIntoDispatcher(t *testing.T) {
	person := viewmodel.PersonRecord{ID: 1, FirstName: "Test", LastName: "Person", DisplayID: "CSA-00001"}
	view := viewmodel.ResearchPickerView{
		CurrentPerson:    &person,
		SearchQuery:      "Test",
		SearchResults:    []viewmodel.PersonRecord{person},
		NextAction:       "research-log",
		SupportedActions: []string{"timeline", "research-log", "conflict-ledger", "research-pack"},
	}

	var buf bytes.Buffer
	if err := ResearchPickerView(view).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	html := buf.String()

	// Match every <form ... method="post" ...> opening tag. The
	// attribute order varies across templ branches (and the picker
	// has 3 distinct POST form groups plus the per-action Continue
	// loop), so we capture the whole opening tag and inspect it
	// for data-dixie-submit. \r\n tolerance comes from templ
	// running on Windows.
	formRe := regexp.MustCompile(`(?is)<form\b[^>]*method="post"[^>]*>`)
	matches := formRe.FindAllString(html, -1)
	if len(matches) == 0 {
		t.Fatalf("no <form method=\"post\"> tags found in rendered picker; template may have been gutted:\n%s", html)
	}

	// Accept either `data-dixie-submit="true"` or the bare
	// `data-dixie-submit` form. Both are valid HTML and the JS
	// selector `form.matches("[data-dixie-submit]")` matches
	// either. The repo uses both shapes; the bug is absence,
	// not spelling.
	bareRe := regexp.MustCompile(`data-dixie-submit(="[^"]*")?`)
	for i, form := range matches {
		if !bareRe.MatchString(form) {
			t.Errorf("form #%d POSTs without data-dixie-submit — native submit will white-screen on /research/select.\n"+
				"See issue #426 root cause.\nOffending tag: %s",
				i+1, strings.TrimSpace(form))
		}
	}
}

// TestResearchRecentFormsOptIntoDispatcher is the slice-3 follow-up
// to TestResearchPickerFormsOptIntoDispatcher. The picker-page
// regression above covers Continue + Change Person + the
// per-search-result Open form (slice 2 commits 6a340fc). It does
// NOT cover the recents-list Open form because the recents list
// is empty on a fresh /research server render — localStorage
// hydration is a JS-driven swap that calls /research/recent and
// renders the ResearchPickerRecent fragment directly.
//
// Symptom: a user with prior localStorage recents (dixiedata.
// research.recents) loads /research. The picker renders empty,
// JS hydrates the recents ul from /research/recent, and every
// recents Open button is a <form> without data-dixie-submit.
// Clicking it does a native POST to /research/select, the
// server returns 200 + X-DixieData-Redirect + empty body, and
// the browser renders the empty 200 body as a blank white page
// at /research/select.
//
// This test renders ResearchPickerRecent with two recent
// persons, scans the rendered HTML for every <form method="post">
// opening tag, and asserts each carries data-dixie-submit. It
// fails loudly with the offending form index + the rendered tag
// string so the regression is debuggable without re-running the
// GUI. Catches the slice-3 omission of data-dixie-submit on the
// recents-list form.
func TestResearchRecentFormsOptIntoDispatcher(t *testing.T) {
	alpha := viewmodel.PersonRecord{ID: 1, FirstName: "Test", LastName: "Alpha", DisplayID: "CSA-00001"}
	bravo := viewmodel.PersonRecord{ID: 2, FirstName: "Test", LastName: "Bravo", DisplayID: "CSA-00002"}
	view := viewmodel.ResearchPickerView{
		RecentPersons: []viewmodel.PersonRecord{alpha, bravo},
		NextAction:    "research-log",
	}

	var buf bytes.Buffer
	if err := ResearchPickerRecent(view).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	html := buf.String()

	formRe := regexp.MustCompile(`(?is)<form\b[^>]*method="post"[^>]*>`)
	matches := formRe.FindAllString(html, -1)
	if len(matches) != 2 {
		t.Fatalf("expected exactly 2 <form method=\"post\"> tags in recents list (one per recent person); got %d.\nRendered HTML:\n%s",
			len(matches), html)
	}

	bareRe := regexp.MustCompile(`data-dixie-submit(="[^"]*")?`)
	for i, form := range matches {
		if !bareRe.MatchString(form) {
			t.Errorf("recents form #%d POSTs without data-dixie-submit — native submit will white-screen on /research/select.\n"+
				"See issue #426 root cause (slice-3 follow-up: recents fragment missed in commit 6a340fc).\n"+
				"Offending tag: %s",
				i+1, strings.TrimSpace(form))
		}
	}
}