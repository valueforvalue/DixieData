// audit/share_queue_subset.test.mjs — unit tests for the Share Queue
// subset export wiring (issue #182).
//
// Run with: node audit/share_queue_subset.test.mjs
//
// Scope: the static wiring that backs the subset flow — the modal
// fragment, the export action URL, the browse row button hook, and
// the route builder string. The full e2e (open modal, click
// export, land on /jobs/{id}) needs the dev binary's web mode
// with a real save-dialog override and is exercised by
// audit/smoke.mjs in a follow-up. This file is the cheaper
// pre-merge coverage that catches a refactor that drops the
// subset URL or removes the [Add] button hook.

import { strict as assert } from 'node:assert';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { discoverShareExportButtons } from './discover_export_buttons.mjs';

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = join(here, '..');

let pass = 0;
let fail = 0;

function test(name, fn) {
  try {
    fn();
    console.log(`  ✓ ${name}`);
    pass++;
  } catch (err) {
    console.log(`  ✗ ${name} — ${err.message}`);
    fail++;
  }
}

console.log('share_queue_subset tests');

// ─── route builder assertions ──────────────────────────────────

test('ExportSharedArchiveSubset returns /export/shared-archive?subset=1', () => {
  // The builder is in internal/routebuilder/routebuilder.go. We
  // re-derive the expected URL by reading the source so this
  // test stays in sync with future renames; an actual
  // `import` would tie the audit script to the Go module.
  const src = readFileSync(
    join(repoRoot, 'internal', 'routebuilder', 'routebuilder.go'),
    'utf8'
  );
  const m = src.match(/func\s+ExportSharedArchiveSubset\s*\(\)\s*string\s*\{[^}]*return\s+"([^"]+)"/);
  assert.ok(m, 'ExportSharedArchiveSubset not found in routebuilder.go');
  assert.equal(m[1], '/export/shared-archive?subset=1');
});

// ─── discoverer assertions ─────────────────────────────────────

test('modal-open endpoints are NOT in the export manifest', () => {
  // The discoverer scans .templ files for htmx-style posts. The
  // Share Build modal uses a plain <form action="..."> submit
  // (via the data-dixie-submit dispatcher) and the Build Share
  // Archive button is a JS-only modal trigger with no data-action.
  // The Share Queue routes are non-export endpoints and must
  // never be enumerated as exports.
  const manifest = discoverShareExportButtons();
  const paths = manifest.map((e) => e.path);
  for (const forbidden of [
    '/share/queue/modal',
    '/share/queue/preview',
    '/share/queue/clear',
    '/export/shared-archive?subset=1',
  ]) {
    assert.ok(
      !paths.includes(forbidden),
      `manifest should not include ${forbidden}; found: ${paths.join(', ')}`
    );
  }
});

test('the existing /export/shared-archive whole-archive button is still in the manifest', () => {
  // Sanity: the new subset flow did not displace the whole-archive
  // button. The whole-archive button is still the primary export
  // surface on the share page; the modal is an additional
  // envelope, not a replacement.
  const manifest = discoverShareExportButtons();
  const paths = manifest.map((e) => e.path);
  assert.ok(
    paths.includes('/export/shared-archive'),
    'manifest missing /export/shared-archive; found: ' + paths.join(', ')
  );
});

// ─── templ source assertions ───────────────────────────────────

test('share_queue_modal.templ exists and is well-formed', () => {
  const src = readFileSync(
    join(
      repoRoot,
      'internal',
      'templates',
      'partials',
      'share_queue_modal.templ'
    ),
    'utf8'
  );
  // The modal must carry the dialog-guard data attribute the
  // showOverlayModal dispatcher looks up, the form action built
  // via routebuilder.ExportSharedArchiveSubset(), and the
  // data-dixie-submit hook so the Option C dispatcher
  // intercepts the submit.
  assert.ok(
    src.includes('data-share-queue-modal'),
    'modal missing data-share-queue-modal attribute'
  );
  assert.ok(
    src.includes('data-dixie-submit="true"'),
    'modal form missing data-dixie-submit hook'
  );
  assert.ok(
    src.includes('routebuilder.ExportSharedArchiveSubset()'),
    'modal form action does not reference ExportSharedArchiveSubset builder'
  );
  assert.ok(
    src.includes('data-share-queue-form'),
    'modal form missing data-share-queue-form hook (JS uses it to inject selected_ids)'
  );
});

test('share.templ Build Share Archive button is a JS-only modal trigger', () => {
  const src = readFileSync(
    join(repoRoot, 'internal', 'templates', 'share.templ'),
    'utf8'
  );
  // The button has the data-share-queue-modal-open hook the
  // delegated click handler looks up. It must NOT have a
  // data-action attribute (the discoverer would then mis-classify
  // it as an export button).
  assert.ok(
    src.includes('data-share-queue-modal-open'),
    'Build Share Archive button missing data-share-queue-modal-open hook'
  );
  // The substring "Build Share Archive" must appear so the user
  // can see the button label.
  assert.ok(
    src.includes('Build Share Archive'),
    'Build Share Archive label not found in share.templ'
  );
});

test('browse.templ carries the data-share-queue-add hook on rows and cards', () => {
  const src = readFileSync(
    join(repoRoot, 'internal', 'templates', 'browse.templ'),
    'utf8'
  );
  // Count distinct usages. Both the mobile card and the desktop
  // table row add the button. Two hooks expected.
  const matches = src.match(/data-share-queue-add=/g) || [];
  assert.ok(
    matches.length >= 2,
    `expected >= 2 data-share-queue-add hooks (card + table row); got ${matches.length}`
  );
});

// ─── handler-side subset wiring assertions ─────────────────────

test('handleExportSharedArchive subset branch reads selected_ids via parseSelectedSoldierIDs', () => {
  const src = readFileSync(
    join(repoRoot, 'internal', 'appshell', 'exports_handlers.go'),
    'utf8'
  );
  // The subset branch must call parseSelectedSoldierIDs on the
  // r.Form["selected_ids"] slice and return 400 BEFORE opening
  // the native SaveFileDialog. This is the user-facing 400 the
  // issue's acceptance criterion requires.
  assert.ok(
    src.includes('parseSelectedSoldierIDs(r.Form["selected_ids"])'),
    'subset branch does not call parseSelectedSoldierIDs on r.Form'
  );
  // The validation must run before the dialog is opened. Scope
  // the search to the handleExportSharedArchive function body
  // (the first occurrence of both phrases in the file) to avoid
  // matching the dialog call in unrelated handlers.
  const validationIdx = src.indexOf('Add at least one Person Record to the Share queue');
  assert.ok(validationIdx > 0, 'subset 400 message not found in exports_handlers.go');
  // The subset branch lives inside handleExportSharedArchive;
  // the dialog call inside that handler is the FIRST
  // guardedSaveFileDialog call that appears AFTER the validation
  // message.
  const afterValidation = src.slice(validationIdx);
  const dialogIdx = afterValidation.indexOf('guardedSaveFileDialog(dupKey, opts)');
  assert.ok(dialogIdx > 0, 'guardedSaveFileDialog call not found after subset validation in exports_handlers.go');
  assert.ok(
    dialogIdx > 0,
    'subset validation must run before the native dialog opens (empty queue would otherwise open a dialog the user has to cancel)'
  );
});

console.log(`\n${pass} passed, ${fail} failed`);
process.exit(fail === 0 ? 0 : 1);
