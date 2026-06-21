// metadata:
//   name: spouse_landscape
//   record_types: [wife, linked_person]
//   orientation: landscape
//   export_types: [record_card]
//   description: Spouse/linked-person record card (landscape).
//
// Covers both "wife" and "linked_person" entry types. They share
// the same structure: a person record linked to a soldier record
// (e.g., a soldier's wife or sister). The variant string
// "spouse" enables the same field visibility rules as "widow"
// (always show Linked Spouse Record, Maiden Name, Rank fields).

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

#render-record-card(opts, branding, s, "spouse")
