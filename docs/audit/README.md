# Audit Documents

This directory contains diagnostic audits and the resulting design proposals for DixieData.

## Status convention

Every audit file in this directory carries one of two statuses, declared as a banner at the top of the file:

- **`STATUS: OPEN`** — Findings not yet addressed. The audit's recommendations are still actionable. The file lives at this directory's root.
- **`STATUS: RESOLVED`** — Findings have been addressed by shipped work (typically a PRD slice). The file is moved to `resolved/` and retained for historical reference. Path citations and code references in resolved audits are likely stale; check the referenced PRD section for the live state.

When an audit is resolved, the banner names the work that resolved it (commit hash or PRD slice) so readers can verify the chain.

## Current contents

### Open audits
- `pragmatic-programmer-audit-2026-07.md` — maps each of the 100 tips from _The Pragmatic Programmer, 20th Anniversary Edition_ to the repo (Enforced / Partial / Gap / N/A). 35+ tips are enforced by `CONTEXT.md` Laws + tier-1 process docs; six high-leverage gap candidates (one per follow-up issue) are surfaced at the bottom. Retained as a **historical artifact** after the principle-spine audit below was added. Tracks issue #567.
- `pragmatic-programmer-principles-audit-2026-07.md` — **primary audit.** Maps the book's *principle spine* (DRY, orthogonality, reversibility, tracer bullets, design by contract, Law of Demeter, MVC, metaprogramming, temporal coupling, program deliberately, algorithm speed, refactoring, code that's easy to test, ubiquitous automation, it's all writing, great expectations, plus the 4 book-end checklists: WISDOM, Architectural Questions, Debugging Checklist, Cutting the Gordian Knot) to the repo. 14 principles ✅ Enforced + 9 ⚠️ Partial + 1 ❌ Gap + 1 ➖ N/A out of 25. Twelve consolidated gap candidates (lowest-effort first) at the bottom. Supersedes the 100-tip view as the primary lens. Tracks issue #567.

The static-web-archive audit (`static-web-archive-audit-2026-06.md`) was resolved by PR #71 (commit c831abf) before the convention was formalized; it lives at the directory root for visibility.

### Resolved audits (`docs/audit/resolved/`)
- `layout-and-theming-audit-prompt.md` — the prompt that drove the layout-theming audits.
- `layout-theming-findings.md` — the literal-by-literal map of fpdf visual output. Path citations reference the fpdf-era stack; the equivalent code now lives in `pkg/render/`.
- `layout-theming-components.md` — visual component inventory, scoped the move to Typst templates.
- `layout-theming-engine-evaluation.md` — three-engine comparison; Typst was selected.
- `layout-theming-token-schema.md` — token schema proposal; rejected in favor of Typst-native theming via `templates/common/theme.typ`.

## Resolving an audit

When all of an audit's findings are addressed:

1. Move the file to `docs/audit/resolved/`.
2. Add a `> **STATUS: RESOLVED.**` banner to the top of the file naming the work that addressed it (PRD section, PR number, or commit hash).
3. Re-cite paths in the file to current locations where straightforward; otherwise note in the banner that the citations are stale.
4. Update the bullet list above.

Re-resolving an audit (re-opening for a follow-up) is fine — move it back to the root, update the banner, and amend the bullet list.

## Origin

The convention was formalized after the layout-theming audit batch (issues #58, #59, #61, #63) was closed out in late June 2026.
