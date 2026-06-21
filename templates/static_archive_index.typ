// metadata:
//   name: static_archive_index
//   record_types: []
//   orientation: landscape
//   export_types: [static_archive_index]
//   description: Static archive printable index page.
//
// Renders the printable page that goes at the front of the
// static archive export. The existing static-archive index
// is an interactive HTML page (see internal/archive/static_archive.go);
// this template produces a printable PDF index that can be
// viewed without JavaScript.
//
// The data shape:
//   - archive_title: string (e.g. "J. Morris's Civil War Research Archive")
//   - records: list[dict] - each record has:
//       - display_id: string
//       - display_name: string (e.g. "Mrs. Jane Doe")
//       - entry_type: string (e.g. "soldier")
//       - birth_year, death_year: int (optional)
//
// Visual: two-column index with section header, alphabetical
// listing, and a footer with archive metadata. Mirrors the
// existing HTML index's data layout; visual fidelity to the
// HTML is NOT required (per PRD: separate audit for static
// archive is forthcoming).

#import "common/record_card.typ": *

#let data = read("data.json", encoding: none)
#let data = json(data)

#let archive-title = data.at("archive_title", default: "DixieData Archive")
#let records = data.at("records", default: ())
#let opts = data.at("options", default: (:))
#let branding = data.at("branding", default: (:))

#let is-landscape = true  // always landscape for the index
#set page(..page-params(is-landscape, branding, opts))
#set text(font: "Arial", size: 9pt, fill: theme.palette.text_primary)
#set par(leading: 0.45em)

// Title block.
#text(
  size: 20pt,
  font: ("Times New Roman", "Liberation Serif", "DejaVu Serif"),
  weight: "bold",
)[#archive-title]
#v(0.2em)
#text(size: 10pt, fill: theme.palette.text_secondary)[
  Printable archive index. The full interactive archive is
  available in the HTML companion.
]
#v(0.6em)

// Section title.
#text(size: 11pt, weight: "bold", fill: theme.palette.accent)[Records]
#v(0.4em)

#if records.len() == 0 [
  #set text(size: 9pt)
  No records in this archive.
] else [
  // Two-column index. Each row: display_id, display_name, entry type.
  #set text(size: 9pt)
  #for record in records [
    #block[
      *#record.at("display_id", default: "")* — #record.at("display_name", default: "") (#record.at("entry_type", default: ""))
      #v(0.15em)
    ]
  ]
]
