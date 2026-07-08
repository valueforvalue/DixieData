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