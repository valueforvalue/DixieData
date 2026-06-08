package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

func TestShareViewShowsPrintableExportHelp(t *testing.T) {
	var buf bytes.Buffer
	err := ShareView(viewmodel.GoogleStatus{}, nil, nil).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"Which export should I choose?",
		"Single-record portrait",
		"Single-record landscape",
		"Full database printable PDF",
		"Full Database Printable PDF Export",
		"Preview Memorial JSON Import (.json)",
		"dry-run analysis",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("share view missing export help content %s", needle)
		}
	}
}
