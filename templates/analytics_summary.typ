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

// Helper: render a bullet list from a list of (label, value) pairs,
// or show a fallback message if the list is empty.
#let bullet-list(rows, empty-msg) = {
  if rows.len() == 0 [
    #set text(size: 9pt, fill: theme.palette.text_secondary)
    #empty-msg
  ] else [
    #set text(size: 9pt)
    #for row in rows [
      - #row.at(0, default: ""): #row.at(1, default: "")
      #v(0.2em)
    ]
  ]
}

// Title.
#text(
  size: 20pt,
  font: ("Times New Roman", "Liberation Serif", "DejaVu Serif"),
  weight: "bold",
)[Archive Summary Report]
#v(0.2em)
#text(size: 10pt, fill: theme.palette.text_secondary)[
  High-level archive analytics covering burial density,
  Confederate Home participation, record types, pension
  geography, unit representation, and decade trends.
]
#v(0.6em)

// Record Types.
#text(size: 9pt, weight: "bold", fill: theme.palette.accent)[Record Types]
#v(0.3em)
#bullet-list(
  (
    ("Soldiers", str(snapshot.at("record_types", default: (:)).at("total_soldiers", default: 0))),
    ("Spouses (Wives & Widows)", str(snapshot.at("record_types", default: (:)).at("total_wives_widows", default: 0))),
    ("Linked People", str(snapshot.at("record_types", default: (:)).at("total_linked_people", default: 0))),
  ),
  "No records to summarise."
)
#v(0.5em)

// Top Cemeteries.
#text(size: 9pt, weight: "bold", fill: theme.palette.accent)[Top Cemeteries]
#v(0.3em)
#bullet-list(snapshot.at("cemetery_density", default: ()), "No burial locations are recorded yet.")
#v(0.5em)

// Confederate Home Participation.
#text(size: 9pt, weight: "bold", fill: theme.palette.accent)[Confederate Home Participation]
#v(0.3em)
#text(size: 9pt, weight: "bold")[Status breakdown]
#v(0.2em)
#bullet-list(snapshot.at("confederate_home_status", default: ()), "No Confederate Home statuses are recorded yet.")
#v(0.3em)
#text(size: 9pt, weight: "bold")[Most frequent home names]
#v(0.2em)
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
#text(size: 9pt, weight: "bold", fill: theme.palette.accent)[Chronological Overview]
#v(0.3em)
#text(size: 9pt, weight: "bold")[Birth decades]
#v(0.2em)
#bullet-list(snapshot.at("birth_decade_distribution", default: ()), "No birth decades are recorded yet.")
#v(0.3em)
#text(size: 9pt, weight: "bold")[Death decades]
#v(0.2em)
#bullet-list(snapshot.at("death_decade_distribution", default: ()), "No death decades are recorded yet.")
