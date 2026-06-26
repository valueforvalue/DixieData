// DixieData frontend debug capture.
// Loaded BEFORE app.js via <script defer src="/debug.js"></script> in
// layout.templ. Self-installing: hooks window.console, window.onerror,
// window.onunhandledrejection. Self-throttling: batch POSTs every 2 s
// OR when buffer reaches 50 entries OR 32 KB; flushes via sendBeacon
// on beforeunload only (to avoid duplicate log entries).
//
// Public surface:
//   window.__dixieDebug = { buffer, flush, push, install, openFolder, clear, setEnabled }
//
// Kill switch: window.__dixieDebugDisabled = true (set before this script runs).

(function () {
  'use strict';

  if (window.__dixieDebugDisabled) return;
  if (window.__dixieDebug) return; // already installed

  const FLUSH_INTERVAL_MS = 2000;
  const FLUSH_THRESHOLD = 50;
  const MAX_BUFFER = 500;
  // sendBeacon payload cap is ~64 KB on most browsers; batch below
  // 32 KB to leave headroom and avoid silent drops.
  const MAX_BEACON_BYTES = 32 * 1024;
  const ENDPOINT = '/debug/client-logs';

  let buffer = [];
  let flushTimer = null;
  let enabled = true;
  let payloadBytes = 0;

  function nowIso() {
    try { return new Date().toISOString(); } catch (_) { return ''; }
  }

  function push(level, args) {
    if (!enabled) return;
    if (buffer.length >= MAX_BUFFER) {
      // Drop oldest to bound memory; keep recent 80%.
      buffer.splice(0, buffer.length - Math.floor(MAX_BUFFER * 0.8));
    }
    let msg = '';
    let stack = '';
    try {
      msg = args.map(function (a) {
        if (a instanceof Error) return a.message;
        if (typeof a === 'string') return a;
        try { return JSON.stringify(a); } catch (_) { return String(a); }
      }).join(' ');
    } catch (_) { msg = '[unserializable]'; }
    if (args[0] && args[0] instanceof Error) {
      stack = args[0].stack || '';
    }
    const entry = {
      ts: nowIso(),
      level: level,
      msg: msg,
      stack: stack,
      url: window.location ? window.location.pathname + window.location.search : '',
    };
    buffer.push(entry);
    payloadBytes += (msg.length || 0) + (stack.length || 0) + 80;
    if (buffer.length >= FLUSH_THRESHOLD || payloadBytes >= MAX_BEACON_BYTES) {
      flush();
    } else if (!flushTimer) {
      flushTimer = setTimeout(flush, FLUSH_INTERVAL_MS);
    }
  }

  function flush() {
    if (flushTimer) { clearTimeout(flushTimer); flushTimer = null; }
    if (buffer.length === 0) return;
    const payload = JSON.stringify({ entries: buffer });
    buffer = [];
    payloadBytes = 0;
    try {
      fetch(ENDPOINT, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: payload,
        keepalive: true,
      }).catch(function () { /* swallow; debug only */ });
    } catch (_) { /* ignore */ }
  }

  // sendBeacon on beforeunload only. Bound by MAX_BEACON_BYTES (entries
  // are split across beacons if needed). Falls back to keepalive fetch
  // when sendBeacon returns false (over quota, etc.).
  function beaconFlush() {
    if (buffer.length === 0) return;
    while (buffer.length > 0) {
      const batch = [];
      let batchBytes = 64;
      while (buffer.length > 0) {
        const entry = buffer[0];
        const entryBytes = (entry.msg.length || 0) + (entry.stack.length || 0) + 80;
        if (batchBytes + entryBytes > MAX_BEACON_BYTES && batch.length > 0) break;
        batch.push(buffer.shift());
        batchBytes += entryBytes;
      }
      try {
        const payload = JSON.stringify({ entries: batch });
        const blob = new Blob([payload], { type: 'application/json' });
        if (!(navigator.sendBeacon && navigator.sendBeacon(ENDPOINT, blob))) {
          fetch(ENDPOINT, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: payload,
            keepalive: true,
          }).catch(function () { /* swallow */ });
        }
      } catch (_) { /* ignore */ }
    }
    payloadBytes = 0;
  }

  function installConsoleHook(method, level) {
    const original = console[method] ? console[method].bind(console) : function () {};
    console[method] = function () {
      try { push(level, Array.prototype.slice.call(arguments)); }
      catch (_) { /* never let logging break app code */ }
      return original.apply(console, arguments);
    };
  }
  installConsoleHook('log', 'info');
  installConsoleHook('info', 'info');
  installConsoleHook('warn', 'warn');
  installConsoleHook('error', 'error');
  installConsoleHook('debug', 'debug');

  window.addEventListener('error', function (ev) {
    push('error', [ev.message || 'window.error', ev.error || '']);
  });
  window.addEventListener('unhandledrejection', function (ev) {
    const reason = ev.reason;
    if (reason instanceof Error) {
      push('error', ['Unhandled rejection: ' + reason.message, reason]);
    } else {
      push('error', ['Unhandled rejection: ' + String(reason)]);
    }
  });

  // beforeunload only (no pagehide / visibilitychange to avoid duplicates).
  window.addEventListener('beforeunload', beaconFlush);

  window.__dixieDebug = {
    buffer: function () { return buffer.slice(); },
    flush: flush,
    push: push,
    openFolder: function () {
      fetch('/debug/open-folder', { method: 'GET' }).catch(function () {});
    },
    clear: function () {
      fetch('/debug/console/clear', { method: 'POST' }).catch(function () {});
    },
    setEnabled: function (v) { enabled = !!v; },
  };
})();