// settings_subpages_test.go -- issue #584 slice 4.
//
// Pins the /settings/<section> sub-page templ contract:
//   - Every sub-page carries its data-page-about="settings-<section>"
//     ID the appshell handler uses for routing.
//   - Every sub-page renders the "Back to Settings" breadcrumb.
//   - The matching panel renders inside each sub-page.
package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

func TestSettingsAppearancePageRenders(t *testing.T) {
	var buf bytes.Buffer
	if err := SettingsAppearancePage("soft", "jobs-page").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	for _, want := range []string{
		`id="page.settings.appearance"`,
		`data-settings-breadcrumb`,
		`Appearance`,
		`id="settings-appearance-panel"`,
		`name="theme"`, // radio group
	} {
		if !strings.Contains(content, want) {
			t.Errorf("SettingsAppearancePage missing %q", want)
		}
	}
}

func TestSettingsUpdatesPageRenders(t *testing.T) {
	var buf bytes.Buffer
	if err := SettingsUpdatesPage(viewmodel.UpdateSettings{}).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	for _, want := range []string{
		`id="page.settings.updates"`,
		`data-settings-breadcrumb`,
		`Software updates`,
		`id="settings-update-panel"`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("SettingsUpdatesPage missing %q", want)
		}
	}
}

func TestSettingsMaintenancePageRenders(t *testing.T) {
	var buf bytes.Buffer
	if err := SettingsMaintenancePage().Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	for _, want := range []string{
		`id="page.settings.maintenance"`,
		`Maintenance`,
		`id="settings-maintenance-panel"`,
		`Scan for Orphaned Images`,
		`Run Data Quality Scan`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("SettingsMaintenancePage missing %q", want)
		}
	}
}

func TestSettingsDataPageRenders(t *testing.T) {
	var buf bytes.Buffer
	if err := SettingsDataPage("INITIALIZE").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	for _, want := range []string{
		`id="page.settings.data"`,
		`Data`,
		`id="settings-data-panel"`,
		`INITIALIZE`,
		`name="confirmation_word"`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("SettingsDataPage missing %q", want)
		}
	}
}

func TestSettingsBuildPageRenders(t *testing.T) {
	var buf bytes.Buffer
	if err := SettingsBuildPage().Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	for _, want := range []string{
		`id="page.settings.build"`,
		`Build information`,
		`id="settings-build-panel"`,
		`data-settings-build`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("SettingsBuildPage missing %q", want)
		}
	}
}

func TestSettingsDiagnosticsPageRenders(t *testing.T) {
	var buf bytes.Buffer
	if err := SettingsDiagnosticsPage(true).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	for _, want := range []string{
		`id="page.settings.diagnostics"`,
		`Diagnostics`,
		`id="settings-diagnostics-panel"`,
		`Support & Diagnostics`,
		`Export Feedback Log`,
		`Report a Bug`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("SettingsDiagnosticsPage missing %q", want)
		}
	}
}

func TestSettingsIndexRendersSixDestinations(t *testing.T) {
	var buf bytes.Buffer
	if err := SettingsIndex().Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	for _, want := range []string{
		`data-settings-index-target="appearance"`,
		`data-settings-index-target="updates"`,
		`data-settings-index-target="maintenance"`,
		`data-settings-index-target="data"`,
		`data-settings-index-target="build"`,
		`data-settings-index-target="diagnostics"`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("SettingsIndex missing %q", want)
		}
	}
}