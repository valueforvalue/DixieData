package appshell

import (
	"os"
	"strings"
	"testing"
)

func TestFrontendIndexUsesLocalBootstrapScript(t *testing.T) {
	data, err := os.ReadFile(repoFixturePath(t, "frontend", "index.html"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, `src="/app.js"`) {
		t.Fatalf("frontend index should load local app.js")
	}
	if !strings.Contains(content, `hx-get="/calendar"`) {
		t.Fatalf("frontend index should bootstrap from /calendar")
	}
	if strings.Contains(content, "unpkg.com/htmx.org") {
		t.Fatalf("frontend index should not depend on remote htmx")
	}
}
