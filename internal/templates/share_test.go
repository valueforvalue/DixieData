package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

// TestShareLandingHasNoModals (issue #284) asserts the /share
// landing is modal-free. The print-config and
// google-calendar-preferences modals moved to /share/exports
// and /share/sync respectively. The subpages own their own
// modals now. This is the inverse of the pre-#284 tests
// (TestSharePrintConfigModalIsCentered +
// TestShareModalsAreOverlayDivs) which were deleted because
// the modals no longer live on this page.
func TestShareLandingHasNoModals(t *testing.T) {
	var buf bytes.Buffer
	if err := ShareView(nil, viewmodel.ArchiveCounts{}, nil).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := buf.String()
	for _, id := range []string{
		"share-print-config-modal",
		"google-calendar-preferences-modal",
	} {
		if strings.Contains(content, `id="`+id+`"`) {
			t.Errorf("/share landing must not contain modal %q (moved to subpage)", id)
		}
	}
}

// TestShareViewShowsSubOverviewHelp (issue #284, #255) asserts
// the /share landing carries the help copy that used to
// be inline on the pre-#284 landing. The export /
// import / print-config help moved to the subpages; the
// landing keeps the high-level "what is this page"
// summary. The Support & Diagnostics section moved off
// /share to /settings (issue #255). The subpage-level
// help (single-record vs full-database PDF, "Analyses the
// archive first", etc.) is now covered by the per-subpage
// render tests in share_subpages_handlers_test.go.
func TestShareViewShowsSubOverviewHelp(t *testing.T) {
	var buf bytes.Buffer
	err := ShareView(nil, viewmodel.ArchiveCounts{}, nil).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		// Landing-level help: what /share is + what
		// the Quick Actions surface offers. Support &
		// Diagnostics help (Export Feedback Log /
		// Bug Report Bundle) moved to /settings
		// (issue #255).
		"Share Archive",
		"Quick actions",
	} {
		if !strings.Contains(content, needle) {
			t.Errorf("/share landing missing sub-overview help %q", needle)
		}
	}
	// Subpage-specific help must NOT be on the landing
	// (it moved to the subpages).
	for _, forbidden := range []string{
		"Which export should I choose?",
		"Single-record portrait",
		"Single-record landscape",
		"Import Memorial JSON (.json)",
		"Analyses the archive first",
	} {
		if strings.Contains(content, forbidden) {
			t.Errorf("/share landing should not contain %q (moved to subpage)", forbidden)
		}
	}
}

// TestShareViewKeepsSubOverviewLayoutContract (issue #284, #255)
// asserts the /share landing uses the new sub-overview grid
// (lg:grid-cols-4 for the Quick Actions). The Support &
// Diagnostics card moved to /settings (issue #255); /share
// now stays focused on Exports / Imports / Sync / Merge
// Review. The old responsive-two-col grid that wrapped the
// inline sections is gone with the sections.
func TestShareViewKeepsSubOverviewLayoutContract(t *testing.T) {
	var buf bytes.Buffer
	err := ShareView(nil, viewmodel.ArchiveCounts{}, nil).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	content := buf.String()
	for _, needle := range []string{
		// New sub-overview Quick Actions grid.
		`sm:grid-cols-2 lg:grid-cols-4`,
	} {
		if !strings.Contains(content, needle) {
			t.Errorf("/share landing missing sub-overview layout contract %q", needle)
		}
	}
	// Support & Diagnostics moved off /share to /settings
	// (issue #255) — the buttons still work via the same
	// /export/feedback-log + /export/bug-report routes, but
	// they render in /settings now, not here.
	for _, forbidden := range []string{
		`/export/feedback-log`,
		`/export/bug-report`,
	} {
		if strings.Contains(content, forbidden) {
			t.Errorf("/share landing should not contain %q (moved to /settings per issue #255)", forbidden)
		}
	}
	// The old 2-col responsive grid that wrapped the
	// three inline sections is gone with the sections.
	for _, forbidden := range []string{
		`class="responsive-two-col relative grid gap-6"`,
		`id="memorial-preview-target"`,
		`memorial-preview-target`,
	} {
		if strings.Contains(content, forbidden) {
			t.Errorf("/share landing should not contain %q (moved to subpage)", forbidden)
		}
	}
}
