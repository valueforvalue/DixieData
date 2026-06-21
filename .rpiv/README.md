# DixieData Workflows

Three rpiv-workflow chains live in `.rpiv/workflows/config.ts`. They share an artifact tree at `.rpiv/artifacts/<bucket>/` and an audit log at `.rpiv/runs/<run-id>.jsonl`.

## Workflows at a glance

| Workflow | When to use | Stages | Output bucket |
|---|---|---|---|
| `plan-build` | New feature, refactor, or design problem — start here | 8 | `discover/`, `research/`, `solutions/`, `plans/`, `audit/` |
| `design-it-twice` | Module-shape exploration before you commit to a plan | 4 (3 parallel) | `designs/`, `reviews/` |
| `ship-issue` | One issue from issue list → implement → review → commit | 4 | `impl/`, `reviews/` |

`plan-build` is the default — `/wf <input>` runs it without a name.

## Pi commands

| Command | What it does |
|---|---|
| `/wf` | List every loaded workflow + its stage graph |
| `/wf <name>` | Preview one workflow's stages (no run) |
| `/wf <name> <input>` | Run a workflow; `<input>` is the brief piped to the start stage |
| `/wf <input>` | Run the default workflow (`plan-build`) with `<input>` |

`<input>` is free text. Make it specific: a one-paragraph brief, an issue number, or a quoted requirement.

## Artifact tree

```
.rpiv/
├── artifacts/                  ← stage outputs (gitignored)
│   ├── discover/              ← FRD from `discover` stage
│   ├── research/              ← research md from `research`
│   ├── solutions/             ← options comparison from `explore`
│   ├── plans/                 ← phased plan from `plan`
│   ├── audit/                 ← ui-ux audit from `ui-ux-audit`
│   ├── designs/               ← 3 proposals + winner + doc (design-it-twice)
│   ├── impl/                  ← impl notes + revise notes (ship-issue)
│   └── reviews/               ← JSON verdicts (gate routing reads these)
└── runs/                      ← JSONL audit trail per run (gitignored)
    └── <run-id>.jsonl
```

To inspect a past run: `cat .rpiv/runs/<run-id>.jsonl | jq`.

## Workflow 1 — `plan-build`

Use when you're starting a piece of work and don't have a plan yet.

```
discover → research → explore → plan → lock-requirements →
frontend-design → ui-ux-audit → commit → stop
```

**Stage breakdown:**

| # | Stage | Skill | Purpose | Artifact |
|---|---|---|---|---|
| 1 | `discover` | `discover` | Interview you one Q at a time, write an FRD | `discover/<ts>.md` |
| 2 | `research` | `research` | Read codebase, answer structured Qs grounded in code | `research/<ts>.md` |
| 3 | `explore` | `explore` | Compare 2-4 solution approaches with pros/cons | `solutions/<ts>.md` |
| 4 | `plan` | `plan` | Decompose chosen approach into phased plan | `plans/<ts>.md` |
| 5 | `lock-requirements` | `lock-requirements` | Pin ambiguous terms (session-continued) | — |
| 6 | `frontend-design` | `frontend-design` | Inject design guidance (session-continued) | — |
| 7 | `ui-ux-audit` | `ui-ux-audit` | Final design pass against the plan | `audit/<ts>.md` |
| 8 | `commit` | `commit` | Git commit of all artifact files | new HEAD sha |

Stages 5–6 use `sessionPolicy: "continue"` — they keep the conversation alive so the critique is grounded in the plan you just wrote.

**Example:**
```
/wf plan-build "ship the toast migration from issue #54: switch 4200ms auto-dismiss to manual, then migrate buried feedback"
```

## Workflow 2 — `design-it-twice`

Use when you've narrowed scope but the **interface shape** is still up for grabs. Runs 3 radically different design proposals in parallel, then a 3-judge panel folds to a winner.

```
fanout-design → panel-review → writeup → commit → stop
       │              │           │
       │              │           └→ designs/<winner>.md
       │              └→ fold majority(correctness, fit, actionability)
       └→ 3 parallel sub-sessions:
           alpha: minimal surface, deep-module
           delta: function-first, generous params
           gamma: facade + worker, explicit data model
```

**Stage breakdown:**

| # | Stage | Skill | Purpose |
|---|---|---|---|
| 1 | `fanout-design` | `design-an-interface` × 3 | Generate 3 proposals in parallel sub-sessions |
| 2 | `panel-review` | `review-correctness` + `review-fit` + `review-actionability` | 3 judges grade each proposal; majority fold |
| 3 | `writeup` | (next session) | Read winner, write a single design doc |
| 4 | `commit` | `commit` | Git commit |

Each design unit and each judge runs in its own Pi session (Pi is single-active-session, so "parallel" means sequential-but-isolated).

**Prereq:** you must have `review-correctness`, `review-fit`, `review-actionability` skills installed. Without them, the panel's `judge({ skill })` calls fail at preflight.

**Example:**
```
/wf design-it-twice "shape the export-link handling: should success path trigger BrowserOpenURL + toast, or keep inline <a> markup?"
```

## Workflow 3 — `ship-issue`

Use when you have one specific GitHub issue and want it implemented + reviewed + shipped.

```
implement → review ──(blockers>0)──→ revise ──→ review (loop)
                └─(blockers=0)──→ commit → stop
```

**Stage breakdown:**

| # | Stage | Skill | Purpose | Routing |
|---|---|---|---|---|
| 1 | `implement` | `implement` | Apply changes per the issue; commit each phase | — |
| 2 | `review` | `code-review` | Adversarial review of the diff; emit JSON verdict | `blockers_count` field |
| 3 | `revise` | `implement` (continued) | Read review, fix blockers | back to `review` |
| 4 | `commit` | `commit` | Final commit-and-close | when `blockers_count = 0` |

The `review` stage must emit a JSON file with at minimum `{ "blockers_count": <int> }`. The `gate("blockers_count", { revise: gt(0), commit: eq(0) })` edge reads this field.

**The review→revise→review loop runs until blockers = 0.** Pi's workflow runner enforces `maxIterations: 32` (default) before soft-stopping.

**Prereq:** `code-review` skill installed. If it doesn't emit JSON, the gate can't route — wire it through `jsonBodyParser` (already done in the workflow) and ensure the review stage writes to `.rpiv/artifacts/reviews/`.

**Example:**
```
/wf ship-issue "implement issue #51: associate form labels with inputs via for/id pairs"
```

## When to use which

| Situation | Workflow |
|---|---|
| I have a problem, no plan | `plan-build` |
| I have a plan, but the module shape is unclear | `design-it-twice` |
| I have a clear issue and just want it shipped | `ship-issue` |
| I'm scoping/refining a vague idea | `plan-build` (start at `discover`) |
| I want to pressure-test a single decision | use `grilling` directly, no workflow |

## Resuming a run

Workflows are durable. If you crash mid-run, `/wf <name> <input>` resumes if the JSONL state is intact. To start fresh: delete `.rpiv/runs/<run-id>.jsonl`.

The runner refuses to resume if a unit generator (`units()`, `next()`) is non-deterministic w.r.t. fold-replayed state. Keep your workflow deterministic — don't read clock in `units()` etc.

## Audit trail

Every run produces `.rpiv/runs/<run-id>.jsonl`. Each row is `{type, ts, stage, ...}` — read with `jq`:

```bash
# Latest run
ls -t .rpiv/runs/ | head -1 | xargs -I{} cat .rpiv/runs/{} | jq

# Just the loop-cap events (soft-stops)
jq 'select(.type == "loop-cap")' .rpiv/runs/<run-id>.jsonl

# Per-stage timing
jq 'select(.type == "stage-start" or .type == "stage-end") | {ts, stage, type}' .rpiv/runs/<run-id>.jsonl
```

## Customization

Add a workflow: append `export const myWorkflow = defineWorkflow({...})` to `.rpiv/workflows/config.ts` and add it to the `workflows: [...]` array in the default export. Default is set via the `default:` key.

Override skill names with aliases:
```typescript
export default {
  workflows: [...],
  default: "plan-build",
  skillAliases: { commit: "attributed-commit" },
};
```

Remap resolves across all loaded workflows (built-in + user + project layers).
