// audit/_lib/config.mjs (issue #707)
//
// Canonical defaults for every audit/smoke_<surface>.mjs probe.
// The probe runner reads PROBE_PORT + SMOKE_BASE_URL env vars for
// ad-hoc overrides (e.g. the aggregator's --strict path wires
// BASE_URL to a CI-managed server). Per-probe `process.env.PROBE_PORT
// || '<fallback>'` patterns used to duplicate this -- now the
// defaults live here so the audit harness has one source of truth.
//
// Override file: audit/config.json (optional). Loaded only when
// present -- the defaults above are the "works out of the box"
// values for a fresh dev checkout. The CI runner does not ship
// a config.json; the local dev can ship one to bump timeouts on
// a slow laptop without editing every probe.
//
// Design contract:
//   - Every probe imports from this module.
//   - Defaults are chosen so that a probe run with no env vars
//     and no config.json on a developer laptop in under 2 minutes.
//   - Overrides (env > config.json > defaults) are resolved at
//     loadConfig() call time, not at import time, so test
//     runners can mutate env between probes.
//   - The module does not spawn child processes or call fetch();
//     it is safe to import from a unit-test context.

import { existsSync, readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = join(__dirname, '..', '..');
const CONFIG_FILE = join(REPO_ROOT, 'audit', 'config.json');

/**
 * @typedef {Object} AuditConfig
 * @property {string}   baseHost              Loopback host the probe server binds to (e.g. "127.0.0.1").
 * @property {number}   defaultPort           The fallback port when PROBE_PORT env var is unset. Probes
 *                                            that need a stable port (no parallel spawn) read this
 *                                            directly; probes that run in the aggregator get a
 *                                            unique port per invocation via PROBE_PORT.
 * @property {number}   portRangeBase         Inclusive low end of the port range the runner uses
 *                                            when allocating unique ports. 8774 chosen to avoid
 *                                            clashing with the dev server (default :8000) and the
 *                                            Wails IPC layer.
 * @property {number}   waitMs                Settle delay after navigation / form submit, before
 *                                            asserting against the DOM. Matches the pre-config
 *                                            value of 400ms across most probes.
 * @property {number}   bootTimeoutMs         Time the server is given to start listening on its
 *                                            port after spawn().
 * @property {number}   navTimeoutMs          Default Playwright navigation timeout. 15s covers a
 *                                            cold server boot on a slow machine.
 * @property {number}   actionTimeoutMs       Default Playwright action timeout (click / fill).
 *                                            5s covers a sluggish-but-healthy server.
 * @property {number}   responseTimeoutMs     Default waitForResponse timeout. 15s to match nav.
 * @property {number}   pageSetupMs            Time the page is given to settle after the initial
 *                                            networkidle wait (the 400ms pattern).
 * @property {number}   pageTearDownMs        Time the page is given to settle after a final
 *                                            action before the test reports results.
 * @property {number}   seedSoldiers          Default seed-data -soldiers value. 3 is enough for
 *                                            form-contract probes; 5 is enough for browse-paginate
 *                                            probes that span 3 pages with page_size=2.
 * @property {string}   testIdentityName      First + middle + last name stamped by the
 *                                            ConfigureUserIdentity call inside the seed.
 * @property {string[]} seedIdentityArgs      Identity args for ConfigureUserIdentity. Single source
 *                                            of truth for the Test Researcher / 1990 fixed
 *                                            identity that every probe relies on.
 */

/** @type {AuditConfig} */
const DEFAULTS = Object.freeze({
  baseHost: '127.0.0.1',
  defaultPort: 8782,
  portRangeBase: 8774,
  waitMs: 250,
  bootTimeoutMs: 15_000,
  navTimeoutMs: 15_000,
  actionTimeoutMs: 5_000,
  responseTimeoutMs: 15_000,
  pageSetupMs: 400,
  pageTearDownMs: 600,
  seedSoldiers: 3,
  testIdentityFirst: 'Test',
  testIdentityMiddle: 'Smoke',
  testIdentityLast: 'Researcher',
  testIdentityBirthYear: 1990,
  binaryName: 'dixiedata-web.exe',
  binaryDir: 'build/bin',
});

/**
 * Resolve the audit config by merging (in increasing priority):
 *   1. DEFAULTS
 *   2. audit/config.json (if present)
 *   3. process.env overrides
 *
 * Returns a frozen object. The merge is shallow because the
 * config surface is flat by design; if a future override
 * file needs nesting, the loader is the only place to change.
 *
 * @returns {AuditConfig}
 */
export function loadConfig() {
  const fromFile = readJsonIfPresent(CONFIG_FILE) ?? {};
  const fromEnv = readEnvOverrides();
  const merged = { ...DEFAULTS, ...fromFile, ...fromEnv };
  return Object.freeze(merged);
}

function readJsonIfPresent(path) {
  if (!existsSync(path)) return null;
  try {
    const raw = readFileSync(path, 'utf8');
    const parsed = JSON.parse(raw);
    if (parsed === null || typeof parsed !== 'object') {
      throw new Error(`audit/config.json must be a JSON object, got ${typeof parsed}`);
    }
    return parsed;
  } catch (err) {
    throw new Error(`failed to parse ${path}: ${err.message}`);
  }
}

function readEnvOverrides() {
  const out = {};
  if (process.env.SMOKE_BASE_URL) {
    try {
      const url = new URL(process.env.SMOKE_BASE_URL);
      out.baseHost = url.hostname;
      if (url.port) out.defaultPort = parseInt(url.port, 10);
    } catch (err) {
      throw new Error(`SMOKE_BASE_URL is not a valid URL: ${process.env.SMOKE_BASE_URL}`);
    }
  }
  if (process.env.PROBE_PORT) {
    const n = parseInt(process.env.PROBE_PORT, 10);
    if (!Number.isFinite(n) || n <= 0 || n > 65535) {
      throw new Error(`PROBE_PORT must be a positive integer <= 65535, got ${process.env.PROBE_PORT}`);
    }
    out.defaultPort = n;
  }
  if (process.env.SMOKE_BOOT_TIMEOUT_MS) {
    out.bootTimeoutMs = parsePositiveInt(process.env.SMOKE_BOOT_TIMEOUT_MS, 'SMOKE_BOOT_TIMEOUT_MS');
  }
  if (process.env.SMOKE_NAV_TIMEOUT_MS) {
    out.navTimeoutMs = parsePositiveInt(process.env.SMOKE_NAV_TIMEOUT_MS, 'SMOKE_NAV_TIMEOUT_MS');
  }
  if (process.env.SMOKE_ACTION_TIMEOUT_MS) {
    out.actionTimeoutMs = parsePositiveInt(process.env.SMOKE_ACTION_TIMEOUT_MS, 'SMOKE_ACTION_TIMEOUT_MS');
  }
  if (process.env.SMOKE_RESPONSE_TIMEOUT_MS) {
    out.responseTimeoutMs = parsePositiveInt(process.env.SMOKE_RESPONSE_TIMEOUT_MS, 'SMOKE_RESPONSE_TIMEOUT_MS');
  }
  if (process.env.SMOKE_WAIT_MS) {
    out.waitMs = parsePositiveInt(process.env.SMOKE_WAIT_MS, 'SMOKE_WAIT_MS');
  }
  if (process.env.SMOKE_SEED_SOLDIERS) {
    out.seedSoldiers = parsePositiveInt(process.env.SMOKE_SEED_SOLDIERS, 'SMOKE_SEED_SOLDIERS');
  }
  return out;
}

function parsePositiveInt(value, name) {
  const n = parseInt(value, 10);
  if (!Number.isFinite(n) || n <= 0) {
    throw new Error(`${name} must be a positive integer, got ${value}`);
  }
  return n;
}

/**
 * Resolve the base URL the probe should use to talk to the
 * server. Honors PROBE_PORT > SMOKE_BASE_URL > the loaded
 * config's defaultPort. Returns a `http://host:port/` URL with
 * a trailing slash so `new URL('/soldiers', base).href` resolves
 * correctly.
 *
 * @param {AuditConfig} cfg
 * @returns {string}
 */
export function resolveBaseUrl(cfg) {
  if (process.env.PROBE_PORT) {
    const port = parseInt(process.env.PROBE_PORT, 10);
    if (process.env.SMOKE_BASE_URL) {
      // Both env vars set: PROBE_PORT wins on port, SMOKE_BASE_URL
      // wins on host. This lets a CI runner point at a non-loopback
      // server while still overriding the port (e.g. for port-forwarding).
      const url = new URL(process.env.SMOKE_BASE_URL);
      return `http://${url.hostname}:${port}/`;
    }
    return `http://${cfg.baseHost}:${port}/`;
  }
  if (process.env.SMOKE_BASE_URL) {
    // SMOKE_BASE_URL already includes the trailing slash; pass through
    // but normalize to a single trailing slash so callers can do
    // `new URL('/path', base)` reliably.
    return process.env.SMOKE_BASE_URL.endsWith('/')
      ? process.env.SMOKE_BASE_URL
      : process.env.SMOKE_BASE_URL + '/';
  }
  return `http://${cfg.baseHost}:${cfg.defaultPort}/`;
}

/**
 * Resolve the test identity fields the seed-data helper should
 * stamp. Returns an object with the four ConfigureUserIdentity
 * fields (first/middle/last/birthYear). The seed-data's identity
 * stamp is the canonical test identity; this helper is the
 * single place that reads the env override (so a future
 * "swap the test identity per suite" feature has one entry point).
 *
 * @param {AuditConfig} cfg
 * @returns {{firstName: string, middleName: string, lastName: string, birthYear: number}}
 */
export function resolveTestIdentity(cfg) {
  return {
    firstName: process.env.SMOKE_TEST_IDENTITY_FIRST ?? cfg.testIdentityFirst,
    middleName: process.env.SMOKE_TEST_IDENTITY_MIDDLE ?? cfg.testIdentityMiddle,
    lastName: process.env.SMOKE_TEST_IDENTITY_LAST ?? cfg.testIdentityLast,
    birthYear: process.env.SMOKE_TEST_IDENTITY_BIRTH_YEAR
      ? parseInt(process.env.SMOKE_TEST_IDENTITY_BIRTH_YEAR, 10)
      : cfg.testIdentityBirthYear,
  };
}

/**
 * Allocate a unique port for a probe. The runner calls this
 * once at the top of each probe so probes can be safely run in
 * parallel by the aggregator (or by `just test-smoke-fast`)
 * without two probes trying to bind the same port. The base is
 * the config's portRangeBase; the offset is the probe's index
 * in the SURFACES[] array. The first probe gets the base port,
 * each subsequent probe gets +1.
 *
 * @param {AuditConfig} cfg
 * @param {number} offset   Zero-based index of the probe in the run queue.
 * @returns {number}
 */
export function allocatePort(cfg, offset) {
  return cfg.portRangeBase + offset;
}

export const DEFAULT_CONFIG = DEFAULTS;
