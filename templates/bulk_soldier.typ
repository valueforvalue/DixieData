// metadata:
//   name: bulk_soldier
//   record_types: [bulk]
//   orientation: any
//   export_types: [bulk_export]
//   description: Bulk Printable Archive — single sorted PDF containing
//     every record in the archive. Replaces the per-record folder of
//     PDFs that the typst path emitted after the fpdf→typst migration.
//
// When PrintSettings.GroupBy* is set, Go partitions the sorted
// records into groups (one per axis value) and emits a divider
// page between groups. Without grouping, the template loops over
// data["soldiers"] as before.
//
// The single-record templates (soldier_landscape.typ etc.) are kept
// for single-record exports. This template emits one continuous PDF
// with a pagebreak between records (or groups) and a divider page
// before each group when grouping is active.

#import "common/record_card.typ": *
#import "common/theme.typ": *

#let data = read("data.json", encoding: none)
#let data = json(data)

#let opts = data.at("options", default: (:))
#let branding = data.at("branding", default: (:))

#let is-landscape = detect-landscape(opts)
#set page(..page-params(is-landscape, branding, opts))
#set text(font: "Arial", size: 9pt, fill: theme.palette.text_primary)
#set par(leading: 0.45em)

#let variant-for(s) = {
  let entry = lower(s.at("entry_type", default: "soldier"))
  if entry == "widow" or entry == "wife" { "spouse" } else { entry }
}

// render-divider-page emits a single divider page sized to the
// current orientation. Matches the layout of
// templates/group_divider.typ but inlined here so the bulk
// template stays self-contained for the loop.
#let render-divider-page(label, value, level) = {
  let title-size = calc.max(20pt, 28pt - 2pt * level)
  v(2em)
  text(
    size: 11pt,
    weight: "bold",
    fill: theme.palette.accent,
  )[Grouped by #label]
  v(1em)
  text(
    size: title-size,
    font: ("Times New Roman", "Liberation Serif", "DejaVu Serif"),
    weight: "bold",
    fill: theme.palette.text_primary,
  )[#value]
  v(0.5em)
  text(size: 9pt, fill: theme.palette.text_secondary)[
    The following record pages belong to this section.
  ]
}

// render-group walks one group: optionally a divider page, then
// each soldier's record card with a pagebreak between soldiers.
// The first record of the entire export starts on page 1 with no
// preceding pagebreak (the page setup above sized it).
#let render-group(group, is-first) = {
  let soldiers = group.at("soldiers", default: ())
  if not is-first and group.at("axis", default: "") != "" {
    pagebreak()
    render-divider-page(
      group.at("label", default: ""),
      group.at("value", default: "(unknown)"),
      group.at("level", default: 1),
    )
  }
  for i in range(soldiers.len()) {
    if i > 0 { pagebreak() }
    render-record-card(opts, branding, soldiers.at(i), variant-for(soldiers.at(i)))
  }
}

#let groups = data.at("groups", default: none)

#if groups == none {
  // No grouping: fall back to the flat array.
  let soldiers = data.at("soldiers", default: ())
  for i in range(soldiers.len()) {
    if i > 0 { pagebreak() }
    render-record-card(opts, branding, soldiers.at(i), variant-for(soldiers.at(i)))
  }
} else {
  for gi in range(groups.len()) {
    render-group(groups.at(gi), gi == 0)
  }
}