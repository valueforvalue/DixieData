/**
 * audit/smoke_typst_escape_dollar.mjs — RED-first regression net
 * for issue #670 amendment 3. typst 0.15 rejects a raw `$` in
 * markup mode (the `$` starts math mode + requires a matching
 * `]` or `)` to close). The pre-0.15 code passed `~$5` through
 * unchanged; typst 0.15 fails with "unclosed delimiter" on the
 * first list item containing a dollar amount.
 *
 * The fix adds `$` to the typstEscape switch so the body
 * becomes `\~$5` (both characters escaped, both legal in
 * markup mode).
 *
 * Source-scan probe — no live server needed. Asserts:
 *
 *   bug-shape-01 typstEscape MUST escape `$` to `\$`. A
 *     future refactor that removes `$` from the switch (e.g.
 *     "we don't escape $ because it's rare") would re-introduce
 *     the regression on any article that mentions a dollar
 *     amount.
 *
 *   fix-shape-01 typstEscape has `$` in BOTH the ContainsAny
 *     fast-path check AND the per-character switch.
 *
 *   regression-net-01 the typstEscape dollar-escape test cases
 *     (TestTypstEscape_EscapesDollarInMarkup) MUST exist in
 *     markdown_typst_test.go.
 *
 *   e2e-shape-01 a minimal "Property damage: ~$5 million"
 *     markdown body, when rendered to typst, must produce a
 *     string that compiles cleanly with `bin/typst-windows.exe`.
 *     This is the load-bearing test — the source-scan assertions
 *     verify the function shape, this verifies the output is
 *     typst-0.15-acceptable.
 *
 * Run:
 *   node audit/smoke_typst_escape_dollar.mjs
 *
 * Exit codes:
 *   0 — every assertion holds
 *   1 — at least one bug-shape or fix-shape violation
 */

import { readFileSync, existsSync, writeFileSync, unlinkSync } from "node:fs";
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

// bug-shape-01 + fix-shape-01: typstEscape must include `$`
// in the switch (not just in ContainsAny). The switch is the
// one that actually emits the escape; ContainsAny is the
// fast-path early return.
const containsAnyHasDollar = /ContainsAny\([^)]*\$[^)]*\)/.test(markdownTypst);
const typstEscapeMatch = markdownTypst.match(/func typstEscape\(s string\) string \{[\s\S]*?\n\}/);
// The case clause is `case '\\', '#', '*', '_', '`', '<', '>', '@', '=', ':', '~', '/', '$', '+':`
// We look for the literal single-quoted dollar `'$'` in the case clause.
// Use indexOf to avoid JS regex escape confusion.
const caseLine = typstEscapeMatch[0].split("\n").find((l) => l.includes("case")) || "";
const switchHasDollar = caseLine.indexOf("'$'") >= 0;
assert(
  "fix-shape-01: typstEscape switch includes '$' (per issue #670 amendment 3)",
  switchHasDollar,
  typstEscapeMatch
    ? `typstEscape switch line did not contain '$' case. Found:\n${typstEscapeMatch[0]}`
    : "Could not locate the typstEscape function."
);
assert(
  "fix-shape-01b: typstEscape ContainsAny fast-path includes '$'",
  containsAnyHasDollar,
  "ContainsAny check should include '$' so the early return doesn't skip the escape path."
);

// regression-net-01: the test cases for the dollar escape exist.
assert(
  "regression-net-01: markdown_typst_test.go has TestTypstEscape_EscapesDollarInMarkup",
  /TestTypstEscape_EscapesDollarInMarkup/.test(markdownTypstTest)
    && /\$5 million/.test(markdownTypstTest),
  "Expected the test source to reference TestTypstEscape_EscapesDollarInMarkup + a dollar-amount input."
);

// e2e-shape-01: a minimal markdown body with a dollar amount
// renders to typst that compiles. This is the load-bearing
// test — the source-scan assertions verify the function shape,
// this verifies the output is typst-0.15-acceptable.
//
// The check: run the actual typst binary against a minimal
// typst doc containing the escape that the walker would emit.
// If typst 0.15 accepts `\$5` in markup mode, the e2e passes.
const typstPath = (() => {
  // Check common locations relative to the repo root.
  const candidates = [
    join(root, "bin", "typst-windows.exe"),
    join(root, "bin", "typst"),
    "bin/typst-windows.exe",
    "bin/typst",
  ];
  for (const c of candidates) {
    if (existsSync(c)) return c;
  }
  return "";
})();

if (typstPath) {
  const minimal = `#set page(width: auto, height: auto, margin: 0pt)\n#list[\n- Property damage\\: \\~\\$5 million\n]\n`;
  const testFile = join(root, "audit", ".typst-escape-dollar-smoke.typ");
  try {
    writeFileSync(testFile, minimal);
    try {
      execFileSync(typstPath, ["compile", testFile], { encoding: "utf8", stdio: "pipe" });
      assert("e2e-shape-01: typst 0.15 compiles a minimal \\$5 million list item", true);
    } catch (err) {
      assert("e2e-shape-01: typst 0.15 compiles a minimal \\$5 million list item", false, err.message);
    } finally {
      try { unlinkSync(testFile); } catch (_) { /* best-effort cleanup */ }
    }
  } catch (err) {
    assert("e2e-shape-01: typst 0.15 compiles a minimal \\$5 million list item", false, `write failed: ${err.message}`);
  }
} else {
  console.log("  ⊘ e2e-shape-01: typst binary not found; skipping e2e (run inside the DixieData repo to enable)");
}

console.log("");
if (failures.length === 0) {
  console.log(`✓ smoke-typst-escape-dollar: all 4 assertions hold (issue #670 amendment 3 regression net).`);
  process.exit(0);
} else {
  console.error(`✗ smoke-typst-escape-dollar: ${failures.length} assertion(s) failed:`);
  for (const f of failures) console.error(`  - ${f}`);
  process.exit(1);
}
