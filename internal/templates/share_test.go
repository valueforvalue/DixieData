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
	err := ShareView(viewmodel.GoogleStatus{}, nil, nil, viewmodel.ArchiveCounts{}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		"Which export should I choose?",
		"Single-record portrait",
		"Single-record landscape",
		"Full database printable PDF",
		"Full Database Printable PDF",
		"Preview Memorial JSON Import (.json)",
		"dry-run analysis",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("share view missing export help content %s", needle)
		}
	}
}

func TestShareViewKeepsResponsiveImportLayoutContract(t *testing.T) {
	var buf bytes.Buffer
	err := ShareView(viewmodel.GoogleStatus{}, nil, nil, viewmodel.ArchiveCounts{}).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		`class="responsive-two-col relative grid gap-6"`,
		`class="rounded-2xl border border-[rgba(141,116,64,0.35)] bg-white/70 p-4"`,
		`class="secondary-button justify-start text-left"`,
		`Preview Memorial JSON Import (.json)`,
		`id="share-status" class="responsive-span-2 md:col-span-2`,
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("share view missing responsive/split-screen contract %s", needle)
		}
	}
}
