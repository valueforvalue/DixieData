// Regression net for issue #293 — verify that the CLI JSON
// output surfaces the v1.{U}.{N} shape (issue #266) explicitly,
// not just as a single opaque string. The probe drives
// `DixieData.exe debug dump --json` (CLI, not HTTP) and asserts:
//   1. `app_version` matches `^\d+\.\d+\.\d+$` (the new shape)
//   2. `update_flow_version` is a positive integer
//   3. `release_counter` is a positive integer
//   4. `migrate status --json` carries the same two new fields
// Pre-fix: the `app_version` field was the legacy v1.2.{N} string
// and the U/N axes were hidden inside that single string.
// Post-fix: every emit site carries explicit U + N so scripts
// can compare U without re-parsing the string.

import { spawn } from "node:child_process";
import { existsSync } from "node:fs";

const BIN = "C:/Development/DixieData/build/bin/DixieData.exe";

if (!existsSync(BIN)) { console.error("missing", BIN); process.exit(2); }

const wait = (ms) => new Promise((r) => setTimeout(r, ms));

async function run(args) {
  return new Promise((resolve, reject) => {
    const child = spawn(BIN, args, { stdio: ["ignore", "pipe", "pipe"] });
    let stdout = "", stderr = "";
    child.stdout.on("data", (b) => stdout += b.toString());
    child.stderr.on("data", (b) => stderr += b.toString());
    child.on("close", (code) => resolve({ code, stdout, stderr }));
    child.on("error", reject);
  });
}

let pass = 0, fail = 0;
function record(name, ok, details = {}) {
  if (ok) { pass++; console.log(`  ✓ ${name} (${JSON.stringify(details)})`); }
  else { fail++; console.log(`  ✗ ${name} (${JSON.stringify(details)})`); }
}

try {
  // Step 1: `debug dump --json` carries the new shape
  console.log("Step 1: debug dump --json carries v1.{U}.{N} + explicit axes");
  const dump = await run(["debug", "dump", "--json"]);
  record("dump-exit-0", dump.code === 0, { code: dump.code, stderr: dump.stderr.slice(0, 200) });
  let inv = null;
  try { inv = JSON.parse(dump.stdout); }
  catch (err) { record("dump-json-parse", false, { err: err.message }); throw err; }
  record("dump-json-parse", true, {});

  // app_version must be 3 dotted integers (legacy v1.2.N is also
  // 3 dotted integers; the test that distinguishes them is the
  // presence of the two explicit fields below).
  record("dump-app-version-shape",
    /^\d+\.\d+\.\d+$/.test(inv.app_version || ""),
    { app_version: inv.app_version });
  // U must be a positive integer exposed explicitly.
  record("dump-update-flow-version-positive-int",
    Number.isInteger(inv.update_flow_version) && inv.update_flow_version >= 1,
    { update_flow_version: inv.update_flow_version });
  // N must be a positive integer exposed explicitly.
  record("dump-release-counter-positive-int",
    Number.isInteger(inv.release_counter) && inv.release_counter >= 1,
    { release_counter: inv.release_counter });

  // Step 2: `migrate status --json` carries the same two new fields
  console.log("\nStep 2: migrate status --json carries the same explicit axes");
  const status = await run(["migrate", "status", "--json"]);
  record("status-exit-0", status.code === 0, { code: status.code, stderr: status.stderr.slice(0, 200) });
  let statusDoc = null;
  try { statusDoc = JSON.parse(status.stdout); }
  catch (err) { record("status-json-parse", false, { err: err.message }); throw err; }
  record("status-json-parse", true, {});
  record("status-app-version-shape",
    /^\d+\.\d+\.\d+$/.test(statusDoc.app_version || ""),
    { app_version: statusDoc.app_version });
  record("status-update-flow-version-positive-int",
    Number.isInteger(statusDoc.update_flow_version) && statusDoc.update_flow_version >= 1,
    { update_flow_version: statusDoc.update_flow_version });
  record("status-release-counter-positive-int",
    Number.isInteger(statusDoc.release_counter) && statusDoc.release_counter >= 1,
    { release_counter: statusDoc.release_counter });
} catch (err) {
  fail++; console.error("probe threw:", err.message);
}

console.log(`\n  ${pass} passed / ${fail} failed`);
process.exit(fail === 0 ? 0 : 1);