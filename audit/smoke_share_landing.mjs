// audit/smoke_share_landing.mjs — regression net for
// issue #265 (Quick Actions + Recent Activity reorg)
// AND issue #284 (the sub-overview landing reorg that
// replaced the inline Export/Import/Google sections
// with a sub-overview of 4 navigate-to-subpage tiles).
// This probe verifies the user-visible state after both
// reorgs:
//
//   1. /share loads with no errors
//   2. Quick Actions section is present (id="panel.share.quick-actions")
//   3. Quick Actions has exactly 4 tiles (issue #284)
//   4. Quick Actions tiles are <a> elements (anchors, not divs)
//      with the expected labels (Export, Import, Share Queue, Sync)
//   5. The 4 tile hrefs navigate to the 3 dedicated subpages
//      + /share/queue (Export → /share/exports; Import →
//      /share/imports; Share Queue → /share/queue; Sync → /share/sync)
//   6. Recent Activity section is present
//      (id="panel.share.recent")
//   7. Recent Activity shows the empty-state copy when
//      no jobs exist (initial state on a fresh scratch)
//   8. The old inline #export-section + #import-section
//      anchors are GONE from the landing (issue #284 —
//      the sections moved to /share/exports + /share/imports)
//   9. Section ordering: Quick Actions + Recent + Support &
//      Diagnostics + (conditional) Merge Review
//   10. Resize to 480px → all sections are still readable
//       (no horizontal scroll on the body)

const PORT = 9993;
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
    try { const r = await fetch(`http://127.0.0.1:${PORT}/`); if (r.status >= 200 && r.status < 500) return; } catch {}
    await wait(500);
  }
  throw new Error("server never came up");
}

let pass = 0, fail = 0;
function record(name, ok, details = {}) {
  if (ok) { pass++; console.log(`  ✓ ${name} (${JSON.stringify(details)})`); }
  else { fail++; console.log(`  ✗ ${name} (${JSON.stringify(details)})`); }
}

let Playwright = null;
try { Playwright = await import("playwright"); }
catch (e) { console.error("playwright import failed:", e.message); process.exit(2); }

try {
  await ready();
  await wait(2000);

  const browser = await Playwright.chromium.launch({ headless: true });
  const ctx = await browser.newContext({ viewport: { width: 1600, height: 1200 } });
  const page = await ctx.newPage();
  page.on("pageerror", (err) => console.log("    [pageerror]", err.message));
  page.on("console", (msg) => {
    if (msg.type() === "error" || msg.text().includes("dispatch") || msg.text().includes("export")) {
      console.log(`    [${msg.type()}]`, msg.text());
    }
  });

  // === Step 1: load /share ===
  console.log("Step 1: load /share");
  await page.goto(`http://127.0.0.1:${PORT}/share`, { waitUntil: "networkidle" });
  await wait(1000);

  // === Step 2: Quick Actions section present ===
  console.log("\nStep 2: Quick Actions section present");
  const quickActions = await page.evaluate(() => {
    const section = document.getElementById("panel.share.quick-actions");
    if (!section) return null;
    const tiles = Array.from(section.querySelectorAll("a"));
    return {
      exists: true,
      tileCount: tiles.length,
      tiles: tiles.map((t) => ({ label: t.querySelector(".text-lg")?.textContent.trim(), href: t.getAttribute("href"), tag: t.tagName })),
    };
  });
  record("quick-actions-section-exists", quickActions && quickActions.exists === true, quickActions);
  record("quick-actions-has-4-tiles", quickActions && quickActions.tileCount === 4, { tileCount: quickActions && quickActions.tileCount });
  record("quick-actions-tiles-are-anchors", quickActions && quickActions.tiles.every((t) => t.tag === "A"), { tags: quickActions && quickActions.tiles.map((t) => t.tag) });
  record("quick-actions-tile-labels", quickActions && quickActions.tiles.map((t) => t.label).join("|") === "Export|Import|Share Queue|Sync", { labels: quickActions && quickActions.tiles.map((t) => t.label) });

  // === Step 3: tile hrefs (all 4 tiles are navigate-to-subpage
  // links per the locked decision in #284; the in-page anchor
  // + data-action submit model from #265 was replaced) ===
  console.log("\nStep 3: Quick Actions tile hrefs (issue #284)");
  const expectedHrefs = ["/share/exports", "/share/imports", "/share/queue", "/share/sync"];
  const actualHrefs = quickActions && quickActions.tiles.map((t) => t.href);
  record("tile-hrefs-navigate-to-subpages", actualHrefs && actualHrefs.join("|") === expectedHrefs.join("|"), { actualHrefs, expectedHrefs });

  // === Step 4: Recent Activity section present ===
  console.log("\nStep 4: Recent Activity section present");
  const recentSection = await page.evaluate(() => {
    const section = document.getElementById("panel.share.recent");
    if (!section) return null;
    return { exists: true, hasEmptyState: section.textContent.includes("No exports or imports yet"), hasRows: section.querySelectorAll("a[href^='/jobs/']").length > 0, rowCount: section.querySelectorAll("a[href^='/jobs/']").length };
  });
  record("recent-section-exists", recentSection && recentSection.exists === true, recentSection);
  // The section must render EITHER the empty state OR the
  // populated list — never neither (which would be a missing
  // render path) and never both (which would be a layout bug).
  // The .scratch dir is persistent across test runs, so the
  // initial state depends on whether the previous test fired
  // a job. Both states are valid; the assertion is on the
  // disjunction.
  record("recent-section-renders-empty-state-or-rows", recentSection && (recentSection.hasEmptyState !== recentSection.hasRows), { hasEmptyState: recentSection && recentSection.hasEmptyState, hasRows: recentSection && recentSection.hasRows, rowCount: recentSection && recentSection.rowCount });

  // === Step 5: old inline sections are GONE from /share (issue #284) ===
  // The Export & Backup (#export-section) and Import & Restore
  // (#import-section) inline cards moved to /share/exports +
  // /share/imports respectively. They must NOT be on the
  // landing anymore.
  console.log("\nStep 5: old inline sections removed from /share (issue #284)");
  const oldSections = await page.evaluate(() => ({
    exportSection: document.getElementById("export-section") !== null,
    importSection: document.getElementById("import-section") !== null,
    googleIntegration: document.body.textContent.includes("Connect Google Account"),
  }));
  record("inline-export-section-gone", oldSections.exportSection === false, oldSections);
  record("inline-import-section-gone", oldSections.importSection === false, oldSections);
  record("inline-google-integration-gone", oldSections.googleIntegration === false, oldSections);

  // === Step 6: section ordering — Quick Actions + Recent + Support & Diagnostics ===
  console.log("\nStep 6: section ordering");
  const order = await page.evaluate(() => {
    const quickActions = document.getElementById("panel.share.quick-actions");
    const recent = document.getElementById("panel.share.recent");
    const supportHeader = Array.from(document.querySelectorAll("p")).find((p) => p.textContent.trim() === "Support & Diagnostics");
    if (!quickActions || !recent) return null;
    return {
      quickActionsTop: quickActions.getBoundingClientRect().top,
      recentTop: recent.getBoundingClientRect().top,
      supportTop: supportHeader ? supportHeader.getBoundingClientRect().top : null,
    };
  });
  record("quick-actions-above-recent", order && order.quickActionsTop < order.recentTop, order);
  record("recent-above-support", order && order.supportTop !== null && order.recentTop < order.supportTop, order);

  // === Step 7: 480px responsive — the /share CONTENT does not
  // overflow horizontally. The top-nav may overflow at 480px
  // because it has 10+ items (Calendar, Search, Browse, Review
  // Queue, Insights, Share, Tags, Settings, Add Person Record)
  // and historically needs a more invasive responsive design to
  // fit on a phone screen. The /share LANDING content — the
  // Quick Actions + Recent Activity + the existing sections —
  // is the new surface this issue introduces, so we scope the
  // responsive check to the <main> element. A follow-up
  // responsive-nav ticket can tackle the top-nav itself. ===
  console.log("\nStep 7: 480px responsive check (main content only)");
  await page.setViewportSize({ width: 480, height: 800 });
  await wait(500);
  const noHorizontalScroll = await page.evaluate(() => {
    const main = document.querySelector("main");
    if (!main) return { error: "no <main> found" };
    const r = main.getBoundingClientRect();
    const offenders = [];
    for (const el of main.querySelectorAll("*")) {
      const er = el.getBoundingClientRect();
      if (er.right > r.right + 1) {
        offenders.push({ tag: el.tagName, id: el.id || null, class: el.className?.toString().slice(0, 80) || null, right: Math.round(er.right) });
        if (offenders.length > 5) break;
      }
    }
    return { mainWidth: Math.round(r.width), mainRight: Math.round(r.right), offenders, noOverflow: offenders.length === 0 };
  });
  record("share-main-no-horizontal-overflow-on-480px", noHorizontalScroll.noOverflow === true, noHorizontalScroll);
  await page.setViewportSize({ width: 1600, height: 1200 });
  await wait(500);

  // === Step 8: navigate to /share/exports via the Export tile
  // and verify the export-subpage loads (issue #284) ===
  console.log("\nStep 8: Export tile navigates to /share/exports (issue #284)");
  await page.goto(`http://127.0.0.1:${PORT}/share`, { waitUntil: "networkidle" });
  await wait(1000);
  const exportTile = page.locator("#panel\\.share\\.quick-actions a").first();
  await exportTile.click({ force: true, timeout: 5000 });
  await page.waitForURL(/\/share\/exports$/, { timeout: 5000 }).catch(() => null);
  const exportUrl = page.url();
  record("export-tile-navigates-to-exports-subpage", /\/share\/exports$/.test(exportUrl), { url: exportUrl });
  // Verify the /share/exports page carries the Export JSON
  // action (which used to be the in-landing quick action).
  const onExportsSubpage = await page.evaluate(() => ({
    hasExportJsonAction: Array.from(document.querySelectorAll("[data-action='/export/json']")).length > 0,
    hasBuildShareArchive: document.getElementById("build-share-archive") !== null,
    hasIncludeTagsCheckbox: document.querySelector("[data-share-include-tags]") !== null,
  }));
  record("exports-subpage-has-export-json-action", onExportsSubpage.hasExportJsonAction, onExportsSubpage);
  record("exports-subpage-has-build-share-archive", onExportsSubpage.hasBuildShareArchive, onExportsSubpage);
  record("exports-subpage-has-include-tags-checkbox", onExportsSubpage.hasIncludeTagsCheckbox, onExportsSubpage);

  // === Step 9: fire an export from /share/exports + verify
  // Recent activity populates on /share (issue #265) ===
  console.log("\nStep 9: fire an export from /share/exports + verify Recent activity populates");
  const requests = [];
  page.on("request", (req) => {
    if (req.url().includes("/export/") || req.url().includes("/jobs/")) {
      requests.push({ url: req.url(), method: req.method() });
    }
  });
  const postPromise = page.waitForRequest((req) => req.url().endsWith("/export/json") && req.method() === "POST", { timeout: 10000 }).catch(() => null);
  const exportBtn = page.locator("[data-action='/export/json']").first();
  await exportBtn.click({ force: true, timeout: 5000 });
  const postReq = await postPromise;
  record("export-json-button-posts-to-handler", postReq !== null, { url: postReq && postReq.url() });

  // Wait for the worker to finish (small archive → a few seconds).
  await wait(5000);
  // Go back to /share and check the Recent activity section.
  await page.goto(`http://127.0.0.1:${PORT}/share`, { waitUntil: "networkidle" });
  await wait(1500);
  const recentAfterExport = await page.evaluate(() => {
    const section = document.getElementById("panel.share.recent");
    if (!section) return null;
    return { rowCount: section.querySelectorAll("a[href^='/jobs/']").length, hasEmptyState: section.textContent.includes("No exports or imports yet") };
  });
  record("recent-section-shows-job-after-export", recentAfterExport && recentAfterExport.rowCount >= 1, recentAfterExport);
  record("recent-section-not-in-empty-state-after-export", recentAfterExport && recentAfterExport.hasEmptyState === false, { hasEmptyState: recentAfterExport && recentAfterExport.hasEmptyState });

  await browser.close();
  console.log(`\n${pass} passed, ${fail} failed`);
  process.exit(fail === 0 ? 0 : 1);
} catch (e) {
  console.error("FATAL", e);
  process.exit(2);
} finally {
  server.kill();
  await wait(500);
}