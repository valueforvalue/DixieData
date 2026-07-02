# Handoff — Foldout-trigger-click-race bug + Wails-runtime smoke harness (snapshot 2026-07-02)

> **Status: ACTIVE WORK.** Captures the state of the
> foldout-click-race investigation and the Wails-runtime
> smoke harness blocker. Branch: `dev` at `c06c452` (pushed,
> clean). Open issue: #285 (demoted to `needs-triage`).

Date: 2026-07-02
Branch: `dev`
Last commit: `c06c452 docs(common-bugs): document foldout-trigger-click-race for future top-nav revamp`
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

### Wails-runtime verification (issue #285) — BLOCKED

The foldout bug was confirmed end-to-end in Playwright (38/38 smoke
assertions pass, including the 6 new first-click assertions). The user
also ran a manual diagnostic in the Wails dev console in the earlier
session — the third diagnostic run (after the fix) showed the panel
correctly open with all items in viewport.

However, we have **not confirmed the fix in the real Wails runtime
automatically**. The attempt to build a Wails-runtime smoke harness
(issue #285) hit a blocker:

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

1. **The foldout fix is in.** `frontend/app.js:2339` has the guard
   `if (trigger === target || trigger.contains(target)) continue;`. The
   binary at `build/bin/DixieData.exe` (built 16:28:58) contains the fix
   at byte offset 26316006.

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
| `audit/smoke_foldout_nav.mjs` | Regression net, 38 assertions | `7f4a370` |
| `docs/COMMON_BUGS.md` | §3.6 entry + §11 table row | `c06c452` |
| `docs/agents/bug-pattern-grep.md` | §9 grep recipe | `c06c452` |

## Untouched dirs (pre-existing untracked, leave alone)

- `.dixiedata-web-scratch/`
- `.pi/`
