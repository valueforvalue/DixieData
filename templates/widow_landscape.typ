// metadata:
//   name: widow_landscape
//   record_types: [widow]
//   orientation: landscape
//   export_types: [record_card]
//   description: Widow record card (landscape).
//
// The widow variant differs from the soldier variant by:
//   - Always shows the Linked Spouse Record field (linking back
//     to the soldier record), even when spouse_name is blank
//   - Always shows Maiden Name even when blank
//   - Always shows the Rank In / Rank Out / Unit labels in the
//     service section (fpdf behavior: labels are always rendered,
//     even when blank). The widow typically has empty rank/unit
//     fields but the labels still appear.

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

#render-record-card(opts, branding, s, "widow")
