package uiids

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegistryIDsAreUnique(t *testing.T) {
	seen := map[string]Surface{}
	for _, surface := range Registry {
		if surface.ID == "" {
			t.Fatal("registry contains empty ID")
		}
		if previous, ok := seen[surface.ID]; ok {
			t.Fatalf("duplicate ID %q for %q and %q", surface.ID, previous.Description, surface.Description)
		}
		seen[surface.ID] = surface
	}
}

func TestDebugEnabledUsesEnvVar(t *testing.T) {
	t.Setenv(DebugEnvVar, "1")
	if !DebugEnabled() {
		t.Fatalf("%s=1 should enable debug UI IDs", DebugEnvVar)
	}

	t.Setenv(DebugEnvVar, "false")
	if DebugEnabled() {
		t.Fatalf("%s=false should disable debug UI IDs", DebugEnvVar)
	}
}

func TestEnableFromArgsRecognizesDebugArg(t *testing.T) {
	t.Setenv(DebugEnvVar, "")
	if !EnableFromArgs([]string{DebugArg}) {
		t.Fatalf("expected %s to be recognized", DebugArg)
	}
	if !DebugEnabled() {
		t.Fatalf("%s should enable debug UI IDs", DebugArg)
	}
}

func TestEnableFromArgsIgnoresUnknownArgs(t *testing.T) {
	t.Setenv(DebugEnvVar, "")
	if EnableFromArgs([]string{"--not-a-real-flag"}) {
		t.Fatal("unexpected debug enable for unknown flag")
	}
	if DebugEnabled() {
		t.Fatal("unknown flags should not enable debug UI IDs")
	}
}

func TestRegistryIncludesResponsiveFoundationSurfaces(t *testing.T) {
	required := []string{
		PageBrowse,
		PageSoldierDetail,
		PageSoldierNew,
		PageSoldierEdit,
		PanelSoldierDetailSummary,
		PanelSoldierDetailRecords,
		PanelSoldierDetailImages,
		PanelSoldierFormRecords,
		PanelSoldierFormImages,
		PageExport,
		PanelExportActions,
		PageInitialSetup,
		PageInsights,
		PageReviewQueue,
		PageReviewQueueCompare,
		PageResearchCollectionsHub,
		PageResearchCollection,
		PageResearchLog,
		PageResearchPack,
		PageServiceTimeline,
		PageUnitCamaraderie,
		PageMergeReviewLedger,
		PageInsightsDrilldown,
		PanelSettingsLayout,
		PanelSettingsInitialize,
		PanelSettingsUpdates,
		OverlayFloatingMenu,
		OverlayFeedbackModal,
		OverlayPrintConfigModal,
		OverlayImageViewer,
	}

	seen := map[string]bool{}
	for _, surface := range Registry {
		seen[surface.ID] = true
	}

	for _, id := range required {
		if !seen[id] {
			t.Fatalf("registry missing responsive foundation surface %q", id)
		}
	}
}

func TestUIDocsCoverRegistryAndAuditFlow(t *testing.T) {
	docPath := filepath.Join("..", "..", "docs", "ui-ids.md")
	contentBytes, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", docPath, err)
	}
	content := string(contentBytes)

	for _, surface := range Registry {
		if !strings.Contains(content, "`"+surface.ID+"`") {
			t.Fatalf("ui id docs missing registry id %q", surface.ID)
		}
	}

	for _, needle := range []string{
		`.\scripts\build-debug.ps1`,
		`.\scripts\run-debug.ps1`,
		`DIXIEDATA_DEBUG_UI_IDS=1`,
		`Responsive audit gate`,
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("ui id docs missing audit instruction %q", needle)
		}
	}
}
