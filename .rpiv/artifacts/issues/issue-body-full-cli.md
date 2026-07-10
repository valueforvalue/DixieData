## Summary

The CLI (`dixiedata <subcommand>`) currently exposes a healthy subset of the Local Archive surface — `doctor`, `list`, `show`, `search`, `export`, `import`, `migrate`, `backup`, `restore point`, `logs`, `config`, `debug` — but a long list of GUI-only operations still have no headless counterpart. Power users, sysadmins, and automation scripts cannot drive Person Record / Event Record CRUD, tags, claims, findings, scratch pad, research log, insights, review queue, merge review, share queue, research collections, image ops, or any `/settings/*` page from a terminal. This issue covers the v1 gap and proposes a phased fill-in plan that follows the established "CLI dispatches to existing `*App` methods, never duplicates handler logic" rule (`AGENTS.md:370`).

## Related issue

- **#370 — Surface git branch + introduce named releases (codename)** — independent; ships on its own slice. Listed for cross-reference only.

## Current state of the CLI

Per `docs/agents/cli-plan.md` lines 5-15, **all 7 phases of the original CLI roadmap have shipped**: smoke (`df981a1`), doctor (`7d9fc69`), list/show/search (`e5f8e61`), export (`e4474b7`), import (`98ddb45`), pre-import restore-point safety net (`35ab425`), admin + `--data-dir` + sibling restore-point root (`91dc8b8`), debug (`5b32510` / `a1e6222`). The dispatcher at `main.go:114-156` routes every documented subcommand through `runQuerySubcommand`, `runExportSubcommand`, `runImportSubcommand`, `runAdminSubcommand`, or `runDebugSubcommand`. Each runner follows one shape (e.g. `main.go:243-251`):

```
opts, err := appshell.ParseXxxArgs(os.Args[1:])
a := appshell.NewApp()
ctx := context.Background()
a.Startup(ctx)
defer a.Shutdown(ctx)
opts.App = a
return appshell.RunXxx(ctx, opts)
```

Every subcommand honours `--data-dir PATH`, `--json` (stable JSON envelope), and the exit-code table at `cli-plan.md:88` (`0 success / 1 op-failed / 2 env-error / 3 usage / 4 auth`). Drift detection at `cli_debug.go:1248+` walks `internal/appshell/cli_*.go` + `docs/agents/cli-plan.md`; `make freshness` runs `dixiedata debug cli-coverage` to assert every documented subcommand is implemented (CONTEXT.md §"Fresh debug build = `make freshness`").

## What's already wired (read-mostly)

- `doctor [--check=...|--fix|--json]` — 12 health checks via `RunDoctor(ctx, DoctorOptions{...})` (`appshell/doctor.go`); `--fix` mutates.
- `list soldiers [--query Q] [--limit N] [--page P] [--json]`, `show soldier <id|dxd-id> [--json]`, `search <q> [--json]` — read-only via `a.soldiers.{List,Get,Search}`.
- `export {pdf|jpg|json|csv|ical|static-archive|backup|shared-archive|bug-report|feedback-log|...}` — every leaf-verb maps to the GUI handler (`handleExportJSON` / `handleExportBugReport` / `archive.ExportPDF` / `a.backup.Export` / `handleExportStaticArchive` / etc.).
- `import {backup|shared-archive|memorial-json|images}` — destructive write via `a.backup.ImportWithLocalIdentity` / `a.backup.ImportSharedBackup` / `a.soldiers.ImportMemorialArchive` / `a.ImportImagePaths` (soldier-scoped only).
- `migrate {status|up}` — `db.Open` + `PRAGMA user_version` / `applySchema`.
- `backup {list|prune --keep-last N}`, `restore point {list|create [--note] [--root]|apply <id>}` — `update.NewRetainedBackupManager`. **Note:** `restore point apply` is currently a stub that prints a record (`cli-plan.md` follow-up §1) — the GUI handler exists but the CLI path is not wired.
- `logs {path|tail [--follow] [--lines N]}`, `config {show|set <key> <value>}` — read/write, with only `debug_mode` writable per `isKnownConfigKey`.
- `debug {dump|hx-invariants|browser-tree|request <path>|cli-coverage|in-place-safety}` — **debug subcommands never write, never accept `--yes`, never accept `--fix`** (per `cli-plan.md` Phase 7 rule).

## What's missing (GUI-only today)

Every operation below is reachable from the WebView but has no CLI equivalent. Grep evidence lives at the `internal/appshell/routes.go` line shown.

### Person Record CRUD
- `52-53, 56-58, 59-62, 65-67, 68-72, 160-164` — **all of `/soldiers` write paths (POST/PUT/PATCH/DELETE) including create, update, delete, tag attach/detach, bulk-tag from browse, event-tab attach/detach/quick-add/attach-by-display-id.** CLI only has `list`/`show`/`search`. No `create`, `update`, `delete`, `attach-tag`, `detach-tag`, `attach-event`, `detach-event`.

### Event Records
- `83-89, 93-94, 99-100, 108-153` — **none of `/events*` is on the CLI.** That includes CRUD (`handleEvents`, `handleNewEvent`, `handleEventByID`, `handleEditEventRoute`), the per-event PDF route, the research-log routes, the source attach/detach routes, the tag routes, the person-link routes, and the image CRUD routes.

### Source Records
- Phase 3 lesson (`cli-plan.md` Phase 3) explicitly deferred Sources CRUD. `show soldier` prints them nested in the soldier JSON; no standalone `create`/`delete`. **Also separate from #368 (reorder) — that issue is about display order; this one is about create/delete.**

### Tags
- `224-228` — `/tags`, `/tags/rename`, `/tags/merge` exist in the GUI; **no `tags` verb in the CLI.** No tag-attach / tag-detach either (those live under `/soldiers/{id}/tags/...`).

### Insights / Review Queue / Merge Review / Duplicate Audit
- `165-167, 168, 184-186, 234, 240` — `/insights`, `/insights/audit/duplicates`, `/insights/audit/duplicates/{id}`, `/review-queue`, `/compare`, `/merge-review/*`, `/insights/pdf` — all GUI-only.

### Research Log / Research Collections
- `108-110` (soldier research-log), `181-182` (collections list/by-ID) — GUI-only.

### Share + Share Queue + Presets
- `173-180` (share landing + subpages), `229` (PATCH export-options), `244` (print-records-fragment), `247-257` (queue presets list/save/delete/apply + queue page) — GUI-only.

### Google Integrations
- `259-269` — 11 routes for `/integrations/google/{calendar,sheets}` (use-managed/preferences/sync/unsync/connect/disconnect/backup/sheets-export etc.) — GUI-only.

### Scratch Pad
- `275` — `/scratchpad/open` — GUI-only.

### Image ops
- `151-153` — per-event image import/delete (soldier-level `import images` exists; event-level does not).
- `271-272` — `/images/screenshot`, `/images/rotate` — GUI-only.

### Export templates CRUD
- `206-222` — `templates {list|save|update|delete|apply}` — GUI-only. CLI exposes the renderers via `export` but not the template CRUD.

### Settings / Update / Debug Console
- `170-171, 188, 190-194, 279-281, 284-287` — `/settings/{setup,version,update-source,update-check,update-apply,bootstrap-health}`, `/export` legacy redirect, debug state/client-logs/toggle, console tail/clear/open-folder — GUI-only.
- `restore point apply` (Phase 6 follow-up §1) — stub only; needs the real handler call.

## Proposed v1 — phased fill-in

Per the AGENTS.md rule ("before adding any new subcommand, read the phase layout"), the new work lands as **Phase 8+ of `cli-plan.md`** — extending the existing 7-phase roadmap rather than replacing it. The v1 scope is the operations a sysadmin / power user is most likely to script:

### Phase 8 — Person & Event Record CRUD + Tags (read+write)
1. `soldier create --from <json> | --from-stdin` / `soldier update <id> --from <json>` / `soldier delete <id>` — dispatches to `a.soldiers.CreateOrUpdate` / `Delete` (same methods `handleNewSoldier` and `handleSoldierByID` use).
2. `event create / update / delete` — mirrors the soldier verbs; dispatches to `handleEvents` / `handleEventByID`.
3. `tags list / create / rename / merge / delete` — dispatches to `/tags` routes.
4. `soldier {attach,detach}-tag <id> <tag>` / `event {attach,detach}-tag <id> <tag>` — dispatches to `/soldiers/{id}/tags/...` and the event-tag routes.

### Phase 9 — Sources + Research Log + Scratch Pad
5. `soldier sources add <id> --from <json>` / `soldier sources remove <id> <sourceId>` (CLI equivalent of the deferred Phase 3 Sources CRUD). `event sources add/remove/list` — dispatches to `EventService.AttachSourcesToEvent` / detach.
6. `soldier research-log {list,add,update,delete}` / `event research-log {list,add,update,delete}`.
7. `soldier scratchpad {show,set}` / `event scratchpad {show,set}` — dispatches to `/scratchpad/open`.

### Phase 10 — Review / Insights / Collections / Share
8. `review-queue {list,bulk,compare}` — dispatches to `/review-queue`.
9. `merge-review conflict <local-id> <incoming-id>` — dispatches to `handleMergeReviewConflict`.
10. `insights {summary,duplicates [--json]}` / `insights duplicates apply <id>` — dispatches to `/insights` + `/insights/audit/duplicates`.
11. `collections {list,show,create,delete}` / `collections add-record / remove-record`.
12. `share {export-options,queue-presets list/save/delete/apply}` — dispatches to `/share/...` + `/share/export-options` + `/share/queue-presets/...`.

### Phase 11 — Image ops + Export templates + Settings
13. `images rotate <image-id> --deg <n>` / `images screenshot <soldier-id> --out <path>` — dispatches to `/images/rotate` + `/images/screenshot`.
14. `templates {list,show,save,update,delete,apply}` — dispatches to `/export/templates/...` (PATCH + POST alias). The apply verb accepts `--soldier <id>` and renders via the same path the GUI uses.
15. `settings {show,set <key> <value>}` (extends the existing Phase 6 `config` verb but reaches beyond `debug_mode`). Plus a real `restore point apply <id>` that calls `a.restorePoints.Apply`.

### Phase 12 — Google Integrations (CLI parity)
16. `integrations google calendar {status,use-managed,preferences,sync,unsync}` / `integrations google calendar connect/disconnect/backup` / same shape for `sheets`. All dispatch to the 11 `/integrations/google/*` routes.

Each phase lands as its own slice in `docs/agents/cli-plan.md` with the same shape the existing 7 phases used:

- Status table row at the top of `cli-plan.md`
- "What shipped" section listing the new verbs + their `*App` method calls
- "Lessons" subsection (Phase 3 / 5 / 7 lessons are the model)
- Updates to `cliHelpText()` in `main.go:63-93` so the help text stays in sync
- `make freshness` passing (`CONTEXT.md:373-394`)
- `cli-coverage` not flagging missing documented subcommands

## v1 apply sites — checklist (Phase 8 — the only phase I'll expand; Phases 9-12 follow the same pattern)

Backend:

- [ ] `internal/appshell/cli_query.go` — extend `runQuerySubcommand` (or split into `runMutateSubcommand` if write verbs get big) with `soldier` and `event` mutate verbs
- [ ] `internal/appshell/cli_admin.go` — extend `runAdminSubcommand` with `tags` verb
- [ ] `main.go:114-156` — wire new subcommand guards (`HasMutateSubcommand`?) into the dispatcher chain; add to `cliHelpText()` at `main.go:63-93`
- [ ] `docs/agents/cli-plan.md` — Phase 8 section with status table row + "What shipped" + "Lessons"
- [ ] New CLI test files under `internal/appshell/` (e.g. `cli_soldier_mutate_test.go`) following the Phase 3 `cli_query_test.go` pattern — boot `*App`, dispatch a verb, assert DB state

UI:

- [ ] No UI apply sites — this issue is CLI-only. The GUI already has every backend method we're exposing; we're not adding new HTTP routes.
- [ ] Confirm `audit/discover_orphan_handlers.mjs` still passes (no new routes added means no orphan risk)

Regression net:

- [ ] `make freshness` passes (per CONTEXT.md §"Fresh debug build")
- [ ] `dixiedata debug cli-coverage` passes
- [ ] Existing `cli_*.go` test files unchanged
- [ ] New test files use the same `--data-dir` + JSON envelope conventions

## Non-goals (defer or file separately)

- **Codename / branch in CLI `--version` output** — that lives in #370 (the companion branch/codename issue). Cross-reference only.
- **Reorder UI / CLI for Source Records** — #368 (already filed). Independent slice.
- **Building a CLI REPL or interactive mode** — separate UX decision; flag as a future issue.
- **Auto-generating the CLI from `*App` method reflection** — keep dispatch hand-written, matching the existing pattern. No framework.
- **Replacing the Wails GUI** — the GUI stays the primary surface; the CLI is power-user / automation parity, not a replacement.

## Open questions

1. **Phase ordering.** Is Phase 8 (Person & Event CRUD + Tags) the right Phase 8, or should Sources / Scratch Pad land first since they touch user data more often? Recommend CRUD + Tags first — users can already read but not write, which is the biggest CLI day-1 limitation.
2. **Mutate subcommand naming.** `soldier create ...` is the natural English. The current `list` / `show` / `search` use nouns and `export` / `import` use verbs; mixing styles is fine (cobra-style) but should be documented. Recommend: noun-grouped subcommands (`soldier {create,update,delete,list,show,search}`) for any noun that already has a verb handler, keep flat verbs (`export`, `import`, `migrate`) only for top-level cross-cutting operations.
3. **Destructive confirmations.** The existing `import` subcommand accepts `--dry-run` and `--yes`. Should every Phase 8+ write verb accept the same flags? Recommend yes — `--dry-run` is the safety net users will reach for first.
4. **JSON envelope for writes.** Should the response on success echo the new/updated record's canonical state, or just `{ "ok": true, "id": "..." }`? Recommend the former so scripts can chain (`dixiedata soldier create --json ... | jq -r .id | xargs dixiedata soldier show --json`).
5. **Backwards-compat with the existing flat `list` / `show` / `search` verbs.** Should Phase 8 deprecate them in favour of `soldier list / show / search`? Recommend keep both for v1 (don't break scripts), file a separate issue to deprecate later.