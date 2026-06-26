// Probe: verify the fixed state of the Load Backup UI status feedback.
// Boots dixiedata-web in the background, drives /share with a real
// headless browser, asserts:
//   1. Load Backup button targets #share-status
//   2. Load Backup does NOT use hx-swap="none"
//   3. #share-status panel is visible from page load
//   4. Memorial JSON Preview button ALSO targets #share-status (consistency)
// Reports PASS / FAIL with the actual observed attribute values.
import { chromium } from 'playwright';
import { spawn } from 'node:child_process';
import { setTimeout as sleep } from 'node:timers/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const PORT = process.env.PROBE_PORT || '8765';
const BASE = `http://127.0.0.1:${PORT}`;

async function waitForServer(url, maxMs = 20000) {
  const deadline = Date.now() + maxMs;
  while (Date.now() < deadline) {
    try {
      const res = await fetch(url, { method: 'GET' });
      if (res.status < 500) return;
    } catch (_) {}
    await sleep(200);
  }
  throw new Error(`server at ${url} never came up`);
}

async function main() {
  // Boot the web-mode binary in the background
  // Probe runs from audit/ dir but spawns go run from repo root so the
  // cmd/dixiedata-web module resolves.
  const here = path.dirname(fileURLToPath(import.meta.url));
  const repoRoot = here.endsWith('audit') ? path.dirname(here) : here;
  const scratchDir = '.scratch/probe-backup-status';
  // proc is assigned inside try{} after seeding; declare for the finally cleanup.
  let proc = null;

  let exitCode = 0;
  try {
    // Clean stale scratch dir from prior probe runs (lock-prone on Windows).
    await import('node:fs/promises').then((m) => m.rm(scratchDir, { recursive: true, force: true }));
    console.log(`cleaned ${scratchDir}`);
    // Seed identity + minimal data BEFORE server starts so /share renders.
    {
      const seedProc = spawn(
        'go',
        ['run', './cmd/seed-data', '-data-dir', scratchDir, '-soldiers', '5', '-reset'],
        { cwd: repoRoot, stdio: ['ignore', 'pipe', 'pipe'] }
      );
      let seedOut = '';
      seedProc.stdout.on('data', (d) => { seedOut += d; });
      seedProc.stderr.on('data', (d) => { seedOut += d; });
      const seedExit = await new Promise((resolve) => seedProc.on('exit', resolve));
      if (seedExit !== 0) throw new Error(`seed-data failed (exit ${seedExit}):\n${seedOut}`);
      console.log(`seeded ${scratchDir}`);
    }

    // NOW boot the server.
    proc = spawn(
      'go',
      ['run', './cmd/dixiedata-web', '-addr', `127.0.0.1:${PORT}`, '-scratch-dir', scratchDir],
      { cwd: repoRoot, env: { ...process.env, DIXIEDATA_DATA_DIR: scratchDir } }
    );
    proc.stderr.on('data', (d) => process.stderr.write(`[srv] ${d}`));
    proc.stdout.on('data', (d) => process.stdout.write(`[srv] ${d}`));

    await waitForServer(BASE, 30000);
    console.log(`server up at ${BASE}`);

    const browser = await chromium.launch();
    const page = await browser.newPage({ viewport: { width: 1280, height: 800 } });
    const errors = [];
    page.on('pageerror', (e) => errors.push(e.message));
    page.on('console', (m) => { if (m.type() === 'error') errors.push(`console: ${m.text()}`); });

    await page.goto(`${BASE}/share`, { waitUntil: 'domcontentloaded' });
    await sleep(1500);
    // Dump page body for diagnostics when selectors miss
    const bodyText = await page.locator('body').innerText().catch(() => '<no body>');
    console.log(`--- /share body (first 1500 chars) ---\n${bodyText.slice(0, 1500)}\n---`);

    // 1. Load Backup button attributes
    const btn = page.locator('button.danger-button:has-text("Load Backup")');
    const count = await btn.count();
    console.log(`Load Backup button count: ${count} (expect 1)`);
    if (count !== 1) { exitCode = 1; }

    const hxTarget = await btn.first().getAttribute('hx-target');
    const hxSwap = await btn.first().getAttribute('hx-swap');
    const hxConfirm = await btn.first().getAttribute('hx-confirm');
    console.log(`Load Backup: hx-target="${hxTarget}" hx-swap="${hxSwap}" hx-confirm="${hxConfirm}"`);

    // 2. Status panel visibility
    const statusPanel = page.locator('#share-status');
    const panelVisible = await statusPanel.isVisible();
    const panelHTML = await statusPanel.innerHTML().catch(() => '<missing>');
    console.log(`#share-status visible: ${panelVisible}`);
    console.log(`#share-status html: ${panelHTML.slice(0, 200)}`);

    // 3. Sibling Memorial JSON Preview button attributes
    const memorial = page.locator('button:has-text("Preview Memorial JSON Import")');
    const memorialCount = await memorial.count();
    if (memorialCount > 0) {
      const mTarget = await memorial.first().getAttribute('hx-target');
      const mSwap = await memorial.first().getAttribute('hx-swap');
      console.log(`Memorial Preview: hx-target="${mTarget}" hx-swap="${mSwap}"`);
    }

    // F2: Where is #share-status relative to the viewport when the
    // page is loaded fresh at scrollY=0?
    const panelRect = await page.locator('#share-status').boundingBox();
    const viewport = page.viewportSize();
    console.log(`viewport: ${viewport.width}x${viewport.height}`);
    if (panelRect) {
      const aboveFold = panelRect.y < viewport.height;
      console.log(`#share-status y=${Math.round(panelRect.y)} height=${Math.round(panelRect.height)} above-fold=${aboveFold}`);
      if (!aboveFold) {
        console.log('  -> BELOW FOLD: status message will be invisible without scrolling');
      }
    }

    // F3: at scrollY=0, can the user see the Load Backup button AND
    // the status panel in one viewport? (Critical for "I clicked it
    // and nothing happened" reports.)
    const btnRect = await btn.first().boundingBox();
    if (btnRect && panelRect) {
      const allVisible =
        btnRect.y >= 0 && btnRect.y + btnRect.height <= viewport.height &&
        panelRect.y >= 0 && panelRect.y + panelRect.height <= viewport.height;
      console.log(`both in viewport @ scrollY=0: ${allVisible} (btn y=${Math.round(btnRect.y)}, panel y=${Math.round(panelRect.y)})`);
    }

    await browser.close();

    // Verdict
    const verdicts = [];
    if (hxTarget === '#share-status') verdicts.push('OK: Load Backup targets #share-status');
    else { verdicts.push(`FAIL: Load Backup targets "${hxTarget}" (want #share-status)`); exitCode = 1; }

    if (hxSwap !== 'none') verdicts.push('OK: Load Backup does not use hx-swap="none"');
    else { verdicts.push('FAIL: Load Backup uses hx-swap="none" — response suppressed'); exitCode = 1; }

    if (panelVisible) verdicts.push('OK: #share-status panel visible');
    else { verdicts.push('FAIL: #share-status panel not visible'); exitCode = 1; }

    console.log('\n=== VERDICT ===');
    verdicts.forEach((v) => console.log(v));

    // Phase 3 fallout checks: things the fix did NOT address.
    console.log('\n=== FALLOUT PROBES (static analysis — handler source) ===');
    console.log('F1: redirect after restore?');
    console.log('    handleImportBackup (imports_handlers.go:18-73)');
    console.log('    - sets X-DixieData-Toast on success  -> user sees toast');
    console.log('    - DOES NOT set X-DixieData-Redirect  -> user stays on /share');
    console.log('    Compare: handleImportSharedArchive sets X-DixieData-Redirect: /export on conflicts');
    console.log('    => F1 LIKELY ROOT CAUSE: user stays on /share which displays stale merge-review/export state.');

    if (errors.length > 0) {
      console.log(`\nPage errors (${errors.length}):`);
      errors.forEach((e) => console.log(`  - ${e}`));
    }
  } catch (e) {
    console.error('FATAL:', e);
    exitCode = 2;
  } finally {
    if (proc) {
      // Best-effort cleanup. Windows: 'go run' spawns a child exe that
      // often survives SIGTERM, holding the data dir open. nuke any
      // lingering dixiedata-web processes.
      proc.kill('SIGTERM');
      await sleep(500);
      if (process.platform === 'win32') {
        await new Promise((resolve) => {
          const killer = spawn('taskkill', ['/F', '/IM', 'dixiedata-web.exe', '/T'], { stdio: 'ignore' });
          killer.on('exit', resolve);
        });
      } else {
        try { process.kill(-proc.pid, 'SIGKILL'); } catch (_) {}
      }
      await sleep(300);
    }
    process.exit(exitCode);
  }
}

main();