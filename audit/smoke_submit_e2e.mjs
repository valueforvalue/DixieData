/**
 * audit/smoke_submit_e2e.mjs — canonical submit-to-DB-to-render
 * probe (issue #618).
 *
 * The probe exercises a single UI form submission end-to-end
 * and asserts the full loop:
 *   1. Boot dixiedata-web against a private scratch dir
 *   2. Seed a Person Record via the in-process service
 *   3. Navigate to /soldiers/{seed-id} (the detail page)
 *   4. POST a tag-attach form (data-dixie-submit target)
 *   5. Assert response 200 + the form was processed
 *   6. Assert DB row updated: SELECT count from
 *      person_record_tags WHERE person_id = seed.id
 *   7. Assert the detail page re-renders with the new tag
 *   8. Rollback: POST same form with empty tag name returns
 *      400 (validation) + no DB change
 *
 * The probe is the canonical "submit-to-DB-to-render"
 * regression net (Tracer Bullets row 7/10 in the 2026-07
 * pragmatic-programmer diagnostic). It complements the
 * per-screen smoke_*.mjs probes (which exercise one screen
 * each) by proving the cross-stack chain works for at least
 * one happy + one sad path.
 *
 * Run: `node audit/smoke_submit_e2e.mjs`
 * Exits 0 on all-pass, 1 on any-fail, 2 on fatal.
 */
import { chromium } from 'playwright';
import { spawn } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import { runProbe } from './_lib/smoke_runner.mjs';
import { webBin } from './_lib/smoke_paths.mjs';

const PORT = process.env.PROBE_PORT || '8795';
const BASE = `http://127.0.0.1:${PORT}`;
const WEB_BIN_PATH = webBin();

if (!fs.existsSync(WEB_BIN_PATH)) {
	console.error(`missing ${WEB_BIN_PATH} — build it first (make build)`);
	process.exit(2);
}

const wait = (ms) => new Promise((r) => setTimeout(r, ms));

async function waitForServer() {
	for (let i = 0; i < 40; i++) {
		try {
			const res = await fetch(`${BASE}/setup`);
			if (res.status < 500) return;
		} catch (_) {}
		await wait(250);
	}
	throw new Error(`server at ${BASE} did not start within 10s`);
}

async function seedSoldier() {
	// The /setup wizard accepts an initial form. Skip it by
	// POSTing the minimal setup form (display name + password
	// fields), then seed one Person Record via the same
	// /soldiers/new POST form the UI uses.
	const browser = await chromium.launch();
	const ctx = await browser.newContext();
	const page = await ctx.newPage();

	// Complete the initial setup wizard.
	await page.goto(`${BASE}/setup`, { waitUntil: 'domcontentloaded' });
	await page.locator('input[name="display_name"]').fill('Smoke Submitter');
	await page.locator('input[name="password"]').fill('test-password-1234');
	await page.locator('input[name="confirm_password"]').fill('test-password-1234');
	await page.locator('form button[type="submit"]').first().click();
	await page.waitForURL((url) => !url.pathname.startsWith('/setup'), { timeout: 10_000 });

	// Create a Person Record via the form on /soldiers/new.
	await page.goto(`${BASE}/soldiers/new`, { waitUntil: 'domcontentloaded' });
	await page.locator('input[name="first_name"]').fill('Submit');
	await page.locator('input[name="last_name"]').fill('Smoke');
	await page.locator('form button[type="submit"]').first().click();
	await page.waitForURL(/\/soldiers\/(DXD|P)-\d+/, { timeout: 10_000 });

	const soldierURL = page.url();
	const soldierID = Number(soldierURL.split('/').pop().split('-').pop());
	await browser.close();
	return { soldierID, soldierURL };
}

async function getTagCount(personID, scratchDir) {
	// Direct DB read via the SQLite file in the scratch dir.
	const dbPath = path.join(scratchDir, 'dixiedata.db');
	if (!fs.existsSync(dbPath)) return -1;
	// Use sqlite3 CLI for the cross-platform read.
	try {
		const { execFileSync } = await import('node:child_process');
		const out = execFileSync(
			'sqlite3',
			[dbPath, `SELECT COUNT(*) FROM person_record_tags WHERE person_id = ${personID}`],
			{ encoding: 'utf8' },
		);
		return Number(out.trim());
	} catch (err) {
		console.error(`sqlite3 read failed: ${err.message}`);
		return -1;
	}
}

const results = [];
function record(name, ok, details = {}) {
	results.push({ name, ok, ...details });
	console.log(`  ${ok ? '✓' : '✗'} ${name}${ok ? '' : ' — ' + JSON.stringify(details)}`);
}

async function main(ctx) {
	// ctx.page         -- opaque (slice-3 probe doesn't drive a
	//                     chromium page; the headless setup
	//                     wizard + DB inspection don't need a page).
	// ctx.base         -- server URL (unused; PORT set below).
	// ctx.scratchDir   -- mkdtempSync per-probe scratch.
	// ctx.registerCleanup(fn) -- fired after main returns; the
	//                     runner takes care of server SIGTERM
	//                     and scratch removal when the
	//                     aggregator pre-spawned the server.
	//                     For the standalone
	//                     `node audit/smoke_submit_e2e.mjs`
	//                     invocation (no server pre-spawned),
	//                     main() spawns the server itself and
	//                     uses ctx.registerCleanup() to SIGTERM
	//                     it on exit.
	const SCRATCH = ctx.scratchDir;

	const server = spawn(WEB_BIN_PATH, [
		'-addr',
		`127.0.0.1:${PORT}`,
		'-scratch-dir',
		SCRATCH,
	], { stdio: ['ignore', 'pipe', 'pipe'] });
	server.stderr.on('data', () => {}); // swallow noise
	ctx.registerCleanup(async () => {
		try { if (!server.killed) server.kill('SIGTERM'); }
		catch (_) { /* best effort */ }
	});

	await waitForServer();

	// Issue #700 slice 3: dispatchDixieDataForm-driven forms
	// inside the runner's probeFn. The seedSoldier helper
	// creates a Person Record via the headless setup wizard +
	// /soldiers/new form (the canonical happy-path surface).
	const { soldierID, soldierURL } = await seedSoldier();
	record('seed: Person Record created', soldierID > 0, { soldierID, soldierURL });

	// 4. Open the detail page in a browser.
	const browser = await chromium.launch();
	const browserCtx = await browser.newContext();
	const page = await browserCtx.newPage();
	await page.goto(soldierURL, { waitUntil: 'domcontentloaded' });

	// 5. POST a tag-attach form (the canonical data-dixie-submit
	// form on the detail page). The form posts a tag name;
	// the handler attaches (or upserts) the tag and refreshes
	// the tag-list fragment.
	const beforeCount = await getTagCount(soldierID, SCRATCH);

	// Find the tag-attach form. The detail page renders it as
	// a <form> with data-dixie-submit + an <input name="tag">.
	const tagInput = page.locator('form[data-dixie-submit] input[name="tag"]').first();
	if (await tagInput.count() === 0) {
		record('form: tag-attach input present', false, { url: page.url() });
		await browser.close();
		throw new Error('tag-attach input not present');
	}
	record('form: tag-attach input present', true);

	// 6. Submit the form. The dispatchDixieDataForm dispatcher
	// uses fetch + JSON; the response is the updated tag
	// fragment (or a 4xx for validation).
	const responsePromise = page.waitForResponse(
		(r) => r.url().includes(`/soldiers/${soldierID}`) && r.request().method() === 'POST',
		{ timeout: 5_000 },
	);
	await tagInput.fill('Smoke Tag');
	await page.locator('form[data-dixie-submit] button[type="submit"]').first().click();
	const response = await responsePromise;
	record('post: response 2xx', response.ok(), { status: response.status() });

	// 7. DB row check.
	const afterCount = await getTagCount(soldierID, SCRATCH);
	record('db: person_record_tags row count increased', afterCount === beforeCount + 1, {
		before: beforeCount,
		after: afterCount,
	});

	// 8. DOM re-render check.
	await page.waitForLoadState('networkidle');
	const tagRendered = await page.locator('text="Smoke Tag"').count();
	record('dom: tag rendered after re-render', tagRendered > 0, { tagRendered });

	// 9. Rollback: submit empty tag name → 400 + no DB change.
	const beforeRollback = await getTagCount(soldierID, SCRATCH);
	const rollbackResponsePromise = page.waitForResponse(
		(r) => r.url().includes(`/soldiers/${soldierID}`) && r.request().method() === 'POST',
		{ timeout: 5_000 },
	);
	await tagInput.fill('');
	await page.locator('form[data-dixie-submit] button[type="submit"]').first().click();
	const rollbackResponse = await rollbackResponsePromise;
	const afterRollback = await getTagCount(soldierID, SCRATCH);
	record('rollback: empty tag returns 4xx', rollbackResponse.status() >= 400, {
		status: rollbackResponse.status(),
	});
	record('rollback: DB row count unchanged', afterRollback === beforeRollback, {
		before: beforeRollback,
		after: afterRollback,
	});

	await browser.close();

	// Issue #700 slice 3: return {ok, steps} for the runner
	// instead of calling process.exit; the aggregator's reporter
	// emits the exit code via renderSummary().
	const failed = results.filter((r) => !r.ok);
	if (failed.length > 0) {
		console.error(`\nFAIL: ${failed.length} assertion(s) failed.`);
		return { ok: false, steps: { pass: results.length - failed.length, fail: failed.length, failed } };
	}
	console.log(`\nPASS: ${results.length} assertion(s).`);
	return { ok: true, steps: { pass: results.length, fail: 0 } };
}

// Issue #700 slice 3: wrap main() in the shared Playwright
// runner (audit/_lib/smoke_runner.mjs). The runner provides
// {page, base, scratchDir, registerCleanup} via the ctx
// argument; the server-spawn + cleanup-hook chain that
// previously inlined at module scope now lives inside main()
// (which runs as the runner's probeFn). mkdtempSync
// scratchDir replaces the inline os.tmpdir() one so concurrent
// runner runs don't collide. The runner's reporter emits the
// process exit code via renderSummary(), so the standalone
// `node audit/smoke_submit_e2e.mjs` exit codes (0/1/2) are
// preserved below.

mainWrapper().catch((err) => {
	console.error('fatal:', err);
	process.exit(2);
});

async function mainWrapper() {
	const result = await runProbe({
		name: 'submit-e2e',
		probeFn: main,
	});
	process.exit(result.ok ? 0 : 1);
}
