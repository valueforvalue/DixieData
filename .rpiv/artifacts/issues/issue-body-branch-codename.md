## Summary

Two related chrome improvements that share the same source-of-truth (`internal/versioninfo/versioninfo.go` + the build-info ldflags):

1. **Surface the current git branch** in the app footer and the OS window title so users and QA always know whether they're running `dev` or `stable`.
2. **Introduce named releases** — a human-friendly codename per release version (macOS-style: Sequoia, Sonoma, etc.) that appears alongside the numeric version in window title, footer, the About / Build panel, and the CLI `--version` output.

## Current behavior

### Build identity today
- `internal/buildinfo/buildinfo.go` exposes `AppName`, `AppVersion`, `SchemaVersion`, `GitCommit` (default `"dev"`), `BuildTimestamp`, plus helpers `AppLabel()` and `BuildIdentity()`.
- `scripts/build-common.ps1:604-619` injects `GitCommit` and `BuildTimestamp` via `-ldflags`. **Branch is never captured.**
- `internal/versioninfo/versioninfo.go` carries `CurrentSchemaVersion` (=61), `CurrentUpdateFlowVersion` (=1), `CurrentAppVersionInt` (=1). **No branch field, no codename field.** The file's own comment (line 20) lists "footer display" as a known follow-up touchpoint.

### Chrome today
- **Footer** — `internal/templates/layout.templ:251-263`:
  ```
  DixieData v1.1.1 · Schema v61 · commit abc1234 · 2026-07-04T...Z
  ```
  Rendered as `<footer data-build-identity={ buildinfo.BuildIdentity() }>`. No branch field.
- **Window title** — `main.go:178`:
  ```go
  Title: fmt.Sprintf("DixieData v%s", db.GetAppVersion()),
  ```
  Hardcoded literal; no branch or codename.
- **WebView `<title>`** — `frontend/index.html:5` hardcodes `<title>DixieData</title>`. `frontend/app.js:491,569` is the only path that writes `document.title` and it only mirrors a server-supplied snapshot. In-WebView title and OS title can drift today.
- **No codename exists anywhere in the repo.** Grep for `codename`, `release name`, `Sonoma`, `Sequoia` returns zero matches.

### DOM-id convention gap
`docs/ui-map/surfaces.md` and `internal/uiids/uiids.go` carry `page.* / panel.* / tab.* / overlay.*` constants only. Header and footer chrome have no typed DOM id today. The new branch pill (if added in a follow-up) and the About-panel codename label should follow the established prefix tree.

## Why now

- `main.go:178`'s hardcoded `Title` is the only place a user or a screenshot can tell what build they're looking at. Today it just says `DixieData v1.1.1` — no way to know whether that came from `dev` or `stable`, which matters when a bug report says "I see this on my machine."
- Named releases give a memorable label for the version-1.2-N release line. The repo is approaching its 55th release per `CHANGELOG.md` (`v1.1.55 - Patch Release`); a codename per release will make release notes, blog posts, and user conversations easier ("are you on Sequoia or Sonoma?").
- Both changes share the same seam (`versioninfo.go` + `buildinfo.go` + the ldflag injection in `scripts/build-common.ps1`), so doing them together is cheaper than two separate slices.

## Proposed v1

### A. Branch detection

1. `scripts/build-common.ps1:604-619` — also compute `$gitBranch = git rev-parse --abbrev-ref HEAD` and inject it via a new ldflag `internal/buildinfo.GitBranch`. Default to `"unknown"` if `git` is absent (cold rebuild from a tarball without `.git/`).
2. `internal/buildinfo/buildinfo.go` — add `var GitBranch string` (default `"dev"` — matches the existing `GitCommit` default).
3. Extend `buildinfo.BuildIdentity()` to append ` · {GitBranch}` so the footer shows `DixieData v1.1.1 · Schema v61 · commit abc1234 · 2026-07-04T...Z · dev`.
4. `main.go:178` — extend the Wails `Title` format to `fmt.Sprintf("DixieData %s · %s · %s", codename, db.GetAppVersion(), buildinfo.GitBranch)` (codename comes from step B below).

Per the user's v1 decision, **branch appears in the footer and the window title only** — no header pill in this issue. (A header pill can land in a follow-up if the footer + title prove insufficient.)

### B. Named releases

1. `internal/versioninfo/versioninfo.go` — add `const CurrentReleaseName = "<chosen codename>"` next to the existing N/U/schema counters. Per the user's decision, this is a **single exported string constant** — bumped by `bump-version.ps1` exactly like the other counters, not a registry map. (A future "v1.2.1 was Pioneer" retrospective can be reconstructed from CHANGELOG.md if the history matters.)
2. `scripts/bump-version.ps1` — extend the existing `-BumpRelease` / `-BumpSchema` / `-BumpUpdateFlow` switch set with a fourth `-BumpCodename` mode (mutually exclusive, like the existing three per the law in `CONTEXT.md` §"Release counter N ≠ schema version"). The switch prompts for the new codename and writes it to `CurrentReleaseName`.
3. `docs/RELEASING.md` — add a "Choosing the codename" subsection to the existing release process. macOS-style: pick a single English word (a place name, a landmark, a Southern place works thematically). Document a deprecation rule for codenames that turn out to be embarrassing later.
4. Surface the codename:
   - **Window title** — main.go's extended format (see A.4 above)
   - **Footer** — extend `buildinfo.AppLabel()` (or add `buildinfo.ReleaseLabel()`) to include ` · {CurrentReleaseName}`; `layout.templ:251-263` reads the new field.
   - **About / Build panel** — confirm an About or Build screen exists in the current chrome (check `docs/ui-map/wireframes/` for any "About" / "Build" / "Diagnostics" panel). If none exists, create the minimum viable one as part of this slice (the user wants v1 coverage there). Grep for `About`, `Build panel`, `diagnostics` in `internal/templates/` to confirm.
   - **CLI `--version`** — `internal/appshell/cli_*.go` reads `versioninfo.CurrentReleaseName` and prints it alongside `AppVersionInt` and `SchemaVersion` (per AGENTS.md §"Fresh debug build = `make freshness`", `dixiedata debug cli-coverage` must keep passing after this).

### C. Recommended first codename

The user has not chosen a name yet — flag this as an open question below so the implementer can pick (or the user can pick at the start of the slice). One rule from the start: codename must be a single English word with no spaces and no hyphens, matching the bump-version.ps1 switch validation.

## v1 apply sites — checklist

Build / source-of-truth:

- [ ] `scripts/build-common.ps1:604-619` — compute and inject `GitBranch` ldflag
- [ ] `internal/buildinfo/buildinfo.go` — add `var GitBranch string` (default `"dev"`); extend `BuildIdentity()` and add `ReleaseLabel()` helper
- [ ] `internal/versioninfo/versioninfo.go` — add `const CurrentReleaseName string = "..."`
- [ ] `scripts/bump-version.ps1` — add `-BumpCodename` switch (mutually exclusive with the existing three)
- [ ] `docs/RELEASING.md` — add "Choosing the codename" subsection
- [ ] `internal/buildinfo/buildinfo.go` doc-comment coverage stays at 100% (law in `CONTEXT.md` §"Exported Go identifiers carry doc comments")

Chrome:

- [ ] `internal/templates/layout.templ:251-263` — footer reads new build-identity + release-label fields
- [ ] `main.go:178` — window title includes codename + branch
- [ ] `frontend/index.html:5` — `<title>` either mirrors the OS title or is removed (decision: keep static "DixieData" so the WebView tab title doesn't churn with the branch)
- [ ] About / Build panel (may need to be created — see B.4)

CLI:

- [ ] `internal/appshell/cli_*.go` — `dixiedata --version` output includes codename + branch

Regression net:

- [ ] `make freshness` passes after the changes (law in `CONTEXT.md` §"Fresh debug build = `make freshness`")
- [ ] `make release-pipeline` gates pass on a clean tree
- [ ] New doc-comment coverage on any new exported identifier (regression gate per CONTEXT.md)

## Non-goals (defer to follow-up)

- **Header branch pill** — user explicitly deferred to v1.1. Footer + title only in this slice. A header pill in the topbar would be a one-line follow-up after the codename lands.
- **Codename registry / historical map** — single constant per the user's decision. If `v1.2.55 = Sequoia` matters for a retrospective later, the history is reconstructable from `CHANGELOG.md`.
- **Branch-switching from inside the app** — the user can already `git checkout dev` from a terminal; surfacing that in the app chrome is a separate (much larger) feature.
- **Displaying the Wails build mode (debug vs production)** — `layout.templ` already exposes an inline debug button when `debug.IsDebugMode(ctx)`; that's the existing surface. Don't add a second one.

## Linked / related issues

- **#368 — Reorder Source Records on Person Record and Event Record detail pages** — independent; ships on its own slice. Listed for cross-reference only.
- **TBD — Full DB commands on the CLI** — the user mentioned this in the same request as a separate but related concern (giving the CLI access to the same DB operations the GUI exposes). Filing as a separate issue immediately after this one, since it touches a different surface (the `internal/appshell/cli_*.go` dispatch table per `docs/agents/cli-plan.md`).

## Open questions

1. **First codename.** The user has not picked one yet. The implementer should either ask before opening the PR or propose 3 candidates in the PR description for the user to pick. Thematically a Southern place name (per DixieData's domain) is one option; macOS's California-landmarks tradition is another. Document the chosen name in `docs/RELEASING.md` so future releases follow the convention.
2. **`frontend/index.html:5` `<title>`** — should it mirror the OS title (so the WebView tab title also says `Sequoia · dev`), or stay static `"DixieData"` so tab-switching in a multi-window scenario doesn't churn? Recommend static for now (OS title is the primary surface; WebView title is rarely visible).
3. **About / Build panel** — does one already exist? If not, is creating one in this slice in-scope, or should v1 limit codename exposure to title + footer + CLI, and a follow-up issue create the panel? Recommend yes-in-scope since the user explicitly asked for About-panel coverage, but flag this for the implementer to confirm during planning.
4. **Branch detection on a tarball build** — if `git` is absent, what should `GitBranch` default to? Recommend `"unknown"` so it surfaces as a visible anomaly rather than silently saying `dev` on a stable tarball.