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

	// Issue #462: codename is a proper noun (First Battle of
	// Bull Run / First Manassas); the Settings Build panel
	// renders it italic alongside monospace + semibold. Pin
	// the class list so a refactor doesn't silently drop
	// the italic styling.
	const codenameOpen = `<dd class="`
	codenameIdx := strings.Index(content, codenameOpen)
	if codenameIdx < 0 {
		t.Fatalf("codename <dd class=...> not found")
	}
	codenameClose := strings.Index(content[codenameIdx:], `">`)
	if codenameClose < 0 {
		t.Fatalf("codename <dd class=...> close not found")
	}
	codenameTag := content[codenameIdx : codenameIdx+codenameClose+2]
	if !strings.Contains(codenameTag, "data-settings-build-codename") {
		t.Fatalf("expected data-settings-build-codename on the codename <dd>: %s", codenameTag)
	}
	if !strings.Contains(codenameTag, "italic") {
		t.Fatalf("codename <dd> should carry the `italic` Tailwind class (issue #462): %s", codenameTag)
	}
}