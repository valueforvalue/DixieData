// metadata:
//   name: biography_appendix
//   record_types: [soldier, widow, wife, linked_person]
//   orientation: portrait
//   export_types: [biography_appendix]
//   description: Full biography appendix page (matches fpdf
//     writePrintableBiographyAppendixPage).
//
// Renders a single-page "Full Biography Appendix" for one record.
// Used by the bulk export when the user enables the
// "Append full biography page" toggle. Mirrors the
// fpdf path's writeFullBiographyPage layout: Times serif 20pt
// title, "DXD-XXXXX | EntryType | Full Biography Appendix"
// subtitle, "Biography" section title, then the full biography
// text.
//
// Pulls the standard helpers from common/record_card.typ.

#import "common/record_card.typ": *

#let data = read("data.json", encoding: none)
#let data = json(data)

#let s = data.at("soldier", default: none)
#let opts = data.at("options", default: (:))
#let branding = data.at("branding", default: (:))

#let is-landscape = detect-landscape(opts)
#set page(..page-params(is-landscape, branding, opts))
#set text(font: "Arial", size: 9pt, fill: theme.palette.text_primary)
#set par(leading: 0.45em)

#let display-id = s.at("display_id", default: "")
#let entry-type-raw = s.at("entry_type", default: "")
#let suffix = s.at("suffix", default: "")
#let biography = s.at("biography", default: "")

#text(
  size: 20pt,
  font: ("Times New Roman", "Liberation Serif", "DejaVu Serif"),
  weight: "bold",
)[
  #compose-name(s)
  #if suffix != "" [, #suffix]
]

#v(0.2em)
#text(size: 10pt, fill: theme.palette.text_secondary)[
  #display-id | #entry-type-label(entry-type-raw) | Full Biography Appendix
]

#v(0.8em)

#text(size: 9pt, weight: "bold", fill: theme.palette.accent)[Biography]
#v(0.4em)

#if biography != none and biography.trim() != "" [
  #set text(size: 9pt)
  #biography
] else [
  #set text(size: 9pt, fill: theme.palette.text_secondary)
  No biography recorded for this person.
]
