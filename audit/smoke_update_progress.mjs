/**
 * audit/smoke_update_progress.mjs — RED-first regression net for
 * issue #661 (download progress bar for in-place update).
 *
 * Source-scan probe — no live server needed. Asserts:
 *
 *   server-01 internal/update/updater.go declares ApplyPhase +
 *     UpdateProgress + downloadFileWithProgress +
 *     PrepareLatestWithProgress. Without these the apply handler
 *     can't report progress to the UI.
 *
 *   server-02 internal/update/updater.go PrepareLatest delegates
 *     to PrepareLatestWithProgress(nil) (back-compat path). A
 *     refactor that breaks this delegation would silently drop the
 *     back-compat callers.
 *
 *   server-03 every ApplyPhase constant is a distinct string so
 *     the UI can switch on Phase without ambiguity.
 *
 *   route-01 internal/appshell/routes.go registers
 *     GET /settings/updates/progress so the JS poller has an
 *     endpoint to fetch.
 *
 *   route-02 internal/appshell/routes.go POST
 *     /settings/updates/apply remains registered (the apply
 *     form's action target).
 *
 *   handler-01 internal/appshell/app_update.go handleApplyLatestUpdate
 *     spawns the prepare in a goroutine + returns within
 *     ~50ms instead of blocking on the multi-MB download. Pin:
 *     the handler returns SettingsUpdateApplyStarting() (not
 *     SettingsUpdateApplyStarted(version) — the latter waits for
 *     the prepare to finish).
 *
 *   handler-02 internal/appshell/app_update.go has a
 *     handleUpdateProgress function that reads from
 *     a.updateProgress and renders SettingsUpdateProgress. Pin:
 *     the function exists + is GET-method-gated.
 *
 *   appshell-01 internal/appshell/app.go has a *updateProgressState
 *     field on App so the apply goroutine and the progress
 *     endpoint share state.
 *
 *   appshell-02 internal/appshell/app.go initializes
 *     a.updateProgress via newUpdateProgressState() during App
 *     construction.
 *
 *   templ-01 internal/templates/entry_form.templ declares the
 *     SettingsUpdateApplyStarting templ component. Pin: function
 *     signature is `templ SettingsUpdateApplyStarting()` (no args)
 *     + the body renders `<div ... data-poll-progress="true">`.
 *
 *   templ-02 internal/templates/entry_form.templ declares
 *     SettingsUpdateProgress(phase, bytes, total, message, error)
 *     templ component. Pin: body renders
 *     `<div ... data-progress-phase={ string(phase) }>` so the JS
 *     poller can read the phase from a single attribute match.
 *
 *   templ-03 the apply form carries
 *     data-results-target="#settings-update-progress" so the
 *     dispatcher's inline render lands in the right place.
 *
 *   templ-04 the panel renders an empty
 *     `<div id="settings-update-progress"></div>` target so the
 *     initial render has somewhere to write.
 *
 *   js-01 frontend/app.js declares startUpdateProgressPollIfNeeded
 *     so the dispatcher can start the polling loop when the
 *     response fragment carries data-poll-progress.
 *
 *   js-02 frontend/app.js dispatches to startUpdateProgressPollIfNeeded
 *     inside the resultsTarget render branch (so a successful
 *     POST that writes the start fragment triggers the poll).
 *
 *   js-03 the poll loop's TERMINAL_PHASES set includes "applying"
 *     AND "error" so the loop stops at both terminal phases.
 *
 *   presentation-01 internal/presentation/views.go exposes
 *     SettingsUpdateApplyStarting() and
 *     SettingsUpdateProgress(progress) so app_update.go can call
 *     them.
 *
 * Pattern mirrors audit/smoke_codename_italics.mjs +
 * audit/smoke_article_cited_persons_no_n.mjs (issue #489 +
 * #663 probes) — source-scan regression nets for typst +
 * backend-template contracts.
 */

import { readFileSync } from 'node:fs';
import { strict as assert } from 'node:assert';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = join(__dirname, '..');

let pass = 0;
let fail = 0;

function test(name, fn) {
  try {
    fn();
    pass++;
    console.log(`  PASS ${name}`);
  } catch (err) {
    fail++;
    console.log(`  FAIL ${name}`);
    console.log(`    ${err.message}`);
  }
}

const UPDATER_GO = readFileSync(join(ROOT, 'internal', 'update', 'updater.go'), 'utf8');
const APP_UPDATE_GO = readFileSync(join(ROOT, 'internal', 'appshell', 'app_update.go'), 'utf8');
const APP_GO = readFileSync(join(ROOT, 'internal', 'appshell', 'app.go'), 'utf8');
const ROUTES_GO = readFileSync(join(ROOT, 'internal', 'appshell', 'routes.go'), 'utf8');
const ENTRY_FORM_TEMPL = readFileSync(
  join(ROOT, 'internal', 'templates', 'entry_form.templ'),
  'utf8',
);
const VIEWS_GO = readFileSync(
  join(ROOT, 'internal', 'presentation', 'views.go'),
  'utf8',
);
const APP_JS = readFileSync(join(ROOT, 'frontend', 'app.js'), 'utf8');

// ----- Server-side apply seam -----

test('server-01 updater.go declares ApplyPhase + UpdateProgress + PrepareLatestWithProgress + downloadFileWithProgress', () => {
  for (const needle of [
    /type\s+ApplyPhase\s+string/,
    /type\s+UpdateProgress\s+struct/,
    /func\s+\(s\s+\*Service\)\s+PrepareLatestWithProgress/,
    /func\s+\(s\s+\*Service\)\s+downloadFileWithProgress/,
    /PhaseIdle|PhaseDownload|PhaseVerify|PhaseExtract|PhaseRestorePoint|PhaseApplyStarted|PhaseError/,
  ]) {
    assert.ok(
      needle.test(UPDATER_GO),
      `updater.go missing pattern ${needle}`,
    );
  }
});

test('server-02 PrepareLatest delegates to PrepareLatestWithProgress(nil) (back-compat path)', () => {
  // The back-compat shape is a single-line return that delegates
  // with nil so existing callers that don't need progress get the
  // old behavior. A refactor that inlines the body instead of
  // delegating would silently drop the progress wiring.
  const delegateRe = /func\s+\(s\s+\*Service\)\s+PrepareLatest\(\)\s*\(\s*PreparedUpdate\s*,\s*error\s*\)\s*\{\s*return\s+s\.PrepareLatestWithProgress\(nil\)\s*\}/;
  assert.ok(
    delegateRe.test(UPDATER_GO),
    'PrepareLatest must be a one-line delegate to PrepareLatestWithProgress(nil) — a refactor that inlines the body would drop the progress wiring',
  );
});

test('server-03 every ApplyPhase constant is a distinct string', () => {
  const phases = [
    'PhaseIdle',
    'PhaseDownload',
    'PhaseVerify',
    'PhaseExtract',
    'PhaseRestorePoint',
    'PhaseApplyStarted',
    'PhaseError',
  ];
  const seen = new Map();
  for (const p of phases) {
    // Match the `PhaseX ApplyPhase = "..."` declaration.
    const re = new RegExp(`${p}\\s+ApplyPhase\\s*=\\s*"([^"]+)"`);
    const m = UPDATER_GO.match(re);
    assert.ok(m, `updater.go must declare ${p}`);
    const value = m[1];
    assert.ok(
      !seen.has(value),
      `phase value "${value}" duplicated (also declared on ${seen.get(value)})`,
    );
    seen.set(value, p);
  }
});

// ----- Routing -----

test('route-01 routes.go registers GET /settings/updates/progress', () => {
  assert.ok(
    /r\.Get\(\s*"\/settings\/updates\/progress"\s*,\s*a\.handleUpdateProgress\s*\)/.test(ROUTES_GO),
    'routes.go must register GET /settings/updates/progress -> handleUpdateProgress',
  );
});

test('route-02 routes.go keeps POST /settings/updates/apply', () => {
  assert.ok(
    /r\.Post\(\s*"\/settings\/updates\/apply"\s*,\s*a\.handleApplyLatestUpdate\s*\)/.test(ROUTES_GO),
    'routes.go must keep POST /settings/updates/apply -> handleApplyLatestUpdate (regression net for the existing route)',
  );
});

// ----- Handler wiring -----

test('handler-01 handleApplyLatestUpdate returns SettingsUpdateApplyStarting (immediate, non-blocking)', () => {
  // The function must render SettingsUpdateApplyStarting, NOT
  // SettingsUpdateApplyStarted(version) — the latter waits for the
  // prepare to finish and would block the response. Pin: the
  // function body contains `SettingsUpdateApplyStarting(`.
  const handlerMatch = APP_UPDATE_GO.match(
    /func\s+\(a\s+\*App\)\s+handleApplyLatestUpdate\([\s\S]*?\n\}/,
  );
  assert.ok(handlerMatch, 'app_update.go must define handleApplyLatestUpdate');
  const body = handlerMatch[0];
  assert.ok(
    body.includes('SettingsUpdateApplyStarting('),
    'handleApplyLatestUpdate must render SettingsUpdateApplyStarting() (the immediate fragment, not SettingsUpdateApplyStarted which waits for the prepare to finish)',
  );
  assert.ok(
    !body.includes('SettingsUpdateApplyStarted('),
    'handleApplyLatestUpdate must NOT call SettingsUpdateApplyStarted (which blocks on the prepare; the new flow returns the starting fragment and polls for live progress)',
  );
});

test('handler-02 app_update.go has handleUpdateProgress that reads a.updateProgress', () => {
  const handlerMatch = APP_UPDATE_GO.match(
    /func\s+\(a\s+\*App\)\s+handleUpdateProgress\([\s\S]*?\n\}/,
  );
  assert.ok(handlerMatch, 'app_update.go must define handleUpdateProgress');
  const body = handlerMatch[0];
  assert.ok(
    /if\s+r\.Method\s+!=\s+http\.MethodGet/.test(body),
    'handleUpdateProgress must gate on GET method',
  );
  assert.ok(
    body.includes('a.updateProgress.get()'),
    'handleUpdateProgress must read a.updateProgress.get() so the JS poller sees the latest state',
  );
  assert.ok(
    body.includes('SettingsUpdateProgress('),
    'handleUpdateProgress must render SettingsUpdateProgress',
  );
});

// ----- App wiring -----

test('appshell-01 App struct carries updateProgress *updateProgressState field', () => {
  assert.ok(
    /updateProgress\s+\*updateProgressState/.test(APP_GO),
    'App struct must carry updateProgress *updateProgressState so the apply goroutine and progress endpoint share state',
  );
});

test('appshell-02 App construction initializes a.updateProgress via newUpdateProgressState()', () => {
  assert.ok(
    /a\.updateProgress\s*=\s*newUpdateProgressState\(\)/.test(APP_GO),
    'App construction must call newUpdateProgressState() — a missing init would nil-deref on the first /settings/updates/progress poll',
  );
});

// ----- Templ -----

test('templ-01 SettingsUpdateApplyStarting renders the data-poll-progress marker', () => {
  // Pin the templ declaration + the body attribute so the JS
  // poller has a stable selector.
  const match = ENTRY_FORM_TEMPL.match(
    /templ\s+SettingsUpdateApplyStarting\(\)\s*\{[\s\S]*?\n\}/,
  );
  assert.ok(match, 'entry_form.templ must declare SettingsUpdateApplyStarting()');
  assert.ok(
    /id="settings-update-progress"/.test(match[0]),
    'SettingsUpdateApplyStarting body must render the #settings-update-progress container',
  );
  assert.ok(
    /data-poll-progress="true"/.test(match[0]),
    'SettingsUpdateApplyStarting body must carry data-poll-progress="true" so the JS dispatcher starts polling',
  );
});

test('templ-02 SettingsUpdateProgress renders the data-progress-phase marker', () => {
  const match = ENTRY_FORM_TEMPL.match(
    /templ\s+SettingsUpdateProgress\([^)]*\)\s*\{[\s\S]*?\n\}/,
  );
  assert.ok(match, 'entry_form.templ must declare SettingsUpdateProgress');
  assert.ok(
    /data-progress-phase=\{\s*string\(phase\)\s*\}/.test(match[0]),
    'SettingsUpdateProgress body must carry data-progress-phase={string(phase)} so the JS poller can read the phase from a single attribute match',
  );
});

test('templ-03 the apply form carries data-results-target="#settings-update-progress"', () => {
  assert.ok(
    /action=\{\s*templ\.SafeURL\(routebuilder\.SettingsUpdateApply\(\)\)\s*\}\s+data-dixie-submit="true"\s+data-results-target="#settings-update-progress"/.test(
      ENTRY_FORM_TEMPL,
    ),
    'apply form must carry data-results-target="#settings-update-progress" so the dispatcher writes the starting fragment to the right target',
  );
});

test('templ-04 the panel renders an empty #settings-update-progress target on first paint', () => {
  // Pin the initial target so the apply form's data-results-target
  // resolves before the first submit.
  assert.ok(
    /<div\s+id="settings-update-progress"\s*><\/div>/.test(ENTRY_FORM_TEMPL),
    'SettingsUpdatePanel must render an empty <div id="settings-update-progress"></div> so the apply form has a stable target on first paint',
  );
});

// ----- Frontend JS -----

test('js-01 app.js declares startUpdateProgressPollIfNeeded', () => {
  assert.ok(
    /async\s+function\s+startUpdateProgressPollIfNeeded\(/.test(APP_JS),
    'app.js must declare startUpdateProgressPollIfNeeded so the dispatcher can start the polling loop when the response fragment carries data-poll-progress',
  );
});

test('js-02 the dispatcher calls startUpdateProgressPollIfNeeded in the resultsTarget render branch', () => {
  // Pin the call site inside the dispatcher's
  // `if (resultsTargetSelector && !redirectTo && responseOk)` branch
  // so a future refactor that moves the inline render elsewhere
  // doesn't silently drop the progress poll trigger. The probe
  // anchors on the unique guard expression + the call within ~2KB.
  const dispatcherStart = APP_JS.indexOf('async function dispatchDixieDataForm');
  assert.ok(dispatcherStart > 0, 'dispatchDixieDataForm must be declared');
  const searchFrom = dispatcherStart + 100;
  const end = APP_JS.indexOf('\nasync function ', searchFrom);
  const body = APP_JS.slice(dispatcherStart, end > 0 ? end : dispatcherStart + 30000);
  const guardIdx = body.indexOf('resultsTargetSelector && !redirectTo && responseOk');
  assert.ok(
    guardIdx > 0,
    'dispatcher must contain the `resultsTargetSelector && !redirectTo && responseOk` guard that gates the inline render branch',
  );
  const after = body.slice(guardIdx, guardIdx + 3000);
  assert.ok(
    after.includes('startUpdateProgressPollIfNeeded(target)'),
    'the dispatcher must call startUpdateProgressPollIfNeeded(target) inside the resultsTarget render branch (within ~2KB of the guard expression)',
  );
});

test('js-03 the poll loop TERMINAL_PHASES set includes "applying" and "error"', () => {
  // The two terminal phases must both be in the terminator set;
  // missing one would cause the poll loop to run forever (or until
  // the user navigates away).
  const match = APP_JS.match(/TERMINAL_PHASES\s*=\s*new\s+Set\(\[([^\]]+)\]\)/);
  assert.ok(match, 'app.js must declare TERMINAL_PHASES = new Set([...])');
  const items = match[1];
  assert.ok(
    /"applying"/.test(items) && /"error"/.test(items),
    `TERMINAL_PHASES must include both "applying" and "error"; got ${items}`,
  );
});

// ----- Presentation facade -----

test('presentation-01 views.go exposes SettingsUpdateApplyStarting + SettingsUpdateProgress', () => {
  assert.ok(
    /func\s+SettingsUpdateApplyStarting\(\)\s+templ\.Component/.test(VIEWS_GO),
    'views.go must expose SettingsUpdateApplyStarting() templ.Component',
  );
  assert.ok(
    /func\s+SettingsUpdateProgress\(progress\s+update\.UpdateProgress\)\s+templ\.Component/.test(VIEWS_GO),
    'views.go must expose SettingsUpdateProgress(progress update.UpdateProgress) templ.Component',
  );
});

console.log(`\nResults: ${pass} pass, ${fail} fail`);
if (fail > 0) {
  process.exit(1);
}
process.exit(0);