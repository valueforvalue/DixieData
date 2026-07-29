// Regression net for issue #532: article body markdown rendering.
// Pre-#532, the hand-rolled [data-article-body] CSS block in
// frontend/tailwind.css covered table/img/del/heading styles
// for the article detail page, but the preview modal uses a
// DIFFERENT selector ([data-article-preview-body]) so the same
// rules didn't apply -- tables rendered unstyled, images at
// intrinsic size, del without line-through. Issue #532 slice 1
// extends every [data-article-body] rule to ALSO apply to
// [data-article-preview-body] (CSS selector list), so both
// surfaces share the article body theme.
//
// This test source-scans frontend/tailwind.css and asserts:
//   1. Every rule that targets [data-article-body] also targets
//      [data-article-preview-body] (so the preview modal gets
//      the same typography + table + img + del + hr + blockquote
//      + code + pre + heading + list styling).
//   2. The HC override for article-body links also covers the
//      preview-body selector (the override was the easy place
//      to forget when extending the selector list).
//   3. The GFM task-list <input type="checkbox"> + <figure>/
//      <figcaption> additions are present (slice-1 added these
//      per the issue body).
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));
const tailwindPath = resolve(here, '..', 'frontend', 'tailwind.css');
const css = readFileSync(tailwindPath, 'utf8');

test('every [data-article-body] selector line is paired with [data-article-preview-body] (issue #532 slice 1)', () => {
  // Find all CSS selector lines that begin with
  // [data-article-body] (the article body theme rules) and
  // assert each is followed by , [data-article-preview-body]
  // before the { that opens the rule body.
  const selectorLineRe = /^\[data-article-body\][^\n{]*\{/gm;
  const matches = css.match(selectorLineRe) || [];
  assert.ok(matches.length > 20, `expected many [data-article-body] rules; found ${matches.length}`);
  for (const line of matches) {
    assert.match(
      line,
      /\[data-article-preview-body\]/,
      `selector line missing [data-article-preview-body] pairing: ${line.trim()}`,
    );
  }
});

test('HC override for article links covers both selectors (issue #532 slice 1)', () => {
  // The HC override at the bottom of the block must target
  // both [data-article-body] a and [data-article-preview-body] a
  // so a markdown link in the preview modal stays grayscale.
  assert.match(
    css,
    /html\[data-theme="high-contrast"\]\s+\[data-article-body\]\s+a[^{]*\{[^}]*color:\s*var\(--theme-text-primary\)/,
    'HC override missing for [data-article-body] a',
  );
  assert.match(
    css,
    /html\[data-theme="high-contrast"\]\s+\[data-article-preview-body\]\s+a[^{]*\{[^}]*color:\s*var\(--theme-text-primary\)/,
    'HC override missing for [data-article-preview-body] a',
  );
});

test('GFM task-list checkbox rule added to both selectors (issue #532 slice 1)', () => {
  // The checkbox rule covers <input type="checkbox"> inside <li>
  // (the GFM task-list shape goldmark + bluemonday emit).
  // Issue #694: the original regex expected the comma-grouped
  // form `[data-article-body], [data-article-preview-body]
  // li > input[type="checkbox"]` (one rule, both selectors
  // share the descendent). The canonical form in
  // frontend/tailwind.css is the alternate grouping:
  // `[data-article-body] li > input[type="checkbox"],
  // [data-article-preview-body] li > input[type="checkbox"]`
  // (two selectors, comma between full compounds). Both produce
  // identical rendered output. The regex now matches either
  // shape: full-compound pair in any order, OR the
  // comma-grouped shared-descendent form.
  assert.match(
    css,
    /\[data-article-body\][\s\S]*?li\s*>\s*input\[type="checkbox"\][\s\S]*?\[data-article-preview-body\][\s\S]*?li\s*>\s*input\[type="checkbox"\]|\[data-article-preview-body\][\s\S]*?li\s*>\s*input\[type="checkbox"\][\s\S]*?\[data-article-body\][\s\S]*?li\s*>\s*input\[type="checkbox"\]|\[data-article-body\],\s*\[data-article-preview-body\]\s+li\s*>\s*input\[type="checkbox"\]/,
    'task-list checkbox rule missing for [data-article-body] li > input',
  );
  assert.match(
    css,
    /\[data-article-body\][\s\S]*?li\s*>\s*p\s*>\s*input\[type="checkbox"\][\s\S]*?\[data-article-preview-body\][\s\S]*?li\s*>\s*p\s*>\s*input\[type="checkbox"\]|\[data-article-preview-body\][\s\S]*?li\s*>\s*p\s*>\s*input\[type="checkbox"\][\s\S]*?\[data-article-body\][\s\S]*?li\s*>\s*p\s*>\s*input\[type="checkbox"\]|\[data-article-body\],\s*\[data-article-preview-body\]\s+li\s*>\s*p\s*>\s*input\[type="checkbox"\]/,
    'task-list checkbox rule missing for [data-article-body] li > p > input',
  );
});

test('figure/figcaption rule added for image captions (issue #532 slice 1)', () => {
  assert.match(
    css,
    /\[data-article-body\]\s+figure[^{]*\{[^}]*margin:\s*0\.85rem\s+0/,
    'figure margin rule missing for [data-article-body]',
  );
  assert.match(
    css,
    /\[data-article-preview-body\]\s+figure[^{]*\{[^}]*margin:\s*0\.85rem\s+0/,
    'figure margin rule missing for [data-article-preview-body]',
  );
  assert.match(
    css,
    /\[data-article-preview-body\]\s+figcaption/,
    'figcaption rule missing for [data-article-preview-body]',
  );
});