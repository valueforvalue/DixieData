// Issue #375: /articles/new (and /articles/{id}/edit, both rendered
// by the shared ArticleArticleForm in article_new.templ) has a "← Back"
// button wired through the form-submit dispatcher (data-dixie-submit="true"
// + data-action="/articles"). The dispatcher coerces the method to POST,
// /articles is GET-only, the response is 405, and the user sees a red
// "Request Failed" toast and stays on the form.
//
// Other Back buttons across the codebase use the data-history-back
// pattern (frontend/app.js:549 — applySmartBackLabels; entry_form.templ,
// event_form.templ, browse.templ, etc.) which is the right shape for
// a button that just navigates.
//
// This test pins the acceptance criterion: the Back button on the
// article form must NOT participate in the form-submit dispatcher.
// It must carry data-history-back so the JS smart-back helper handles
// navigation. Both the new and edit form paths render through
// ArticleArticleForm; the assertion covers both.
package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

func TestArticleFormBackButtonUsesHistoryBackNotDispatcher(t *testing.T) {
	cases := []struct {
		name   string
		isEdit bool
	}{
		{"new", false},
		{"edit", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := ArticleArticleForm(viewmodel.Article{}, tc.isEdit).Render(context.Background(), &buf)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			content := buf.String()

			// Locate the Back button by its visible label so the
			// assertion targets the right element rather than
			// scanning every button on the page.
			idx := strings.Index(content, "← Back")
			if idx < 0 {
				t.Fatalf("Back button label not found in rendered form")
			}
			// Walk forward to the next opening tag after the
			// label and inspect its attributes. The label lives
			// inside the Button component output, so the nearest
			// preceding <button> is the element under test.
			preceding := content[:idx]
			btnStart := strings.LastIndex(preceding, "<button")
			if btnStart < 0 {
				t.Fatalf("Back button element not found before label")
			}
			btnEnd := strings.Index(content[btnStart:], ">")
			if btnEnd < 0 {
				t.Fatalf("Back button element not closed")
			}
			btn := content[btnStart : btnStart+btnEnd+1]

			// The Back button must NOT carry data-dixie-submit.
			// Carrying it puts the button through dispatchDixieDataForm
			// which coerces GET -> POST and 405s against the
			// list route. See issue #375 root cause analysis.
			if strings.Contains(btn, "data-dixie-submit") {
				t.Errorf("Back button carries data-dixie-submit; will dispatch as POST and 405: %s", btn)
			}
			// The Back button must NOT carry data-action. data-action
			// is the dispatcher trigger attribute; the Back button
			// should navigate via history, not via fetch.
			if strings.Contains(btn, "data-action") {
				t.Errorf("Back button carries data-action; should use history-back instead: %s", btn)
			}
			// The Back button MUST carry data-history-back so the
			// JS smart-back helper (app.js:applySmartBackLabels)
			// picks it up and handles navigation.
			if !strings.Contains(btn, "data-history-back") {
				t.Errorf("Back button missing data-history-back; cannot navigate: %s", btn)
			}
			// type="button" prevents any default form submit even
			// if the dispatcher wiring is removed in a future edit.
			if !strings.Contains(btn, `type="button"`) {
				t.Errorf("Back button missing type=\"button\"; risks accidental submit: %s", btn)
			}
		})
	}
}