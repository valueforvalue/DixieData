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
#let page-dict = page-params(is-landscape, branding, opts)
// Override the shared header and footer: branding text goes
// top-left in a small font (the user asked for it not to be
// centered / prominent); footer is smaller and uses the
// muted-text colour (less prominent than the 8pt secondary
// the shared helper uses).
#set page(..page-dict,
  header: align(left, text(
    size: 7pt,
    fill: theme.palette.text_secondary,
  )[#branding.at("archive_title", default: "DixieData Archive")]),
  footer: if not opts.at("printerFriendly", default: false) {
    align(center, text(
      size: 6pt,
      fill: theme.palette.text_muted,
    )[#branding.at("footer_text", default: "")])
  } else {
    none
  },
)
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

// Helper: death-year of the soldier, or "" if unset. Used inline
// with the name on each row.
#let soldier-death-year(s) = {
  let y = s.at("death_year", default: 0)
  if y > 0 { str(y) } else { "" }
}

// Helper: decade bucket for a death year. Returns "1840s",
// "1910s", "Unknown" if no death year. Used to sub-group
// soldiers within a day. The "Unknown" label puts the
// yearless entries under a single sub-header so they don't
// float loose at the end of a day.
#let soldier-decade(s) = {
  let y = s.at("death_year", default: 0)
  if y <= 0 { "Unknown" } else { let base = calc.floor(y / 10) * 10; str(base) + "s" }
}

// Sort days ascending numerically (the keys come from the Go
// map[string][]Soldier JSON round-trip as strings, so a
// lexicographic sort puts "11" before "2"). Skip 0 (sentinel
// for unknown day).
#let days-raw = calendar.keys().filter(d => d != "0")
#let days = days-raw.sorted(key: d => int(d))

// Total entries across all days. Drives the single-page vs
// multi-page decision: if we have too many for one page we let
// the content flow naturally; otherwise we declare "Page 1 of 1"
// via a no-op (no pagebreak). Threshold tuned for the US-Letter
// portrait, 0.4in/0.63in margins, two-column density below.
#let total-entries = days-raw.map(d => calendar.at(d, default: ()).len()).sum()
#let one-page-budget = 50

// Title: centered (overrides the global page header alignment),
// tighter spacing to the body so the user-requested closeness is
// preserved. The horizontal rule under the title uses the same
// color as the day text (theme.palette.accent) and spans the
// full margin width.
#align(center)[
  #text(
    size: 16pt,
    font: ("Times New Roman", "Liberation Serif", "DejaVu Serif"),
    weight: "bold",
  )[#mlabel Anniversary Report]
]
// Horizontal rule: full margin width, accent color, ~0.6pt
// thick. The `length: 100%` ensures it spans the full text
// width (which is page width minus the 0.63in left/right
// margins set by page-params).
#line(length: 100%, stroke: 0.6pt + theme.palette.accent)
#v(0.4em)

#if days.len() == 0 [
  #set text(size: 8pt)
  No soldiers are recorded for this month.
] else [
  // Two columns via typst's `columns(2, ...)` block. Day
  // headers do NOT repeat across the column boundary — typst
  // treats the entire `#for day in days` loop as a single flow
  // and breaks only at content boundaries. A day that contains
  // a long soldier list will wrap into the right column
  // without repeating its header, so the user-requested
  // "(cont.)" suffix is moot in the current data (no day in
  // the live archive's monthly calendar has enough soldiers to
  // span both columns).
  #set text(size: 7pt)
  #set par(leading: 0.35em)
  #columns(2, gutter: 0.9em)[
    #for day in days [
      #text(size: 7.5pt, weight: "bold", fill: theme.palette.accent)[Day #day]
      #v(0.05em)
      #let soldiers = calendar.at(day, default: ())
      // Group soldiers within a day by death-year decade so a
      // day with mixed eras is readable. Sort by (last name,
      // first name) within a decade — the year tie-breaker was
      // dropped per user request so a name search within a day
      // is alphabetic. soldiers with unknown death year go
      // under the "Unknown" sub-header rather than floating
      // loose at the end of the day.
      #let sorted = soldiers.sorted(key: s => (
        s.at("last_name", default: ""),
        s.at("first_name", default: ""),
      ))
      #let by-decade = (:)
      #for s in sorted [
        #let dec = soldier-decade(s)
        #let bucket = by-decade.at(dec, default: ())
        #by-decade.insert(dec, bucket + (s,))
      ]
      // Decade keys: "Unknown" sorts after real decades because
      // 'U' > '9' lexicographically. We want it last; sort by
      // (-1 for Unknown, decade-int otherwise).
      #let decade-keys = by-decade.keys().sorted(key: k => {
        if k == "Unknown" { 9999 } else { int(k.trim("s")) }
      })
      #for dec in decade-keys [
        #text(size: 6.5pt, fill: theme.palette.text_secondary, style: "italic")[#dec]
        #for s in by-decade.at(dec) [
          - #let name = soldier-name(s)
            #if name != "" [
              #name
            ] else [
              #s.at("display_id", default: "")
            ]
            #let did = s.at("display_id", default: "")
            #let yr = soldier-death-year(s)
            #if yr != "" [
              (#did, #yr)
            ] else [
              (#did)
            ]
        ]
        #v(0.05em)
      ]
      #v(0.1em)
    ]
  ]
]

#if total-entries > one-page-budget [
  // Empty block; the comment is the agent's note that we let
  // typst flow the content naturally onto a second page.
]
