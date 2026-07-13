// Regression net for issue #292 + #529 — visual + WCAG contrast
// review for the Phase 2 token migration, extended in A4a to all
// 3 themes. Phase 1 added 6 palette tokens (#252, no visual change
// verified by md5sum on app.css). Phase 2 wired those tokens into
// literal sites. Phase 3 measures the WCAG ratio for each surface
// touched by Phase 2 and locks the invariant against future drift.
// Slice A4a (issue #529) extends the probe to all 3 themes:
//   - default (gold/sepia, ThemeClassic)
//   - high-contrast (white/black, no decorative washes)
//   - soft (parchment/brown, ThemeSoft)
//
// The probe captures the user's current theme at start, flips to
// each of the 3 themes in turn via POST /settings/theme, runs the
// contrast assertions on /browse + /soldiers/{id}, then RESTORES
// the original theme at the end so a manual run doesn't leave a
// sticky theme change. Theme flips are logged so a failure can be
// reproduced by replaying the flip sequence.
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

// flipTheme POSTs the form-encoded theme to /settings/theme and
// waits for the response so the next navigation picks up the new
// theme. The /settings/theme handler writes local_settings.json,
// so a 2xx + X-DixieData-Redirect response is the success signal.
async function flipTheme(page, theme) {
  const form = new URLSearchParams({ theme }).toString();
  const resp = await page.request.post(`http://127.0.0.1:${PORT}/settings/theme`, {
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    data: form,
    maxRedirects: 0,
  });
  // The handler may respond 200 + X-DixieData-Redirect, or 303
  // See Other on a native form post. Both are success.
  if (resp.status() < 200 || resp.status() >= 400) {
    throw new Error(`flipTheme(${theme}) status=${resp.status()}`);
  }
  console.log(`    [theme] flipped to ${theme} (status=${resp.status()})`);
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

  // Step 0: capture the user's current theme so we can restore it
  // at the end. /settings renders the Appearance card with the
  // checked radio; we read the data-theme attribute from <html>
  // because that is the source of truth the per-page surfaces
  // theme off of.
  console.log("Step 0: capture starting theme for restore-at-end");
  await page.goto(`http://127.0.0.1:${PORT}/browse`, { waitUntil: "domcontentloaded" });
  await wait(800);
  const startingTheme = await page.evaluate(() => document.documentElement.getAttribute("data-theme") || "default");
  console.log(`    starting theme: ${startingTheme}`);

  // Step 1: capture before/after screenshots for each route × theme
  // so a reviewer can compare without grep. The first pass
  // captures the current theme; the next two pass through the
  // alternate themes and re-shoot.
  console.log("\nStep 1: capture phase3 after screenshots (per theme)");
  const screenshotRoutes = ["/calendar", "/browse", "/share/exports", "/soldiers/1"];

  // The 3 themes the issue lists. The user's current theme is
  // excluded from the flip list so the restore step always
  // converges to a different value than the current one (avoids
  // a no-op flip that confuses the diff).
  const ALL_THEMES = ["default", "high-contrast", "soft"];
  const flipList = ALL_THEMES.filter((t) => t !== startingTheme);
  // Always include the starting theme as the baseline pass
  // (so the report has a "before flip" row + "after flip" rows
  // for every route).
  const themeSequence = [startingTheme, ...flipList];

  for (const theme of themeSequence) {
    // Flip if not the starting theme.
    if (theme !== startingTheme) {
      await flipTheme(page, theme);
      await wait(500);
    }
    console.log(`  theme: ${theme}`);
    for (const route of screenshotRoutes) {
      let gotoOk = true;
      try {
        await page.goto(`http://127.0.0.1:${PORT}${route}`, { waitUntil: "domcontentloaded", timeout: 8000 });
      } catch (_) {
        gotoOk = false;
      }
      if (!gotoOk) { record(`screenshot-${theme}-route${route}`, false, { note: "navigation failed; route may not exist on dev" }); continue; }
      await wait(1500);
      const safeName = route.replace(/[\/]/g, "_") || "root";
      const out = path.join(REPORT_DIR, `phase3-after-${theme}${safeName}.png`);
      await page.screenshot({ path: out, fullPage: false });
      record(`screenshot-${theme}-route${route}`, existsSync(out), { out });
    }
  }

  // Step 2: WCAG contrast ratios per theme. For each theme in
  // the sequence (already covered above), measure the same
  // surfaces Phase 2 touched: primary-button + secondary-button
  // + pill-link + field-input on /browse, ghost-link on
  // /soldiers/{id}. The flip happens once per theme; the
  // measurements run on the same theme the screenshots captured.
  console.log("\nStep 2: WCAG ratio audit per theme");

  for (const theme of themeSequence) {
    // Always explicitly flip — Step 1's loop ended in a
    // different theme (the last item in themeSequence), so the
    // first iteration here MUST re-apply the starting theme
    // before reading the page; otherwise theme-applied check
    // catches a stale theme from the previous loop.
    await flipTheme(page, theme);
    await wait(500);
    console.log(`  theme: ${theme}`);

    // /browse: primary-button + secondary-button + pill-link + field-input
    await page.goto(`http://127.0.0.1:${PORT}/browse`, { waitUntil: "domcontentloaded" });
    await wait(1500);

    // Verify the theme actually applied — if it didn't, every
    // measurement is stale and the per-theme rows are
    // meaningless. Read data-theme on the <html> element; it
    // must match the requested theme.
    const appliedTheme = await page.evaluate(() => document.documentElement.getAttribute("data-theme") || "default");
    if (appliedTheme !== theme) {
      record(`theme-applied-${theme}`, false, { applied: appliedTheme, expected: theme, note: "data-theme attribute did not update after flip; measurements would be stale" });
      continue;
    }
    record(`theme-applied-${theme}`, true, { applied: appliedTheme });

    const browseMeasurements = await page.evaluate(() => {
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
        const cs = getComputedStyle(el);
        const bgImage = cs.backgroundImage || "";
        const gradStart = bgImage.indexOf("linear-gradient(");
        if (gradStart >= 0) {
          let depth = 0;
          let gradEnd = -1;
          for (let i = gradStart + "linear-gradient(".length; i < bgImage.length; i++) {
            if (bgImage[i] === "(") depth++;
            else if (bgImage[i] === ")") {
              if (depth === 0) { gradEnd = i; break; }
              depth--;
            }
          }
          const gradientText = gradEnd >= 0 ? bgImage.slice(gradStart + "linear-gradient(".length, gradEnd) : "";
          const stops = [...gradientText.matchAll(/(?:rgba?|hsla?)\(\s*([\d.,\s]+)\s*\)/gi)].map((m) => parseRGB(`rgb(${m[1]})`)).filter(Boolean);
          if (stops.length >= 2) {
            const lightest = stops.reduce((acc, s) => lum(s) > lum(acc) ? s : acc, stops[0]);
            return lightest;
          }
        }
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
    for (const [name, c] of Object.entries(browseMeasurements)) {
      const fullName = `contrast-${theme}-${name}-on-browse`;
      if (!c.found) { record(fullName, false, { found: false, note: "selector not present on /browse" }); continue; }
      const threshold = c.large ? WCAG_AA_LARGE : WCAG_AA_BODY;
      const ok = c.ratio !== undefined && c.ratio >= threshold;
      record(fullName, ok, { ratio: c.ratio?.toFixed(2), threshold, fontWeight: c.fontWeight, fontSize: c.fontSize });
    }

    // /soldiers/1: ghost-link
    await page.goto(`http://127.0.0.1:${PORT}/soldiers/1`, { waitUntil: "domcontentloaded" });
    await wait(1500);
    const soldiersChecks = await page.evaluate(() => {
      function parseRGB(s) { const m = (s || "").match(/rgba?\(([^)]+)\)/); if (!m) return null; const parts = m[1].split(",").map((x) => parseFloat(x.trim())); return parts.length >= 3 ? parts : null; }
      function lum(c) { const [r, g, b] = c.map((v) => { v = v / 255; return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4); }); return 0.2126 * r + 0.7152 * g + 0.0722 * b; }
      function ratio(fg, bg) { const l1 = lum(fg), l2 = lum(bg); const [hi, lo] = l1 > l2 ? [l1, l2] : [l2, l1]; return (hi + 0.05) / (lo + 0.05); }
      function composedOverWhite(bg) { if (!bg) return null; const alpha = bg.length === 4 ? bg[3] : 1; return [Math.round(bg[0] * alpha + 255 * (1 - alpha)), Math.round(bg[1] * alpha + 255 * (1 - alpha)), Math.round(bg[2] * alpha + 255 * (1 - alpha))]; }
      function pickFont(el) { const cs = getComputedStyle(el); const fg = parseRGB(cs.color); return { fg, fontWeight: cs.fontWeight, fontSize: cs.fontSize }; }
      function isLarge(f) { const size = parseFloat(f.fontSize); const isBold = parseInt(f.fontWeight, 10) >= 700; return size >= 24 || (isBold && size >= 18.66); }
      function ratioFor(selector) {
        const el = document.querySelector(selector);
        if (!(el instanceof HTMLElement)) return { found: false };
        const fg = pickFont(el);
        if (!fg.fg) return { found: false };
        let cur = el; let bg = null;
        while (cur instanceof HTMLElement && !bg) {
          const bgRaw = parseRGB(getComputedStyle(cur).backgroundColor);
          if (bgRaw && bgRaw.length === 3) bg = bgRaw;
          else if (bgRaw && bgRaw.length === 4 && bgRaw[3] > 0) bg = composedOverWhite(bgRaw);
          cur = cur.parentElement;
        }
        if (!fg.fg || !bg) return { found: false };
        return { found: true, ratio: ratio(fg.fg, bg), fg: fg.fg, bg, large: isLarge(fg), fontWeight: fg.fontWeight, fontSize: fg.fontSize };
      }
      return {
        ghostLink: ratioFor(".ghost-link"),
      };
    });
    for (const [name, c] of Object.entries(soldiersChecks)) {
      const fullName = `contrast-${theme}-${name}-on-soldiers`;
      if (!c.found) { record(fullName, false, { found: false, note: "selector not present on /soldiers/1" }); continue; }
      const threshold = c.large ? WCAG_AA_LARGE : WCAG_AA_BODY;
      record(fullName, c.ratio >= threshold, { ratio: c.ratio?.toFixed(2), threshold, fontWeight: c.fontWeight });
    }
  }

  // Step 3: restore the user's original theme. Always run, even
  // on failure, so a manual run never leaves the dev scratch
  // archive in a sticky alternate theme.
  console.log(`\nStep 3: restore starting theme (${startingTheme})`);
  try {
    await flipTheme(page, startingTheme);
    record(`theme-restored-to-${startingTheme}`, true, { note: "user's starting theme restored" });
  } catch (err) {
    record(`theme-restored-to-${startingTheme}`, false, { error: err.message, note: "RESTORE FAILED — manually flip /settings/theme" });
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
