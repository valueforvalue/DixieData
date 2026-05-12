package templates

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/buildinfo"
)

func TestLayoutUsesLocalBootstrapScript(t *testing.T) {
	var buf bytes.Buffer
	if err := Layout("Test").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	if !strings.Contains(content, `src="/app.js"`) {
		t.Fatalf("layout should load local app.js")
	}
	if !strings.Contains(content, `href="/calendar"`) {
		t.Fatalf("layout should link calendar navigation to /calendar")
	}
	if strings.Contains(content, "unpkg.com/htmx.org") {
		t.Fatalf("layout should not depend on remote htmx")
	}
	if !strings.Contains(content, buildinfo.AppLabel()) || !strings.Contains(content, fmt.Sprintf("Schema v%d", buildinfo.SchemaVersion)) {
		t.Fatalf("layout should include app and schema versions")
	}
}
