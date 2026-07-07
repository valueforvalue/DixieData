# MISTAKES — workspace-specific gotchas

Lessons that recur. Update when a fresh failure teaches a durable
rule. New agents: read this before reaching for tools that "should
just work".

---

## RTK test aggregator reports phantom "X passed" on failing runs

**Symptom (recurring, last seen 2026-07-07 during CI-failure triage):**
Bash output ends with `📋 Test Results: ✅ 0 passed ❌ 4 failed` — but the
**raw `go test` stdout is gone**. The wrapper summary asserts pass/fail
counts from the raw output, so when stdout is suppressed the summary is
fabricated from incomplete signals. Same `testOutputAggregation` knob
as the 2026-07-01 lesson, worse symptom: a `cgo: C compiler "gcc" not
found` *failure* can surface as "0 passed" with zero raw `--- FAIL`
lines visible, and `EXIT=$?` in the same bash invocation can claim 0
because the wrapper's stdout-replacement layer desyncs the exit code
tracking. Diagnosed live this session by piping through
`pwsh -NoProfile -Command '...'` (RTK doesn't intercept PowerShell
output the same way — the wrapper summary vanished and real
`--- FAIL github.com/valueforvalue/DixieData/internal/dates` lines
came through).

**Avoid:**
- Trusting pi's RTK-wrapped bash output as the source of truth for
  `go test` (or `npm test`, or any build/test runner) when stdout
  matters. The wrapper summary can mask real failures and leak
  bogus "X passed" counts.
- Chaining `go test ... ; echo "EXIT=$?"` in the same bash line —
  RTK's stdout interception makes `$?` unreliable in the same
  invocation. Read `$?` from a fresh shell or a separate call.

**Do:**
- At session start on a Go (or any build/test) workspace, set
  `rtk_configure testOutputAggregation=false` (and
  `buildOutputFiltering=false`, `linterAggregation=false` when those
  matter — they share the interception layer).
- Read the raw `--- PASS` / `--- FAIL` lines from the bash output.
  If they're missing, the wrapper is active — fix the config or use
  the PowerShell escape below.
- When RTK is unfixable mid-session, wrap the command in
  `pwsh -NoProfile -Command '<cmd> 2>&1 | Select-String ...'` —
  verified reliable for `go test` this session (RTK didn't intercept
  the PowerShell process's stdout).
- For long-running test/build output, redirect to a log file:
  `go test ... > build/log/foo.log 2>&1; tail build/log/foo.log`.
  `build/log/` is gitignored; the redirect survives any wrapper.

**Fix applied 2026-07-07:**
- Project-level rtk config (see `.pi/agents/` / rtk_configure calls
  in AGENTS.md session rules): toggle the three aggregation knobs
  off by default for any dev session that may run Go tests, npm
  scripts, or make targets.
- Lesson supersedes the 2026-07-01 entry below; keep the older
  "phantom X failed" symptom in this section as a related case.

---

## RTK test aggregator reports phantom "X failed" on passing runs (legacy)

**Symptom:** Bash output ends with "X passed, Y failed" (often
"4 failed") when the test process actually exited 0 and the
log file shows all `--- PASS`. (See the 2026-07-07 entry above
for the more dangerous inverse case — phantom "X passed" on
actual failures, which is the form that bites triage work.)

**Cause:** `testOutputAggregation: true` collapses verbose test
output into a one-line summary. The summary parser
occasionally over-reports failures, especially when
subtests have unusual whitespace or panic-recover lines.

**Fix applied (2026-07-01):**
- `testOutputAggregation: false` — see raw `--- PASS` /
  `--- FAIL` per test
- `truncationMaxChars: 25000` — 2.5x headroom for verbose
  test output (was 10000)

**Rule for future sessions:** if RTK shows "X failed" but
the underlying command exit code is 0, trust the exit code.
Confirm by `tail -N build/log/<target>.log` (workaround from
session-end handoff, lesson #10). Don't re-run a fix on a
passing test.

---

## Tool-layer output suppression (legacy lesson from handoff)

Bash tool results are occasionally eaten by the layer
(returning blank or partial output). The reliable workaround:

```sh
go test > build/log/foo.log 2>&1
tail -50 build/log/foo.log
```

Use redirect + log file rather than relying on the live stream.
The `build/log/` directory is gitignored.

---

## `frontend/app.css` is a pre-existing dirty file on dev

Background: an earlier session left `frontend/app.css` modified
without committing. Per session-end handoff, this is
intentional and NOT a regression. Don't auto-include it in
new commits. If you genuinely need to touch it, do it in a
dedicated commit with a clear "feat:" / "chore:" subject.

---

## Sandbox bash: `internal/appshell` is unreachable via `cd` / direct `ls`

**Symptom (seen 2026-07-07 during #378/#379):** `cd internal/appsshell`
→ `No such file or directory`. `ls internal/appsshell` → same.
But `ls internal/` lists `appshell`. `find internal/ -maxdepth 2 -name
"X.go"` finds files. `sed -n '...' /c/Development/DixieData/internal/appsshell/X.go`
→ `No such file or directory`. Pattern is repeatable and inconsistent.

**Cause:** The bash tool's git-bash/MSYS2 layer in this sandbox has a
mount-point / symlink quirk on `internal/appsshell` specifically.
Direct path access fails; traversal via parent then find works.

**Fix:** Use one of:
- `cd internal && find appsshell -maxdepth 1 -name "X.go" -exec sed -n '...' {} \;`
- Read files via the `read` tool with full Windows path (`C:\Development\DixieData\internal\appsshell\X.go`)
- `grep -rn PATTERN internal/appsshell/` from a parent dir (grep walks fine)

**Rule for future sessions:** if a directory is listed by `ls parent`
but unreachable via `cd` / direct `ls`, switch to the `read` tool or
`find ... -exec` from the parent. Don't burn cycles debugging the
sandbox — it's the tool layer, not the filesystem.

---

## Stash tests for "is this pre-existing" need explicit untracked include

**Symptom:** Ran `git stash` to verify a test failure was pre-existing,
got `No local changes to save` because the new files were untracked.
The pre-existing-failure verification silently failed.

**Fix:** `git stash -u` (include untracked). The new files come back
with `git stash pop`. Use this pattern when validating "did this slice
break X" against a clean baseline.

**Rule for future sessions:** when the slice you're about to commit
includes untracked files, always stash with `-u`.
