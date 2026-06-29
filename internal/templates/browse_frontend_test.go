package templates

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func runBrowseFrontendHarness(t *testing.T, script string) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(file)))
	appJSPath := filepath.Join(repoRoot, "frontend", "app.js")

	scriptPath := filepath.Join(t.TempDir(), "browse_frontend_harness.js")
	if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cmd := exec.Command("node", scriptPath)
	cmd.Env = append(os.Environ(), "APP_JS_PATH="+appJSPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node harness failed: %v\n%s", err, strings.TrimSpace(string(output)))
	}
}

func TestBrowseResetClearsSavedStateBeforeRedirect(t *testing.T) {
	script := `
const fs = require("fs");
const vm = require("vm");

class HTMLElement {
  constructor() {
    this._attrs = new Map();
    this.classList = { toggle() {}, add() {}, remove() {} };
  }
  setAttribute(name, value) { this._attrs.set(name, String(value)); }
  getAttribute(name) { return this._attrs.has(name) ? this._attrs.get(name) : null; }
  hasAttribute(name) { return this._attrs.has(name); }
  removeAttribute(name) { this._attrs.delete(name); }
  querySelectorAll() { return []; }
  querySelector() { return null; }
  appendChild() {}
  matches() { return false; }
  closest(selector) {
    if (selector === "[data-browse-reset]" && this.hasAttribute("data-browse-reset")) {
      return this;
    }
    return null;
  }
}
class HTMLButtonElement extends HTMLElement {}
class HTMLAnchorElement extends HTMLElement {}
class HTMLInputElement extends HTMLElement {}
class HTMLTextAreaElement extends HTMLElement {}
class HTMLSelectElement extends HTMLElement {}
class HTMLFormElement extends HTMLElement {}
class StorageMock {
  constructor(seed = {}) { this.map = new Map(Object.entries(seed)); }
  getItem(key) { return this.map.has(key) ? this.map.get(key) : null; }
  setItem(key, value) { this.map.set(key, String(value)); }
  removeItem(key) { this.map.delete(key); }
}

const listeners = {};
const documentMock = {
  body: new HTMLElement(),
  addEventListener(name, handler) { listeners[name] = handler; },
  querySelector() { return null; },
  querySelectorAll() { return []; },
  getElementById() { return null; },
  createElement() { return new HTMLElement(); },
};

let assignedPath = "";
const windowMock = {
  document: documentMock,
  localStorage: new StorageMock({
    "dixiedata.browse.state": JSON.stringify({ entry_type: "soldier", unit: "Alabama Cavalry" }),
  }),
  sessionStorage: new StorageMock(),
  location: { pathname: "/browse", search: "", hash: "", origin: "http://localhost" },
  history: { replaceState() {} },
  requestAnimationFrame(fn) { fn(); },
  addEventListener() {},
  removeEventListener() {},
  getSelection() { return { toString: () => "" }; },
  scrollTo() {},
};
windowMock.location.assign = (path) => { assignedPath = path; };

Object.assign(global, {
  window: windowMock,
  document: documentMock,
  HTMLElement,
  HTMLButtonElement,
  HTMLAnchorElement,
  HTMLInputElement,
  HTMLTextAreaElement,
  HTMLSelectElement,
  HTMLFormElement,
  MutationObserver: class MutationObserver { observe() {} disconnect() {} },
  URLSearchParams,
  fetch: async () => ({ ok: false, text: async () => "" }),
  console,
  setTimeout,
  clearTimeout,
});

global.location = windowMock.location;
global.history = windowMock.history;
global.localStorage = windowMock.localStorage;
global.sessionStorage = windowMock.sessionStorage;
global.requestAnimationFrame = windowMock.requestAnimationFrame;

vm.runInThisContext(fs.readFileSync(process.env.APP_JS_PATH, "utf8"), { filename: process.env.APP_JS_PATH });

const resetButton = new HTMLButtonElement();
resetButton.setAttribute("data-browse-reset", "");
resetButton.setAttribute("data-browse-reset-path", "/browse");
listeners.click({
  target: resetButton,
  defaultPrevented: false,
  preventDefault() { this.defaultPrevented = true; },
});

if (windowMock.localStorage.getItem("dixiedata.browse.state") !== null) {
  throw new Error("browse state was not cleared");
}
if (assignedPath !== "/browse") {
  throw new Error("reset did not redirect to /browse");
}
`

	runBrowseFrontendHarness(t, script)
}

func TestBrowseInitialLoadRestoresDraftFiltersWithoutAutoApplyingThem(t *testing.T) {
	script := `
const fs = require("fs");
const vm = require("vm");

class HTMLElement {
  constructor() {
    this._attrs = new Map();
    this.children = [];
    this.parentElement = null;
    this.classList = { toggle() {}, add() {}, remove() {}, contains() { return false; } };
    this.style = {};
    this.value = "";
    this.name = "";
  }
  setAttribute(name, value) { this._attrs.set(name, String(value)); }
  getAttribute(name) { return this._attrs.has(name) ? this._attrs.get(name) : null; }
  hasAttribute(name) { return this._attrs.has(name); }
  removeAttribute(name) { this._attrs.delete(name); }
  querySelectorAll() { return []; }
  querySelector() { return null; }
  appendChild(child) { child.parentElement = this; this.children.push(child); }
  matches(selector) {
    if (selector === "[data-browse-filter-input]") {
      return this.hasAttribute("data-browse-filter-input");
    }
    return false;
  }
  closest(selector) {
    if (selector === "[data-browse-filter-input]" && this.hasAttribute("data-browse-filter-input")) {
      return this;
    }
    if (selector === "form") {
      return this.parentElement instanceof HTMLFormElement ? this.parentElement : null;
    }
    return null;
  }
  addEventListener() {}
  focus() {}
  scrollIntoView() {}
}
class HTMLButtonElement extends HTMLElement {}
class HTMLAnchorElement extends HTMLElement {}
class HTMLInputElement extends HTMLElement {
  constructor() {
    super();
    this.type = "text";
  }
}
class HTMLTextAreaElement extends HTMLElement {}
class HTMLSelectElement extends HTMLElement {}
class HTMLFormElement extends HTMLElement {
  constructor() {
    super();
    this._fields = new Map();
    this.elements = {
      namedItem: (name) => this._fields.get(name) || null,
    };
  }
  register(field) {
    field.parentElement = this;
    this._fields.set(field.name, field);
    return field;
  }
  querySelector(selector) {
    if (selector === "[data-browse-page-input]") {
      for (const field of this._fields.values()) {
        if (field.hasAttribute("data-browse-page-input")) {
          return field;
        }
      }
    }
    return null;
  }
}
class File {}
class FormData {
  constructor(form) {
    this.entriesList = [];
    if (form instanceof HTMLFormElement) {
      for (const field of form._fields.values()) {
        this.entriesList.push([field.name, field.value]);
      }
    }
  }
  append(key, value) {
    this.entriesList.push([key, value]);
  }
  get(key) {
    for (const [entryKey, value] of this.entriesList) {
      if (entryKey === key) {
        return value;
      }
    }
    return null;
  }
  entries() {
    return this.entriesList[Symbol.iterator]();
  }
  values() {
    return this.entriesList.map(([, value]) => value)[Symbol.iterator]();
  }
}
class StorageMock {
  constructor(seed = {}) { this.map = new Map(Object.entries(seed)); }
  getItem(key) { return this.map.has(key) ? this.map.get(key) : null; }
  setItem(key, value) { this.map.set(key, String(value)); }
  removeItem(key) { this.map.delete(key); }
}

const listeners = {};
const page = new HTMLElement();
page.setAttribute("data-browse-page", "");
page.setAttribute("data-browse-current-page", "1");
const results = new HTMLElement();
const form = new HTMLFormElement();
form.id = "browse-filters";
form.setAttribute("hx-get", "/browse/results");
form.setAttribute("hx-target", "#browse-results");

function registerField(name, value, attrs = {}) {
  const field = attrs.select ? new HTMLSelectElement() : new HTMLInputElement();
  field.name = name;
  field.value = value;
  for (const [attr, attrValue] of Object.entries(attrs)) {
    if (attr === "select") {
      continue;
    }
    field.setAttribute(attr, attrValue);
  }
  form.register(field);
  return field;
}

const pageField = registerField("page", "1", { "data-browse-page-input": "" });
registerField("page_size", "100", { select: true });
registerField("scope", "all", { select: true });
registerField("sort", "display_id_asc", { select: true });
registerField("entry_type", "", { select: true });
const unitField = registerField("unit", "", { "data-browse-filter-input": "" });
registerField("buried_in", "", { "data-browse-filter-input": "" });
registerField("pension_state", "", { "data-browse-filter-input": "" });
registerField("review_status", "", { select: true, "data-browse-filter-input": "" });
registerField("confederate_home_status", "", { select: true, "data-browse-filter-input": "" });

const documentMock = {
  body: new HTMLElement(),
  addEventListener(name, handler) {
    listeners[name] = listeners[name] || [];
    listeners[name].push(handler);
  },
  querySelector(selector) {
    if (selector === "[data-browse-page]") {
      return page;
    }
    if (selector === "#browse-results") {
      return results;
    }
    return null;
  },
  querySelectorAll() { return []; },
  getElementById(id) {
    if (id === "browse-filters") {
      return form;
    }
    return null;
  },
  createElement() { return new HTMLElement(); },
};

let fetchCalls = 0;
const windowMock = {
  document: documentMock,
  localStorage: new StorageMock({
    "dixiedata.browse.state": JSON.stringify({ entry_type: "soldier", unit: "Alabama Cavalry" }),
  }),
  sessionStorage: new StorageMock(),
  location: { pathname: "/browse", search: "", hash: "", origin: "http://localhost" },
  history: { replaceState() {} },
  scrollX: 0,
  scrollY: 0,
  requestAnimationFrame(fn) { fn(); },
  addEventListener() {},
  removeEventListener() {},
  getSelection() { return { toString: () => "" }; },
  scrollTo() {},
};
windowMock.location.assign = () => {};

Object.assign(global, {
  window: windowMock,
  document: documentMock,
  Node: HTMLElement,
  HTMLElement,
  HTMLButtonElement,
  HTMLAnchorElement,
  HTMLInputElement,
  HTMLTextAreaElement,
  HTMLSelectElement,
  HTMLFormElement,
  FormData,
  File,
  MutationObserver: class MutationObserver { observe() {} disconnect() {} },
  URLSearchParams,
  fetch: async (url) => {
    if (!url || !url.includes("bootstrap")) {
      fetchCalls += 1;
    }
    return {
      ok: true,
      redirected: false,
      text: async () => "<div></div>",
      headers: { get() { return null; } },
    };
  },
  console,
  setTimeout,
  clearTimeout,
});

global.location = windowMock.location;
global.history = windowMock.history;
global.localStorage = windowMock.localStorage;
global.sessionStorage = windowMock.sessionStorage;
global.requestAnimationFrame = windowMock.requestAnimationFrame;

vm.runInThisContext(fs.readFileSync(process.env.APP_JS_PATH, "utf8"), { filename: process.env.APP_JS_PATH });

for (const handler of listeners.DOMContentLoaded || []) {
  handler({});
}

if (fetchCalls !== 0) {
  throw new Error("browse init should not auto-apply saved filters");
}
if (unitField.value !== "Alabama Cavalry") {
  throw new Error("browse init did not restore saved filter draft");
}
if (pageField.value !== "1") {
  throw new Error("browse init should keep the current page until Apply Filters is used");
}
`

	runBrowseFrontendHarness(t, script)
}

func TestBrowseFilterChangeAutoAppliesAndPersistsDraft(t *testing.T) {
	script := `
(async () => {
const fs = require("fs");
const vm = require("vm");

class HTMLElement {
  constructor() {
    this._attrs = new Map();
    this.children = [];
    this.parentElement = null;
    this.classList = { toggle() {}, add() {}, remove() {}, contains() { return false; } };
    this.style = {};
    this.value = "";
    this.name = "";
  }
  setAttribute(name, value) { this._attrs.set(name, String(value)); }
  getAttribute(name) { return this._attrs.has(name) ? this._attrs.get(name) : null; }
  hasAttribute(name) { return this._attrs.has(name); }
  removeAttribute(name) { this._attrs.delete(name); }
  querySelectorAll() { return []; }
  querySelector() { return null; }
  appendChild(child) { child.parentElement = this; this.children.push(child); }
  matches(selector) {
    if (selector === "[data-browse-filter-input]") {
      return this.hasAttribute("data-browse-filter-input");
    }
    if (selector === "form[data-pdf-pref-scope]") {
      return false;
    }
    return false;
  }
  closest(selector) {
    if (selector === "[data-browse-filter-input]" && this.hasAttribute("data-browse-filter-input")) {
      return this;
    }
    if (selector === "form") {
      return this.parentElement instanceof HTMLFormElement ? this.parentElement : null;
    }
    return null;
  }
  addEventListener() {}
  focus() {}
  scrollIntoView() {}
}
class HTMLButtonElement extends HTMLElement {}
class HTMLAnchorElement extends HTMLElement {}
class HTMLInputElement extends HTMLElement {
  constructor() {
    super();
    this.type = "text";
  }
}
class HTMLTextAreaElement extends HTMLElement {}
class HTMLSelectElement extends HTMLElement {}
class HTMLFormElement extends HTMLElement {
  constructor() {
    super();
    this._fields = new Map();
    this.elements = {
      namedItem: (name) => this._fields.get(name) || null,
    };
  }
  register(field) {
    field.parentElement = this;
    this._fields.set(field.name, field);
    return field;
  }
  querySelector(selector) {
    if (selector === "[data-browse-page-input]") {
      for (const field of this._fields.values()) {
        if (field.hasAttribute("data-browse-page-input")) {
          return field;
        }
      }
    }
    return null;
  }
}
class File {}
class FormData {
  constructor(form) {
    this.entriesList = [];
    if (form instanceof HTMLFormElement) {
      for (const field of form._fields.values()) {
        this.entriesList.push([field.name, field.value]);
      }
    }
  }
  append(key, value) {
    this.entriesList.push([key, value]);
  }
  get(key) {
    for (const [entryKey, value] of this.entriesList) {
      if (entryKey === key) {
        return value;
      }
    }
    return null;
  }
  entries() {
    return this.entriesList[Symbol.iterator]();
  }
  values() {
    return this.entriesList.map(([, value]) => value)[Symbol.iterator]();
  }
}
class StorageMock {
  constructor(seed = {}) { this.map = new Map(Object.entries(seed)); }
  getItem(key) { return this.map.has(key) ? this.map.get(key) : null; }
  setItem(key, value) { this.map.set(key, String(value)); }
  removeItem(key) { this.map.delete(key); }
}

const listeners = {};
const page = new HTMLElement();
page.setAttribute("data-browse-page", "");
page.setAttribute("data-browse-current-page", "1");
const results = new HTMLElement();
const form = new HTMLFormElement();
form.id = "browse-filters";
form.setAttribute("hx-get", "/browse/results");
form.setAttribute("hx-target", "#browse-results");

function registerField(name, value, attrs = {}) {
  const field = attrs.select ? new HTMLSelectElement() : new HTMLInputElement();
  field.name = name;
  field.value = value;
  for (const [attr, attrValue] of Object.entries(attrs)) {
    if (attr === "select") {
      continue;
    }
    field.setAttribute(attr, attrValue);
  }
  form.register(field);
  return field;
}

const pageField = registerField("page", "3", { "data-browse-page-input": "" });
registerField("page_size", "100", { select: true });
registerField("scope", "all", { select: true });
registerField("sort", "display_id_asc", { select: true });
registerField("entry_type", "", { select: true, "data-browse-filter-input": "" });
const unitField = registerField("unit", "", { "data-browse-filter-input": "" });
registerField("buried_in", "", { "data-browse-filter-input": "" });
registerField("pension_state", "", { "data-browse-filter-input": "" });
registerField("review_status", "", { select: true, "data-browse-filter-input": "" });
registerField("confederate_home_status", "", { select: true, "data-browse-filter-input": "" });

const documentMock = {
  body: new HTMLElement(),
  addEventListener(name, handler) {
    listeners[name] = listeners[name] || [];
    listeners[name].push(handler);
  },
  querySelector(selector) {
    if (selector === "[data-browse-page]") {
      return page;
    }
    if (selector === "#browse-results") {
      return results;
    }
    return null;
  },
  querySelectorAll() { return []; },
  getElementById(id) {
    if (id === "browse-filters") {
      return form;
    }
    return null;
  },
  createElement() { return new HTMLElement(); },
};

let fetchCalls = 0;
const windowMock = {
  document: documentMock,
  localStorage: new StorageMock(),
  sessionStorage: new StorageMock(),
  location: { pathname: "/browse", search: "", hash: "", origin: "http://localhost" },
  history: { replaceState() {} },
  scrollX: 0,
  scrollY: 0,
  requestAnimationFrame(fn) { fn(); },
  addEventListener() {},
  removeEventListener() {},
  getSelection() { return { toString: () => "" }; },
  scrollTo() {},
};
windowMock.location.assign = () => {};
windowMock.setTimeout = setTimeout;
windowMock.clearTimeout = clearTimeout;

Object.assign(global, {
  window: windowMock,
  document: documentMock,
  Node: HTMLElement,
  HTMLElement,
  HTMLButtonElement,
  HTMLAnchorElement,
  HTMLInputElement,
  HTMLTextAreaElement,
  HTMLSelectElement,
  HTMLFormElement,
  FormData,
  File,
  MutationObserver: class MutationObserver { observe() {} disconnect() {} },
  URLSearchParams,
  fetch: async (url) => {
    if (!url || !url.includes("bootstrap")) {
      fetchCalls += 1;
    }
    return {
      ok: true,
      redirected: false,
      text: async () => "<div></div>",
      headers: { get() { return null; } },
    };
  },
  console,
  setTimeout,
  clearTimeout,
});

global.location = windowMock.location;
global.history = windowMock.history;
global.localStorage = windowMock.localStorage;
global.sessionStorage = windowMock.sessionStorage;
global.requestAnimationFrame = windowMock.requestAnimationFrame;

vm.runInThisContext(fs.readFileSync(process.env.APP_JS_PATH, "utf8"), { filename: process.env.APP_JS_PATH });

for (const handler of listeners.DOMContentLoaded || []) {
  handler({});
}

unitField.value = "Alabama Cavalry";
for (const handler of listeners.change || []) {
  handler({ target: unitField });
}

// Drain pending timers so the 200ms debounce in the browse-filter
// change handler has a chance to fire before we assert. The debounce
// exists so rapid filter changes (typing in a select) don't fire
// a fetch storm.
await new Promise((resolve) => setImmediate(resolve));
await new Promise((resolve) => setTimeout(resolve, 300));

// Behavior change (2026-06-27): browse filter changes now auto-apply.
// The change handler in app.js debounces + fires the form's hx-get
// /browse/results request after 200ms. Before this change the filters
// only saved draft state and required an explicit submit; users
// reported "browse alphabetically doesn't load results" and "filter
// changes don't refresh the table" as bugs. The smoke test in
// audit/smoke.mjs covers the new auto-apply behavior end-to-end.
if (fetchCalls !== 1) {
  throw new Error("changing browse filters should auto-apply once after debounce; got fetchCalls=" + fetchCalls);
}
if (pageField.value !== "1") {
  throw new Error("changing browse filters should reset the pending page to 1");
}
const saved = JSON.parse(windowMock.localStorage.getItem("dixiedata.browse.state") || "{}");
if (saved.unit !== "Alabama Cavalry") {
  throw new Error("changing browse filters should persist the draft state");
}
})().catch((err) => { console.error(err); process.exit(1); });
`

	runBrowseFrontendHarness(t, script)
}
