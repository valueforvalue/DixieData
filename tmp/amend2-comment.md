## Amendment (2026-07-23, #2): add markdown→typst converter theming

Per the second-pass investigation of the "markdown preview ≠ PDF
theme" report (filed as issue #663's sibling concern — #663 covers
the `\n` artifact in `article_portrait.typ:141,143`; this amendment
covers the broader theme drift), the audit agent traced the
divergence between the browser preview and the PDF export and found
that **`internal/records/markdown_typst.go` ignores the
`ThemeConfig` tokens entirely** for every markdown construct it
emits. The `ThemeConfig` schema is single-source-of-truth on paper
(`internal/config/config.go:116` — "shared between CSS and Typst
PDF rendering") but only the `.typ` chrome reads it; the converter
emits flat `11pt` Arial with no link color, no monospace font, no
blockquote style.

The agent's report pinned the divergence to these converter sites:

| Site | What it emits today | What it should emit |
|---|---|---|
| `markdown_typst.go:148-172` (`ast.Link`) | `#link("dest")[label]` — uses typst default link color | `#text(fill: theme.palette.link)[#link("dest")[label]]` |
| `markdown_typst.go:189` (`ast.CodeSpan`) + `:189-191` (`ast.FencedCodeBlock`) + `:191-193` (`ast.CodeBlock`) | `#raw(..., block: true)` — code inherits body Arial | Wrap in `#text(font: theme.fonts.mono, size: theme.type_scale.code_size)`; block gets a left-border via `#block(fill: theme.palette.code_bg, inset: 0.5em)` |
| `markdown_typst.go:176-178` (`ast.Blockquote`) | `#quote(block: true)[...]` — no border, no italic | `#quote(block: true)[...]` + emit the inner content in italic + a `theme.palette.blockquote_border` left border |
| `markdown_typst.go:207-217` (`ast.List`) | `#list[...]` / `#enum(numbering: "1.")` — typst default marker | `#set list(marker: [#text(fill: theme.palette.accent)[•]])` before list, restore after |
| `markdown_typst.go:118-127` (`ast.Heading`) | Flat `11pt` (via `article_portrait.typ:124`) | `#set text(size: theme.type_scale.heading_h1.size_pt * 1pt, weight: "bold")` keyed by `v.Level`; `#text(fill: theme.palette.heading_color)` |

The user's report — "markdown preview theme doesn't match the PDF
output theme" — is the visible symptom of this gap. The fix needs
**two coordinated halves**:

1. **Extend `ThemeConfig`** with the tokens the converter reads
   (this is the same extension the CSS-side apply-sites in this
   issue already need, so the JSON keys are added once and both
   consumers use them):
   - `theme.fonts.mono` (already exists for PDF; reuse for
     converter code)
   - `theme.palette.link` (already exists; reuse)
   - `theme.palette.heading_color` (new — h1/h2/h3/h4 ink)
   - `theme.palette.blockquote_border` (new)
   - `theme.palette.code_bg` (new — block-code background tint)
   - `theme.type_scale.heading_h1/h2/h3/h4/body_prose/code_size`
     (extend `TypeScale` map with these keys)
2. **Wire the converter to read those tokens** at render time.
   Today the converter is a pure function
   `(r *MarkdownRenderer) RenderTypst(source string)` with no theme
   argument. It needs to gain a theme parameter — either a method
   receiver (`r.RenderTypstWithTheme(source, theme)`) or a struct
   field on `MarkdownRenderer` populated at construction. The
   latter matches the existing pattern (the renderer already holds
   state); the existing `RenderTypst` keeps its current signature
   for back-compat, defaulting to a no-theme (flat) render that
   callers can opt into the themed render.

The Article PDF export (`internal/records/article_service.go` per
the agent's reference to `RenderPDF`) is the only consumer and the
only call site to update. Browser preview keeps using
`/boot-config.js` (already plumbed in this issue's existing
apply-sites).

### Why this belongs in #660 (not a new issue)

The CSS-side apply-sites in the original #660 body already need
the same `theme.palette.heading_color` and `theme.type_scale.heading_*`
keys to drive `frontend/tailwind.css:1121-1136`. Splitting them
into two issues would mean two commits both claiming to be "the
theme drift fix", with each commit leaving the other half broken.
The convert-typst work is the same reviewable unit as the
CSS-extraction work: one commit, one test net, one regression.

### What changes

- `internal/records/markdown_typst.go` — `MarkdownRenderer` gains
  a `theme *config.ThemeConfig` field; `RenderTypst` reads it via
  a new internal `renderTypstWithTheme(source, theme)`; the
  public `RenderTypst` keeps its signature (calls the themed
  variant with `nil` → flat default, back-compat)
- `internal/records/article_service.go` — `RenderPDF` calls
  `RenderTypstWithTheme(source, app.cfg.Theme)` (or equivalent
  wiring site; see `appshell/lifecycle.go` for the pattern)
- `internal/config/config.go` — `TypeScale` map gains keys
  `heading_h1` / `heading_h2` / `heading_h3` / `heading_h4` /
  `body_prose` / `code_size`; `Palette` gains keys
  `heading_color` / `blockquote_border` / `code_bg`;
  `Defaults()` seeds them; `mergeConfig` partial-merge applies
- `config.default.json` — document the new theme tokens
- `internal/config/config_test.go` — extended defaults tests for
  the new theme keys
- `internal/appshell/config_consumers_test.go` — extended
  `TestConfigConsumers_CustomValuesFlowToBootConfig` with the
  new theme keys (browser preview still consumes the same
  ClientConfig subset)
- `audit/smoke_markdown_preview_matches_pdf.mjs` — new probe:
  asserts that every CSS selector in `frontend/tailwind.css` that
  styles `.prose` (preview class) has a typst counterpart in the
  converter + `.typ` templates that consumes the same theme
  token. This is the "preview matches PDF" regression net; pattern
  mirrors `audit/smoke_codename_italics.mjs`

### Acceptance criteria (additional)

- [ ] `markdown_typst.go` emits `#text(fill: theme.palette.link)`
      around every `#link(...)` call.
- [ ] `markdown_typst.go` emits `#text(font: theme.fonts.mono)`
      around every code span and block.
- [ ] `markdown_typst.go` emits blockquote content in italic with
      a `theme.palette.blockquote_border` left border.
- [ ] `markdown_typst.go` emits heading text at
      `theme.type_scale.heading_h{1..4}.size_pt` with
      `theme.palette.heading_color`.
- [ ] When `cfg.Theme` is nil (back-compat path), the converter
      still produces valid typst markup with flat 11pt Arial —
      no regressions for any existing caller.
- [ ] `audit/smoke_markdown_preview_matches_pdf.mjs` passes; both
      browser preview and PDF export reference the same set of
      theme keys for every markdown construct.

### Slice plan change

Still one Tier-2 vertical commit (per AGENTS.md §3-tier commit
rule). The convert-typst work is part of the same plumbing pass
the original slice already covers; it does not justify a second
commit. The only delta in the slice body is the new
`internal/records/markdown_typst.go` consumer + the
`article_service.go` wiring call site + the new theme keys in
`ThemeConfig` + the new audit probe.

### Estimate change

+ O=0.5 / A=1 / N=2 (converter wiring + theme-key extension +
new audit probe + back-compat nil-theme path). Total issue PERT
now O=2.5 / A=4 / N=7 → P = (2.5 + 4*4 + 7) / 6 = 25.5/6 ≈ 4.25
days. Confidence stays medium (theme-key extension is mechanical;
the converter receiver-field change has back-compat surface area;
the new audit probe is a string scan).