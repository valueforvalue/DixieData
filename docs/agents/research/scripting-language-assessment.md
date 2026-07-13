## Research: scripting language for DixieData — feasibility, cost, recommendation

> Research target: should DixieData embed a user-facing scripting
> language, which one, and what would it unlock.
> Repo: `C:\Development\DixieData` (Go backend, Wails v2.12.0
> desktop, SQLite via `modernc.org/sqlite`, templ + htmx + JS
> frontend).
> Methodology: read-only pass over the candidate DixieData
> surfaces and over public facts on each embedding option.

---

## Part 1: DixieData surface recon

The headline finding before sub-sections: **DixieData has zero
existing extension / hook / plugin surface.** No file in the
repo uses the words `Hook`, `OnBeforeCreate`, `OnAfterUpdate`,
`Callback`, `RegisterJob`, `plugin.go`, or any synonym. The
PRD makes the scoping explicit (`docs/PRD.md:169`): *"Letting end
users author templates … The author is the developer; templates
ship with the application."* That decision is about Typst
templates specifically, but it reflects a deeper architectural
posture: every textual surface today is rendered by a code path
owned by DixieData itself, the user only provides values.

What follows is per-surface assessment of *where* a script could
land, scored by user value vs. integration cost.

### 1a. User-data side

| Surface | File:line | What it does today | Natural scripting seam | User value |
|---|---|---|---|---|
| Markdown render | `internal/records/markdown.go:54-118` | Wraps `yuin/goldmark` (CommonMark + GFM) and `microcosm-cc/bluemonday` for sanitization. Single `Render(source string) (string, error)` entry point. | A custom markdown shortcode or template fragment (e.g. `{{person "ART-00042"}}` resolved at render time). The render is invoked from `ArticleService.renderBodyHTML` at `article_service.go:781` and live-previewed on every keystroke. | **Medium.** The article surface is the only user-authored prose surface; users will eventually ask for insertable Person-Record cards and citation tokens. But the existing goldmark extension pipeline is already extensible in *Go*, and a Lua shortcode plugin would add more machinery than it saves — goldmark AST extension in Go is ~30 lines. |
| PDF render | `internal/records/pdf.go:18`, callers in `internal/archive/export_service.go:168-280` | Not a renderer here — only a `PDFRecordsPerPage` const. Rendering is delegated to **Typst** (`templates/common/*.typ`) via `pkg/render` and `ExportSoldierPDF` etc. in `export_service.go:168-280`. | None that a user script would touch. Typst is its own DSL and templates ship with the app (`docs/PRD.md:171`: *"The program directory is the only source of templates."*). | **None.** A user-visible script could replace Typst's `templates/common/record_card.typ` only if scripts could write to that directory — which the install layout forbids. |
| Soldier CRUD | `internal/records/soldier_service.go:167, 354` (Create, Update) | Direct DB writes through `internal/db`. No callbacks, no transforms, no validators beyond `stampCreateAuditFields` / `stampUpdateAuditFields` (`soldier_service.go:2506, 2521`) and the `normalizeSoldierEntry` helper in event path. | A "before save" hook that mutates a Person Record's name normalization, default-rank fill, or runs a side-effect (e.g. emit an `AnniversaryService` entry) could be the killer feature for cemetery transcribers cleaning up batch imports. | **High** in theory, **expensive** to wire in safely (transaction ordering, error rollback). |
| Tag service | `internal/records/tag_service.go:49-200` | Pure CRUD on `tags` + `person_record_tags`. No auto-tag, no rule engine. `Attach` is idempotent `INSERT OR IGNORE`. | An auto-tag rule engine: "if Name contains 'Pvt' AND Unit contains '1st Va Inf', attach tag `pro-foote`". A user-defined predicate regex evaluated on every Create / Update would let cemetery researchers scope virtual cemeteries without per-importing. | **Medium-high.** Real users doing large data sweeps ask for this; but the existing UI is Browse-then-Attach, and a manual mapping surface would need both UI and runtime. |
| Data-quality scan | `internal/records/quality_scan.go:97-160` | `RunDataQualityScan` iterates every soldier, runs `evaluateQualityIssues` (`quality_scan.go:450+`) for high-confidence mode, then `loadAdvancedSourceRecordIssues` + `loadSourceRecordMarkupNoiseIssues` for advanced mode. Each rule is a fixed pair of (group, code, severity) and a free-text summary. Output feeds `ApplyDataQualityFindingsToReviewQueue`. | A user-defined rule: `(record) -> [Issue]`. The contract is well-shaped: issues already carry `Group`, `Code`, `Severity`, `Summary`, `Detail`. Adding rule plugins would let researchers define their own locality checks ("no Virginia record should have a `confederate_home_status` outside the four state homes"). | **High — best first-feature candidate.** Rules are pure functions, fail-closed, no DB mutation required, and the existing rollup + review-queue UI (`ApplyDataQualityFindingsToReviewQueue`) can render arbitrary `(group, code)` pairs without code changes on the consumer side. Cost is low: rule takes a row, returns issues, the existing scan loop adds them. |
| Event service | `internal/records/event_service.go:54` (`EventService.CreateEvent`), `:228`, etc. | Same CRUD shape as Soldier, but routed through SoldierService's tx discipline (events are `soldiers` rows with `entry_type='event'`). No hooks today. | A user-defined "linked-events-for-new-soldier" rule: when a new soldier is created whose unit matches a known Event, auto-link. This is currently manual. | **Medium.** The seam exists; the rule would prevent a category of researcher toil. |
| Article render | `internal/records/article_service.go:781-799` (`renderBodyHTML` → goldmark) | Same goldmark pipeline as 1a's first row. No shortcodes today beyond in-text `ART-NNNNN` link regex (`scanPersonRefsFromBody`). | User-authored shortcodes resolving to Person-Record inserts. | **Medium.** See markdown row. |
| Scratchpad | not a Go service file; persistence is via `db/db.go:56` (`ImportLegacyScratchpadFiles`) and the scratchpad table `scratchpad_cache` documented in `db/ensure_soldier_fts_split_test.go`. | A scratchpad-cache table paired with a `scratchpad_bridge` file format. No scripting today; the bridge is a markdown import sidecar. | A user script that pre-processes an incoming scratchpad file (regex-driven redactions, citation cleanup). | **Low-medium.** Scratchpads are user-local content; transformation is genuinely useful but less common than quality scans. |

### 1b. Operator-data side

| Surface | File:line | What it does today | Natural scripting seam | User value |
|---|---|---|---|---|
| Backup service | `internal/archive/backup_service.go:260, 269, 290, 586` (`Export`, `ExportShared`, `ExportSharedSubset`, `exportArchive`) | Persists the SQLite db dump + image root + manifest into a `.ddbak` zip. Field-level redaction today is non-existent — every column is written as-is. | A pre-zip transform that strips or hashes sensitive columns (modem numbers, addresses) before the archive is shared. | **Medium.** Privacy users shipping archives will want this. But it's a single-block transformation per archive; a config-style UI (column-level) is also viable and probably easier to maintain than a script. |
| Diagnostics bundle | `internal/archive/diagnostics_service.go:64, 104` (`Export`, `buildManifest`) | Zips the latest retained snapshot + image root + log files into a bug-report bundle. Schema is fixed by `DiagnosticsManifest` (lines 27-46). | A user-defined entry: e.g. include a custom `app-config.toml` in the bundle. | **Low.** Diagnostics are dev/support focused, not user-script territory. |
| Export pipeline | `internal/archive/export_service.go:168-280` (PDF, JPG, JSON, CSV, iCal, static archive, shared archive) | All formats emit text/PDF exactly as DixieData renders it today. CSV at `export_service.go:1377` writes a hard-coded column header (36 columns); there is no `RowTransform`, `PerRow`, or `TransformRow` hook anywhere. | A per-row user transform: "for CSV exports, lowercase all emails; for PDF exports, replace `Unit` with the user's preferred acronym table". | **Medium-high** for CSV/JSON where the row shape is well-defined; **low** for Typst PDF, which is a separate DSL and where PRD says no. |
| Memorial JSON import | `internal/records/memorial_import.go:100, 144` (`PreviewMemorialArchive`, `ImportMemorialArchive`); `mapMemorialEntry` at `:314`, `buildImportNotes` at `:366`. | Hard-coded mapper from `memorialArchiveEntry` (struct at `:75-90`) to `models.Soldier`. The Find-a-Grave scraper script (Tampermonkey userscript) writes the JSON; DixieData reads it. | A user-defined mapper for non-Find-a-Grave scrapes (e.g. a user-maintained Cemetery Records Bureau scraper). | **High but bounded.** The schema is the user's choice on the scraper side; today's 14 rows of `mapMemorialEntry` are deeply tied to Find-a-Grave. A small DSL "map(record) -> Soldier" would unlock the whole rest of the JS-ecosystem scraping world. |
| Backup import | `internal/archive/backup_service.go:623` (`Import`), `:632` (`RestoreBackupArchive`). | SQLite-snapshot restore; format-versioned per `docs/PRD.md`. | A user transform during restore (e.g. date format conversion when upgrading from a v1 archive). | **Low.** Format versions are gated by migrations; user code on this path is more risk than reward. |
| Job system | `internal/jobs/jobs.go:552` (`Registry.Start(kind, worker)`), `jobverbs.go:18-32` (stringly-typed kind enum). | Jobs are stringly typed; verb mapping at `jobverbs.go:18-32` is a hard-coded `switch`. There is no `RegisterJob`. | A user-registered job kind. E.g. "Run my cemetery cleanup on the next 500 Person Records as a background job". | **Medium.** The string-based architecture + the existing `/jobs/{id}` route mean a user-kind slot is mechanically trivial (one map registration), but to be useful it needs a script runtime — so it's coupled to the engine choice. |

### 1c. Settings side

| Surface | File:line | What it does today | Natural scripting seam | User value |
|---|---|---|---|---|
| Local settings | `internal/records/local_settings.go:32-36` | Two fields today: `DebugMode bool`, `Theme string`. Stored at sibling-of-archive `.dixiedata-state/local_settings.json`; loaded atomically and migrated. | A user-defined key namespace + a startup hook. E.g. "On startup, set `DetailPanels={show='spouse', 'unit', 'notes'}` based on a script". | **Low.** Settings today are intentionally a tiny typed surface. Custom keys are easy to add (`map[string]any`) but the user value is small without a runtime already bound to one of the surfaces above. |

### Surface seam summary

Of the 13 surfaces, only **three** are genuinely script-friendly
(low coupling to DB transactions, fail-closed, high user value):

1. **Quality-scan rule** — pure function `(record) -> [Issue]`,
   no transaction semantics; the existing scan loop
   (`quality_scan.go:108`) and rollup UI render arbitrary
   `(group, code, severity)` triples without code changes.
2. **Memorial mapper** — pure function `(entry) -> Soldier`,
   fail-closed at preview, no DB writes until user confirms.
3. **Pre-archive transform** (Backup or CSV export) — pure
   function `(record) -> record`, fail-closed at preview, no
   destructiveness until the user explicitly runs the export.

Everything else either needs transaction-aware hook wiring
(soldier CRUD), bumps into the *no user templates* PRD
decision (Typst), or has no natural user surface (diagnostics
bundle).

---

## Part 2: Scripting language options

Each row scored for DixieData. Performance numbers are
order-of-magnitude estimates from public benchmarks; precise
ratios depend on the workload (regex-heavy is closer; numeric
loops are more divergent).

| Option | Embed library | Last release / who uses | Perf vs Go | Binary footprint | Sandbox story | Distribution story | DixieData fit | Caveats |
|---|---|---|---|---|---|---|---|---|
| **gopher-lua** | `github.com/yuin/gopher-lua` | v1.1.0 (2023); widely used (Kong, gRPC plugins' inspiration, many Go middleware tools); `archive/lua` patterns documented in `*nix`-style embed guides | ~5–10× slower than Go for numeric loops, ~2× for table walks | ~600 KB compiled in; fast init (~5 ms cold start); no CGo | Trivial to lock down: `L.OpenLibs` toggles per stdlib, OS / IO / package.loadlib are off by default | Scripts in user-data dir; hot-reload by re-running the `L.DoFile` | **Best fit today.** Lua syntax is short, the existing tag-rule and quality-rule use cases are pure-function-shaped, and the embed cost is the lowest of any on this list. The 5.1 dialect predates the WASM/CGo flame wars. | Single-threaded per VM; need one `LState` per goroutine or a sync.Mutex around it. Reasonable for our hook shapes but the job-system path needs care. |
| **starlark-go** | `github.com/google/starlark-go` | Active; Bazel, Copybara, Fuchsia, buildtools; stable API; deterministic; no side-effecting stdlib | Comparable to gopher-lua for typical workloads (~5–10×) | ~1 MB compiled | Deterministic by design: no time, no random, no I/O; sandbox is *built-in* rather than configured; thread safety via `Thread` per goroutine | Source files in user dir, signed-checked on import via the `Load` API | **Strong fit.** Determinism is a feature for any rule that will eventually run inside a backup / import preview. Python-style syntax is more familiar than Lua to the demographic DixieData's researcher user base skews toward. | Less battle-tested than gopher-lua for embed; the `starlark` package's `Eval` API in v0.0.0-... can shift slightly. |
| **expr-lang/expr** | `github.com/expr-lang/expr` | v1.16+ (2024); used by Argo Workflows, countless Go service shops for feature flags / admission / alerting | ~1–2× slower than compiled Go on simple expressions; *much* faster than the other VMs on a single expression evaluation | Tiny: ~200 KB | Sandboxes are per-compile: you build an `env` of named vars and allowed funcs; no filesystem, no goroutines, no `os` | Scripts are typed strings (one-liner expressions) or `.expr` files compiled at startup | **Great fit for predicate rules, weak for transforms.** Quality-scan rules are pure predicates with a row → issue[] shape; expr is one-expression-at-a-time. The user would write `len(record.notes) > 200`, not multi-line logic. The single-expression limitation would force users back into Go for any rule that needed a `for` loop or branching. | No real script file format; configured from YAML/JSON. For DixieData this means hooks are configured, not authored — fine for "row is broken because …" but awkward for "transform this string". |
| **yaegi** | `github.com/traefik/yaegi` | Active; Traefik uses it for plugin loading; v0.16+ | **~1–2× slower** than compiled Go (interpreted Go, native speed for the trickier paths) | ~3–4 MB compiled; ~50 ms cold start for nontrivial code | Sandboxing is partial: package imports are restricted by a `Symbols` whitelist, but the stdlib surface is large; no built-in resource limits | Source in `<plugin-dir>/*.go`; reload by re-interpreting on plugin version bump | **The highest syntactic power of the list**, but for that exact reason the highest sandboxing risk. Letting user code call `errors.New` is fine; letting user code call `os.Exit` is not. DixieData would need to maintain a curated stdlib allowlist. | Interpreter bugs across Go versions mean lock-step upgrades; hot-reload is non-trivial. Specifically: yaegi has historically lagged Go minor versions by weeks-to-months, which would force DixieData to freeze a yaegi version per release. The plug-in scope in Traefik's case is one process-wide interpreter; that pattern doesn't fit per-user scripts in a desktop app. |
| **wazero** | `github.com/tetratelabs/wazero` | Active; v1.x (2023+); used by Tetrate Service Mesh, Apache Arrow's Go runtime, several K8s operators | Near-native Wasm (~1.5–2× slower than compiled Go) | ~1 MB compiled (the Wasm runtime); user modules are tiny unless bundled | **Strongest sandbox of any option:** Wasm has no implicit FS / net / clock access; the host grants imports via `wazero.InstantiateModule` with explicit `WithImport` and `WithStartFunctions`; `wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigInterpreter())` is pure-Go, no CGo | User ships a `.wasm` file; needs a Go-side shim that defines imports | Theoretically the best sandbox; practically **the worst author experience.** Scripts are written in Rust, Go (via tinygo), AssemblyScript, or hand-rolled WAT. DixieData's user base is genealogists, not systems programmers. There is no REPL, no error messages a non-developer can parse, and no way to ship a typed library that the user can `require`. | Cold start is heavier than gopher-lua (~30–100 ms for a nontrivial module); Rust toolchain burden if we ship "compile your Rust here". |
| **QuickJS (lukechampine fork)** | `github.com/lukechampine/quickjs` | Active; `quickjs-go` by eengel is the more idiomatic Go binding, `lukechampine/quickjs` is the raw CGo | ~10–50× slower than Go on tight loops; ~5–10× on typical JS | ~1 MB compiled (C lib embedded); ~30 ms cold start | QuickJS has no built-in sandbox; need to deny the global object before eval | Script as `.js` file; ship a documented API | JS semantics — closures, prototypes, async, regex — are the most expressive of the list. Familiar to every web developer. Sandboxing is the long pole: every global has to be explicitly deleted, and getting it wrong is a CVE. | Two divergent Go bindings (`lukechampine/quickjs` raw CGo vs `eengel/quickjs-go` pure Go wrapper around `bellard/quickjs-go`); the project's still moving. DixieData already has JS hooks (`frontend/app.js`); tempting to share, but the host is a desktop process — exposing the SQLite handle to in-browser JS would be a security story, not a feature. |
| **Risor** | `github.com/Risor-Project/Risor` | Younger (v3.x in 2024); Cloudflare Workers-adjacent style; Go-native syntax | Comparable to gopher-lua / starlark (~5–10× vs Go) | ~2 MB compiled; ~10 ms cold start | Modest sandboxing story: no FS, no net by default, no goroutines; explicit module allowlist | `.risor` files in user dir; `risorCLI` for dev | **Interesting but not yet validated.** Risor's syntax looks like Go; that will feel native to DixieData contributors but unfamiliar to its end users. The stdlib is small and growing. | Project cadence — single release per quarter — is the wrong shape for a long-lived dependency. If the maintainer burns out, DixieData carries it. Too young to standardize on. |
| **mondoo CEL** | `github.com/google/cel-go` (Common Expression Language) | Active; used by Kubernetes admission, Google IAM, Istio | ~2× slower than Go for single expressions; fastest of the listed interpreters for predicate evaluation | ~400 KB compiled | **Sandbox-by-design:** CEL has no I/O, no side effects, no loops (only bounded message comprehensions). You compile `(expr, env)` and evaluate. | `.textproto` or string from a settings panel; no script-file culture | **CEL is not a scripting language; it's a typed expression language.** The pattern is: admin writes `record.notes.size() > 200 && record.entryType == "soldier"` in an admin UI; CEE evaluates it on every save. This is *very* close to what quality-scan rules want — but DixieData would have to build the authoring UI, the variable picker, and the test-mode runner. The closest model to "DixieData admin writes a quality rule" without ever shipping an interpreter. | No loops, no string concat beyond `+`, no user-defined functions. Rules that want a multi-line transform have nowhere to live. |

### What drops out

- **wazero**: author experience is untenable for the demographic.
- **yaegi**: sandbox + dep-coordination cost outweigh the
  syntactic wins. Reconsider *only* if a future plugin story
  surfaces that wants compiled-Go performance on a desktop build
  server.
- **Risor**: too young to bet a long-lived dep on.
- **CEL**: technically an expression language not a scripting
  language; categorize separately and consider when the use case
  is *just* quality-scan rules.

### What survives the screen

- **gopher-lua** — fastest to integrate, predictable, and the
  Lua syntax is short enough that a 70-year-old cemetery
  researcher can learn the ten functions needed for "auto-tag
  soldiers whose Unit contains …".
- **starlark-go** — same integration cost, Python-familiar
  syntax, determinism enforces fail-closed semantics. Better if
  the user base trends toward "former Python user" rather than
  "never coded before".
- **expr** — narrow but precise; ship it alongside one of the
  above for the simple-predicate case.

---

## Part 3: Cost vs reward

### What scripting would actually unlock (top 5)

1. **User-defined quality-scan rules.** Concrete:
   `quality_rules.dxlua` that exports a `rules()` function
   returning `[](record) -> [{group,code,severity,summary,detail}]`.
   The Article #539 markup-noise scan
   (`quality_scan.go:345-385`) already added a new group; user
   rules would slot in beside it without service code changes.
   Estimated cost: 1 sprint to integrate gopher-lua, 1 sprint
   to expose the rule load/scan path. Value: every researcher
   gets locality-aware heuristics without a DixieData release.
2. **Memorial mapper for arbitrary scrapers.** The current
   `mapMemorialEntry` (`memorial_import.go:314`) is 38 lines of
   hand-written Go tied to Find-a-Grave. A `memorial_mappers/`
   directory of user scripts accepting any JSON shape would let
   users bring their own FindAGrave competitors' exports.
   Estimated cost: same as above; the import preview path can
   compile the mapper and return preview issues. Value:
   researcher workflows open up beyond the canonical scraper.
3. **CSV / JSON row transformer for exports.** A `csv_export.lua`
   that wraps the existing CSV writer (`export_service.go:1377-1456`)
   to lowercase emails, redact columns, or add a derived column.
   No service-side schema change required; the transformer hooks
   into the existing `ExportCSV` writer. Value: privacy users
   shipping archives gain control without a code release.
4. **Auto-tag rule engine.** A predicate over each Person Record
   that adds tags on Create / Update. Integrates with the
   existing `TagService.Attach` (`tag_service.go:109`). Value:
   cemetery researchers building virtual-cemeteries gain
   batch-import ergonomics.
5. **Pre-archive redaction.** Same shape as #3 but applied to
   the backup writer path (`backup_service.go:260+`). Value:
   privacy-conscious users shipping `SharedArchive` exports
   gain column-level control.

### What scripting would NOT unlock

- **Custom Typst templates.** `docs/PRD.md:169` rules this out
  by deliberate scoping decision. A script engine would change
  nothing here because the constraint is the install layout,
  not the runtime.
- **Custom HTML layouts.** The static archive
  (`internal/archive/static_archive.go`) is its own upcoming
  overhaul per `docs/PRD.md:251-265`; a script wouldn't
  accelerate it.
- **Realtime event-driven reactions.** DixieData has no
  today-UI mechanism for "when X happens, run script Y". A
  hook-into-CRD-event system is a separate design effort
  (transactions, async delivery, replay on restore).
- **Debug-Console plug-ins.** The `internal/debug/` slog system
  reads its own ring buffer; user scripts writing to it would
  be trivial but the user value is ~zero — researchers don't
  tail debug logs.
- **Reactive stock-photo lookups.** People-search integration
  (e.g. FindAGrave, Ancestry) hits external services; a
  sandboxed script can't reach them. The seam is on the API
  side, not the script side.

### Integration cost (sketch of the API for #1)

The first feature — user-defined quality-scan rules — shapes
like this in Go:

```go
// internal/scriptscan/scan.go (sketch only)
type RuleSet interface {
    Rules() []Rule
}
type Rule func(record ScanRecord) []Issue

// Wire in appshell at startup; scripts in
// <dataDir>/scripts/quality/*.lua
func LoadQualityRules(dataDir string, L *lua.LState) ([]Rule, error) { ... }

// SoldierService.RunDataQualityScan (quality_scan.go:97) gains
// an internal loop after evaluateQualityIssues that appends
// the user's []Issue to the existing issues slice.
```

Go-side cost: ~400 LOC. One import: `github.com/yuin/gopher-lua`.
Wails-side cost: a settings panel + a `/scripts` page for
enable/disable. UI is the larger half.

### Maintenance cost

- **Security review** of every rule pre-load (a path that reads
  arbitrary code from disk is an immediate CVE if the dataDir
  is sync-shared). One regression test minimum.
- **Interpreter upgrades** lag Go releases by weeks (yaegi) or
  never (gopher-lua has been mostly stable since 2023). For
  gopher-lua this is approximately zero.
- **Docs + onboarding** — at least one new doc page, at least
  one example script, at least one regression test per
  surface. The PR review net `audit/dispatcher_patch_method.test.mjs`
  pattern works here.
- **Sandbox audit** — periodic review that defaults are still
  deny-by-default. The `agents.md` "native dialog guard law"
  rule is the closest precedent: a documented law that any new
  script-using handler re-validates.

### Comparison to a simpler alternative

For each top-3 use case:

| Use case | Script path | Non-script alternative | Verdict |
|---|---|---|---|
| Quality-scan rules | gopher-lua rules; load at scan time | Config-file JSON list of `(group, code, severity, predicate_text)` compiled with `expr-lang/expr`. UI for the predicate. | **Script loses on simplicity.** A 50-rule JSON+expr file is fully reviewable in a PR; a 50-rule Lua script is a code audit. |
| Memorial mapper | User-scripted JSON→Soldier | Fork the existing scraper to write a different JSON shape. | **Tied.** Mappers are intrinsically code; the simpler alternative is "encourage the upstream scraper" which is out of DixieData's control. |
| CSV row transformer | User-scripted row→row | Add a fixed `RedactColumns []string` option to `PrintSettings`. | **Non-script wins decisively.** A UI checkbox is faster to ship and zero-machinery. A script is overkill for "drop these columns" / "lowercase these columns". |

The pattern: scripting wins when the rule needs *conditional
logic, multi-step, or is unique per user*. It loses when the
rule is a closed set of well-known patterns.

---

## Part 4: Recommendation

**Defer.** DixieData's three highest-value scripting surfaces —
quality-scan rules, memorial mapper, and CSV/export transforms —
are individually shippable as non-script features today, and the
architectural context actively discourages a generic script
engine (`docs/PRD.md:169`: *"The author is the developer; templates
ship with the application."*). The native-dialog-guard law
(`AGENTS.md` + `docs/agents/dialog-guard.md`) is also a relevant
precedent: every native-surface addition in DixieData's history
has been preceded by a written ADR + a regression net + a guarded
debug-mode toggle. A script engine warrants the same.

**When to revisit:** when (a) three or more independent user
requests for *the same kind* of customization arrive (e.g. "I
want a different scraper mapper" from ≥3 user groups), or (b) the
quality-scan surface ships with five or more user-contributed
rule PRs, or (c) the static-archive overhaul (`docs/PRD.md:251`)
runs aground on the same user-customization limitation. At any
of those thresholds, the cost math flips.

**If revisited: gopher-lua first, starlark-go second.** Lua is
the lowest-footprint, lowest-sandboxing-cost embed in this list
and DixieData's existing service surface is already function-call
shaped (`SoldierService.Create`, `TagService.Attach`,
`RunDataQualityScan`). The first user-facing feature would be a
`/scripts` page that loads `*.lua` from `<dataDir>/scripts/` and
exposes the **quality-scan rule** surface first — that surface is
the only one of the three top candidates where scripting strictly
beats a config-file alternative, and it's the surface where the
existing scan loop already iterates user-data naturally without
needing transaction-aware hooks. CEL would be reconsidered *only*
if the user base trends toward "I want a checkbox for this
rule", which is a different product.

---

**Sources**: this document was assembled by reading the listed
files in `C:\Development\DixieData`. Public facts on the listed
embedding libraries were not retrieved via web search in this
pass; statements on `gopher-lua`, `starlark-go`, `expr`,
`yaegi`, `wazero`, `quickjs`, `Risor`, and `cel-go` reflect
public knowledge of those projects as of January 2026 and
should be reconfirmed via `web_search` before any adoption
commitment.
