// audit/smoke_jobs_log_location.mjs — regression probe for the
// user-reported bug:
//
//   "rename .dixiedata → .dixiedata-previous-* failed after 5
//    attempts: Access is denied"
//
// Root cause: DixieData.exe (and dixiedata-web.exe) held an open
// file handle on <dataDir>/jobs.jsonl for the lifetime of the
// process. The atomic rename replaceDataDir performs at the start
// of a .ddbak import could not proceed on Windows because of
// that descendant handle. Commit 9df785d's sibling (this fix)
// moves jobs.jsonl to <dataDir-parent>/.dixiedata-logs/jobs.jsonl
// (the same convention appdata.LogsDir uses for app.log.jsonl).
// The open handle is now outside the data dir; the rename
// succeeds.
//
// Regression net: boot dixiedata-web with a fresh scratch dir,
// wait for the jobs registry to open its append log, assert the
// file landed under .dixiedata-logs/ and NOT under the data dir.
//
// Run after dixiedata-web is built at $WEB_BIN:
//
//   WEB_BIN=... node audit/smoke_jobs_log_location.mjs
//
// Exit code is non-zero when the file lands in the wrong place.

import { spawn } from "node:child_process";
import { mkdirSync, rmSync, existsSync, readdirSync } from "node:fs";
import { renameSync } from "node:fs";

const PORT = 9921;
const SCRATCH = process.env.SCRATCH_DIR || "C:/Users/value/dixie-jobs-loglocation";
const LOGS_PARENT = SCRATCH.substring(0, SCRATCH.lastIndexOf("/"));
const WEB_BIN = process.env.WEB_BIN || "C:/Development/DixieData/build/bin/dixiedata-web.exe";

if (!existsSync(WEB_BIN)) {
  console.error("missing web binary at", WEB_BIN);
  process.exit(2);
}

rmSync(SCRATCH, { recursive: true, force: true });
mkdirSync(SCRATCH, { recursive: true });

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

const server = spawn(
  WEB_BIN,
  ["-addr", `127.0.0.1:${PORT}`, "-scratch-dir", SCRATCH],
  { stdio: ["ignore", "pipe", "pipe"] }
);
server.stderr.on("data", () => {});

let pass = 0;
let fail = 0;
function record(name, ok, details = {}) {
  if (ok) { pass++; console.log(`  ✓ ${name}`); }
  else { fail++; console.log(`  ✗ ${name}\n    ${JSON.stringify(details)}`); }
}

try {
  await ready();
  await wait(2000);

  // appdata.LogsDir(dataDir) = <parent>/.dixiedata-logs/.
  const dataDir = `${SCRATCH}/.dixiedata`;
  const logsDir = `${LOGS_PARENT}/.dixiedata-logs`;
  const dataJobsLog = `${dataDir}/jobs.jsonl`;
  const logsJobsLog = `${logsDir}/jobs.jsonl`;

  console.log("\njobs log location regression");
  console.log("----------------------------");
  console.log("dataDir:", dataDir);
  console.log("logsDir:", logsDir);
  if (existsSync(dataDir)) {
    console.log("dataDir contents:", readdirSync(dataDir));
  } else {
    console.log("dataDir does not exist yet (fresh server, no setup done)");
  }
  if (existsSync(logsDir)) {
    console.log("logsDir contents:", readdirSync(logsDir));
  } else {
    console.log("logsDir does not exist");
  }

  // Assertion 1: jobs.jsonl must live under .dixiedata-logs/ after
  // a fresh server boot (the jobs registry opens its append log
  // during App.Startup).
  record(
    "jobs-jsonl-under-logs-dir",
    existsSync(logsJobsLog),
    { expected: logsJobsLog },
  );

  // Assertion 2: jobs.jsonl must NOT live inside the data dir.
  // (Skipped if data dir doesn't exist — fresh server without
  // setup doesn't create the data dir yet.)
  if (existsSync(dataDir)) {
    record(
      "jobs-jsonl-not-in-data-dir",
      !existsSync(dataJobsLog),
      { forbidden: dataJobsLog },
    );
  } else {
    console.log(`  (assertion 2 skipped: data dir not yet created)`);
  }

  // Assertion 3: replaceDataDir-equivalent (Node fs.renameSync)
  // succeeds on the live data dir. The running server holds an
  // open handle on the jobs log file outside the data dir; the
  // rename of the data dir itself must not be blocked.
  if (existsSync(dataDir)) {
    const tmp = `${LOGS_PARENT}/dixie-jobs-loglocation-rename-test`;
    rmSync(tmp, { recursive: true, force: true });
    mkdirSync(tmp, { recursive: true });
    try {
      renameSync(dataDir, `${tmp}/.dixiedata`);
      record("data-dir-rename-with-live-handle", true, { moved: tmp });
      // Restore.
      renameSync(`${tmp}/.dixiedata`, dataDir);
    } catch (e) {
      record("data-dir-rename-with-live-handle", false, { error: e.message });
    }
  } else {
    console.log(`  (assertion 3 skipped: data dir not yet created)`);
  }

  console.log(`\n${pass} passed, ${fail} failed`);
  process.exit(fail === 0 ? 0 : 1);
} catch (e) {
  console.error("FATAL", e);
  process.exit(2);
} finally {
  server.kill();
  await wait(500);
}