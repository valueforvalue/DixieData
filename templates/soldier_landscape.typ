// metadata:
//   name: soldier_landscape
//   record_types: [soldier]
//   orientation: landscape
//   export_types: [record_card]
//   description: Standard Soldier record card (landscape).
//
// All real logic lives in templates/common/record_card.typ. This
// file loads the data, applies page setup at document scope
// (required because #set rules must be at the document root in
// Typst, not inside function bodies), then dispatches to the
// shared helper.

#import "common/record_card.typ": *
#import "common/debug_grid.typ": render-debug-grid

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
#render-debug-grid(opts)
