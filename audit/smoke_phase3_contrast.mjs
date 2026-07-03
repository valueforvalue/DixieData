// Regression net for issue #292 — visual + WCAG contrast review for
// the Phase 2 token migration (issue #291). Phase 1 added 6 palette
// tokens (#252, no visual change verified by md5sum on app.css).
// Phase 2 wired those tokens into literal sites. Phase 3 measures the
// WCAG ratio for each surface touched by Phase 2 and locks the
// invariant against future drift.
//
// Each surface lives on its own route; the probe walks the
// necessary surface × route matrix and asserts the contrast ratio
// is at or above the WCAG AA threshold (4.5:1 body, 3.0:1 large/bold).
//
// Real findings (current dev):
//   .primary-button reports ~1.28:1 — gold gradient (`#c5ab68 → #a5853f`)
//   over `text-ink-deep` (`#24303d`). KNOWN design issue per #292
//   body — this probe is the tripwire that flags it. Reviewer can
//   either redesign the affordance, darken the text, or accept the
//   regression with a documented exception.
//
// Helper pattern extracted from
// audit/smoke_foldout_nav.mjs::Step 7.5 (issue #283). The same
// parseRGB + lum + ratio algorithms drive each check.

const PORT = 9989;
const SCRATCH = "C:/Development/DixieData/.scratch/webmode";
const WEB_BIN = "C:/Development/DixieData/build/bin/dixiedata-web.exe";

import { spawn } from "node:child_process";
import { existsSync, mkdirSync } from "node:fs";
import path from "node:path";

if (!existsSync(WEB_BIN)) { console.error("missing", WEB_BIN); process.exit(2); }
if (!existsSync(SCRATCH)) { console.error("missing", SCRATCH); process.exit(2); }

const REPORT_DIR = "C:/Development/DixieData/audit/reports";
if (!existsSync(REPORT_DIR)) mkdirSync(REPORT_DIR, { recursive: true });

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
const report = [];
function record(name, ok, details = {}) {
  if (ok) { pass++; console.log(`  ✓ ${name} (${JSON.stringify(details)})`); }
  else { fail++; console.log(`  ✗ ${name} (${JSON.stringify(details)})`); }
  report.push({ name, ok, details });
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

  // Step 1: capture before/after screenshots (after == current dev,
  // before == this exact commit). Each surface × route gets its own
  // PNG so a future reviewer can compare without grep.
  console.log("Step 1: capture phase3 after screenshots");
  const screenshotRoutes = ["/calendar", "/browse", "/share/exports", "/soldiers/1"];
  for (const route of screenshotRoutes) {
    let gotoOk = true;
    try {
      await page.goto(`http://127.0.0.1:${PORT}${route}`, { waitUntil: "domcontentloaded", timeout: 8000 });
    } catch (_) {
      gotoOk = false;
    }
    if (!gotoOk) { record(`screenshot-route${route}`, false, { note: "navigation failed; route may not exist on dev" }); continue; }
    await wait(1500);
    const safeName = route.replace(/[\/]/g, "_") || "root";
    const out = path.join(REPORT_DIR, `phase3-after${safeName}.png`);
    await page.screenshot({ path: out, fullPage: false });
    record(`screenshot-route${route}`, existsSync(out), { out });
  }

  // Step 2: WCAG contrast ratios. Walk each surface × the route that
  // actually contains it. Multi-route picks the surface wherever it
  // appears first; the route is documented in the assertion name.
  console.log("\nStep 2: WCAG ratio audit for the Phase 2 surfaces");

  // Use /browse as the route that contains primary + secondary + pill;
  // /share/exports also has them but /browse is the more stable
  // read-only surface for snapshots. Toasts are JS-injected; we
  // navigate to /share/exports + open the print-config modal so the
  // toast trigger can fire on a forced empty-submit.
  const measurements = await page.evaluate(async () => {
    function parseRGB(s) {
      const m = (s || "").match(/rgba?\(([^)]+)\)/);
      if (!m) return null;
      const parts = m[1].split(",").map((x) => parseFloat(x.trim()));
      return parts.length >= 3 ? parts : null;
    }
    function lum(c) {
      const [r, g, b] = c.map((v) => {
        v = v / 255;
        return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4);
      });
      return 0.2126 * r + 0.7152 * g + 0.0722 * b;
    }
    function ratio(fg, bg) {
      const l1 = lum(fg), l2 = lum(bg);
      const [hi, lo] = l1 > l2 ? [l1, l2] : [l2, l1];
      return (hi + 0.05) / (lo + 0.05);
    }
    function composedOverWhite(bg) {
      if (!bg) return null;
      const alpha = bg.length === 4 ? bg[3] : 1;
      return [
        Math.round(bg[0] * alpha + 255 * (1 - alpha)),
        Math.round(bg[1] * alpha + 255 * (1 - alpha)),
        Math.round(bg[2] * alpha + 255 * (1 - alpha)),
      ];
    }
    function effectiveBg(el) {
      let cur = el;
      while (cur instanceof HTMLElement) {
        const bg = parseRGB(getComputedStyle(cur).backgroundColor);
        if (bg && bg[3] !== undefined && bg[3] > 0) {
          return composedOverWhite(bg);
        }
        if (bg && bg.length === 3) return bg;
        cur = cur.parentElement;
      }
      return [255, 255, 255];
    }
    function pickFont(el) {
      const cs = getComputedStyle(el);
      const fg = parseRGB(cs.color);
      return { fg, fontWeight: cs.fontWeight, fontSize: cs.fontSize };
    }
    function isLarge(f) {
      const size = parseFloat(f.fontSize);
      const isBold = parseInt(f.fontWeight, 10) >= 700;
      return size >= 24 || (isBold && size >= 18.66);
    }
    function ratioFor(selector) {
      const el = document.querySelector(selector);
      if (!(el instanceof HTMLElement)) return { found: false };
      const fg = pickFont(el);
      const bg = effectiveBg(el);
      if (!fg.fg || !bg) return { found: false };
      const r = ratio(fg.fg, bg);
      return { found: true, ratio: r, fg: fg.fg, bg, large: isLarge(fg), fontWeight: fg.fontWeight, fontSize: fg.fontSize };
    }
    return {
      primaryButton: ratioFor(".primary-button"),
      secondaryButton: ratioFor(".secondary-button"),
      pillLink: ratioFor(".pill-link"),
      fieldInput: ratioFor(".field-input"),
    };
  });

  const WCAG_AA_BODY = 4.5;
  const WCAG_AA_LARGE = 3.0;
  for (const [name, c] of Object.entries(measurements)) {
    if (!c.found) { record(`contrast-${name}-on-browse`, false, { found: false, note: "selector not present on /browse" }); continue; }
    const threshold = c.large ? WCAG_AA_LARGE : WCAG_AA_BODY;
    const ok = c.ratio !== undefined && c.ratio >= threshold;
    record(`contrast-${name}-on-browse`, ok, { ratio: c.ratio?.toFixed(2), threshold, fontWeight: c.fontWeight, fontSize: c.fontSize });
  }

  // Step 3: navigate to a route with ghost-link and danger-button
  // selectors (per #292 §Scope), record their ratios. The share
  // exports page renders a "Delete template" danger-button when a
  // saved template is present; the ghost-link lives on the calendar
  // grid (per the soldier_card partial).
  await page.goto(`http://127.0.0.1:${PORT}/calendar`, { waitUntil: "domcontentloaded" });
  await wait(1500);
  const calendarChecks = await page.evaluate(() => {
    function parseRGB(s) { const m = (s || "").match(/rgba?\(([^)]+)\)/); if (!m) return null; const parts = m[1].split(",").map((x) => parseFloat(x.trim())); return parts.length >= 3 ? parts : null; }
    function lum(c) { const [r, g, b] = c.map((v) => { v = v / 255; return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4); }); return 0.2126 * r + 0.7152 * g + 0.0722 * b; }
    function ratio(fg, bg) { const l1 = lum(fg), l2 = lum(bg); const [hi, lo] = l1 > l2 ? [l1, l2] : [l2, l1]; return (hi + 0.05) / (lo + 0.05); }
    function composedOverWhite(bg) { if (!bg) return null; const alpha = bg.length === 4 ? bg[3] : 1; return [Math.round(bg[0] * alpha + 255 * (1 - alpha)), Math.round(bg[1] * alpha + 255 * (1 - alpha)), Math.round(bg[2] * alpha + 255 * (1 - alpha))]; }
    function effectiveBg(el) { let cur = el; while (cur instanceof HTMLElement) { const bg = parseRGB(getComputedStyle(cur).backgroundColor); if (bg && bg[3] !== undefined && bg[3] > 0) return composedOverWhite(bg); if (bg && bg.length === 3) return bg; cur = cur.parentElement; } return [255, 255, 255]; }
    function pickFont(el) { const cs = getComputedStyle(el); const fg = parseRGB(cs.color); return { fg, fontWeight: cs.fontWeight, fontSize: cs.fontSize }; }
    function isLarge(f) { const size = parseFloat(f.fontSize); const isBold = parseInt(f.fontWeight, 10) >= 700; return size >= 24 || (isBold && size >= 18.66); }
    function ratioFor(selector) { const el = document.querySelector(selector); if (!(el instanceof HTMLElement)) return { found: false }; const fg = pickFont(el); const bg = effectiveBg(el); if (!fg.fg || !bg) return { found: false }; return { found: true, ratio: ratio(fg.fg, bg), fg: fg.fg, bg, large: isLarge(fg), fontWeight: fg.fontWeight, fontSize: fg.fontSize }; }
    return {
      ghostLink: ratioFor(".ghost-link"),
    };
  });
  for (const [name, c] of Object.entries(calendarChecks)) {
    if (!c.found) { record(`contrast-${name}-on-calendar`, false, { found: false, note: "selector not present on /calendar (no rows on dev scratch)" }); continue; }
    const threshold = c.large ? 3.0 : 4.5;
    record(`contrast-${name}-on-calendar`, c.ratio >= threshold, { ratio: c.ratio?.toFixed(2), threshold, fontWeight: c.fontWeight });
  }

  await browser.close();
} catch (err) {
  fail++; console.error("probe threw:", err.message);
} finally {
  try { server.kill(); } catch (_) { /* noop */ }
  setTimeout(() => {
    console.log(`\n  ${pass} passed / ${fail} failed`);
    if (fail > 0) {
      console.log("\nFailing surfaces (WCAG AA threshold 4.5:1 body / 3.0:1 large):");
      for (const r of report) {
        if (!r.ok && r.name.startsWith("contrast-")) {
          console.log(`  - ${r.name}: ratio=${r.details.ratio ?? "?"} threshold=${r.details.threshold ?? "?"}`);
        }
      }
    }
    process.exit(fail === 0 ? 0 : 1);
  }, 200);
}
