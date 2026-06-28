# doctor implementation notes

## Templates parse check — use real Typst

**Decision:** use the bundled `typst-windows.exe` to validate
each `*.typ` in the templates dir. Spec option of `os.ReadFile +
UTF-8 spot-check` is weaker than what the bundled binary can
do in <5s per file.

Approach per template:
1. Write a 1-line stub `data.json` next to the template:
   `{"soldier":{"display_id":"DXD-0001"},"error":null}`
   Not all templates use all fields — Typst will complain if a
   required key is missing, but that's a runtime concern, not a
   parse concern. The stub exists so `read("data.json")` calls
   don't fail before Typst gets to parse the body.
2. Run `typst compile --ignore-system-fonts --input data.json
   data.json <template> --output nul --format pdf` with a 5s
   timeout.
3. Capture stderr. If any line matches `error:` or starts with
   `error:`, fail the check.
4. Delete the stub `data.json` after each test.

Templates that fail because of missing keys (not parse errors)
should be marked optional for now — the check's job is to catch
**syntax** regressions, not data-shape regressions. If a template
parse-passes but compile-fails on missing data, that's a separate
template issue, not a parse issue.

Reference data shape for the stub: pick the smallest known-good
shape across templates. If templates disagree on the root key
(e.g. `data.soldier` vs `data.record`), use a generic stub:
`{}`. Templates that require data will fail at runtime, but
Typst still parses the syntax.

Actually simplest: use `typst eval --in <file>` with `--format
plain`. This parses the file and dumps it as plain text — if
syntax is bad, Typst errors out before reaching the runtime
phase. No data.json needed.

```
typst eval --in templates/soldier_landscape.typ --format plain
```

If stderr contains `error:`, fail. Else pass.

## Check filter semantics

`dixiedata doctor --check=data_dir --check=sqlite` runs ONLY
those two checks. Match is prefix-by-name (so `--check=data_dir`
also matches `data_dir_resolves`). Use exact match if no prefix
intent; we'll use exact match to avoid surprises.

## Fix mode

`dixiedata doctor --fix` runs repair operations AFTER the check
phase completes. Each fix maps to a known fixable failure:

| Failed check           | Fix operation           |
|------------------------|-------------------------|
| `feedback_log_open`    | `truncate_feedback_log` |
| `migrations_applied`   | `reapply_migrations`    |
| `oauth_defaults_loaded`| `restore_oauth_defaults`|

If the check passed, the fix is a no-op. Each fix prints a one-line
report ("truncated 247 corrupt lines", "reopened DB, user_version
unchanged at 55", etc.).

`--fix` is opt-in. Without it, doctor only reports. NEVER auto-fix.

## Exit codes

Reuse smoke's convention: 0/1/2.