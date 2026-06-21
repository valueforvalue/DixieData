// metadata:
//   name: soldier_portrait
//   record_types: [soldier]
//   orientation: portrait
//   export_types: [record_card]
//   description: Standard Soldier record card (portrait).
//
// Portrait variant of the soldier record card. The layout is
// single-column (vs the landscape two-column layout) because
// portrait pages are too narrow for two columns of label/value
// pairs. All sections stack vertically.

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

#render-record-card(opts, branding, s, "soldier")
