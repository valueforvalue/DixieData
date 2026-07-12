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
  __dixieBrowseFilterTimer?: ReturnType<typeof setTimeout>;
  __dixieDebug?: unknown;
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
  }
  interface HTMLElement {
    __copyPathBound?: boolean;
  }
}

export {};
