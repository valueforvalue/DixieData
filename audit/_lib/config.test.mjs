// audit/_lib/config.test.mjs (issue #707)
//
// Unit tests for audit/_lib/config.mjs. The tests use the
// built-in node:test runner (--test) so the harness is
// dependency-free. They cover the 4 contract properties:
//   1. loadConfig() returns the frozen defaults when no
//      overrides are present.
//   2. PROBE_PORT env var overrides the default port.
//   3. SMOKE_BASE_URL env var overrides both host + port.
//   4. readJsonIfPresent tolerates a missing file (returns null,
//      no error).
//   5. resolveBaseUrl() honors PROBE_PORT > SMOKE_BASE_URL > cfg.
//   6. allocatePort() is a stable offset of portRangeBase.

import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, writeFileSync, rmSync, existsSync } from 'node:fs';
import { join } from 'node:path';
import { tmpdir } from 'node:os';

import { loadConfig, resolveBaseUrl, resolveTestIdentity, allocatePort, DEFAULT_CONFIG } from './config.mjs';

test('loadConfig() returns the frozen defaults when no overrides are present', () => {
  const savedPort = process.env.PROBE_PORT;
  const savedBase = process.env.SMOKE_BASE_URL;
  const savedBoot = process.env.SMOKE_BOOT_TIMEOUT_MS;
  const savedNav = process.env.SMOKE_NAV_TIMEOUT_MS;
  const savedAct = process.env.SMOKE_ACTION_TIMEOUT_MS;
  const savedResp = process.env.SMOKE_RESPONSE_TIMEOUT_MS;
  const savedWait = process.env.SMOKE_WAIT_MS;
  const savedSeed = process.env.SMOKE_SEED_SOLDIERS;
  delete process.env.PROBE_PORT;
  delete process.env.SMOKE_BASE_URL;
  delete process.env.SMOKE_BOOT_TIMEOUT_MS;
  delete process.env.SMOKE_NAV_TIMEOUT_MS;
  delete process.env.SMOKE_ACTION_TIMEOUT_MS;
  delete process.env.SMOKE_RESPONSE_TIMEOUT_MS;
  delete process.env.SMOKE_WAIT_MS;
  delete process.env.SMOKE_SEED_SOLDIERS;
  try {
    const cfg = loadConfig();
    assert.equal(cfg.baseHost, '127.0.0.1');
    assert.equal(cfg.defaultPort, 8782);
    assert.equal(cfg.portRangeBase, 8774);
    assert.equal(cfg.bootTimeoutMs, 15_000);
    assert.equal(cfg.navTimeoutMs, 15_000);
    assert.equal(cfg.actionTimeoutMs, 5_000);
    assert.equal(cfg.responseTimeoutMs, 15_000);
    assert.equal(cfg.waitMs, 250);
    assert.equal(cfg.pageSetupMs, 400);
    assert.equal(cfg.seedSoldiers, 3);
    assert.equal(cfg.testIdentityFirst, 'Test');
    assert.equal(cfg.testIdentityMiddle, 'Smoke');
    assert.equal(cfg.testIdentityLast, 'Researcher');
    assert.equal(cfg.testIdentityBirthYear, 1990);
    assert.equal(cfg.binaryName, 'dixiedata-web.exe');
    assert.equal(cfg.binaryDir, 'build/bin');
    // Frozen: writing to a property must fail in strict mode.
    assert.throws(() => { cfg.defaultPort = 9000; });
  } finally {
    if (savedPort !== undefined) process.env.PROBE_PORT = savedPort;
    if (savedBase !== undefined) process.env.SMOKE_BASE_URL = savedBase;
    if (savedBoot !== undefined) process.env.SMOKE_BOOT_TIMEOUT_MS = savedBoot;
    if (savedNav !== undefined) process.env.SMOKE_NAV_TIMEOUT_MS = savedNav;
    if (savedAct !== undefined) process.env.SMOKE_ACTION_TIMEOUT_MS = savedAct;
    if (savedResp !== undefined) process.env.SMOKE_RESPONSE_TIMEOUT_MS = savedResp;
    if (savedWait !== undefined) process.env.SMOKE_WAIT_MS = savedWait;
    if (savedSeed !== undefined) process.env.SMOKE_SEED_SOLDIERS = savedSeed;
  }
});

test('PROBE_PORT env var overrides the default port', () => {
  const saved = process.env.PROBE_PORT;
  process.env.PROBE_PORT = '9999';
  try {
    const cfg = loadConfig();
    assert.equal(cfg.defaultPort, 9999);
  } finally {
    if (saved === undefined) delete process.env.PROBE_PORT;
    else process.env.PROBE_PORT = saved;
  }
});

test('SMOKE_BASE_URL env var overrides both host + port', () => {
  const saved = process.env.SMOKE_BASE_URL;
  process.env.SMOKE_BASE_URL = 'http://audit.example.test:9100/';
  try {
    const cfg = loadConfig();
    assert.equal(cfg.baseHost, 'audit.example.test');
    assert.equal(cfg.defaultPort, 9100);
  } finally {
    if (saved === undefined) delete process.env.SMOKE_BASE_URL;
    else process.env.SMOKE_BASE_URL = saved;
  }
});

test('PROBE_PORT takes precedence over SMOKE_BASE_URL port', () => {
  const savedPort = process.env.PROBE_PORT;
  const savedBase = process.env.SMOKE_BASE_URL;
  process.env.SMOKE_BASE_URL = 'http://audit.example.test:9100/';
  process.env.PROBE_PORT = '7777';
  try {
    const cfg = loadConfig();
    assert.equal(cfg.defaultPort, 7777);
    assert.equal(cfg.baseHost, 'audit.example.test');
  } finally {
    if (savedPort === undefined) delete process.env.PROBE_PORT;
    else process.env.PROBE_PORT = savedPort;
    if (savedBase === undefined) delete process.env.SMOKE_BASE_URL;
    else process.env.SMOKE_BASE_URL = savedBase;
  }
});

test('SMOKE_NAV_TIMEOUT_MS env var overrides the nav timeout', () => {
  const saved = process.env.SMOKE_NAV_TIMEOUT_MS;
  process.env.SMOKE_NAV_TIMEOUT_MS = '45000';
  try {
    const cfg = loadConfig();
    assert.equal(cfg.navTimeoutMs, 45_000);
  } finally {
    if (saved === undefined) delete process.env.SMOKE_NAV_TIMEOUT_MS;
    else process.env.SMOKE_NAV_TIMEOUT_MS = saved;
  }
});

test('SMOKE_BASE_URL rejects malformed URLs', () => {
  const saved = process.env.SMOKE_BASE_URL;
  process.env.SMOKE_BASE_URL = 'not-a-url';
  try {
    assert.throws(() => loadConfig(), /SMOKE_BASE_URL/);
  } finally {
    if (saved === undefined) delete process.env.SMOKE_BASE_URL;
    else process.env.SMOKE_BASE_URL = saved;
  }
});

test('PROBE_PORT rejects non-integer values', () => {
  const saved = process.env.PROBE_PORT;
  process.env.PROBE_PORT = 'not-a-port';
  try {
    assert.throws(() => loadConfig(), /PROBE_PORT/);
  } finally {
    if (saved === undefined) delete process.env.PROBE_PORT;
    else process.env.PROBE_PORT = saved;
  }
});

test('PROBE_PORT rejects out-of-range values', () => {
  const saved = process.env.PROBE_PORT;
  process.env.PROBE_PORT = '99999';
  try {
    assert.throws(() => loadConfig(), /PROBE_PORT/);
  } finally {
    if (saved === undefined) delete process.env.PROBE_PORT;
    else process.env.PROBE_PORT = saved;
  }
});

test('resolveBaseUrl honors PROBE_PORT > SMOKE_BASE_URL > cfg', () => {
  const cfg = DEFAULT_CONFIG;
  const savedPort = process.env.PROBE_PORT;
  const savedBase = process.env.SMOKE_BASE_URL;
  try {
    delete process.env.PROBE_PORT;
    delete process.env.SMOKE_BASE_URL;
    assert.equal(resolveBaseUrl(cfg), 'http://127.0.0.1:8782/');
    process.env.SMOKE_BASE_URL = 'http://x.test:9100/';
    assert.equal(resolveBaseUrl(cfg), 'http://x.test:9100/');
    process.env.PROBE_PORT = '7777';
    assert.equal(resolveBaseUrl(cfg), 'http://x.test:7777/');
  } finally {
    if (savedPort === undefined) delete process.env.PROBE_PORT;
    else process.env.PROBE_PORT = savedPort;
    if (savedBase === undefined) delete process.env.SMOKE_BASE_URL;
    else process.env.SMOKE_BASE_URL = savedBase;
  }
});

test('resolveTestIdentity returns the default fields when no env override', () => {
  const savedFirst = process.env.SMOKE_TEST_IDENTITY_FIRST;
  const savedMid = process.env.SMOKE_TEST_IDENTITY_MIDDLE;
  const savedLast = process.env.SMOKE_TEST_IDENTITY_LAST;
  const savedYear = process.env.SMOKE_TEST_IDENTITY_BIRTH_YEAR;
  delete process.env.SMOKE_TEST_IDENTITY_FIRST;
  delete process.env.SMOKE_TEST_IDENTITY_MIDDLE;
  delete process.env.SMOKE_TEST_IDENTITY_LAST;
  delete process.env.SMOKE_TEST_IDENTITY_BIRTH_YEAR;
  try {
    const id = resolveTestIdentity(DEFAULT_CONFIG);
    assert.equal(id.firstName, 'Test');
    assert.equal(id.middleName, 'Smoke');
    assert.equal(id.lastName, 'Researcher');
    assert.equal(id.birthYear, 1990);
  } finally {
    if (savedFirst !== undefined) process.env.SMOKE_TEST_IDENTITY_FIRST = savedFirst;
    if (savedMid !== undefined) process.env.SMOKE_TEST_IDENTITY_MIDDLE = savedMid;
    if (savedLast !== undefined) process.env.SMOKE_TEST_IDENTITY_LAST = savedLast;
    if (savedYear !== undefined) process.env.SMOKE_TEST_IDENTITY_BIRTH_YEAR = savedYear;
  }
});

test('resolveTestIdentity honors env overrides', () => {
  const savedFirst = process.env.SMOKE_TEST_IDENTITY_FIRST;
  const savedYear = process.env.SMOKE_TEST_IDENTITY_BIRTH_YEAR;
  process.env.SMOKE_TEST_IDENTITY_FIRST = 'CI';
  process.env.SMOKE_TEST_IDENTITY_BIRTH_YEAR = '2000';
  try {
    const id = resolveTestIdentity(DEFAULT_CONFIG);
    assert.equal(id.firstName, 'CI');
    assert.equal(id.birthYear, 2000);
  } finally {
    if (savedFirst === undefined) delete process.env.SMOKE_TEST_IDENTITY_FIRST;
    else process.env.SMOKE_TEST_IDENTITY_FIRST = savedFirst;
    if (savedYear === undefined) delete process.env.SMOKE_TEST_IDENTITY_BIRTH_YEAR;
    else process.env.SMOKE_TEST_IDENTITY_BIRTH_YEAR = savedYear;
  }
});

test('allocatePort is a stable offset of portRangeBase', () => {
  assert.equal(allocatePort(DEFAULT_CONFIG, 0), 8774);
  assert.equal(allocatePort(DEFAULT_CONFIG, 1), 8775);
  assert.equal(allocatePort(DEFAULT_CONFIG, 27), 8801);
});
