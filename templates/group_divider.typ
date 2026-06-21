// metadata:
//   name: group_divider
//   record_types: [soldier, widow, wife, linked_person]
//   orientation: portrait
//   export_types: [group_divider]
//   description: Group divider page (matches fpdf
//     writePDFGroupDividerPage).
//
// A single-page divider used between groups in a bulk export.
// Data shape:
//   - label: string (e.g. "Unit", "Pension State")
//   - value: string (e.g. "Co. A, 1st TX Cavalry")
//   - level: int (1-3, determines title size)
//   - options: PDFOptions
//   - branding: BrandingInfo

#import "common/record_card.typ": *

#let data = read("data.json", encoding: none)
#let data = json(data)

#let opts = data.at("options", default: (:))
#let branding = data.at("branding", default: (:))
#let label = data.at("label", default: "")
#let value = data.at("value", default: "")
#let level = data.at("level", default: 1)

#let is-landscape = detect-landscape(opts)
#set page(..page-params(is-landscape, branding, opts))
#set text(font: "Arial", size: 9pt, fill: theme.palette.text_primary)
#set par(leading: 0.45em)

// Match fpdf writePDFGroupDividerPage: title size scales with
// nesting level. fpdf uses 20pt for level 1 and 28 - 2*level for
// deeper levels; we replicate that formula.
#let title-size = calc.max(20pt, 28pt - 2pt * level)

#v(2em)
#text(
  size: 11pt,
  weight: "bold",
  fill: theme.palette.accent,
)[Grouped by #label]
#v(1em)
#text(
  size: title-size,
  font: ("Times New Roman", "Liberation Serif", "DejaVu Serif"),
  weight: "bold",
  fill: theme.palette.text_primary,
)[#if value == none or value == "" { [(unknown)] } else [#value]]
#v(0.5em)
#text(size: 9pt, fill: theme.palette.text_secondary)[
  The following record pages belong to this section.
]
