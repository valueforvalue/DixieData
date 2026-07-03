# Subagent Model Scope — Why DixieData Pins Subagents to MiniMax-M3

## TL;DR

DixieData forces every `Agent` tool subagent spawn to use `minimax-m3-clean/MiniMax-M3` (or whatever the project's default model is). Two layers enforce this:

1. **Project agent shadow files** in `.pi/agents/` override the three built-in defaults (`Explore`, `Plan`, `general-purpose`) that `@tintinweb/pi-subagents` ships with. Those defaults hardcode `model: anthropic/claude-haiku-4-5`.
2. **The `enabledModels` allowlist + `scopeModels: true`** combination rejects any out-of-scope model at runtime, with a hard error for caller-supplied strays.

## The problem we are solving

`@tintinweb/pi-subagents` v0.13.0 ships three built-in agents in `default-agents.ts`:

- `Explore` — `model: "anthropic/claude-haiku-4-5"` (hardcoded)
- `Plan` — `model: inherit` (inherits parent's, fine)
- `general-purpose` — `model: inherit` (fine)

Without intervention, every `Agent({ subagent_type: "Explore", ... })` call routes to Anthropic's claude-haiku-4-5. On a machine with no Anthropic key but an OpenRouter key (DixieData's case), the call transparently fails over to OpenRouter's `anthropic/claude-haiku-4-5` listing — billing hits OpenRouter, not Anthropic directly, but the model is still Claude. That is the leak: subagents run on a model the project's primary sessions never use.

The project's primary sessions all use `minimax-m3-clean/MiniMax-M3` (see `AGENTS.md` and `~/.pi/agent/settings.json`). Subagents silently swapping to a different model family breaks four things:

1. **Cost visibility** — the bill arrives through a different channel than the user is watching.
2. **Behavior parity** — parent agent uses M3 thinking + tools, subagent uses Haiku's shorter context and weaker tool-use. Subagent output diverges from what the parent would have produced.
3. **Audit trail** — subagent transcripts land in `.pi/output/agent-*.jsonl` referencing a model name that does not appear anywhere else in the repo's workflow docs.
4. **Cache hygiene** — the `pi-minimax-m3-caching-fix` extension (`minimax-m3-clean` provider) strips the `<think>` duplicate so prompt-cache hits land. Subagents running on a different provider/model bypass that fix and trigger cold cache reads on every turn.

## Layer 1: Project agent shadow files (`.pi/agents/*.md`)

These three files shadow the built-ins and pin the model. **Use the `minimax-m3-clean` provider id, not `minimax`** — the caching-fix extension only registers under `minimax-m3-clean`, and the cache-hit guarantee depends on routing through it. Subagents inheriting the vanilla `minimax/MiniMax-M3` would silently drop the cache-fix and trigger cold cache reads on every turn.

- `.pi/agents/Explore.md` — `model: minimax-m3-clean/MiniMax-M3`, `tools: read, bash, grep, find, ls`
- `.pi/agents/Plan.md` — `model: minimax-m3-clean/MiniMax-M3`, `tools: read, bash, grep, find, ls`
- `.pi/agents/general-purpose.md` — `model: minimax-m3-clean/MiniMax-M3`, `tools: read, bash, grep, find, ls, write, edit`, `prompt_mode: append`

Discovery priority: `.pi/agents/<name>.md` (project) > `~/.pi/agent/agents/<name>.md` (global) > built-in `DEFAULT_AGENTS` from the package. The project-level file wins by name. Per the pi-subagents docs: *"Project-level agents override global ones with the same name, so you can customize a global agent for a specific project."*

Frontmatter `model:` is authoritative — caller-supplied `Agent({ model: "..." })` cannot override it. This is the v0.5.1 guarantee referenced in the package docs.

## Layer 2: `enabledModels` allowlist + `scopeModels: true`

Two settings work together to refuse any model outside the project allowlist:

**`~/.pi/agent/settings.json`** (global pi settings):

```json
{
  "enabledModels": ["minimax-m3-clean/MiniMax-M3"]
}
```

**`~/.pi/agent/subagents.json`** (pi-subagents persistent settings):

```json
{
  "scopeModels": true
}
```

When `scopeModels` is on, every subagent spawn validates its effective model against `enabledModels`. Three failure modes:

| Model source | Out-of-scope behavior |
|---|---|
| Caller-supplied via `Agent({ model: "haiku" })` | Hard error returned to orchestrator, lists allowed models |
| Pinned in agent frontmatter (e.g. a global agent with `model: haiku`) | Warning toast + pinned model runs (frontmatter is authoritative) |
| Parent-inherited (neither set) | Warning toast + parent's model runs |

Layer 1 (project shadow files) eliminates the frontmatter-pinned case — there is no frontmatter pointing at haiku because every built-in is shadowed. Layer 2 (scopeModels) eliminates the caller-supplied case — any future `Agent({ model: "..." })` call with a non-allowlisted model errors out.

## Why two layers, not one

Either layer alone leaves a gap:

- **Shadow files only** — caller can still pass `model: "anthropic/claude-haiku-4-5"` to the `Agent` tool. The shadow file pins the default but doesn't reject strays.
- **scopeModels only** — frontmatter pins run with a warning, so a global agent in `~/.pi/agent/agents/*.md` with `model: haiku` would still fire. Project shadowing is needed to neutralize the frontmatter route.

Both together = no path to a non-allowlisted model survives.

## What this does NOT cover

- **Other repos on the same machine.** This is project-local. `~/.pi/agent/settings.json` `enabledModels` is global, but that is intentional — the user's overall model set is one thing, the per-project enforcement is another. If you need `enabledModels` to vary per-project, move it to `.pi/settings.json` (project-local) instead.
- **The orchestrator session itself.** This doc is about subagents. The primary agent uses the project's `defaultProvider` / `defaultModel` from `~/.pi/agent/settings.json`, which is independent.
- **Extensions that spawn their own model calls** (e.g. MCP servers). Out of scope for this doc.

## Replicating this in a new repo

1. Copy the three `.pi/agents/{Explore,Plan,general-purpose}.md` files. Update the `model:` field to whatever the repo's default model is (don't blindly paste `minimax-m3-clean/MiniMax-M3`). The provider id must match what the project uses for primary sessions — if the project uses a custom provider extension (e.g. caching fix), use that id; using a vanilla `provider/model` form silently bypasses the extension.
2. Add `enabledModels: ["<provider>/<model>"]` to `~/.pi/agent/settings.json` (or `.pi/settings.json` if per-repo scope is wanted).
3. Set `scopeModels: true` in `~/.pi/agent/subagents.json` (create the file if absent). Use `/agents → Settings → Scope models` to flip the toggle interactively after first run.
4. Verify by spawning one of each agent type and checking the widget — the model column should show the pinned model, not Haiku.

## Diagnostic: how to confirm haiku is no longer being called

- Watch the subagent widget above the editor during a spawn — the model column shows the resolved model.
- Grep `.pi/output/agent-*.jsonl` for `"model"` after a session — every entry should reference the project's pinned model.
- `/agents → Agent types` — each agent's row shows its configured model. A `(unavailable, fallback: inherit)` marker means resolution failed.
- If `scopeModels` is on and you pass `Agent({ model: "haiku" })`, the tool result surfaces an explicit error listing the allowed set. That is the canary.

## References

- [`@tintinweb/pi-subagents` docs](https://pi.dev/packages/@tintinweb/pi-subagents) — Custom Agents, Model Scope, Default Agent Types sections
- `~/.pi/agent/npm/node_modules/@tintinweb/pi-subagents/src/default-agents.ts:40` — hardcoded haiku pin (line may shift across versions)
- `~/.pi/agent/npm/node_modules/@tintinweb/pi-subagents/src/enabled-models.ts` — allowlist resolution
- `~/.pi/agent/npm/node_modules/@tintinweb/pi-subagents/src/settings.ts` — `scopeModels` toggle
- `CONTEXT.md` — domain vocabulary + non-negotiable laws
- `AGENTS.md` — session protocol, branch policy