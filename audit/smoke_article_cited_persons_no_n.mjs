/**
 * audit/smoke_article_cited_persons_no_n.mjs — RED-first regression
 * net for issue #663 (stray `n` glyph before each cited-person row
 * in Article PDF exports).
 *
 * Source-scan probe — no live server needed. Asserts:
 *
 *   bug-shape-01 templates/article_portrait.typ:141 (resolved branch)
 *     MUST NOT contain a literal `\n` inside the typst content-mode
 *     `[ ... ]` block. Content mode has no `\n` escape; typst emits
 *     the two glyphs `\` and `n` verbatim, producing the user-reported
 *     artifact ("DXD-00001 ... Looney nDXD-00002 ... Fletcher").
 *
 *   bug-shape-02 templates/article_portrait.typ:143 (unresolved branch)
 *     same rule — literal `\n` inside `[ ... ]` produces the artifact
 *     on the "⚠ Unknown: DXD-XXXXX" row variant.
 *
 *   fix-shape-01 the cited-persons loop in article_portrait.typ MUST
 *     contain a typst line-break primitive (`#linebreak()`,
 *     `#par[]`, `#v(...)`) inside the resolved/unresolved branches
 *     so each row gets a typst-evaluated break instead of two literal
 *     glyphs.
 *
 *   scope-shape-01 only one Article template ships the cited-persons
 *     block (article_portrait.typ). If a future landscape Article
 *     template is added it must follow the same fix-shape rule. The
 *     probe currently asserts ONLY article_portrait.typ — this is a
 *     guard against future-template regression rather than a missing
 *     fix.
 *
 *   scope-shape-02 event_landscape.typ + event_portrait.typ do NOT
 *     render the cited-persons block — they render linked Person
 *     Records as a typst `table()` (verified clean in the bug report).
 *     If either gains a `#for r in refs` content-mode loop with a
 *     `\n` inside, the bug-class regression net below still catches
 *     the literal-glyph shape across all typst templates.
 *
 *   bug-class-net     every .typ file under templates/ MUST NOT
 *     contain a literal `\n` inside a content-mode `[ ... ]` block
 *     inside a `#for` loop. The regex is structural enough to catch
 *     the same bug shape if it ever appears in a different template.
 *
 * Pattern mirrors audit/smoke_codename_italics.mjs (the issue #489
 * probe) which is the established source-scan regression net for
 * typst-template bugs.
 */

import { readFileSync, readdirSync, statSync } from 'node:fs';
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

const ARTICLE_PORTRAIT = readFileSync(
  join(ROOT, 'templates', 'article_portrait.typ'),
  'utf8',
);

// Extract the cited-persons `#for r in refs { ... }` loop body. The
// loop body is unique enough to anchor on (it's the only `#for r in
// refs` in the template) and gives us a tightly-scoped slice to
// assert against. Lines 134-145 in the current template. The regex
// anchors on the inner `}\s*\n\s*}` so the body slice stops at the
// end of the for-loop (not the if-resolved branch — the inner `}` is
// matched first because non-greedy + the trailing-newline anchor
// resolves on the first valid position).
const loopBodyMatch = ARTICLE_PORTRAIT.match(
  /#for\s+r\s+in\s+refs\s*\{([\s\S]*?)\}\s*\n\s*\}/,
);
assert.ok(
  loopBodyMatch,
  'article_portrait.typ must contain a `#for r in refs { ... }` cited-persons loop',
);
const LOOP_BODY = loopBodyMatch[1];

// ----- Bug-shape assertions (issue #663 directly) -----

test('bug-shape-01 resolved branch MUST NOT contain a literal `\\n` inside the content block', () => {
  // The resolved branch lives at article_portrait.typ:141:
  //   [#text(fill: theme.palette.text_primary)[*#did*] #h(0.5em) #name \n]
  // Extract the resolved branch (between `if resolved {` and the next `} else`)
  // and assert it contains no bare `\n`.
  const resolvedBranchMatch = LOOP_BODY.match(
    /if\s+resolved\s*\{([\s\S]*?)\}\s*else\s*\{/,
  );
  assert.ok(
    resolvedBranchMatch,
    'cited-persons loop must contain an `if resolved { ... } else { ... }` block',
  );
  const resolvedBranch = resolvedBranchMatch[1];
  // Strip `\n` only inside typst string literals (the only legitimate
  // use of `\n` in typst markup is inside a `"..."` or `raw("...")`
  // string — but the cited-persons block has no string literals, so a
  // bare `\n` anywhere in the branch is the bug.
  const bareBackslashN = /(^|[^"\\])\\n/;
  assert.ok(
    !bareBackslashN.test(resolvedBranch),
    `resolved branch contains a literal "\\n" outside a string literal — typst content mode emits the two glyphs verbatim, producing the "DXD-00001 ... nDXD-00002" artifact.\nBranch:\n${resolvedBranch}`,
  );
});

test('bug-shape-02 unresolved branch MUST NOT contain a literal `\\n` inside the content block', () => {
  // The unresolved branch lives at article_portrait.typ:143:
  //   [#text(fill: red)[⚠ Unknown: #did] \n]
  // The branch body is everything from `} else {` to the closing
  // content `]` (the `}` that closes the branch is outside the
  // LOOP_BODY slice). We anchor on `] \n      ` (the content close
  // + newline + indent) so we don't match the outer for-loop close.
  const unresolvedBranchMatch = LOOP_BODY.match(
    /}\s*else\s*\{([\s\S]*?)\]\s*$/,
  );
  assert.ok(
    unresolvedBranchMatch,
    'cited-persons loop must contain an `} else { ... ]` branch',
  );
  const unresolvedBranch = unresolvedBranchMatch[1];
  const bareBackslashN = /(^|[^"\\])\\n/;
  assert.ok(
    !bareBackslashN.test(unresolvedBranch),
    `unresolved branch contains a literal "\\n" outside a string literal — typst content mode emits the two glyphs verbatim, producing the "n⚠ Unknown: DXD-..." artifact.\nBranch:\n${unresolvedBranch}`,
  );
});

// ----- Fix-shape assertion -----

test('fix-shape-01 cited-persons loop MUST contain a typst break primitive inside the branches', () => {
  // The fix is `#linebreak()` (preferred, mirrors the markdown
  // converter at internal/records/markdown_typst.go:341), `#par[]`,
  // or `#v(<length>)`. Any of these three is acceptable; the
  // assertion accepts all three so the slice's reviewer can pick
  // the cleanest shape without breaking the regression net.
  const breakPrimitive = /#(linebreak\(\)|par\[\s*\]|v\([^)]*\))/;
  assert.ok(
    breakPrimitive.test(ARTICLE_PORTRAIT),
    'article_portrait.typ cited-persons loop must emit a typst break primitive (#linebreak(), #par[], or #v(...)) so each row gets a typst-evaluated break instead of two literal glyphs',
  );
});

// ----- Scope-shape assertions -----

test('scope-shape-01 only one Article template exists; the probe targets article_portrait.typ exclusively', () => {
  const templatesDir = join(ROOT, 'templates');
  const articleTemplates = readdirSync(templatesDir).filter((f) =>
    /^article.*\.typ$/.test(f),
  );
  assert.deepEqual(
    articleTemplates,
    ['article_portrait.typ'],
    `templates/ must contain exactly one Article PDF template (article_portrait.typ); found ${JSON.stringify(articleTemplates)}`,
  );
});

test('scope-shape-02 event_landscape.typ + event_portrait.typ do NOT render a `#for r in refs` content-mode loop', () => {
  const eventLandscape = readFileSync(
    join(ROOT, 'templates', 'event_landscape.typ'),
    'utf8',
  );
  const eventPortrait = readFileSync(
    join(ROOT, 'templates', 'event_portrait.typ'),
    'utf8',
  );
  // The cited-persons-bug shape is `#for r in refs { ... \n ... }`.
  // Event templates use a typst `table()` for linked Person Records
  // (verified clean at template landing); they should NOT have this
  // loop shape.
  const citedRefsLoop = /#for\s+r\s+in\s+refs\s*\{[\s\S]*?\\n[\s\S]*?\}/;
  assert.ok(
    !citedRefsLoop.test(eventLandscape),
    'event_landscape.typ must NOT contain a `#for r in refs { ... \\n ... }` loop — it renders linked Person Records via typst `table()`',
  );
  assert.ok(
    !citedRefsLoop.test(eventPortrait),
    'event_portrait.typ must NOT contain a `#for r in refs { ... \\n ... }` loop — it renders linked Person Records via typst `table()`',
  );
});

// ----- Bug-class net: scan every .typ file for the same shape -----

function walkTypFiles(dir) {
  const out = [];
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry);
    const st = statSync(full);
    if (st.isDirectory()) {
      out.push(...walkTypFiles(full));
    } else if (entry.endsWith('.typ')) {
      out.push(full);
    }
  }
  return out;
}

test('bug-class-net no .typ file contains a literal `\\n` inside a `#for ... { ... }` content-mode loop', () => {
  // The bug shape, generalized: any typst template that uses a
  // `#for X in Y { [ ... \n ... ] }` loop will emit the literal
  // `\n` glyphs in the PDF. This net walks every .typ file and
  // fails on any future regression of the same shape.
  const typFiles = walkTypFiles(join(ROOT, 'templates'));
  const bugShape = /#for\s+\w+\s+in\s+\w+\s*\{[\s\S]*?\[([\s\S]*?)\][\s\S]*?\}/g;
  const offenders = [];
  for (const path of typFiles) {
    const rel = path.slice(ROOT.length + 1);
    const body = readFileSync(path, 'utf8');
    bugShape.lastIndex = 0;
    let m;
    while ((m = bugShape.exec(body)) !== null) {
      const block = m[1];
      const bareBackslashN = /(^|[^"\\])\\n/;
      if (bareBackslashN.test(block)) {
        offenders.push(`${rel} — block: ${m[0].replace(/\s+/g, ' ').slice(0, 80)}…`);
      }
    }
  }
  assert.deepEqual(
    offenders,
    [],
    `the following .typ files contain the literal "\\n" bug shape inside a #for-loop content block:\n  ${offenders.join('\n  ')}`,
  );
});

console.log(`\nResults: ${pass} pass, ${fail} fail`);
if (fail > 0) {
  process.exit(1);
}
process.exit(0);