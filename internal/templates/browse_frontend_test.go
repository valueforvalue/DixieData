package templates

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBrowseResetClearsSavedStateBeforeRedirect(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(file)))
	appJSPath := filepath.Join(repoRoot, "frontend", "app.js")

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

	scriptPath := filepath.Join(t.TempDir(), "browse_reset_harness.js")
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
