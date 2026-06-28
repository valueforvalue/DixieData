// Phase 1 feedback loop for /setup stacking bug.
// Boots the seeded dixiedata-web binary, navigates to /setup,
// counts how many times the layout header appears in the DOM,
// and reports whether the user-facing inputs are obscured.

import { chromium } from "playwright";
import { spawn } from "node:child_process";
import { mkdirSync } from "node:fs";

const PORT = 9877;
const DATA_DIR = "C:/Users/value/dixie-setup-data";

mkdirSync(DATA_DIR, { recursive: true });
// Wipe any previous run so we land in setup-required state.
const { rmSync } = await import("node:fs");
rmSync(DATA_DIR, { recursive: true, force: true });
mkdirSync(DATA_DIR, { recursive: true });

const server = spawn(
  "C:/Users/value/dixiedata-web-test.exe",
  ["-addr", `127.0.0.1:${PORT}`],
  { env: { ...process.env, DIXIEDATA_DATA_DIR: DATA_DIR }, stdio: ["ignore", "pipe", "pipe"] },
);

const wait = (ms) => new Promise((r) => setTimeout(r, ms));
async function ready() {
  for (let i = 0; i < 30; i++) {
    try {
      const r = await fetch(`http://127.0.0.1:${PORT}/`);
      if (r.status >= 200 && r.status < 500) return;
    } catch {}
    await wait(300);
  }
  throw new Error("server never came up");
}

let exitCode = 0;
try {
  await ready();
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();
  page.on("console", (msg) => {
    if (msg.type() === "error") console.log(`[console.${msg.type()}]`, msg.text());
  });
  page.on("pageerror", (err) => console.log(`[pageerror]`, err.message));

  // Direct navigation to /setup (server returns 303 to /setup on /)
  await page.goto(`http://127.0.0.1:${PORT}/setup`);
  // Give htmx + the jobs-progress-overlay poll time to fire.
  await wait(2500);

  // Count repeated top-nav headers + setup cards + feedback modals.
  const counts = await page.evaluate(() => ({
    headers: document.querySelectorAll("header.top-shell").length,
    setupCards: document.querySelectorAll('[data-ui-id="page.setup"]').length,
    feedbackModals: document.querySelectorAll("#feedback-modal").length,
    bodies: document.querySelectorAll("body").length,
    mains: document.querySelectorAll("main").length,
    firstNameInput: !!document.querySelector('input[name="first_name"]'),
    firstNameBox: document.querySelector('input[name="first_name"]')?.getBoundingClientRect()?.toJSON?.() ?? null,
  }));

  console.log("COUNTS", JSON.stringify(counts, null, 2));

  // Try to click + type in first_name to see if input actually works.
  const input = page.locator('input[name="first_name"]');
  const inputCount = await input.count();
  if (inputCount > 0) {
    const box = await input.first().boundingBox();
    console.log("input first_name box:", JSON.stringify(box));
    try {
      await input.first().click({ timeout: 2000 });
      await input.first().type("TestUser", { timeout: 2000 });
      const value = await input.first().inputValue();
      console.log("typed value:", JSON.stringify(value));
    } catch (e) {
      console.log("INTERACTION FAILED:", e.message);
      exitCode = 2;
    }
  } else {
    console.log("NO first_name INPUT FOUND");
    exitCode = 3;
  }

  await page.screenshot({ path: "audit/setup-stacking.png", fullPage: true });
  console.log("screenshot: audit/setup-stacking.png");

  await browser.close();
} catch (e) {
  console.error("REPRO ERROR:", e);
  exitCode = 1;
} finally {
  server.kill();
}

process.exit(exitCode);