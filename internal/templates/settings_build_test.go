// RED-first regression net for issue #370 (the /settings/build
// panel). Pins the panel's chrome surface so a future
// refactor doesn't drop the codename or branch display.
package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestSettingsBuildPanelRendersCodenameAndBranch(t *testing.T) {
	var buf bytes.Buffer
	err := SettingsBuildPanel().Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()

	// The codename is the single most important piece of
	// build identity for a user; if the panel is rendered
	// but doesn't include it, the panel is useless.
	if !strings.Contains(content, "First Manassas") {
		t.Errorf("SettingsBuildPanel output missing codename 'First Manassas':\n%s", content)
	}

	// Branch is the other half of the v1 contract.
	if !strings.Contains(content, "dev") {
		t.Errorf("SettingsBuildPanel output missing branch 'dev':\n%s", content)
	}

	// Must carry a stable data-* selector so the smoke
	// probe can target it.
	if !strings.Contains(content, "data-settings-build") {
		t.Errorf("SettingsBuildPanel output missing data-settings-build selector:\n%s", content)
	}
}