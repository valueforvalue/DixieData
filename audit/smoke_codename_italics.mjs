/**
 * audit/smoke_codename_italics.mjs — RED-first regression net for
 * issue #489 (render release codename in italics across chrome + PDF).
 *
 * Source-scan probe — no live server needed. Asserts:
 *
 *   Chrome (3 wrap sites):
 *     1. internal/templates/layout.templ:112 wraps buildinfo.Codename()
 *        in <em> for the top nav brand pill.
 *     2. internal/templates/layout.templ:327 wraps buildinfo.Codename()
 *        in <em> for the page footer.
 *     3. internal/templates/entry_form.templ:1232 wraps
 *        buildinfo.Codename() in <em> inside the already-italic <dd>
 *        (Settings → Build panel).
 *
 *   PDF (5 wrap sites):
 *     4. templates/common/record_card.typ wraps branding.at("codename")
 *        in #emph() in the shared footer.
 *     5-8. templates/article_landscape.typ, article_portrait.typ,
 *          event_landscape.typ, event_portrait.typ each declare a
 *          codename let-binding from branding and wrap it in
 *          #emph() in their inline footer call.
 *
 *   CLI / OS-title safety:
 *     9. internal/versioninfo/versioninfo.go:104 is unchanged
 *        (still `var CurrentReleaseName = "First Manassas"` plain
 *        string — no HTML wrapping at the constant level).
 *    10. main.go does NOT introduce <em> tags around the codename
 *        in either the CLI --version path (line 41) or the Wails
 *        OS Title bar (line 195).
 */

import { readFileSync, existsSync } from 'node:fs';
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

// ----- Chrome -----
const LAYOUT = readFileSync(join(ROOT, 'internal', 'templates', 'layout.templ'), 'utf8');
const ENTRY_FORM = readFileSync(join(ROOT, 'internal', 'templates', 'entry_form.templ'), 'utf8');

test('chrome-01 layout.templ top nav brand wraps Codename() in <em>', () => {
  // Find a 200-char window containing "DixieData —" + "buildinfo.Codename()"
  // and assert <em>...</em> wraps it.
  const re = /DixieData\s*—\s*<em>\{\s*buildinfo\.Codename\(\)\s*\}<\/em>/;
  assert.ok(
    re.test(LAYOUT),
    'layout.templ top nav brand must wrap buildinfo.Codename() in <em>; expected `DixieData — <em>{ buildinfo.Codename() }</em>`',
  );
});

test('chrome-02 layout.templ footer wraps Codename() in <em>', () => {
  const re = /<em>\{\s*buildinfo\.Codename\(\)\s*\}<\/em>\s*·\s*Schema v/;
  assert.ok(
    re.test(LAYOUT),
    'layout.templ footer must wrap buildinfo.Codename() in <em> with the · Schema separator following',
  );
});

test('chrome-03 entry_form.templ Settings Build <dd> wraps Codename() in <em>', () => {
  // The <dd> already carries `font-mono font-semibold italic`. Now it
  // also wraps the codename in <em> for semantic correctness.
  const re = /<dd[^>]*data-settings-build-codename[^>]*>\s*<em>\{\s*buildinfo\.Codename\(\)\s*\}<\/em>\s*<\/dd>/;
  assert.ok(
    re.test(ENTRY_FORM),
    'entry_form.templ Settings Build <dd> must wrap buildinfo.Codename() in <em> inside the italic <dd>',
  );
});

// ----- PDF -----
const RECORD_CARD = readFileSync(join(ROOT, 'templates', 'common', 'record_card.typ'), 'utf8');
const ARTICLE_LANDSCAPE = readFileSync(join(ROOT, 'templates', 'article_landscape.typ'), 'utf8');
const ARTICLE_PORTRAIT = readFileSync(join(ROOT, 'templates', 'article_portrait.typ'), 'utf8');
const EVENT_LANDSCAPE = readFileSync(join(ROOT, 'templates', 'event_landscape.typ'), 'utf8');
const EVENT_PORTRAIT = readFileSync(join(ROOT, 'templates', 'event_portrait.typ'), 'utf8');

test('pdf-01 templates/common/record_card.typ wraps branding.codename in #emph()', () => {
  // The shared footer reads branding.at("codename") and wraps it.
  assert.ok(
    /#emph\s*\[\s*#branding\.at\(\s*"codename"/.test(RECORD_CARD),
    'record_card.typ footer must wrap #branding.at("codename", ...) in #emph()',
  );
});

test('pdf-02 article_landscape.typ declares codename let-binding and wraps in #emph()', () => {
  assert.ok(
    /#let codename\s*=\s*if "codename" in branding/.test(ARTICLE_LANDSCAPE),
    'article_landscape.typ must declare `#let codename = if "codename" in branding { ... }`',
  );
  assert.ok(
    /#emph\s*\[\s*#codename\s*\]/.test(ARTICLE_LANDSCAPE),
    'article_landscape.typ footer must wrap #codename in #emph()',
  );
});

test('pdf-03 article_portrait.typ declares codename let-binding and wraps in #emph()', () => {
  assert.ok(
    /#let codename\s*=\s*if "codename" in branding/.test(ARTICLE_PORTRAIT),
    'article_portrait.typ must declare `#let codename = if "codename" in branding { ... }`',
  );
  assert.ok(
    /#emph\s*\[\s*#codename\s*\]/.test(ARTICLE_PORTRAIT),
    'article_portrait.typ footer must wrap #codename in #emph()',
  );
});

test('pdf-04 event_landscape.typ declares codename let-binding and wraps in #emph()', () => {
  assert.ok(
    /#let codename\s*=\s*if "codename" in branding/.test(EVENT_LANDSCAPE),
    'event_landscape.typ must declare `#let codename = if "codename" in branding { ... }`',
  );
  assert.ok(
    /#emph\s*\[\s*#codename\s*\]/.test(EVENT_LANDSCAPE),
    'event_landscape.typ footer must wrap #codename in #emph()',
  );
});

test('pdf-05 event_portrait.typ declares codename let-binding and wraps in #emph()', () => {
  assert.ok(
    /#let codename\s*=\s*if "codename" in branding/.test(EVENT_PORTRAIT),
    'event_portrait.typ must declare `#let codename = if "codename" in branding { ... }`',
  );
  assert.ok(
    /#emph\s*\[\s*#codename\s*\]/.test(EVENT_PORTRAIT),
    'event_portrait.typ footer must wrap #codename in #emph()',
  );
});

// ----- CLI / OS-title safety -----
const VERSIONINFO = readFileSync(join(ROOT, 'internal', 'versioninfo', 'versioninfo.go'), 'utf8');
const MAIN_GO = readFileSync(join(ROOT, 'main.go'), 'utf8');

test('cli-safety-01 internal/versioninfo/versioninfo.go constant is still plain string', () => {
  assert.ok(
    /var CurrentReleaseName\s*=\s*"First Manassas"/.test(VERSIONINFO),
    'internal/versioninfo/versioninfo.go must still declare `var CurrentReleaseName = "First Manassas"` as a plain string (no HTML wrapping at the constant level)',
  );
  // The constant value should not carry <em> tags.
  assert.ok(
    !/<em>/i.test(VERSIONINFO),
    'internal/versioninfo/versioninfo.go must NOT carry <em> tags anywhere — CLI output stays plain text',
  );
});

test('cli-safety-02 main.go does NOT wrap codename in <em>', () => {
  assert.ok(
    !/<em>/i.test(MAIN_GO),
    'main.go must NOT carry <em> tags — the CLI --version path (line 41) and the Wails OS Title bar (line 195) stay plain text',
  );
});

console.log(`\nResults: ${pass} pass, ${fail} fail`);
if (fail > 0) {
  process.exit(1);
}
process.exit(0);