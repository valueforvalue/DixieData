# TypeScript baseline — frontend/app.js + friends

The DixieData frontend has zero TypeScript today. `frontend/app.js` (6079 lines,
245 functions) and `frontend/debug-toolbox.js` are served raw from disk by
the Go asset server (`internal/appshell/lifecycle.go:288` →
`handleFrontendAsset`). No bundler, no transpiler, no sourcemap. The only
JS-adjacent build step is `npm run build:css` for Tailwind.

This note captures the **typecheck baseline** at the slice-1 commit so future
agents have a stable reference point. The numbers here are what
`npm run typecheck` reports against the freshly-introduced `jsconfig.json`.

## Goal (recap)

Enable cheap type-checking via TypeScript's `checkJs` mode **without introducing
a build step**. `app.js` continues to be served raw. Editors get
`lib: ["ES2022", "DOM", "DOM.Iterable"]` for signatures on `Element`,
`HTMLFormElement`, `URLSearchParams`, `localStorage`, `addEventListener`, etc.
CI gets `make lint-typecheck` as an observability signal — not a strict gate
on day 1.

The runtime path is unchanged: `<script defer src="/app.js">` → file on disk
→ browser.

## Configuration

- `tsc` is a `devDependency` (~`typescript@^7.0.2`).
- `jsconfig.json` mirrors `tsconfig.json`'s `compilerOptions` shape; `noEmit`
  keeps tsc purely a checker.
- All `strict*` flags are explicitly `false` on day 1. The whole-file
  `lib.dom` typecheck is the headline win, not strict-correctness sweep.
  Promoting to strict-mode is its own slice.
- `include`: `frontend/**/*.js`. `exclude`: `frontend/eslint-plugin-dixie/**`
  (Node plugin code, not browser) and `frontend/wailsjs/**` (generated
  bindings owned by the Wails CLI).

## Baseline error counts (slice-1 commit)

| File                             | Errors |
| -------------------------------- | -----: |
| `frontend/app.js`                |    117 |
| `frontend/debug-toolbox.js`      |     48 |
| `frontend/debug.js`              |      3 |
| **Total**                        | **168** |

Top error categories (sampled from `app.js`; `debug-toolbox.js` skews
similarly):

| TS code | Meaning                                       | Count |
| ------- | --------------------------------------------- | ----: |
| TS2339  | Property does not exist on inferred type      |   145 |
| TS2554  | Argument-count mismatch at call site          |    20 |
| TS2304  | Cannot find name (undeclared identifier)      |     2 |
| TS2740  | Missing-property cast (`Element` → subclass)  |     1 |
| TS2345  | Argument type not assignable to parameter     |     1 |

The TS2339 cluster splits further into two patterns:

1. **`Element` not narrowed to `HTMLElement` / `HTMLInputElement` / `HTMLFormElement`**
   at `querySelector*` sites. ~70 instances. Each is a latent bug: the code
   reads `.value`, `.name`, `.checked`, `.classList` on a possibly-`Element`
   result without an `instanceof` guard. The runtime works for the happy path
   because templates do produce `HTMLElement`s, but misselectors would silently
   throw `undefined.foo` at the call site.
2. **Custom properties hung off `window` / `HTMLElement` / `Element`**
   (`__foldoutInstallN`, `__dixieDebug`, `__dixieLiveCountHandler`, etc.).
   These are install-once markers referenced from the outside-installer
   wrappers. Correct behavior, no type. Fix is a single JSDoc
   `@type {WeakMap<Window, boolean>}`-style declaration per file.

The TS2554 and TS2304 sites look like refactor leftovers (dead variables,
unused argument captures). They are the highest-signal slice-2 fix candidates.

## What is **not** in slice 1

- No `.ts` files. JS-only via `checkJs`.
- No build step. `npm run typecheck` is `--noEmit`.
- No CI gate change. `test.yml` is untouched in slice 1; promotion to required
  status lives in slice 4.
- No strict-mode flags flipped. They stay off until slice 2 / 3 shrink the
  baseline into a manageable shape.
- No ESLint integration with `@typescript-eslint/parser`. The existing
  `npm run lint:js` keeps its current shape.

## Validation

- `npm run typecheck` exits with code 2 against the unmodified tree, printing
  the 168 errors to stdout.
- `make lint-typecheck` exits with code 2 as well (Makefile wires the same
  command).
- No Go, no templ, no CSS changed. Run `make test` (or just `go test ./...
  -short`) and `make audit` independently of this slice.
