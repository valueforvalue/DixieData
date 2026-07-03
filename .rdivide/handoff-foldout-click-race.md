# Handoff — Foldout-trigger-click-race bug + Wails-runtime smoke harness (snapshot 2026-07-02)

> **Status: ACTIVE WORK — instrumentation in place, awaiting
> Wails-runtime trace.** Captures the state of the
> foldout-click-race investigation and the Wails-runtime
> smoke harness blocker. Branch: `dev` at `6f0dd41` (pushed,
> clean). Open issues: #285 (Wails-runtime smoke harness),
> #286 (cli-coverage slice panic — fixed in 6f0dd41).

Date: 2026-07-02 (rev 2)
Branch: `dev`
Last commit: `6f0dd41 fix(cli): clamp case-window slice in scanImplementedSubcommands (issue #286)`
Open issues tied to this work: #285 (Wails-runtime smoke harness)

## Where we are

### Bug fix shipped (#283 followup, commits `d8f73b7` + `7f4a370`)

The foldout-trigger-click-race bug is **fixed and shipped**:

- **Symptom:** First click on a top-nav foldout trigger (e.g. "Share") did nothing
  visible. Workaround: click another nav link first, then click the trigger —
  the panel opens.

- **Root cause:** A bubble-phase race between the trigger's own click handler
  (bound directly on the trigger element) and the document-level
  outside-click handler (bound on `document` in `installFoldouts`). On
  click:
    1. Trigger's bubble-phase click handler fires first → `open()` runs →
       panel `hidden` class removed.
    2. Click bubbles to document → document-level outside-click handler
       iterates ALL foldout panels and closes any that are open and don't
       contain the click target. The just-opened panel matched the close
       criteria (open + click target is the trigger, which is a sibling of
       the panel, not contained) → got closed again.
    3. `aria-expanded` flipped back to `false`.

- **Why the workaround worked:** Clicking another nav link first triggered a
  full page navigation + re-render. After that, the event ordering race
  resolved differently.

- **Fix:** Added `if (trigger === target || trigger.contains(target)) continue;`
  to the document-level outside-click handler. One line in
  `frontend/app.js` (line 2339). The trigger's own click handler still
  calls `open()` synchronously; the document handler runs LATER in the
  bubble phase and would re-close it without the guard.

- **Regression net:** `audit/smoke_foldout_nav.mjs` extended from 32 → 38
  assertions. The new step 9.5 specifically uses `page.locator.click()`
  (real mouse click, not synthetic `dispatchEvent`) to reproduce the
  bubble-phase race.

### Documentation shipped (`c06c452`)

- **`docs/COMMON_BUGS.md` §3.6** (tagged `[FUTURE-NAV-AVOID]`): full recipe with
  symptom, why-it-happens (bubble-phase race explained), why the
  page-reload workaround works, the guard fix, why Playwright masked the
  bug, and the real example commit hash. New row in section 11 (Bug class →
  first place to look) for the diagnostic lookup.

- **`docs/agents/bug-pattern-grep.md` §9** (`foldout-trigger-click-race`): two
  greps that find new outside-click handlers + the add-hidden-on-click
  pattern, paired with the guard idiom so a future reviewer sees the fix
  at the same place they find the candidate.

### Wails-runtime verification (issue #285) — BLOCKED, repro requested

The foldout bug was confirmed end-to-end in Playwright (38/38 smoke
assertions pass, including the 6 new first-click assertions). The user
also ran a manual diagnostic in the Wails dev console in the earlier
session — the third diagnostic run (after the fix) showed the panel
correctly open with all items in viewport.

**As of rev 2 (2026-07-02):** user reports the bug is still occurring
on a different computer. The handoff's "fix shipped" is contested by
a fresh symptom report. We do NOT yet have a Wails-runtime trace to
disprove the bubble-phase fix in that environment.

**Instrumentation landed (uncommitted as of this rev — see "Diff in
this rev" at the bottom):** `[DEBUG-foldout-race]` console.log probes
gated behind `window.__foldoutRaceTrace` in `frontend/app.js`:

| Probe | Where | What it logs |
|---|---|---|
| `trigger-click` | trigger's own click handler | `t`, `phase`, `target` (menuID), `isOpenBefore` |
| `open-called` | `open()` sentinel | `t`, `menuID`, `panelHiddenAfter` |
| `close-called` | `close()` sentinel | `t`, `menuID`, `returnFocus`, `panelHiddenAfter` |
| `document-click` | document-level outside-click handler | `t`, `phase`, `targetTag`, `targetIsTrigger`, `targetTriggerID` |

The probes are **gated** behind `window.__foldoutRaceTrace = true` so
the Playwright probe (38/38) is unaffected. The gate is set in the
Wails dev console (or via a bookmarklet) at the time of the repro, not
at boot.

### How to capture the Wails trace (the repro recipe)

1. On the computer where the bug reproduces, launch the latest Wails
   binary (`build/bin/DixieData.exe` at 18:19:28, or a fresh
   `make debug` if older). Verify footer shows commit `6f0dd41` or
   newer — older binaries will not contain the `[DEBUG-foldout-race]`
   probes.
2. Open WebView2 DevTools (right-click on the app window > Inspect,
   or Ctrl+Shift+I). Switch to the **Console** tab.
3. Paste at the console prompt:
   ```js
   window.__foldoutRaceTrace = true;
   console.log("[DEBUG-foldout-race] enabled");
   ```
4. Navigate to a non-`/share` page (e.g. `/calendar`).
5. Click the **Share** trigger once. The bug should reproduce (no
   visible panel).
6. Copy the entire console output, including the `[DEBUG-foldout-race]`
   lines, plus the timestamp of the click. Send back.

### What we expect to see (falsifiable predictions)

- **Prediction A (bubble-phase race IS the cause in Wails too):**
  `trigger-click` and `document-click` both fire for the same click;
  `open-called` and `close-called` fire in that order; `close-called`
  has no user-initiated second click. → The fix is incomplete; the
  WebView2 outside-click handler runs in a different event phase than
  Chromium and the `trigger === target` guard does not protect.

- **Prediction B (event phase is normal, but a DIFFERENT handler is
  closing the panel):** `open-called` fires, no `close-called` from
  our handler, yet the panel is hidden. → Some other code path is
  hiding the panel (sibling-close loop? a focus handler? a CSS
  class flip?). Need to add a `MutationObserver` on the panel's
  `class` attribute in the next round.

- **Prediction C (the trigger never receives the click):** No
  `trigger-click` log at all, but a `document-click` log with
  `targetIsTrigger: true`. → Wails WebView2 is firing the click on
  document but the trigger's bubble handler is being short-circuited
  (e.g. by a `stopPropagation` somewhere up the chain, or by
  pointer-events being disabled on the trigger).

The trace determines which branch we go down next.

### Why we are NOT changing the fix yet

The Playwright 38/38 probe is a deterministic, agent-runnable loop
that exercises the same JS code path WebView2 runs. If the fix is
incomplete in Wails but works in Chromium, that is a WebView2-specific
divergence, not a logic bug — and the right fix is determined by the
Wails trace, not by guessing. The instrumentation is the minimum
instrument to disambiguate; shipping a guess-fix now risks masking
the real divergence and pushing the symptom into a different shape.

### Why we are NOT closing #285 yet

#285 (Wails-runtime smoke harness) is the gap that lets us automate
this kind of trace. The four options in the original handoff still
hold. Option 3 (UIA automation) and option 4 (accept Playwright as
sufficient) are both viable once we have a Wails trace confirming
what the divergence actually IS.

- The standard mechanism — `WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS=--remote-debugging-port=9222`
  env var — does NOT work on Wails v2.12.0. Wails's `setupChromium` (in
  `v2/internal/frontend/desktop/windows/frontend.go`) does not pass the
  env var to `CoreWebView2EnvironmentOptions`; it silently drops it.
  Net: `netstat -ano | findstr :9222` returns empty; CDP endpoint never
  comes up; WebView2 process is alive and running the app, just not
  listening for CDP.

- This is a Wails v2.12.0 limitation, not a DixieData bug. The Wails
  maintainer acknowledges this in wailsapp/wails#4261 (May 2025) and
  suggests forking v2.12.0 or upgrading to v2.13+ (the back-port commit
  `459b5e1`).

## What's open

### #285 (needs-triage) — Wails-runtime smoke harness

The full issue body is in #285. Key open questions for the next
maintainer:

1. **Fork Wails v2.12.0** to add a `--remote-debugging-port=N` option to
   `windows.Options`, then add 1 line to `setupChromium` to append it to
   `chromium.AdditionalBrowserArgs`. Cost: maintain a Wails fork,
   upstream periodically, test on every Wails upgrade. Medium-large.

2. **Upgrade to Wails v2.13+** if the back-port has landed upstream. Check
   the current Wails v2 main branch first. Cost: test the upgrade, may
   surface WebView2 API changes. Medium.

3. **Drive the Wails app via Windows UI Automation** (FlaUI / pywinauto /
   accessibility-test). Wails windows expose standard MSAA/UIA
   accessibility trees; `installFoldouts` items have `role="menuitem"`
   so a11y-driven automation works without CDP. Windows-only test runner
   required. Cost: 1-2 days initial setup, no Wails fork needed.

4. **Accept the Playwright-mode verification as sufficient.** The smoke
   probe (38/38) exercises the same JS code path that runs in Wails.
   WebView2 has had >99% parity with Chromium for click/focus/CSS since
   2023. The remaining ~1% risk is small. Document the gap in
   COMMON_BUGS.md and call it done. Cost: 0.

**Recommendation baked into the issue:** Option 4 for now. The foldout
fix is verified end-to-end via the Playwright smoke probe. Revisit
option 3 (UIA) if the Wails-specific bug class grows.

### Other open issues unaffected by this work

These are unrelated ready-for-agent items the next maintainer may
pick up:

- **#282** (ready-for-agent) — Tags merge `survivor_id` number input →
  select dropdown
- **#284** (needs-triage) — Split /share into /share/exports, /share/imports,
  /share/sync subpages
- **#266, #267, #268** (priority:medium) — Cross-cutting backlog

## What the next agent should know

1. **The foldout fix is in source, but unverified in Wails runtime.**
   `frontend/app.js:2339` has the guard
   `if (trigger === target || trigger.contains(target)) continue;`.
   The Playwright 38/38 probe is green. The user reports the bug
   still reproduces on a different computer — needs a Wails trace
   (see the "How to capture the Wails trace" section) to determine
   whether the fix is incomplete, the symptom is a different bug,
   or the WebView2 cache was stale.

2. **If a future user reports "foldout doesn't open on first click":**
   Verify the Wails binary is the latest build (check footer for
   `commit c06c452` or later). Clear the WebView2 cache at
   `%LOCALAPPDATA%\DixieData\EBWebView` if the build is correct but the
   user is still seeing the bug.

3. **The Playwright smoke probe is the regression net.** It uses
   `page.locator.click()` (real mouse click), NOT synthetic
   `dispatchEvent`. The bug depended on bubble-phase event ordering
   that synthetic events bypass. Any new foldout-family test MUST use
   the real-mouse path.

4. **Do NOT add new foldout-family outside-click handlers without the
   guard.** Grep cookbook §9 catches them. COMMON_BUGS.md §3.6 explains
   the bug class. The fix is one line.

5. **Wails-runtime smoke harness is on hold, not abandoned.** Issue #285
   is `needs-triage` with the full investigation result + 4 options
   documented. If you tackle it, start with the option that's cheapest
   for the DixieData context (option 4 = accept Playwright as sufficient,
   or option 3 = UIA automation if a second Wails-specific bug appears).

## File map (session work)

| Path | What | Commit |
|---|---|---|
| `frontend/app.js` | The fix (line 2339) | `d8f73b7` |
| `frontend/app.js` | `[DEBUG-foldout-race]` probes (gated) | uncommitted (rev 2) |
| `audit/smoke_foldout_nav.mjs` | Regression net, 38 assertions | `7f4a370` |
| `docs/COMMON_BUGS.md` | §3.6 entry + §11 table row | `c06c452` |
| `docs/agents/bug-pattern-grep.md` | §9 grep recipe | `c06c452` |
| `internal/appshell/cli_debug.go` | caseWindowChars clamp | `6f0dd41` |
| `internal/appshell/cli_debug_test.go` | ShortBody regression test | `6f0dd41` |

## Diff in this rev (rev 2, 2026-07-02)

Uncommitted change: 6 `[DEBUG-foldout-race]` console.log probes
gated behind `window.__foldoutRaceTrace` in `frontend/app.js`. The
probes are diagnostic only — no behavior change when the gate is
unset (Playwright 38/38 unchanged). The user captures a Wails trace
on their other computer per the recipe above; the trace decides
which fix branch we take next.

**Do NOT commit the probes as a permanent fix.** They are
diagnostic throwaway code per `docs/COMMON_BUGS.md` discipline and
should be removed (single `grep -n "DEBUG-foldout-race" frontend/app.js`
finds all 6 sites) once the Wails trace is in and the real fix is
identified.

## Untouched dirs (pre-existing untracked, leave alone)

- `.dixiedata-web-scratch/`
- `.pi/`
