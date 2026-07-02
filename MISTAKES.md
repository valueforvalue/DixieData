# MISTAKES — workspace-specific gotchas

Lessons that recur. Update when a fresh failure teaches a durable
rule. New agents: read this before reaching for tools that "should
just work".

---

## RTK test aggregator reports phantom "X failed" on passing runs

**Symptom:** Bash output ends with "X passed, Y failed" (often
"4 failed") when the test process actually exited 0 and the
log file shows all `--- PASS`.

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
