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
];
