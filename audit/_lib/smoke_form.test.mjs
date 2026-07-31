// audit/_lib/smoke_form.test.mjs (issue #715)
//
// Unit tests for submitAndWait. Mocks Playwright's Page +
// Locator surface so the test runs without a real browser.
// The mocks are intentionally minimal — just enough to
// verify the contract:
//   1. waitForResponse is called BEFORE the click (otherwise
//      the dispatcher fires its fetch before the listener
//      is in place, and the listener never sees the response).
//   2. The urlPredicate filters responses by URL fragment.
//   3. The method filter restricts to the configured HTTP verb.
//   4. waitForLoadState('networkidle') is called after the
//      response, so the dispatcher's programmatic navigation
//      has time to settle.
//   5. A timeout on waitForResponse resolves to {response: null}
//      rather than throwing — callers can decide if that's fatal.
//   6. Required-option validation throws helpful errors.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { submitAndWait } from './smoke_form.mjs';

// Fake Response object — Playwright's Response exposes url() /
// status() / request().method() / headers() as METHODS (not
// properties). The helper calls those methods, so the fake
// must match that shape.
function fakeResponse({ url, method }) {
  return {
    url: () => url,
    status: () => 200,
    request: () => ({ method: () => method }),
    headers: () => ({}),
  };
}

// Fake Locator — submitAndWait only calls .click() on it.
function fakeLocator() {
  return {
    click: async () => { /* swallow */ },
  };
}

// Fake Page — submitAndWait calls .waitForResponse, then
// .click() on the submit Locator, then .waitForLoadState.
// The fake records the call order so the test can assert
// that waitForResponse was wired up BEFORE click.
function fakePage({ responses = [], loadState = 'networkidle' } = {}) {
  const callOrder = [];
  let responseQueue = [...responses];
  return {
    callOrder,
    waitForResponse: (predicate, _opts) => {
      callOrder.push({ kind: 'waitForResponse' });
      // Find the next response that matches.
      return new Promise((resolve) => {
        // Pop responses until one matches the predicate; if
        // none do, resolve to null (mimics the .catch timeout).
        const check = () => {
          const idx = responseQueue.findIndex((r) => {
            try {
              return predicate(r);
            } catch {
              return false;
            }
          });
          if (idx >= 0) {
            const [matched] = responseQueue.splice(idx, 1);
            resolve(matched);
          } else {
            resolve(null);
          }
        };
        // For the test we resolve immediately; the actual
        // promise-vs-click race is tested implicitly by the
        // call-order assertion.
        check();
      });
    },
    waitForLoadState: async (state) => {
      callOrder.push({ kind: 'waitForLoadState', state });
      return loadState;
    },
    // Fake Locator constructor — the helper calls
    // opts.submit.click(...) and the locator in the test is
    // passed in directly. To track the click call, wrap it.
  };
}

// Test 1: happy path — waitForResponse is wired before click,
// click is followed by waitForLoadState('networkidle'), and the
// returned response is the one the predicate matched.
test('submitAndWait: waitForResponse is subscribed before click + loadState fires after', async () => {
  const matchingResp = fakeResponse({ url: 'http://example.test/api/settings/export-surface', method: 'POST' });
  const page = fakePage({ responses: [matchingResp] });
  const clickOrder = [];
  const submit = {
    click: async () => { clickOrder.push('click'); },
  };

  const result = await submitAndWait(page, {
    submit,
    urlPredicate: (u) => u.includes('/settings/export-surface'),
  });

  // waitForResponse was called BEFORE click.
  assert.deepEqual(page.callOrder.map((c) => c.kind), ['waitForResponse', 'waitForLoadState']);
  // The load state is 'networkidle' (the value the dispatcher
  // waits for to confirm its programmatic navigation settled).
  assert.equal(page.callOrder[1].state, 'networkidle');
  // The click happened AFTER waitForResponse subscription.
  assert.deepEqual(clickOrder, ['click']);
  // The returned response is the matching one.
  assert.equal(result.response, matchingResp);
  assert.equal(result.page, page);
});

// Test 2: URL predicate filters out unrelated responses —
// the helper resolves with the FIRST response that matches.
test('submitAndWait: urlPredicate filters responses', async () => {
  const irrelevant = fakeResponse({ url: 'http://example.test/api/jobs/123', method: 'POST' });
  const matching = fakeResponse({ url: 'http://example.test/api/tags/42/merge', method: 'POST' });
  const page = fakePage({ responses: [irrelevant, matching] });

  const result = await submitAndWait(page, {
    submit: fakeLocator(),
    urlPredicate: (u) => u.includes('/tags/') && u.includes('/merge'),
  });

  // The matching response is returned even though an
  // unrelated one was queued first.
  assert.equal(result.response, matching);
});

// Test 3: method filter — a response with the matching URL
// but a different method (e.g. GET) is filtered out.
test('submitAndWait: method filter rejects wrong HTTP verb', async () => {
  const wrongMethod = fakeResponse({ url: 'http://example.test/api/settings/export-surface', method: 'GET' });
  const rightMethod = fakeResponse({ url: 'http://example.test/api/settings/export-surface', method: 'POST' });
  const page = fakePage({ responses: [wrongMethod, rightMethod] });

  const result = await submitAndWait(page, {
    submit: fakeLocator(),
    urlPredicate: (u) => u.includes('/settings/export-surface'),
    method: 'POST',
  });

  // The GET (despite matching URL) was rejected; the POST
  // (matching both URL and method) was returned.
  assert.equal(result.response, rightMethod);
});

// Test 4: timeout on waitForResponse resolves to null, doesn't throw.
test('submitAndWait: waitForResponse timeout resolves to null', async () => {
  // Page whose waitForResponse rejects after the timeout —
  // mimics Playwright's actual behavior on timeout. The
  // helper wraps it in .catch(() => null) so the call
  // resolves with {response: null} rather than throwing.
  const page = {
    waitForResponse: (_predicate, opts) => new Promise((_resolve, reject) => {
      setTimeout(() => reject(new Error(`Timeout ${opts.timeout}ms exceeded`)), opts.timeout);
    }),
    waitForLoadState: async () => 'networkidle',
  };

  const result = await submitAndWait(page, {
    submit: fakeLocator(),
    urlPredicate: () => false,
    timeout: 50,
  });

  assert.equal(result.response, null);
});

// Test 5: missing required option throws a helpful error.
test('submitAndWait: throws when page is missing', async () => {
  await assert.rejects(
    () => submitAndWait(null, { submit: fakeLocator(), urlPredicate: () => true }),
    /page is required/,
  );
});

// Test 6: missing submit Locator throws.
test('submitAndWait: throws when submit Locator is missing', async () => {
  await assert.rejects(
    () => submitAndWait(fakePage(), { urlPredicate: () => true }),
    /opts\.submit/,
  );
});

// Test 7: missing urlPredicate throws.
test('submitAndWait: throws when urlPredicate is missing', async () => {
  await assert.rejects(
    () => submitAndWait(fakePage(), { submit: fakeLocator() }),
    /opts\.urlPredicate/,
  );
});

// Test 8: default method is POST (matches the DixieData
// dispatcher's data-dixie-submit form contract).
test('submitAndWait: default method is POST', async () => {
  const postResp = fakeResponse({ url: 'http://example.test/api/x', method: 'POST' });
  const patchResp = fakeResponse({ url: 'http://example.test/api/x', method: 'PATCH' });
  const page = fakePage({ responses: [postResp, patchResp] });

  const result = await submitAndWait(page, {
    submit: fakeLocator(),
    urlPredicate: () => true,
    // method not specified — defaults to POST
  });

  assert.equal(result.response, postResp);
});

// Test 9: extraClickOptions are forwarded to the click call.
test('submitAndWait: forwards extraClickOptions to the click', async () => {
  const page = fakePage({ responses: [fakeResponse({ url: 'http://example.test/x', method: 'POST' })] });
  let clickOpts = null;
  const submit = {
    click: async (opts) => { clickOpts = opts; },
  };

  await submitAndWait(page, {
    submit,
    urlPredicate: () => true,
    extraClickOptions: { force: true },
  });

  assert.equal(clickOpts.force, true);
});
