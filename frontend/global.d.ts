// DixieData Window / HTMLElement augmentation for the install-once
// marker pattern used by frontend/app.js. Each marker is hung off
// `window` or per-element as a private flag to guard idempotent
// install helpers (installFoldouts, installMegaMenus, initializeCopyPathButtons,
// initializeLiveCounts). tsc with checkJs treats these props as unknown
// by default; the augmentation makes them optional-typed so the install
// sites can read AND write without TS2339 noise.
//
// Important invariants:
//   - tsc runs with `noEmit: true` (see jsconfig.json). This file is
//     consumed by the type-checker only. It never ships in the binary.
//   - All properties are declared optional (? is included) so that
//     pre-existing window/element accesses don't type-error when the
//     runtime install hasn't run yet. Readers can fall back through
//     the truthy-check (`if (!window.__foldoutInstallN)`) that the
//     existing install code uses.
//   - The `__dixieLiveCountHandler` and `__copyPathBound` props live
//     on the augmentable HTMLInputElement / HTMLElement interface.
//     `input` in initializeLiveCounts is narrowed to HTMLInputElement
//     in the source code; the augmentation surfaces that intent
//     to the type-checker.

interface DixieDataWindow {
  __foldoutInstallN?: number;
  __foldoutProbeReinit?: () => void;
  __foldoutDocHandlerBound?: boolean;
  __foldoutBoundTriggers?: WeakSet<HTMLElement>;
  __megaMenuInstallN?: number;
  __megaMenuDocHandlerBound?: boolean;
  __megaMenuBoundTriggers?: WeakSet<HTMLElement>;
  // Issue #476 floating-nav panel (the Menu button's floating panel).
  // Same install-once marker pattern as the foldout + megamenu above.
  // Declared so tsc's checkJs stops emitting TS2339/TS2551 at the
  // installFloatingNavPanel read/write sites in app.js.
  __floatingNavInstallN?: number;
  __floatingNavBoundTriggers?: WeakSet<HTMLElement>;
  // Issue #564 slice 2: installTermDisclosures installs the
  // document-level outside-click + Escape handler once per
  // DOMContentLoaded; the installFoldouts-equivalent dedupe.
  __termDisclosureDocHandlerBound?: boolean;
  // Issue #573: shared debounce helper + the window-stashed
  // browse-filter debounce instance that owns the 200ms
  // trailing-edge filter-refresh timer. The helper attaches
  // `__dixieDebounce` from a separate <script>; consumers see
  // it as `(fn, ms) => wrapped`. The browse-filter instance
  // passes the freshest (form, url, target) as trailing-fire
  // args so re-mounts keep using the latest values.
  __dixieDebounce?: <F extends (...args: any[]) => any>(fn: F, ms: number) => ((...args: Parameters<F>) => void) & { cancel: () => void; schedule: () => void };
  __dixieBrowseFilterDebounce?: (form: HTMLFormElement, url: string, target: string) => void;
  // Issue #576: shared clipboard helper attached by
  // frontend/_lib/clipboard.js (loaded via a <script defer>
  // in index.html). Returns a Promise that resolves after
  // the write succeeds or the legacy fallback completes.
  // Both [data-copy-path] and the Markdown cheatsheet per-row
  // copy buttons route through this helper.
  __dixieCopyText?: (text: string) => Promise<void>;
  __dixieDebug?: {
    openFolder?: () => void | Promise<void>;
    copyEntries?: () => void | Promise<void>;
    [key: string]: unknown;
  };
  __dixieDebugDisabled?: boolean;
  __dixieGoogleSettings?: Record<string, unknown>;
  __dixieLocalSettings?: Record<string, unknown>;
  __dixieUpdateSettings?: Record<string, unknown>;
  // The DixieData debugger installed by frontend/debug-toolbox.js.
  // Install-once guarded via window.dixie.__installed; exposes the
  // raw runtime + cached breadcrumb + network log to the dev-tools
  // panel. The precise type isn't documented outside this file
  // (debug-toolbox.js is read-only on dev builds), so unknown is
  // the honest shape.
  dixie?: { __installed?: boolean; __version?: string } & Record<string, unknown>;
  // The DixieData frontend loads htmx via <script defer src="/htmx.min.js">.
  // The global `window.htmx` is provided by that script; it lives
  // here in the augmentation because tsc (with checkJs) cannot
  // resolve the runtime-provided property otherwise. The "loose"
  // shape mirrors the htmx 2.x type declarations — narrowed at
  // call sites (window.htmx?.on, window.htmx && typeof === "function").
  htmx?: {
    on: (event: string, handler: (evt: CustomEvent<{ elt?: unknown; xhr?: { status?: number; responseURL?: string; getResponseHeader: (name: string) => string | null }; target?: unknown }>) => void) => void;
    off: (event: string, handler?: (evt: CustomEvent<{ elt?: unknown; xhr?: { status?: number; responseURL?: string; getResponseHeader: (name: string) => string | null }; target?: unknown }>) => void) => void;
  };
  // DX-mode flag toggled by the dev page badge; default false in
  // production builds. The thin typing reflects the actual usage.
  DIXIEDATA_DEVTOOLS?: boolean;
}

declare global {
  interface Window extends DixieDataWindow {}
  interface HTMLInputElement {
    __dixieLiveCountHandler?: (this: HTMLInputElement, ev?: Event) => void;
  }
  interface HTMLTextAreaElement {
    // initializeLiveCounts walks both input and textarea DOM
    // nodes; the per-element handler is attached to whichever
    // element actually received the install. The base Element
    // declaration below covers the read sites.
    __dixieLiveCountHandler?: (this: HTMLTextAreaElement, ev?: Event) => void;
  }
  interface Element {
    // initializeCopyPathButtons attaches the copy-path click
    // handler to whatever `[data-copy-path]` resolves to in the
    // DOM (typically an HTMLButtonElement, but the source uses a
    // generic selector). Declaring the marker on Element keeps
    // every per-element read/write path type-safe without a
    // per-element-type augmentation.
    __copyPathBound?: boolean;
    // Issue #574: initializePersonRecordPicker binds once per
    // picker-clear button via this guard. Mirrors the
    // __copyPathBound pattern (idempotent install on htmx
    // re-render). Declared on Element so the picker-clear
    // querySelectorAll loop's per-element read/write is
    // type-safe under strictNullChecks.
    __pickerClearBound?: boolean;
    // Issue #576: initializeMarkdownCheatsheet binds once per
    // cheatsheet per-row copy button via this guard. Mirrors
    // the __copyPathBound / __pickerClearBound pattern.
    __cheatsheetCopyBound?: boolean;
  }
  interface HTMLElement {
    __copyPathBound?: boolean;
    __pickerClearBound?: boolean;
    __cheatsheetCopyBound?: boolean;
    // Issue #583: initializeInventoryMetricsChart paints the
    // Activity metrics SVG once per chart wrapper; the guard
    // prevents double-painting on htmx re-render. Mirrors the
    // __copyPathBound pattern (idempotent install).
    __inventoryChartPainted?: boolean;
    // Issue #564 slice 2: installTermDisclosures wires the
    // per-trigger click listener once per element. htmx:load
    // can re-install without re-attaching; the guard mirrors
    // the __copyPathBound / __cheatsheetCopyBound pattern.
    __termDisclosureWired?: boolean;
  }
}

export {};
