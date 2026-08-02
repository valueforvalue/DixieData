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
// seam? -- the seam is the DOM surface ID from internal/uiids,
// and the index list is the catalog of which surfaces have
// assertions).

// Slice 2 populates the first Playwright entry: the
// soldier-side images panel smoke (issue #392). Subsequent
// slices migrate smoke_submit_e2e (slice 3) and
// smoke_mega_menu_nav (slice 4); slice 5 populates the
// static-scanner kind:'scanner' entries for the class-2/4/6/9
// lints.
export const SURFACES = [
  {
    name: 'soldier-images',
    file: 'audit/smoke_soldier_images.mjs',
    kind: 'playwright',
    class: 4, // nested-form regression class
    note: 'issue #392 -- soldier-side Images panel UIIDs',
  },
  {
    name: 'submit-e2e',
    file: 'audit/smoke_submit_e2e.mjs',
    kind: 'playwright',
    class: 1, // Wails WebView2 body-stripping class
    note: 'issue #618 -- canonical submit-to-DB-to-render probe',
  },
  {
    name: 'mega-menu-nav',
    file: 'audit/smoke_mega_menu_nav.mjs',
    kind: 'playwright',
    class: 4, // nested-form regression class (mega-menu trigger must avoid wrapping/escaping the panel)
    note: 'issue #380 -- post-#380 Share & Review mega-menu regression net',
  },
  {
    name: 'article-edit',
    file: 'audit/smoke_article_edit.mjs',
    kind: 'playwright',
    class: 4, // nested-form regression class (image picker modal must be a sibling of the article form, not a descendant)
    note: 'issue #321 slice 3.7 -- article edit + new surface (Save / EditorToolbar / Preview / Cheatsheet / Image picker)',
  },
  {
    name: 'calendar',
    file: 'audit/smoke_calendar.mjs',
    kind: 'playwright',
    class: 4, // nested-form regression class
    note: 'issue #700 tier-1 -- /calendar month nav + day click + month export',
  },
  {
    name: 'browse',
    file: 'audit/smoke_browse.mjs',
    kind: 'playwright',
    class: 4, // nested-form regression class (filters form + bulk toolbar)
    note: 'issue #700 tier-1 -- /browse filter drawer + sort + row select + reset',
  },
  {
    name: 'article-new',
    file: 'audit/smoke_article_new.mjs',
    kind: 'playwright',
    class: 4, // nested-form regression class
    note: 'issue #700 tier-1 -- /articles/new save flow + /articles/{id} detail render',
  },
  {
    name: 'event-edit',
    file: 'audit/smoke_event_edit.mjs',
    kind: 'playwright',
    class: 4, // nested-form regression class
    note: 'issue #700 tier-1 -- /events/{id}/edit save flow + tags + linked persons forms',
  },
  {
    name: 'soldier-new',
    file: 'audit/smoke_soldier_new.mjs',
    kind: 'playwright',
    class: 4, // nested-form regression class (soldier new form must avoid wrapping issue #689/691 hazards)
    note: 'issue #700 tier-1 -- /soldiers/new save flow + /soldiers/{id} detail render + display_id readonly invariant',
  },
  {
    name: 'event-new',
    file: 'audit/smoke_event_new.mjs',
    kind: 'playwright',
    class: 4, // nested-form regression class (event new form must avoid wrapping issue #689/691 hazards)
    note: 'issue #700 tier-1 -- /events/new save flow + /events/{id} detail render + display_id readonly invariant',
  },
  {
    name: 'settings-appearance',
    file: 'audit/smoke_settings_appearance.mjs',
    kind: 'playwright',
    class: 2, // form-contract drift (settings appearance is the canonical #684 surface; field names must match handler)
    note: 'issue #700 tier-2 -- /settings/appearance theme + export-surface save round-trip',
  },
  {
    name: 'settings-maintenance',
    file: 'audit/smoke_settings_maintenance.mjs',
    kind: 'playwright',
    class: 2, // form-contract drift (settings subpages are the canonical #684 surface; form actions + field names must match handlers)
    note: 'issue #700 tier-2 -- /settings/maintenance image-orphan + data-quality scan form contracts + fragment-swap round-trip',
  },
  {
    name: 'settings-diagnostics',
    file: 'audit/smoke_settings_diagnostics.mjs',
    kind: 'playwright',
    class: 2, // form-contract drift (debug-mode toggle contract is #685-protected but the visual path is a regression net)
    note: 'issue #700 tier-2 -- /settings/diagnostics debug-mode toggle round-trip + bug-report form contract + feedback-log data-action',
  },
  {
    name: 'settings-data',
    file: 'audit/smoke_settings_data.mjs',
    kind: 'playwright',
    class: 8, // class 8 in spirit -- the .dixiedata base-name check is the operational guard; a future refactor that drops it is the regression
    note: 'issue #700 tier-2 -- /settings/data Initialize Local Archive confirmation-word guard + wipe round-trip',
  },
  {
    name: 'share-exports',
    file: 'audit/smoke_share_exports.mjs',
    kind: 'playwright',
    class: 6, // button-URL drift (data-action attrs on the 5 export buttons must resolve to /export/...)
    note: 'issue #700 tier-2 -- /share/exports 5 export data-action buttons + static-archive form contract + include-tags form contract (round-trip blocked: #705 dead-UI)',
  },
  {
    name: 'share-queue',
    file: 'audit/smoke_share_queue.mjs',
    kind: 'playwright',
    class: 2, // form-contract drift (the subset-export form action URL pattern is the canonical regression net)
    note: 'issue #700 tier-2 -- /share/queue empty-state + staged-rows + bulk-export form contract + per-row Remove round-trip',
  },
  {
    name: 'share-imports',
    file: 'audit/smoke_share_imports.mjs',
    kind: 'playwright',
    class: 6, // button-URL drift (3 import data-action buttons must resolve to /import/...)
    note: 'issue #700 tier-2 -- /share/imports 3 import data-action buttons + backup data-confirm',
  },
  {
    name: 'share-sync',
    file: 'audit/smoke_share_sync.mjs',
    kind: 'playwright',
    class: 6, // button-URL drift (10 Google Integration data-action buttons must resolve to /integrations/google/...)
    note: 'issue #700 tier-2 -- /share/sync 10 Google Integration data-action buttons + Unsync data-confirm + status region',
  },
  {
    name: 'review-queue-bulk',
    file: 'audit/smoke_review_queue_bulk.mjs',
    kind: 'playwright',
    class: 6, // button-URL drift (the per-row Mark-as-Resolved data-action URL must include /review/resolve?context=queue -- issue #248 fix regression net)
    note: 'issue #700 tier-2 -- /review-queue empty-state + 2 NeedsReview entries + bulk-action form contract + Mark-as-Resolved data-action regression net',
  },
  {
    name: 'tags',
    file: 'audit/smoke_tags.mjs',
    kind: 'playwright',
    class: 2, // form-contract drift (the rename / merge / delete forms on /tags must agree with handlers; survivor_id exclusion is the canonical #282 contract)
    note: 'issue #700 tier-2 -- /tags rename + merge-survivor-picker + delete form contracts + rename + merge round-trip',
  },
  {
    name: 'settings-updates',
    file: 'audit/smoke_settings_updates.mjs',
    kind: 'playwright',
    class: 2, // form-contract drift (settings update panel is the canonical #684 surface for fragment-swap + field name agreement)
    note: 'issue #700 tier-2 -- /settings/updates source_url save + Use Default + Check for Updates form contracts + round-trip',
  },
  {
    name: 'research-log',
    file: 'audit/smoke_research_log.mjs',
    kind: 'playwright',
    class: 2, // form-contract drift (research-log task create contract; title + evidence_type + notes field names must match handler)
    note: 'issue #700 tier-2 -- /soldiers/{id}/research-log Add Research Task form contract + task create round-trip + per-task resolve data-action regression net',
  },
  {
    name: 'initial-setup',
    file: 'audit/smoke_initial_setup.mjs',
    kind: 'playwright',
    class: 2, // form-contract drift (first_name + middle_name + last_name + birth_year must all be present per BuildUserNodePrefix)
    note: 'issue #700 tier-3 -- /setup first-launch wizard form contract + identity save round-trip + setupRequired-clear verification',
  },
  {
    name: 'recovery',
    file: 'audit/smoke_recovery.mjs',
    kind: 'playwright',
    class: 1, // pendingRecovery is the canonical regression net -- a guard regression would let a broken update proceed past the recovery gate
    note: 'issue #700 tier-3 -- /recovery healthy-state guard (303 -> /calendar) + no-render-of-recovery-markup verification',
  },
  {
    name: 'jobs-status',
    file: 'audit/smoke_jobs_status.mjs',
    kind: 'playwright',
    class: 1, // /jobs/active 204 in quiet state is a load-bearing contract for the layout progress-slot poll
    note: 'issue #700 tier-3 -- /jobs/active empty-state 204 + /jobs/{id} crash-resistance on non-existent + malformed IDs',
  },
  {
    name: 'jobs-active',
    file: 'audit/smoke_jobs_active.mjs',
    kind: 'playwright',
    class: 1, // /jobs/{id} live-render regression net (the handler chain is a load-bearing contract)
    note: 'issue #700 tier-3 -- /jobs/active quiet-state 204 + live export triggers enqueueExport -> /jobs/{id} redirect + page renders + back-link to /share',
  },
  {
    name: 'empty-body-dispatch',
    file: 'audit/smoke_empty_body_dispatch.mjs',
    kind: 'playwright',
    class: 9, // empty-body dispatch (issue #691 -- body-construction uses raw closest() instead of the resolved form)
    note: 'class 9 regression net -- /soldiers/{id}/edit Save Changes fires a fetch with a non-empty body that includes the filled field values + hidden fields',
  },
  {
    name: 'htmx-swap-rebind',
    file: 'audit/smoke_htmx_swap_rebind.mjs',
    kind: 'playwright',
    class: 3, // htmx swap re-binding (the cb4ac34 fix -- swapHtmlIntoTarget must call htmx.process(target) for swapped-in hx-* attrs to fire)
    note: 'class 3 regression net -- /browse pagination swap: page 1 -> Next -> swapped-in Prev pager link is htmx-wired and clickable (issue #706)',
  },
  {
    name: 'scanner-nested-forms',
    file: 'audit/lint_no_nested_forms.mjs',
    kind: 'scanner',
    class: 4, // nested-form class (issue #682)
    note: 'issue #682 -- no <form> inside <form>; HTML5 parser auto-closes outer form',
  },
  {
    name: 'scanner-invoker-resolve',
    file: 'scripts/lint-button-actions-resolve.mjs',
    kind: 'scanner',
    class: 6, // invoker-URL drift class (issue #687)
    note: 'issue #687 -- templ invoker URLs must resolve to a registered route',
  },
  {
    name: 'scanner-init-guards',
    file: 'scripts/lint-js-init-guards.mjs',
    kind: 'scanner',
    class: 9, // initialiser re-binding class (issue #685)
    note: 'issue #685 -- initializeDynamicContent dispatched callbacks must carry __<feature>Wired sentinel',
  },
  {
    name: 'scanner-orphan-handlers',
    file: 'audit/discover_orphan_handlers.mjs',
    kind: 'scanner',
    class: 2, // handler-without-invoker class (the "handler returns 200 but renders nothing" bug class)
    note: 'discover_orphan_handlers -- every registered route must have at least one templ/data-action invoker',
  },
  {
    name: 'scanner-embed-tree',
    file: 'audit/verify_embed_tree.mjs',
    kind: 'scanner',
    class: 5, // embed asset skip (issue #686; `//go:embed frontend` skips `_`/`.` prefixed files)
    note: 'verify_embed_tree -- every frontend asset referenced by live HTML is reachable via the go:embed directive',
  },
  {
    name: 'button-matrix',
    file: 'audit/smoke_button_matrix.mjs',
    kind: 'playwright',
    class: 1, // button-does-nothing class (the dominant UI bug class per docs/COMMON_BUGS.md §3)
    note: 'plan 2026-08-01 slice 1 (RED stub) -- per-button 4-state matrix on every probed surface. Slice 2 ships the real catalog walker.',
  },
];
