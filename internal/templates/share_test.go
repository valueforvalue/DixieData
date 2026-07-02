package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

// TestSharePrintConfigModalIsCentered is the regression test for
// issue #128. The print-config modal must render with the
// CSS classes that center it horizontally (justify-center) and
// vertically (items-center on >=640px viewports). Without the
// justify-center class the inner card aligns to the start edge of
// the overlay, which the user perceived as "loading on the left
// of the page instead of center".
func TestSharePrintConfigModalIsCentered(t *testing.T) {
	var buf bytes.Buffer
	if err := ShareView(viewmodel.GoogleStatus{}, nil, nil, viewmodel.ArchiveCounts{}, false).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	for _, needle := range []string{
		`id="share-print-config-modal"`,
		`justify-center`,
		`items-center`,
	} {
		if !strings.Contains(content, needle) {
			t.Errorf("ShareView print-config modal missing centering class %s; full HTML:\n%s", needle, content)
		}
	}
	if !strings.Contains(content, `data-print-config-modal`) {
		t.Errorf("ShareView print-config modal missing data-print-config-modal hook used by showOverlayModal")
	}
}

// TestShareModalsAreOverlayDivs asserts the print-config and
// google-calendar-preferences modals render as
// <div role="dialog" aria-modal="true"> overlays, not native
// <dialog> elements.
//
// The native <dialog> swap in issue #117 introduced WebView2
// focus-event reentry that crashed every native SaveFileDialog
// and OpenFileDialog opened from inside the modal (or from any
// sibling export button). Reverting to the div overlay restores
// pre-#117 behaviour while keeping focus trap + ESC close
// implemented manually in app.js.
func TestShareModalsAreOverlayDivs(t *testing.T) {
	var buf bytes.Buffer
	if err := ShareView(viewmodel.GoogleStatus{}, nil, nil, viewmodel.ArchiveCounts{}, false).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	for _, id := range []string{"share-print-config-modal", "google-calendar-preferences-modal"} {
		divNeedle := `<div id="` + id + `" role="dialog" aria-modal="true"`
		if !strings.Contains(content, divNeedle) {
			t.Fatalf("ShareView should render %s as a div overlay with role/aria-modal; got:\n%s", id, content)
		}
		dialogNeedle := `<dialog id="` + id + `"`
		if strings.Contains(content, dialogNeedle) {
			t.Fatalf("ShareView must not render %s as a native <dialog>; it regresses to the WebView2 focus-event crash", id)
		}
	}
}

func TestShareViewShowsPrintableExportHelp(t *testing.T) {
	var buf bytes.Buffer
	err := ShareView(viewmodel.GoogleStatus{}, nil, nil, viewmodel.ArchiveCounts{}, false).Render(context.Background(), &buf)
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
		"Import Memorial JSON (.json)",
		"Analyses the archive first",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("share view missing export help content %s", needle)
		}
	}
}

func TestShareViewKeepsResponsiveImportLayoutContract(t *testing.T) {
	var buf bytes.Buffer
	err := ShareView(viewmodel.GoogleStatus{}, nil, nil, viewmodel.ArchiveCounts{}, false).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		`class="responsive-two-col relative grid gap-6"`,
		`class="rounded-2xl border border-[rgba(141,116,64,0.35)] bg-white/70 p-4"`,
		`class="secondary-button justify-start text-left"`,
		`Import Memorial JSON (.json)`,
		`id="share-status" class="responsive-span-2 md:col-span-2`,
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("share view missing responsive/split-screen contract %s", needle)
		}
	}
}
