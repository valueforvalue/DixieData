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

const PORT = process.env.PROBE_PORT || '8795';
const BASE = `http://127.0.0.1:${PORT}`;
const SCRATCH = fs.mkdtempSync(path.join(os.tmpdir(), 'dixiedata-smoke-submit-'));
const WEB_BIN = process.env.WEB_BIN || 'C:/Development/DixieData/build/bin/dixiedata-web.exe';

if (!fs.existsSync(WEB_BIN)) {
	console.error(`missing ${WEB_BIN} — build it first (make build)`);
	process.exit(2);
}

const server = spawn(WEB_BIN, ['-addr', `127.0.0.1:${PORT}`, '-scratch-dir', SCRATCH], {
	stdio: ['ignore', 'pipe', 'pipe'],
});
server.stderr.on('data', () => {}); // swallow noise

function cleanup() {
	try {
		server.kill();
	} catch (_) {}
	try {
		fs.rmSync(SCRATCH, { recursive: true, force: true });
	} catch (_) {}
}
process.on('exit', cleanup);
process.on('SIGINT', () => {
	cleanup();
	process.exit(130);
});

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

async function getTagCount(personID) {
	// Direct DB read via the SQLite file in the scratch dir.
	const dbPath = path.join(SCRATCH, 'dixiedata.db');
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

async function main() {
	await waitForServer();

	const { soldierID, soldierURL } = await seedSoldier();
	record('seed: Person Record created', soldierID > 0, { soldierID, soldierURL });

	// 4. Open the detail page in a browser.
	const browser = await chromium.launch();
	const ctx = await browser.newContext();
	const page = await ctx.newPage();
	await page.goto(soldierURL, { waitUntil: 'domcontentloaded' });

	// 5. POST a tag-attach form (the canonical data-dixie-submit
	// form on the detail page). The form posts a tag name;
	// the handler attaches (or upserts) the tag and refreshes
	// the tag-list fragment.
	const beforeCount = await getTagCount(soldierID);

	// Find the tag-attach form. The detail page renders it as
	// a <form> with data-dixie-submit + an <input name="tag">.
	const tagInput = page.locator('form[data-dixie-submit] input[name="tag"]').first();
	if (await tagInput.count() === 0) {
		record('form: tag-attach input present', false, { url: page.url() });
		await browser.close();
		process.exit(1);
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
	const afterCount = await getTagCount(soldierID);
	record('db: person_record_tags row count increased', afterCount === beforeCount + 1, {
		before: beforeCount,
		after: afterCount,
	});

	// 8. DOM re-render check.
	await page.waitForLoadState('networkidle');
	const tagRendered = await page.locator('text="Smoke Tag"').count();
	record('dom: tag rendered after re-render', tagRendered > 0, { tagRendered });

	// 9. Rollback: submit empty tag name → 400 + no DB change.
	const beforeRollback = await getTagCount(soldierID);
	const rollbackResponsePromise = page.waitForResponse(
		(r) => r.url().includes(`/soldiers/${soldierID}`) && r.request().method() === 'POST',
		{ timeout: 5_000 },
	);
	await tagInput.fill('');
	await page.locator('form[data-dixie-submit] button[type="submit"]').first().click();
	const rollbackResponse = await rollbackResponsePromise;
	const afterRollback = await getTagCount(soldierID);
	record('rollback: empty tag returns 4xx', rollbackResponse.status() >= 400, {
		status: rollbackResponse.status(),
	});
	record('rollback: DB row count unchanged', afterRollback === beforeRollback, {
		before: beforeRollback,
		after: afterRollback,
	});

	await browser.close();

	const failed = results.filter((r) => !r.ok);
	if (failed.length > 0) {
		console.error(`\nFAIL: ${failed.length} assertion(s) failed.`);
		process.exit(1);
	}
	console.log(`\nPASS: ${results.length} assertion(s).`);
	process.exit(0);
}

main().catch((err) => {
	console.error('fatal:', err);
	process.exit(2);
});
