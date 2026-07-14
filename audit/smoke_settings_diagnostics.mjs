// Regression net for issue #255 — Support & Diagnostics card moved
// from /share to /settings. The card has the same two buttons
// (Export Feedback Log, Export Bug Report Bundle) at the same URLs
// (/export/feedback-log, /export/bug-report) but renders in the
// Settings page now, alongside Debug Mode, Data Quality Scan, etc.
// Pre-fix: clicking either button on /share fetched; no card on
// /settings. Post-fix: card is on /settings, NOT on /share; both
// endpoints still serve.

const PORT = 9990;
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

  // Step 1: /settings renders the Support & Diagnostics card
  console.log("Step 1: Support & Diagnostics card is on /settings");
  const settingsHtml = await (await fetch(`http://127.0.0.1:${PORT}/settings`)).text();
  record("settings-has-diagnostics-panel-id", settingsHtml.includes('id="settings-diagnostics-panel"'), {});
  record("settings-has-section-eyebrow", settingsHtml.includes("Support &amp; Diagnostics") || settingsHtml.includes("Support & Diagnostics"), {});
  record("settings-has-troubleshooting-bundle", settingsHtml.includes("Troubleshooting bundle"), {});
  record("settings-has-feedback-log-btn", settingsHtml.includes("Export Feedback Log"), {});
  record("settings-has-bug-report-btn", settingsHtml.includes("Export Bug Report Bundle"), {});
  record("settings-has-feedback-log-action", settingsHtml.includes('data-action="/export/feedback-log"'), {});
  // Issue #545 slice 3: the bug-report endpoint is reached via
  // a <form action="/export/bug-report"> carrying an
  // "Include images" checkbox, default checked.
  record("settings-has-bug-report-form-action", settingsHtml.includes('action="/export/bug-report"'), {});
  record("settings-has-include-images-checkbox", /name="include_images"[^>]*checked/.test(settingsHtml), {});
  record("settings-has-include-images-data-attr", settingsHtml.includes('data-include-images-checkbox="true"'), {});
  record("settings-has-bug-report-action", settingsHtml.includes('data-action="/export/bug-report"'), {});

  // Step 2: /share no longer renders the card (only the
  // eyebrow text appears in the page footer as a navigation
  // hint; the buttons themselves must not be there)
  console.log("\nStep 2: Support & Diagnostics card is NOT on /share");
  const shareHtml = await (await fetch(`http://127.0.0.1:${PORT}/share`)).text();
  record("share-no-troubleshooting-bundle", !shareHtml.includes("Troubleshooting bundle"), {});
  record("share-no-feedback-log-action", !shareHtml.includes('data-action="/export/feedback-log"'), {});
  record("share-no-bug-report-action", !shareHtml.includes('data-action="/export/bug-report"'), {});
  // Issue #545 slice 3: the bug-report endpoint is now a
  // <form action="/export/bug-report">, not a data-action
  // button. The form must NOT appear on /share either.
  record("share-no-bug-report-form-action", !shareHtml.includes('action="/export/bug-report"'), {});

  // Step 3: the endpoints still serve (not moved in routes.go,
  // just templ rendering moved). Driving them through fetch
  // confirms the handlers are wired and the path is unchanged.
  console.log("\nStep 3: endpoints still serve at the same URLs");
  for (const route of ["/export/feedback-log", "/export/bug-report"]) {
    const resp = await fetch(`http://127.0.0.1:${PORT}${route}`, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body: "",
      redirect: "manual",
    });
    // The async exports redirect via Option C 200 + X-DixieData-Redirect
    // (to /jobs/{id}) or the older 303 + Location. Either way, the
    // handler reached successfully (2xx/3xx), not 404/405.
    const ok = resp.status >= 200 && resp.status < 400;
    record(`endpoint${route}-serves`, ok, { status: resp.status });
  }
} catch (err) {
  fail++; console.error("probe threw:", err.message);
} finally {
  try { server.kill(); } catch (_) { /* noop */ }
  setTimeout(() => {
    console.log(`\n  ${pass} passed / ${fail} failed`);
    process.exit(fail === 0 ? 0 : 1);
  }, 200);
}
