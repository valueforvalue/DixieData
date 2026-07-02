// audit/smoke_browse_nav_speed.mjs — regression net for the click-
// delay on /browse + /share top-nav links (issues #176 + #234).
//
// Pre-fix: /browse GET took 261ms at 5k records (bench-verified).
// Post-fix: /browse GET should stay under 500ms end-to-end on a
// typical archive; the unit-level bench is the tight bound.
//
// Probe boots dixiedata-web against the project's pre-configured
// .scratch/webmode (which has identity already set up — the probe
// is validating nav latency, not the setup flow), then measures
// /browse + /share response times and asserts the fragment
// endpoint is reachable.
//
// Run after dixiedata-web is built at $WEB_BIN (defaults to
// build/bin/dixiedata-web.exe):
//
//   node audit/smoke_browse_nav_speed.mjs
//
// Exit code is non-zero when any assertion fails.

import { spawn } from "node:child_process";
import { existsSync } from "node:fs";

const PORT = 9950;
// .scratch/webmode is the audit harness's seed-loaded archive
// (identity configured). A fresh scratch dir would 303 to /setup
// which would mask the nav latency we want to measure.
const SCRATCH = "C:/Development/DixieData/.scratch/webmode";
const WEB_BIN = process.env.WEB_BIN || "C:/Development/DixieData/build/bin/dixiedata-web.exe";

if (!existsSync(WEB_BIN)) {
  console.error("missing web binary at", WEB_BIN);
  process.exit(2);
}
if (!existsSync(SCRATCH)) {
  console.error("missing pre-configured scratch dir at", SCRATCH);
  console.error("(run the audit harness once to populate .scratch/webmode)");
  process.exit(2);
}

const server = spawn(
  WEB_BIN,
  ["-addr", `127.0.0.1:${PORT}`, "-scratch-dir", SCRATCH],
  { stdio: ["ignore", "pipe", "pipe"] }
);
server.stderr.on("data", () => {});

const wait = (ms) => new Promise((r) => setTimeout(r, ms));
async function ready() {
  for (let i = 0; i < 60; i++) {
    try {
      const r = await fetch(`http://127.0.0.1:${PORT}/`);
      if (r.status >= 200 && r.status < 500) return;
    } catch {}
    await wait(500);
  }
  throw new Error("server never came up");
}

let pass = 0;
let fail = 0;
function record(name, ok, details = {}) {
  if (ok) { pass++; console.log(`  ✓ ${name} (${JSON.stringify(details)})`); }
  else { fail++; console.log(`  ✗ ${name} (${JSON.stringify(details)})`); }
}

try {
  await ready();
  // Wait for the server's startup to finish (DB open + jobs log).
  await wait(2000);

  console.log("\nbrowse + share nav latency (against .scratch/webmode)");
  console.log("-----------------------------------------------------");

  // /browse: must be fast (issue #234 dropped listAllSoldiers).
  // 5-call average to smooth jitter.
  const browseElapsed = await averageElapsed(PORT, 5, "/browse", 500);
  record("browse-fast", browseElapsed.avg < 500, browseElapsed);

  // /share: also fast on the GET; the modal's fragment fetches
  // separately when opened (not exercised in this probe).
  const shareElapsed = await averageElapsed(PORT, 5, "/share", 500);
  record("share-fast", shareElapsed.avg < 500, shareElapsed);

  // Fragment endpoint: confirm reachable, returns 200 + body
  // contains the sentinel div that htmx swaps into.
  const fragStart = Date.now();
  const fragResp = await fetch(`http://127.0.0.1:${PORT}/share/print-records-fragment`);
  const fragElapsed = Date.now() - fragStart;
  const fragBody = await fragResp.text();
  record(
    "fragment-reachable",
    fragResp.status === 200 && fragBody.includes("data-print-config-body"),
    { elapsed_ms: fragElapsed, status: fragResp.status, body_len: fragBody.length },
  );

  // Second fragment fetch should be similarly fast (no caching at
  // the server side; the JS caches client-side per Slice 5).
  const frag2Start = Date.now();
  const frag2Resp = await fetch(`http://127.0.0.1:${PORT}/share/print-records-fragment`);
  const frag2Elapsed = Date.now() - frag2Start;
  record(
    "fragment-repeat-fast",
    frag2Resp.status === 200 && frag2Elapsed < 500,
    { elapsed_ms: frag2Elapsed, status: frag2Resp.status },
  );

  console.log(`\n${pass} passed, ${fail} failed`);
  process.exit(fail === 0 ? 0 : 1);
} catch (e) {
  console.error("FATAL", e);
  process.exit(2);
} finally {
  server.kill();
  await wait(500);
}

async function averageElapsed(port, n, path, warmupMs) {
  const url = `http://127.0.0.1:${port}${path}`;
  await wait(warmupMs);
  const times = [];
  for (let i = 0; i < n; i++) {
    const start = Date.now();
    const resp = await fetch(url, { redirect: "manual" });
    await resp.body?.cancel();
    times.push(Date.now() - start);
    await wait(50);
  }
  const avg = times.reduce((s, t) => s + t, 0) / times.length;
  return { avg: Math.round(avg), max: Math.max(...times), min: Math.min(...times), n };
}