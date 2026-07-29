// audit/_lib/smoke_index.mjs (issue #700, ADR 0011)
//
// Ordered registry of every surface the smoke runner drives.
// Each entry has:
//   name   - the human-readable label (also the JSON key
//            in audit/smoke_summary.json).
//   file   - absolute or repo-relative path to the script.
//   kind   - 'playwright' (drives a real chromium via
//            _lib/smoke_runner.mjs::runProbe) OR 'scanner'
//            (spawns a static-source probe via the bridge
//            in smoke_aggregator.mjs).
//   class  - optional, maps to the 9-class button-bug
//            catalog from the audit (issue #681). Helps the
//            aggregator group findings in the JSON summary.
//
// Adding a probe = append one line. That's the entire
// contribution contract (per docs/agents/tdd.md §What's the
// seam? — the seam is the DOM surface ID from internal/uiids,
// and the index list is the catalog of which surfaces have
// assertions).

// Slice 1 is a no-op stub. The full population is filled in
// by slice 2 (smoke_soldier_images.mjs migration), slice 3
// (smoke_submit_e2e.mjs), slice 4 (smoke_mega_menu_nav.mjs),
// and slice 5 (the static-scanner bridge that adds the
// class-2/4/6/8/9 lint entries as kind: 'scanner' rows).
export const SURFACES = [];
