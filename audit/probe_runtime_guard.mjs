#!/usr/bin/env node
// probe_runtime_guard.mjs — issue #614 (scoped down).
//
// Regression net for the Wails-runtime web-mode contract:
//
//   - Wails dialog APIs (SaveFileDialog / OpenFileDialog /
//     OpenDirectoryDialog / OpenMultipleFilesDialog) are
//     only available in the Wails-GUI binary.
//   - The dixiedata-web binary serves the same UI over HTTP
//     but cannot pop native dialogs.
//   - Per `cmd/dixiedata-web/main.go:7-9` the contract is
//     "handlers that call Wails dialog APIs will panic" —
//     a comment, not an enforcement.
//
// The repo already has guards in `internal/appshell/runtime.go`
// (`wailsHasFrontend(ctx)` + `errWailsFrontendUnavailable`)
// that prevent the panic by short-circuiting before the
// underlying `wailsruntime.*` call. The probe asserts the
// contract is actually held in a live dixiedata-web run by
// hitting at least one route that calls a dialog in Wails
// mode + asserting the response is NOT a Go panic stack trace.
//
// Run:
//   node audit/probe_runtime_guard.mjs
//
// Exit 0 = no Wails-call route panicked.
// Exit 1 = at least one route returned an unhandled-panic signature.
// Exit 2 = fatal (binary missing, server didn't start, etc.).

import { spawn } from 'node:child_process';
import { setTimeout as sleep } from 'node:timers/promises';
import fs from 'node:fs';
import path from 'path';
import process from 'node:process';

const PORT = process.env.PROBE_PORT || '8796';
const BASE = `http://127.0.0.1:${PORT}`;
const SCRATCH = process.env.SCRATCH_DIR || 'C:/Development/DixieData/.scratch/runtime-guard';
const WEB_BIN = process.env.WEB_BIN || 'C:/Development/DixieData/build/bin/dixiedata-web.exe';

if (!fs.existsSync(WEB_BIN)) {
	console.error(`missing ${WEB_BIN} — build it first (make build)`);
	process.exit(2);
}
fs.mkdirSync(SCRATCH, { recursive: true });

const server = spawn(WEB_BIN, ['-addr', `127.0.0.1:${PORT}`, '-scratch-dir', SCRATCH], {
	stdio: ['ignore', 'pipe', 'pipe'],
});
server.stderr.on('data', () => {}); // swallow noise

function cleanup() {
	try { server.kill(); } catch (_) {}
	try { fs.rmSync(SCRATCH, { recursive: true, force: true }); } catch (_) {}
}
process.on('exit', cleanup);
process.on('SIGINT', () => { cleanup(); process.exit(130); });

async function waitForServer() {
	for (let i = 0; i < 40; i++) {
		try {
			const res = await fetch(`${BASE}/setup`);
			if (res.status < 500) return;
		} catch (_) {}
		await sleep(250);
	}
	throw new Error(`server at ${BASE} did not start within 10s`);
}

// Routes that historically called Wails dialog APIs in the
// GUI binary. Hitting them from the web binary must NOT
// panic. The web binary's `saveFileDialogOverride` +
// `openDirectoryDialogOverride` + `openFileDialogOverride`
// + `openMultipleFilesDialogOverride` (set in
// `cmd/dixiedata-web/main.go:84-160`) auto-route these
// flows to deterministic paths, so the routes should
// complete cleanly (200/4xx/5xx in the expected range).
const WAILS_DIALOG_ROUTES = [
	// POST /export/json — calls a.SaveFileDialog (exports
	// via the JSON exporter). The web override sets the
	// path to <dataDir>/exports/dixiedata-export-N.json.
	{ method: 'POST', path: '/export/json', expectedStatus: [200, 202, 4, 5] },
	// GET /soldiers — list route; doesn't call Wails APIs
	// directly but exercises the request pipeline. If the
	// guard breaks ANY request flow, this would fail.
	{ method: 'GET', path: '/soldiers', expectedStatus: [200, 401, 403, 302] },
];

let pass = 0;
let fail = 0;
const failures = [];

function record(name, ok, details) {
	if (ok) {
		pass++;
		console.log(`  ✓ ${name}`);
	} else {
		fail++;
		failures.push({ name, details });
		console.error(`  ✗ ${name} — ${JSON.stringify(details).slice(0, 200)}`);
	}
}

// Detect the "unhandled panic" signature: Go's net/http
// panic handler writes the stack trace to the response
// body. Look for the canonical Go stack-trace markers.
function isUnhandledPanic(body) {
	if (!body) return false;
	return /goroutine\s+\d+\s+\[running\]/.test(body)
		|| /runtime error:/.test(body)
		|| /index out of range|invalid memory address|nil pointer dereference/.test(body)
		|| /panic:\s+/.test(body);
}

async function hit(method, urlPath) {
	const res = await fetch(`${BASE}${urlPath}`, { method });
	const body = await res.text();
	return { status: res.status, body };
}

async function main() {
	await waitForServer();
	for (const route of WAILS_DIALOG_ROUTES) {
		const r = await hit(route.method, route.path);
		const panicSignature = isUnhandledPanic(r.body);
		const inExpectedStatus = route.expectedStatus.some((prefix) =>
			Math.floor(r.status / 100) === prefix || r.status === prefix,
		);
		record(
			`${route.method} ${route.path} → no panic`,
			!panicSignature,
			{ status: r.status, inExpectedStatus, bodyLen: r.body.length },
		);
	}

	if (fail > 0) {
		console.error(`\nFAIL: ${fail} assertion(s) failed.`);
		for (const f of failures) console.error(`  ${f.name}: ${JSON.stringify(f.details).slice(0, 300)}`);
		process.exit(1);
	}
	console.log(`\nPASS: ${pass} assertion(s).`);
	process.exit(0);
}

main().catch((err) => {
	console.error('fatal:', err);
	process.exit(2);
});
