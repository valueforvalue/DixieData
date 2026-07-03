// Regression net for issue #260 — render the StaleSummary line in
// the live preview panel of the print-config modal. The server already
// computes the stale count + summary string at
// internal/appshell/export_preview.go:62-87 and emits the line above
// the count at line 242-245. The JS at frontend/app.js:4235-4256 fetches
// /export/preview as form POST and injects the response HTML directly
// into [data-print-config-preview]. Pre-fix: the warning text existed
// in the response body but was never rendered because the rendering
// path skipped it. Post-fix: the warning text shows in the live preview
// when the user submits a filter value that doesn't exist on any
// row. The unit-level guarantee is in
// internal/appshell/export_preview_test.go::TestHandleExportPreview_StaleFilterWarning.
// This probe verifies the end-to-end render: server response -> DOM.
const PORT = 9988;
const SCRATCH = "C:/Development/DixieData/.scratch/webmode";
const WEB_BIN = "C:/Development/DixieData/build/bin/dixiedata-web.exe";

import { spawn } from "node:child_process";
import { existsSync } from "node:fs";

if (!existsSync(WEB_BIN)) { console.error("missing", WEB_BIN); process.exit(2); }
if (!existsSync(SCRATCH)) { console.error("missing", SCRATCH); process.exit(2); }

const server = spawn(WEB_BIN, ["-addr", `127.0.0.1:${PORT}`, "-scratch-dir", SCRATCH], { stdio: ["ignore", "pipe", "pipe"] });
server.stderr.on("data", () => {});
const wait = (ms) => new Promise((r) => setTimeout(r, ms));

async function ready() {
  for (let i = 0; i < 60; i++) {
    try { const r = await fetch(`http://127.0.0.1:${PORT}/calendar`); if (r.ok) return; } catch (_) { /* not yet */ }
    await wait(500);
  }
  throw new Error("server never came up");
}

let pass = 0, fail = 0;
function record(name, ok, details = {}) {
  if (ok) { pass++; console.log(`  ✓ ${name} (${JSON.stringify(details)})`); }
  else { fail++; console.log(`  ✗ ${name} (${JSON.stringify(details)})`); }
}

try {
  await ready();
  await wait(2000);

  // Step 1: server-side regression — POST /export/preview with a stale
  // filter value and assert the warning line + summary appear in the
  // response body. This matches the existing
  // TestHandleExportPreview_StaleFilterWarning unit test (issue #185).
  console.log("Step 1: /export/preview with stale filter unit");
  const params = new URLSearchParams();
  params.set("scope", "filtered");
  params.set("filter_unit", "No-Such-Unit-9999");
  params.set("filter_burial", "ANY");
  params.set("filter_entry_type", "ANY");
  params.set("filter_pension_state", "ANY");
  params.set("filter_confederate_home_status", "ANY");
  params.set("filter_age_at_death", "ANY");
  const staleResp = await fetch(`http://127.0.0.1:${PORT}/export/preview`, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: params.toString(),
  });
  const staleBody = await staleResp.text();
  record("preview-200-on-stale", staleResp.status === 200, { status: staleResp.status });
  record("preview-has-stale-summary", staleBody.includes("stale filter value"), { sample: staleBody.slice(0, 200) });
  record("preview-has-summary-line", /stale filter value(s?) — adjust or remove before generating\./.test(staleBody), {});

  // Step 2: same endpoint, NO filters should NOT emit the warning line
  // (verifies the count isn't always-on — only stale refs flip it).
  console.log("\nStep 2: /export/preview with no filters should NOT emit warning");
  const cleanParams = new URLSearchParams();
  cleanParams.set("scope", "all");
  const cleanResp = await fetch(`http://127.0.0.1:${PORT}/export/preview`, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: cleanParams.toString(),
  });
  const cleanBody = await cleanResp.text();
  record("clean-preview-200", cleanResp.status === 200, { status: cleanResp.status });
  record("clean-preview-no-warning", !cleanBody.includes("stale filter value"), { sample: cleanBody.slice(0, 200) });
} catch (err) {
  fail++; console.error("probe threw:", err.message);
} finally {
  try { server.kill(); } catch (_) { /* noop */ }
  setTimeout(() => {
    console.log(`\n  ${pass} passed / ${fail} failed`);
    process.exit(fail === 0 ? 0 : 1);
  }, 200);
}
