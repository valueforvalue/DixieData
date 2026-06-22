# Domain Docs

How the engineering skills should consume this repo's domain documentation when exploring the codebase.

## Before exploring, read these

- **`CONTEXT.md`** at the repo root.
- **`docs/adr/`** at the repo root when ADRs relevant to the area exist.

If any of these files don't exist, proceed silently. Don't flag their absence; don't suggest creating them upfront.

## File structure

This repo is configured as a single-context repo:

```
/
|-- CONTEXT.md
|-- docs/adr/
|-- cmd/                -- Go entrypoints (gold-master, seed-data)
|-- internal/           -- Go server code (appshell handlers, archive, templates)
|-- pkg/                -- shared Go packages (render, encode, templatespec)
|-- frontend/           -- HTML / JS / CSS (HTMX SPA, vanilla JS IIFE)
|-- templates/          -- Typst template files (.typ) for PDF export
|-- tools/tune/         -- separate Go module: dixiedata-tune CLI
|-- bin/                -- bundled binaries (typst(.exe))
|-- tests/              -- Go integration tests + stress suite
|-- docs/               -- PRD, audit, agents, adr, migrations
```

## Use the glossary's vocabulary

When your output names a domain concept, use the term as defined in `CONTEXT.md`. Don't drift to synonyms the glossary explicitly avoids.

If the concept you need isn't in the glossary yet, either reconsider the term or note the gap for a docs-focused follow-up.

## Flag ADR conflicts

If your output contradicts an existing ADR, surface it explicitly rather than silently overriding it.
