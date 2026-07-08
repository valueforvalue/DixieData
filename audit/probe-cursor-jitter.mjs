// Phase 1 feedback loop for /setup cursor jitter.
// Symptom: cursor rapidly swaps text<->pointer for ~30s after
// arriving on the credentials screen. User cannot click anything
// during the jitter. fd92f73 fixed the scrollbar-jump half; this
// probe targets the cursor-half.
//
// Method: park a synthetic mouse at the same (x,y) every frame
// and ask the page what element is under the cursor + what its
// computed `cursor` style is. Count how often the value changes
// in a 5s window. The probe asserts:
//   - swap count > threshold -> bug present (cursor is oscillating)
//   - element under cursor is stable (jitter is NOT from a
//     moving element, it's from re-rendering or layout shift)
//
// Boot the seeded dixiedata-web binary, navigate to /setup,
// then run the cursor sampler for 5s and print the swap count.

import { chromium } from "playwright";
import { spawn } from "node:child_process";
import { mkdirSync } from "node:fs";

// Fresh /tmp scratch dir per run. The binary migrates a clean
// DB on first boot; reusing stale scratch dirs under the user's
// home dir trips the "no such column: images.soldier_id"
// migration path. /tmp gives a clean slate.
const PORT = 9878;
const DATA_DIR = `C:/Users/value/AppData/Local/Temp/dixie-cursor-jitter-${Date.now()}`;

mkdirSync(DATA_DIR, { recursive: true });

const server = spawn(
  "C:/Users/value/dixiedata-web-test.exe",
  ["-addr", `127.0.0.1:${PORT}`, "-scratch-dir", DATA_DIR],
  { stdio: ["pipe", "pipe", "pipe"] },
);
server.stdout.on("data", (d) => process.stderr.write(`[srv.out] ${d}`));
server.stderr.on("data", (d) => process.stderr.write(`[srv.err] ${d}`));

const wait = (ms) => new Promise((r) => setTimeout(r, ms));
async function ready() {
  // First, wait until the server is even listening.
  for (let i = 0; i < 30; i++) {
    try {
      const r = await fetch(`http://127.0.0.1:${PORT}/`);
      if (r.status > 0) return;
    } catch {}
    await wait(300);
  }
  throw new Error("server never came up");
}

let exitCode = 0;
try {
  await ready();
// Diagnostic: hit /setup via raw fetch right before Playwright.
// Same binary, same port — but a different outcome. Log it.
for (let i = 0; i < 3; i++) {
  try {
    const r = await fetch(`http://127.0.0.1:${PORT}/setup`);
    const t = await r.text();
    console.log(`warmup ${i}: status=${r.status} length=${t.length} has_first_name=${t.includes("first_name")}`);
  } catch (e) {
    console.log(`warmup ${i} error:`, e.message);
  }
  await wait(500);
}
const browser = await chromium.launch({ headless: true });
const context = await browser.newContext({ bypassCSP: true });
await context.route("**/*", (route) => route.continue());
const page = await context.newPage();
  page.on("console", (msg) => {
    if (msg.type() === "error") console.log(`[console.${msg.type()}]`, msg.text());
  });
  page.on("pageerror", (err) => console.log(`[pageerror]`, err.message));

  // Navigate to /setup. Use `/` so the server's 303 -> /setup
  // chain runs through Playwright's redirect handling.
  let setupReady = false;
  for (let attempt = 0; attempt < 3; attempt++) {
    try {
      const resp = await page.goto(`http://127.0.0.1:${PORT}/setup`, {
        waitUntil: "domcontentloaded",
        timeout: 5000,
      });
      const url = page.url();
      const status = resp ? resp.status() : 0;
      console.log(`attempt ${attempt + 1}: /setup -> ${status}, url=${url}`);
      if (status === 200) {
        setupReady = true;
        break;
      }
      // Dump body for debugging.
      const body = await page.content();
      console.log(`body length: ${body.length}, head: ${body.slice(0, 200)}`);
    } catch (e) {
      console.log(`attempt ${attempt + 1} error:`, e.message);
    }
    await wait(500);
  }
  if (!setupReady) {
    console.log("setup page never returned 200");
    exitCode = 3;
    throw new Error("setup unreachable");
  }
  await wait(1500); // initial settle

  // Park the mouse at the dead center of the first_name input.
  // Inputs are .field-input; cursor over input = "text", over
  // surrounding card = "default" pointer. If the layout is
  // reflowing, the element under the cursor will change.
  const samplePoint = await page.evaluate(() => {
    const input = document.querySelector('input[name="first_name"]');
    if (!input) return null;
    const rect = input.getBoundingClientRect();
    // Sample 1px inside the right edge of the input — most
    // likely to land on the gap between input and adjacent
    // element during a reflow.
    return { x: rect.right - 1, y: rect.top + rect.height / 2 };
  });
  if (!samplePoint) {
    console.log("NO first_name input found");
    exitCode = 3;
    throw new Error("setup missing");
  }
  await page.mouse.move(samplePoint.x, samplePoint.y);

  // Sample for 5s. Track (elementTagName, elementId, computedCursor).
  const samples = await page.evaluate(async () => {
    const start = performance.now();
    const out = [];
    while (performance.now() - start < 5000) {
      const el = document.elementFromPoint(window.__sampleX, window.__sampleY);
      const cs = el ? window.getComputedStyle(el) : null;
      out.push({
        t: Math.round(performance.now() - start),
        tag: el ? el.tagName : null,
        cls: el ? (el.className || "").toString().slice(0, 60) : null,
        cursor: cs ? cs.cursor : null,
        bbox: el ? (() => {
          const r = el.getBoundingClientRect();
          return { x: Math.round(r.x), y: Math.round(r.y), w: Math.round(r.width), h: Math.round(r.height) };
        })() : null,
      });
      await new Promise((r) => requestAnimationFrame(r));
    }
    return out;
  }, samplePoint);

  // Summarise: count unique (tag, cls, bbox) sequences.
  const transitions = [];
  let prev = null;
  for (const s of samples) {
    const key = `${s.tag}|${s.cls}|${s.bbox ? `${s.bbox.x},${s.bbox.y},${s.bbox.w}x${s.bbox.h}` : ""}|${s.cursor}`;
    if (prev !== key) {
      transitions.push({ ...s, key });
      prev = key;
    }
  }
  const cursorSwaps = (() => {
    let n = 0;
    let prevCur = null;
    for (const s of samples) {
      if (prevCur !== null && prevCur !== s.cursor) n++;
      prevCur = s.cursor;
    }
    return n;
  })();

  console.log(`SAMPLES: ${samples.length} over 5s`);
  console.log(`TRANSITIONS: ${transitions.length}`);
  console.log(`CURSOR_SWAPS: ${cursorSwaps}`);
  console.log(`UNIQUE_TAGS: ${[...new Set(samples.map((s) => s.tag))].join(",")}`);
  console.log(`UNIQUE_CURSORS: ${[...new Set(samples.map((s) => s.cursor))].join(",")}`);
  console.log("FIRST 10 TRANSITIONS:");
  for (const t of transitions.slice(0, 10)) {
    console.log(`  t=${t.t}ms  ${t.tag}  cursor=${t.cursor}  bbox=${JSON.stringify(t.bbox)}`);
  }

  await browser.close();
} catch (e) {
  console.error("REPRO ERROR:", e);
  exitCode = 1;
} finally {
  server.kill();
}

process.exit(exitCode);