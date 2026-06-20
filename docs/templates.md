# Authoring a Typst template

DixieData exports PDFs by compiling `.typ` files in `<repo>/templates/`.
This document describes how to author a new template.

## File location

Templates are `.typ` files in `<repo>/templates/`. A new template is
discovered automatically at startup; no registration is required.
The file's name (minus the `.typ` extension) is the template name
referenced in `PrintSettings.Template` and the `dixiedata-tune` CLI.

## Template structure

Every template has two parts: a metadata header and a body.

### Metadata header

The metadata block is a series of `// ` comments at the top of the
file. The first comment is `// metadata:`; subsequent lines are
`// key: value` pairs.

```typst
// metadata:
//   name: soldier_landscape
//   record_types: [soldier]
//   orientation: landscape
//   export_types: [record_card]
//   description: Standard Soldier record card (landscape)
```

| Field | Purpose |
|---|---|
| `name` | Template name. Must match the filename (without extension). |
| `record_types` | Comma-separated list of Person Record subtypes this template supports: `soldier`, `spouse`, `widow`. |
| `orientation` | `landscape` or `portrait` (or `any`). |
| `export_types` | Free-form tag list (e.g. `record_card`, `biography`, `analytics`). |
| `description` | One-liner shown in the export UI. |

Lines that don't match the `key: value` shape (or aren't a `//` comment)
end the metadata block. Templates with no metadata block are still
discovered, but use the filename as the name and have empty
record-type / orientation fields.

### Body

Below the metadata, the template is regular Typst. The data is
exposed as `sys.inputs` (a JSON object that `go-typst` populates
from the Go side).

The data shape (the canonical `TemplateData` from
`pkg/encode/encode.go`):

```typst
#let opts = sys.inputs.options      // PrintSettings
#let s = sys.inputs.soldier         // a single Soldier (for single-export templates)
#let branding = sys.inputs.branding // { archive_title, footer_text }
#let app = sys.inputs.app           // { version, build_identity }
```

The full fields of each are described in `pkg/encode/encode.go`.
For the most common case (a single-soldier record card):

```typst
#import "common/theme.typ"

#set page(
  paper: "us-letter",
  margin: theme.geometry.page_margin,
  header: align(center, text(size: theme.type-scale.header.size, weight: "bold", fill: theme.palette.text_primary)[
    #branding.archive_title
  ]),
)

#set text(
  font: ("Helvetica Neue", "Arial"),
  size: theme.type_scale.body.size,
  fill: theme.palette.text_primary,
)

#grid(
  columns: (theme.geometry.record_card_left_ratio, theme.geometry.column_gap, 1fr),
  [
    // Left column
    = section-title("Identity & Vital Details")
    #field-row("Display ID", s.display_id)
    ...
  ],
  [],
  [
    // Right column
    ...
  ]
)
```

## Theme tokens

The `templates/common/theme.typ` file holds centralized design tokens
that every template can import. The values are derived from the
audit's `theme.json` deliverable (see
`docs/audit/layout-theming-token-schema.md`).

| Token | Type | Used for |
|---|---|---|
| `theme.palette.accent` | `rgb` | Section title color |
| `theme.palette.text_primary` | `rgb` | Body text |
| `theme.palette.text_secondary` | `rgb` | Labels |
| `theme.palette.divider` | `rgb` | Rules and borders |
| `theme.geometry.page_margin` | `dict` | Page margins |
| `theme.geometry.column_gap` | `length` | Gap between columns |
| `theme.type_scale.body.size` | `length` | Body text size |
| ... | | |

To change a color or font across all templates, edit `theme.typ`.
Every template that imports it picks up the change.

## Iteration workflow

1. Drop a `.typ` file in `templates/`.
2. Run `dixiedata-tune list-templates` to confirm it's discovered.
3. Run `dixiedata-tune render --template <name> --record <id> --out out.pdf`
   against a real record to see the result.
4. Compare with the fpdf baseline:
   `dixiedata-tune compare --template <name> --record <id>` writes
   both PDFs to `compare/`.
5. Write notes in a sidecar `.md` file alongside the rendered PDF
   (e.g. `out.md` next to `out.pdf`). The next render of the same
   template+record will read this file as context.
6. Iterate. When the Typst output is "close enough to be recognizable"
   as the same export, ship the template.

## Annotation convention

The tuning tool uses sidecar `.md` files for feedback. The convention:

- Filename: `<template>_<record_id>.md` (or any `.md` file alongside the PDF).
- Format: free-form markdown.
- Most recent `.md` for a given `(template, record)` pair is the
  source of truth for the next render.

Example:

```markdown
## What looks right
- Header is correct.
- Two-column record card structure.

## What needs to change
- Section title color is too dark. Make it lighter.
- The "Display ID" field should appear first in the identity section.
- Need to add a "Burial Location" field to the identity section.
```

The agent (or the developer) reads this file before the next render
and incorporates the feedback.

## Common pitfalls

- **Font families.** Typst does not bundle Helvetica. Templates that
  reference "Helvetica Neue" will see a warning. Use the typst-bundled
  "Libertinus Serif" or system fonts.
- **Forward slashes.** Typst 0.15 requires forward slashes in file
  paths. Backslashes in `image("path\to\file")` will fail.
- **No backslashes in file paths.** Use forward slashes in any
  `image("...")` or `#include "..."` argument.
- **Fonts not bundled.** If your template needs a specific font, drop
  the `.ttf` files in `templates/common/fonts/` and register them
  with `#set text(font: "MyFont", ...)`.
- **JSON read.** Templates read data via
  `#let data = read("data.json", encoding: none); #let data = json(data);`.
  This is the contract that `go-typst` expects.

## See also

- `docs/audit/typst-migration-plan.md` — the migration plan that
  introduced this template system.
- `docs/audit/layout-theming-token-schema.md` — the design token
  schema that `theme.typ` is derived from.
- `tools/tune/README.md` — the tuning tool documentation.
