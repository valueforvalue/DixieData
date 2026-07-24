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
CI gets `just lint-typecheck` as an observability signal — not a strict gate
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

## Slice progression

| Slice | What changed                                                                                  | Error count | Test budget added         |
| ----- | --------------------------------------------------------------------------------------------- | ----------: | ------------------------- |
| 1     | `jsconfig.json` + `npm run typecheck` + `just lint-typecheck` + baseline doc                  |         168 | (observability only)      |
| 2     | Reorder `dispatchDixieDataForm` (TDZ) + 4 narrowing fixes                                    |         157 | `dispatcher_tdz_fix` × 4  |
| 3     | `frontend/global.d.ts` augmentation + `eventTargetElement` helper + per-site narrowing        |           0 | `typecheck_augmentations` × 6 |
| 4     | CI gate: `just lint-typecheck` + the two JS regression nets land on every PR via `test.yml` |           0 | (gates the prior slices) |
| 5a    | Hygiene flags (`useUnknownInCatchVariables`, `noUnusedLocals`, `noUnusedParameters`, `noFallthroughCasesInSwitch`) + 11 sites of dead-code removal | 0 | (slice-specific) |
| 5b    | `strictNullChecks: true` + nullable-annotation sweep (JSDoc `T \| null` on every `let X = null` site)                                       | 0 | — |
| 5c-A  | `noImplicitAny: true` enabled; JSDoc @param sweep in `debug.js` + `debug-toolbox.js` (PR #478 `feature/strict-mode-sweep`)                 |   0 (in scope) → 258 remaining in app.js | — |
| 5c-B  | JSDoc sweep in app.js lines 1-3000 (`feature/strict-mode-sweep`)                                                                          | 0 (in scope) | — |
| 5c-C  | JSDoc sweep in app.js lines 3000-5300 (`feature/strict-mode-sweep`)                                                                       | 0 (in scope) | — |
| 5c-D  | Narrowing guards + `@type` annotations + `RequestInit` cast + last 5-error cleanup (`feature/strict-mode-sweep`)                            |           0 | — |
| 5d    | `strict: true` umbrella flag flipped on (`feature/strict-mode-sweep` PR)                                                                  |           0 | — |

The full strict-mode sweep (5a through 5d) lands on `dev` via PR #478
(`feature/strict-mode-sweep` → `dev`). Once merged, the typechecker is
the strictest available (strict: true + all auxiliary hygiene flags on)
and the runtime path is unchanged (`app.js` continues to be served raw
from disk by `internal/appshell/lifecycle.go`).

## Final state

```jsonc
// jsconfig.json — post-5d
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "ESNext",
    "moduleResolution": "Bundler",
    "lib": ["ES2022", "DOM", "DOM.Iterable"],
    "checkJs": true,
    "allowJs": true,
    "noEmit": true,
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true,
    "skipLibCheck": true,
    "esModuleInterop": true,
    "resolveJsonModule": true,
    "isolatedModules": true,
    "jsx": "preserve"
  }
}
```

`npm run typecheck` exits 0 against `frontend/app.js` + `frontend/debug-toolbox.js` + `frontend/debug.js` + `frontend/global.d.ts`. The frontend ships unchanged: no bundler, no tsc emit, no build step.

## What is **not** in slice 1

- No `.ts` files. JS-only via `checkJs`.
- No build step. `npm run typecheck` is `--noEmit`.
- No CI gate change. `test.yml` is untouched in slice 1; promotion to required
  status lives in slice 4.
- No strict-mode flags flipped. They stay off until slice 2 / 3 shrink the
  baseline into a manageable shape.
- No ESLint integration with `@typescript-eslint/parser`. The existing
  `npm run lint:js` keeps its current shape.

## Slice 4 — CI gate

`.github/workflows/test.yml` runs three new steps after the existing
`htmx-guard lint` step:

```yaml
- name: Frontend type-check + JS regression nets (typecheck-baseline)
  shell: bash
  run: |
    just lint-typecheck
    just lint-dispatcher-tdz-test
    just lint-typecheck-augmentations-test
  timeout-minutes: 10
```

The three targets:

1. **`just lint-typecheck`** → `tsc -p jsconfig.json --noEmit` against
   `frontend/**/*.js`. Catches any TypeScript error introduced by a future
   commit. Replaces the slice-3 baseline (0 errors); the commit that breaks
   the baseline fails the step.
2. **`just lint-dispatcher-tdz-test`** →
   `audit/dispatcher_tdz_fix.test.mjs`. Pins the slice-2
   `dispatchDixieDataForm` temporal-dead-zone fix (the empty-name save flow).
3. **`just lint-typecheck-augmentations-test`** →
   `audit/typecheck_augmentations.test.mjs`. Pins the slice-3
   `frontend/global.d.ts` augmentation shape (16 install-once window markers,
   per-element markers, the htmx CustomEvent detail shape with
   `xhr.getResponseHeader`, the `eventTargetElement` helper).

Sequencing matters: `lint-typecheck-augmentations-test` exercises the file
shape that powers the clean tsc baseline. If a future PR deletes the
augmentation file, **both** `lint-typecheck` (errors back up) AND
`lint-typecheck-augmentations-test` (interface blocks gone) fail at the
same step — belt and suspenders.

The lint gate is required in CI: any PR that fails any of the three targets
fails the workflow and is blocked from merge. `dev` and `stable` are both
covered (the workflow triggers on both branches).

## What is **not** in slice 4

- **No `@typescript-eslint/parser` integration with ESLint.** The TypeScript
  parser can surface JSDoc types to ESLint and enable type-aware rules
  (`@typescript-eslint/no-floating-promises`, etc.). Holding back because:
  (a) tsc already catches every bug class those rules catch; (b) adds a
  significant devDep; (c) ESLint's flat config already works for the
  existing rules. If a future slice wants typescript-eslint rules, that is
  its own decision in its own slice.
- **No strict-mode flag flips.** The slice-3 baseline is zero with all
  `strict*` flags explicitly false. Tightening (`strict: true`,
  `noImplicitAny: true`, `strictNullChecks: true`) requires writing JSDoc
  for every parameter in app.js; that's a multi-PR refactor and not in
  scope for this slice.

## Validation

- `npm run typecheck` exits with code 0 against the slice-3 tree (the
  slice-1 baseline was 168 errors; slice 3 cleared them all).
- `just lint-typecheck`, `just lint-dispatcher-tdz-test`, and
  `just lint-typecheck-augmentations-test` all exit 0 on the slice-4 tree.
- Go backstop: `go test -short -count=1 ./internal/appshell/...` exits 0;
  Go + templ untouched.
- CI: the new `Frontend type-check + JS regression nets` step in
  `.github/workflows/test.yml` runs on every push to `dev` / `stable` and
  on every PR targeting either.
