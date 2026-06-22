// metadata:
//   name: anniversary
//   record_types: [soldier, widow, wife, linked_person]
//   orientation: portrait
//   export_types: [anniversary]
//   description: Monthly Anniversary Report (matches fpdf
//     ExportMonthlyAnniversaryPDF).
//
// Renders a single-page report of soldiers whose birth or
// death anniversary falls in the given month. The data shape
// comes from the fpdf ExportMonthlyAnniversaryPDF signature:
//   - month: int (1-12)
//   - calendar: dict[int -> list[Soldier]]
//   - options: PDFOptions
//   - branding: BrandingInfo

#import "common/record_card.typ": *

#let data = read("data.json", encoding: none)
#let data = json(data)

#let opts = data.at("options", default: (:))
#let branding = data.at("branding", default: (:))
#let month = data.at("month", default: 0)
#let calendar = data.at("calendar", default: (:))

#let is-landscape = detect-landscape(opts)
#set page(..page-params(is-landscape, branding, opts))
#set text(font: "Arial", size: 9pt, fill: theme.palette.text_primary)
#set par(leading: 0.45em)

#let month-names = (
  "1": "January",
  "2": "February",
  "3": "March",
  "4": "April",
  "5": "May",
  "6": "June",
  "7": "July",
  "8": "August",
  "9": "September",
  "10": "October",
  "11": "November",
  "12": "December",
)

#let mlabel = month-names.at(str(month), default: "Unknown")

// Title (Times serif 20pt, matching fpdf writePDFTitleBlock).
#text(
  size: 20pt,
  font: ("Times New Roman", "Liberation Serif", "DejaVu Serif"),
  weight: "bold",
)[#mlabel Anniversary Report]
#v(0.6em)

// Helper: assemble a soldier's display name from first/middle/last.
// The Go-side Soldier.DisplayName method isn't included in the
// JSON payload sent to typst, so the template composes it from
// the available fields. Returns "" when no name parts are set;
// the caller is expected to fall back to the display_id.
#let soldier-name(s) = {
  let parts = (
    s.at("first_name", default: ""),
    s.at("middle_name", default: ""),
    s.at("last_name", default: ""),
  ).filter(p => p != "")
  parts.join(" ")
}

// Sort days ascending numerically (the keys come from the Go
// map[string][]Soldier JSON round-trip as strings, so a
// lexicographic sort puts "11" before "2"). Skip 0 (sentinel
// for unknown day).
#let days-raw = calendar.keys().filter(d => d != "0")
#let days = days-raw.sorted(key: d => int(d))

#if days.len() == 0 [
  #set text(size: 9pt)
  No soldiers are recorded for this month.
] else [
  // For each day, render a section. Two columns so a 31-day
  // month with many anniversaries still fits on a single page.
  #set text(size: 9pt)
  #columns(2, gutter: 1.2em)[
    #for day in days [
      #text(size: 9pt, weight: "bold", fill: theme.palette.accent)[Day #day]
      #v(0.3em)
      #let soldiers = calendar.at(day, default: ())
      #for s in soldiers [
        - #let name = soldier-name(s)
          #if name != "" [
            #name
          ] else [
            #s.at("display_id", default: "")
          ]
          (#s.at("display_id", default: ""))
        #v(0.2em)
      ]
      #v(0.4em)
    ]
  ]
]
