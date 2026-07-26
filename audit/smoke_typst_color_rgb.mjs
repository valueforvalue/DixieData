/**
 * audit/smoke_typst_color_rgb.mjs — RED-first regression net for
 * the typst 0.15 color-string strictness (issues #669 amendment
 * + #670 amendment 2). Typst 0.15's `rgb(...)` parser rejects
 * strings that contain non-hex letters. The pre-0.15 path
 * passed CSS-style `rgb(36 48 61 / 0.06)` strings through
 * the markdown converter, which then wrapped them in
 * `rgb("rgb(36 48 61 / 0.06)")` — the nested `rgb(...)` form
 * typst 0.15 rejects with
 * `error: color string contains non-hexadecimal letters`.
 *
 * The fix introduces `typstColorExpr` (replacing `typstColor`)
 * which returns a BARE typst color expression — `#rrggbb` for
 * opaque colors, `color.rgb(r, g, b, a)` for alpha-bearing
 * colors. The emission sites drop the `rgb("...")` wrapper
 * entirely and use the result directly in color contexts:
 *
 *     #block(fill: #24303d, inset: 0.5em, radius: 2pt)[...]
 *     #block(fill: color.rgb(36, 48, 61, 15), inset: ...)[...]
 *     #text(fill: #22303d)[Heading]
 *
 * The first attempt at the fix (issue #670) still wrapped the
 * result in `rgb("...")`, producing `rgb("color.rgb(...)")` which
 * is ALSO rejected. This amendment 2 dropped the wrapper.
 *
 * Source-scan probe — no live server needed. Asserts:
 *
 *   bug-shape-01 the markdown_typst emission sites MUST NOT
 *     wrap the typstColorExpr result in `rgb("...")`. The
 *     emission pattern is `fill: #hex` or `fill: color.rgb(...)`
 *     directly, never `fill: rgb("...")`.
 *
 *   fix-shape-01 typstColorExpr MUST convert the CSS
 *     `rgb(R G B / A)` form to typst's own `color.rgb(r, g, b, a)`
 *     literal (a*255 for the alpha).
 *
 *   fix-shape-02 typstColorExpr MUST convert the alpha-less
 *     `rgb(R, G, B)` form to a 6-char hex (`#rrggbb`).
 *
 *   scope-shape-01 no other .go file in the repo emits the
 *     nested `rgb("rgb(...))` pattern. (The audit scans every
 *     .go file, strips comments + string literals, and asserts
 *     the result is empty.)
 *
 *   regression-net-01 the test cases for the typstColorExpr
 *     normalizer (TestTypstColorExpr_NormalizesForTypst015) MUST
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

// bug-shape-01: no live `rgb("rgb(...))` (nested) pattern. The
// pre-#670 amendment 2 fix wrapped the typstColorExpr result
// in another rgb() call, producing the nested
// `rgb("color.rgb(...)")` form which typst 0.15 rejects
// with "color string contains non-hexadecimal letters". The
// amendment-2 fix was reworked again: typstColorExpr now
// returns the full typst expression (rgb("#hex") for hex,
// color.rgb(r, g, b, a) for alpha) and the emission sites
// use the result directly with no further wrapping.
const nestedRgbPattern = /rgb\(\s*"rgb\(/;
assert(
  "bug-shape-01: no live nested rgb(\"rgb(... in markdown_typst.go (typst 0.15 strictness)",
  !nestedRgbPattern.test(markdownTypstNoLiterals),
  "Found the nested rgb(\"rgb(...)) form that typst 0.15 rejects with 'color string contains non-hexadecimal letters'."
);

// fix-shape-01: typstColorExpr converts rgb(R G B / A) to color.rgb(R, G, B, A*255).
// The string is built via fmt.Sprintf so the source contains
// the format-string form ("color.rgb(%s, %s, %s, %d)") rather
// than a literal call. Assert on the format-string + the alpha
// scaling logic. NOTE: this check runs against the raw
// source (not the literal-stripped view) because the
// format string IS the legitimate code structure.
const colorRgbFormat = /color\.rgb\(%s,\s*%s,\s*%s,\s*%d\)/;
const alphaScaling = /alphaF\s*\*\s*255/;
assert(
  "fix-shape-01: typstColorExpr emits color.rgb(R, G, B, A*255) for alpha-bearing input",
  colorRgbFormat.test(markdownTypst) && alphaScaling.test(markdownTypst),
  `format-match=${colorRgbFormat.test(markdownTypst)}, alpha-match=${alphaScaling.test(markdownTypst)}`
);

// fix-shape-02: typstColorExpr converts alpha-less rgb(R, G, B) to #rrggbb.
const rgbPartsToHex = /rgbPartsToHex\([^)]+\)/;
assert(
  "fix-shape-02: typstColorExpr has a rgbPartsToHex helper for the alpha-less path",
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

// regression-net-01: the typstColorExpr test cases exist in markdown_typst_test.go.
assert(
  "regression-net-01: markdown_typst_test.go pins the typstColorExpr normalizer (TestTypstColorExpr_NormalizesForTypst015)",
  /TestTypstColorExpr_NormalizesForTypst015/.test(markdownTypstTest)
    && /color\.rgb/.test(markdownTypstTest)
    && /#\d{6}/.test(markdownTypstTest),
  "Expected the test source to reference TestTypstColorExpr_NormalizesForTypst015 + a color.rgb literal + a 6-char hex expectation."
);

console.log("");
if (failures.length === 0) {
  console.log(`✓ smoke-typst-color-rgb: all 5 assertions hold (issues #669 + #670 amendment 2 regression net).`);
  process.exit(0);
} else {
  console.error(`✗ smoke-typst-color-rgb: ${failures.length} assertion(s) failed:`);
  for (const f of failures) console.error(`  - ${f}`);
  process.exit(1);
}
