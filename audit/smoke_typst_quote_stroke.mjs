/**
 * audit/smoke_typst_quote_stroke.mjs — RED-first regression net for
 * issue #669. Typst 0.15 removed the `stroke:` argument from the
 * `#quote(...)` function; emitting the pre-0.15 shape fails the
 * build with "unexpected argument: stroke" and breaks /articles PDF
 * export for every article with a markdown blockquote.
 *
 * Source-scan probe — no live server needed. Asserts:
 *
 *   bug-shape-01 internal/records/markdown_typst.go (the Blockquote
 *     branch in the goldmark walker) MUST NOT emit a
 *     `#quote(block: true, stroke: ...)` call. The pre-0.15 shape
 *     is exactly this; the regression is the first thing a
 *     contributor restoring the old shape would reintroduce.
 *
 *   fix-shape-01 the Blockquote branch when a theme is wired MUST
 *     emit a `#block(inset: (left: 1em), stroke: (left: 2pt + rgb(...)))`
 *     + `#set par(first-line-indent: 0pt)` — the typst 0.15-compatible
 *     shape that preserves the left-border design (the
 *     `--theme-sepia` token) without using the removed `stroke:`
 *     arg on `#quote`.
 *
 *   fix-shape-02 the Blockquote branch when no theme is wired MUST
 *     emit a plain `#quote(block: true)[` with no `stroke:` arg.
 *     This is the back-compat path for callers that don't pass a
 *     theme.
 *
 *   scope-shape-01 only the Blockquote branch in markdown_typst.go
 *     is affected. Other typst-emitting code in the repo (the
 *     Article templates, the event templates) does not call
 *     `#quote(...)` — the markdown converter is the single source
 *     of blockquote markup. The probe asserts no other .go file
 *     emits a `#quote(..., stroke:` pattern.
 *
 *   regression-net-01 the markdown_typst_test.go test cases for
 *     the blockquote (issue #669) MUST exist so a future refactor
 *     of the walker doesn't silently drop the fix.
 *
 * Run:
 *   node audit/smoke_typst_quote_stroke.mjs
 *
 * Exit codes:
 *   0 — every assertion holds
 *   1 — at least one bug-shape or fix-shape violation
 */

import { readFileSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const root = join(__dirname, "..");

const failures = [];
function assert(name, condition, detail) {
  if (condition) {
    console.log(`  ✓ ${name}`);
  } else {
    console.error(`  ✗ ${name}${detail ? "\n      " + detail : ""}`);
    failures.push(name);
  }
}

const markdownTypst = readFileSync(join(root, "internal/records/markdown_typst.go"), "utf8");
const markdownTypstTest = readFileSync(join(root, "internal/records/markdown_typst_test.go"), "utf8");

// bug-shape-01: no live (non-comment) `#quote(block: true, stroke: ...`
// in the walker. The pre-0.15 shape is exactly this; the regression
// is the first thing a contributor restoring the old shape would
// reintroduce. Comments that mention the old shape (as the issue
// comment does) are fine — we strip line comments before matching.
const markdownTypstNoComments = markdownTypst
  .split("\n")
  .map((line) => {
    const i = line.indexOf("//");
    return i === -1 ? line : line.slice(0, i);
  })
  .join("\n");
assert(
  "bug-shape-01: no live #quote(block: true, stroke: ... in markdown_typst.go",
  !/#quote\(block: true,\s*stroke:/.test(markdownTypstNoComments),
  "Found a live (non-comment) pre-typst-0.15 quote+stroke call that would fail with 'unexpected argument: stroke'."
);

// fix-shape-01: themed path emits the typst 0.15-compatible shape.
// The branch lives inside the `if s.theme != nil && s.theme.BlockquoteBorder != ""` guard.
// The post-#670 amendment 2 fix uses `typstColorExpr` which
// returns `rgb("#hex")` for hex input (the rgb wrapper is
// needed because typst markup mode treats bare `#hex` as a
// function call). The emitted stroke arg is `stroke: (left: 2pt + rgb("#hex"))`.
const themedBlock = /s\.theme\s*!=\s*nil\s*&&\s*s\.theme\.BlockquoteBorder\s*!=\s*""\s*\{[\s\S]*?\}/m;
const themedMatch = markdownTypst.match(themedBlock);
assert(
  "fix-shape-01: themed blockquote branch emits #block + #set par(first-line-indent: 0pt) with rgb(\"#hex\") wrapper",
  themedMatch !== null
    && /#block\(inset:\s*\(left:\s*1em\),\s*stroke:\s*\(left:\s*2pt\s*\+\s*%\w\)/.test(themedMatch[0])
    && /typstColorExpr\(s\.theme\.BlockquoteBorder\)/.test(themedMatch[0])
    && /#set par\(first-line-indent:\s*0pt\)/.test(themedMatch[0])
    // Plus verify typstColorExpr returns rgb("#hex") for hex input.
    // The Go source uses the literal backslash-quote form.
    && /rgb\(\\"#%s\\"\)/.test(markdownTypst),
  themedMatch
    ? `Themed branch source:\n${themedMatch[0]}`
    : "Could not locate the themed blockquote branch."
);

// fix-shape-02: no-theme fallback emits plain #quote (no stroke: arg).
// The Blockquote branch is an if/else over `s.theme != nil && ... .BlockquoteBorder != ""`;
// the else clause must be plain `#quote(block: true)[` with no `stroke:`.
const blockquoteBranch = /case \*ast\.Blockquote:[\s\S]*?writeTextToBuf[\s\S]*?(?=\n\tcase |\n\tdefault:|\n\t}|\nfunc |\Z)/m;
const branchMatch = markdownTypst.match(blockquoteBranch);
assert(
  "fix-shape-02: no-theme blockquote branch emits plain #quote(block: true)[ with no stroke arg",
  branchMatch !== null
    && /#quote\(block: true\)\[/.test(branchMatch[0])
    // The else clause (after the themed if) must not mention stroke:
    // Use a positive lookbehind: the second occurrence of #quote lives
    // in the else clause, and it must be a plain one.
    && (() => {
        // Split the branch on 'else {' to isolate the fallback block.
        const elseIdx = branchMatch[0].lastIndexOf("else {");
        if (elseIdx === -1) return false;
        const elseBlock = branchMatch[0].slice(elseIdx);
        return !/stroke:/.test(elseBlock);
      })(),
  branchMatch
    ? `Branch source:\n${branchMatch[0]}`
    : "Could not locate the Blockquote branch."
);

// scope-shape-01: no live (non-comment, non-string-literal)
// `#quote(block: true, stroke:` in any other .go file. markdown_typst.go
// is the single source of blockquote markup; if a future contributor
// adds a second emitter (e.g. a raw-typst template that hand-rolls a
// quote) they MUST use the typst-0.15-compatible shape (or, better,
// delegate to the markdown converter).
import { execFileSync } from "node:child_process";
const grepPattern = "#quote(block: true, stroke:";
const candidateFiles = execFileSync(
  "grep",
  ["-rl", "--include=*.go", "-e", grepPattern, "."],
  { cwd: root, encoding: "utf8" }
)
  .trim()
  .split("\n")
  .filter(Boolean);
// For each candidate, re-scan and strip line comments + string
// literals (which may legitimately contain the old shape as a
// `mustNot` assertion or a documentation example) before matching.
const offendingFiles = candidateFiles.filter((rel) => {
  const src = readFileSync(join(root, rel), "utf8");
  // Strip "..." string literals (single + double + backtick) and
  // // line comments. Approximate; the file is not Go-parsed.
  const stripped = src
    .split("\n")
    .map((line) => {
      // 1) drop // line comments
      const i = line.indexOf("//");
      const code = i === -1 ? line : line.slice(0, i);
      // 2) drop "..." string contents (handles \" escapes)
      return code
        .replace(/"(?:\\.|[^"\\])*"/g, '""')
        .replace(/`[^`]*`/g, "``");
    })
    .join("\n");
  const re = new RegExp(grepPattern.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"));
  return re.test(stripped);
});
assert(
  "scope-shape-01: no other .go file emits #quote(block: true, stroke: ...",
  offendingFiles.length === 0,
  offendingFiles.length ? `Offending files:\n${offendingFiles.join("\n")}` : null
);

// regression-net-01: the test cases for the issue #669 blockquote fix exist.
// Asserts on the test source so a future refactor of the walker that
// drops the test cases will fail this probe.
assert(
  "regression-net-01: markdown_typst_test.go pins the typst-0.15-compatible blockquote shape",
  /issue\s*#669/.test(markdownTypstTest)
    && /blockquote uses #block/.test(markdownTypstTest)
    && /blockquote without theme falls back/.test(markdownTypstTest),
  "Expected the test source to reference issue #669 + the two blockquote cases (themed + unthemed)."
);

console.log("");
if (failures.length === 0) {
  console.log(`✓ smoke-typst-quote-stroke: all 5 assertions hold (issue #669 regression net).`);
  process.exit(0);
} else {
  console.error(`✗ smoke-typst-quote-stroke: ${failures.length} assertion(s) failed:`);
  for (const f of failures) console.error(`  - ${f}`);
  process.exit(1);
}
