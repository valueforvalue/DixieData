package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

func TestEntryFormIncludesScratchPadLauncher(t *testing.T) {
	var buf bytes.Buffer
	err := EntryForm(models.Soldier{DisplayID: "DXD-00001"}, false).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	if !strings.Contains(content, "Open Scratch Pad") {
		t.Fatalf("entry form missing scratch pad button")
	}
	if !strings.Contains(content, `data-ui-id="panel.soldier.form.scratchpad"`) {
		t.Fatalf("entry form missing scratch pad surface id")
	}
}
