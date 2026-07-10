// audit/_lib/paths.mjs — shared path-resolution helpers for audit
// probes. Issue #448. The setup probes previously hardcoded
// "C:/Users/value/..." for both the data dir and the web-test
// binary path, which made them CI-broken (they couldn't run on any
// machine other than the original author's). This helper
// parameterizes both with env-var overrides + sensible defaults
// that work in CI and on developer machines.

// resolveWebTestBin returns the path to the dixiedata-web-test
// binary. Order of precedence:
//   1. process.env.DIXIE_WEB_TEST_BIN (explicit override)
//   2. discovered on PATH via `which` / `where`
//   3. conventional build output: build/bin/dixiedata-web-test.exe
//
// Returns the path string. Throws if the binary does not exist
// at the resolved location.
export function resolveWebTestBin() {
  const explicit = process.env.DIXIE_WEB_TEST_BIN;
  if (explicit && existsSync(explicit)) {
    return explicit;
  }
  // Try PATH discovery.
  const discovered = discoverOnPath("dixiedata-web-test");
  if (discovered) {
    return discovered;
  }
  // Conventional build output relative to the repo root.
  const repoRoot = findRepoRoot();
  const conventional = path.join(
    repoRoot,
    "build",
    "bin",
    process.platform === "win32" ? "dixiedata-web-test.exe" : "dixiedata-web-test",
  );
  if (existsSync(conventional)) {
    return conventional;
  }
  throw new Error(
    `dixiedata-web-test binary not found. Set DIXIE_WEB_TEST_BIN or build to build/bin/.`,
  );
}

// resolveProbeDataDir returns a writable scratch directory for a
// given probe name. Order of precedence:
//   1. process.env.DIXIE_PROBE_DATA_DIR (explicit override)
//   2. process.env.TMPDIR / os.tmpdir() + "dixie-<name>-<pid>"
//
// Returns the path string. Caller is responsible for ensuring
// the directory exists (mkdirSync recursive).
export function resolveProbeDataDir(probeName) {
  const explicit = process.env.DIXIE_PROBE_DATA_DIR;
  if (explicit) {
    return explicit;
  }
  const base = process.env.TMPDIR || os.tmpdir();
  return path.join(base, `dixie-${probeName}-${process.pid}-${Date.now()}`);
}

// resolveWebBin is the non-test variant for probes that exercise
// the full DixieData.exe (e.g. smoke_jobs_log_location.mjs).
// Same precedence as resolveWebTestBin but for dixiedata-web.
export function resolveWebBin() {
  const explicit = process.env.DIXIE_WEB_BIN || process.env.WEB_BIN;
  if (explicit && existsSync(explicit)) {
    return explicit;
  }
  const discovered = discoverOnPath("dixiedata-web");
  if (discovered) {
    return discovered;
  }
  const repoRoot = findRepoRoot();
  const conventional = path.join(
    repoRoot,
    "build",
    "bin",
    process.platform === "win32" ? "dixiedata-web.exe" : "dixiedata-web",
  );
  if (existsSync(conventional)) {
    return conventional;
  }
  throw new Error(
    `dixiedata-web binary not found. Set DIXIE_WEB_BIN or build to build/bin/.`,
  );
}

// --- internal helpers ---

import { existsSync } from "node:fs";
import { execSync } from "node:child_process";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

function discoverOnPath(binaryName) {
  try {
    const cmd = process.platform === "win32" ? "where" : "which";
    const out = execSync(`${cmd} ${binaryName}`, { encoding: "utf8", stdio: ["ignore", "pipe", "ignore"] });
    const first = out.split(/\r?\n/).find((line) => line.trim().length > 0);
    return first ? first.trim() : null;
  } catch {
    return null;
  }
}

// findRepoRoot walks up from this module until it finds a
// go.mod file. Used so the conventional build/bin/ default
// works regardless of where the probe is invoked from.
function findRepoRoot() {
  let dir = path.dirname(fileURLToPath(import.meta.url));
  for (let i = 0; i < 8; i++) {
    if (existsSync(path.join(dir, "go.mod"))) {
      return dir;
    }
    const parent = path.dirname(dir);
    if (parent === dir) {
      break;
    }
    dir = parent;
  }
  // Fallback: cwd.
  return process.cwd();
}