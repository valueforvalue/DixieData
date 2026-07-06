# REPL-driven reasoning (Python scratch)

The `repl` skill (PoT/PAL pattern) is a scratch tool for **deterministic
investigation**. It is NOT part of the DixieData build. It is NEVER
imported by Go code. It is NEVER run in CI.

Python is incidental — the value is the **pattern**:
frame → trace → eval → compare → persist. Two green runs = done.

## When to reach for it

Use Python REPL when the task is:

- **Parsing scratch** — CSV/JSON/XML/regex probes before committing a Go
  converter in `internal/records/` or the import/export pipeline.
- **Numeric / date transforms** — pension math, age arithmetic, date
  conversions where a wrong digit matters.
- **Sweeps across fixture data** — hypothesis testing against exported
  archives or seed fixtures under `tests/fixtures/`.
- **Format validation** — verify a Source Record format converter
  round-trips losslessly against a known-good sample.
- **Read-only SQLite experiments** — `sqlite3` queries against a test DB
  in `.scratch/` (already gitignored). NEVER against `.dixiedata/` prod.

## When NOT to reach for it

Do NOT use Python REPL for:

- **Go source** — use `go test`, `go run ./cmd/...`, `make audit`,
  `dixiedata --smoke`. Go is the language of the build.
- **Production data mutation** — destructive operations belong in Go
  handlers with an audit trail and the dialog-guard pattern
  ([`dialog-guard.md`](dialog-guard.md)).
- **CI scripts** — PowerShell + bash only. Python is not in the
  pipeline.
- **Trivial recall** — if the model answers in one shot, don't repl.
- **Pure prose** — writing, summarising, ideation. The loop is overhead.

## The loop

1. **Frame** — restate as a deterministic function with concrete types.
2. **Trace** — sketch the algorithm in NL inside the code as comments.
3. **Eval** — run via `python ~/.agents/skills/repl/repl.py plan.md`.
   Mark blocks with `### repl: run`. Namespace persists across blocks.
4. **Compare** — output vs frame. Fix the *program*, not the explanation.
5. **Persist** — only commit when two identical runs produce
   byte-identical output.

See `SKILL.md` in the skill dir for the full discipline, failure modes,
and references (PoT, PAL, Self-Debugging, CodeAct vs ReAct contrast).

## Boundary

- **Scratch location:** Python scratch lives in `/tmp`, `tools/scratch/`,
  or `.scratch/`. `tools/scratch/` and `.scratch/` are gitignored.
- **Promotion rule:** if a Python script proves useful enough to keep,
  port it to Go. NEVER commit `.py` files to the repo unless they are
  deliberately tracked tooling (none today).
- **Skill source:** `~/.agents/skills/repl/` is installed from
  `valueforvalue/my-skill-framework` (see provenance note in
  `SKILL.md` frontmatter). Treat as third-party. Re-install with
  `gh repo clone valueforvalue/my-skill-framework /tmp/msf && cp -r /tmp/msf/skills/repl ~/.agents/skills/`.

## Pairing with Go-native surfaces

| Task | Tool |
|---|---|
| Go logic verification | `go test ./internal/...` |
| Full UI/UX sweep | `make audit` |
| Headless boot + smoke | `dixiedata --smoke` |
| Single-shot CLI probe | `dixiedata <subcommand>` |
| **Scratch parsing / numeric / sweep** | **Python REPL** (`~/.agents/skills/repl/repl.py`) |

The Go surfaces verify code that ships. Python REPL verifies hypotheses
before they ship.