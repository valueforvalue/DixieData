package uiids

import (
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
