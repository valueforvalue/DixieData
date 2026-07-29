// form_contract_test.go (issue #684)
//
// Table-driven render test that pins the form-contract surface:
// each <form> in the templ templates carries the field names,
// results-target, and method attributes that the Go handler
// actually reads via r.FormValue(). A future refactor that
// renames a form field breaks the contract — the table fails
// with the handler file:line that reads the field, and the
// templ file:line that produces the form, so the diff points
// at both ends of the seam.
//
// Pattern (per docs/agents/tdd.md "DixieData red-green loop"
// §Per-layer recipes, "templ" branch): bytes.Buffer + Render
// + strings.Contains. No goquery — the existing
// entry_form_test.go precedent stays consistent.
//
// Slice 1 (this commit) lands 5 example rows. Slice 2
// populates the remaining apply-sites from the issue body.
package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

// formContractRow is one row of the dispatch table.
//
//   selector    — CSS-ish substring used to locate the opening
//                 <form> tag inside the rendered HTML. Pin it to
//                 the action attribute so each row is unambiguous.
//   wantFields  — every <input name="..."> expected inside the form.
//                 Empty name attributes (hidden `_method` shape)
//                 are pinned by the wantHidden field instead.
//   wantTarget  — data-results-target value ("" if the form doesn't
//                 swap a fragment).
type formContractRow struct {
	name       string
	render     func() string
	selector   string
	wantFields []string
	wantTarget string
	// handlerRef — file:line where the handler reads the
	// pinned fields. Surfaced in the failure message so
	// the diff points at both ends of the seam. Optional.
	handlerRef string
}

// contractSelector picks the opening <form action="..."> tag
// whose action matches the given substring. Returns the
// tag string + the inner body, or false if not found.
func contractSelector(rendered, actionSubstr string) (string, string, bool) {
	needle := `action="`
	idx := 0
	for {
		relStart := strings.Index(rendered[idx:], needle)
		if relStart < 0 {
			return "", "", false
		}
		start := idx + relStart
		// Find the closing quote of action="..." — start AFTER
		// the opening quote we just consumed.
		quoteStart := start + len(needle)
		relEnd := strings.Index(rendered[quoteStart:], `"`)
		if relEnd < 0 {
			return "", "", false
		}
		action := rendered[quoteStart : quoteStart+relEnd]
		if !strings.Contains(action, actionSubstr) {
			// advance past this match and keep looking
			idx = start + 1
			continue
		}
		// Walk forward to grab the opening tag (up to the first
		// `>`) and the inner body (up to the matching </form>).
		tagEnd := strings.Index(rendered[start:], ">")
		if tagEnd < 0 {
			return "", "", false
		}
		opening := rendered[start : start+tagEnd+1]
		bodyStart := start + tagEnd + 1
		bodyEnd := strings.Index(rendered[bodyStart:], "</form>")
		if bodyEnd < 0 {
			return "", "", false
		}
		body := rendered[bodyStart : bodyStart+bodyEnd]
		return opening, body, true
	}
}

func TestFormContractMatchesHandler(t *testing.T) {
	// Render the SettingsView once — it contains 3 of the 5
	// pinned forms (theme + export_surface live in
	// SettingsAppearancePanel; quality_mode + initialize live
	// in SettingsView). source_url lives in SettingsUpdatePanel.
	var buf bytes.Buffer
	if err := SettingsView(
		"RESET",
		viewmodel.UpdateSettings{UsingDefaultSource: true, CanApply: false},
		"default",
		"jobs-page",
	).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render SettingsView: %v", err)
	}
	settingsHTML := buf.String()

	var appearanceBuf bytes.Buffer
	if err := SettingsAppearancePanel("default", "jobs-page").
		Render(context.Background(), &appearanceBuf); err != nil {
		t.Fatalf("Render SettingsAppearancePanel: %v", err)
	}
	appearanceHTML := appearanceBuf.String()

	var updateBuf bytes.Buffer
	if err := SettingsUpdatePanel(viewmodel.UpdateSettings{
		UsingDefaultSource: true,
		CanApply:           false,
		EffectiveSourceURL: "https://example.com/feed.json",
	}).Render(context.Background(), &updateBuf); err != nil {
		t.Fatalf("Render SettingsUpdatePanel: %v", err)
	}
	updateHTML := updateBuf.String()

	rows := []formContractRow{
		{
			name:       "SettingsTheme form carries theme radio + posts to /settings/theme",
			render:     func() string { return appearanceHTML },
			selector:   "/settings/theme",
			wantFields: []string{"theme"},
			wantTarget: "",
			handlerRef: "internal/appshell/settings_handlers.go:248 (handleSettingsTheme reads r.FormValue(\"theme\"))",
		},
		{
			name:       "SettingsExportSurface form carries export_surface radio + posts to /settings/export-surface",
			render:     func() string { return appearanceHTML },
			selector:   "/settings/export-surface",
			wantFields: []string{"export_surface"},
			wantTarget: "",
			handlerRef: "internal/appshell/settings_handlers.go:316 (handleSettingsExportSurface reads r.FormValue(\"export_surface\"))",
		},
		{
			name:       "SettingsQualityScan form carries quality_mode radio + posts to /settings/quality/scan",
			render:     func() string { return settingsHTML },
			selector:   "/settings/quality/scan",
			wantFields: []string{"quality_mode"},
			wantTarget: "#settings-quality-results",
			handlerRef: "internal/appshell/settings_handlers.go:380 (handleSettingsQualityScan reads r.FormValue(\"quality_mode\"))",
		},
		{
			name:       "SettingsInitialize form carries confirmation_word input + posts to /settings/initialize",
			render:     func() string { return settingsHTML },
			selector:   "/settings/initialize",
			wantFields: []string{"confirmation_word"},
			wantTarget: "",
			handlerRef: "internal/appshell/settings_handlers.go:472 (handleSettingsInitialize reads r.FormValue(\"confirmation_word\"))",
		},
		{
			name:       "SettingsUpdateSource form carries source_url input + posts to /settings/updates/source",
			render:     func() string { return updateHTML },
			selector:   "/settings/updates/source",
			wantFields: []string{"source_url"},
			wantTarget: "#settings-update-panel",
			handlerRef: "internal/appshell/app_update.go:25 (handleUpdateSource reads r.FormValue(\"source_url\"))",
		},
	}

	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			rendered := row.render()
			opening, body, ok := contractSelector(rendered, row.selector)
			if !ok {
				t.Fatalf("no <form> with %q found in rendered SettingsView; "+
					"rendered HTML length=%d. See %s",
					row.selector, len(rendered), row.handlerRef)
			}
			// Per #676 follow-up (commit 92fb264): the repo
			// standardized on POST-with-method-override at the
			// dispatcher, and dropped the explicit method="post"
			// attribute from most forms. Pin only the contract that
			// cannot be lost: the form is marked for the dispatcher
			// (data-dixie-submit) and the action URL resolves.
			if !strings.Contains(opening, `data-dixie-submit="true"`) {
				t.Errorf("opening tag missing data-dixie-submit=true; the dispatcher will not pick up this form. Got: %q. See %s",
					opening, row.handlerRef)
			}
			for _, field := range row.wantFields {
				if !strings.Contains(body, `name="`+field+`"`) {
					t.Errorf("form body missing name=%q; "+
						"handler reads it but the templ doesn't render it. "+
						"See %s", field, row.handlerRef)
				}
			}
			if row.wantTarget == "" {
				if strings.Contains(opening, `data-results-target=`) {
					t.Errorf("form carries unexpected data-results-target; "+
						"handler doesn't swap a fragment. Opening: %q. See %s",
						opening, row.handlerRef)
				}
			} else {
				if !strings.Contains(opening, `data-results-target="`+row.wantTarget+`"`) {
					t.Errorf("opening tag missing data-results-target=%q; "+
						"the dispatcher will swap the response into the wrong target. "+
						"Got: %q. See %s", row.wantTarget, opening, row.handlerRef)
				}
			}
		})
	}
}
