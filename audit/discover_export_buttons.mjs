// audit/discover_export_buttons.mjs — derives the share-page export
// smoke manifest by scanning internal/templates/*.templ at runtime,
// so a new export button added to share.templ is automatically
// covered by audit/smoke.mjs without manually editing the harness.
//
// Why this exists — the bug class:
//
//   audit/smoke.mjs historically hand-maintained a `shareButtons`
//   array mapping each export endpoint to the regex of its button
//   label. Every time a new export route was added to share.templ
//   the harness had to be updated in a separate commit, and a
//   forgotten update meant the new button shipped without a
//   `share-{path}-navigates-to-jobs` assertion. The htmx 303 silent-
//   swallow bug shipped twice in part because the harness only knew
//   about the buttons present when the array was last touched.
//
//   This module replaces the hand-maintained array with auto-
//   discovery: walk every .templ file, find every form whose
//   `hx-post` (or bare button's `hx-post`) targets a URL under
//   `/export/`, `/integrations/google/`, or `/import/`, find the
//   associated button label, and emit a manifest the smoke harness
//   can iterate over without manual curation.
//
// Limitations (deliberate, documented):
//
//   - Templ files use templ.SafeURL(routebuilder.X()) for URLs;
//     resolving the routebuilder call requires either parsing Go
//     or reading the source to extract the builder name and
//     matching it against routebuilder.go. This module takes the
//     latter approach (regex over source).
//
//   - Buttons without a literal text label (icon-only buttons,
//     dropdown items) are skipped with a warning. If a future
//     edit needs to cover one, add `data-smoke-label="..."` to
//     the button — the scanner picks it up.
//
//   - The scanner only knows the share-page button surface. Other
//     pages (settings, review queue, insights) are not yet
//     auto-covered. Adding them is a follow-up — see the
//     pageButtonsFor() allowlist below for the current scope.

import { readFileSync, readdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join, relative, resolve } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(here, '..');
const templDir = join(repoRoot, 'internal', 'templates');

// discoverEligibleBuilderNames returns the Set of routebuilder
// function names that the smoke harness should cover. Eligibility
// is name-based (not URL-based) because some routebuilder
// functions serve /integrations/google/ endpoints that are
// preferences forms, not export buttons (e.g.
// GoogleCalendarPreferencesSave). Mixing those into the smoke
// manifest would generate false negatives.
//
// The eligible name prefixes are the ones DixieData treats as
// "export-style POST-then-navigate":
//
//   - Export* — share-page export buttons
//   - Import* — share-page import buttons (some are GETs, but the
//     file-upload ones POST and navigate)
//   - GoogleBackup, GoogleSheetsExport — the two Google
//     integration exports (other Google* builders are
//     preferences/calendar flows, not exports)
//
// Builders that do not start with these prefixes stay out of the
// smoke manifest even if their URL prefix matches.
function discoverEligibleBuilderNames() {
  return new Set([
    'ExportJSON',
    'ExportCSV',
    'ExportICalendar',
    'ExportStaticArchive',
    'ExportBackup',
    'ExportSharedArchive',
    'ExportDatabasePDFAsync',
    'ExportBugReport',
    'ExportFeedbackLog',
    'GoogleBackup',
    'GoogleSheetsExport',
    'ImportBackup',
    'ImportSharedArchive',
    'ImportMemorialJSON',
    'ImportFindAGraveCSV',
    'ImportResearchLogJSON',
  ]);
}

// eligiblePrefixes returns the URL prefixes that the smoke harness
// will exercise. Used to filter the discovered manifest down to
// the subset reachable from the current page (currently only
// /share). Future passes can scope to /settings, /review-queue,
// etc.
const eligiblePrefixes = ['/export/', '/integrations/google/', '/import/'];

// discoverShareExportButtons scans every .templ file in
// internal/templates/ for forms and bare buttons that post via
// htmx to an eligible URL, and returns a manifest of
// { label, path } entries the smoke harness can iterate over.
//
// Discovery algorithm:
//
//   1. For each eligible routebuilder builder name, find every
//      call site `routebuilder.Name(...)` in templ source.
//   2. From each call site, walk back ~200 chars to find the
//      enclosing form or button (whichever comes first) and
//      extract the button text.
//   3. De-duplicate by path (multiple buttons can post to the same
//      URL — we want one test case per URL, not one per button).
//
// The algorithm is intentionally narrow: it exists to catch the
// "forgot to add a smoke case for a new export" bug, not to be a
// general templ parser. If a future button does not show up in
// the manifest, the right fix is usually to give the button a
// `data-smoke-label` attribute (see findLabelForButton below).
export function discoverShareExportButtons() {
  const builders = discoverEligibleBuilderNames();
  const templFiles = readdirSync(templDir).filter((f) => f.endsWith('.templ'));
  const buttonsByPath = new Map();
  const unresolved = [];

  for (const file of templFiles) {
    const path = join(templDir, file);
    const src = readFileSync(path, 'utf8');
    const matches = scanForEligibleButtons(src, builders);
    for (const m of matches) {
      // Two filter passes:
      //   1. The URL must be one we have an explicit override for.
      //      This is what keeps the GoogleCalendar* / connect /
      //      disconnect endpoints out of the export manifest even
      //      though they share the /integrations/google/ prefix.
      //   2. The button must have a resolvable label so the smoke
      //      harness can click it.
      const prefix = m.builderName
        ? urlPrefixForBuilder(m.builderName)
        : m.pathPrefix;
      if (!prefix) continue;
      const isKnownExport = m.builderName
        ? Boolean(builderPrefixOverrides[m.builderName])
        : Boolean(
            literalPathOverrides[m.pathPrefix] ||
              actionPathOverrides[m.pathPrefix.split('?')[0]]
          );
      if (!isKnownExport) {
        // Literal URL not in the override table — skip to avoid
        // surfacing connect/disconnect/sync endpoints as exports.
        continue;
      }
      if (excludedPaths.has(prefix)) {
        // Path is covered by a dedicated smoke block elsewhere.
        continue;
      }
      const rawLabel = findLabelForButton(src, m.callIndex) || m.builderName;
      const label = labelToRegex(rawLabel);
      if (!label) {
        unresolved.push({
          path: prefix,
          source: relative(repoRoot, path),
          reason: 'no resolvable button label',
        });
        continue;
      }
      if (!buttonsByPath.has(prefix)) {
        buttonsByPath.set(prefix, {
          label,
          path: m.pathPrefix,
          source: relative(repoRoot, path),
          builderName: m.builderName,
        });
      }
    }
  }

  if (unresolved.length > 0 && process.env.SMOKE_DEBUG) {
    console.warn(
      '[discover] skipping buttons without a resolvable label; add data-smoke-label="..." to opt in:',
      unresolved
    );
  }

  return Array.from(buttonsByPath.values()).sort((a, b) =>
    a.path.localeCompare(b.path)
  );
}

// scanForEligibleButtons returns one record per templ site
// (form `hx-post`, `hx-put`, or `action=` attribute) whose URL
// matches one of `eligiblePrefixes`. Three flavours of templ
// markup are recognised:
//
//   - `hx-post={ templ.SafeURL(routebuilder.X()) }` — the typed
//     routebuilder form (preferred; enforced by hx_guard_test.go).
//   - `hx-post="/literal/path"` — the literal-string form used
//     by the share page's primary export grid.
//   - `action="/literal/path"` on a plain <form method="post">
//     without htmx (e.g. the static-archive button, which is
//     the canonical "plain form" case the rest of the
//     hx-swap=none navigation pattern was carved out to handle).
function scanForEligibleButtons(src, builders) {
  const out = [];
  // Pattern 1: `routebuilder.X(...)` call site.
  const callRe = /routebuilder\.(\w+)\s*\(/g;
  let m;
  while ((m = callRe.exec(src)) !== null) {
    const name = m[1];
    if (!builders.has(name)) continue;
    const prefix = urlPrefixForBuilder(name);
    if (!prefix) continue;
    out.push({
      builderName: name,
      callIndex: m.index,
      pathPrefix: prefix,
    });
  }
  // Pattern 2: literal `hx-post="/some/path"` (inside templ.Attributes
  // or directly on a <form>).
  const hxLiteralRe = /(?:^|\s|")hx-post":?\s*"(\/[^"]+)"/g;
  while ((m = hxLiteralRe.exec(src)) !== null) {
    const path = m[1];
    if (!eligiblePrefixes.some((p) => path.startsWith(p))) continue;
    out.push({
      builderName: null,
      callIndex: m.index,
      pathPrefix: path,
    });
  }
  // Pattern 2b: literal `data-action="/some/path"` (Option C
  // convention for bare buttons after the templ retag). Same
  // shape as hx-post.
  const dataActionRe = /(?:^|\s|")data-action":?\s*"(\/[^"]+)"/g;
  while ((m = dataActionRe.exec(src)) !== null) {
    const path = m[1];
    if (!eligiblePrefixes.some((p) => path.startsWith(p))) continue;
    out.push({
      builderName: null,
      callIndex: m.index,
      pathPrefix: path,
    });
  }
  // Pattern 3: literal `action="/some/path"` on a <form method="post">.
  // The static-archive button uses this form. We need to match
  // the action attribute but skip the `hx-post` matches above so
  // we don't double-count.
  const actionRe = /<form[^>]*\baction="(\/[^"]+)"/g;
  while ((m = actionRe.exec(src)) !== null) {
    const path = m[1];
    if (!eligiblePrefixes.some((p) => path.startsWith(p))) continue;
    out.push({
      builderName: null,
      callIndex: m.index,
      pathPrefix: path,
    });
  }
  return out;
}

// urlPrefixForBuilder maps a routebuilder function name to the
// URL prefix the smoke harness will match against. Keep in sync
// with the actual routes in routes.go. Builders that return
// URLs with arguments (e.g. SoldierPDF(id) -> /soldiers/{id}/pdf)
// still match by the prefix because the live server resolves the
// full path when the form posts.
const builderPrefixOverrides = {
  ExportJSON: '/export/json',
  ExportCSV: '/export/csv',
  ExportICalendar: '/export/ical',
  ExportStaticArchive: '/export/static-archive',
  ExportBackup: '/export/backup',
  ExportSharedArchive: '/export/shared-archive',
  ExportDatabasePDFAsync: '/export/database-pdf?async=1',
  ExportBugReport: '/export/bug-report',
  ExportFeedbackLog: '/export/feedback-log',
  // Issue #183: tag surfaces + share export-options toggle.
  TagsPage: '/tags',
  TagDetail: '/tags/1',
  BrowseBulkTag: '/browse/bulk-tag',
  ShareExportOptions: '/share/export-options',
  GoogleBackup: '/integrations/google/backup',
  GoogleSheetsExport: '/integrations/google/sheets/export',
  ImportBackup: '/import/backup',
  ImportSharedArchive: '/import/shared-archive',
  ImportMemorialJSON: '/import/memorial-json',
  ImportFindAGraveCSV: '/import/findagrave-csv',
  ImportResearchLogJSON: '/import/research-log-json',
};

// literalPathOverrides maps bare-string hx-post URLs (used in
// share.templ before the routebuilder migration) to the same
// URL prefix shape. Add a new export here in the same commit
// that adds the bare-string path to a .templ file; otherwise
// the scanner will skip the button (the entry stays as an
// `unresolved` warning when SMOKE_DEBUG is set).
const literalPathOverrides = {
  '/export/json': true,
  '/export/csv': true,
  '/export/ical': true,
  '/export/static-archive': true,
  '/export/backup': true,
  '/export/shared-archive': true,
  '/export/bug-report': true,
  '/export/feedback-log': true,
  // Issue #182: Share Queue endpoints are NOT exports in the
  // strict sense (they serve the modal/preview UX, not the
  // export pipeline). Listed so the discover test doesn't fire
  // a false-orphan assertion; they remain out of scope for the
  // 'all six canonical share-page exports' regression net.
  '/share/queue/modal': true,
  '/share/queue/preview': true,
  '/share/queue/clear': true,
  '/export/shared-archive?subset=1': true,
  // Issue #192: saved Share Queue presets. The literal list +
  // save + delete + apply endpoints are not 'exports' in the
  // strict sense (they're preset CRUD); listed so the discover
  // test doesn't fire a false-orphan assertion.
  '/share/queue/presets': true,
  '/share/queue/presets/1': true,
  '/share/queue/presets/1/apply': true,
  '/tags': true,
  '/tags/1': true,
  '/browse/bulk-tag': true,
  '/share/export-options': true,
  '/integrations/google/backup': true,
  '/integrations/google/sheets/export': true,
};

// Bare <form> action URLs that the harness should also exercise.
// The static-archive export uses a plain <form method="post">
// (no htmx), so the scanner picks it up via the action-attribute
// regex; this table opts it into the same export-manifest
// treatment as the htmx-driven buttons.
const actionPathOverrides = {
  '/export/static-archive': true,
  '/export/static-archive?async=1': true,
};

// excludedPaths lists button paths the auto-discovered manifest
// should skip. Used for endpoints that have a dedicated smoke
// block elsewhere in the harness (so the manifest would
// double-cover the same surface).
const excludedPaths = new Set([
  // The printable PDF modal has a dedicated [5b] smoke block in
  // audit/smoke.mjs that opens the modal and submits through any
  // button — its label inference can't pick a single button
  // cleanly because the modal has Close + Generate buttons.
  '/export/database-pdf?async=1',
]);

function urlPrefixForBuilder(name) {
  // Explicit override is the only source of truth. Falling back
  // to name-derivation produced false matches like
  // `GoogleCalendarPreferencesSave -> /integrations/google/calendarpreferencessave`
  // (which is not a valid URL and not an export button anyway).
  // If a new export builder is added, add it here in the same
  // commit; the scanner will then pick up its templ call sites.
  return builderPrefixOverrides[name] || null;
}

// findLabelForButton walks FORWARD from a routebuilder call site
// (or from a literal hx-post position) to the nearest
// components.Button / components.ButtonContent invocation INSIDE
// the same form. Returns the literal string passed as the first
// arg, OR the text inside a <span|div class="font-bold"> child,
// OR the value of a `data-smoke-label="..."` override attribute,
// OR null if nothing resolvable is found.
//
// Two patterns need to be handled:
//
//   Pattern A — label and hx-post on the SAME components.Button:
//     @components.Button("Label", ..., templ.Attributes{
//         "hx-post": "/path",
//     })
//   In this case the `callIndex` lands on the hx-post string
//   inside templ.Attributes, and the label is the first arg of
//   the same components.Button call. We walk BACK to find
//   components.Button( and extract its first arg.
//
//   Pattern B — label inside a form, hx-post on the form opening:
//     <form hx-post={ templ.SafeURL(routebuilder.X()) }>
//         @components.Button("Label", ...)
//     </form>
//   In this case the callIndex lands on the routebuilder call
//   site. The label is the first components.Button inside the
//   form body. We walk FORWARD to the next </form>, then find
//   the first components.Button call in that span.
//
// The walk stops at the next `</form>` close so it does not
// leak into sibling forms (e.g. the Google Backup button is in
// the same flex container as the Google Calendar buttons; a
// unbounded forward scan would otherwise pick up "Use Calendar"
// as the backup button's label).
function findLabelForButton(src, callIndex) {
  // Pattern A: label sits in the components.Button(...) call that
  // contains this hx-post attribute. Walk back to find the
  // matching `components.Button(` (the most recent one before
  // callIndex) and read its first string arg. For
  // components.Button the first string arg is the label; for
  // components.ButtonContent the label lives inside the trailing
  // `}) { <div class="font-bold">Label</div> }` body.
  const backWindow = src.slice(Math.max(0, callIndex - 300), callIndex);
  const lastBtnCall = backWindow.match(
    /components\.Button(?:Content)?\([^)]*$/m
  );
  if (lastBtnCall) {
    const start = Math.max(0, callIndex - 300) + lastBtnCall.index;
    const sameCall = src.slice(start, callIndex + 400);
    if (lastBtnCall[0].startsWith('components.ButtonContent(')) {
      // ButtonContent: label is in the first <span|div
      // class="font-bold"> after the call's `}) {`.
      const contentLabel = sameCall.match(
        /}\)\s*\{\s*<(?:span|div) class="font-bold">([^<]+)</
      );
      if (contentLabel) return contentLabel[1].trim();
    } else {
      const labelInCall = sameCall.match(
        /components\.Button\(\s*"([^"]+)"/
      );
      if (labelInCall) {
        return labelInCall[1];
      }
    }
  }

  // Pattern B: form-wrapped button. Walk forward to the next
  // </form>, then find the first components.Button(...) call in
  // that span.
  const closeIdx = src.indexOf('</form>', callIndex);
  const hi = closeIdx > 0 ? closeIdx : Math.min(src.length, callIndex + 600);
  const window = src.slice(callIndex, hi);

  // 1. Explicit `data-smoke-label="..."` override.
  const override = window.match(/data-smoke-label="([^"]+)"/);
  if (override) return override[1];

  // 2. components.Button("Label", ...) literal.
  const btn = window.match(/components\.Button\(\s*"([^"]+)"/);
  if (btn) return btn[1];

  // 3. components.ButtonContent { ... } child element with a
  //    font-bold label.
  const content = window.match(/<(?:span|div) class="font-bold">([^<]+)</);
  if (content) return content[1].trim();

  // 4. Bare <button>...</button> with literal text inside.
  const bare = window.match(/<button[^>]*>([^<]+)</);
  if (bare) return bare[1].trim();

  return null;
}

// labelToRegex converts a button label string to a case-insensitive
// regex anchored at the start, mirroring the hand-written entries
// that previously lived in smoke.mjs (`/^Export JSON/i` etc.).
// Returns null if the label could not be resolved; the caller
// then falls back to a `data-smoke-label` author hint or skips
// the entry with a warning.
function labelToRegex(label) {
  if (!label) return null;
  const escaped = label.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  return new RegExp('^' + escaped, 'i');
}

// pageButtonsFor returns only the manifest entries that should be
// exercised from a specific page. The smoke harness currently
// visits /share for every button; future iterations can add
// /settings, /review-queue, /insights as separate passes with
// their own scopes.
export function pageButtonsFor(_pagePath, manifest = discoverShareExportButtons()) {
  return manifest.filter((b) => eligiblePrefixes.some((p) => b.path.startsWith(p)));
}