// audit/smoke_tag_merge.mjs — regression net for the
// /tags merge-survivor picker (issue #282 slice 2).
//
// The /tags row merge form's `<input type="number" name="survivor_id">`
// became a `<select name="survivor_id">` populated with every other
// tag in the archive, formatted as `{id} — {name} ({N} members)`,
// sorted by name. The handler contract is unchanged — `survivor_id=N`
// still merges the source row into the survivor row.
//
// This probe verifies end-to-end on a freshly-seeded archive:
//
//   1. Seed 5 soldiers (provides at least one Person Record to
//      attach tags to).
//   2. Boot dixiedata-web and open the first soldier's detail.
//   3. Attach two distinct tags to the same soldier — the
//      resulting two `/tags` rows each have MemberCount=1.
//   4. Load `/tags` and assert the picker shape:
//        a. Two rows render with `<select name="survivor_id">`.
//        b. Each picker has a default placeholder option.
//        c. The Source Tag row's picker excludes its own id and
//           includes the Survivor Tag row's id formatted as
//           `{id} — Surivor Tag (1 members)`.
//   5. Pick the Survivor Tag in the Source Tag row, click Merge.
//   6. POST succeeds (303 → /tags); the tag-management page
//      reloads to a single row whose MemberCount is 2.

const PORT = 9988;
const SCRATCH = "C:/Development/DixieData/.scratch/webmode-tag-merge";
const WEB_BIN = "C:/Development/DixieData/build/bin/dixiedata-web.exe";
const SEED_BIN = "C:/Development/DixieData/build/bin/seed-data.exe";

import { spawn } from "node:child_process";
import { existsSync, rmSync } from "node:fs";

if (!existsSync(WEB_BIN)) { console.error("missing", WEB_BIN); process.exit(2); }
if (!existsSync(SEED_BIN)) { console.error("missing", SEED_BIN); process.exit(2); }

rmSync(SCRATCH, { recursive: true, force: true });

const wait = (ms) => new Promise((r) => setTimeout(r, ms));
const cwd = process.cwd();

let pass = 0, fail = 0;
function record(name, ok, details = {}) {
  if (ok) { pass++; console.log(`  \u2713 ${name} (${JSON.stringify(details)})`); }
  else { fail++; console.log(`  \u2717 ${name} (${JSON.stringify(details)})`); }
}

async function ready(url) {
  for (let i = 0; i < 60; i++) {
    try { const r = await fetch(url); if (r.status >= 200 && r.status < 500) return; } catch {}
    await wait(500);
  }
  throw new Error(`server never came up: ${url}`);
}

let Playwright = null;
try { Playwright = await import("playwright"); }
catch (e) { console.error("playwright import failed:", e.message); process.exit(2); }

// Seed first (synchronous binary, then async server).
console.log("Seeding scratch archive with 5 soldiers...");
const seed = spawn(SEED_BIN, ["-data-dir", SCRATCH, "-soldiers", "5", "-reset"], {
  cwd, stdio: ["ignore", "inherit", "inherit"],
});
await new Promise((resolve, reject) => {
  seed.on("exit", (code) => code === 0 ? resolve() : reject(new Error(`seed exit ${code}`)));
});
record("seed-data-succeeded", true);

// Boot the web server on the scratch dir.
const server = spawn(WEB_BIN, ["-addr", `127.0.0.1:${PORT}`, "-scratch-dir", SCRATCH], {
  cwd, stdio: ["ignore", "pipe", "pipe"],
});
server.stderr.on("data", () => {});
server.on("exit", (code) => { if (code !== 0) console.log("    [server-exit]", code); });

try {
  await ready(`http://127.0.0.1:${PORT}/`);
  await wait(1500);

  const browser = await Playwright.chromium.launch({ headless: true });
  const ctx = await browser.newContext({ viewport: { width: 1600, height: 1200 } });
  const page = await ctx.newPage();
  page.on("pageerror", (err) => console.log("    [pageerror]", err.message));
  page.on("requestfailed", (req) => {
    if (!req.url().includes(`127.0.0.1:${PORT}`)) return;
    // The Option C JS submit handler redirects the page on
    // `data-dixie-submit`, which Playwright reports as the
    // in-flight POST being ABORTED — expected; not a failure.
    const text = req.failure()?.errorText || "";
    if (text.includes("ERR_ABORTED")) return;
    console.log("    [request-failed]", req.url(), text);
  });

  // Load dashboard to get a soldier id from the response or
  // fall back to a known-good link.
  console.log("\nStep 1: open a soldier detail + attach two tags");
  await page.goto(`http://127.0.0.1:${PORT}/`, { waitUntil: "networkidle" });
  await wait(500);

  // The Browse link in the top-nav is the deterministic
  // starting point. Issue #380 slice 2: Browse moved into
  // the Records mega-menu (More records group). Open the
  // mega-menu first, then read the Browse menuitem href.
  await page.locator('[data-mega-menu-trigger="layout.records.menu"]').click();
  await wait(300);
  const browseUrl = await page.evaluate(() => {
    const panel = document.querySelector('[data-mega-menu-panel="layout.records.menu"]');
    if (!panel) return null;
    const a = Array.from(panel.querySelectorAll('a[role="menuitem"]'))
      .find((x) => (x.textContent || "").trim() === "Filter");
    return a ? a.getAttribute("href") : null;
  });
  if (!browseUrl) {
    console.error("could not find Browse nav link");
    record("browse-nav-link-present", false);
    throw new Error("no browse link");
  }
  record("browse-nav-link-present", true, { href: browseUrl });
  await page.goto(`http://127.0.0.1:${PORT}${browseUrl}`, { waitUntil: "networkidle" });
  await wait(500);

  // Open the first soldier row in the browse list.
  const firstSoldierHref = await page.evaluate(() => {
    // Browse rows use data-browse-row-href on <tr>.
    const row = document.querySelector("tr[data-browse-row-href]");
    return row ? row.getAttribute("data-browse-row-href") : null;
  });
  record("first-soldier-row-found", !!firstSoldierHref, { href: firstSoldierHref });
  if (!firstSoldierHref) {
    await browser.close();
    throw new Error("no soldier row");
  }
  await page.goto(`http://127.0.0.1:${PORT}${firstSoldierHref}`, { waitUntil: "networkidle" });
  await wait(500);

  const url = page.url();
  record("on-soldier-detail-page", /\/soldiers\/\d+/.test(url), { url });

  // Attach a tag to the soldier currently displayed in the page.
  // Each tag lands on a different soldier so the post-merge
  // MemberCount on the survivor goes 1 → 2.
  async function attachTag(label) {
    await page.evaluate(() => {
      const d = Array.from(document.querySelectorAll("details")).find((x) =>
        /^\+ Add tag$/.test(x.querySelector("summary")?.textContent?.trim() || ""));
      if (d) d.open = true;
    });
    await wait(200);
    const formSel = `form[action$='/tags'][method='post']`;
    await page.locator(`${formSel} input[name='tag_name']`).fill(label);
    await page.locator(`${formSel} button[type='submit']`).click();
    await page.waitForLoadState("networkidle", { timeout: 5000 });
    await wait(300);
  }

  // Attach Source Tag to soldier 1.
  await attachTag("Source Tag");
  record("attached-source-tag", true);

  // Navigate back to /browse to pick up a second soldier so
  // post-merge MemberCount on Survivor Tag goes 1 → 2.
  await page.goto(`http://127.0.0.1:${PORT}/browse`, { waitUntil: "networkidle" });
  await wait(500);
  const secondRow = await page.evaluate(() => {
    const rows = Array.from(document.querySelectorAll("tr[data-browse-row-href]"));
    return rows.length >= 2 ? rows[1].getAttribute("data-browse-row-href") : null;
  });
  record("second-soldier-row-found", !!secondRow, { href: secondRow });
  if (!secondRow) {
    await browser.close();
    throw new Error("no second soldier row");
  }
  await page.goto(`http://127.0.0.1:${PORT}${secondRow}`, { waitUntil: "networkidle" });
  await wait(500);
  await attachTag("Survivor Tag");
  record("attached-survivor-tag", true);

  console.log("\nStep 2: load /tags + assert picker shape");
  await page.goto(`http://127.0.0.1:${PORT}/tags`, { waitUntil: "networkidle" });
  await wait(500);

  // Two rows, each with a <select name="survivor_id"> + a
  // default placeholder option <option value="" disabled selected>.
  const rowData = await page.evaluate(() => {
    const rows = Array.from(document.querySelectorAll("tr[id^='tag-row-']"));
    return rows.map((row) => {
      const id = row.id.replace("tag-row-", "");
      const select = row.querySelector("select[name='survivor_id']");
      const input = row.querySelector("input[name='survivor_id']");
      const options = select
        ? Array.from(select.querySelectorAll("option")).map((o) => ({
            value: o.getAttribute("value"),
            text: (o.textContent || "").trim(),
            selected: o.selected,
            disabled: o.disabled,
          }))
        : null;
      const memberCell = row.querySelectorAll("td")[1];
      return {
        tagId: id,
        hasSelect: !!select,
        hasInput: !!input,
        options,
        memberCount: memberCell ? Number(memberCell.textContent.trim()) : null,
      };
    });
  });
  record("two-tag-rows-rendered", rowData.length === 2, { rows: rowData.map((r) => r.tagId) });
  record("every-row-has-select", rowData.every((r) => r.hasSelect));
  record("no-row-has-legacy-input", rowData.every((r) => !r.hasInput));

  for (const r of rowData) {
    const placeholder = r.options?.find((o) => o.value === "");
    record(
      `placeholder-default-row-${r.tagId}`,
      placeholder && placeholder.selected === true && placeholder.disabled === true,
      { memberCount: r.memberCount }
    );
  }

  // Each picker's options exclude the source row's own id.
  for (const r of rowData) {
    const ids = r.options?.map((o) => o.value).filter(Boolean) || [];
    record(
      `picker-excludes-own-id-${r.tagId}`,
      !ids.includes(r.tagId),
      { own: r.tagId, pickerIds: ids }
    );
  }

  // Source Tag picker includes the Survivor Tag id formatted as
  // `{id} — Survivor Tag (1 members)`.
  const sourceRow = rowData.find((r) =>
    r.options?.some((o) => o.text === "Survivor Tag (1 members)" || o.text.includes("Survivor Tag"))
  ) || (rowData.length ? rowData[0] : null);
  // Pull the survivor option by its text containing "Survivor Tag".
  const survivorOpt = sourceRow?.options?.find((o) => /Survivor Tag/.test(o.text));
  record(
    "source-picker-lists-survivor-with-format",
    !!survivorOpt && /^\d+ \u2014 Survivor Tag \(1 members\)$/.test(survivorOpt.text),
    { option: survivorOpt }
  );

  console.log("\nStep 3: pick survivor + click Merge");
  // The source row is whichever does NOT list "Source Tag" (1
  // members) — we want the row to be the source row (whose own
  // picker excludes itself and includes "Survivor Tag"). Source
  // row's options don't include "Source Tag" as a label.
  // Easier: the survivorOpt.value is the survivor's id; the
  // source row id is the OTHER row's id.
  const survivorId = survivorOpt?.value;
  const sourceRowId = rowData.find((r) => r.tagId !== survivorId)?.tagId;
  record("identified-source-and-survivor", !!survivorId && !!sourceRowId, {
    sourceId: sourceRowId, survivorId,
  });

  // Pick the survivor value on the source row's select + submit
  // the merge form. We dispatch the submit event so the Option C
  // `data-dixie-submit` interceptor in app.js owns the redirect
  // via X-DixieData-Redirect (POST /tags/{src}/merge responds 303
  // with X-DixieData-Redirect: /tags). form.submit() bypasses that
  // event and lands on the form-action URL instead — we need the
  // event so the JS interceptor fires.
  await page.evaluate(({ sourceId, survivorId }) => {
    const row = document.getElementById(`tag-row-${sourceId}`);
    const select = row?.querySelector("select[name='survivor_id']");
    if (select) {
      select.value = survivorId;
      select.dispatchEvent(new Event("change", { bubbles: true }));
    }
    const form = row?.querySelector("form[action$='/merge']");
    if (form) form.dispatchEvent(new SubmitEvent("submit", { bubbles: true, cancelable: true }));
  }, { sourceId: sourceRowId, survivorId });
  await page.waitForLoadState("networkidle", { timeout: 5000 });
  await wait(400);

  // After Merge the page reloads with the source row gone.
  console.log("\nStep 4: assert post-merge state");
  const afterUrl = page.url();
  record("post-merge-on-tags-page", /\/tags/.test(afterUrl), { url: afterUrl });

  const afterRows = await page.evaluate(() => {
    const rows = Array.from(document.querySelectorAll("tr[id^='tag-row-']"));
    return rows.map((row) => {
      const id = row.id.replace("tag-row-", "");
      const memberCell = row.querySelectorAll("td")[1];
      return { tagId: id, memberCount: Number(memberCell?.textContent.trim() || 0) };
    });
  });
  record("source-row-removed", !afterRows.some((r) => r.tagId === sourceRowId), {
    after: afterRows,
  });
  const survivorAfter = afterRows.find((r) => r.tagId === survivorId);
  record("survivor-row-member-count-is-2", survivorAfter?.memberCount === 2, {
    survivor: survivorAfter,
  });

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
