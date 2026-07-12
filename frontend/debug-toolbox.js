// DixieData debug toolbox (issue #309). A small read-only toolbox
// callable from the browser devtools console as `window.dixie.*`.
// Three witnesses for "what page am I on" exist: the breadcrumb
// (rendered by Layout from the path the server set via
// SetCurrentPagePath), the dev badge (visible to devs only), and
// this toolbox's dixie.page() function. If the three disagree
// the layout is broken; if they agree the page is correctly
// identified.
//
// All functions are read-only -- the toolbox never mutates state.
//
// installDixieDebugToolbox is called once at page load from the
// main app.js installer; it exposes window.dixie = { ... } plus
// hangs global error + fetch interceptors off the window so the
// toolbox has data to inspect.

const DIXIE_TOOLBOX_VERSION = "1";
const DIXIE_MAX_NETWORK_LOG = 50;
const DIXIE_MAX_ERROR_LOG = 30;

// recentNetwork entries: { method, path, status, startedAt, ms }.
// We attach to window.fetch via a wrapper that records each call.
// lastNetwork(n) returns the most recent n entries newest-first.
let recentNetwork = [];
let recentErrors = [];

// pathForCurrentPage computes the same crumb chain that the
// server-rendered breadcrumb shows. Mirrors the Go helper at
// internal/templates/components/breadcrumb_helpers.go +
// BreadcrumbCrumbs(). If you change the algorithm, sync the
// Go side too (the test TestBreadcrumbCrumbs_KeyRoutes pins both).
function pathCrumbsForCurrentPath(currentPath) {
  const path = currentPath || "/";
  const cleanPath = String(path).split("?")[0];
  if (!cleanPath || cleanPath === "/") {
    return [
      { label: "Home", href: "/calendar", isCurrent: false },
      { label: "Calendar", href: "/calendar", isCurrent: true },
    ];
  }
  function join(...items) {
    const crumbs = [{ label: "Home", href: "/calendar", isCurrent: false }];
    for (let i = 0; i + 2 < items.length; i += 3) {
      const label = items[i];
      const href = items[i + 1];
      const isCurrent = items[i + 2];
      if (!label) continue;
      crumbs.push({ label, href, isCurrent });
    }
    return crumbs;
  }
  function labelFromPath(p) {
    const parts = String(p).split("/");
    const last = (parts[parts.length - 1] || "").replace(/[-_]/g, " ").trim();
    if (!last) return p;
    return last
      .split(/\s+/)
      .map((w) => (w.length > 0 ? w.charAt(0).toUpperCase() + w.slice(1) : w))
      .join(" ");
  }
  function titleCaseMonth(slug) {
    if (!slug) return slug;
    return slug.charAt(0).toUpperCase() + slug.slice(1);
  }
  function jobIDLabel(p) {
    const id = String(p).replace(/^\/jobs\//, "");
    if (!id || id === "active") return "Job";
    return "Job " + id;
  }
  if (cleanPath === "/calendar") {
    return join("Calendar", "/calendar", true);
  }
  if (cleanPath.startsWith("/calendar/")) {
    const rest = cleanPath.replace(/^\/calendar\//, "");
    const parts = rest.split("/");
    if (parts.length === 1) {
      return join("Calendar", "/calendar", false, titleCaseMonth(parts[0]), cleanPath, true);
    }
    if (parts.length >= 4 && parts[1] === "anniversary") {
      return join(
        "Calendar",
        "/calendar",
        false,
        "Anniversaries",
        "/anniversary",
        false,
        titleCaseMonth(parts[0]),
        cleanPath,
        true
      );
    }
    return join("Calendar", "/calendar", false, labelFromPath(cleanPath), cleanPath, true);
  }
  if (cleanPath.startsWith("/anniversary/")) {
    const rest = cleanPath.replace(/^\/anniversary\//, "");
    const parts = rest.split("/");
    if (parts.length >= 2) {
      return join(
        "Anniversaries",
        "/anniversary",
        false,
        titleCaseMonth(parts[0]),
        cleanPath,
        false,
        parts[1],
        cleanPath,
        true
      );
    }
    if (parts.length === 1) {
      return join("Anniversaries", "/anniversary", false, titleCaseMonth(parts[0]), cleanPath, true);
    }
    return join("Anniversaries", "/anniversary", true);
  }
  if (cleanPath === "/soldiers") {
    return join("Search", "/soldiers", true);
  }
  if (cleanPath === "/soldiers/new") {
    return join("Search", "/soldiers", false, "Add Person", "/soldiers/new", true);
  }
  if (cleanPath === "/soldiers/search") {
    return join("Search", "/soldiers", false, "Search", "/soldiers/search", true);
  }
  if (cleanPath.startsWith("/soldiers/search/")) {
    return join("Search", "/soldiers", false, "Advanced Search", cleanPath, true);
  }
  if (cleanPath.startsWith("/soldiers/display/")) {
    const id = cleanPath.replace(/^\/soldiers\/display\//, "");
    return join("Search", "/soldiers", false, id, cleanPath, true);
  }
  if (
    cleanPath.startsWith("/soldiers/") &&
    cleanPath !== "/soldiers/new" &&
    !cleanPath.startsWith("/soldiers/search")
  ) {
    const rest = cleanPath.replace(/^\/soldiers\//, "");
    const parts = rest.split("/");
    const id = parts[0];
    if (parts.length >= 2 && parts[1] === "tags") {
      return join(
        "Search",
        "/soldiers",
        false,
        "#" + id,
        "/soldiers/" + id,
        false,
        "Tags",
        cleanPath,
        true
      );
    }
    return join("Search", "/soldiers", false, "#" + id, cleanPath, true);
  }
  if (cleanPath === "/browse") return join("Browse", "/browse", true);
  if (cleanPath === "/browse/results") {
    return join("Browse", "/browse", false, "Results", "/browse/results", true);
  }
  if (cleanPath === "/review-queue") return join("Review Queue", "/review-queue", true);
  if (cleanPath.startsWith("/review-queue/compare/")) {
    return join("Review Queue", "/review-queue", false, "Compare", cleanPath, true);
  }
  if (cleanPath === "/compare") return join("Compare", "/compare", true);
  if (cleanPath === "/insights") return join("Insights", "/insights", true);
  if (cleanPath === "/share") return join("Share", "/share", true);
  if (cleanPath === "/share/exports") {
    return join("Share", "/share", false, "Exports", "/share/exports", true);
  }
  if (cleanPath === "/share/imports") {
    return join("Share", "/share", false, "Imports", "/share/imports", true);
  }
  if (cleanPath === "/share/sync") {
    return join("Share", "/share", false, "Sync", "/share/sync", true);
  }
  if (cleanPath === "/share/queue") {
    return join("Share", "/share", false, "Queue", "/share/queue", true);
  }
  if (cleanPath === "/tags") return join("Tags", "/tags", true);
  if (cleanPath.startsWith("/tags/")) {
    return join("Tags", "/tags", false, "Tag", cleanPath, true);
  }
  if (cleanPath.startsWith("/settings")) return join("Settings", "/settings", true);
  if (cleanPath === "/jobs/active") return join("Jobs", "/jobs/active", true);
  if (cleanPath.startsWith("/jobs/")) {
    return join("Jobs", "/jobs/active", false, jobIDLabel(cleanPath), cleanPath, true);
  }
  if (cleanPath === "/scratchpad") return join("Scratchpad", "/scratchpad", true);
  if (cleanPath.startsWith("/feedback")) return join("Feedback", "/feedback", true);
  if (cleanPath === "/recovery") return join("Recovery", "/recovery", true);
  if (cleanPath === "/setup") return join("Setup", "/setup", true);
  return join("Home", "/calendar", false, labelFromPath(cleanPath), cleanPath, true);
}

// readShareQueueFromLocalStorage mirrors the helper pair in
// app.js (readShareQueue + writeShareQueue). Kept duplicated here
// so this file is standalone-loadable and the toolbox can run
// even before app.js has installed its globals.
function readShareQueueFromLocalStorage() {
  try {
    const raw = window.localStorage.getItem("dixiedata.share-queue");
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed : [];
  } catch (err) {
    // Issue #384 / Slice 1: surface the failure to the debug console so a
    // user whose share queue vanished has a breadcrumb to act on. The
    // toolbox is standalone-loadable (runs before app.js installs
    // showToast) so console.warn is the right channel here. Return []
    // to preserve prior behavior for the rest of the app.
    if (typeof console !== "undefined") {
      console.warn("[dixie:toolbox] readShareQueueFromLocalStorage failed", err);
    }
    return [];
  }
}

function writeShareQueueToLocalStorage(ids) {
  try {
    window.localStorage.setItem("dixiedata.share-queue", JSON.stringify(ids));
    return true;
  } catch (err) {
    // Issue #384 / Slice 1: log quota / serialization errors instead of
    // silently returning false. Callers can still inspect the boolean;
    // the log gives the user a hint that LS is full or the value is
    // not serializable.
    if (typeof console !== "undefined") {
      console.warn("[dixie:toolbox] writeShareQueueToLocalStorage failed", err);
    }
    return false;
  }
}

function readPresetsFromLocalStorage() {
  try {
    const raw = window.localStorage.getItem("dixiedata.share-queue.presets");
    if (!raw) return null;
    return JSON.parse(raw);
  } catch (err) {
    // Issue #384 / Slice 1: presets vanishing silently is the worst kind
    // of swallowed error — the user customizes once, then it disappears
    // and they have no idea why. Log so the debug-console dump surfaces it.
    if (typeof console !== "undefined") {
      console.warn("[dixie:toolbox] readPresetsFromLocalStorage failed", err);
    }
    return null;
  }
}

function readLocalSettingsFromWindow() {
  // The appshell exposes records.LocalSettings via a global set
  // by the page hydrator. Fall back to LS read so the toolbox has
  // something to show even before hydration.
  if (typeof window.__dixieLocalSettings === "object" && window.__dixieLocalSettings !== null) {
    return window.__dixieLocalSettings;
  }
  return null;
}

function readUiidsFromBody() {
  const body = document.body;
  if (!(body instanceof HTMLElement)) return null;
  const id = body.getAttribute("data-dixie-page");
  return id || null;
}

// The toolbox. All functions return arrays/objects so console.table
// gives a nice render -- never raw strings.
const dixie = {
  // dixie.help() prints the function list (one line each) and the
  // toolbox's own version. Discoverability affordance.
  help() {
    const lines = [
      "dixie.debug-toolbox v" + DIXIE_TOOLBOX_VERSION,
      "",
      "Available functions (all return JSON-friendly data):",
      "  dixie.page()        - current page: {path, label, uiidsPageId, surfaceRegistryEntry}",
      "  dixie.queue()       - share-queue contents: {ids, presets, lastModified}",
      "  dixie.lastNetwork(n?)- last N requests: [{method, path, status, ms, startedAt}]",
      "  dixie.activity()    - active background jobs: [{kind, startedAt, percent, status}]",
      "  dixie.settings()    - records.LocalSettings + Google + update prefs",
      "  dixie.storage()     - all localStorage keys + sizes",
      "  dixie.errors()      - recent error states captured by the appshell",
      "  dixie.route(path)   - parse a path into {handlerName, query, params}",
      "  dixie.breadcrumb()  - the current breadcrumb chain (3rd witness check)",
      "  dixie.help()        - this message",
    ];
    console.log(lines.join("\n"));
    return lines;
  },

  page() {
    const path = (window.location && window.location.pathname) || "/";
    const crumbs = pathCrumbsForCurrentPath(path);
    const leaf = crumbs.length > 0 ? crumbs[crumbs.length - 1] : null;
    return {
      path: path,
      label: leaf ? leaf.label : "?",
      crumbs: crumbs,
      uiidsPageId: readUiidsFromBody(),
      h1: document.querySelector("h1") ? document.querySelector("h1").textContent.trim() : null,
      title: document.title || null,
      timestamp: new Date().toISOString(),
    };
  },

  queue() {
    const ids = readShareQueueFromLocalStorage();
    const presetsRaw = readPresetsFromLocalStorage();
    return {
      ids: ids,
      count: ids.length,
      presets: presetsRaw,
      lastModified: window.localStorage.getItem("dixiedata.share-queue.mtime") || null,
      storageKey: "dixiedata.share-queue",
    };
  },

  lastNetwork(n = 10) {
    return recentNetwork.slice(0, Math.max(0, Math.min(n, DIXIE_MAX_NETWORK_LOG)));
  },

  async activity() {
    // The active jobs region (data-jobs-progress-region) is
    // fetched via GET /jobs/active every 3s; read its current
    // rendered innerHTML if present, or fall back to a fresh
    // fetch. The user's debug pill on the existing page already
    // surfaces this data; toolbox just exposes it formatted.
    const region = document.querySelector("[data-jobs-progress-region]");
    if (!(region instanceof HTMLElement)) {
      return [];
    }
    const text = region.textContent || "";
    if (text.trim() === "") return [];
    // Best-effort: parse a single-line "Job ... — 50%" form into
    // structured entries. Production active-job popups render
    // richer data but the toolbox is for triage, not analysis.
    const jobs = [];
    const matches = text.matchAll(/(Job|Export|Import|Merge):\s+([^\n—]+?)\s+—\s+(\d+)%/g);
    for (const m of matches) {
      jobs.push({ kind: m[1], title: m[2].trim(), percent: Number(m[3]) });
    }
    return jobs;
  },

  settings() {
    const local = readLocalSettingsFromWindow();
    return {
      local: local,
      google: typeof window.__dixieGoogleSettings === "object" ? window.__dixieGoogleSettings : null,
      update: typeof window.__dixieUpdateSettings === "object" ? window.__dixieUpdateSettings : null,
    };
  },

  storage() {
    const out = {};
    try {
      for (let i = 0; i < window.localStorage.length; i += 1) {
        const key = window.localStorage.key(i);
        if (typeof key !== "string") continue;
        const value = window.localStorage.getItem(key) || "";
        out[key] = { sizeBytes: value.length, preview: value.slice(0, 80) };
      }
    } catch (err) {
      return { error: String(err) };
    }
    return out;
  },

  errors() {
    return recentErrors.slice(0, DIXIE_MAX_ERROR_LOG);
  },

  route(path) {
    if (typeof path !== "string" || path.length === 0) {
      return { error: "path must be a non-empty string" };
    }
    const url = new URL(path, window.location.origin);
    const segments = url.pathname.split("/").filter(Boolean);
    return {
      path: url.pathname,
      query: Object.fromEntries(url.searchParams.entries()),
      fragments: segments,
      firstSegment: segments.length > 0 ? segments[0] : null,
      isSharePath: url.pathname.startsWith("/share/"),
      isSoldierPath: /^\/soldiers\/[0-9]+/.test(url.pathname),
    };
  },

  breadcrumb() {
    return pathCrumbsForCurrentPath(window.location.pathname || "/");
  },
};

// installDixieDebugToolbox wires the toolbox onto window + the
// interceptors that feed its data. Called once at page load by
// the main app.js installer.
function installDixieDebugToolbox() {
  if (window.dixie && window.dixie.__installed === true) {
    return;
  }
  window.dixie = dixie;
  window.dixie.__installed = true;
  window.dixie.__version = DIXIE_TOOLBOX_VERSION;

  // wrap fetch so every request feeds the network log
  const originalFetch = window.fetch ? window.fetch.bind(window) : null;
  if (originalFetch) {
    window.fetch = function patchedFetch(input, init) {
      const startedAt = Date.now();
      let method = (init && init.method) || undefined;
      if (!method && input instanceof Request) {
        method = input.method;
      }
      method = method || "GET";
      let urlPath;
      try {
        const req = input instanceof Request ? input : new Request(input, init);
        urlPath = new URL(req.url, window.location.origin).pathname;
      } catch (err) {
        urlPath = String(input);
      }
      return originalFetch(input, init).then(
        (response) => {
          recentNetwork.unshift({
            method: String(method).toUpperCase(),
            path: urlPath,
            status: response.status,
            ms: Date.now() - startedAt,
            startedAt: new Date(startedAt).toISOString(),
          });
          if (recentNetwork.length > DIXIE_MAX_NETWORK_LOG) {
            recentNetwork.length = DIXIE_MAX_NETWORK_LOG;
          }
          return response;
        },
        (err) => {
          recentNetwork.unshift({
            method: String(method).toUpperCase(),
            path: urlPath,
            status: 0,
            ms: Date.now() - startedAt,
            startedAt: new Date(startedAt).toISOString(),
            error: String(err),
          });
          throw err;
        }
      );
    };
  }

  // capture uncaught errors + promise rejections
  if (typeof window.addEventListener === "function") {
    window.addEventListener("error", (ev) => {
      recentErrors.unshift({
        kind: "error",
        message: ev.message || "(no message)",
        filename: ev.filename || null,
        lineno: ev.lineno || null,
        colno: ev.colno || null,
        stack: ev.error && ev.error.stack ? ev.error.stack : null,
        timestamp: new Date().toISOString(),
      });
      if (recentErrors.length > DIXIE_MAX_ERROR_LOG) {
        recentErrors.length = DIXIE_MAX_ERROR_LOG;
      }
    });
    window.addEventListener("unhandledrejection", (ev) => {
      const reason = ev.reason;
      recentErrors.unshift({
        kind: "unhandledrejection",
        message:
          reason && reason.message
            ? reason.message
            : typeof reason === "string"
            ? reason
            : "(no message)",
        stack: reason && reason.stack ? reason.stack : null,
        timestamp: new Date().toISOString(),
      });
      if (recentErrors.length > DIXIE_MAX_ERROR_LOG) {
        recentErrors.length = DIXIE_MAX_ERROR_LOG;
      }
    });
  }
}
