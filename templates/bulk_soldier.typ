// metadata:
//   name: bulk_soldier
//   record_types: [soldier, widow, wife, linked_person]
//   orientation: any
//   export_types: [bulk_export]
//   description: Bulk Printable Archive — single sorted PDF containing
//     every record in the archive. Replaces the per-record folder of
//     PDFs that the typst path emitted after the fpdf→typst migration.
//
// The single-record templates (soldier_landscape.typ etc.) are kept
// for single-record exports. This template emits one continuous PDF
// with a pagebreak between records, sorted and (optionally) grouped
// per PrintSettings.SortBy / GroupBy* — see
// internal/archive/export_service.go::exportFullDatabasePDFViaRegistry
// for the Go side of the contract.

#import "common/record_card.typ": *

#let data = read("data.json", encoding: none)
#let data = json(data)

#let opts = data.at("options", default: (:))
#let branding = data.at("branding", default: (:))
#let soldiers = data.at("soldiers", default: ())

#let is-landscape = detect-landscape(opts)
#set page(..page-params(is-landscape, branding, opts))
#set text(font: "Arial", size: 9pt, fill: theme.palette.text_primary)
#set par(leading: 0.45em)

#let variant-for(s) = {
  let entry = lower(s.at("entry_type", default: "soldier"))
  if entry == "widow" or entry == "wife" { "spouse" } else { entry }
}

// Iterate every soldier and render the full record card, with a
// pagebreak between records. The first record starts on page 1 with
// no preceding pagebreak (the page setup above already sized it).
#for i in range(soldiers.len()) {
  let s = soldiers.at(i)
  render-record-card(opts, branding, s, variant-for(s))
  if i < soldiers.len() - 1 {
    pagebreak()
  }
}