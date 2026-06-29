// Shared cleanup helper for audit/probe-*.mjs scripts.
//
// Why this exists:
//   Probe scripts spawn `build/bin/dixiedata-web.exe` (and sometimes
//   `build/bin/seed-data.exe`) as child processes. Two failure modes
//   leak these binaries:
//     1. The user hits Ctrl-C. Node's default SIGINT handler calls
//        process.exit() immediately, so any `finally` block that
//        would have killed the child never runs.
//     2. The probe script throws before its `finally` block, or the
//        parent `go run` re-spawns a grandchild that SIGTERM doesn't
//        reach (Windows process tree quirk).
//   When a probe leaks, the next `make debug` or `make audit` step
//   that rebuilds `dixiedata-web.exe` fails with:
//     `unlinkat ... dixiedata-web.exe: The process cannot access the
//      file because it is being used by another process.`
//
// Usage:
//   import { registerCleanup, runWithCleanup } from './_lib/cleanup.mjs';
//
//   const proc = spawn('build/bin/dixiedata-web.exe', [...args], opts);
//   registerCleanup({ proc, processNames: ['dixiedata-web.exe'] });
//   // ... do work ...
//
//   // Or wrap the entire script body:
//   runWithCleanup(async ({ registerCleanup }) => {
//     const proc = spawn(...);
//     registerCleanup({ proc, processNames: ['dixiedata-web.exe'] });
//     // ... do work ...
//   });

import { spawn } from 'node:child_process';
import { setTimeout as sleep } from 'node:timers/promises';

let registered = false;
let cleanupArgs = null;

/**
 * Kill a process tree by exe name. Cross-platform: Windows uses
 * taskkill /F /T (which walks the process tree), POSIX uses pkill -9.
 */
function killByName(processName) {
  return new Promise((resolve) => {
    let killer;
    if (process.platform === 'win32') {
      killer = spawn('taskkill', ['/F', '/IM', processName, '/T'], { stdio: 'ignore' });
    } else {
      killer = spawn('pkill', ['-9', '-f', processName], { stdio: 'ignore' });
    }
    killer.on('exit', () => resolve());
    killer.on('error', () => resolve()); // pkill missing on minimal POSIX — ignore
    // Safety net: never hang more than 5s on a stuck taskkill.
    setTimeout(resolve, 5000).unref();
  });
}

/**
 * Best-effort termination of the registered proc plus any extra
 * process names. Idempotent — safe to call from multiple paths
 * (signal handler, finally block, error path).
 */
async function runCleanup() {
  if (!cleanupArgs || cleanupArgs.done) return;
  cleanupArgs.done = true;

  const { proc, processNames = [], extraCleanup } = cleanupArgs;

  // 1. Politely ask the direct child to exit. On Windows, SIGTERM
  //    is largely a no-op for native children, but it doesn't hurt.
  if (proc && !proc.killed) {
    try { proc.kill('SIGTERM'); } catch (_) { /* already dead */ }
  }
  await sleep(400);

  // 2. Nuke by exe name — covers the grandchild (e.g. go run's
  //    actual exe) and any siblings the probe spawned (seed-data).
  for (const name of processNames) {
    await killByName(name);
  }
  await sleep(200);

  // 3. Hard-kill the direct child if it's still alive.
  if (proc && !proc.killed) {
    try { proc.kill('SIGKILL'); } catch (_) { /* already dead */ }
  }

  // 4. Caller-supplied extra cleanup hook.
  if (typeof extraCleanup === 'function') {
    try { await extraCleanup(); } catch (_) { /* best effort */ }
  }
}

/**
 * Install process-wide handlers so SIGINT / SIGTERM / uncaught
 * exceptions all run the cleanup before exit. After cleanup
 * completes the process exits with code 130 (standard "interrupted"
 * convention) for signals, or 1 for uncaught exceptions.
 */
export function registerCleanup({ proc, processNames = [], extraCleanup } = {}) {
  if (registered) {
    // Merge — last call wins for proc, union for processNames.
    cleanupArgs.proc = proc ?? cleanupArgs.proc;
    cleanupArgs.processNames = [...new Set([...cleanupArgs.processNames, ...processNames])];
    if (extraCleanup) cleanupArgs.extraCleanup = extraCleanup;
    return;
  }
  registered = true;
  cleanupArgs = { proc, processNames, extraCleanup, done: false };

  let exiting = false;
  const exitAfter = (code) => {
    if (exiting) return;
    exiting = true;
    runCleanup().finally(() => process.exit(code));
  };

  process.on('SIGINT', () => exitAfter(130));
  process.on('SIGTERM', () => exitAfter(143));
  process.on('SIGHUP', () => exitAfter(129));

  // Uncaught errors should still kill the child before bailing.
  process.on('uncaughtException', (err) => {
    console.error('UNCAUGHT:', err);
    exitAfter(1);
  });
  process.on('unhandledRejection', (reason) => {
    console.error('UNHANDLED REJECTION:', reason);
    exitAfter(1);
  });

  // Normal exit — run cleanup so we don't leak on a clean run either.
  process.on('exit', () => {
    if (cleanupArgs && !cleanupArgs.done) {
      // Synchronous fast path: no awaits here, but taskkill is
      // already in flight so the OS will reap on its own.
      try { runCleanup(); } catch (_) {}
    }
  });
}

/**
 * Wrap the entire probe script body. Returns the script's exit code
 * after cleanup completes. Convenience over `registerCleanup` for
 * scripts that want one-line integration.
 */
export async function runWithCleanup(scriptFn) {
  let exitCode = 0;
  let context = { registerCleanup };
  try {
    await scriptFn(context);
  } catch (e) {
    console.error('FATAL:', e);
    exitCode = 2;
  } finally {
    await runCleanup();
  }
  return exitCode;
}