/**
 * audit/smoke_seed_markdown_articles.mjs — RED-first regression net
 * for issue #523 (--articles-format flag + markdown corpus).
 *
 * Source-scan probe — no live server needed. Asserts:
 *
 *   Seed package (5 assertions):
 *     1. internal/seed/seed.go declares ArticleBodyFormat enum with
 *        ArticleBodyPlain (zero value) + ArticleBodyMarkdown constants
 *        + String() method.
 *     2. internal/seed/seed.go carries articleMarkdownBodies corpus
 *        (>= 30 entries) and the corpus contains a markdown heading,
 *        a list, a fenced code block, and an image — kitchen-sink
 *        coverage pinned so a future refactor can't silently drop a
 *        feature from the operator fixture.
 *     3. internal/seed/seed.go Options struct has an
 *        ArticlesFormat ArticleBodyFormat field (with a comment).
 *     4. internal/seed/seed.go::seedArticles signature includes the
 *        format param and the markdown branch calls
 *        records.NewMarkdownRenderer().
 *     5. internal/seed/seed.go::seedArticles markdown branch writes
 *        the rendered HTML (NOT the <p>-wrapped legacy form).
 *
 *   Records package (1 assertion):
 *     6. internal/records/markdown.go::MarkdownRenderer is exported
 *        (so the seed package can import it). Without this export
 *        the slice cannot land.
 *
 *   CLI (2 assertions):
 *     7. cmd/seed-data/main.go registers a `--articles-format` flag
 *        accepting "plain" and "markdown".
 *     8. cmd/seed-data/main.go's articlesFormatFlag.Set rejects
 *        invalid values (paranoia: silent acceptance would let a
 *        typo land a plain-format run when the operator wanted
 *        markdown).
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

const SEED = readFileSync(join(ROOT, 'internal', 'seed', 'seed.go'), 'utf8');
const RECORDS_MD = readFileSync(join(ROOT, 'internal', 'records', 'markdown.go'), 'utf8');
const SEED_DATA_MAIN = readFileSync(join(ROOT, 'cmd', 'seed-data', 'main.go'), 'utf8');

// ----- Seed package -----

test('seed-01 ArticleBodyFormat enum + Plain/Markdown constants + String() method', () => {
  // Type declaration with iota + zero-value plain default.
  assert.ok(
    /type ArticleBodyFormat\s+int/.test(SEED),
    'must declare `type ArticleBodyFormat int`',
  );
  assert.ok(
    /ArticleBodyPlain\s+ArticleBodyFormat\s*=\s*iota/.test(SEED),
    'ArticleBodyPlain must be the zero value (iota, no explicit number)',
  );
  assert.ok(
    /ArticleBodyMarkdown/.test(SEED),
    'ArticleBodyMarkdown constant must be declared',
  );
  // String() method so fmt.Stringer works (the CLI flag uses it).
  assert.ok(
    /func \(f ArticleBodyFormat\) String\(\)\s+string/.test(SEED),
    'ArticleBodyFormat must have a String() method (fmt.Stringer contract)',
  );
  // The String() method should return "markdown" / "plain" literally.
  assert.ok(
    /return\s+"markdown"/.test(SEED) && /return\s+"plain"/.test(SEED),
    'String() method must return "markdown" / "plain" string literals',
  );
});

test('seed-02 articleMarkdownBodies corpus (>=30 entries) + kitchen-sink coverage', () => {
  // Locate the articleMarkdownBodies slice declaration.
  const sliceMatch = SEED.match(/articleMarkdownBodies\s*=\s*\[\]string\{([\s\S]*?)\n\s{0,4}\}/);
  assert.ok(
    sliceMatch !== null,
    'must declare `articleMarkdownBodies = []string{ ... }` slice',
  );
  const sliceBody = sliceMatch[1];
  // Count entries: each entry starts with `// <index>` comment OR
  // a top-level string literal on its own line. Use the comment
  // marker pattern (// 1. ... // 30. ...) that the corpus uses.
  const commentEntries = sliceBody.match(/^\t\t\/\/ \d+\./gm) || [];
  assert.ok(
    commentEntries.length >= 30,
    `articleMarkdownBodies must carry >= 30 entries; found ${commentEntries.length} // N. comments`,
  );

  // Kitchen-sink coverage pins (one entry per feature). The corpus
  // uses Go interpreted string literals so the source file carries
  // the literal characters "\n" (backslash + n) instead of actual
  // newlines — match the escaped form, not the raw form. Each entry
  // is one long line in the source file; the first token may sit
  // either right after the opening `"` or after a `\n`.
  const features = [
    { name: 'h1 heading', pattern: /(?:# |\\n# )/ },
    { name: 'h2 heading', pattern: /\\n## / },
    { name: 'unordered list', pattern: /\\n- / },
    { name: 'ordered list', pattern: /\\n\d+\. / },
    { name: 'blockquote', pattern: /\\n> / },
    { name: 'fenced code block', pattern: /```/ },
    { name: 'table', pattern: /\|.*\|/ },
    { name: 'link syntax', pattern: /\[[^\]]+\]\([^)]+\)/ },
    { name: 'image syntax', pattern: /!\[[^\]]*\]\([^)]+\)/ },
    { name: 'horizontal rule', pattern: /\\n---\\n/ },
  ];
  const missing = features.filter((f) => !f.pattern.test(sliceBody));
  assert.ok(
    missing.length === 0,
    `articleMarkdownBodies corpus missing kitchen-sink features: ${missing.map((f) => f.name).join(', ')}`,
  );

  // Image URLs must use public-domain Wikimedia Commons (not HTTP
  // non-public-domain URLs that might rot).
  assert.ok(
    /upload\.wikimedia\.org/.test(sliceBody),
    'image URLs in the corpus must reference upload.wikimedia.org (public domain)',
  );

  // Person Record tokens in the corpus must use out-of-range IDs
  // (DXD-999NN) so renderLinkedText falls back to literal text
  // without needing a real Person Record to resolve against.
  const tokenMatches = sliceBody.match(/\[\[DXD-\d{5}\]\]/g) || [];
  assert.ok(
    tokenMatches.length > 0,
    'corpus must use [[DXD-NNNNN]] Person Record tokens',
  );
  const outOfRange = tokenMatches.filter((t) => {
    const id = parseInt(t.match(/DXD-(\d+)/)[1], 10);
    return id >= 99900 && id <= 99999;
  });
  assert.ok(
    outOfRange.length === tokenMatches.length,
    `all Person Record tokens must use out-of-range IDs (DXD-99900..99999); found ${tokenMatches.length - outOfRange.length} in-range (would resolve against seeded soldiers and break the test)`,
  );
});

test('seed-03 Options.ArticlesFormat field with doc comment', () => {
  // The field carries a doc comment that explains the locked decision
  // (zero-value default = plain, opt-in markdown).
  assert.ok(
    /ArticlesFormat\s+ArticleBodyFormat/.test(SEED),
    'Options struct must have an ArticlesFormat ArticleBodyFormat field',
  );
  // Field must appear after Articles int field (grouping invariant).
  const articlesIdx = SEED.search(/\bArticles\s+int\b/);
  const formatIdx = SEED.search(/\bArticlesFormat\s+ArticleBodyFormat\b/);
  assert.ok(
    articlesIdx > 0 && formatIdx > articlesIdx,
    'ArticlesFormat field must follow the Articles int field in the Options struct',
  );
});

test('seed-04 seedArticles signature includes format param + markdown branch calls records.NewMarkdownRenderer()', () => {
  // Signature must include the format param.
  assert.ok(
    /func seedArticles\([^)]*format ArticleBodyFormat[^)]*\)/.test(SEED),
    'seedArticles signature must take a `format ArticleBodyFormat` parameter',
  );
  // Markdown branch must construct the renderer via the records
  // package (NOT redeclare a local renderer).
  assert.ok(
    /records\.NewMarkdownRenderer\(\)/.test(SEED),
    'markdown branch must call records.NewMarkdownRenderer() — the same pipeline the Wails app uses on save',
  );
});

test('seed-05 seedArticles markdown branch writes rendered HTML (NOT the legacy <p>-wrapped form)', () => {
  // Locate the seedArticles function body, then find the markdown
  // case branch inside it (NOT the ArticleBodyFormat.String()
  // method's switch, which also has a `case ArticleBodyMarkdown:`
  // returning "markdown").
  // Locate the seedArticles function body. The function ends with
  // `return nil\n}` (no trailing newline after the closing brace
  // in this codebase), so use a less greedy anchor.
  const seedArticlesMatch = SEED.match(/func seedArticles\([\s\S]*?\n\}\n?/);
  assert.ok(
    seedArticlesMatch !== null,
    'must have a seedArticles function body to inspect',
  );
  const fn = seedArticlesMatch[0];
  // Inside seedArticles, the switch is `switch format { case
  // ArticleBodyMarkdown: ... default: ... }`. Match greedily from
  // the markdown case to the default case.
  const markdownBranch = fn.match(/case ArticleBodyMarkdown:[\s\S]*?default:\s*\n/);
  assert.ok(
    markdownBranch !== null,
    'seedArticles must have a `case ArticleBodyMarkdown:` branch (inside the format switch, NOT the String() method switch)',
  );
  const branch = markdownBranch[0];
  assert.ok(
    /bodyHTML\s*=\s*rendered/.test(branch),
    'markdown branch must assign bodyHTML from the rendered HTML (`bodyHTML = rendered`)',
  );
  assert.ok(
    !/<p>"\s*\+\s*bodyMD\s*\+\s*"<\/p>"/.test(branch),
    'markdown branch must NOT use the legacy `<p>` + bodyMD + `</p>` form',
  );
});

// ----- Records package -----

test('records-md-01 internal/records/markdown.go::MarkdownRenderer is exported', () => {
  assert.ok(
    /^type MarkdownRenderer struct/m.test(RECORDS_MD),
    'MarkdownRenderer must be exported (capital M) so the seed package can import it',
  );
  assert.ok(
    /^func NewMarkdownRenderer\(\)\s*\*MarkdownRenderer/m.test(RECORDS_MD),
    'NewMarkdownRenderer constructor must be exported',
  );
  assert.ok(
    /^func \(r \*MarkdownRenderer\) Render\(source string\) \(string, error\)/m.test(RECORDS_MD),
    'Render method must be exported (already was, but pin the signature)',
  );
});

// ----- CLI -----

test('cli-01 cmd/seed-data/main.go registers a --articles-format flag', () => {
  assert.ok(
    /flag\.Var\([^,]+,\s*"articles-format"/.test(SEED_DATA_MAIN) ||
      /flag\.StringVar\([^,]+,\s*"articles-format"/.test(SEED_DATA_MAIN),
    'cmd/seed-data must register a --articles-format flag (flag.Var or flag.StringVar)',
  );
});

test('cli-02 cmd/seed-data/main.go articlesFormatFlag accepts plain + markdown', () => {
  // The Set method must switch on the value and reject anything else.
  assert.ok(
    /case\s+"plain":/.test(SEED_DATA_MAIN) && /case\s+"markdown":/.test(SEED_DATA_MAIN),
    'articlesFormatFlag.Set must accept "plain" and "markdown"',
  );
  assert.ok(
    /return\s+fmt\.Errorf\(/.test(SEED_DATA_MAIN),
    'articlesFormatFlag.Set must return an error for unrecognized values (paranoia: silent acceptance would let a typo land a plain-format run when the operator wanted markdown)',
  );
  assert.ok(
    /invalid --articles-format/.test(SEED_DATA_MAIN),
    'error message must name the flag (`invalid --articles-format`) so the operator sees the failure source',
  );
});

console.log(`\nResults: ${pass} pass, ${fail} fail`);
if (fail > 0) {
  process.exit(1);
}
process.exit(0);