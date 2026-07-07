# probe-clean.ps1 — the build-cleanup PowerShell script

This file is the canonical reference for `scripts/probe-clean.ps1`,
the script that runs as `make probe-clean` (and is also wired
into the `make debug` + `make web` pre-build steps). If you
are debugging a "I can't rebuild because Windows says the
binary is in use" failure on Windows, **start here**.

## Why this file exists

Wails + Go on Windows builds to `build/bin/dixiedata-web.exe`.
If a previous build left the binary running, the next build
fails with:

```
unlinkat ... dixiedata-web.exe: The process cannot access the
file because it is being used by another process.
```

That error is opaque: it doesn't say *which* process holds the
binary. The historical recipe (`taskkill /F` with `-@` ignore)
swallowed it. `probe-clean.ps1` (added in `f51a60b`) makes the
failure actionable by:

1. Querying `tasklist` to confirm which `dixiedata-*` processes
   are alive (file:line cite: `scripts/probe-clean.ps1`).
2. Killing each via `taskkill /F /T`.
3. Re-querying. If anything survived, retrying once after a
   500 ms sleep.
4. If anything is STILL alive after the retry, surfacing the
   surviving PIDs + a yellow-banner diagnosis and exiting 1 so
   `make` halts with context.

## When the script reports survivors

The script prints a yellow-banner listing the survivors and
their PIDs. The three common causes are **outside the
script's control**:

### 1. Antivirus hold

Windows Defender (or a third-party AV) can keep a file handle
to `build/bin/dixiedata-web.exe` for 5-30 seconds after the
process dies while it scans the binary post-mortem. The
script's 1.5-second retry budget does not cover this.

**How to identify:** Windows Event Viewer → Windows Logs →
Application → look for `Event ID 1015` (malware detected) or
your AV vendor's equivalent around the time the process was
killed.

**Recovery:** wait 10-30 seconds for the AV scan to complete
the binary, then re-run `make probe-clean` (the script is
idempotent).

### 2. Debugger attached

VS Code's `dlv` instance (used by the `delve`-based launch
configs in `.vscode/`) and the Wails DevTools attach point
can leave a transient child `dixiedata.exe` or
`DixieData.exe` that the script's filter misses — both
debuggers are case-sensitive on Windows process names. The
script tries to kill both casings
(`scripts/probe-clean.ps1`: `$targets` array).

**How to identify:** taskbar shows a `dlv` or `Code (Debugger)`
icon. Or `tasklist /V /FI "IMAGENAME eq dixiedata*"` while the
script has reported success.

**Recovery:** detach the debugger (VS Code: stop the debug
session; Wails DevTools: close the DevTools window), then
re-run `make probe-clean`.

### 3. Re-spawning watcher

A developer-mode file watcher (`entr`, `nodemon`, VS Code's
"watch mode" task, or a custom script) pointed at `build/bin/`
will immediately respawn `dixiedata-web.exe` after the script
kills it. The script kills → verifies alive → kills → verifies
alive → exits 1, but the watcher is never identified as the
cause. The yellow-banner diagnosis names it as a possibility.

**How to identify:** running `watchexec 'dixiedata-web.exe'`
or similar in another terminal; VS Code's Run and Debug view
showing a "Watch" task.

**Recovery:** kill the watcher (Ctrl-C in its terminal, or the
VS Code task), then re-run `make probe-clean`.

## Policy: best-effort kill + clear diagnosis, no longer retry budget

The triage pass on #367 explicitly chose **document-only** over
"longer retry budget (10-30s with periodic re-kill)". Reasons:

- The script already exits 1 with a clear diagnosis, so the
  actual user-facing failure mode is "I don't know what to do
  next" — a doc answers that directly.
- AV-hold and watcher-respawn failures are inherently outside
  the script's control. A longer retry budget trades latency
  for nothing in the watchdog case (a watcher respawns faster
  than any bounded loop can re-kill).
- Adding a 10-30s pause would make every `make probe-clean`
  caller wait that long, even the happy path where no AV is
  involved.

If a future regression analysis shows the rare AV-hold case
is hitting one developer every week (or every CI run), revisit
the policy with a feature flag — change the script to accept
`$env:PROBE_CLEAN_TIMEOUT_SECONDS = "30"` and only sleep the
extra time when the env var is set, so the default stays
fast.

## Idempotency contract

**`make probe-clean` is safe to re-run anytime.** No error
if nothing is running. No state held between invocations. No
files written.

So the recovery recipe for any of the three failure modes is
always the same:

```
<wait or fix the underlying cause>
$ make probe-clean
```

Re-run as many times as needed. The script will only return
exit 0 once the binary is actually released.

## Caller integration

`scripts/probe-clean.ps1` is invoked from:

- `make probe-clean` — explicit cleanup target.
- `Makefile` pre-build steps for `make debug` + `make web` —
  runs automatically before each build.

If you add a new build target that produces
`build/bin/dixiedata-web.exe`, wire `scripts/probe-clean.ps1`
into its pre-build step per the existing pattern.

## Why the script doesn't do AV exclusion

Adding a Windows Defender exclusion for `build/bin/` would be
a per-developer machine setting, not a project change. It also
silently disables a security check. The triage comment on #367
explicitly excluded this direction.

## Out of scope (deferred)

- Replacing `taskkill` with a Win32 `CloseHandle` wrapper to
  force the OS to release the binary's file handle. Marginal
  gain (the script already exits 1 cleanly); large surface
  (requires a small Win32 binary dep).
- Pester test for the script's idle / single-target-kill /
  survivor-retry paths. The issue body AC flags this as
  optional. Pester is not currently a dev dep; adding it for
  one script is overkill. If Pester gets added for other
  scripts (build-common.ps1 etc.), revisit.

## Related

- `f51a60b` — `probe-clean.ps1` initial addition.
- `0d33a43` — parent #366 chain refactor that exposed the need.
- #366 — original "build chain" bug.
- #367 — this issue + the triage that chose the document-only
  policy.
- `scripts/probe-clean.ps1` — the script itself.
