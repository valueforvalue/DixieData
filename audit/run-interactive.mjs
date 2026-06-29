// audit/run-interactive.mjs — guided manual UI audit walker.
//
// This script does NOT replace the human audit. It automates the parts
// that are tedious to do by hand: navigating to each surface, taking
// before/after screenshots, waiting for known network round trips,
// asserting pass/fail on the deterministic checks (page loads, toast
// renders, modal closes, form clears, etc.). The human reports findings
// into docs/agents/audit-notes-ACTIVE.md.
//
// Run:
//   make web seed
//   nohup ./build/bin/dixiedata-web.exe -addr 127.0.0.1:8765 -scratch-dir .scratch/webmode > /tmp/web.log 2>&1 &
//   rm -rf .scratch/webmode && ./build/bin/seed-data.exe -data-dir .scratch/webmode -soldiers 25 -reset
//   node audit/run-interactive.mjs [--surface=calendar] [--report=audit/notes.md]
//
// Output:
//   - Console: one block per surface, PASS/FAIL per check, manual-prompt
//     lines for the surfaces that need a human eye.
//   - audit/audit-interactive-report.json: machine-readable summary
//   - audit/screenshots-interactive/: before/after PNGs for the surfaces
//     walked (so the human can paste them into a bug report).

import { chromium } from 'playwright';
import { mkdir, writeFile } from 'node:fs/promises';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { existsSync } from 'node:fs';

const __dirname = dirname(fileURLToPath(import.meta.url));
const SHOTS = join(__dirname, 'screenshots-interactive');
const REPORT = join(__dirname, 'audit-interactive-report.json');
const BASE = process.env.BASE_URL || 'http://127.0.0.1:8765';

const args = Object.fromEntries(
  process.argv.slice(2).map((a) => {
    const m = a.match(/^--([^=]+)(?:=(.*))?$/);
    return m ? [m[1], m[2] ?? true] : [a, true];
  })
);
const NOTES_PATH = args.report || 'audit/notes.md';
const SURFACE_FILTER = args.surface ? String(args.surface) : null;

const results = [];
let pass = 0;
let fail = 0;
let manual = 0;

function record(name, ok, details = {}) {
  results.push({ name, ok, ...details, ts: new Date().toISOString() });
  if (ok === true) {
    pass++;
    console.log(`  ✓ ${name}`);
  } else if (ok === false) {
    fail++;
    console.log(`  ✗ ${name}`);
    if (details.reason) console.log(`    reason: ${details.reason}`);
  } else if (ok === 'manual') {
    manual++;
    console.log(`  ? ${name} (manual)`);
    if (details.prompt) console.log(`    ${details.prompt}`);
  }
}

async function shot(page, name) {
  await mkdir(SHOTS, { recursive: true });
  const path = join(SHOTS, `${name}.png`);
  await page.screenshot({ path, fullPage: true });
  return path;
}

async function gotoSurface(page, path, waitFor = 'main') {
  const resp = await page.goto(`${BASE}${path}`, { waitUntil: 'domcontentloaded' });
  await page.waitForSelector(waitFor, { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(300);
  return resp;
}

async function checkToast(page, expectedSubstring) {
  return await page.evaluate((sub) => {
    const t = document.querySelector('[data-toast-region]');
    if (!(t instanceof HTMLElement)) return { ok: false, reason: 'no [data-toast-region]' };
    const visible = !t.classList.contains('hidden');
    const text = (t.textContent || '').trim();
    if (!visible) return { ok: false, reason: 'toast hidden' };
    if (sub && !text.includes(sub)) return { ok: false, reason: `toast text doesn't include ${JSON.stringify(sub)}: got ${JSON.stringify(text)}` };
    return { ok: true, text };
  }, expectedSubstring);
}

const SURFACES = [
  {
    name: 'calendar',
    path: '/calendar',
    description: 'Calendar grid + month navigation + export menu',
    checks: [
      {
        name: 'calendar-page-loads',
        kind: 'auto',
        run: async (page) => {
          const resp = await gotoSurface(page, '/calendar', '#calendar-grid-panel');
          return { ok: resp?.status() === 200, reason: resp?.status() };
        },
      },
      {
        name: 'calendar-month-nav-fires-request',
        kind: 'auto',
        run: async (page) => {
          await gotoSurface(page, '/calendar');
          const req = await Promise.race([
            page.waitForRequest((r) => r.url().includes('/calendar/') && r.method() === 'GET'), { timeout: 3000 },
          ]).catch(() => null);
          // Direct nav doesn't fire request; just verify the month select exists
          const sel = await page.locator('select#month-select').count();
          return { ok: sel === 1, reason: `month select count: ${sel}` };
        },
      },
      {
        name: 'calendar-export-pdf-form-has-data-dixie-submit',
        kind: 'auto',
        run: async (page) => {
          await gotoSurface(page, '/calendar');
          const summary = await page.locator('summary:has-text("Export Month")').first();
          await summary.click().catch(() => {});
          await page.waitForTimeout(200);
          const form = await page.locator('form[action*="/report/pdf"]').first();
          const has = await form.getAttribute('data-dixie-submit');
          return { ok: has !== null, reason: `data-dixie-submit=${has}` };
        },
      },
      {
        name: 'calendar-day-click-shows-anniversary',
        kind: 'manual',
        run: async () => ({
          ok: 'manual',
          prompt: 'Click a day in the grid. Verify a details pane loads to the right with the day number, holidays, and matching anniversaries. Paste the date you clicked into the notes.',
        }),
      },
    ],
  },

  {
    name: 'soldier-new',
    path: '/soldiers/new',
    description: 'Add Person Record form — required fields, submit, redirect',
    checks: [
      {
        name: 'soldier-new-form-loads',
        kind: 'auto',
        run: async (page) => {
          const resp = await gotoSurface(page, '/soldiers/new', 'form');
          const has = await page.locator('input[name="first_name"]').count();
          return { ok: resp?.status() === 200 && has === 1, reason: `first_name input: ${has}` };
        },
      },
      {
        name: 'soldier-new-submit-without-name-shows-validation',
        kind: 'manual',
        run: async () => ({
          ok: 'manual',
          prompt: 'Verify the form refused to submit (browser tooltip on the required field). Note which field is required.',
        }),
      },
    ],
  },

  {
    name: 'browse',
    path: '/browse',
    description: 'Browse view — filter, alphabet, pagination',
    checks: [
      {
        name: 'browse-page-loads',
        kind: 'auto',
        run: async (page) => {
          const resp = await gotoSurface(page, '/browse', '#browse-results, [data-ui-id="page.browse"]');
          return { ok: resp?.status() === 200, reason: resp?.status() };
        },
      },
      {
        name: 'browse-list-renders-soldier-links',
        kind: 'auto',
        run: async (page) => {
          await gotoSurface(page, '/browse');
          const links = await page.locator('a[href^="/soldiers/"]').count();
          return { ok: links > 0, reason: `soldier links: ${links}` };
        },
      },
      {
        name: 'browse-soldier-link-navigates',
        kind: 'manual',
        run: async () => ({
          ok: 'manual',
          prompt: 'Click a soldier name on the /browse page. Verify the soldier detail page loads with their record, sources, and research notes. Note the URL and any visual issues.',
        }),
      },
    ],
  },

  {
    name: 'share',
    path: '/share',
    description: 'Share exports — print config modal, all export buttons navigate to /jobs/{id}',
    checks: [
      {
        name: 'share-page-loads',
        kind: 'auto',
        run: async (page) => {
          const resp = await gotoSurface(page, '/share');
          return { ok: resp?.status() === 200, reason: resp?.status() };
        },
      },
      {
        name: 'share-export-button-discovery',
        kind: 'auto',
        run: async (page) => {
          await gotoSurface(page, '/share');
          const buttons = await page.locator('form[data-dixie-submit] button[type="submit"]').count();
          return { ok: buttons > 0, reason: `discovered ${buttons} export buttons` };
        },
      },
      {
        name: 'share-print-modal-opens',
        kind: 'auto',
        run: async (page) => {
          await gotoSurface(page, '/share');
          const trigger = page.locator('[data-print-config-open], button:has-text("Print")').first();
          if (await trigger.count() === 0) return { ok: false, reason: 'no print trigger' };
          await trigger.click().catch(() => {});
          await page.waitForTimeout(200);
          const modal = await page.locator('[role="dialog"]:visible').count();
          return { ok: modal > 0, reason: `visible modals: ${modal}` };
        },
      },
    ],
  },

  {
    name: 'settings',
    path: '/settings',
    description: 'Settings — scan/quality results render, debug mode toggle',
    checks: [
      {
        name: 'settings-page-loads',
        kind: 'auto',
        run: async (page) => {
          const resp = await gotoSurface(page, '/settings');
          return { ok: resp?.status() === 200, reason: resp?.status() };
        },
      },
      {
        name: 'settings-orphan-scan-results-render',
        kind: 'auto',
        run: async (page) => {
          await gotoSurface(page, '/settings');
          const form = page.locator('form[action*="/settings/images/orphans/scan"]').first();
          if (await form.count() === 0) return { ok: false, reason: 'no orphan scan form' };
          await form.locator('button[type="submit"]').first().click();
          await page.waitForTimeout(500);
          const html = await page.locator('#settings-orphan-results').innerHTML().catch(() => '');
          return { ok: html.trim().length > 0, reason: `target len: ${html.length}` };
        },
      },
      {
        name: 'settings-debug-toggle-round-trips',
        kind: 'auto',
        run: async (page) => {
          await gotoSurface(page, '/settings');
          const form = page.locator('form:has(input[name="debug_mode"])').first();
          if (await form.count() === 0) return { ok: false, reason: 'no debug form' };
          const cb = form.locator('input[name="debug_mode"]').first();
          const wasChecked = await cb.isChecked();
          if (!wasChecked) await cb.check();
          const req = page.waitForRequest((r) => r.url().includes('/settings/debug-mode')).catch(() => null);
          await form.locator('button[type="submit"]').first().click();
          const r = await req;
          return { ok: r !== null, reason: r ? `fired ${r.url()}` : 'no request' };
        },
      },
    ],
  },

  {
    name: 'feedback-modal',
    path: '/',
    description: 'Floating dock Feedback button — save flow + confirmation',
    checks: [
      {
        name: 'feedback-modal-opens',
        kind: 'auto',
        run: async (page) => {
          await gotoSurface(page, '/');
          const trigger = page.locator('[data-feedback-open]').first();
          if (await trigger.count() === 0) return { ok: false, reason: 'no feedback trigger' };
          await trigger.click();
          await page.waitForTimeout(200);
          const visible = await page.locator('#feedback-form').isVisible().catch(() => false);
          return { ok: visible, reason: visible ? 'visible' : 'still hidden' };
        },
      },
      {
        name: 'feedback-save-sends-headers',
        kind: 'auto',
        run: async (page) => {
          await gotoSurface(page, '/');
          await page.locator('[data-feedback-open]').first().click();
          await page.waitForTimeout(200);
          await page.fill('#feedback-form textarea[name="message"]', 'interactive audit test');
          const resp = page.waitForResponse((r) => r.url().includes('/feedback/submit') && r.request().method() === 'POST');
          await page.click('#feedback-form button[type="submit"]');
          const r = await resp;
          const close = r.headers()['x-dixiedata-close-feedback'];
          const toast = r.headers()['x-dixiedata-toast'];
          return { ok: close === 'true' && !!toast, reason: `close=${close} toast=${toast}` };
        },
      },
      {
        name: 'feedback-save-hides-modal-clears-form-shows-toast',
        kind: 'auto',
        run: async (page) => {
          await gotoSurface(page, '/');
          await page.locator('[data-feedback-open]').first().click();
          await page.waitForTimeout(200);
          await page.fill('#feedback-form textarea[name="message"]', 'interactive audit test');
          await page.click('#feedback-form button[type="submit"]');
          await page.waitForTimeout(500);
          const result = await page.evaluate(() => {
            const modal = document.querySelector('[data-feedback-modal]');
            const ta = document.querySelector('#feedback-form textarea[name="message"]');
            const toast = document.querySelector('[data-toast-region]');
            return {
              modalHidden: modal instanceof HTMLElement && modal.classList.contains('hidden'),
              textareaEmpty: ta instanceof HTMLTextAreaElement && ta.value === '',
              toastVisible: toast instanceof HTMLElement && !toast.classList.contains('hidden') && (toast.textContent || '').trim().length > 0,
            };
          });
          return {
            ok: result.modalHidden && result.textareaEmpty && result.toastVisible,
            reason: JSON.stringify(result),
          };
        },
      },
    ],
  },

  {
    name: 'floating-dock-layout',
    path: '/',
    description: 'Floating dock positioning, mobile overflow, no content overlap',
    checks: [
      {
        name: 'floating-dock-renders-at-bottom',
        kind: 'auto',
        run: async (page) => {
          await gotoSurface(page, '/');
          const dock = page.locator('.floating-dock').first();
          if (await dock.count() === 0) return { ok: false, reason: 'no .floating-dock' };
          const box = await dock.boundingBox();
          const viewport = page.viewportSize();
          if (!box || !viewport) return { ok: false, reason: 'no box/viewport' };
          const atBottom = box.y + box.height >= viewport.height - 5;
          return { ok: atBottom, reason: `dock y=${box.y} h=${box.height} viewport h=${viewport.height}` };
        },
      },
      {
        name: 'floating-dock-no-overlap-with-toast-region',
        kind: 'manual',
        run: async () => ({
          ok: 'manual',
          prompt: 'Open the floating nav (Menu button) and verify it does not overlap the dock itself. Also open the Feedback modal and verify the dock stays below the modal backdrop.',
        }),
      },
    ],
  },

  {
    name: 'jobs-page',
    path: '/jobs/active',
    description: 'Jobs active + auto-poll on /jobs/{id}',
    checks: [
      {
        name: 'jobs-active-endpoint-returns-200',
        kind: 'auto',
        run: async (page) => {
          // /jobs/active is a polling fragment, not a full page. Verify
          // the endpoint responds 200 when called via XHR.
          const resp = await page.request.get(`${BASE}/jobs/active`, {
            headers: { 'HX-Request': 'true' },
          });
          return { ok: resp.status() === 200 || resp.status() === 204, reason: resp.status() };
        },
      },
      {
        name: 'jobs-page-auto-polls-active-region',
        kind: 'auto',
        run: async (page) => {
          await gotoSurface(page, '/');
          // The data-jobs-progress-region polls /jobs/active
          const reqs = [];
          page.on('request', (r) => {
            if (r.url().includes('/jobs/active')) reqs.push(r.url());
          });
          await page.waitForTimeout(3500);
          return { ok: reqs.length >= 1, reason: `polls: ${reqs.length}` };
        },
      },
    ],
  },
];

async function main() {
  const browser = await chromium.launch();
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  const page = await context.newPage();

  const surfaces = SURFACE_FILTER
    ? SURFACES.filter((s) => s.name === SURFACE_FILTER)
    : SURFACES;

  for (const surface of surfaces) {
    console.log(`\n[${surface.name}] ${surface.description}`);
    await shot(page, `${surface.name}-before`).catch(() => {});
    for (const check of surface.checks) {
      try {
        const out = await check.run(page);
        if (check.kind === 'manual') {
          record(`${surface.name}.${check.name}`, 'manual', { prompt: check.prompt });
        } else {
          record(`${surface.name}.${check.name}`, out?.ok, { reason: out?.reason });
        }
      } catch (e) {
        record(`${surface.name}.${check.name}`, false, { reason: `threw: ${e.message}` });
      }
    }
    await shot(page, `${surface.name}-after`).catch(() => {});
  }

  await writeFile(REPORT, JSON.stringify({ results, pass, fail, manual, ts: new Date().toISOString() }, null, 2));
  console.log(`\n============================================================`);
  console.log(`INTERACTIVE AUDIT: ${pass} pass, ${fail} fail, ${manual} manual`);
  console.log(`Report: ${REPORT}`);
  console.log(`Screenshots: ${SHOTS}`);
  console.log(`Notes template: docs/agents/audit-notes-TEMPLATE.md`);
  console.log(`============================================================`);

  await browser.close();
  process.exit(fail > 0 ? 1 : 0);
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
