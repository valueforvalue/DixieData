// audit/smoke_share_queue_page.mjs — regression net for
// /share/queue page main list showing empty even when the
// pill count > 0. Root cause: renderShareQueuePage targeted
// the tbody (data-share-queue-page-body) and early-returned
// when it was missing. When the user navigates to /share/queue
// for the first time after staging items via [+ Queue], the
// server renders the empty-state branch (no tbody), the
// function silently no-ops, and the empty card stays on
// screen even though the pill says "Share queue: 3".
//
// Fix: target the section (id="panel.share-queue.list") which
// is always present. Fetch the page with the current ids and
// replace the section's inner contents. The install target
// shifts to the section too so event handlers + the
// installed-flag persist across re-renders. The probe covers
// 5 transitions: filled + empty + re-fill + remove-all + re-add.
//
// Run with: node audit/smoke_share_queue_page.mjs
// Exit code is non-zero when any assertion fails.

const PORT = 9956;
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
try {
  Playwright = await import("playwright");
} catch (e) {
  console.error("playwright import failed:", e.message);
  process.exit(2);
}

try {
  await ready();
  await wait(2000);

  const browser = await Playwright.chromium.launch({ headless: true });
  const ctx = await browser.newContext({ viewport: { width: 1600, height: 1200 } });
  const page = await ctx.newPage();

  // Listen for failed console + network errors.
  page.on("console", (msg) => {
    if (msg.type() === "error") console.log("    [console.error]", msg.text());
  });
  page.on("pageerror", (err) => console.log("    [pageerror]", err.message));

  console.log("Step 1: add 3 soldiers to share queue from /browse");
  await page.goto(`http://127.0.0.1:${PORT}/browse`, { waitUntil: "networkidle" });
  await page.evaluate(() => {
    const btns = document.querySelectorAll("[data-share-queue-add]");
    [0, 1, 2].forEach((i) => { if (btns[i]) btns[i].click(); });
  });
  await wait(500);
  const queue = await page.evaluate(() => localStorage.getItem("dixiedata.share-queue"));
  console.log("    localStorage dixiedata.share-queue:", queue);
  record("queue-has-3-items", queue !== null && JSON.parse(queue).length === 3, { queue });

  console.log("\nStep 2: navigate to /share/queue");
  await page.goto(`http://127.0.0.1:${PORT}/share/queue`, { waitUntil: "networkidle" });
  await wait(1500); // let renderShareQueuePage finish its fetch

  const pageState = await page.evaluate(() => {
    const pill = document.querySelector("[data-share-queue-pill]");
    const tbody = document.querySelector("[data-share-queue-page-body]");
    const rows = tbody ? tbody.querySelectorAll("[data-share-queue-page-row-id]") : [];
    const panelShare = document.querySelector("[id='panel.share-queue.list']");
    const bodyDirect = document.querySelector("body > *");
    // Dump the body innerHTML to see what's actually rendered.
    const allTbodies = document.querySelectorAll("tbody");
    const allElementsWithShareQueue = document.querySelectorAll("[data-share-queue-page-body], [data-share-queue-page-row-id], [data-share-queue-empty]");
    return {
      pillVisible: pill ? !pill.classList.contains("hidden") : false,
      pillText: pill ? pill.textContent.trim() : null,
      tbodyExists: tbody !== null,
      rowCount: rows.length,
      sampleRowId: rows[0] ? rows[0].getAttribute("data-share-queue-page-row-id") : null,
      url: window.location.href,
      panelShareExists: panelShare !== null,
      allTbodyCount: allTbodies.length,
      shareQueueAttrCount: allElementsWithShareQueue.length,
      bodyHtmlHead: document.body.innerHTML.substring(0, 600),
      bodyHtmlTail: document.body.innerHTML.substring(document.body.innerHTML.length - 1500),
    };
  });
  console.log("    /share/queue page state:", JSON.stringify(pageState, null, 2));

  record("page-tbody-exists", pageState.tbodyExists);
  record("page-has-rows", pageState.rowCount > 0, { rowCount: pageState.rowCount });
  record("pill-visible", pageState.pillVisible, { pillText: pageState.pillText });

  console.log("\nStep 3: fetch /share/queue?ids=N directly to inspect server response");
  const ids = JSON.parse(queue);
  const serverResp = await page.evaluate(async (idsList) => {
    const r = await fetch(`/share/queue?ids=${idsList.join(",")}`);
    const html = await r.text();
    const doc = new DOMParser().parseFromString(html, "text/html");
    const rows = doc.querySelectorAll("[data-share-queue-page-row-id]");
    const tbody = doc.querySelector("[data-share-queue-page-body]");
    return {
      status: r.status,
      bodyLen: html.length,
      tbodyPresent: tbody !== null,
      rowCount: rows.length,
      sampleRowHTML: rows[0] ? rows[0].outerHTML.substring(0, 200) : null,
    };
  }, ids);
  console.log("    server /share/queue?ids=N response:", JSON.stringify(serverResp, null, 2));
  record("server-renders-rows", serverResp.rowCount === ids.length, { serverRows: serverResp.rowCount, expected: ids.length });

  console.log("\nStep 4: clear localStorage queue, expect empty state on page");
  await page.evaluate(() => localStorage.removeItem("dixiedata.share-queue"));
  await page.reload({ waitUntil: "networkidle" });
  await wait(1500);
  const emptyState = await page.evaluate(() => {
    const section = document.querySelector("section[id='panel.share-queue.list']");
    const tbody = document.querySelector("[data-share-queue-page-body]");
    const emptyText = section ? section.textContent.includes("No Person Records staged") : false;
    return {
      tbodyExists: tbody !== null,
      emptyStateShown: emptyText,
      sectionInnerHTML: section ? section.innerHTML.substring(0, 300) : null,
    };
  });
  console.log("    empty state:", JSON.stringify(emptyState, null, 2));
  record("empty-state-after-clear", emptyState.emptyStateShown, { tbody: emptyState.tbodyExists });

  console.log("\nStep 5: re-add items, expect rows return after navigation");
  // Set localStorage then re-navigate to trigger a fresh
  // install + render. (renderShareQueuePage is in the IIFE
  // scope, not exposed on window, so we drive the lifecycle
  // via navigation instead of a direct call.)
  await page.evaluate(() => localStorage.setItem("dixiedata.share-queue", JSON.stringify([1, 2])));
  await page.goto(`http://127.0.0.1:${PORT}/share/queue`, { waitUntil: "networkidle" });
  await wait(1500);
  const reAddedState = await page.evaluate(() => {
    const tbody = document.querySelector("[data-share-queue-page-body]");
    const rows = tbody ? tbody.querySelectorAll("[data-share-queue-page-row-id]") : [];
    return { rowCount: rows.length, tbodyExists: tbody !== null };
  });
  console.log("    re-added:", JSON.stringify(reAddedState));
  record("rows-return-after-re-add", reAddedState.rowCount === 2, { rowCount: reAddedState.rowCount });

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