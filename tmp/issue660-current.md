## Summary
Move the remaining hardcoded URLs, service endpoints, timeouts, file-type
allowlists, and the full browser theme palette into the existing external
JSON config (`config.default.json` → `internal/config/config.go`), so
configurable behavior lives in one place.

## User story
As a DixieData user / packager, I want every tunable value (URLs,
endpoints, timeouts, file allowlists, theme tokens) to live in
`config.json` rather than be baked into the binary, so that I can
point the app at a fork / private update server / custom feedback
endpoint / rebranded theme without recompiling.

## Locked decisions
1. **Config lives in `config.default.json` (root) at the state root.**
   Decided in issues #636–#640 series (closed 2026-06). The current
   `internal/config` package is the seam — extend it; do not fork.
2. **Single JSON file, partial-merge semantics.** Missing keys fall
   back to defaults. Malformed JSON returns an error (already enforced).
3. **Scope = "clear gaps" only.** Audit surfaced 19 `gap` items; the
   `maybe` items (OAuth loopback host, Google Sheets/Drive URL
   templates, retry tuning, scanner buffer sizes, restart delays) are
   deferred to a follow-up issue if needed.
4. **Theme tokens move from `frontend/tailwind.css` literals into
   config, injected as CSS custom properties at boot.** Browser palette
   + semantic calendar/activity-chart palette + browser font stacks +
   browser type scale. PDF-rendering theme stays in the same struct
   (already there).
5. **Default values in code stay authoritative** — `Defaults()` in
   `internal/config/config.go` is the source of truth; `config.default.json`
   mirrors it for documentation / first-run discovery.
6. **One commit on `dev`, single reviewable unit.** Per AGENTS.md
   §Commits, multi-layer feature that decomposes naturally into a
   single reviewable slice: struct + tests + defaults + plumbing.

## Apply sites (v1 checklist)
URLs + endpoints:
- [ ] `internal/update/updater.go:29` — `services.update_check_url`
- [ ] `internal/appshell/about_handlers.go:138-140` — `services.repository_url` (derived commit/license links)
- [ ] `internal/templates/about.templ:245,253` — same `services.repository_url` for commit links
- [ ] `internal/supportuploader/default_endpoint.go:18` — `services.feedback_endpoint`

Timeouts + limits:
- [ ] `internal/appshell/app_feedback.go:125` — `timing.feedback_send_timeout_s`
- [ ] `internal/supportuploader/formspark.go:22` + `supportuploader.go:44` — `timing.feedback_upload_timeout_s`
- [ ] `internal/integrations/google_service.go:158,483` — `timing.google_health_timeout_s`
- [ ] `internal/integrations/google_service.go:323` — `timing.google_oauth_wait_timeout_s`
- [ ] `cmd/dixiedata-web/main.go:172` — use existing `cfg.Timing.ShutdownTimeoutS` (was duplicated literal)
- [ ] `internal/appshell/smoke.go:390` — same `cfg.Timing.ShutdownTimeoutS`
- [ ] `internal/viewmodel/article.go:27` — `limits.article_excerpt_chars`
- [ ] `internal/appshell/lifecycle.go:136` — `limits.debug_log_ring_size`
- [ ] `internal/archive/export_service.go:29` — `limits.export_batch_size`

File-type allowlist:
- [ ] `internal/templates/entry_form.templ:403`, `event_detail.templ:136`, `soldier_card.templ:589` — `files.allowed_image_mime_types`
- [ ] `internal/appshell/articles_handlers.go:851-852`, `events_handlers.go:1113-1114` — derive dialog patterns from same source

Theme tokens:
- [ ] `frontend/tailwind.css:38-1218` — browser palette → `theme.palette_browser` (CSS custom properties)
- [ ] `frontend/tailwind.css:411,415,1114,1218` — browser font stacks → `theme.fonts_browser`
- [ ] `internal/templates/about.templ:431-452` — activity-chart palette → `theme.palette_activity`
- [ ] `internal/templates/calendar.templ:91,93,118-119,148,154,157,250,257` — calendar semantic tokens (event/holiday/today/quote-text)
- [ ] `internal/templates/calendar_day.templ:246-262` — reuse calendar semantic tokens
- [ ] `frontend/tailwind.css:457-1218` — browser type scale → `theme.type_scale_browser`

The feature is not "shipped" until every box is checked.

## Glossary changes (if any)
None. The glossary already covers `Local Archive` and `Restore Point`,
which is where `config.json` lives. The config schema itself is the
contract; no new domain terms.

## Acceptance criteria
- [ ] Every value listed in "Apply sites" is read from `cfg.X` (or
      `cfg.Theme.X`); the literal no longer exists in production code.
- [ ] `config.default.json` carries the new sections (`services`,
      `files`, expanded `theme`, expanded `timing`, expanded `limits`).
- [ ] `internal/config/config.go` exposes the new fields in `Config`,
      `Defaults()`, `mergeConfig`, and (where the frontend reads them)
      `ClientConfig`.
- [ ] Browser theme tokens reach the page as CSS custom properties
      (`--theme-accent`, `--theme-event-color`, etc.) injected from
      config; `tailwind.css` references the variables rather than hex
      literals.
- [ ] `make test -short` green; `make tpl` no-op; `make verify-fresh-bake`
      green (no bake generators touched, but the discipline gate runs).
- [ ] One CHANGELOG `[Unreleased]` bullet under `### Maintenance`
      naming the 19 sites consolidated.

## Slice plan
This is a single Tier-2 vertical slice (per AGENTS.md §3-tier commit
rule): one commit because the struct extension + the consumer
plumbing + the CSS injection only make sense together. The RED test
extension in `internal/appshell/config_consumers_test.go` is the
regression net.

### Slice 1 (tracer bullet — fully detailed)
- Files:
  - `config.default.json` — add `services`, `files`, expand `timing`,
    `limits`, `theme`
  - `internal/config/config.go` — extend `Config`, `Defaults()`,
    `mergeConfig`, `ClientConfig`, add `ServicesConfig`, `FilesConfig`,
    extend `ThemeConfig`
  - `internal/config/config_test.go` — extend tests for new defaults
    + partial-merge
  - `internal/appshell/config_consumers_test.go` — extend with one
    assertion per new field (boot-config + /settings/config reach)
  - `internal/update/updater.go` — read `cfg.Services.UpdateCheckURL`
  - `internal/supportuploader/default_endpoint.go` +
    `internal/supportuploader/formspark.go` +
    `internal/supportuploader/supportuploader.go` — read
    `cfg.Services.FeedbackEndpoint` + `cfg.Timing.FeedbackUploadTimeoutS`
  - `internal/appshell/app_feedback.go` — read `cfg.Timing.FeedbackSendTimeoutS`
  - `internal/appshell/about_handlers.go` + `internal/templates/about.templ` —
    read `cfg.Services.RepositoryURL`, build commit/license URLs
    server-side
  - `internal/integrations/google_service.go` — read
    `cfg.Timing.GoogleHealthTimeoutS`, `GoogleOAuthWaitTimeoutS`
  - `cmd/dixiedata-web/main.go:172` + `internal/appshell/smoke.go:390` —
    use existing `cfg.Timing.ShutdownTimeoutS`
  - `internal/viewmodel/article.go` — read `cfg.Limits.ArticleExcerptChars`
  - `internal/appshell/lifecycle.go:136` — read `cfg.Limits.DebugLogRingSize`
  - `internal/archive/export_service.go` — read `cfg.Limits.ExportBatchSize`
  - `internal/appshell/articles_handlers.go` +
    `internal/appshell/events_handlers.go` +
    `internal/templates/entry_form.templ` +
    `internal/templates/event_detail.templ` +
    `internal/templates/soldier_card.templ` — derive image MIME accept
    from `cfg.Files.AllowedImageMIMETypes`
  - `frontend/tailwind.css` — replace hex/font/size literals with
    `var(--theme-*)` references
  - `internal/appshell/boot_config.go` (or equivalent injection point)
    — emit `<style>:root{--theme-accent:...}</style>` from cfg
  - `CHANGELOG.md` — Maintenance bullet
- Success criteria:
  - `go test -short -count=1 ./...` passes
  - `internal/appshell/config_consumers_test.go` `TestConfigConsumers_CustomValuesFlowToBootConfig`
    passes with every new field asserted
  - Manual: edit `config.json`, restart app, see new URLs/timeouts/theme
    take effect
- Contract touch: new public seams `config.ServicesConfig`,
  `config.FilesConfig`, extended `config.TimingConfig`,
  `config.LimitsConfig`, `config.ThemeConfig`; caller obligations are
  to read from `cfg.X` instead of the literal; observable guarantees
  are that a configured value reaches the consumer (pinned by the
  extended regression test). The existing `mergeConfig` partial-merge
  contract is preserved — see `internal/config/config_test.go`
  `TestLoad_PartialMerge` for the shape.
- Regression net:
  - `internal/appshell/config_consumers_test.go` (extended) — pins
    "configured value reaches consumer" for every new field
  - `internal/config/config_test.go` — pins defaults + merge semantics
  - Manual smoke: edit `~/.dixiedata-state/config.json`, set
    `services.feedback_endpoint` to a localhost receiver, send
    feedback, observe request lands at receiver

### Subsequent slices
None. The 19 gaps fit one reviewable unit; further work (OAuth loopback
host, Sheets/Drive URL templates, retry tuning, scanner buffer sizes,
restart delays, `maybe` items from the audit) lands as a separate
follow-up issue if/when it becomes user-visible.

## Principle warnings
- **Principle:** DRY (Pragmatic Principles §1.1).
- **Operational form being violated:** the existing config has a mix of
  nested objects (e.g. `timing.{jobs_poll_ms}`) and flat fields. The
  new `services.{update_check_url, repository_url, feedback_endpoint}`
  follows the nested-object pattern; `files.allowed_image_mime_types`
  introduces an array value, which `mergeConfig` handles by
  full-replacement (consistent with `theme.fonts.body_serif`). The
  browser-theme injection adds a CSS custom-property layer; the
  two-theme-config split (`theme.*` for PDF + `theme.palette_browser` /
  `theme.fonts_browser` / `theme.type_scale_browser` for CSS) is
  intentional — they are consumed by different renderers.
- **Rationale for the temporary violation:** the two-theme-config
  split keeps the existing PDF theme struct stable (Typst renderer
  reads it directly) and adds new fields rather than refactoring
  existing ones. Refactoring the existing `theme` struct would risk
  breaking the Typst pipeline.
- **Cleanup plan:** if a future slice unifies the two themes, refactor
  `ThemeConfig` to a single nested struct + a `BrowserTheme` map;
  that work is not in this slice.

## What assumptions does this PR make?
- **Assumption 1:** the partial-merge contract in `mergeConfig` is
  sufficient for the new fields. Pinned by `TestLoad_PartialMerge`
  extended with `services` + `files` + `theme.palette_browser`.
- **Assumption 2:** the existing `ClientConfig` boot-config injection
  is the right channel for browser-theme CSS variables. Pinned by
  `TestConfigConsumers_CustomValuesFlowToBootConfig` extended to assert
  the new fields reach `/boot-config.js` (and the injection template
  emits the `<style>` block). If the injection needs to move (e.g. to
  a dedicated `/theme.css` endpoint), that's a follow-up.

## Files
- `config.default.json`
- `internal/config/config.go`
- `internal/config/config_test.go`
- `internal/appshell/config_consumers_test.go`
- `internal/update/updater.go`
- `internal/supportuploader/default_endpoint.go`
- `internal/supportuploader/formspark.go`
- `internal/supportuploader/supportuploader.go`
- `internal/appshell/app_feedback.go`
- `internal/appshell/about_handlers.go`
- `internal/templates/about.templ`
- `internal/integrations/google_service.go`
- `cmd/dixiedata-web/main.go`
- `internal/appshell/smoke.go`
- `internal/viewmodel/article.go`
- `internal/appshell/lifecycle.go`
- `internal/archive/export_service.go`
- `internal/appshell/articles_handlers.go`
- `internal/appshell/events_handlers.go`
- `internal/templates/entry_form.templ`
- `internal/templates/event_detail.templ`
- `internal/templates/soldier_card.templ`
- `frontend/tailwind.css`
- boot-config injection site (likely `internal/appshell/boot_config.go`
  or `internal/appshell/app.go` boot handler)
- `CHANGELOG.md`

## Regression net
- `internal/appshell/config_consumers_test.go` — extended; one
  assertion per new field
- `internal/config/config_test.go` — extended defaults + merge tests
- Manual smoke: `edit config.json → restart → verify`

## Related
- Issue #636 — initial config plumbing
- Issue #637 — `/settings/config` page
- Issue #638 — settings viewmodel
- Issue #639 — boot-config injection
- Issue #640 — end-to-end config coverage (created the regression net
  this slice extends)
- `docs/COMMON_BUGS.md` — keep the wails.localhost PATCH / FormData
  quirks in mind if any of the touched forms change dispatch behavior

## Estimate
PERT: O=1 / A=2 / N=4
P = (1 + 4*2 + 4) / 6 = 13/6 ≈ 2.2 days
Confidence: medium (single-author, 6+ files across 4 layers, theme
injection has CSS-custom-property footguns; PDF theme stays unchanged
but `palette_browser` is a new field that touches ~50 sites in
`tailwind.css`).
