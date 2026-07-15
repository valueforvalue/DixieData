// metadata:
//   name: analytics_summary
//   record_types: []
//   orientation: portrait
//   export_types: [analytics_summary]
//   description: Archive Summary Report (matches fpdf
//     ExportAnalyticsSummaryPDF).
//
// Renders archive analytics. The data shape:
//   - snapshot: AnalyticsSnapshot
//   - options: PDFOptions
//   - branding: BrandingInfo
//
// Sections match the fpdf layout:
//   - Record Types (Soldiers, Spouses (Wives & Widows),
//     Linked People)
//   - Top Cemeteries
//   - Confederate Home Participation (Status + Names)
//   - Pension Distribution
//   - Unit Representation
//   - Chronological Overview (Birth + Death decades)

#import "common/record_card.typ": *

#let data = read("data.json", encoding: none)
#let data = json(data)

#let opts = data.at("options", default: (:))
#let branding = data.at("branding", default: (:))
#let snapshot = data.at("snapshot", default: (:))

#let is-landscape = detect-landscape(opts)
#set page(..page-params(is-landscape, branding, opts))
#set text(font: "Arial", size: 9pt, fill: theme.palette.text_primary)
#set par(leading: 0.45em)

// Helper: render a bullet list from a list of dicts, each with
// `label` and `count` keys (matching the JSON shape of
// records.AnalyticsCount). The data is normalized to this shape
// by `normalize-rows` so all call sites pass the same type.
#let bullet-list(rows, empty-msg) = {
  if rows.len() == 0 [
    #set text(size: 9pt, fill: theme.palette.text_secondary)
    #empty-msg
  ] else [
    #set text(size: 9pt)
    #for row in rows [
      + [#row.label: #row.count]
      #v(0.2em)
    ]
  ]
}

// normalize-rows converts a list of mixed-shape rows to a
// uniform list of (label, count) dicts. Accepts either:
//   - dicts with `label` and `count` (the JSON shape), or
//   - arrays of two elements (label, count) (the legacy shape).
// Returns a list of dicts.
#let normalize-rows(rows) = {
  let out = ()
  for row in rows {
    if type(row) == dictionary {
      out.push(row)
    } else if type(row) == array and row.len() == 2 {
      out.push((label: str(row.at(0)), count: str(row.at(1))))
    }
  }
  out
}

// Title. Issue #581: dropped the 38-word subtitle that named
// every section in prose; the section headings below carry the
// same information without duplication.
#text(
  size: 20pt,
  font: ("Times New Roman", "Liberation Serif", "DejaVu Serif"),
  weight: "bold",
)[Archive Summary Report]
#v(0.6em)

// Record Types.
#text(size: 9pt, weight: "bold", fill: theme.palette.accent)[Record Types]
#v(0.3em)
#bullet-list(
  normalize-rows((
    (label: "Soldiers", count: str(snapshot.at("record_types", default: (:)).at("total_soldiers", default: 0))),
    (label: "Spouses (Wives & Widows)", count: str(snapshot.at("record_types", default: (:)).at("total_wives_widows", default: 0))),
    (label: "Linked People", count: str(snapshot.at("record_types", default: (:)).at("total_linked_people", default: 0))),
  )),
  "No records to summarise."
)
#v(0.5em)

// Top Cemeteries.
#text(size: 9pt, weight: "bold", fill: theme.palette.accent)[Top Cemeteries]
#v(0.3em)
#bullet-list(snapshot.at("cemetery_density", default: ()), "No burial locations are recorded yet.")
#v(0.5em)

// Confederate Home Participation.
// Issue #581: dropped the stacked "Status breakdown" and "Most
// frequent home names" sub-headings; the parent heading names
// the section and the bullets are self-explanatory.
#text(size: 9pt, weight: "bold", fill: theme.palette.accent)[Confederate Home Participation]
#v(0.3em)
#bullet-list(snapshot.at("confederate_home_status", default: ()), "No Confederate Home statuses are recorded yet.")
#v(0.3em)
#bullet-list(snapshot.at("confederate_home_names", default: ()), "No Confederate Home names are recorded yet.")
#v(0.5em)

// Pension Distribution.
#text(size: 9pt, weight: "bold", fill: theme.palette.accent)[Pension Distribution]
#v(0.3em)
#bullet-list(snapshot.at("pension_distribution", default: ()), "No pension states are recorded yet.")
#v(0.5em)

// Unit Representation.
#text(size: 9pt, weight: "bold", fill: theme.palette.accent)[Unit Representation]
#v(0.3em)
#bullet-list(snapshot.at("unit_representation", default: ()), "No units are recorded yet.")
#v(0.5em)

// Chronological Overview.
// Issue #581: dropped the stacked "Birth decades" and "Death
// decades" sub-headings; the bullet values are self-evidently
// decades (e.g. "1840s: 4").
#text(size: 9pt, weight: "bold", fill: theme.palette.accent)[Chronological Overview]
#v(0.3em)
#bullet-list(snapshot.at("birth_decade_distribution", default: ()), "No birth decades are recorded yet.")
#v(0.3em)
#bullet-list(snapshot.at("death_decade_distribution", default: ()), "No death decades are recorded yet.")
