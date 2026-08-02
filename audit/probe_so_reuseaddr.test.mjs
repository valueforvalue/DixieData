// audit/probe_so_reuseaddr.test.mjs
//
// Regression net for issue #708: dixiedata-web must set
// SO_REUSEADDR on the TCP listener so back-to-back probe
// invocations against the same port don't fail with WSAEACCES
// (10013) while the previous server's socket is still in
// TIME_WAIT.
//
// The Windows TCP stack holds a bound port in TIME_WAIT for
// 30-60s after the listener closes. With the stdlib's default
// listener (no SO_REUSEADDR), the next process that tries to
// bind the same port gets an error and the audit probe crashes
// before its assertions run.
//
// What the test does:
//   1. Boot dixiedata-web on a unique port.
//   2. GET /  (sanity: server is up).
//   3. SIGTERM the server, wait for the OS to release the
//      socket, immediately boot a second dixiedata-web on
//      the SAME port.
//   4. GET /  on the second server (sanity: it came up).
//   5. Assert the second boot did not log "listen ... : bind
//      ..." or any WSAEACCES / EADDRINUSE error.
//
// Why a Node test (not a Go unit test):
//   - The fix lives in cmd/dixiedata-web/main.go which has no
//     Go test files (the binary is an entry point, not a lib).
//   - Other listener-behavior tests in the repo follow the same
//     "spawn the binary, observe behavior" pattern
//     (audit/smoke_*.mjs uses spawn from node:child_process).
//   - A Node test keeps the toolchain single-pass for the
//     audit harness.
//
// Failure mode being prevented:
//   Without SO_REUSEADDR on Windows, `just test-smoke-strict`
//   intermittently fails because the second probe (soldier-images
//   on 8774 in the legacy ordering, or 8774+index in the #707
//   ordering) tries to bind a port still held in TIME_WAIT by
//   the previous probe. The aggregator reports the probe as
//   "fail: 1, exit 1" with stderr "bind: ... WSAEACCES (10013)".
//   The fix is to set SO_REUSEADDR on the listener before
//   Serve() so Windows allows the rebind.

import { spawn } from 'node:child_process';
import { existsSync } from 'node:fs';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { setTimeout as sleep } from 'node:timers/promises';
import { strict as assert } from 'node:assert';
import { webBin } from './_lib/smoke_paths.mjs';

const WEB_BIN = webBin();
if (!existsSync(WEB_BIN)) {
  console.error(`probe_so_reuseaddr: web binary missing at ${WEB_BIN}; build it with \`go build -o ${WEB_BIN} ./cmd/dixiedata-web\` (or \`just web\`) before running this test.`);
  process.exit(2);
}

// Use a high random port to avoid colliding with anything the
// audit aggregator or the developer has bound. Range: 49000-
// 50000 is rarely used and not in the default 8774+ range.
const PORT = 49000 + Math.floor(Math.random() * 1000);
const ADDR = `127.0.0.1:${PORT}`;

async function bootOnce(label) {
  const scratch = mkdtempSync(join(tmpdir(), `probe-so-reuseaddr-${label}-`));
  const proc = spawn(WEB_BIN, ['-addr', ADDR, '-scratch-dir', scratch], {
    stdio: ['ignore', 'pipe', 'pipe'],
    env: { ...process.env, DIXIEDATA_DATA_DIR: scratch },
  });
  let stdout = '';
  let stderr = '';
  proc.stdout.on('data', (d) => { stdout += d; });
  proc.stderr.on('data', (d) => { stderr += d; });

  // Wait up to 10s for the server to come up. The probe fetches
  // /calendar which is a real route in the app.
  let up = false;
  for (let i = 0; i < 40; i++) {
    if (proc.exitCode !== null) break;
    try {
      const r = await fetch(`http://${ADDR}/calendar`);
      if (r.status < 500) { up = true; break; }
    } catch {}
    await sleep(250);
  }
  if (!up) {
    proc.kill();
    rmSync(scratch, { recursive: true, force: true });
    throw new Error(`${label} server did not come up on ${ADDR}; stderr=${stderr.slice(-400)}`);
  }
  return { proc, scratch, stdout, stderr };
}

async function main() {
  console.log(`probe_so_reuseaddr: target addr = ${ADDR}`);

  const first = await bootOnce('first');
  console.log('probe_so_reuseaddr: first server up, killing…');
  first.proc.kill('SIGTERM');
  // Wait for the process to actually exit + the socket to
  // transition into TIME_WAIT. On Windows the socket stays
  // bound for ~30s after close; we don't wait that long — the
  // whole point of SO_REUSEADDR is that we don't have to.
  await new Promise((resolve) => first.proc.on('exit', resolve));
  rmSync(first.scratch, { recursive: true, force: true });

  // Immediate rebind on the same port. With SO_REUSEADDR this
  // succeeds; without it, the second boot's listener errors
  // out and the process exits within a few hundred ms.
  const second = await bootOnce('second');
  console.log('probe_so_reuseaddr: second server up on the same port — SO_REUSEADDR is working.');

  // Also assert the second server's stderr doesn't carry any
  // bind error. bootOnce throws if it can't reach /, but a
  // bind error in stderr would still be visible even if some
  // other retry path got lucky. Be strict.
  assert.doesNotMatch(
    second.stderr,
    /WSAEACCES|EADDRINUSE|bind:.*forbidden|bind:.*already in use/i,
    `second server stderr must not carry a bind error; SO_REUSEADDR is not set or is failing. saw=${JSON.stringify(second.stderr.slice(-400))}`,
  );

  second.proc.kill('SIGTERM');
  await new Promise((resolve) => second.proc.on('exit', resolve));
  rmSync(second.scratch, { recursive: true, force: true });

  // If bootOnce returned for the second server, the rebind
  // succeeded. The earlier failure mode (no SO_REUSEADDR on
  // Windows) is that the second Listen returns WSAEACCES and
  // the process log.Fatalfs before the /calendar GET would
  // ever succeed — so bootOnce throws long before we reach
  // this line. The end-of-test stderr sanity check is
  // informational (the server may not have flushed its log
  // buffer yet, as we saw in early runs); the structural
  // assertion is that bootOnce('second') did not throw.

  console.log('probe_so_reuseaddr: 2 passed, 0 failed');
}

main().catch((err) => {
  console.error('probe_so_reuseaddr: FAIL', err);
  process.exit(1);
});