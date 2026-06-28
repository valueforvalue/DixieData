// Full flow: navigate to /setup, configure identity, navigate to
// /settings, click Initialize Data, watch what happens next.

import { chromium } from "playwright";
import { spawn } from "node:child_process";
import { mkdirSync, rmSync } from "node:fs";

const PORT = 9879;
const DATA_DIR = "C:/Users/value/dixie-setup-flow2";

rmSync(DATA_DIR, { recursive: true, force: true });
mkdirSync(DATA_DIR, { recursive: true });

const server = spawn(
  "C:/Users/value/dixiedata-web-test.exe",
  ["-addr", `127.0.0.1:${PORT}`, "-scratch-dir", DATA_DIR],
  {
    env: { ...process.env, DIXIEDATA_DEBUG: "1" },
    stdio: ["ignore", "pipe", "pipe"],
  },
);
server.stderr.on("data", (chunk) => process.stderr.write(`[srv] ${chunk}`));
server.stdout.on("data", (chunk) => process.stdout.write(`[srv] ${chunk}`));

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
  // Verify server state.
  const r1 = await fetch(`http://127.0.0.1:${PORT}/`, { redirect: "manual" });
  console.log("GET / status:", r1.status, "location:", r1.headers.get("location"));
  const r2 = await fetch(`http://127.0.0.1:${PORT}/setup`, { redirect: "manual" });
  console.log("GET /setup status:", r2.status, "location:", r2.headers.get("location"));
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();
  const errors = [];
  page.on("pageerror", (err) => errors.push(err.message));
  page.on("response", (resp) => {
    if (resp.status() >= 300 && resp.status() < 400) {
      console.log(`[redirect ${resp.status()}] ${resp.url()} -> ${resp.headers().location}`);
    }
  });

  // Step 1: configure identity.
  console.log("step 1: navigate to /setup");
  await page.goto(`http://127.0.0.1:${PORT}/setup`);
  await page.locator('input[name="first_name"]').waitFor({ timeout: 5000 });
  await page.locator('input[name="first_name"]').fill("Test");
  await page.locator('input[name="middle_name"]').fill("M");
  await page.locator('input[name="last_name"]').fill("User");
  await page.locator('input[name="birth_year"]').fill("1980");
  await page.locator('button:has-text("Save Identity")').click();
  await page.waitForURL(/\/calendar/, { timeout: 10000 });
  console.log("step 1 done: url is", page.url());

  // Step 2: navigate to /settings, click Initialize Data.
  console.log("step 2: navigate to /settings");
  await page.goto(`http://127.0.0.1:${PORT}/settings`);
  await page.locator('input[name="confirmation_word"]').waitFor({ timeout: 5000 });
  await page.locator('input[name="confirmation_word"]').fill("INITIALIZE");
  await page.locator('button:has-text("Initialize Data")').click();
  await wait(2000);
  console.log("step 2 done: url is", page.url());

  // NOTE: dixiedata-web's scratch dir does NOT end in .dixiedata,
  // so initializeLocalData's guard REFUSES to clear the DB. The
  // click above succeeds (toast fires) but the data persists.
  // To simulate the user-facing reset we wipe the scratch dir
  // ourselves and restart the binary so setupRequired re-reads
  // the now-empty DB.
  console.log("step 2b: wiping scratch dir + restarting binary");
  const { rmSync, readdirSync } = await import("node:fs");
  server.kill();
  await wait(1000);
  for (const f of readdirSync(DATA_DIR)) {
    rmSync(`${DATA_DIR}/${f}`, { recursive: true, force: true });
  }
  // Restart
  const newServer = spawn(
    "C:/Users/value/dixiedata-web-test.exe",
    ["-addr", `127.0.0.1:${PORT}`, "-scratch-dir", DATA_DIR],
    { env: { ...process.env, DIXIEDATA_DEBUG: "1" }, stdio: ["ignore", "pipe", "pipe"] },
  );
  newServer.stderr.on("data", (chunk) => process.stderr.write(`[srv] ${chunk}`));
  await ready();

  // Step 3: try to navigate to /calendar — should redirect to /setup.
  console.log("step 3: navigate to /calendar");
  const calResp = await fetch(`http://127.0.0.1:${PORT}/calendar`, { redirect: "manual" });
  console.log("GET /calendar status:", calResp.status, "location:", calResp.headers.get("location"));
  await page.goto(`http://127.0.0.1:${PORT}/calendar`);
  await wait(500);
  console.log("step 3 done: url is", page.url());

  // Step 4: wait for htmx polling + everything to settle.
  await wait(8000);

  // Inspect network: did htmx fire /jobs/active?
  const networkLog = [];
  page.on("request", (req) => {
    if (req.url().includes("jobs/active")) {
      networkLog.push(req.url());
    }
  });

  await wait(2000);
  console.log("networkLog (jobs/active requests):", networkLog.length, networkLog.slice(0, 5));

  const counts = await page.evaluate(() => ({
    headers: document.querySelectorAll("header.top-shell").length,
    setupCards: document.querySelectorAll('[data-ui-id="page.setup"]').length,
    feedbackModals: document.querySelectorAll("#feedback-modal").length,
    bodies: document.querySelectorAll("body").length,
    mains: document.querySelectorAll("main").length,
    inputBox: (() => {
      const el = document.querySelector('input[name="first_name"]');
      return el ? { rect: el.getBoundingClientRect().toJSON(), visible: !!el.offsetParent } : null;
    })(),
    pageHeight: document.documentElement.scrollHeight,
  }));
  console.log("COUNTS:", JSON.stringify(counts, null, 2));

  await page.screenshot({ path: "audit/setup-flow-final.png", fullPage: true });
  console.log("screenshot: audit/setup-flow-final.png");

  // Step 5: try typing.
  try {
    const input = page.locator('input[name="first_name"]').first();
    await input.click({ timeout: 2000 });
    await input.type("Hello", { timeout: 2000 });
    const val = await input.inputValue();
    console.log("typed value:", JSON.stringify(val));
  } catch (e) {
    console.log("INTERACTION FAILED:", e.message.split("\n")[0]);
    exitCode = 2;
  }

  if (errors.length > 0) {
    console.log("PAGE ERRORS:", errors.slice(0, 5));
  }

  await browser.close();
} catch (e) {
  console.error("REPRO ERROR:", e.message.split("\n")[0]);
  exitCode = 1;
} finally {
  try { server.kill(); } catch {}
}

process.exit(exitCode);