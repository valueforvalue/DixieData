/**
 * audit/smoke_typst_color_rgb.mjs — RED-first regression net for
 * the typst 0.15 color-string strictness (issue #669 amendment).
 * Typst 0.15's `rgb(...)` parser rejects strings that contain
 * non-hex letters. The pre-0.15 path passed CSS-style
 * `rgb(36 48 61 / 0.06)` strings through the markdown converter,
 * which then wrapped them in `rgb("rgb(36 48 61 / 0.06)")` —
 * the nested `rgb(...)` form typst 0.15 rejects with
 * `error: color string contains non-hexadecimal letters`.
 *
 * Source-scan probe — no live server needed. Asserts:
 *
 *   bug-shape-01 internal/records/markdown_typst.go MUST NOT emit
 *     the nested `rgb("rgb(...)")` pattern. A future refactor of
 *     `typstColor` that returns a `rgb(...)` form unchanged (the
 *     pre-0.15 behavior) would re-introduce the bug class.
 *
 *   fix-shape-01 `typstColor` MUST convert the CSS `rgb(R G B / A)`
 *     form to typst's own `color.rgb(R, G, B, A*255)` literal so
 *     the outer `rgb("%s")` wrapper in the markdown_typst call
 *     sites produces a string typst 0.15 accepts.
 *
 *   fix-shape-02 `typstColor` MUST convert the alpha-less
 *     `rgb(R, G, B)` form to a 6-char hex (`#rrggbb`) so the
 *     outer wrapper has a simple hex string to wrap.
 *
 *   scope-shape-01 no other .go file in the repo emits the
 *     nested `rgb("rgb(...)")` pattern. (The audit scans every
 *     .go file, strips comments + string literals, and asserts
 *     the result is empty.)
 *
 *   regression-net-01 the test cases for the typstColor
 *     normalizer (TestTypstColor_NormalizesForTypst015) MUST
 *     exist in markdown_typst_test.go so a future refactor of
 *     the parser doesn't silently drop the conversions.
 *
 * Run:
 *   node audit/smoke_typst_color_rgb.mjs
 *
 * Exit codes:
 *   0 — every assertion holds
 *   1 — at least one bug-shape or fix-shape violation
 */

import { readFileSync } from "node:fs";
import { execFileSync } from "node:child_process";
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

// Strip line comments + string literals so a comment or test
// fixture that mentions the bad pattern doesn't trip the
// nested-rgb + scope-shape probes. Used by bug-shape-01
// and scope-shape-01 only — the fix-shape checks run
// against the raw source because the format-string form
// the Sprintf produces IS the legitimate code structure.
const markdownTypstNoLiterals = markdownTypst
  .split("\n")
  .map((line) => {
    const i = line.indexOf("//");
    const code = i === -1 ? line : line.slice(0, i);
    return code
      .replace(/"(?:\\.|[^"\\])*"/g, '""')
      .replace(/`[^`]*`/g, "``");
  })
  .join("\n");

// bug-shape-01: no live `rgb("rgb(` pattern. The pre-typst-0.15
// shape wrapped the result of `typstColor` in another `rgb()`
// call, producing `rgb("rgb(36 48 61 / 0.06)")` for the
// CodeBackground. The fix is in typstColor itself; the
// emission sites still wrap in rgb() but the wrapped string
// is now a typst-0.15-compatible literal (hex or color.rgb).
const nestedRgbPattern = /rgb\(\s*"rgb\(/;
assert(
  "bug-shape-01: no live rgb(\"rgb(... in markdown_typst.go (typst 0.15 nested-rgb rejection)",
  !nestedRgbPattern.test(markdownTypstNoLiterals),
  "Found the nested rgb(\"rgb(...)) form that typst 0.15 rejects with 'color string contains non-hexadecimal letters'."
);

// fix-shape-01: typstColor converts rgb(R G B / A) to color.rgb(R, G, B, A*255).
// The string is built via fmt.Sprintf so the source contains
// the format-string form ("color.rgb(%s, %s, %s, %d)") rather
// than a literal call. Assert on the format-string + the alpha
// scaling logic. NOTE: this check runs against the raw
// source (not the literal-stripped view) because the
// format string IS the legitimate code structure.
const colorRgbFormat = /color\.rgb\(%s,\s*%s,\s*%s,\s*%d\)/;
const alphaScaling = /alphaF\s*\*\s*255/;
assert(
  "fix-shape-01: typstColor emits color.rgb(R, G, B, A*255) for alpha-bearing input",
  colorRgbFormat.test(markdownTypst) && alphaScaling.test(markdownTypst),
  `format-match=${colorRgbFormat.test(markdownTypst)}, alpha-match=${alphaScaling.test(markdownTypst)}`
);

// fix-shape-02: typstColor converts alpha-less rgb(R, G, B) to #rrggbb.
const rgbPartsToHex = /rgbPartsToHex\([^)]+\)/;
assert(
  "fix-shape-02: typstColor has a rgbPartsToHex helper for the alpha-less path",
  rgbPartsToHex.test(markdownTypstNoLiterals),
  "Could not find the rgbPartsToHex helper. The alpha-less CSS rgb() path falls back to 6-char hex; the helper is the implementation."
);

// scope-shape-01: no other .go file emits the nested rgb("rgb(...))
// pattern. Same string-literal-stripping discipline as
// smoke_typst_quote_stroke.mjs scope-shape-01.
const nestedRgbGrep = execFileSync(
  "grep",
  ["-rl", "--include=*.go", "-e", 'rgb("rgb(', "."],
  { cwd: root, encoding: "utf8" }
)
  .trim()
  .split("\n")
  .filter(Boolean);
const nestedRgbFiles = nestedRgbGrep.filter((rel) => {
  const src = readFileSync(join(root, rel), "utf8");
  const stripped = src
    .split("\n")
    .map((line) => {
      const i = line.indexOf("//");
      const code = i === -1 ? line : line.slice(0, i);
      return code
        .replace(/"(?:\\.|[^"\\])*"/g, '""')
        .replace(/`[^`]*`/g, "``");
    })
    .join("\n");
  return /rgb\(\s*"rgb\(/.test(stripped);
});
assert(
  "scope-shape-01: no other .go file emits the nested rgb(\"rgb(...)) form",
  nestedRgbFiles.length === 0,
  nestedRgbFiles.length ? `Offending files:\n${nestedRgbFiles.join("\n")}` : null
);

// regression-net-01: the typstColor test cases exist in markdown_typst_test.go.
assert(
  "regression-net-01: markdown_typst_test.go pins the typstColor normalizer (TestTypstColor_NormalizesForTypst015)",
  /TestTypstColor_NormalizesForTypst015/.test(markdownTypstTest)
    && /color\.rgb/.test(markdownTypstTest)
    && /#\d{6}/.test(markdownTypstTest),
  "Expected the test source to reference TestTypstColor_NormalizesForTypst015 + a color.rgb literal + a 6-char hex expectation."
);

console.log("");
if (failures.length === 0) {
  console.log(`✓ smoke-typst-color-rgb: all 5 assertions hold (issue #669 amendment regression net).`);
  process.exit(0);
} else {
  console.error(`✗ smoke-typst-color-rgb: ${failures.length} assertion(s) failed:`);
  for (const f of failures) console.error(`  - ${f}`);
  process.exit(1);
}
